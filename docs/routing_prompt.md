# Ghost MCP — Tool Routing Guide

Ghost MCP gives you OS-level UI automation via mouse, keyboard, screen capture,
and OCR. This guide tells you which tool to reach for first, how to chain tools
together, and when to invest in Learning Mode for complex sessions.

> This document is also delivered automatically to AI clients as the `ghost_mcp_guide`
> MCP prompt on connect. The server instructs the AI to read it before taking any actions.

---

## 0. CRITICAL RULES — Read Before Acting

These rules override everything else. Violating them can cause data loss or runaway loops.

1. **Failsafe — NEVER move the mouse to (0, 0).** Moving to top-left triggers an emergency
   shutdown. Do not do this unless the goal is to stop the server.

2. **Loop protection — 25-call limit.** `find_and_click` enforces a 25-call-per-session
   limit. If you are approaching it, switch to `execute_workflow` for remaining steps.

3. **Stale view — clear after navigation.** After any click that navigates to a new page,
   opens a dialog, or causes significant UI change, call `clear_learned_view` immediately.
   Stale positions cause mis-clicks on the wrong elements.

4. **Verify destructive actions.** After delete, submit, or confirm, always call
   `wait_for_text` to verify the outcome before proceeding.

---

## 1. Quick-Reference Decision Table

| What you need to do | Best tool |
|---|---|
| **1. SCAN (Capture UI)** | `learn_screen` |
| **2. MAP (Searchable Text/IDs)** | `get_learned_view` |
| **3. VERIFY (Visual ID Badges)** | `get_annotated_view` |
| **4. ACT (Precision Click)** | `click_at(id=N)` |
| **4. ACT (Focus & Type)** | `click_and_type(id=N, text="...")` |
| **4. ACT (Hover/Drag Start)** | `move_mouse(id=N)` |
| **4. ACT (Open/Activate)** | `double_click(id=N)` |

**CRITICAL FLOW:** You must call `get_learned_view` to load the text-to-ID mappings into your context window, followed by `get_annotated_view` to visually confirm where those IDs are located.

**Avoid:** using `take_screenshot` to read text — use the 1-2-3-4 flow above instead.

---

## 2. Optimal Tool Paths by Scenario

### Scenario A — First Time on a New Screen (MANDATORY PRECISION FLOW)

You MUST follow this 4-step sequence. Skipping ANY step is a failure:

1. **Scan**: `json {"tool": "learn_screen", "arguments": {"max_pages": 1}} ` (Capture)
2. **Read Map**: `json {"tool": "get_learned_view", "arguments": {}} ` (**REQUIRED**: Load ID mappings)
3. **Verify UI**: `json {"tool": "get_annotated_view", "arguments": {}} ` (**REQUIRED**: See the ID badges)
4. **Interact**: Use IDs from Step 2 confirmed in Step 3 for 100% reliable automation.

**CRITICAL:** NEVER use `take_screenshot` to identify IDs or read labels. `get_annotated_view` is the ONLY tool that provides the visual ID badges required for interaction.

### Scenario B — Click a button by label (Quick Task)

If you only need one quick click and the UI is familiar:

```json
{"tool": "find_and_click", "arguments": {"text": "Save"}}
```

### Scenario C — Fill a complex form

1. Map the screen once: `learn_screen`
2. Act on elements using the cached map (10× faster):

```json
{"tool": "find_click_and_type", "arguments": {"text": "Username", "type_text": "alice@example.com"}}
{"tool": "find_click_and_type", "arguments": {"text": "Password", "type_text": "hunter2"}}
{"tool": "find_and_click",      "arguments": {"text": "Sign in"}}
```

### Scenario D — Multiple instructions given at once (batch, same screen)

Batch Actions (Same Screen): ALWAYS use `execute_workflow`. It caches the screen map once,
running 3–6× faster than individual calls. Constraint: all steps MUST be on the same page.
If spanning multiple pages, use individual calls and call `clear_learned_view` between pages.

```json
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
```

### Scenario E — Scroll and find content

```json
{"tool": "scroll_until_text", "arguments": {"text": "Privacy Policy", "direction": "down", "max_scrolls": 10}}
```

### Scenario F — Verify the UI changed before acting

Always use `wait_for_text` rather than a fixed delay:

