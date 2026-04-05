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
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(ghostMCPGuide)),
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
| **Map the interface for the first time** | ` + "`learn_screen`" + ` |
| **Visually see elements and their IDs** | ` + "`get_annotated_view`" + ` |
| **Click an element by its visual ID badge** | ` + "`click_at(id=N)`" + ` |
| **Focus and type into a field by its ID** | ` + "`click_and_type(id=N)`" + ` |
| Click a button or link by its visible label | ` + "`find_and_click`" + ` |
| Click a field and type into it | ` + "`find_click_and_type`" + ` |
| Read all visible text + coordinates quickly | ` + "`find_elements`" + ` |
| Wait for a UI change before proceeding | ` + "`wait_for_text`" + ` |
| Run multiple sequential steps on one screen | ` + "`execute_workflow`" + ` |
| Learn screen + click in one convenience call | ` + "`smart_click`" + ` |
| Inspect what learn_screen found / debug misses | ` + "`get_learned_view`" + ` |
| Scroll and search for text | ` + "`scroll_until_text`" + ` |
| Take a raw screenshot (visual-only) | ` + "`take_screenshot`" + ` |

**Avoid:** using ` + "`take_screenshot`" + ` to read text — use ` + "`get_annotated_view`" + ` instead to see labels AND IDs.

---

## 2. Optimal Tool Paths by Scenario

### Scenario A — First Time on a Screen (REQUIRED FIRST STEP)

Always start with a learning scan to understand the layout and gain visual IDs:

` + "```" + `json
{"tool": "learn_screen", "arguments": {"max_pages": 1}}
{"tool": "get_annotated_view", "arguments": {}}
` + "```" + `

Now you can see the screen with numeric ID badges (e.g. ` + "`[5]`" + `, ` + "`[12]`" + `). Use these IDs for 100% click precision:

` + "```" + `json
{"tool": "click_at", "arguments": {"id": 12}}
` + "```" + `

### Scenario B — Click a button by label (Quick Task)

If you only need one quick click and the UI is familiar:

` + "```" + `json
{"tool": "find_and_click", "arguments": {"text": "Save"}}
` + "```" + `

### Scenario C — Fill a complex form

1. Map the screen once: ` + "`learn_screen`" + `
2. Act on elements using the cached map (10× faster):

` + "```" + `json
{"tool": "find_click_and_type", "arguments": {"text": "Username", "type_text": "alice@example.com"}}
{"tool": "find_click_and_type", "arguments": {"text": "Password", "type_text": "hunter2"}}
{"tool": "find_and_click",      "arguments": {"text": "Sign in"}}
` + "```" + `

---

## 3. Exploration Hierarchy — How to Discover the UI

When encountering an unknown interface, follow this strict priority:

1. **Primary — Learn & Annotate:** ` + "`learn_screen`" + ` followed by ` + "`get_annotated_view`" + `. This gives you a machine-readable map AND a visual confirmation with precise IDs.
2. **Secondary — Fast Text Dump:** ` + "`find_elements`" + ` only if you need a quick, non-image list of visible text.
3. **Tertiary — Raw Visual:** ` + "`take_screenshot`" + ` only if OCR fails to find icon-only buttons.

` + "```" + `json
{"tool": "learn_screen",        "arguments": {"max_pages": 3}}
{"tool": "get_annotated_view",  "arguments": {}}
` + "```" + `

After ` + "`find_elements`" + ` returns results:
- If you see the target element → use ` + "`find_and_click`" + ` or ` + "`find_click_and_type`" + ` directly
- If the screen is COMPLEX (many buttons/fields) → use ` + "`get_annotated_view`" + ` then ` + "`click_at(id=N)`" + `

**OFF-SCREEN ELEMENTS (MULTI-PAGE):**
- If the target is likely below the fold, use ` + "`learn_screen(max_pages=3)`" + ` to scan everything.
- To see IDs for lower sections, use ` + "`get_annotated_view(page_index=1)`" + ` (for the second page).
- **IMPORTANT**: To click an ID on another page, you MUST scroll there first before calling ` + "`click_at(id=N)`" + `.

### SCENARIO A: First Time on a New Screen
1. ` + "`learn_screen(max_pages=2)`" + ` → map the current interface.
2. ` + "`get_annotated_view`" + ` → see visual ID badges for the top section.
3. (Optional) ` + "`get_annotated_view(page_index=1)`" + ` → see visual ID badges for below the fold.
4. ` + "`click_at(id=5)`" + ` → 100% precise interaction.

### Scenario I — One-off click on a new screen (convenience)

` + "`smart_click`" + ` combines ` + "`learn_screen`" + ` + ` + "`find_and_click`" + ` in a single call.
Use it when you need one click on an unfamiliar screen and don't want to manage
the learn/act/clear lifecycle manually:

` + "```" + `json
{"tool": "smart_click", "arguments": {"text": "Submit"}}
` + "```" + `

### Scenario J — Hard-to-reach or icon-heavy UIs (Visual Anchors)

If ` + "`find_and_click`" + ` fails or you see many similar buttons (e.g. 10 trash icons),
use the **Visual Anchor** workflow for 100% precision:

1. **Get the map:** ` + "```" + `json {"tool": "get_annotated_view"} ` + "```" + `
2. **Inspect the image:** Look for the numeric ID badge (e.g. ` + "`[12]`" + `) next to your target.
3. **Click by ID:** ` + "```" + `json {"tool": "click_at", "arguments": {"id": 12}} ` + "```" + `

This eliminates OCR "drift" and ensures you hit exactly the element you see.

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
