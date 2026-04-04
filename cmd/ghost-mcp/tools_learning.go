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
		mcp.WithDescription(`STEP 2 — Read the full element map after learn_screen.

Returns every text element found by learn_screen: label, coordinates, size, confidence, and the scroll page it was on. Call this immediately after learn_screen to understand the complete interface before deciding which elements to interact with. This is much faster than calling find_elements repeatedly.

Each element in the response has:
  • text        — the OCR-detected label (use this exact string in find_and_click)
  • x, y        — screen coordinates (top-left of the text bounding box)
  • width/height — size of the element
  • page_index  — 0 = top of screen, 1+ = required scroll position to see it
  • confidence  — OCR confidence 0–100 (prefer elements > 60)

USE THIS TO:
  • Decide which element to click — pick the exact label text shown here.
  • Discover elements below the fold (page_index > 0).
  • Verify the page loaded correctly before acting.

Returns {"learned":false} if learn_screen has not been called yet — call learn_screen first.`),
	), handleGetLearnedView)

	mcpServer.AddTool(mcp.NewTool("clear_learned_view",
		mcp.WithDescription(`Discard the current learned view so the next learn_screen builds a fresh one.

CALL THIS when the UI has changed substantially and the stored view is stale:
  • After clicking a link or button that navigates to a new page.
  • After a dialog or modal opens or closes.
  • After a single-page-app route change.
  • Any time find_and_click or find_elements returns surprising results.

TYPICAL PATTERN AFTER NAVIGATION:
  1. clear_learned_view   ← discard the old page's map
  2. learn_screen         ← scan the new page
  3. get_learned_view     ← read the new element list
  4. find_and_click ...   ← interact`),
	), handleClearLearnedView)

	mcpServer.AddTool(mcp.NewTool("set_learning_mode",
		mcp.WithDescription(`Enable or disable learning mode at runtime.

When ENABLED (recommended for all UI automation tasks):
  • The first call to find_and_click or find_elements automatically triggers
    a learn_screen scan if no view exists yet — no manual call required.
  • All subsequent OCR tool calls use the learned view for fast, region-targeted
    lookups instead of scanning the full screen every time (10–25× faster).

When DISABLED:
  • Every find_and_click / find_elements call scans the full screen from scratch.
  • Use this only if you want to skip learning and always do fresh full-screen OCR.

Learning mode can also be enabled permanently at startup by setting
GHOST_MCP_LEARNING=1 in the server environment (no tool call needed).

CALL set_learning_mode {enabled:true} at the start of any automation session
unless GHOST_MCP_LEARNING=1 is already set in the server config.`),
		mcp.WithBoolean("enabled", mcp.Description("true to enable learning mode, false to disable."), mcp.Required()),
	), handleSetLearningMode)

	logging.Info("Learning mode tools registered")
}
