# Multi-Step Workflows with Learning Mode

## Overview

Learning mode is **PERFECT for multi-step workflows** because you learn the screen ONCE, then execute many steps quickly using the cached view.

## The Problem

Without learning mode, each step requires:
1. Full-screen OCR scan (~2-3 seconds per step)
2. Element search
3. Action (click/type)

**Example: 5-step form = 10-15 seconds**

## The Solution: `execute_workflow`

With learning mode workflow:
1. Learn screen ONCE (~3 seconds)
2. Execute ALL steps using cached view (~0.1 seconds per step)
3. Clear view when done

**Example: 5-step form = ~3.5 seconds (3-4x faster!)**

## Usage

### Basic Form Filling

```json
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "click", "text": "Email:"},
      {"action": "type", "text": "Email:", "value": "user@example.com"},
      {"action": "click", "text": "Password:"},
      {"action": "type", "text": "Password:", "value": "secret123"},
      {"action": "click", "text": "Sign In"}
    ],
    "clear_view_after": true
  }
}
```

### Multi-Page Wizard

```json
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "click", "text": "Next"},
      {"action": "wait", "delay_ms": 500},
      {"action": "click", "text": "Continue"},
      {"action": "wait", "delay_ms": 500},
      {"action": "click", "text": "Finish"}
    ],
    "clear_view_after": false
  }
}
```

### Complex Workflow with Delays

```json
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "click", "text": "Settings"},
      {"action": "wait", "delay_ms": 1000},
      {"action": "click", "text": "Advanced"},
      {"action": "type", "text": "Timeout:", "value": "30"},
      {"action": "scroll", "amount": 10, "direction": "down"},
      {"action": "click", "text": "Save"}
    ],
    "clear_view_after": true
  }
}
```

## Supported Actions

### `click` - Click a button/link
```json
{"action": "click", "text": "Submit"}
```

### `type` - Click then type (for inputs)
```json
{"action": "type", "text": "Email:", "value": "user@example.com"}
```

### `wait` - Wait for specified milliseconds
```json
{"action": "wait", "delay_ms": 1000}
```

### `scroll` - Scroll the viewport
```json
{"action": "scroll", "amount": 10, "direction": "down"}
```

## Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `steps` | array | ✅ Yes | - | Array of workflow steps |
| `clear_view_after` | boolean | No | `true` | Clear learned view after workflow |

## Response Format

```json
{
  "success": true,
  "steps_executed": 5,
  "steps_failed": 0,
  "total_duration": "3.52s",
  "step_results": [
    {"step": 0, "action": "click", "success": true, "duration": "0.08s"},
    {"step": 1, "action": "type", "success": true, "duration": "0.12s"},
    {"step": 2, "action": "click", "success": true, "duration": "0.07s"},
    {"step": 3, "action": "type", "success": true, "duration": "0.15s"},
    {"step": 4, "action": "click", "success": true, "duration": "0.09s"}
  ]
}
```

## Performance Comparison

### Scenario: 10-Step Form

| Method | Time | Speed |
|--------|------|-------|
| Individual `find_and_click` calls | ~25s | Baseline |
| `execute_workflow` with learning | ~4s | **6x faster!** |

### Why So Much Faster?

1. **One-time learning**: `learn_screen` called once (~3s)
2. **Cached lookups**: Each step uses learned view (~0.1s vs ~2.5s)
3. **No redundant OCR**: Screen captured once, not per step

## Best Practices

### 1. Use for Multi-Step Tasks
```
✅ Good: 5+ steps in one workflow
❌ Bad: Single click (use smart_click instead)
```

### 2. Add Delays for Page Transitions
```json
{
  "steps": [
    {"action": "click", "text": "Next"},
    {"action": "wait", "delay_ms": 500},  // Wait for page load
    {"action": "click", "text": "Continue"}
  ]
}
```

### 3. Keep View for Related Workflows
```json
{
  "clear_view_after": false  // Keep for next workflow call
}
```

### 4. Handle Errors Gracefully
```json
{
  "success": false,
  "steps_executed": 3,
  "steps_failed": 1,
  "step_results": [
    {"step": 0, "success": true},
    {"step": 1, "success": true},
    {"step": 2, "success": false, "error": "text \"Next\" not found"}
  ]
}
```

## Example Workflows

### Login and Navigate
```json
{
  "steps": [
    {"action": "type", "text": "Username:", "value": "admin"},
    {"action": "type", "text": "Password:", "value": "secret"},
    {"action": "click", "text": "Login"},
    {"action": "wait", "delay_ms": 1000},
    {"action": "click", "text": "Dashboard"}
  ]
}
```

### E-commerce Checkout
```json
{
  "steps": [
    {"action": "click", "text": "Add to Cart"},
    {"action": "wait", "delay_ms": 500},
    {"action": "click", "text": "Checkout"},
    {"action": "type", "text": "Email:", "value": "user@example.com"},
    {"action": "type", "text": "Address:", "value": "123 Main St"},
    {"action": "click", "text": "Continue to Payment"},
    {"action": "wait", "delay_ms": 1000},
    {"action": "click", "text": "Place Order"}
  ],
  "clear_view_after": true
}
```

### Settings Configuration
```json
{
  "steps": [
    {"action": "click", "text": "Settings"},
    {"action": "wait", "delay_ms": 500},
    {"action": "click", "text": "Advanced"},
    {"action": "type", "text": "Timeout:", "value": "60"},
    {"action": "type", "text": "Retries:", "value": "3"},
    {"action": "scroll", "amount": 5, "direction": "down"},
    {"action": "click", "text": "Save Changes"},
    {"action": "wait", "delay_ms": 1000},
    {"action": "click", "text": "OK"}
  ]
}
```

## Error Handling

The workflow continues on errors and reports which steps failed:

```json
{
  "success": false,
  "steps_executed": 4,
  "steps_failed": 1,
  "step_results": [
    {"step": 0, "action": "click", "success": true},
    {"step": 1, "action": "type", "success": true},
    {"step": 2, "action": "click", "success": false, "error": "text \"Submit\" not found"},
    {"step": 3, "action": "click", "success": true},
    {"step": 4, "action": "click", "success": true}
  ]
}
```

## When to Use

### ✅ Perfect For:
- Multi-step forms (login, registration, checkout)
- Wizard navigation (Next → Next → Finish)
- Batch operations (select multiple items)
- Complex workflows with delays
- Repetitive tasks

### ❌ Use Other Tools For:
- Single click → Use `smart_click`
- Need precise control → Use individual tools
- Screen changes mid-workflow → Use separate workflows

## Files

- `cmd/ghost-mcp/tools_workflow.go` - Implementation
- `cmd/ghost-mcp/main.go` - Registration

## Related Tools

- `smart_click` - Single click with auto-learn
- `learn_screen` - Manual screen learning
- `find_and_click` - Individual click operation
- `find_and_click_all` - Click multiple buttons
