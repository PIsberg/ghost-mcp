package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerOCRTools registers the read_screen_text tool.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
func registerOCRTools(mcpServer *server.MCPServer) {
	mcpServer.AddTool(mcp.NewTool("read_screen_text",
		mcp.WithDescription(`Scan the screen with OCR and return all visible text with the pixel position of each word. Use this to locate UI elements by their text label so you can click them accurately.

Returns: {text: string, words: [{text, x, y, width, height, confidence}]}
- x, y is the TOP-LEFT corner of each word in screen pixels.
- width, height is the size of the word's bounding box.

TO CLICK A WORD: compute its center — move_x = x + width/2, move_y = y + height/2 — then call move_mouse with those values.

TIPS:
- Narrow the scan region (x, y, width, height) to a specific window or panel to get faster, more accurate results and avoid picking up unrelated text.
- Filter results by confidence (higher = more reliable). Low-confidence words may be misread.
- If a word is not found, try take_screenshot to see if it is actually visible, then re-scan the relevant region.
- Works best on crisp UI text. May struggle with stylised fonts, low-contrast text, or very small text — use take_screenshot for visual confirmation in those cases.`),
		mcp.WithNumber("x", mcp.Description("X coordinate of the top-left corner of the region to scan (default: 0).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of the top-left corner of the region to scan (default: 0).")),
		mcp.WithNumber("width", mcp.Description("Width of the region to scan in pixels (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of the region to scan in pixels (default: full screen height).")),
	), handleReadScreenText)

	mcpServer.AddTool(mcp.NewTool("find_and_click",
		mcp.WithDescription(`PREFERRED WAY TO CLICK UI ELEMENTS. Scans the screen with OCR, finds the word matching the given text, and clicks its center — all in one call.

Prefer this over the read_screen_text + click_at two-step pattern whenever you know the label of what you want to click (button, link, menu item, checkbox label, etc.).

Match is case-insensitive substring: "save" matches "Save", "SAVE ALL", "Auto-save", etc.

SPEED TIP: Supply x/y/width/height to scan only the relevant area of the screen (e.g. a toolbar, dialog, or panel). Scanning a small region is significantly faster than scanning the full screen.

If the element is not found: call take_screenshot to verify it is visible, then retry with a more specific or shorter text fragment.`),
		mcp.WithString("text", mcp.Description("Text to search for (case-insensitive substring match)."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("nth", mcp.Description("Which occurrence to click if the text appears multiple times (default: 1 = first match).")),
		mcp.WithNumber("x", mcp.Description("X coordinate of the top-left corner of the region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of the top-left corner of the region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of the region to scan in pixels (default: full screen width). Smaller regions scan faster.")),
		mcp.WithNumber("height", mcp.Description("Height of the region to scan in pixels (default: full screen height). Smaller regions scan faster.")),
	), handleFindAndClick)
}