```json
{"tool": "find_and_click", "arguments": {"text": "Delete"}}
{"tool": "wait_for_text",  "arguments": {"text": "Are you sure?", "visible": true, "timeout_ms": 3000}}
{"tool": "find_and_click", "arguments": {"text": "Confirm"}}
{"tool": "wait_for_text",  "arguments": {"text": "Deleted", "visible": true, "timeout_ms": 5000}}
```

### Scenario G — Complex app with many elements / deep scrolling

Invest in learning once; all subsequent find operations use the cached map:

```json
{"tool": "learn_screen",        "arguments": {"max_pages": 5}}
{"tool": "get_learned_view",    "arguments": {}}
{"tool": "find_and_click",      "arguments": {"text": "Settings"}}
{"tool": "find_click_and_type", "arguments": {"text": "API Key", "type_text": "abc123"}}
{"tool": "find_and_click",      "arguments": {"text": "Save"}}
{"tool": "clear_learned_view",  "arguments": {}}
```

### Scenario H — Exploration Hierarchy — How to Discover the UI

When encountering an unknown interface, follow this strict priority:

1. **Primary — Capture & Map:** `learn_screen` followed by `get_learned_view`. This gives you a machine-readable JSON inventory of every element and its ID.
2. **Secondary — Visual Anchor:** `get_annotated_view`. This gives you a visual "Set-of-Marks" screenshot to confirm the ID badges.
3. **Tertiary — Raw Visual:** `take_screenshot` only if OCR failed to find icon-only buttons.

### Scenario I — One-off click on a new screen (convenience)

`smart_click` combines `learn_screen` + `find_and_click` in a single call.
Use it when you need one click on an unfamiliar screen and don't want to manage
the learn/act/clear lifecycle manually:

```json
{"tool": "smart_click", "arguments": {"text": "Submit"}}
```

For more than one action on the same screen, use `learn_screen` explicitly so
you control when `clear_learned_view` is called.

### Scenario J — High-Precision Visual Anchors

When standard OCR matching fails or the UI is icon-heavy, use **Visual Anchors** (also known as Set-of-Marks) for 100% click precision.

### Mandatory 4-Step Workflow

For maximum reliability, always follow this sequence:

1.  **Scan**: Call `learn_screen` to capture the interface.
2.  **Map**: Call `get_learned_view` to load the **Machine-Map** (the list of text labels and their numeric IDs).
3.  **Verify**: Call `get_annotated_view`. This returns an image with numeric **ID badges** (e.g., `[5]`, `[12]`) overlaid on every element.
4.  **Act**: Look at the ID badge in the image and use one of the **ID-ready tools**:
    -   `click_at({"id": N})`
    -   `click_and_type({"id": N, "text": "..."})`
    -   `move_mouse({"id": N})`
    -   `double_click({"id": N})`

This eliminates "pixel drift" and ensures you interact with exactly what you see.

### Tool: get_annotated_view

Captures the current viewport and overlays visual IDs from the last scan. Use `page_index` to see IDs for elements discovered during a multi-page scroll.

**Arguments** (all optional):
- `page_index`: The scroll-page index (0, 1, 2...) from the last scan.
- `x`, `y`: Top-left of capture region (logical pixels).
- `width`, `height`: Dimensions of region.

**Returns**: JSON metadata + JPEG image:
```json
{ "success": true, "element_count": 42, "format": "jpeg", "size_bytes": 125400 }
```

---

## 3. Learning Mode — When and Why

**Learning mode is ON by default.** The first OCR call (`find_and_click`, `find_elements`, etc.)
automatically runs `learn_screen` if no cached view exists yet. You do not need to call
`set_learning_mode` or `learn_screen` manually to get this benefit.

**Call `learn_screen` explicitly when:**
- You want to control exactly when the scan happens (before a burst of actions).
- The page is long/scrollable and you need `max_pages > 1`.
- The view is stale after navigation (after `clear_learned_view`).

**Call `clear_learned_view` when:**
- You navigate to a new page, open a dialog, or close a modal.
- `find_and_click` starts mis-clicking (stale cached coordinates).
- You want to force a fresh scan of the current screen.

**Lifecycle:**

```json
{"tool": "learn_screen",        "arguments": {}}
{"tool": "find_and_click",      "arguments": {"text": "..."}}
{"tool": "find_click_and_type", "arguments": {"text": "...", "type_text": "..."}}
{"tool": "clear_learned_view",  "arguments": {}}
```

