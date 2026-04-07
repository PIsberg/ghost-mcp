<!-- This file mirrors the MCP prompt in cmd/ghost-mcp/prompts.go. Keep in sync. -->

# Ghost MCP — Tool Routing Guide

Ghost MCP gives you OS-level UI automation via mouse, keyboard, screen capture,
and OCR. Follow this guide exactly. Do not improvise tool sequences.

---

## 0. CRITICAL SAFETY RULES

These rules override everything else. Violating them causes data loss or crashes.

1. **NEVER move the mouse to (0, 0).** This triggers emergency shutdown.
2. **25-call limit.** `find_and_click` stops after 25 calls per session.
3. **Clear after navigation.** After ANY click that changes the screen (new page,
   dialog, modal), call `clear_learned_view` immediately. Stale data = mis-clicks.
4. **Verify destructive actions.** After delete/submit/confirm, call `wait_for_text`
   to verify the outcome before proceeding.

---

## 1. THE PRIMARY WORKFLOW

Every UI interaction follows this pattern: scan once, then use the fastest path.

```
Step 1:  learn_screen(max_pages: 3)          // Capture the full interface
Step 2:  get_learned_view()                  // Search JSON for target text
         ├─ FOUND?   → click_at(x, y)       // Path A: use coordinates from JSON
         └─ MISSING?                         // Path B: look at annotated screenshot
              → get_annotated_view()         //   the image has numbered overlays
              → find your target element     //   on each UI element (buttons, links, etc.)
              → read the overlay number      //   e.g. the number "12" on the INFO button
              → click_at(visual_id=12)       //   pass that number as visual_id
```

### Step 1 — SCAN: `learn_screen`
Indexes the full interface. **Always use max_pages: 3** as your default.
Use max_pages: 5-10 for long forms/lists.

```json
{"tool": "learn_screen", "arguments": {"max_pages": 3}}
```

### Step 2 — SEARCH: `get_learned_view`
Returns a JSON list of all elements found by OCR with their coordinates.
**Search this JSON for your target text.** (Note: Pure visual icons like ⚙️ or 🏠 that contain no text will appear with `type: "icon"` and empty text. For these, use Path B).

```json
{"tool": "get_learned_view", "arguments": {}}
```

Example output:
```json
{"elements": [
  {"ocr_id": 1, "text": "Home",   "type": "link",   "x": 100, "y": 50,  "page_index": 0},
  {"ocr_id": 2, "text": "Submit", "type": "button", "x": 350, "y": 780, "page_index": 0},
  {"ocr_id": 3, "text": "",       "type": "icon",   "x": 40,  "y": 40,  "page_index": 0}
]}
```

(`ocr_id` is just a sequence number — it is NOT a visual visual_id overlay ID.)

---

### Path A — Target FOUND in JSON (use coordinates)

If you found your target text in the JSON, click it using the x/y coordinates:

```json
{"tool": "click_at", "arguments": {"x": 350, "y": 780}}
```

This is the fast path. No image parsing needed.

---

### Path B — Target NOT FOUND in JSON (use annotated image)

If OCR missed your target (the text is not in the JSON), you must fall back
to the annotated screenshot. Call:

```json
{"tool": "get_annotated_view", "arguments": {"page_index": 0}}
```

This returns a screenshot of the UI. **On top of the normal UI, the image has
numbered overlays placed on every detected element.** These overlays are called
**visual_id overlays**. Here is exactly what they look like:

- Each visual_id overlay is a **small solid-colored rectangle** (blue, green, red, etc.)
  with a **white number** printed inside it.
- Each visual_id overlay is placed at the **top-left corner** of the UI element it labels,
  such as a button, input field, link, or icon.
- Every visual_id overlay has a **unique number** like 1, 5, 12, 23, etc.

**How to use the image:**

1. **Scan the image** for the UI element you want to interact with.
   For example: you are looking for a button labeled "INFO".
2. **Find that element** in the image. You will see the "INFO" button rendered
   as part of the normal UI.
3. **Look at the visual_id overlay** placed on or near the "INFO" button. It will be a small
   colored rectangle. Read the number inside it. Suppose it says **12**.
4. **That number is the `visual_id`.** Call:

