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

## 0. CRITICAL RULES — Read Before Acting

These rules override everything else. Violating them can cause data loss or runaway loops.

1. **Failsafe — NEVER move the mouse to (0, 0).** Moving to top-left triggers an emergency
   shutdown. Do not do this unless the goal is to stop the server.

2. **Loop protection — 25-call limit.** ` + "`find_and_click`" + ` enforces a 25-call-per-session
   limit. If you are approaching it, switch to ` + "`execute_workflow`" + ` for remaining steps.

3. **Stale view — clear after navigation.** After any click that navigates to a new page,
   opens a dialog, or causes significant UI change, call ` + "`clear_learned_view`" + ` immediately.
   Stale positions cause mis-clicks on the wrong elements.

4. **Verify destructive actions.** After delete, submit, or confirm, always call
   ` + "`wait_for_text`" + ` to verify the outcome before proceeding.

---

## Safety & Loop Protection

Ghost MCP has **built-in safeguards** to prevent infinite retry loops and runaway automation:

- **Global call limit (25/session).** ` + "`find_and_click`" + ` stops after 25 calls. Switch to
  ` + "`execute_workflow`" + ` when approaching the limit.
- **Consecutive failure detection (3 strikes).** Failing 3 times on the same search returns a
  "GIVE UP RECOMMENDATION" — stop and try a different approach.
- **Repeated click detection (5 clicks).** Clicking the same coordinates 5 times in 30 seconds
  triggers a warning to verify the action is correct.
- **Failsafe corner (0,0).** Moving the mouse to top-left triggers emergency shutdown.

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

` + "```" + `json
{"tool": "find_and_click", "arguments": {"text": "Save"}}
` + "```" + `

If you are unsure the click succeeded, chain with ` + "`wait_for_text`" + `:

` + "```" + `json
{"tool": "find_and_click", "arguments": {"text": "Save"}}
{"tool": "wait_for_text",  "arguments": {"text": "Saved successfully", "visible": true, "timeout_ms": 5000}}
` + "```" + `

### Scenario B — Fill a simple form

` + "```" + `json
{"tool": "find_click_and_type", "arguments": {"text": "Username", "type_text": "alice@example.com"}}
{"tool": "find_click_and_type", "arguments": {"text": "Password", "type_text": "hunter2"}}
{"tool": "find_and_click",      "arguments": {"text": "Sign in"}}
{"tool": "wait_for_text",       "arguments": {"text": "Dashboard", "visible": true, "timeout_ms": 8000}}
` + "```" + `

Or in one call using ` + "`execute_workflow`" + ` (screen is learned once; all steps share that cached map):

` + "```" + `json
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "type",  "text": "Username", "value": "alice@example.com"},
      {"action": "type",  "text": "Password", "value": "hunter2"},
      {"action": "click", "text": "Sign in"}
    ]
  }
}
` + "```" + `

> ` + "`execute_workflow`" + ` does not support ` + "`wait_for_text`" + `. To verify post-login navigation, call
> ` + "`wait_for_text`" + ` as a separate tool call after the workflow completes.

### Scenario C — Multiple instructions given at once (batch, same screen)

Batch Actions (Same Screen): ALWAYS use ` + "`execute_workflow`" + `. It caches the screen map once,
running 3–6× faster than individual calls. Constraint: all steps MUST be on the same page.
If spanning multiple pages, use individual calls and call ` + "`clear_learned_view`" + ` between pages.

` + "```" + `json
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "click", "text": "Accept"},
      {"action": "type",  "text": "Name", "value": "Bob"},
      {"action": "click", "text": "Submit"}
    ]
  }
}
` + "```" + `

### Scenario D — Click multiple distinct elements atomically

` + "`find_and_click_all`" + ` clicks every element in the list in a single call.
Use it when you need to click several known, distinct targets that are all
present on screen — for example, checking three checkboxes or clicking a
sequence of toolbar buttons. Do NOT use it to try alternative labels for one
target; use ` + "`find_and_click`" + ` with ` + "`select_best=true`" + ` for fuzzy matching instead.

