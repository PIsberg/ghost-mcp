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
and OCR. Follow this guide exactly. Do not improvise tool sequences.

---

## 0. CRITICAL SAFETY RULES

These rules override everything else. Violating them causes data loss or crashes.

1. **NEVER move the mouse to (0, 0).** This triggers emergency shutdown.
2. **25-call limit.** ` + "`find_and_click`" + ` stops after 25 calls per session.
3. **Clear after navigation.** After ANY click that changes the screen (new page,
   dialog, modal), call ` + "`clear_learned_view`" + ` immediately. Stale data = mis-clicks.
4. **Verify destructive actions.** After delete/submit/confirm, call ` + "`wait_for_text`" + `
   with ` + "`visible=false`" + ` to confirm the deletion.

---

## 1. PRIMARY WORKFLOW (Recommended)

Use learning mode for major tasks. It is 10-25x faster than individual OCR calls.

### Step 1: Scan the screen
` + "```" + `json
{"tool": "set_learning_mode", "arguments": {"enabled": true}}
{"tool": "learn_screen", "arguments": {"max_pages": 5}}
` + "```" + `

### Step 2: Extract data
` + "```" + `json
{"tool": "get_learned_view", "arguments": {}}
` + "```" + `
*Analyzes all pages (scrolled content). Look for ` + "`ocr_id`" + ` to use Path A.*

### Step 3: View a specific page (if Path A fails)
` + "```" + `json
{"tool": "get_annotated_view", "arguments": {"page_index": 0}}
` + "```" + `
*Shows numbered overlays (Path B).*

### Step 4: Interact
| Action | By coordinates (Path A) | By visual_id (Path B) |
|--------|------------------------|-------------------|
| Click  | ` + "`click_at(x, y)`" + ` | ` + "`click_at(visual_id=N)`" + ` |
| Type   | ` + "`click_and_type(x, y, text=\"...\")`" + ` | ` + "`click_and_type(visual_id=N, text=\"...\")`" + ` |
| Hover  | ` + "`move_mouse(x, y)`" + ` | ` + "`move_mouse(visual_id=N)`" + ` |
| Open   | ` + "`double_click(x, y)`" + ` | ` + "`double_click(visual_id=N)`" + ` |

### After navigation — RESET
When a click changes the screen (new page, dialog opens, modal closes):

` + "```" + `json
{"tool": "clear_learned_view", "arguments": {}}
` + "```" + `

Then go back to Step 1.

---

## 2. SHORTCUTS (use only when appropriate)

These are acceptable ONLY for simple, single-action tasks on screens you
do not need to explore. If the shortcut fails, fall back to the Primary Workflow.

| Shortcut | When to use |
|----------|-------------|
| ` + "`find_and_click(text=\"Save\")`" + ` | Single known-label click. |
| ` + "`smart_click(text=\"Submit\")`" + ` | One-off click on unfamiliar screen (auto-scans). |
| ` + "`find_click_and_type(text=\"Username\", type_text=\"alice\")`" + ` | Quick text entry by label. |
| ` + "`execute_workflow(steps=[...])`" + ` | Batch multiple steps on one screen (3-6x faster). |

**If a shortcut fails or the screen is complex, switch to the Primary Workflow immediately.**

---

## 3. LONG PAGES AND OFF-SCREEN ELEMENTS

If the target might be below the fold, increase ` + "`max_pages`" + `:

` + "```" + `json
{"tool": "learn_screen", "arguments": {"max_pages": 5}}
{"tool": "get_learned_view", "arguments": {}}
` + "```" + `

── FIELD RE-TYPES & ERRORS ──────────────────────────────────────────────────
If a ` + "`type_text`" + ` or ` + "`click_and_type`" + ` call returns a verification error:
- ALWAYS use ` + "`find_and_clear`" + ` (or ` + "`Ctrl+A`" + ` then ` + "`Backspace`" + `) before retrying.
- Failure to clear creates duplicated/corrupted field values (e.g. "oldnew").

── CLICK-AND-TYPE ACCURACY ──────────────────────────────────────────────────
If text doesn't appear after typing:
- Check if the field was actually focused (look for a flashing cursor in get_annotated_view).
- If focus was missed, the tool will automatically try one clear-and-retry.
- If it still fails, the target might be a "phantom" element (visible but not focusable).

- ` + "`get_learned_view`" + ` returns elements from ALL pages at once.
- If the element is not found, call ` + "`get_annotated_view(page_index: N)`" + ` to
  visually inspect a specific page and read the visual_id from the overlay.
- ` + "`click_at(visual_id=N)`" + ` works for any indexed element. The server scrolls for you.

**NEVER** manually scroll + screenshot to find things. Index once, then act.

---

## 4. TROUBLESHOOTING

### Element not found after scan
1. Call ` + "`get_learned_view`" + ` to see what OCR actually detected.
2. If the target is missing, re-run ` + "`learn_screen`" + ` with higher ` + "`max_pages`" + `.
3. If still missing, use ` + "`get_annotated_view`" + ` to visually find it and read its visual_id.

### Click did not work (UI unchanged)
1. Call ` + "`wait_for_text`" + ` with a short timeout — the UI may be loading.
2. Call ` + "`get_learned_view`" + ` to confirm the button's exact label.
3. If the button is disabled, look for an enabling condition (required field, checkbox).

### Stale coordinates (clicking wrong element)
The learned view is out of date. Reset and re-scan:

` + "```" + `json
{"tool": "clear_learned_view", "arguments": {}}
{"tool": "learn_screen", "arguments": {"max_pages": 3}}
{"tool": "get_learned_view", "arguments": {}}
` + "```" + `

### Consecutive failures (3 strikes)
If ` + "`find_and_click`" + ` fails 3 times on the same text, STOP. Switch to the
Primary Workflow.

---

## 5. COMMON MISTAKES

| Mistake | Why it fails | Fix |
|---------|-------------|-----|
| Calling ` + "`take_screenshot`" + ` to find elements | No visual_ids, no coordinates | Use ` + "`get_learned_view`" + ` or ` + "`get_annotated_view`" + ` |
| Paging through ` + "`get_annotated_view`" + ` blindly | Wastes calls. Use ` + "`get_learned_view`" + ` to narrow the page first | Search JSON first, then view ONE page |
| Using ` + "`ocr_id`" + ` as ` + "`visual_id`" + ` | They are different. ` + "`ocr_id`" + ` is a JSON counter. ` + "`visual_id`" + ` is from the annotated image. | Read the overlay numbers from the annotated screenshot |
| Scrolling + peeking repeatedly | Wastes tool calls, slow | Use ` + "`learn_screen(max_pages: 5)`" + ` once |
| Skipping ` + "`get_learned_view`" + ` | You have no text map — you're guessing | Always call it after ` + "`learn_screen`" + ` |
| Not clearing after navigation | Stale data points to wrong elements | Call ` + "`clear_learned_view`" + ` immediately |
`
