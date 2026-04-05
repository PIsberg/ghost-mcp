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
		mcp.WithDescription(`EXPLORATION TOOL — Scan the full interface, run multi-pass OCR, and cache the element map.
This is the RECOMMENDED logic for any new screen or complex workflow.

EXPLORATION HIERARCHY:
  1. learn_screen    ← PRIMARY for exploration and complex workflows.
  2. find_elements   ← SECONDARY for quick text-only dumps.
  3. take_screenshot ← TERTIARY (only for icon-only interfaces).

Use learn_screen when:
• You first arrive on a new screen or UI state.
• You have 3+ sequential actions on the same screen (caches the map for all steps).
• The page scrolls and you need elements below the visible area.
• After navigating to a new page, opening a dialog, or switching tabs.

── RECOMMENDED WORKFLOW ───────────────────────────────────────────────────────
  1. learn_screen          ← map the interface (scroll through if needed)
  2. get_annotated_view    ← VISUALLY confirm elements and their numeric IDs
  3. click_at(id=N)        ← interact with elements using 100% precise IDs
  4. clear_learned_view    ← after navigating away, then go back to step 1

The tool scrolls DOWN during scanning and automatically scrolls BACK UP when done.`),
		mcp.WithNumber("x", mcp.Description("Left edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("y", mcp.Description("Top edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of the scan region in pixels (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of the scan region in pixels (default: full screen height).")),
		mcp.WithNumber("max_pages", mcp.Description("Maximum number of scroll pages to scan (default: 10). Each page scrolls scroll_amount ticks downward.")),
		mcp.WithNumber("scroll_amount", mcp.Description("Number of wheel-click ticks to scroll per page (default: 5).")),
		mcp.WithString("scroll_direction", mcp.Description("Direction to scroll while scanning: 'down' (default) or 'up'.")),
	), handleLearnScreen)

	mcpServer.AddTool(mcp.NewTool("get_learned_view",
		mcp.WithDescription(`Return the full element map from the last learn_screen call.

Each element includes: ID (can be used with click_at), text, x/y coordinates,
width/height, page_index (0=top, 1+=requires scrolling), and confidence.

Call immediately after learn_screen to inspect the complete interface before acting.
Returns {"learned":false} if learn_screen has not been called yet.`),
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