**Performance:** After `learn_screen`, each `find_and_click` narrows its OCR scan
to a small bounding box — typically 10–25× faster than a full-screen scan.

**Debugging:** If `find_and_click` fails to find an element after `learn_screen`,
call `get_learned_view` to see the full element list. If the target is missing,
OCR did not detect it — try `take_screenshot` to check visibility, or re-run
`learn_screen` with a higher `max_pages` value.

---

## 3a. Element Type Filtering — Precision Targeting

All OCR search tools support an `element_type` parameter to filter results to specific UI component types.

**Valid types:** `button`, `input`, `checkbox`, `radio`, `dropdown`, `toggle`, `slider`, `label`, `heading`, `link`, `value`, `text`

### When to Use Element Type Filtering

**Use `element_type` when:**
- Multiple elements share the same text but have different types
  - Example: "Submit" appears as both a button AND a heading — use `element_type: "button"` to target the button
- You want to discover all elements of a specific type
  - Example: Find all input fields on a form before filling them
- You need to avoid false matches on non-actionable elements
  - Example: Don't click a "Save" label when looking for the "Save" button

### Element Type Decision Guide

| Target | element_type | Example |
|--------|--------------|---------|
| Clickable buttons | `button` | Submit, Save, Cancel, OK |
| Text input fields | `input` | Username, Email, Search |
| Checkboxes | `checkbox` | "I agree", "Remember me" |
| Radio buttons | `radio` | Option selections |
| Dropdown menus | `dropdown` | Select menus |
| Field labels | `label` | "Email:", "Name:" |
| Page/section titles | `heading` | "Dashboard", "Settings" |
| Navigation links | `link` | "Home", "Profile" |
| Numeric displays | `value` | Prices, scores, counts |

### Examples

```json
// Click only the button, ignoring labels with same text
{"tool": "find_and_click", "arguments": {"text": "Submit", "element_type": "button"}}

// Get all input fields to understand form structure
{"tool": "find_elements", "arguments": {"element_type": "input"}}

// Wait for a success label to appear
{"tool": "wait_for_text", "arguments": {"text": "Saved!", "element_type": "label"}}

// Find and type into an input field
{"tool": "find_click_and_type", "arguments": {"text": "Email:", "type_text": "user@example.com", "element_type": "input"}}
```

### Important Notes

- Element types are **inferred** from text patterns, dimensions, and context — not explicitly tagged in the UI
- If you specify an `element_type` and no matching element is found, the tool returns "not found"
- You can omit `element_type` to search all types (default behavior)
- Works with all search features: scrolling, multi-page, and region caching

---

## 4. Troubleshooting & Fallbacks

UI agents fail frequently due to dynamic loading, OCR misses, or timing issues.
When a tool returns an error or unexpected result, follow these recovery paths:

### `wait_for_text` times out

Do NOT blindly retry. First assess the actual UI state:

```json
{"tool": "find_elements", "arguments": {}}
```

Or take a screenshot to visually inspect:

```json
{"tool": "take_screenshot", "arguments": {}}
```

Only retry after confirming what is actually on screen.

### `find_and_click` fails (element not found)

The error response includes `candidates` (what OCR actually parsed) and a `suggestion` field:

- `"scroll_may_help"` → add `scroll_direction: "down"` to the same call
- `"try_different_search_term"` → call `find_elements` to see the real OCR text, then use the
  exact string it returned
- `"no_matches_found"` → the element may not be on screen; check with `take_screenshot`

Do NOT retry the exact same text. Try a shorter or different term, or use `find_elements` first:

```json
{"tool": "find_elements", "arguments": {}}
```

If still failing after two attempts with different terms, re-run `learn_screen` to rebuild
the cached view.

### Action succeeds but the UI does not change

The button may be disabled or the click may have missed. Steps:

1. Call `wait_for_text` with a short timeout to see if a delayed response arrives.
2. Call `find_elements` to confirm the button exists and check its label exactly.
3. If the button appears disabled, look for an enabling condition (required field, checkbox).

### `find_and_click` returns stale coordinates (mis-click)

The learned view is out of date. Call `clear_learned_view` then `learn_screen` before retrying:

```json
{"tool": "clear_learned_view", "arguments": {}}
{"tool": "learn_screen",       "arguments": {}}
```
