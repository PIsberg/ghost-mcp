package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerOCRTools registers the OCR-based tools.
// Requires Tesseract OCR libraries to be installed (e.g., via vcpkg).
func registerOCRTools(mcpServer *server.MCPServer) {

	mcpServer.AddTool(mcp.NewTool("find_and_click",
		mcp.WithDescription(`Find text on screen and click its center. Case-insensitive substring match.

For best performance, call learn_screen first — subsequent calls use the cached element
location (10–25× faster). For a single one-off click, find_and_click works without learn_screen.

WHEN TO USE:
- Click a single button, link, or menu item by its visible text label.
- Text can be partial: "save" matches "Save", "SAVE ALL", "Auto-save".

WHEN NOT TO USE:
- Multiple buttons in sequence → use find_and_click_all or execute_workflow.
- Need to verify UI changed → chain with wait_for_text after this call.
- Screen just changed (new page/dialog) → call clear_learned_view + learn_screen first.

SCROLL-AND-SEARCH: If text may be off-screen, set scroll_direction="down". The tool
scrolls up to max_scrolls times and searches after each scroll.

MULTI-PAGE SEARCH: Set next_page_keys="Page_Down" and max_pages=5 to search across pages.
Add select_best=true to scan all pages first and click the highest-scoring match.

ON FAILURE — the error response includes:
{
  "error": "text not found...",
  "candidates": [{"text": "Click", "score": 100, "x": 50, "y": 50}],
  "suggestion": "scroll_may_help" | "text_continues_off_screen" | "try_different_search_term" | "no_matches_found"
}
Act on the suggestion before retrying:
- "scroll_may_help" → add scroll_direction:"down"
- "try_different_search_term" → call find_elements to see actual OCR text
- "no_matches_found" → element may not be on screen; check with take_screenshot

SMART MATCHING SCORES (candidates array):
- 1000: exact match  500: prefix  400: suffix  300: standalone word  100: boundary  50: inside word
Prefer exact or prefix matches; if the best candidate score is 50, use a more specific term.

RESPONSE: {success, found, box, requested_x, requested_y, actual_x, actual_y, button, occurrence, candidates}`),
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
		mcp.WithDescription(`Find and click multiple text labels in one atomic operation (single OCR scan).

WHEN TO USE:
- Click several known, distinct targets all present on screen (e.g., check three checkboxes).
- Do NOT use to try alternative labels for the same target — use find_and_click with select_best=true instead.
- Do NOT use if you need to verify UI changes between each click — use individual find_and_click + wait_for_text.

texts must be a JSON-encoded array string: "[\"Button1\", \"Button2\"]"
List elements in the order you want them clicked. If any is not found, the operation stops.

Use delay_ms=200-500 if the UI needs time to update between clicks.

RESPONSE: {success, clicked_count, clicks: [{text, box, clicked_x, clicked_y, actual_x, actual_y, button}]}`),
		mcp.WithString("texts", mcp.Description("JSON array of text labels to find and click. Example: [\"Primary\", \"Success\", \"Warning\"] - MUST be a valid JSON array string."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after EACH click (default: 100). Use 200-500 for UI updates, 0 for speed. Max: 10000.")),
	), handleFindAndClickAll)

	mcpServer.AddTool(mcp.NewTool("wait_for_text",
		mcp.WithDescription(`Wait for text to appear or disappear. Use after any action that triggers a UI change.

WHEN TO USE:
- After "Save" → wait for "Saved successfully" to appear.
- After "Delete" → wait for item text to disappear (visible=false).
- After navigation → wait for the expected page title to appear.

WHEN NOT TO USE:
- Instant UI changes — just proceed.
- Visual changes without text labels — use take_screenshot instead.

visible=false waits for text to disappear. Default timeout=5000ms, max=30000ms.
Use region (x,y,width,height) to watch a specific area (faster than full screen).

RESPONSE: {success, text, visible, waited_ms} or {error: "timeout..."}`),
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

EXPLORATION HIERARCHY — use tools in this order when encountering an unknown screen:
  1. find_elements   ← PRIMARY: fast text dump, always try this first
  2. take_screenshot ← SECONDARY: only if find_elements misses icon-only elements
  3. learn_screen    ← TERTIARY: only if the page scrolls and you need below-fold content

⚡ LEARNING MODE: If learning mode is on and no view exists yet, this call
automatically triggers learn_screen first. When a multi-page view exists the
response also includes learned_off_page_elements — all elements found on
scroll pages below the current viewport — so you can see the FULL UI in one
call without manually scrolling.

🎯 WHEN TO USE:
- First tool to call on any unknown screen.
- find_and_click failed — call this to see what OCR actually detected.
- You need center_x/center_y coordinates for click_at.
- Auditing what text is visible in a specific screen region.

⚠️ LIMITATIONS:
- Detects TEXT ONLY — not icons or images without text labels.
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
- FILTER BY TYPE: Use type field to find specific elements:
  - type=button for buttons
  - type=input for text input fields
  - type=checkbox for checkboxes
  - type=radio for radio buttons
  - type=dropdown for dropdown menus
  - type=toggle for on/off switches
  - type=slider for sliders/range controls
  - type=label for field labels
- Element types: button, input, checkbox, radio, dropdown, toggle, slider, label, heading, link, value, text
- width/height help identify element type (wide+short = likely button)

EXAMPLE: Find form elements:
// Response includes type field for each element:
{
  "elements": [
    {"text": "Email:", "type": "label", "center_x": 100, "center_y": 100},
    {"text": "Enter your email", "type": "input", "center_x": 250, "center_y": 100},
    {"text": "☑ I agree", "type": "checkbox", "center_x": 150, "center_y": 150},
    {"text": "Volume 50%", "type": "slider", "center_x": 200, "center_y": 200},
    {"text": "Submit", "type": "button", "center_x": 200, "center_y": 250}
  ]
}
// Then interact:
// - Click checkbox: {"tool": "click_at", "arguments": {"x": 150, "y": 150}}
// - Type in input: {"tool": "find_click_and_type", "arguments": {"text": "Email:", "type_text": "user@example.com"}}
// - Adjust slider: {"tool": "click_at", "arguments": {"x": 200, "y": 200}} (then drag)
// - Click button: {"tool": "find_and_click", "arguments": {"text": "Submit"}}`),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of region to scan (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of region to scan (default: full screen height).")),
	), handleFindElements)

	mcpServer.AddTool(mcp.NewTool("find_click_and_type",
		mcp.WithDescription(`Find a label by text, click the associated input, and type — all in one atomic call.
Primary tool for filling form fields.

Use x_offset/y_offset to click beside or below the label (e.g., x_offset=100 clicks the input
field to the right of a "Name:" label). Use scroll_direction if the field may be below the fold.
On failure the error includes closest OCR matches so you can refine the search text.`),
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
		mcp.WithDescription(`Click at coordinates and retry until confirmation text appears (up to max_clicks times).

WHEN TO USE: You know the coordinates and need to verify the click worked (e.g., confirm a menu
opened, a save succeeded, or navigation occurred).

WHEN NOT TO USE: You don't know the coordinates — use find_and_click instead.

Polls every 500ms. Clicks again if text has not appeared and max_clicks not yet reached.

RESPONSE: {success, text, clicks, waited_ms, found}`),
		mcp.WithNumber("x", mcp.Description("X coordinate to click."), mcp.Required()),
		mcp.WithNumber("y", mcp.Description("Y coordinate to click."), mcp.Required()),
		mcp.WithString("wait_for_text", mcp.Description("Text to wait for after clicking."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("timeout_ms", mcp.Description("Maximum time to wait in milliseconds (default: 5000, max: 30000).")),
		mcp.WithNumber("max_clicks", mcp.Description("Maximum click attempts before giving up (default: 3).")),
	), handleClickUntilTextAppears)
}
