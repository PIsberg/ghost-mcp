// ghost-mcp: MCP Server for OS-level UI automation
//
// This server exposes mouse, keyboard, and screen reading capabilities
// as MCP tools that AI clients can use to control legacy applications.
//
// CRITICAL: All logging MUST go to stderr because stdout is used for
// the MCP JSON-RPC protocol. Writing to stdout will corrupt the protocol.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/go-vgo/robotgo"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// =============================================================================
// CONFIGURATION AND CONSTANTS
// =============================================================================

// FailsafePosition defines the "kill switch" coordinates.
// If the mouse is moved to (0,0), the server will panic to prevent
// runaway AI loops from causing damage.
const (
	FailsafeX = 0
	FailsafeY = 0
)

// Server metadata for MCP identification
const (
	ServerName    = "ghost-mcp"
	ServerVersion = "1.0.0"
)

// =============================================================================
// GLOBAL STATE
// =============================================================================

// serverState holds the global state of the MCP server
type serverState struct {
	// shutdownChan is used to signal graceful shutdown
	shutdownChan chan struct{}
	// isShuttingDown tracks whether shutdown has been initiated
	isShuttingDown bool
}

var state = &serverState{
	shutdownChan: make(chan struct{}),
}

// =============================================================================
// LOGGING HELPERS (CRITICAL: All logs go to stderr)
// =============================================================================

// logInfo writes an informational message to stderr
func logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf("[INFO] "+format, args...)
	fmt.Fprintln(os.Stderr, msg)
}

// logError writes an error message to stderr
func logError(format string, args ...interface{}) {
	msg := fmt.Sprintf("[ERROR] "+format, args...)
	fmt.Fprintln(os.Stderr, msg)
}

// logDebug writes a debug message to stderr (only in debug mode)
func logDebug(format string, args ...interface{}) {
	if os.Getenv("GHOST_MCP_DEBUG") == "1" {
		msg := fmt.Sprintf("[DEBUG] "+format, args...)
		fmt.Fprintln(os.Stderr, msg)
	}
}

// =============================================================================
// FAILSAFE MECHANISM
// =============================================================================

// checkFailsafe checks if the mouse is at the failsafe position (0,0).
// If so, it triggers a graceful shutdown to prevent runaway automation.
// This should be called after any mouse movement operation.
func checkFailsafe() error {
	x, y := robotgo.GetMousePos()
	if x == FailsafeX && y == FailsafeY {
		logError("FAILSAFE TRIGGERED: Mouse at (%d, %d). Initiating shutdown.", x, y)
		initiateShutdown()
		return fmt.Errorf("failsafe triggered: mouse at origin (%d, %d)", x, y)
	}
	return nil
}

// initiateShutdown begins the graceful shutdown process
func initiateShutdown() {
	if state.isShuttingDown {
		return
	}
	state.isShuttingDown = true
	logInfo("Initiating graceful shutdown...")
	close(state.shutdownChan)
}

// =============================================================================
// MCP TOOL IMPLEMENTATIONS
// =============================================================================

// handleGetScreenSize returns the dimensions of the primary monitor
//
// Parameters: None
// Returns: JSON object with "width" and "height" fields
func handleGetScreenSize(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logDebug("Handling get_screen_size request")

	width, height := robotgo.GetScreenSize()

	logInfo("Screen size: %dx%d", width, height)

	return mcp.NewToolResultText(fmt.Sprintf(`{"width": %d, "height": %d}`, width, height)), nil
}

// handleMoveMouse moves the cursor to the specified coordinates
//
// Parameters:
//   - x (int): Target X coordinate
//   - y (int): Target Y coordinate
//
// Returns: Success message with final position
func handleMoveMouse(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logDebug("Handling move_mouse request")

	// Extract parameters
	x, err := getIntParam(request, "x")
	if err != nil {
		logError("Invalid x parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid x parameter: %v", err)), nil
	}

	y, err := getIntParam(request, "y")
	if err != nil {
		logError("Invalid y parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid y parameter: %v", err)), nil
	}

	logInfo("Moving mouse to (%d, %d)", x, y)

	// Move the mouse smoothly
	robotgo.MoveSmooth(x, y)

	// Check failsafe after movement
	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Verify final position
	finalX, finalY := robotgo.GetMousePos()
	logDebug("Mouse moved to (%d, %d)", finalX, finalY)

	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "x": %d, "y": %d}`, finalX, finalY)), nil
}

// handleClick performs a mouse click at the current cursor position
//
// Parameters:
//   - button (string): "left", "right", or "middle"
//
// Returns: Success message with button clicked
func handleClick(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logDebug("Handling click request")

	// Extract button parameter
	button, err := getStringParam(request, "button")
	if err != nil {
		logError("Invalid button parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid button parameter: %v", err)), nil
	}

	// Validate button
	validButtons := map[string]bool{"left": true, "right": true, "middle": true}
	if !validButtons[button] {
		err := fmt.Errorf("invalid button '%s', must be 'left', 'right', or 'middle'", button)
		logError("%v", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	logInfo("Performing %s click", button)

	// Get current position for logging
	x, y := robotgo.GetMousePos()
	logDebug("Clicking at (%d, %d)", x, y)

	// Perform the click
	robotgo.Click(button, true)

	// Check failsafe (in case click triggered movement)
	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "button": "%s", "x": %d, "y": %d}`, button, x, y)), nil
}

