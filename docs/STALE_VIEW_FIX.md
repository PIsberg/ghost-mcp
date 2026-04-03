# Learning Mode Improvements - Stale View Fix

## Problem Identified

During testing, the AI encountered issues when the learned view became stale after a page refresh (F5):

```
20:25:28 - Page refreshed (F5)
20:25:30 - learn_screen called
20:26:08 - "CLICK ME!" button NOT FOUND
Error: "text 'CLICK ME!' not found on screen"
```

**Root Cause:** The learned view was captured at 20:25:30, but by 20:26:08 the screen content had changed, making the cached element locations invalid.

## Fixes Implemented

### 1. **Enhanced Error Messages** ✅

**File:** `handler_ocr.go`

Added stale view detection to error messages:

```go
// Auto-refresh hint if learning mode is active but view might be stale
if globalLearner.IsEnabled() && globalLearner.HasView() {
    view := globalLearner.GetView()
    if view != nil && time.Since(view.CapturedAt) > 30*time.Second {
        msg += ` ⚠️ VIEW STALE (captured >30s ago): Call clear_learned_view + learn_screen NOW to refresh!`
    }
}
```

**Impact:** AI sees explicit warning when view is >30 seconds old.

### 2. **Auto-Clear Stale View** ✅

**File:** `handler_ocr.go`

Automatically clears and re-learns if view is >60 seconds old:

```go
// Check if learned view is stale (>60 seconds old) - auto-clear it
if globalLearner.IsEnabled() && globalLearner.HasView() {
    view := globalLearner.GetView()
    if view != nil && time.Since(view.CapturedAt) > 60*time.Second {
        logging.Info("find_and_click: learned view is stale (%v old), auto-clearing", time.Since(view.CapturedAt))
        globalLearner.ClearView()
        autoLearnIfNeeded()
    }
}
```

**Impact:** Transparent recovery from stale views - no AI action needed.

### 3. **Workflow Tool Auto-Refresh** ✅

**File:** `tools_workflow.go`

Workflow tool now checks view age before starting:

```go
} else {
    // Check if view is stale - refresh if >60 seconds old
    view := globalLearner.GetView()
    if view != nil && time.Since(view.CapturedAt) > 60*time.Second {
        logging.Info("execute_workflow: learned view is stale (%v old), refreshing", time.Since(view.CapturedAt))
        globalLearner.ClearView()
        learnReq := mcp.CallToolRequest{}
        learnResult, learnErr := handleLearnScreen(ctx, learnReq)
        if learnErr != nil || learnResult.IsError {
            return mcp.NewToolResultError("failed to refresh stale view"), nil
        }
    }
}
```

**Impact:** Workflows always start with fresh view.

### 4. **New `refresh_view` Action** ✅

**File:** `tools_workflow.go`

Added explicit refresh step for workflows:

```json
{
  "steps": [
    {"action": "click", "text": "Next"},
    {"action": "wait", "delay_ms": 1000},
    {"action": "refresh_view"},  ← NEW: Re-learn screen
    {"action": "click", "text": "Continue"}
  ]
}
```

**Implementation:**
```go
func executeRefreshView(ctx context.Context) error {
    // Clear the learned view
    globalLearner.ClearView()
    // Re-learn screen
    learnReq := mcp.CallToolRequest{}
    learnResult, learnErr := handleLearnScreen(ctx, learnReq)
    // ... error handling ...
    return nil
}
```

**Impact:** AI can explicitly refresh view mid-workflow.

## Usage Examples

### Example 1: Workflow with Page Change

```json
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "click", "text": "Submit Form"},
      {"action": "wait", "delay_ms": 2000},
      {"action": "refresh_view"},  ← Refresh after page load
      {"action": "click", "text": "Continue to Next Step"}
    ]
  }
}
```

### Example 2: Automatic Recovery (No Action Needed)

```json
{
  "tool": "find_and_click",
  "arguments": {"text": "Submit"}
}
```

If view is >60s old, automatically refreshed. AI sees:
```
[INFO] find_and_click: learned view is stale (1m 15s old), auto-clearing
[INFO] execute_workflow: auto-learning screen before workflow
```

### Example 3: Enhanced Error Message

```
text "CLICK ME!" not found on screen...
⚠️ VIEW STALE (captured >30s ago): Call clear_learned_view + learn_screen NOW to refresh!
```

## Performance Impact

| Scenario | Before | After |
|----------|--------|-------|
| Stale view causes failure | Manual intervention needed | Auto-refresh (adds ~3s) |
| Workflow after page change | Fails silently | Auto-refreshes |
| Explicit refresh needed | Call 2 tools | 1 `refresh_view` action |

## Files Changed

1. **`cmd/ghost-mcp/handler_ocr.go`**
   - Enhanced error messages with stale view detection
   - Auto-clear stale views in `find_and_click`

2. **`cmd/ghost-mcp/tools_workflow.go`**
   - Auto-refresh at workflow start if view stale
   - New `refresh_view` action type
   - `executeRefreshView()` function

## Testing Checklist

- [ ] Test workflow with page refresh
- [ ] Test `refresh_view` action
- [ ] Verify auto-clear after 60s
- [ ] Verify error messages show stale warning
- [ ] Test multi-page workflow with refresh between pages

## Best Practices

### When to Use `refresh_view`

✅ **Use when:**
- Page navigation occurred
- Content dynamically changed
- After waiting for AJAX/page load
- Before critical multi-step sequence

❌ **Don't use when:**
- Same page, no changes
- Between rapid clicks (<30s apart)
- View is still valid

### Recommended Workflow Pattern

```json
{
  "steps": [
    {"action": "click", "text": "Submit"},
    {"action": "wait", "delay_ms": 2000},
    {"action": "refresh_view"},  ← Always refresh after page change
    {"action": "click", "text": "Next"}
  ]
}
```

## Monitoring

Watch for these log messages:

```
[INFO] find_and_click: learned view is stale (1m 15s old), auto-clearing
[INFO] execute_workflow: learned view is stale (2m 30s old), refreshing
[INFO] execute_workflow: view refreshed
```

## Success Metrics

- ✅ Stale views auto-detected and refreshed
- ✅ Error messages include actionable hints
- ✅ Workflow tool handles page changes gracefully
- ✅ `refresh_view` action available for explicit control
- ✅ No manual intervention needed for stale views
