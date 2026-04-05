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
		mcp.WithDescription(`PRIMARY EXPLORATION TOOL — Perform a deep scan of the current screen or web page.
This tool builds an internal map of all text, buttons, and input fields.

🎯 MANDATORY FOLLOW-UP: Immediately call get_annotated_view after this tool.
You cannot interact with IDs until you have seen them on the annotated image.

── WHY CALL THIS? ─────────────────────────────────────────────────────────────
- It is 100% precise. No coordinate guessing or OCR drift.
- It is 10–25× faster for subsequent actions as results are cached.
- It detects elements across multiple scroll pages if max_pages > 1.`),
		mcp.WithNumber("x", mcp.Description("Left edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("y", mcp.Description("Top edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of the scan region in pixels (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of the scan region in pixels (default: full screen height).")),
		mcp.WithNumber("max_pages", mcp.Description("Maximum number of scroll pages to scan (default: 10). Each page scrolls scroll_amount ticks downward.")),
		mcp.WithNumber("scroll_amount", mcp.Description("Number of wheel-click ticks to scroll per page (default: 5).")),
		mcp.WithString("scroll_direction", mcp.Description("Direction to scroll while scanning: 'down' (default) or 'up'.")),
	), handleLearnScreen)

	mcpServer.AddTool(mcp.NewTool("get_learned_view",
		mcp.WithDescription(`MACHINE-READABLE MAP — Retrieve the full JSON element list from the last scan.
Includes all discovered text, classified element types, coordinates, and numeric IDs.

🎯 ESSENTIAL STEP: Call this immediately after learn_screen to populate your context
with the searchable text and IDs of every UI element.

── WHY CALL THIS? ─────────────────────────────────────────────────────────────
- It allows you to search for specific button/label text and find its unique ID.
- It provides the 'id' parameter required for high-precision click_at(id=N).
- It gives you the full text-based inventory of the screen (machine-readable).

After calling this, use get_annotated_view to visually confirm the IDs.`),
	), handleGetLearnedView)

	mcpServer.AddTool(mcp.NewTool("get_annotated_view",
		mcp.WithDescription(`VISUAL ANCHOR TOOL — Capture a screenshot (live or historical) and overlay numeric IDs.
This is the HIGHEST PRECISION interaction flow. It returns an image with ID markers
(e.g. [5], [12]) overlaid on every discovered element.

── USAGE MODES ─────────────────────────────────────────────────────────────
1. LIVE VIEWPORT (Default): Omitting page_index captures a fresh screenshot of
   the current screen and overlays IDs from the last scan.
2. PAGE HISTORY: Passing page_index (e.g. 1, 2) returns the STORED screenshot
   captured during the last learn_screen session, annotated at that time.
   Use this to see "off-screen" content without scrolling back.

── WORKFLOW ────────────────────────────────────────────────────────────────
1. Call learn_screen first to map the screen (optionally with max_pages > 1).
2. Call get_annotated_view to see the visual "ID badges".
3. Use the ID badge numbers in the image to call click_at(id=N).`),
		mcp.WithNumber("page_index", mcp.Description("Optional: The scroll-page index (0, 1, 2...) from the last learn_screen session. If omitted, captures the live viewport.")),
		mcp.WithNumber("x", mcp.Description("X coordinate of region (live mode only, default: 0).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region (live mode only, default: 0).")),
		mcp.WithNumber("width", mcp.Description("Width of region (live mode only, default: full screen).")),
		mcp.WithNumber("height", mcp.Description("Height of region (live mode only, default: full screen).")),
	), handleGetAnnotatedView)

	mcpServer.AddTool(mcp.NewTool("clear_learned_view",
		mcp.WithDescription(`Discard the current learned view so the next learn_screen builds a fresh one.

Call after any navigation, dialog open/close, or significant UI change to prevent
stale element positions from causing mis-clicks. Then call learn_screen to rebuild.`),
	), handleClearLearnedView)

	mcpServer.AddTool(mcp.NewTool("set_learning_mode",
		mcp.WithDescription(`Enable or disable learning mode at runtime.

Learning mode is ON by default. When enabled, the first OCR call auto-runs learn_screen,
and all subsequent calls use the cached view (10–25× faster than full-screen scans).

Set enabled=false only if you need raw full-screen OCR on every call without caching.
To disable at startup instead, set GHOST_MCP_LEARNING=0 in the server environment.`),
		mcp.WithBoolean("enabled", mcp.Description("true to enable learning mode, false to disable."), mcp.Required()),
	), handleSetLearningMode)

	logging.Info("Learning mode tools registered")
}
