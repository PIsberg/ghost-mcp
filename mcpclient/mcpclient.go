// mcpclient - MCP client helper for integration tests
//
// This package provides a simple MCP client that can connect to
// the ghost-mcp server via stdio and call its tools for testing.
package mcpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// JSON-RPC message types
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type ToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

// Client represents an MCP client connected via stdio
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner
	mu      sync.Mutex
	nextID  int64
	timeout time.Duration
	closed  bool
}

// Config holds client configuration
type Config struct {
	BinaryPath string        // Path to ghost-mcp binary
	Timeout    time.Duration // Default timeout for tool calls
	Env        []string      // Environment variables
}

// NewClient creates a new MCP client and starts the server process
func NewClient(config Config) (*Client, error) {
	if config.BinaryPath == "" {
		config.BinaryPath = "./ghost-mcp.exe"
		if _, err := os.Stat("./ghost-mcp"); err == nil {
			config.BinaryPath = "./ghost-mcp"
		}
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	cmd := exec.Command(config.BinaryPath)
	cmd.Env = append(os.Environ(), config.Env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Stderr goes to our stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	client := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		scanner: bufio.NewScanner(stdout),
		timeout: config.Timeout,
		nextID:  1,
	}

	// Give the server a moment to initialize
	time.Sleep(100 * time.Millisecond)

	return client, nil
}

// Close shuts down the MCP server process
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Try graceful shutdown first
	c.cmd.Process.Signal(os.Interrupt)

	// Wait briefly, then force kill
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		c.cmd.Process.Kill()
		return <-done
	}
}

// CallTool calls an MCP tool with the given arguments
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	params := ToolCallParams{
		Name:      name,
		Arguments: args,
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	request := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params:  paramsJSON,
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	if _, err := c.stdin.Write(append(requestJSON, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response with timeout
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	responseChan := make(chan *JSONRPCResponse, 1)
	errChan := make(chan error, 1)

	go func() {
		if c.scanner.Scan() {
			var response JSONRPCResponse
			if err := json.Unmarshal(c.scanner.Bytes(), &response); err != nil {
				errChan <- fmt.Errorf("failed to parse response: %w", err)
				return
			}
			responseChan <- &response
		} else {
			errChan <- fmt.Errorf("no response from server: %w", c.scanner.Err())
		}
	}()

	var response *JSONRPCResponse
	select {
	case response = <-responseChan:
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for response")
	}

	if response.Error != nil {
		return &ToolResult{
			IsError: true,
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: response.Error.Message}},
		}, nil
	}

	var result ToolResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// CallToolString is a convenience method for calling tools and parsing text results
func (c *Client) CallToolString(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	result, err := c.CallTool(ctx, name, args)
	if err != nil {
		return "", err
	}

	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("tool error")
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty result")
	}

	return result.Content[0].Text, nil
}

// Helper methods for common tool calls

// GetScreenSize returns the screen dimensions
func (c *Client) GetScreenSize(ctx context.Context) (width, height int, err error) {
	result, err := c.CallToolString(ctx, "get_screen_size", nil)
	if err != nil {
		return 0, 0, err
	}

	var dims struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	if err := json.Unmarshal([]byte(result), &dims); err != nil {
		return 0, 0, fmt.Errorf("failed to parse screen size: %w", err)
	}

	return dims.Width, dims.Height, nil
}

// MoveMouse moves the mouse to the specified coordinates
func (c *Client) MoveMouse(ctx context.Context, x, y int) error {
	_, err := c.CallToolString(ctx, "move_mouse", map[string]interface{}{
		"x": x,
		"y": y,
	})
	return err
}

// Click performs a mouse click
func (c *Client) Click(ctx context.Context, button string) error {
	_, err := c.CallToolString(ctx, "click", map[string]interface{}{
		"button": button,
	})
	return err
}

// TypeText types the specified text
func (c *Client) TypeText(ctx context.Context, text string) error {
	_, err := c.CallToolString(ctx, "type_text", map[string]interface{}{
		"text": text,
	})
	return err
}

// PressKey presses a single key
func (c *Client) PressKey(ctx context.Context, key string) error {
	_, err := c.CallToolString(ctx, "press_key", map[string]interface{}{
		"key": key,
	})
	return err
}

// ReadScreenText runs OCR on a screen region and returns extracted text and word positions.
func (c *Client) ReadScreenText(ctx context.Context, args map[string]interface{}) (text string, words []map[string]interface{}, err error) {
	result, err := c.CallToolString(ctx, "read_screen_text", args)
	if err != nil {
		return "", nil, err
	}

	var data struct {
		Text  string                   `json:"text"`
		Words []map[string]interface{} `json:"words"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return "", nil, fmt.Errorf("failed to parse read_screen_text result: %w", err)
	}

	return data.Text, data.Words, nil
}

// TakeScreenshot captures the screen and returns the base64 PNG data
func (c *Client) TakeScreenshot(ctx context.Context) (filepath, base64Data string, width, height int, err error) {
	result, err := c.CallToolString(ctx, "take_screenshot", nil)
	if err != nil {
		return "", "", 0, 0, err
	}

	var data struct {
		Filepath string `json:"filepath"`
		Base64   string `json:"base64"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return "", "", 0, 0, fmt.Errorf("failed to parse screenshot: %w", err)
	}

	return data.Filepath, data.Base64, data.Width, data.Height, nil
}
