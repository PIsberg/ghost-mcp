package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerOCRTools registers the OCR-based tools.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
func registerOCRTools(mcpServer *server.MCPServer) {

	mcpServer.AddTool(mcp.NewTool("find_and_click",
		mcp.WithDescription(`⚠️ CRITICAL: ALWAYS CALL learn_screen FIRST BEFORE THIS TOOL!

This tool works BEST with learning mode enabled. Without learn_screen, this tool
is SLOWER and LESS ACCURATE. The workflow is:
  1. learn_screen() ← REQUIRED: captures full UI with 4-pass OCR
  2. get_learned_view() ← See what elements exist
  3. find_and_click() ← Uses cached view (10-25x faster, more accurate)

If you haven't called learn_screen in this session, DO IT NOW before clicking.

🎯 WHEN TO USE:
- You need to click a single button/link/menu item by its text label
- Works for ALL button styles: colored backgrounds, dark themes, gradients
- Text can be partial: "save" matches "Save", "SAVE ALL", "Auto-save"

🚫 WHEN NOT TO USE:
- Multiple buttons in sequence → use find_and_click_all instead
- Need to verify UI change after click → use find_and_click + wait_for_text
- Screen just changed (new page/dialog) → call learn_screen FIRST, then this

⚡ AUTOMATIC OPTIMIZATION: After learn_screen, this tool uses cached regions.
Subsequent calls scan only the cached region (10-25x faster).

🎯 SMART MATCHING: Uses intelligent scoring to prefer standalone buttons over
text inside other elements (e.g., prefers "Click" button over "Button Click Tests" header).

SPEED TIP: Supply x/y/width/height to scan only the relevant area (e.g., a dialog or toolbar).

HOW IT WORKS:
1. If learning mode ON + view exists: Uses cached element location (instant)
2. Otherwise: Captures screen once, runs 4-pass OCR (normal→inverted→bright→color)
3. Finds text matching your search (case-insensitive substring)
4. Clicks the CENTER of the matched button
5. Returns exact coordinates

📜 SCROLL-AND-SEARCH: If text may be off-screen, add scroll_direction="down" to
automatically scroll and search. Scrolls up to max_scrolls times.

📄 MULTI-PAGE SEARCH: Add next_page_keys="Page_Down" and max_pages=5 to search
multiple pages. Tool navigates and searches each page until found.

🎯 SELECT BEST MODE: Add select_best=true to scan ALL pages first, compare scores,
then click the highest-score match. Slower but more accurate.

IF TEXT NOT FOUND:
The error response includes helpful guidance:

{
  "error": "text not found...",
  "candidates": [
    {"text": "Click", "score": 100, "x": 50, "y": 50}
  ],
  "suggestion": "scroll_may_help" | "text_continues_off_screen" | "try_different_search_term" | "no_matches_found"
}

Based on suggestion:
- "scroll_may_help" → Add scroll_direction:"down" to search off-screen
- "text_continues_off_screen" → Text is partially visible, scroll to see rest
- "try_different_search_term" → Use find_elements to see what text exists
- "no_matches_found" → Text not on current screen, may need multi-page search

Use candidates array to see what OCR detected and their confidence scores.

RESPONSE: {success, found, box: {x,y,width,height}, requested_x/y, actual_x/y, button, occurrence, candidates}
- box is the OCR text bounding box (tight around characters, not the full button background)
- requested_x/y is the center of that box — where the click is aimed
- actual_x/y is where the mouse actually landed (verify this matches expected)
- candidates is an array of all potential matches with scores (see SMART MATCHING below)
- For standard buttons with symmetric padding the text center == button center, so the click lands correctly

SMART MATCHING SCORES (in candidates array):
- score:1000 = Exact match ("Click Me!" = "Click Me!") ← BEST
- score:500 = Prefix match ("Click Me!" starts with "Click")
- score:400 = Suffix match ("Button Click" ends with "Click")
- score:300 = Standalone word ("Click" as separate word) ← GOOD
- score:100 = Substring with boundaries
- score:50 = Inside another word (avoid) ← AVOID

Use candidates to verify the AI chose the right element. If the clicked element has low score (50-100), consider using a more specific search term.`),
		mcp.WithString("text", mcp.Description("Text to search for (case-insensitive substring match). Example: \"save\" matches \"Save\", \"SAVE ALL\"."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("nth", mcp.Description("Which occurrence to click if text appears multiple times (default: 1 = first).")),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of region to scan (default: full screen). Smaller = faster.")),
		mcp.WithNumber("height", mcp.Description("Height of region to scan (default: full screen). Smaller = faster.")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after click (default: 100). Set to 0 to skip. Max: 10000.")),
		mcp.WithBoolean("grayscale", mcp.Description("Use grayscale OCR (default: true). Set false for color-dependent text.")),
		mcp.WithString("scroll_direction", mcp.Description("Optional: scroll direction to search off-screen ('up' or 'down'). If set, tool will scroll and search until text is found.")),
		mcp.WithNumber("max_scrolls", mcp.Description("Maximum scroll attempts when scroll_direction is set (default: 8).")),
		mcp.WithNumber("scroll_amount", mcp.Description("Scroll amount per step when scroll_direction is set (default: 5).")),
		mcp.WithString("next_page_keys", mcp.Description("Optional: keyboard keys to navigate to next page (e.g., 'Page_Down', 'Arrow_Down', 'Tab'). Use comma-separated for multiple keys. Enables multi-page search.")),
		mcp.WithNumber("max_pages", mcp.Description("Maximum pages to search when next_page_keys is set (default: 1, set >1 for multi-page search).")),
		mcp.WithBoolean("select_best", mcp.Description("If true, scans ALL pages first and clicks the highest-score match. If false (default), clicks first match found. Recommended for multi-page searches.")),
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
		mcp.WithDescription(`READ ALL VISIBLE TEXT ON SCREEN with coordinates and bounding boxes.

⚡ LEARNING MODE: If learning mode is on and no view exists yet, this call
automatically triggers learn_screen first. When a multi-page view exists the
response also includes learned_off_page_elements — all elements found on
scroll pages below the current viewport — so you can see the FULL UI in one
call without manually scrolling.

PREFERRED WORKFLOW FOR NEW SCREENS:
  1. learn_screen           ← scan entire interface (once per page)
  2. get_learned_view       ← full element list including below-fold elements
  3. find_elements          ← confirm visible elements before acting (optional)
  4. find_and_click / find_click_and_type

🎯 WHEN TO USE WITHOUT LEARNING MODE:
- find_and_click failed — call this to see what OCR actually detected.
- You need center_x/center_y coordinates for click_at.
- Auditing what text is visible in a specific screen region.

⚠️ LIMITATIONS:
- Detects TEXT ONLY — not icons or images.
- Only shows what is CURRENTLY VISIBLE (use learn_screen to see below-fold).
- Colored buttons (white on dark) may require grayscale=false.

🚫 DO NOT USE to look up a button before clicking — call find_and_click directly.

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
- Elements filtered by confidence (min 40%) and size (min 15x8px)
- center_x/center_y are ready to use with click_at or move_mouse
- All coordinates use logical pixels
- FILTER BY TYPE: Use type field to find buttons: look for type=button
- Element types: button, label, heading, link, value, text
- width/height help identify element type (wide+short = likely button)

EXAMPLE: Find only buttons:
// Response includes type field for each element:
{
  "elements": [
    {"text": "Primary", "type": "button", "center_x": 174, "center_y": 770},
    {"text": "Email:", "type": "label", "center_x": 100, "center_y": 100}
  ]
}
// Then click the button: {"tool": "click_at", "arguments": {"x": 174, "y": 770}}`),
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

	mcpServer.AddTool(mcp.NewTool("get_region_cache_stats",
		mcp.WithDescription(`Returns statistics about the region cache used to optimize find_and_click and other OCR tools.

The region cache automatically remembers the screen positions of buttons and UI elements after they are found. Subsequent calls use cached regions for 10-25x faster performance.

Returns: {entries: N, hits: X, misses: Y, hit_rate: Z%, evictions: E}

Use this to monitor cache effectiveness. A high hit rate (>80%) indicates good cache utilization.`),
	), handleGetRegionCacheStats)

	mcpServer.AddTool(mcp.NewTool("clear_region_cache",
		mcp.WithDescription(`Clears all cached UI element regions.

Use this when:
- Screen resolution has changed
- UI layout has been significantly rearranged
- Cache is returning stale results
- You want to force a full-screen OCR rescan

After clearing, the next find_and_click call will do a full-screen scan and rebuild the cache.`),
	), handleClearRegionCache)

	mcpServer.AddTool(mcp.NewTool("click_until_text_appears",
		mcp.WithDescription(`CLICK WITH VERIFICATION: Clicks coordinates and waits for text to appear. Use this to verify your click had the expected effect.

🎯 WHEN TO USE:
- After clicking a button, wait for confirmation text (e.g., "Saved!", "Success")
- Verify a menu opened by waiting for menu text
- Confirm navigation by waiting for page title
- Retry clicking if text doesn't appear (up to max_clicks times)

🚫 WHEN NOT TO USE:
- If you don't know the coordinates → use find_and_click instead
- If there's no text verification → just use click_at

HOW IT WORKS:
1. Clicks at specified coordinates
2. Polls screen every 500ms for wait_for_text
3. If not found and max_clicks not reached, clicks again
4. Stops when text appears OR timeout/max_clicks reached

EXAMPLE: Click "Save" button at (400,300), wait for "Saved!" confirmation:
{
  "tool": "click_until_text_appears",
  "arguments": {
    "x": 400,
    "y": 300,
    "wait_for_text": "Saved!",
    "timeout_ms": 5000,
    "max_clicks": 3
  }
}

Returns: {success: true/false, text: "...", clicks: N, waited_ms: N, found: true/false}
On failure: Click may have missed, or expected text is different than actual.`),
		mcp.WithNumber("x", mcp.Description("X coordinate to click."), mcp.Required()),
		mcp.WithNumber("y", mcp.Description("Y coordinate to click."), mcp.Required()),
		mcp.WithString("wait_for_text", mcp.Description("Text to wait for after clicking."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("timeout_ms", mcp.Description("Maximum time to wait in milliseconds (default: 5000, max: 30000).")),
		mcp.WithNumber("max_clicks", mcp.Description("Maximum click attempts before giving up (default: 3).")),
	), handleClickUntilTextAppears)
}
