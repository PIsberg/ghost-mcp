package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerOCRTools registers the read_screen_text tool.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
func registerOCRTools(mcpServer *server.MCPServer) {

	mcpServer.AddTool(mcp.NewTool("find_and_click",
		mcp.WithDescription(`THE ONLY TOOL YOU NEED TO CLICK A BUTTON. Start here for every click task. Do NOT call get_screen_size, take_screenshot, read_screen_text, or find_elements first — just call this.

🎯 WHEN TO USE:
- You need to click a single button/link/menu item by its text label
- Works for ALL button styles: colored backgrounds, dark themes, gradients
- Text can be partial: "save" matches "Save", "SAVE ALL", "Auto-save"

🚫 WHEN NOT TO USE:
- Multiple buttons in sequence → use find_and_click_all instead
- Need to verify UI change after click → use find_and_click + wait_for_text

SPEED TIP: Supply x/y/width/height to scan only the relevant area (e.g., a dialog or toolbar). Much faster than full screen.

HOW IT WORKS:
1. Captures screen (or region) once
2. Automatically runs OCR with 3 passes: grayscale → inverted (dark backgrounds) → color (colored buttons)
3. Finds text matching your search (case-insensitive substring)
4. Clicks the CENTER of the matched button (merges multi-word labels)
5. Returns exact coordinates

COLORED BUTTONS (white text on blue/green/red/cyan): handled automatically by the color pass. No special parameters needed — just call find_and_click with the button label.

IF TEXT NOT FOUND:
- DO NOT guess coordinates — guessing will miss
- Read the returned closest OCR matches first — they often reveal the exact visible label
- If the target may be off-screen, use scroll_until_text instead of manual scroll loops
- Call find_elements only if you need raw OCR diagnostics after those hints

RESPONSE: {success, found, box: {x,y,width,height}, requested_x/y, actual_x/y, button, occurrence}
- box is the OCR text bounding box (tight around characters, not the full button background)
- requested_x/y is the center of that box — where the click is aimed
- actual_x/y is where the mouse actually landed (verify this matches expected)
- For standard buttons with symmetric padding the text center == button center, so the click lands correctly`),
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

	mcpServer.AddTool(mcp.NewTool("find_elements",
		mcp.WithDescription(`THE PRIMARY TOOL FOR READING SCREEN TEXT. Scans the screen with OCR and returns all visible text grouped into elements (buttons, links, labels) with bounding boxes.

🎯 WHEN TO USE:
- You need to read text on the screen (e.g. verify a label, read a status message, extract values).
- find_and_click couldn't find your text — use this to see what OCR actually detected.
- You need center_x/center_y coordinates for click_at (when there's no text label available).
- Auditing what text is visible in a specific region of the screen.

⚠️ IMPORTANT LIMITATIONS:
- OCR detects TEXT ONLY — it cannot tell if text is on a button, link, or label.
- Same text may appear multiple times (e.g., "Submit" in header vs button).
- Icons/images without text are NOT detected.
- Colored buttons (white text on blue/green/red) may not appear in grayscale (disable grayscale to see them).

🚫 WHEN NOT TO USE:
- You want to click a button — use find_and_click directly, do NOT scan first.
- Need to see visual layout or non-text icons → use take_screenshot.

DIAGNOSTIC WORKFLOW (when find_and_click fails):
1. find_and_click fails → call find_elements to see what OCR detected.
2. Examine results to identify the true visible label text.
3. If text is a partial match, retry find_and_click with the exact string. If no text exists, use click_at with center_x/center_y from step 2.

EXAMPLE USAGE:
// Find all elements in button area (faster than full screen)
{"x": 0, "y": 600, "width": 800, "height": 200}

// Response includes ready-to-use coordinates:
{
  "success": true,
  "element_count": 5,
  "elements": [
    {"text": "Primary", "center_x": 174, "center_y": 770, "confidence": 95.2, "width": 80, "height": 35},
    {"text": "Success", "center_x": 425, "center_y": 770, "confidence": 94.8, "width": 80, "height": 35}
  ]
}

// Verify before clicking (optional but recommended for unknown UI):
{"tool": "take_screenshot", "arguments": {"x": 150, "y": 750, "width": 120, "height": 50}}

TIPS:
- Use region parameters to scan specific areas (10x faster than full screen)
- Elements filtered by confidence (min 50%) and size (min 20x10px)
- center_x/center_y are ready to use with click_at or move_mouse
- All coordinates use logical pixels
- width/height help identify element type (wide+short = likely button)`),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of region to scan (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of region to scan (default: full screen height).")),
	), handleFindElements)

	mcpServer.AddTool(mcp.NewTool("find_click_and_type",
		mcp.WithDescription(`Combines find_and_click and type_text into a single, highly efficient atomic operation.
This is the primary tool for filling out forms and interacting with text inputs.

🎯 HOW IT WORKS:
1. Performs OCR to find the specified label/text (e.g. "Username").
2. Calculates the click target, optionally applying x_offset and y_offset to click beside or below the label.
3. Clicks the calculated coordinate to focus the input field.
4. Waits for the field to focus (delay_ms).
5. Types the requested text.
6. Optionally presses Enter.

SPEED TIP: Use region constraints (x, y, width, height) to scan only the relevant form area.
MULTI-WORD LABELS: Works with OCR text split across adjacent words, e.g. "Type here or use".
OFF-SCREEN FIELDS: If the input may be below the fold, set scroll_direction to let the tool scroll and keep searching in one call.
ON FAILURE: the error now includes closest OCR matches plus the searched region so you can refine the text before falling back to diagnostics.

EXAMPLE: Clicking an input field to the right of a "Name:" label:
{
  "text": "Name:",
  "type_text": "John Doe",
  "x_offset": 100,
  "delay_ms": 100
}

EXAMPLE: Search downward for an off-screen placeholder, then type:
{
  "text": "Type here or use",
  "type_text": "Ghost MCP rocks!",
  "scroll_direction": "down",
  "max_scrolls": 8
}`),
		mcp.WithString("text", mcp.Description("The label or text to search for (e.g. \"Email:\"). Case-insensitive substring match."), mcp.Required()),
		mcp.WithString("type_text", mcp.Description("The exact text to type after clicking."), mcp.Required()),
		mcp.WithNumber("x_offset", mcp.Description("Horizontal pixel offset from the center of the found text to click. Use positive for right, negative for left (default: 0).")),
		mcp.WithNumber("y_offset", mcp.Description("Vertical pixel offset from the center of the found text to click. Use positive for below, negative for above (default: 0).")),
		mcp.WithBoolean("press_enter", mcp.Description("If true, presses Enter after typing (default: false).")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after clicking before typing (default: 100). Gives UI time to focus input.")),
		mcp.WithNumber("nth", mcp.Description("Which occurrence to click if label appears multiple times (default: 1).")),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of region to scan (default: full screen).")),
		mcp.WithNumber("height", mcp.Description("Height of region to scan (default: full screen).")),
		mcp.WithString("scroll_direction", mcp.Description("Optional scroll direction ('up', 'down', 'left', 'right') to keep searching if the field is off-screen.")),
		mcp.WithNumber("scroll_amount", mcp.Description("Scroll steps per retry when scroll_direction is set (default: 5).")),
		mcp.WithNumber("max_scrolls", mcp.Description("Maximum scroll attempts when scroll_direction is set (default: 8).")),
		mcp.WithNumber("scroll_x", mcp.Description("X coordinate to scroll at (default: screen center). Useful for scrolling a specific panel.")),
		mcp.WithNumber("scroll_y", mcp.Description("Y coordinate to scroll at (default: screen center). Useful for scrolling a specific panel.")),
		mcp.WithBoolean("grayscale", mcp.Description("Use grayscale OCR (default: true).")),
	), handleFindClickAndType)
}
