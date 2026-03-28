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
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/ghost-mcp/internal/audit"
	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/transport"
	"github.com/ghost-mcp/internal/validate"
	"github.com/go-vgo/robotgo"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	FailsafeX = 0
	FailsafeY = 0
)

const (
	ServerName    = "ghost-mcp"
	ServerVersion = "1.0.0"
)

// TokenEnvVar is the environment variable for the authentication token.
const TokenEnvVar = "GHOST_MCP_TOKEN"

// =============================================================================
// GLOBAL STATE
// =============================================================================

type serverState struct {
	shutdownChan   chan struct{}
	isShuttingDown bool
}

var state = &serverState{
	shutdownChan: make(chan struct{}),
}

// =============================================================================
// FAILSAFE
// =============================================================================

func checkFailsafe() error {
	x, y := robotgo.GetMousePos()
	if x == FailsafeX && y == FailsafeY {
		logging.Error("FAILSAFE TRIGGERED: Mouse at (%d, %d). Initiating shutdown.", x, y)
		initiateShutdown()
		return fmt.Errorf("failsafe triggered: mouse at origin (%d, %d)", x, y)
	}
	return nil
}

func initiateShutdown() {
	if state.isShuttingDown {
		return
	}
	state.isShuttingDown = true
	logging.Info("Initiating graceful shutdown...")
	close(state.shutdownChan)
}

// =============================================================================
// MCP TOOL HANDLERS
// =============================================================================

func handleGetScreenSize(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling get_screen_size request")
	width, height := robotgo.GetScreenSize()
	logging.Info("Screen size: %dx%d", width, height)
	return mcp.NewToolResultText(fmt.Sprintf(`{"width": %d, "height": %d}`, width, height)), nil
}

func handleMoveMouse(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling move_mouse request")

	x, err := getIntParam(request, "x")
	if err != nil {
		logging.Error("Invalid x parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid x parameter: %v", err)), nil
	}
	y, err := getIntParam(request, "y")
	if err != nil {
		logging.Error("Invalid y parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid y parameter: %v", err)), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		logging.Error("Coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	logging.Info("Moving mouse to (%d, %d)", x, y)
	robotgo.MoveSmooth(x, y)

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	finalX, finalY := robotgo.GetMousePos()
	logging.Debug("Mouse moved to (%d, %d)", finalX, finalY)
	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "x": %d, "y": %d}`, finalX, finalY)), nil
}

func handleClick(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling click request")

	button, err := getStringParam(request, "button")
	if err != nil {
		logging.Error("Invalid button parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid button parameter: %v", err)), nil
	}

	validButtons := map[string]bool{"left": true, "right": true, "middle": true}
	if !validButtons[button] {
		err := fmt.Errorf("invalid button '%s', must be 'left', 'right', or 'middle'", button)
		logging.Error("%v", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	logging.Info("Performing %s click", button)
	x, y := robotgo.GetMousePos()
	logging.Debug("Clicking at (%d, %d)", x, y)
	robotgo.Click(button, true)

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "button": "%s", "x": %d, "y": %d}`, button, x, y)), nil
}

func handleTypeText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling type_text request")

	text, err := getStringParam(request, "text")
	if err != nil {
		logging.Error("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}

	if err := validate.Text(text); err != nil {
		logging.Error("Text validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text: %v", err)), nil
	}

	logging.Info("Typing text (%d characters)", len(text))
	robotgo.TypeStr(text)
	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "characters_typed": %d}`, len(text))), nil
}

func handlePressKey(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling press_key request")

	key, err := getStringParam(request, "key")
	if err != nil {
		logging.Error("Invalid key parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid key parameter: %v", err)), nil
	}

	if err := validate.Key(key); err != nil {
		logging.Error("Key validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid key: %v", err)), nil
	}

	logging.Info("Pressing key: %s", key)
	robotgo.KeyTap(key)
	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "key": "%s"}`, key)), nil
}

