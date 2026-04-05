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
		mcp.WithDescription(`Scan the full interface, run multi-pass OCR, and cache the element map.

EXPLORATION HIERARCHY — for unknown screens, prefer find_elements first:
  1. find_elements   ← PRIMARY exploration (fast, visible elements only)
  2. take_screenshot ← SECONDARY (only for icon-only interfaces)
  3. learn_screen    ← TERTIARY for exploration, but PRIMARY for complex workflows

Use learn_screen when:
• You have 3+ sequential actions on the same screen (caches the map for all steps).
• The page scrolls and you need elements below the visible area.
• After navigating to a new page, opening a dialog, or switching tabs.
• find_and_click or find_elements returns unexpected results (rebuild stale view).

Skip learn_screen when:
• You only need one or two actions — find_and_click directly is faster.
• The page navigates away immediately after the action.

── RECOMMENDED WORKFLOW ───────────────────────────────────────────────────────
  1. learn_screen          ← map the full interface (scroll through if needed)
  2. get_learned_view      ← read the element map; decide what to interact with
  3. find_and_click / find_click_and_type / find_elements  ← act on elements
  4. clear_learned_view    ← after navigating away, then go back to step 1

── CHOOSING max_pages ─────────────────────────────────────────────────────────
  • Short page / desktop app: max_pages=1 (default 10 is fine too)
  • Long webpage / settings panel: max_pages=5 to max_pages=10
  • When in doubt: leave at default; the tool stops early when content repeats.

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
		mcp.WithDescription(`Capture a screenshot of the current viewport and overlay visual IDs from the last scan.

This is the HIGH-PRECISION visual exploration tool. It returns an image with bounding
boxes and ID markers (e.g. [5], [12]) overlaid on every discovered element.

WORKFLOW:
1. Call learn_screen or find_elements first to discover elements.
2. Call get_annotated_view to see the visual "Set-of-Marks".
3. Use the ID badge numbers you see in the image to call click_at(id=N).

Parameters (x, y, width, height) are optional and default to full screen.`),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to capture (default: 0).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to capture (default: 0).")),
		mcp.WithNumber("width", mcp.Description("Width of region to capture (default: full screen).")),
		mcp.WithNumber("height", mcp.Description("Height of region to capture (default: full screen).")),
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
