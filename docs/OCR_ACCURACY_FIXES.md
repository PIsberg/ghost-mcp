# OCR Accuracy Improvements - Root Cause Fixes

## Problems Identified from Logs

### 1. **"CLICK ME!" Button Not Found** ❌

**Root Cause:** OCR struggles with:
- Exclamation marks (`!`)
- Multi-word text with special characters
- Styled/button text (gradients, shadows)

**Log Evidence:**
```
20:26:08 - "CLICK ME!" not found
OCR sees: ["File" "View" "discover" "in" "y!"]
Tried 5+ times, all failed
```

### 2. **Wrong Search Text (Placeholder vs Label)** ❌

**Root Cause:** AI searches for placeholder text instead of label

**Log Evidence:**
```
20:22:55 - text: "Type here or use" (PLACEHOLDER) → FAILED
Error suggested: "Search for LABEL text like 'Text Input:' or 'Email:' (not placeholder)"

20:23:25 - text: "Input:" (LABEL) → SUCCESS ✅
```

### 3. **Repeated Failures Without Adaptation** ❌

**Root Cause:** Same search text tried 5+ times, no variation

**Log Evidence:**
```
20:26:21 - "CLICK ME!" failed 4 times in same second
No attempt to try "CLICK" or "ME" or "CLICK ME"
```

---

## Fixes Implemented

### 1. **Auto-Try Text Variations** ✅

**File:** `handler_ocr.go` - `tryTextVariations()`

**What it does:**
When original text fails, automatically tries:

#### Punctuation Removal
```
"CLICK ME!"  → "CLICK ME"
"Submit?"    → "Submit"
"OK,"        → "OK"
```

#### Multi-Word Splitting
```
"CLICK ME!"  → "CLICK", "ME"
"Sign In"    → "Sign", "In"
"Get Started" → "Get", "Started"
```

**Integration:**
```go
// In handleFindAndClick, before returning error:
if variationResult, found := tryTextVariations(...); found {
    return variationResult, nil  // Success!
}
// Only return error if all variations fail
```

**Response includes variation:**
```json
{
  "success": true,
  "text": "CLICK ME!",
  "x": 832,
  "y": 554,
  "variation": "CLICK"  ← Shows what actually matched
}
```

### 2. **Enhanced Error Messages** ✅

**File:** `handler_ocr.go` - `buildFindTextFailureMessage()`

**New suggestions:**

#### (e) Use find_elements
```
(e) Use find_elements to see ALL visible text on screen.
```
**When:** Repeated failures or multi-word text

#### (f) Try without punctuation
```
(f) Try without punctuation: "CLICK ME"
```
**When:** Text contains `!`, `?`, `,`, `.`

#### (g) Try shorter substrings
```
(g) Try shorter: "CLICK" or "ME!"
```
**When:** Multi-word text (2+ words)

### 3. **Stale View Detection** ✅ (Already Implemented)

**File:** `handler_ocr.go`

**Auto-refresh when view >60s old:**
```go
if view != nil && time.Since(view.CapturedAt) > 60*time.Second {
    logging.Info("learned view is stale, auto-clearing")
    globalLearner.ClearView()
    autoLearnIfNeeded()
}
```

**Error message warning:**
```
⚠️ VIEW STALE (captured >30s ago): Call clear_learned_view + learn_screen NOW to refresh!
```

---

## Expected Impact

### Before Fixes
```
"CLICK ME!" → FAIL (5 attempts)
"Type here or use" → FAIL
Manual intervention required
```

### After Fixes
```
"CLICK ME!" → Auto-tries "CLICK ME", "CLICK", "ME" → SUCCESS ✅
"Type here or use" → Error suggests "Input:" → AI uses label → SUCCESS ✅
Repeated failures → Auto-variations tried → Higher success rate ✅
```

---

## Testing Recommendations

### Test Case 1: Punctuation
```json
{"tool": "find_and_click", "arguments": {"text": "CLICK ME!"}}
```
**Expected:** Tries "CLICK ME", "CLICK", "ME" automatically

### Test Case 2: Multi-Word
```json
{"tool": "find_and_click", "arguments": {"text": "Sign In Now"}}
```
**Expected:** Tries "Sign", "In", "Now" automatically

### Test Case 3: Placeholder vs Label
```json
{"tool": "find_click_and_type", "arguments": {"text": "Type here", "type_text": "test"}}
```
**Expected:** Error suggests using label like "Email:" or "Input:"

### Test Case 4: Stale View
```json
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "click", "text": "Submit"},
      {"action": "wait", "delay_ms": 65000},  // Wait 65s
      {"action": "click", "text": "Next"}  // Should auto-refresh view
    ]
  }
}
```
**Expected:** Auto-refreshes view before second click

---

## Files Changed

1. **`cmd/ghost-mcp/handler_ocr.go`** (+82 lines)
   - `tryTextVariations()` - New function
   - `buildFindTextFailureMessage()` - Enhanced suggestions
   - `handleFindAndClick` - Integrated variations

2. **`cmd/ghost-mcp/tools_workflow.go`** (Previous commit)
   - Auto-refresh stale views
   - `refresh_view` action

3. **`cmd/ghost-mcp/handler_ocr.go`** (Previous commit)
   - Stale view auto-detection
   - Enhanced error messages

---

## Monitoring

Watch for these log messages:

```
[INFO] tryTextVariations: trying without punctuation: "CLICK ME"
[INFO] tryTextVariations: trying word: "CLICK"
[INFO] tryTextVariations: FOUND with variation "CLICK" at (832,554)
[INFO] ACTION: Clicked "CLICK" (variation of "CLICK ME!") at (857,564)
```

---

## Success Metrics

- ✅ "CLICK ME!" finds "CLICK" or "ME" automatically
- ✅ Placeholder searches get label suggestions
- ✅ Multi-word text tries individual words
- ✅ Stale views auto-refresh
- ✅ Error messages actionable and specific
- ✅ No manual intervention for common OCR issues

---

## Remaining Challenges

### OCR Limitations (Cannot Fix in Code)
- Very small font (<10px)
- Extreme contrast issues
- Heavily stylized text (cursive, decorative)
- Non-Latin scripts without proper tessdata

### AI Behavior (Requires Prompt Engineering)
- Still needs to use `find_elements` to discover actual text
- Should try suggested variations from error messages
- Use `execute_workflow` for multi-step tasks
- Call `refresh_view` after page changes

---

## Next Steps

1. **Test with real AI client** - Verify variations work
2. **Monitor logs** - Check if "CLICK ME!" now succeeds
3. **Gather feedback** - Ask AI if error messages helpful
4. **Iterate** - Add more variation patterns if needed