```json
{"tool": "click_at", "arguments": {"visual_id": 12}}
```

**Important:** The `visual_id` parameter ONLY comes from reading these visual_id overlay
numbers in the `get_annotated_view` image. It is NOT the `ocr_id` from the JSON.
They are completely different numbers. Never use `ocr_id` as `visual_id`.

---

### Action tools that accept coordinates OR visual_id

| Action | By coordinates (Path A) | By visual_id overlay (Path B) |
|--------|------------------------|-------------------|
| Click | `click_at(x, y)` | `click_at(visual_id=N)` |
| Type | `click_and_type(x, y, text="...")` | `click_and_type(visual_id=N, text="...")` |
| Hover | `move_mouse(x, y)` | `move_mouse(visual_id=N)` |
| Open | `double_click(x, y)` | `double_click(visual_id=N)` |

### After navigation — RESET
When a click changes the screen (new page, dialog opens, modal closes):

```json
{"tool": "clear_learned_view", "arguments": {}}
```

Then go back to Step 1.

---

## 2. SHORTCUTS (use only when appropriate)

These are acceptable ONLY for simple, single-action tasks on screens you
do not need to explore. If the shortcut fails, fall back to the Primary Workflow.

| Shortcut | When to use |
|----------|-------------|
| `find_and_click(text="Save")` | Single known-label click. |
| `smart_click(text="Submit")` | One-off click on unfamiliar screen (auto-scans). |
| `find_click_and_type(text="Username", type_text="alice")` | Quick text entry by label. |
| `execute_workflow(steps=[...])` | Batch multiple steps on one screen (3-6x faster). |

**If a shortcut fails or the screen is complex, switch to the Primary Workflow immediately.**

---

## 3. LONG PAGES AND OFF-SCREEN ELEMENTS

If the target might be below the fold, increase `max_pages`:

```json
{"tool": "learn_screen", "arguments": {"max_pages": 5}}
{"tool": "get_learned_view", "arguments": {}}
```

- `get_learned_view` returns elements from ALL pages at once.
- If the element is not found, call `get_annotated_view(page_index: N)` to
  visually inspect a specific page and read the visual_id overlay number.
- `click_at(visual_id=N)` works for any indexed element. The server scrolls for you.

**NEVER** manually scroll + screenshot to find things. Index once, then act.

---

## 4. TROUBLESHOOTING

### Element not found after scan
1. Call `get_learned_view` to see what OCR actually detected.
2. If the target is missing, re-run `learn_screen` with higher `max_pages`.
3. If still missing, use `get_annotated_view` to visually find it and read its visual_id overlay.

### Click did not work (UI unchanged)
1. Call `wait_for_text` with a short timeout — the UI may be loading.
2. Call `get_learned_view` to confirm the button's exact label.
3. If the button is disabled, look for an enabling condition (required field, checkbox).

### Stale coordinates (clicking wrong element)
The learned view is out of date. Reset and re-scan:

```json
{"tool": "clear_learned_view", "arguments": {}}
{"tool": "learn_screen", "arguments": {"max_pages": 3}}
{"tool": "get_learned_view", "arguments": {}}
```

### Consecutive failures (3 strikes)
If `find_and_click` fails 3 times on the same text, STOP. Switch to the
Primary Workflow.

---

## 5. COMMON MISTAKES

| Mistake | Why it fails | Fix |
|---------|-------------|-----|
| Calling `take_screenshot` to find elements | No visual_id overlays, no coordinates | Use `get_learned_view` or `get_annotated_view` |
| Paging through `get_annotated_view` blindly | Wastes calls. Use `get_learned_view` to narrow the page first | Search JSON first, then view ONE page |
| Using `ocr_id` as `visual_id` | They are different. `ocr_id` is a JSON counter. `visual_id` is a visual_id overlay from the image. | Read visual_id overlay numbers from the annotated screenshot |
| Scrolling + peeking repeatedly | Wastes tool calls, slow | Use `learn_screen(max_pages: 5)` once |
| Skipping `get_learned_view` | You have no text map — you're guessing | Always call it after `learn_screen` |
| Not clearing after navigation | Stale data points to wrong elements | Call `clear_learned_view` immediately |
