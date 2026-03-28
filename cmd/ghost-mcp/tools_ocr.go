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
}
