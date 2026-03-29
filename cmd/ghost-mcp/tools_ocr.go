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
		mcp.WithBoolean("grayscale", mcp.Description("Convert to greyscale and apply contrast stretching before OCR (default: true). Set to false when colour context matters — e.g. 'find the red button' — so that colour differences are visible to the OCR engine.")),
	), handleReadScreenText)

	mcpServer.AddTool(mcp.NewTool("find_and_click",
		mcp.WithDescription(`PREFERRED WAY TO CLICK UI ELEMENTS. Scans the screen with OCR, finds the word matching the given text, and clicks its center — all in one call.

🎯 WHEN TO USE:
- You need to click a single button/link/menu item by its text label
- Prefer this over read_screen_text + click_at (simpler, more reliable)
- Text can be partial: "save" matches "Save", "SAVE ALL", "Auto-save"

🚫 WHEN NOT TO USE:
- Multiple buttons in sequence → use find_and_click_all instead
- Need to verify UI change after click → use find_and_click + wait_for_text

SPEED TIP: Supply x/y/width/height to scan only the relevant area (e.g., a dialog or toolbar). Much faster than full screen.

HOW IT WORKS:
1. Captures screen (or region) once
2. Runs OCR with 3 passes: normal → inverted (for dark backgrounds) → color
3. Finds text matching your search (case-insensitive substring)
4. Clicks the CENTER of the matched button (merges multi-word labels)
5. Returns exact coordinates for verification

IMPORTANT:
- If text not found, DO NOT guess coordinates
- Instead: call read_screen_text to see what OCR actually detected
- Or: use take_screenshot to verify the text is visible
- For colored buttons (white text on green/red/blue), may need multiple passes

RESPONSE: {success, found, box: {x,y,width,height}, requested_x/y, actual_x/y, button, occurrence}
- box shows the full button bounds (merged for multi-word labels)
- actual_x/y is where the mouse actually landed (verify this matches expected)`),
		mcp.WithString("text", mcp.Description("Text to search for (case-insensitive substring match). Example: \"save\" matches \"Save\", \"SAVE ALL\"."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("nth", mcp.Description("Which occurrence to click if text appears multiple times (default: 1 = first).")),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of region to scan (default: full screen). Smaller = faster.")),
		mcp.WithNumber("height", mcp.Description("Height of region to scan (default: full screen). Smaller = faster.")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after click (default: 100). Set to 0 to skip. Max: 10000.")),
		mcp.WithBoolean("grayscale", mcp.Description("Use grayscale OCR (default: true). Set false for color-dependent text.")),
	), handleFindAndClick)

	mcpServer.AddTool(mcp.NewTool("find_and_click_all",
		mcp.WithDescription(`ATOMIC MULTI-CLICK: Finds and clicks multiple text labels in one operation. Use this to click multiple buttons without verification loops between each.

🎯 WHEN TO USE:
- You need to click multiple buttons in sequence (e.g., "Primary", "Success", "Warning")
- You want to avoid OCR verification failures between clicks
- Faster than multiple find_and_click calls (single OCR scan)

🚫 WHEN NOT TO USE:
- If you need to verify UI changes between each click → use find_and_click + wait_for_text instead
- If buttons might appear/disappear dynamically → use individual find_and_click calls

EXAMPLE USAGE:
{
  "tool": "find_and_click_all",
  "arguments": {
    "texts": ["Primary", "Success", "Warning"],
    "delay_ms": 200
  }
}

Returns: {success: true, clicked_count: N, clicks: [{text, box, clicked_x, clicked_y, actual_x, actual_y, button}, ...]}

TIPS:
- texts must be a JSON array of strings: ["Button1", "Button2"]
- List buttons in the order you want them clicked (left to right, top to bottom)
- Use delay_ms=200-500 between clicks if UI needs time to update
- If any text is not found, the operation stops immediately and returns an error
- All coordinates use logical pixels (same as get_screen_size)`),
		mcp.WithString("texts", mcp.Description("JSON array of text labels to find and click. Example: [\"Primary\", \"Success\", \"Warning\"] - MUST be a valid JSON array string."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after EACH click (default: 100). Use 200-500 for UI updates, 0 for speed. Max: 10000.")),
	), handleFindAndClickAll)

	mcpServer.AddTool(mcp.NewTool("wait_for_text",
		mcp.WithDescription(`Wait for text to appear or disappear from the screen. Use this to verify UI state changes after clicking a button.

🎯 WHEN TO USE:
- After clicking "Save" → wait for "Saved successfully" to appear
- After clicking "Delete" → wait for item text to disappear
- After navigation → wait for expected page title to appear
- Before clicking → wait for a button to become visible (e.g., after page load)

🚫 WHEN NOT TO USE:
- For instant UI changes → just proceed to next action
- For visual changes without text → use take_screenshot instead

COMMON PATTERNS:
  // Wait for success message
  find_and_click {text: "Submit"}
  wait_for_text {text: "Success", timeout_ms: 5000}
  
  // Wait for item to disappear
  find_and_click {text: "Delete"}
  wait_for_text {text: "Item Name", visible: false, timeout_ms: 5000}

Returns: {success: true, text: "...", visible: true/false, waited_ms: N}
On timeout: {error: "timeout waiting for text..."}

TIPS:
- visible=false waits for text to disappear (default=true waits for appear)
- Use region (x,y,width,height) to watch specific area (faster than full screen)
- Default timeout=5000ms, max=30000ms
- Checks every 500ms for efficiency
- All coordinates use logical pixels`),
		mcp.WithString("text", mcp.Description("Text to wait for (case-insensitive substring match)."), mcp.Required()),
		mcp.WithBoolean("visible", mcp.Description("true (default) = wait for text to appear. false = wait for text to disappear.")),
		mcp.WithNumber("timeout_ms", mcp.Description("Maximum time to wait in milliseconds (default: 5000, max: 30000).")),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of region to scan (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of region to scan (default: full screen height).")),
	), handleWaitForText)
}