` + "```" + `json
{
  "tool": "find_and_click_all",
  "arguments": {"texts": "[\"Enable logging\", \"Enable metrics\", \"Enable tracing\"]"}
}
` + "```" + `

To dismiss a dialog where you know only one label will match:

` + "```" + `json
{"tool": "find_and_click", "arguments": {"text": "OK"}}
` + "```" + `

### Scenario E — Scroll and find content

` + "```" + `json
{"tool": "scroll_until_text", "arguments": {"text": "Privacy Policy", "direction": "down", "max_scrolls": 10}}
` + "```" + `

Or use ` + "`find_and_click`" + ` with scroll parameters when you also want to click it:

` + "```" + `json
{"tool": "find_and_click", "arguments": {"text": "Privacy Policy", "scroll_direction": "down", "max_scrolls": 10}}
` + "```" + `

### Scenario F — Verify the UI changed before acting

Always use ` + "`wait_for_text`" + ` rather than a fixed delay:

` + "```" + `json
{"tool": "find_and_click", "arguments": {"text": "Delete"}}
{"tool": "wait_for_text",  "arguments": {"text": "Are you sure?", "visible": true, "timeout_ms": 3000}}
{"tool": "find_and_click", "arguments": {"text": "Confirm"}}
{"tool": "wait_for_text",  "arguments": {"text": "Deleted", "visible": true, "timeout_ms": 5000}}
` + "```" + `

### Scenario G — Complex app with many elements / deep scrolling

Invest in learning once; all subsequent find operations use the cached map:

` + "```" + `json
{"tool": "learn_screen",        "arguments": {"max_pages": 5}}
{"tool": "get_learned_view",    "arguments": {}}
{"tool": "find_and_click",      "arguments": {"text": "Settings"}}
{"tool": "find_click_and_type", "arguments": {"text": "API Key", "type_text": "abc123"}}
{"tool": "find_and_click",      "arguments": {"text": "Save"}}
{"tool": "clear_learned_view",  "arguments": {}}
` + "```" + `

` + "`learn_screen`" + ` scans the full interface across scroll positions. ` + "`get_learned_view`" + ` returns the
element list — inspect it to verify OCR found the targets. ` + "`find_and_click`" + ` uses the cached
bounding box (fast — no full-screen scan). ` + "`clear_learned_view`" + ` discards after navigation to
avoid stale positions.

### Scenario H — Unknown interface (first time seeing it)

Use a strict hierarchy to explore before acting:

1. **Primary — fast text dump:** ` + "`find_elements`" + ` returns all visible text with coordinates and types.

` + "```" + `json
{"tool": "find_elements", "arguments": {}}
` + "```" + `

2. **Secondary — visual inspection:** Only if ` + "`find_elements`" + ` misses visual cues (icons without text),
   use ` + "`take_screenshot`" + ` to visually inspect.

` + "```" + `json
{"tool": "take_screenshot", "arguments": {}}
` + "```" + `

3. **Tertiary — scrollable/long page:** Only if the page scrolls and you need below-fold elements,
   use ` + "`learn_screen`" + `.

` + "```" + `json
{"tool": "learn_screen", "arguments": {"max_pages": 5}}
` + "```" + `

After ` + "`find_elements`" + ` returns results:
- If you see the target element → use ` + "`find_and_click`" + ` or ` + "`find_click_and_type`" + ` directly
- If the interface is large or scrollable → proceed to ` + "`learn_screen`" + `
- If you need to act on many elements → switch to ` + "`execute_workflow`" + ` or ` + "`learn_screen`" + ` path

### Scenario I — One-off click on a new screen (convenience)

` + "`smart_click`" + ` combines ` + "`learn_screen`" + ` + ` + "`find_and_click`" + ` in a single call.
Use it when you need one click on an unfamiliar screen and don't want to manage
the learn/act/clear lifecycle manually:

` + "```" + `json
{"tool": "smart_click", "arguments": {"text": "Submit"}}
` + "```" + `

