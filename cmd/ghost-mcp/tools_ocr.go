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

⚠️ USE element_type TO AVOID FALSE MATCHES:
If the search text could match multiple element types (e.g. "Submit" appears as both a button AND a heading),
set element_type to target the correct one. Examples:
  - "click the Submit button" → {"text": "Submit", "element_type": "button"}
  - "click the Save button" → {"text": "Save", "element_type": "button"}
  - "click the checkbox" → {"text": "I agree", "element_type": "checkbox"}
  - "click the link" → {"text": "Privacy Policy", "element_type": "link"}
If the text is unique and unambiguous, you can omit element_type.

For best performance, call learn_screen first — subsequent calls use the cached element
location (10–25× faster). For a single one-off click, find_and_click works without learn_screen.

🎯 FOR HIGHEST PRECISION (MANDATORY): Always consider calling learn_screen then 
get_annotated_view FIRST. This gives you visual_id overlays (e.g. 5, 12). 
Use these with click_at(visual_id=N) for 100% precision.

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
		mcp.WithString("element_type", mcp.Description("Use when text could match multiple element types. 'button' for buttons, 'checkbox' for checkboxes, 'radio' for radio buttons, 'link' for hyperlinks, 'label' for field labels, 'input' for text fields, 'dropdown' for select menus, 'toggle' for switches, 'slider' for range controls, 'heading' for titles, 'value' for numbers, 'text' for body text. EXAMPLE: User says 'click the Save button' → set element_type='button'.")),
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
		mcp.WithString("element_type", mcp.Description("Filter to specific element type: 'button', 'input', 'checkbox', 'radio', 'dropdown', 'toggle', 'slider', 'label', 'heading', 'link', 'value', 'text'. Only waits for elements of this type.")),
	), handleWaitForText)

	mcpServer.AddTool(mcp.NewTool("find_elements",
		mcp.WithDescription(`QUICK TEXT CHECK — Read visible text on screen with coordinates.

── EXPLORATION HIERARCHY ──────────────────────────────────────────────────────
  1. learn_screen    ← PRIMARY: full-interface map + visual IDs (MANDATORY for interaction)
  2. find_elements   ← SECONDARY: fast text-only check (NOT for interaction)

⚠️ WARNING: This tool returns text but NO visual context. You cannot know which ID 
corresponds to which UI element without calling get_annotated_view first.

For any task involving 2+ steps, PREFER learn_screen followed by get_annotated_view.
This gives you visual_id overlays that you can use with click_at(visual_id=N) for 100% precision.

RESPONSE STRUCTURE:
- actionable_elements: focusable items like buttons, inputs, links.
- elements: complete list including headings and body text.
- labels: text ending with ":" (useful for find_click_and_type).`),
		mcp.WithNumber("x", mcp.Description("X coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of region to scan (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of region to scan (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of region to scan (default: full screen height).")),
		mcp.WithString("element_type", mcp.Description("Filter to specific types: 'button', 'input', 'checkbox', 'radio', 'dropdown', 'toggle', 'slider', 'label', 'link', 'heading', 'value', 'text'.")),
		mcp.WithNumber("scan_pages", mcp.Description("Number of scroll pages to scan (default: 1).")),
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
		mcp.WithString("element_type", mcp.Description("Filter search to specific element type: 'button', 'input', 'checkbox', 'radio', 'dropdown', 'toggle', 'slider', 'label', 'heading', 'link', 'value', 'text'. Only matches elements of this type.")),
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
