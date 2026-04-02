package main

import (
	"github.com/ghost-mcp/internal/logging"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerLearningTools registers the learning-mode MCP tools.
// Called from registerTools() in main.go.
func registerLearningTools(mcpServer *server.MCPServer) {
	logging.Info("Registering learning mode tools...")

	mcpServer.AddTool(mcp.NewTool("learn_screen",
		mcp.WithDescription(`Perform a full GUI reconnaissance scan and store an internal view of the current interface.

Learning mode takes screenshots and runs OCR across the visible viewport and, optionally, multiple scroll positions. The result is an internal map of every text element found — including elements below the current scroll position. Subsequent calls to find_and_click and find_elements use this learned view for fast region-targeted lookups instead of scanning the entire screen every time.

When to use:
- Call learn_screen once at the start of a workflow to map the current window or page.
- Re-call after navigating to a new page or after the UI changes significantly.
- Use get_learned_view to inspect what was found.
- Use clear_learned_view to discard the current view and start over.

The tool scrolls down during scanning then returns the viewport to its original position.`),
		mcp.WithNumber("x", mcp.Description("Left edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("y", mcp.Description("Top edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of the scan region in pixels (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of the scan region in pixels (default: full screen height).")),
		mcp.WithNumber("max_pages", mcp.Description("Maximum number of scroll pages to scan (default: 10). Each page scrolls scroll_amount ticks downward.")),
		mcp.WithNumber("scroll_amount", mcp.Description("Number of wheel-click ticks to scroll per page (default: 5).")),
		mcp.WithString("scroll_direction", mcp.Description("Direction to scroll while scanning: 'down' (default) or 'up'.")),
	), handleLearnScreen)

	mcpServer.AddTool(mcp.NewTool("get_learned_view",
		mcp.WithDescription(`Return the current learned view as a JSON object.

The response includes all text elements discovered by the most recent learn_screen call, with their screen coordinates and the page index (scroll position) on which each was found. Use this to understand the full structure of the current interface before deciding which elements to interact with.

Returns {"learned":false} if learn_screen has not been called yet.`),
	), handleGetLearnedView)

	mcpServer.AddTool(mcp.NewTool("clear_learned_view",
		mcp.WithDescription(`Discard the current learned view.

After clearing, the next call to learn_screen (or the automatic trigger when learning mode is enabled) will build a fresh view. Use this when the UI has changed substantially — for example after a page navigation or a dialog opening.`),
	), handleClearLearnedView)

	mcpServer.AddTool(mcp.NewTool("set_learning_mode",
		mcp.WithDescription(`Enable or disable learning mode at runtime.

When learning mode is enabled:
- The first call to find_and_click or find_elements automatically triggers a learn_screen scan if no view exists yet.
- Subsequent OCR tool calls use the learned view for fast, region-targeted lookups.

Learning mode can also be enabled at startup by setting GHOST_MCP_LEARNING=1 in the server environment.`),
		mcp.WithBoolean("enabled", mcp.Description("true to enable learning mode, false to disable."), mcp.Required()),
	), handleSetLearningMode)

	logging.Info("Learning mode tools registered")
}