func handleTakeScreenshot(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling take_screenshot request")

	screenW, screenH := robotgo.GetScreenSize()
	x, _ := getIntParam(request, "x")
	y, _ := getIntParam(request, "y")
	width := screenW
	height := screenH
	if w, err := getIntParam(request, "width"); err == nil {
		width = w
	}
	if h, err := getIntParam(request, "height"); err == nil {
		height = h
	}

	if err := validate.ScreenRegion(x, y, width, height, screenW, screenH); err != nil {
		logging.Error("Screenshot region validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid screenshot region: %v", err)), nil
	}

	logging.Info("Taking screenshot at (%d, %d) with size %dx%d", x, y, width, height)

	img, captureErr := robotgo.CaptureImg(x, y, width, height)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	filename := fmt.Sprintf("ghost-mcp-screenshot-%d.png", time.Now().UnixNano())
	fpath := filepath.Join(os.TempDir(), filename)

	if saveErr := robotgo.SavePng(img, fpath); saveErr != nil {
		logging.Error("Failed to save screenshot: %v", saveErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save screenshot: %v", saveErr)), nil
	}
	logging.Info("Screenshot saved to: %s", fpath)

	data, readErr := os.ReadFile(fpath)
	if readErr != nil {
		logging.Error("Failed to read screenshot file: %v", readErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to read screenshot: %v", readErr)), nil
	}

	os.Remove(fpath)
	logging.Debug("Temporary screenshot file cleaned up")

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "filepath": "%s", "base64": "%s", "width": %d, "height": %d}`,
		fpath, base64.StdEncoding.EncodeToString(data), width, height,
	)), nil
}

// =============================================================================
// PARAMETER EXTRACTION
// =============================================================================

func getStringParam(request mcp.CallToolRequest, name string) (string, error) {
	val, ok := request.GetArguments()[name]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", name)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", name)
	}
	return str, nil
}

func getIntParam(request mcp.CallToolRequest, name string) (int, error) {
	val, ok := request.GetArguments()[name]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: %s", name)
	}
	if f, ok := val.(float64); ok {
		if f != math.Trunc(f) {
			return 0, fmt.Errorf("parameter %s must be a whole number, got %v", name, f)
		}
		return int(f), nil
	}
	if i, ok := val.(int); ok {
		return i, nil
	}
	if i, ok := val.(int64); ok {
		return int(i), nil
	}
	return 0, fmt.Errorf("parameter %s must be an integer", name)
}

// =============================================================================
// TOKEN AUTHENTICATION
// =============================================================================

func validateStartupToken() (string, error) {
	token := os.Getenv(TokenEnvVar)
	if token == "" {
		return "", fmt.Errorf("%s environment variable is not set", TokenEnvVar)
	}
	return token, nil
}

func makeTokenValidator(expectedToken string, al *audit.Logger) func(ctx context.Context, id any, message any) error {
	return func(ctx context.Context, id any, message any) error {
		if os.Getenv(TokenEnvVar) != expectedToken {
			logging.Error("Authentication failed: %s mismatch or missing", TokenEnvVar)
			al.Log(audit.EventAuthFailure, "", fmt.Sprintf("invalid or missing %s", TokenEnvVar), nil)
			return fmt.Errorf("%w: invalid or missing %s", audit.ErrAuthFailed, TokenEnvVar)
		}
		return nil
	}
}

// =============================================================================
// SERVER SETUP
// =============================================================================

func createServer(token string, al *audit.Logger) *server.MCPServer {
	logging.Info("Creating MCP server: %s v%s", ServerName, ServerVersion)

	hooks := &server.Hooks{}
	hooks.AddOnRequestInitialization(makeTokenValidator(token, al))
	audit.RegisterHooks(hooks, al)

	mcpServer := server.NewMCPServer(
		ServerName,
		ServerVersion,
		server.WithResourceCapabilities(true, true),
		server.WithHooks(hooks),
	)
	registerTools(mcpServer)
	return mcpServer
}

func registerTools(mcpServer *server.MCPServer) {
	logging.Info("Registering tools...")

	mcpServer.AddTool(mcp.NewTool("get_screen_size",
		mcp.WithDescription("Returns the width and height of the primary monitor"),
	), handleGetScreenSize)

	mcpServer.AddTool(mcp.NewTool("move_mouse",
		mcp.WithDescription("Moves the mouse cursor to the specified coordinates"),
		mcp.WithNumber("x", mcp.Description("Target X coordinate"), mcp.Required()),
		mcp.WithNumber("y", mcp.Description("Target Y coordinate"), mcp.Required()),
	), handleMoveMouse)

	mcpServer.AddTool(mcp.NewTool("click",
		mcp.WithDescription("Performs a mouse click at the current cursor position"),
		mcp.WithString("button", mcp.Description("Mouse button: 'left', 'right', or 'middle'"), mcp.Required()),
	), handleClick)

	mcpServer.AddTool(mcp.NewTool("type_text",
		mcp.WithDescription("Types out text via the keyboard"),
		mcp.WithString("text", mcp.Description("The text to type"), mcp.Required()),
	), handleTypeText)

	mcpServer.AddTool(mcp.NewTool("press_key",
		mcp.WithDescription("Presses a single key on the keyboard"),
		mcp.WithString("key", mcp.Description("Key to press (e.g., 'enter', 'tab', 'esc', 'ctrl')"), mcp.Required()),
	), handlePressKey)

	mcpServer.AddTool(mcp.NewTool("take_screenshot",
		mcp.WithDescription("Captures the screen and returns a base64-encoded PNG"),
		mcp.WithNumber("x", mcp.Description("X coordinate of screenshot region (default: 0)")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of screenshot region (default: 0)")),
		mcp.WithNumber("width", mcp.Description("Width of screenshot region (default: full screen)")),
		mcp.WithNumber("height", mcp.Description("Height of screenshot region (default: full screen)")),
	), handleTakeScreenshot)

	logging.Info("All tools registered successfully")
}

// =============================================================================
// MAIN
// =============================================================================

func main() {
	logging.Info("Starting %s v%s...", ServerName, ServerVersion)
	logging.Info("Platform: %s/%s", runtime.GOOS, runtime.GOARCH)

	token, err := validateStartupToken()
	if err != nil {
		logging.Error("Authentication not configured: %v", err)
		logging.Error("Ghost MCP requires a secret token to prevent unauthorized access.")
		logging.Error("Set %s to a random secret string in your environment:", TokenEnvVar)
		logging.Error("  Linux/macOS: export %s=$(openssl rand -hex 32)", TokenEnvVar)
		logging.Error("  Windows:     $env:%s = -join ((1..32)|%%{'{0:x}' -f (Get-Random -Max 256)})", TokenEnvVar)
		logging.Error("Then add it to your MCP client config under the 'env' key.")
		os.Exit(1)
	}
	logging.Info("Token authentication enabled (%s is set)", TokenEnvVar)

	auditLog, auditErr := audit.New()
	if auditErr != nil {
		logging.Error("Audit log unavailable: %v", auditErr)
	}
	defer auditLog.Close()
	auditLog.Log(audit.EventServerStart, "", "", map[string]interface{}{
		"version":  ServerVersion,
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
	})

	logging.Info("Failsafe position: (%d, %d) - Move mouse here to trigger emergency shutdown", FailsafeX, FailsafeY)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logging.Info("Received signal: %v", sig)
		initiateShutdown()
	}()

	cfg, err := transport.Load()
	if err != nil {
		logging.Error("Transport configuration error: %v", err)
		os.Exit(1)
	}
	logging.Info("Transport: %s", cfg.Mode)

	mcpServer := createServer(token, auditLog)

	switch cfg.Mode {
	case transport.Stdio:
		logging.Info("Starting stdio transport...")
		logging.Info("IMPORTANT: All application logs are written to stderr")
		logging.Info("stdout is reserved for MCP JSON-RPC protocol")
		if err = server.ServeStdio(mcpServer); err != nil {
			auditLog.Log(audit.EventServerStop, "", err.Error(), nil)
			logging.Error("Server error: %v", err)
			os.Exit(1)
		}

	case transport.HTTP:
		logging.Info("Starting HTTP/SSE transport...")
		if err = transport.ServeHTTP(state.shutdownChan, mcpServer, cfg, token, auditLog); err != nil {
			auditLog.Log(audit.EventServerStop, "", err.Error(), nil)
			logging.Error("HTTP server error: %v", err)
			os.Exit(1)
		}
	}

	auditLog.Log(audit.EventServerStop, "", "", nil)
	logging.Info("Server stopped gracefully")
}
