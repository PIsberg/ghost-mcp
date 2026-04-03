package main

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPrompts registers the Ghost MCP routing guide as a discoverable
// MCP prompt. AI clients that call prompts/list will find it and can request
// it at the start of a session to understand which tools to use and when.
func registerPrompts(mcpServer *server.MCPServer) {
	mcpServer.AddPrompt(
		mcp.NewPrompt("ghost_mcp_guide",
			mcp.WithPromptDescription(
				"Tool routing guide for Ghost MCP — explains which tool to use for each task, "+
					"optimal tool sequences, and when to use learning mode. Read this at the start "+
					"of a session when you need to automate a UI.",
			),
		),
		handleGhostMCPGuide,
	)
}

func handleGhostMCPGuide(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Ghost MCP tool routing guide",
		Messages: []mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Text: ghostMCPGuide,
				},
			},
		},
	}, nil
}

const ghostMCPGuide = `# Ghost MCP — Tool Routing Guide

Ghost MCP gives you OS-level UI automation via mouse, keyboard, screen capture,
and OCR. This guide tells you which tool to reach for first, how to chain tools
together, and when to invest in Learning Mode for complex sessions.

---

## 1. Quick-Reference Decision Table

| What you need to do | Best tool |
|---|---|
| Click a button or link by its visible label | ` + "`find_and_click`" + ` |
| Click multiple distinct elements atomically | ` + "`find_and_click_all`" + ` |
| Click a field and type into it | ` + "`find_click_and_type`" + ` |
| Read every visible text element + coordinates | ` + "`find_elements`" + ` |
| Wait for a UI change before proceeding | ` + "`wait_for_text`" + ` |
| Click coordinates and wait for confirmation text | ` + "`click_until_text_appears`" + ` |
| Run multiple sequential steps on one screen | ` + "`execute_workflow`" + ` |
| Map a scrollable or complex interface first | ` + "`learn_screen`" + ` |
| Automate many steps after mapping the screen | ` + "`learn_screen`" + ` → cached ` + "`find_and_click`" + ` |
| Learn screen + click in one convenience call | ` + "`smart_click`" + ` |
| Inspect what learn_screen found / debug misses | ` + "`get_learned_view`" + ` |
| Type at the current cursor position | ` + "`type_text`" + ` |
| Press a keyboard shortcut | ` + "`press_key`" + ` |
| Scroll and search for text | ` + "`scroll_until_text`" + ` |
| Take a screenshot for visual inspection | ` + "`take_screenshot`" + ` |
| Get screen size / DPI | ` + "`get_screen_size`" + ` |

**Avoid:** using ` + "`take_screenshot`" + ` to read text — use ` + "`find_elements`" + ` instead (10× faster, machine-readable).

---

## 2. Optimal Tool Paths by Scenario

### Scenario A — Click a single button
` + "```" + `
find_and_click("Save")
` + "```" + `
If you are unsure the click succeeded:
` + "```" + `
find_and_click("Save")
wait_for_text("Saved successfully", visible=true, timeout_ms=5000)
` + "```" + `

### Scenario B — Fill a simple form
` + "```" + `
find_click_and_type("Username", "alice@example.com")
find_click_and_type("Password", "hunter2")
find_and_click("Sign in")
wait_for_text("Dashboard", visible=true, timeout_ms=8000)
` + "```" + `
Or in one call using execute_workflow (all steps share one learned screen map):
` + "```" + `
execute_workflow(steps=[
  {action:"find_click_and_type", text:"Username", type_text:"alice@example.com"},
  {action:"find_click_and_type", text:"Password", type_text:"hunter2"},
  {action:"find_and_click",      text:"Sign in"},
  {action:"wait_for_text",       text:"Dashboard", visible:true}
])
` + "```" + `

### Scenario C — Multiple instructions given at once (batch, same screen)
When given several things to do on the same screen, prefer ` + "`execute_workflow`" + `
over many individual calls. The screen is learned once and all steps reuse that
cached map — 3–6× faster than a full-screen OCR scan per step.
Only use this when all steps are on the same screen. If steps span multiple pages,
use individual calls and call ` + "`clear_learned_view`" + ` between pages.
` + "```" + `
execute_workflow(steps=[
  {action:"find_and_click",      text:"Accept"},
  {action:"find_click_and_type", text:"Name", type_text:"Bob"},
  {action:"find_and_click",      text:"Submit"}
])
` + "```" + `

### Scenario D — Click multiple distinct elements atomically
` + "`find_and_click_all`" + ` clicks every element in the list in a single call.
Use it when you need to click several known, distinct targets that are all
present on screen — for example, checking three checkboxes or clicking a
sequence of toolbar buttons. Do NOT use it to try alternative labels for one
target; use ` + "`find_and_click`" + ` with ` + "`select_best=true`" + ` for fuzzy matching instead.
` + "```" + `
find_and_click_all(["Enable logging", "Enable metrics", "Enable tracing"])
` + "```" + `
To dismiss a dialog where you know only one label will match:
` + "```" + `
find_and_click("OK")
` + "```" + `

### Scenario E — Scroll and find content
` + "```" + `
scroll_until_text("Privacy Policy", scroll_direction="down", max_scrolls=10)
` + "```" + `
Or use ` + "`find_and_click`" + ` with scroll parameters when you also want to click it:
` + "```" + `
find_and_click("Privacy Policy", scroll_direction="down", max_scrolls=10)
` + "```" + `

### Scenario F — Verify the UI changed before acting
Always use ` + "`wait_for_text`" + ` rather than a fixed delay:
` + "```" + `
find_and_click("Delete")
wait_for_text("Are you sure?", visible=true, timeout_ms=3000)
find_and_click("Confirm")
wait_for_text("Deleted", visible=true, timeout_ms=5000)
` + "```" + `

### Scenario G — Complex app with many elements / deep scrolling
Invest in learning once; all subsequent find operations use the cached map:
` + "```" + `
learn_screen(max_pages=5)       // scan full interface across scroll positions
get_learned_view()              // call this if find_and_click misses elements —
                                // inspect the element list to verify OCR found them
find_and_click("Settings")      // fast — narrows OCR to cached bounding box
find_click_and_type("API Key", "abc123")
find_and_click("Save")
clear_learned_view()            // discard after navigation to avoid stale positions
` + "```" + `

### Scenario H — Unknown interface (first time seeing it)
Start by reading the screen, then choose the right tool based on what's there:
` + "```" + `
find_elements()                 // returns all visible text with coordinates and types
` + "```" + `
Use the result to understand the layout:
- If you see the target element → use ` + "`find_and_click`" + ` or ` + "`find_click_and_type`" + ` directly
- If the interface is large or scrollable → call ` + "`learn_screen`" + ` before acting
- If elements are missing or text is hard to read → call ` + "`take_screenshot`" + ` to visually inspect
- If you need to act on many elements → switch to ` + "`execute_workflow`" + ` or ` + "`learn_screen`" + ` path

### Scenario I — One-off click on a new screen (convenience)
` + "`smart_click`" + ` combines ` + "`learn_screen`" + ` + ` + "`find_and_click`" + ` in a single call.
Use it when you need one click on an unfamiliar screen and don't want to manage
the learn/act/clear lifecycle manually:
` + "```" + `
smart_click("Submit")
` + "```" + `
For more than one action on the same screen, use ` + "`learn_screen`" + ` explicitly so
you control when ` + "`clear_learned_view`" + ` is called.

---

## 3. Learning Mode — When and Why

**Use learning mode when:**
- You have 3 or more sequential actions on the same screen
- The interface scrolls and elements may be off-screen
- A prompt gives you multiple instructions to carry out on one screen

**Skip learning mode when:**
- You only need to do one or two things
- The page will navigate away immediately after the action
- You have already called ` + "`learn_screen`" + ` and haven't navigated

**Lifecycle:**
` + "```" + `
learn_screen()          // once per "screen"
... many find_and_click, find_click_and_type calls (all cached) ...
clear_learned_view()    // after navigation or significant UI change
` + "```" + `

**Performance:** After ` + "`learn_screen`" + `, each ` + "`find_and_click`" + ` narrows its OCR scan
to a small bounding box — typically 10–25× faster than a full-screen scan.

**Debugging:** If ` + "`find_and_click`" + ` fails to find an element after ` + "`learn_screen`" + `,
call ` + "`get_learned_view`" + ` to see the full element list. If the target is missing,
OCR did not detect it — try ` + "`take_screenshot`" + ` to check visibility, or re-run
` + "`learn_screen`" + ` with a higher ` + "`max_pages`" + ` value.

---

## 4. Safety and Verification Rules

1. **Always verify destructive actions** (delete, submit, confirm) with
   ` + "`wait_for_text`" + ` before treating them as complete.
2. **Failsafe:** Moving the mouse to (0, 0) triggers an emergency shutdown.
   Never instruct the user to move the mouse there unless stopping is intentional.
3. **Loop protection:** ` + "`find_and_click`" + ` enforces a 25-call-per-session limit.
   If you are approaching it, switch to ` + "`execute_workflow`" + ` for remaining steps.
4. **After page navigation:** call ` + "`clear_learned_view`" + ` so stale element
   positions don't cause mis-clicks on the new page.
`
