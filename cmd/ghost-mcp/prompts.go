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
| Click several buttons in one call | ` + "`find_and_click_all`" + ` |
| Click a field and type into it | ` + "`find_click_and_type`" + ` |
| Read every visible text element + coordinates | ` + "`find_elements`" + ` |
| Wait for a UI change before proceeding | ` + "`wait_for_text`" + ` |
| Click, wait for confirmation, retry if needed | ` + "`click_until_text_appears`" + ` |
| Run a multi-step form or wizard in one call | ` + "`execute_workflow`" + ` |
| Map a scrollable or complex interface first | ` + "`learn_screen`" + ` |
| Automate many steps after mapping the screen | ` + "`learn_screen`" + ` → cached ` + "`find_and_click`" + ` |
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
Or in one call:
` + "```" + `
execute_workflow(steps=[
  {action:"find_click_and_type", text:"Username", type_text:"alice@example.com"},
  {action:"find_click_and_type", text:"Password", type_text:"hunter2"},
  {action:"find_and_click",      text:"Sign in"},
  {action:"wait_for_text",       text:"Dashboard", visible:true}
])
` + "```" + `

### Scenario C — Multiple instructions given at once (batch)
When prompted with a list of things to do ("click Accept, then fill in the form,
then submit"), prefer a single ` + "`execute_workflow`" + ` call over many individual calls.
This is 3–6× faster because the screen is learned once and reused for all steps.
` + "```" + `
execute_workflow(steps=[
  {action:"find_and_click",      text:"Accept"},
  {action:"find_click_and_type", text:"Name", type_text:"Bob"},
  {action:"find_and_click",      text:"Submit"}
])
` + "```" + `

### Scenario D — Dismiss a dialog / confirm a prompt
` + "```" + `
find_and_click_all(["OK", "Accept", "Confirm"])   // tries each label
` + "```" + `
Or if only one label applies:
` + "```" + `
find_and_click("OK")
` + "```" + `

### Scenario E — Scroll and find content
` + "```" + `
scroll_until_text("Privacy Policy", scroll_direction="down", max_scrolls=10)
` + "```" + `
Or use ` + "`find_and_click`" + ` with scroll parameters:
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
learn_screen(max_pages=5)                         // scan full interface
get_learned_view()                                // inspect what was found (optional)
find_and_click("Settings")                        // fast — uses cache
find_click_and_type("API Key", "abc123")          // fast — uses cache
find_and_click("Save")
clear_learned_view()                              // discard after navigation
` + "```" + `

### Scenario H — Unknown interface (first time seeing it)
` + "```" + `
find_elements()                  // read all visible text with coordinates
                                 // then decide which tool fits the task
` + "```" + `

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

---

## 5. Tool Category Summary

### Core input tools (no OCR needed)
` + "`click_at`" + `, ` + "`type_text`" + `, ` + "`press_key`" + `, ` + "`scroll`" + `, ` + "`double_click`" + `
→ Use when you already know exact coordinates or want raw keyboard input.

### OCR-based finding tools (most common)
` + "`find_and_click`" + `, ` + "`find_elements`" + `, ` + "`find_click_and_type`" + `, ` + "`find_and_click_all`" + `
→ Use for every "click button by name" or "read what's on screen" task.

### Waiting and verification tools
` + "`wait_for_text`" + `, ` + "`click_until_text_appears`" + `
→ Use after every action that triggers a UI change.

### Learning mode tools
` + "`learn_screen`" + `, ` + "`get_learned_view`" + `, ` + "`clear_learned_view`" + `, ` + "`set_learning_mode`" + `
→ Use for complex or multi-step sessions to eliminate redundant full-screen OCR.

### Automation tools
` + "`execute_workflow`" + `
→ Use when given multiple sequential instructions; bundles learn+act+verify.

### Diagnostic tools
` + "`take_screenshot`" + `, ` + "`get_screen_size`" + `, ` + "`get_region_cache_stats`" + `
→ Use sparingly; prefer OCR tools for reading text.
`