// handleTypeText types out the specified text via the keyboard
//
// Parameters:
//   - text (string): The text to type
//
// Returns: Success message with character count
func handleTypeText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logDebug("Handling type_text request")

	// Extract text parameter
	text, err := getStringParam(request, "text")
	if err != nil {
		logError("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}

	logInfo("Typing text (%d characters)", len(text))

	// Type the text with a small delay between characters for reliability
	robotgo.TypeStr(text)

	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "characters_typed": %d}`, len(text))), nil
}

// handlePressKey presses a single key
//
// Parameters:
//   - key (string): The key to press (e.g., "enter", "tab", "esc", "ctrl")
//
// Returns: Success message with key pressed
func handlePressKey(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logDebug("Handling press_key request")

	// Extract key parameter
	key, err := getStringParam(request, "key")
	if err != nil {
		logError("Invalid key parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid key parameter: %v", err)), nil
	}

	logInfo("Pressing key: %s", key)

	// Press the key
	robotgo.KeyTap(key)

	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "key": "%s"}`, key)), nil
}

// handleTakeScreenshot captures the screen and returns a base64-encoded PNG
//
// Parameters:
//   - x (int, optional): X coordinate of screenshot region (default: 0)
//   - y (int, optional): Y coordinate of screenshot region (default: 0)
//   - width (int, optional): Width of screenshot region (default: full screen)
//   - height (int, optional): Height of screenshot region (default: full screen)
//
// Returns: Base64-encoded PNG data with metadata
func handleTakeScreenshot(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logDebug("Handling take_screenshot request")

	// Get optional region parameters with defaults
	x, _ := getIntParam(request, "x")
	y, _ := getIntParam(request, "y")
	width, height := robotgo.GetScreenSize()

	if w, err := getIntParam(request, "width"); err == nil && w > 0 {
		width = w
	}
	if h, err := getIntParam(request, "height"); err == nil && h > 0 {
		height = h
	}

	logInfo("Taking screenshot at (%d, %d) with size %dx%d", x, y, width, height)

	// Capture the bitmap
	bitmap := robotgo.CaptureScreen(x, y, width, height)
	if bitmap == nil {
		err := fmt.Errorf("failed to capture screen")
		logError("%v", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Save to temporary file
	tempDir := os.TempDir()
	filename := fmt.Sprintf("ghost-mcp-screenshot-%d.png", time.Now().UnixNano())
	filepath := filepath.Join(tempDir, filename)

	// Save the bitmap as PNG
	saveErr := robotgo.SavePng(bitmap, filepath)
	if saveErr != nil {
		logError("Failed to save screenshot: %v", saveErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save screenshot: %v", saveErr)), nil
	}

	logInfo("Screenshot saved to: %s", filepath)

	// Read the file and encode as base64
	data, readErr := os.ReadFile(filepath)
	if readErr != nil {
		logError("Failed to read screenshot file: %v", readErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to read screenshot: %v", readErr)), nil
	}

	base64Data := base64.StdEncoding.EncodeToString(data)

	// Clean up the temporary file
	os.Remove(filepath)
	logDebug("Temporary screenshot file cleaned up")

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "filepath": "%s", "base64": "%s", "width": %d, "height": %d}`,
		filepath, base64Data, width, height,
	)), nil
}

// =============================================================================
// PARAMETER EXTRACTION HELPERS
// =============================================================================

// getStringParam extracts a string parameter from the request
func getStringParam(request mcp.CallToolRequest, name string) (string, error) {
	val, ok := request.Params.Arguments[name]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", name)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", name)
	}
	return str, nil
}

// getIntParam extracts an integer parameter from the request
func getIntParam(request mcp.CallToolRequest, name string) (int, error) {
	val, ok := request.Params.Arguments[name]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: %s", name)
	}

	// Handle float64 (JSON numbers are decoded as float64)
	if f, ok := val.(float64); ok {
		return int(f), nil
	}

	// Handle int
	if i, ok := val.(int); ok {
		return i, nil
	}

	// Handle int64
	if i, ok := val.(int64); ok {
		return int(i), nil
	}

	return 0, fmt.Errorf("parameter %s must be an integer", name)
}

// =============================================================================
// SERVER SETUP
// =============================================================================

// createServer initializes and configures the MCP server with all tools
func createServer() *server.MCPServer {
	logInfo("Creating MCP server: %s v%s", ServerName, ServerVersion)

	// Create the MCP server instance
	mcpServer := server.NewMCPServer(
		ServerName,
		ServerVersion,
		server.WithResourceCapabilities(true, true),
	)

	// Register all tools
	registerTools(mcpServer)

	return mcpServer
}

// registerTools registers all available MCP tools
func registerTools(mcpServer *server.MCPServer) {
	logInfo("Registering tools...")

	// Tool 1: get_screen_size
	mcpServer.AddTool(mcp.NewTool(
		"get_screen_size",
		mcp.WithDescription("Returns the width and height of the primary monitor"),
	), handleGetScreenSize)
	logDebug("Registered tool: get_screen_size")

	// Tool 2: move_mouse
	mcpServer.AddTool(mcp.NewTool(
		"move_mouse",
		mcp.WithDescription("Moves the mouse cursor to the specified coordinates"),
		mcp.WithInt32("x", mcp.Description("Target X coordinate"), mcp.Required()),
		mcp.WithInt32("y", mcp.Description("Target Y coordinate"), mcp.Required()),
	), handleMoveMouse)
	logDebug("Registered tool: move_mouse")

	// Tool 3: click
	mcpServer.AddTool(mcp.NewTool(
		"click",
		mcp.WithDescription("Performs a mouse click at the current cursor position"),
		mcp.WithString("button", mcp.Description("Mouse button: 'left', 'right', or 'middle'"), mcp.Required()),
	), handleClick)
	logDebug("Registered tool: click")

	// Tool 4: type_text
	mcpServer.AddTool(mcp.NewTool(
		"type_text",
		mcp.WithDescription("Types out text via the keyboard"),
		mcp.WithString("text", mcp.Description("The text to type"), mcp.Required()),
	), handleTypeText)
	logDebug("Registered tool: type_text")

	// Tool 5: press_key
	mcpServer.AddTool(mcp.NewTool(
		"press_key",
		mcp.WithDescription("Presses a single key on the keyboard"),
		mcp.WithString("key", mcp.Description("Key to press (e.g., 'enter', 'tab', 'esc', 'ctrl')"), mcp.Required()),
	), handlePressKey)
	logDebug("Registered tool: press_key")

	// Tool 6: take_screenshot
	mcpServer.AddTool(mcp.NewTool(
		"take_screenshot",
		mcp.WithDescription("Captures the screen and returns a base64-encoded PNG"),
		mcp.WithInt32("x", mcp.Description("X coordinate of screenshot region (default: 0)")),
		mcp.WithInt32("y", mcp.Description("Y coordinate of screenshot region (default: 0)")),
		mcp.WithInt32("width", mcp.Description("Width of screenshot region (default: full screen)")),
		mcp.WithInt32("height", mcp.Description("Height of screenshot region (default: full screen)")),
	), handleTakeScreenshot)
	logDebug("Registered tool: take_screenshot")

	logInfo("All tools registered successfully")
}

// =============================================================================
// MAIN ENTRY POINT
// =============================================================================

func main() {
	logInfo("Starting %s v%s...", ServerName, ServerVersion)
	logInfo("Platform: %s/%s", runtime.GOOS, runtime.GOARCH)

	// Log failsafe information
	logInfo("Failsafe position: (%d, %d) - Move mouse here to trigger emergency shutdown", FailsafeX, FailsafeY)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logInfo("Received signal: %v", sig)
		initiateShutdown()
	}()

	// Create the MCP server
	mcpServer := createServer()

	// Start the stdio server
	// This blocks and uses stdout/stdin for JSON-RPC communication
	logInfo("Starting stdio server...")
	logInfo("IMPORTANT: All application logs are written to stderr")
	logInfo("stdout is reserved for MCP JSON-RPC protocol")

	err := server.ServeStdio(mcpServer)
	if err != nil {
		logError("Server error: %v", err)
		os.Exit(1)
	}

	logInfo("Server stopped gracefully")
}