For more than one action on the same screen, use ` + "`learn_screen`" + ` explicitly so
you control when ` + "`clear_learned_view`" + ` is called.

---

## 3. Learning Mode — When and Why

**Learning mode is ON by default.** The first OCR call (` + "`find_and_click`" + `, ` + "`find_elements`" + `, etc.)
automatically runs ` + "`learn_screen`" + ` if no cached view exists yet. You do not need to call
` + "`set_learning_mode`" + ` or ` + "`learn_screen`" + ` manually to get this benefit.

**Call ` + "`learn_screen`" + ` explicitly when:**
- You want to control exactly when the scan happens (before a burst of actions).
- The page is long/scrollable and you need ` + "`max_pages > 1`" + `.
- The view is stale after navigation (after ` + "`clear_learned_view`" + `).

**Call ` + "`clear_learned_view`" + ` when:**
- You navigate to a new page, open a dialog, or close a modal.
- ` + "`find_and_click`" + ` starts mis-clicking (stale cached coordinates).
- You want to force a fresh scan of the current screen.

**Lifecycle:**

` + "```" + `json
{"tool": "learn_screen",        "arguments": {}}
{"tool": "find_and_click",      "arguments": {"text": "..."}}
{"tool": "find_click_and_type", "arguments": {"text": "...", "type_text": "..."}}
{"tool": "clear_learned_view",  "arguments": {}}
` + "```" + `

**Performance:** After ` + "`learn_screen`" + `, each ` + "`find_and_click`" + ` narrows its OCR scan
to a small bounding box — typically 10–25× faster than a full-screen scan.

**Debugging:** If ` + "`find_and_click`" + ` fails to find an element after ` + "`learn_screen`" + `,
call ` + "`get_learned_view`" + ` to see the full element list. If the target is missing,
OCR did not detect it — try ` + "`take_screenshot`" + ` to check visibility, or re-run
` + "`learn_screen`" + ` with a higher ` + "`max_pages`" + ` value.

---

## 4. Troubleshooting & Fallbacks

UI agents fail frequently due to dynamic loading, OCR misses, or timing issues.
When a tool returns an error or unexpected result, follow these recovery paths:

### ` + "`wait_for_text`" + ` times out

Do NOT blindly retry. First assess the actual UI state:

` + "```" + `json
{"tool": "find_elements", "arguments": {}}
` + "```" + `

Or take a screenshot to visually inspect:

` + "```" + `json
{"tool": "take_screenshot", "arguments": {}}
` + "```" + `

Only retry after confirming what is actually on screen.

### ` + "`find_and_click`" + ` fails (element not found)

The error response includes ` + "`candidates`" + ` (what OCR actually parsed) and a ` + "`suggestion`" + ` field:

- ` + `"scroll_may_help"` + ` → add ` + "`scroll_direction: \"down\"`" + ` to the same call
- ` + `"try_different_search_term"` + ` → call ` + "`find_elements`" + ` to see the real OCR text, then use the
  exact string it returned
- ` + `"no_matches_found"` + ` → the element may not be on screen; check with ` + "`take_screenshot`" + `

Do NOT retry the exact same text. Try a shorter or different term, or use ` + "`find_elements`" + ` first:

` + "```" + `json
{"tool": "find_elements", "arguments": {}}
` + "```" + `

If still failing after two attempts with different terms, re-run ` + "`learn_screen`" + ` to rebuild
the cached view.

### Action succeeds but the UI does not change

The button may be disabled or the click may have missed. Steps:

1. Call ` + "`wait_for_text`" + ` with a short timeout to see if a delayed response arrives.
2. Call ` + "`find_elements`" + ` to confirm the button exists and check its label exactly.
3. If the button appears disabled, look for an enabling condition (required field, checkbox).

### ` + "`find_and_click`" + ` returns stale coordinates (mis-click)

The learned view is out of date. Call ` + "`clear_learned_view`" + ` then ` + "`learn_screen`" + ` before retrying:

` + "```" + `json
{"tool": "clear_learned_view", "arguments": {}}
{"tool": "learn_screen",       "arguments": {}}
` + "```" + `
`
