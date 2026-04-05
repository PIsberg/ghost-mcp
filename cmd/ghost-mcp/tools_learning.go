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
		mcp.WithDescription(`INDEX FULL INTERFACE — Scan the UI across multiple scroll positions.

🚨 LONG PAGES: You MUST set max_pages > 1 for scrollable UIs. 
Calling learn_screen with max_pages: 1 (default) on a long page is a failure.

🚫 NO-PEEK RULE: Do NOT use "Scroll -> take_screenshot" repeatedly. 
Instead, call learn_screen(max_pages: 10) once to index the whole UI.
The server will then automatically reach any ID for you.`),
		mcp.WithNumber("x", mcp.Description("Left edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("y", mcp.Description("Top edge of the scan region in pixels (default: 0 / full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of the scan region in pixels (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of the scan region in pixels (default: full screen height).")),
		mcp.WithNumber("max_pages", mcp.Description("Optional: Max scroll pages to index (default: 1). Set >1 for long forms.")),
		mcp.WithNumber("scroll_amount", mcp.Description("Optional: Wheel ticks per page (default: 5).")),
		mcp.WithString("scroll_direction", mcp.Description("Optional: 'down' (default) or 'up'.")),
	), handleLearnScreen)

	mcpServer.AddTool(mcp.NewTool("get_learned_view",
		mcp.WithDescription(`MACHINE-READABLE MAP — Retrieve the full JSON element list from the last scan.
Includes all discovered text, classified element types, coordinates, and numeric IDs.

🎯 ESSENTIAL STEP: Call this immediately after learn_screen to populate your context
with the searchable text and IDs of every UI element.

── WHY CALL THIS? ─────────────────────────────────────────────────────────────
- 🚀 UNIVERSAL DISCOVERY: It returns IDs for ALL elements found across 
  all scroll pages. You now know the ID of every button, even those off-screen.
- It provides the 'id' parameter required for precision interaction.
- After calling this, use get_annotated_view to visually confirm the IDs.

🚫 NO-PEEK RULE: Do NOT use "Scroll -> take_screenshot" when learning mode is active.
Always use learn_screen to index, then get_learned_view to retrieve data.`),
	), handleGetLearnedView)

	mcpServer.AddTool(mcp.NewTool("get_annotated_view",
		mcp.WithDescription(`VISUAL ID MAP — Returns a screenshot with numeric ID badges for all elements.
This tool is the ONLY way to see the [5], [12] badges required for precision clicking.

🎯 MANDATORY STEP: Call this after learn_screen + get_learned_view to visually 
verify the interface and identify the correct IDs for interaction.

── WHY CALL THIS? ─────────────────────────────────────────────────────────────
- It provides numeric badges (e.g. [5]) overlaid on every button/input.
- It is the ONLY source for the 'id' parameter used in click_at(id=N).
- ⚡ PAGE HISTORY: Use 'page_index' (0, 1, 2...) to see elements from the last 
  learn_screen session. This allows you to inspect off-screen content WITHOUT 
  manually scrolling there.

🚫 NO-PEEK RULE: Do NOT "Scroll -> get_annotated_view" repeatedly. 
Instead, call learn_screen(max_pages: 3) once, then use page_index to inspect.`),
		mcp.WithNumber("page_index", mcp.Description("Optional: The scroll-page index (0, 1, 2...) from the last scan.")),
		mcp.WithNumber("x", mcp.Description("X coordinate (live mode only, default: 0).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate (live mode only, default: 0).")),
		mcp.WithNumber("width", mcp.Description("Width (live mode only, default: full screen).")),
		mcp.WithNumber("height", mcp.Description("Height (live mode only, default: full screen).")),
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
