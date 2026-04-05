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
		mcp.WithDescription(`OCR ELEMENT INDEX — Returns a JSON list of all text elements found by OCR.

Each element has text, coordinates, type, and page_index. Use this to find
elements by their text content and click them by coordinates.

EXAMPLE OUTPUT:
  {"elements": [
    {"ocr_id": 1, "text": "Home",    "type": "link",   "page_index": 0, "x": 100, "y": 50},
    {"ocr_id": 2, "text": "Submit",  "type": "button", "page_index": 0, "x": 350, "y": 780},
    {"ocr_id": 3, "text": "INFO",    "type": "button", "page_index": 1, "x": 200, "y": 400}
  ]}

WORKFLOW:
1. Search the elements array for your target text.
2. If found: click_at(x=200, y=400) using the coordinates from the JSON.
3. If NOT found: call get_annotated_view and look at the image to find
   your target. Read the visual_id number from the overlay and use click_at(visual_id=N).

Note: ocr_id is just an internal sequence number. It is NOT a visual_id.
The visual_id comes ONLY from reading the annotated screenshot image.`),
	), handleGetLearnedView)

	mcpServer.AddTool(mcp.NewTool("get_annotated_view",
		mcp.WithDescription(`ANNOTATED SCREENSHOT — Returns a screenshot of your UI with visual_id overlays.

The image shows the actual UI PLUS small solid-colored rectangles placed on
every detected element. The white number inside each rectangle IS the visual_id.

WHAT THE OVERLAYS LOOK LIKE:
- A small solid-colored rectangle (blue, green, red, etc.) with a white number.
- Placed at the top-left corner of the element it labels.
- Example: a rectangle showing "12" on the "INFO" button means visual_id=12.

HOW TO USE THIS IMAGE:
1. Scan the image for the UI element you need (e.g. the "INFO" button).
2. Look at the overlay on or near that element. Read the number inside it.
3. Call click_at(visual_id=12) using that number.

IMPORTANT: visual_id ONLY comes from reading overlay numbers in THIS image.
It is NOT the same as ocr_id from get_learned_view. Never confuse them.

WHEN TO CALL:
- After get_learned_view did NOT find your target text (OCR missed it).
- When the UI has icon-only buttons with no text for OCR to find.
- Use page_index to view a specific scroll page from the last learn_screen scan.`),
		mcp.WithNumber("page_index", mcp.Description("The scroll-page index (0, 1, 2...) from the last learn_screen scan.")),
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
