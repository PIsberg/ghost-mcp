# Ghost MCP Architecture

This document describes the internal architecture and design decisions of the Ghost MCP server.

## System Overview

![Architecture Diagram](./diagrams/01-architecture.png)

The Ghost MCP server sits between an AI client (like Claude) and the operating system, providing a safe, sandboxed interface for UI automation through the MCP protocol over stdio.

## Core Components

### 1. Main Entry Point (`main()`)

![Startup Diagram](./diagrams/06-startup.png)

The entry point orchestrates server initialization:

1. **Logging Setup**: Initializes stderr-based logging
2. **Token Validation**: Reads `GHOST_MCP_TOKEN`; exits with error if not set
3. **Signal Handling**: Registers handlers for SIGINT/SIGTERM
4. **Server Creation**: Calls `createServer(token)` to build the MCP server
5. **Blocking Serve**: Calls `server.ServeStdio()` which blocks until shutdown

### 2. MCP Server (`createServer(token)`)

Creates and configures the MCP server instance with an authentication hook:

```go
hooks := &server.Hooks{}
hooks.AddOnRequestInitialization(makeTokenValidator(token))

mcpServer := server.NewMCPServer(
    ServerName,     // "ghost-mcp"
    ServerVersion,  // "1.0.0"
    server.WithResourceCapabilities(true, true),
    server.WithHooks(hooks),
)
```

The `OnRequestInitialization` hook runs before every MCP request and rejects calls where `GHOST_MCP_TOKEN` no longer matches the token captured at startup.

### 3. Tool Registration (`registerTools()`)

Registers each tool with its schema and handler. Mouse/keyboard/screen tools live in `cmd/ghost-mcp/main.go`; OCR tools live in `cmd/ghost-mcp/tools_ocr.go` and `handler_ocr.go`:

```
registerTools()                          (main.go)
  │
  ├─► AddTool("get_screen_size",  handleGetScreenSize)
  ├─► AddTool("move_mouse",       handleMoveMouse)
  ├─► AddTool("click",            handleClick)
  ├─► AddTool("click_at",         handleClickAt)
  ├─► AddTool("double_click",     handleDoubleClick)
  ├─► AddTool("scroll",           handleScroll)
  ├─► AddTool("scroll_until_text", handleScrollUntilText)
  ├─► AddTool("type_text",        handleTypeText)
  ├─► AddTool("click_and_type",   handleClickAndType)
  ├─► AddTool("press_key",        handlePressKey)
  ├─► AddTool("take_screenshot",  handleTakeScreenshot)
  └─► registerOCRTools()               (tools_ocr.go)
        ├─► AddTool("find_elements",    handleFindElements)
        ├─► AddTool("find_and_click",   handleFindAndClick)
        ├─► AddTool("find_click_and_type", handleFindClickAndType)
        ├─► AddTool("find_and_click_all",  handleFindAndClickAll)
        └─► AddTool("wait_for_text",    handleWaitForText)
```

Each tool definition includes:
- **Name**: Unique identifier
- **Description**: Human-readable explanation
- **Parameters**: Typed arguments with descriptions
- **Handler**: Function that executes the tool

### 4. Tool Handlers

![Tool Handler Flow](./diagrams/03-tool-handler.png)

Each tool follows a consistent pattern:

#### Handler Signatures

All handlers follow the MCP SDK convention:

```go
func handleToolName(
    ctx context.Context, 
    request mcp.CallToolRequest,
) (*mcp.CallToolResult, error)
```

### 5. Concurrent OCR Engine Pipeline

The `handler_ocr.go` pipeline leverages a pure-concurrency model to minimize latency when detecting UI elements that require extensive preprocessing (like inverted or bright text):

1. **Primed Tesseract Client Pool**: OCR uses a `sync.Pool` of `*gosseract.Client` instances. The pool lazily primes four clients on first use with a tiny warm-up image so `eng.traineddata` is loaded once per pooled worker instead of once per request. This removes repeated `TessBaseAPI::Init()` disk reads from steady-state OCR traffic while preserving lock-free concurrent access.
2. **Prepared OCR Variant Set**: Before launching the OCR race, the handler preprocesses and upscales each required variant exactly once (`normal`, `inverted`, `bright-text`, `color`) and passes the encoded bytes into the worker goroutines. This moves the large scaling allocations out of the hot race and prevents four concurrent passes from rebuilding the same 3x images independently.
   - `Normal`: Unmodified contrast
   - `Inverted`: Flips pixels (dark to light)
   - `Color`: Preserves color (no grayscale)
   - `Bright-Text`: Highlights pure-white letters
3. **Short-Circuit Cancellation**: A synchronized `context.WithCancel` channel structure immediately aborts and garbage-collects all remaining fallback passes the millisecond a matched coordinate is found.
4. **Fast Wait Polling**: `wait_for_text` polls every 100ms instead of 500ms, so UI transition checks return closer to the actual screen change rather than spending most of their latency budget asleep between captures.

### 7. Smart OCR Matching with Region Cache

![Smart OCR Matching](./diagrams/07-ocr-smart-matching.png)

The `find_and_click` tool now includes intelligent features for faster and more reliable UI automation:

#### Region Cache

After successfully finding and clicking a button, the screen region is cached:

```go
type RegionCache struct {
    entries   map[string]*RegionCacheEntry
    maxSize   int           // 100 entries max
    maxAge    time.Duration // 24 hours
    stats     CacheStats
}
```

**Benefits:**
- **10-25x faster** on repeat clicks (scans ~100x50px region vs 1920x1080 full screen)
- **Case-insensitive** lookups ("Click", "CLICK", "click" all use same cache)
- **Screen resolution aware** (auto-invalidates on resolution change)
- **LRU eviction** (keeps 100 most recently used entries)

#### Smart Matching Scoring

Instead of simple substring matching, every OCR-detected word is scored:

| Score | Match Type | Example |
|-------|------------|---------|
| 1000 | Exact match | "Click Me!" = "Click Me!" |
| 500 | Prefix match | "Click Me!" starts with "Click" |
| 400 | Suffix match | "Button Click" ends with "Click" |
| 300 | Standalone word | "Click" as separate word |
| 100 | Substring | "Click" with word boundaries |
| 50 | Inside word | "Click" inside "Button Click Tests" |

This ensures the **best match** is selected, not just the first match found.

#### Viewport Awareness

When text is not found, the error response includes actionable suggestions:

```json
{
  "error": "text \"Submit\" not found...",
  "candidates": [{"text": "Submit", "score": 300, "confidence": 95.2}],
  "suggestion": "scroll_may_help",
  "consecutive_failures": 2,
  "remaining_calls": 23
}
```

**Suggestion values:**
- `scroll_may_help` - Text might be off-screen
- `text_continues_off_screen` - Partial text visible
- `try_different_search_term` - Related text found
- `no_matches_found` - Nothing similar on screen

### 8. Safety Features & Loop Protection

![Component Diagram](./diagrams/08-classes-updated.png)

Built-in safeguards prevent infinite retry loops and runaway automation:

#### Global Call Limit (25 calls/session)
```
WARNING: Only 5 tool calls remaining before forced stop.
MAXIMUM TOOL CALLS REACHED (25). Stop and try a completely different approach.
```

#### Consecutive Failure Detection (3 strikes)
```
GIVE UP RECOMMENDATION: Failed 3 times with "Submit". STOP trying this text.
Try: (1) find_elements to see actual text, (2) shorter search term, 
     (3) scroll_direction if off-screen, or (4) completely different approach.
```

#### Repeated Click Detection (5 clicks at same spot)
```json
{
  "success": true,
  "warning": {
    "should_stop": true,
    "reason": "Clicked same coordinates (403,767) 5 times in 30 seconds",
    "click_count": 5,
    "message": "You've clicked this spot 5 times. Verify this is correct before continuing."
  }
}
```

#### Call Tracker
```go
type callTracker struct {
    consecutiveFailures  map[string]int  // key: tool:text
    clickHistory         []clickHistoryEntry
    totalCalls           int
}
```

Tracks:
- Consecutive failures per tool+text combination
- Click history (last 100 clicks, 30-second window)
- Total calls remaining before forced stop

### 9. New Verification Tool: click_until_text_appears

Clicks coordinates and waits for confirmation text:

```json
{
  "tool": "click_until_text_appears",
  "arguments": {
    "x": 400,
    "y": 300,
    "wait_for_text": "Saved!",
    "timeout_ms": 5000,
    "max_clicks": 3
  }
}
```

**Use cases:**
- Wait for "Saved!" after clicking Save button
- Verify menu opened by waiting for menu text
- Confirm navigation by waiting for page title
- Auto-retry if first click missed

### 10. Performance Optimizations

#### Removed Redundant OCR
Previously, `getMatchCandidates()` ran a second OCR pass on every successful `find_and_click`. Now:
- **Success**: Fast response with just the click result
- **Failure**: Full candidates array to help AI debug

This cuts the first response time in half for successful clicks.

#### Increased Scroll Amount
Default scroll amount increased from 3/5 to **15**:
- `scroll` tool: 3 → 15 (5x faster page coverage)
- `scroll_until_text`: 5 → 15 (3x faster search)

Now only 3-4 scrolls needed to cover a full page instead of 10+.

### 6. Parameter Extraction Helpers

Generic functions for type-safe parameter extraction:

```go
// getStringParam extracts a string parameter
func getStringParam(request mcp.CallToolRequest, name string) (string, error)

// getIntParam extracts an integer parameter (handles float64 from JSON)
func getIntParam(request mcp.CallToolRequest, name string) (int, error)
```

These handle JSON's tendency to decode numbers as `float64`.

### 6. Logging System

**Critical Design Decision**: All logs go to stderr.

```
┌────────────────────────────────────────────────────┐
│                    stdout                          │
│  (MCP JSON-RPC Protocol - MUST stay clean)         │
│  {"jsonrpc":"2.0","result":{...}}                  │
└────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────┐
│                    stderr                          │
│  (Application Logs - Safe for debugging)           │
│  [INFO] Starting ghost-mcp v1.0.0...               │
│  [DEBUG] Handling move_mouse request               │
│  [ERROR] Invalid parameter: x                      │
└────────────────────────────────────────────────────┘
```

Logging functions:
- `logInfo()`: Informational messages
- `logError()`: Error conditions
- `logDebug()`: Debug output (only when `GHOST_MCP_DEBUG=1`)

### 7. Failsafe Mechanism

Emergency shutdown to prevent runaway automation:

```
checkFailsafe()
  │
  ├─► robotgo.GetMousePos() - Get current position
  │
  ├─► if x == 0 && y == 0
  │     │
  │     ├─► logError("FAILSAFE TRIGGERED...")
  │     │
  │     └─► initiateShutdown()
  │           │
  │           ├─► Set state.isShuttingDown = true
  │           │
  │           └─► Close state.shutdownChan
  │
  └─► return nil (if not triggered)
```

**When triggered**:
1. Logs error to stderr
2. Sets shutdown flag
3. Closes shutdown channel
4. Returns error to tool caller

### 8. Global State

```go
type serverState struct {
    shutdownChan chan struct{}  // Signal for shutdown
    isShuttingDown bool          // Shutdown flag
}

var state = &serverState{
    shutdownChan: make(chan struct{}),
}
```

## Data Flow

### Request Flow (Client → Server → Tool)

```
1. Client sends JSON-RPC request via stdin
   {"jsonrpc":"2.0","method":"tools/call",
    "params":{"name":"move_mouse","arguments":{"x":100,"y":200}}}

2. MCP SDK parses and validates JSON

3. SDK routes to registered handler
   handleMoveMouse(ctx, request)

4. Handler extracts parameters
   x := getIntParam(request, "x")  // 100
   y := getIntParam(request, "y")  // 200

5. Handler calls RobotGo
   robotgo.Move(100, 200)

6. Handler checks failsafe
   checkFailsafe()

7. Handler returns result
   return mcp.NewToolResultText(`{"success":true,"x":100,"y":200}`)

8. MCP SDK formats response
   {"jsonrpc":"2.0","result":{"content":[{"text":"..."}]}}

9. Response written to stdout
```

### Response Format

Most tool responses are JSON strings in a single `text` content block:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      { "type": "text", "text": "{\"success\": true, \"x\": 100, \"y\": 200}" }
    ]
  }
}
```

`take_screenshot` returns two content blocks — JSON metadata followed by the PNG image:

```json
{
  "result": {
    "content": [
      { "type": "text",  "text": "{\"success\": true, \"filepath\": \"...\", \"width\": 1920, \"height\": 1080}" },
      { "type": "image", "data": "<base64-png>", "mimeType": "image/png" }
    ]
  }
}
```

## Error Handling

### Parameter Errors

```go
// Missing parameter
x, err := getIntParam(request, "x")
if err != nil {
    return mcp.NewToolResultError(err.Error()), nil
}

// Invalid type
button, err := getStringParam(request, "button")
if err != nil {
    return mcp.NewToolResultError(fmt.Sprintf("invalid button: %v", err)), nil
}
```

### RobotGo Errors

```go
bitmap := robotgo.CaptureScreen(x, y, width, height)
if bitmap == nil {
    return mcp.NewToolResultError("failed to capture screen"), nil
}
```

### Failsafe Errors

```go
if err := checkFailsafe(); err != nil {
    return mcp.NewToolResultError(err.Error()), nil
}
```

## Concurrency Model

![Concurrency Diagram](./diagrams/04-concurrency.png)

The server handles requests sequentially via ServeStdio(), which is appropriate for stdio transport.
## Tool Specifications

### get_screen_size

| Aspect | Details |
|--------|---------|
| **Purpose** | Get primary monitor dimensions |
| **Parameters** | None |
| **Returns** | `{"width": int, "height": int}` |
| **RobotGo Call** | `robotgo.GetScreenSize()` |

### move_mouse

| Aspect | Details |
|--------|---------|
| **Purpose** | Move cursor to absolute coordinates |
| **Parameters** | `x` (int, required), `y` (int, required) |
| **Returns** | `{"success": bool, "x": int, "y": int}` |
| **RobotGo Call** | `robotgo.Move(x, y)` |
| **Failsafe** | ✓ Checked after movement |

### click

| Aspect | Details |
|--------|---------|
| **Purpose** | Click at current cursor position |
| **Parameters** | `button` ("left", "right", "middle") |
| **Returns** | `{"success": bool, "button": string, "x": int, "y": int}` |
| **RobotGo Call** | `robotgo.Click(button, false)` |
| **Failsafe** | ✓ Checked after click |

### click_at

| Aspect | Details |
|--------|---------|
| **Purpose** | Move to coordinates and click in one call |
| **Parameters** | `x` (int, required), `y` (int, required), `button` (string, default "left") |
| **Returns** | `{"success": bool, "button": string, "x": int, "y": int}` |
| **RobotGo Calls** | `robotgo.Move(x, y)`, `robotgo.Click(button, false)` |
| **Failsafe** | ✓ Checked between move and click |

### double_click

| Aspect | Details |
|--------|---------|
| **Purpose** | Move to coordinates and double-click |
| **Parameters** | `x` (int, required), `y` (int, required) |
| **Returns** | `{"success": bool, "x": int, "y": int}` |
| **RobotGo Calls** | `robotgo.Move(x, y)`, `robotgo.Click("left", true)` |
| **Failsafe** | ✓ Checked between move and click |

### scroll

| Aspect | Details |
|--------|---------|
| **Purpose** | Move to coordinates and scroll the mouse wheel |
| **Parameters** | `x` (int, required), `y` (int, required), `direction` ("up"/"down"/"left"/"right"), `amount` (int, default 3) |
| **Returns** | `{"success": bool, "x": int, "y": int, "direction": string, "amount": int}` |
| **RobotGo Calls** | `robotgo.Move(x, y)`, `robotgo.ScrollDir(amount, direction)` |
| **Failsafe** | ✓ Checked after move |

### scroll_until_text

| Aspect | Details |
|--------|---------|
| **Purpose** | Search the current viewport for text, then scroll and retry in one bounded tool call |
| **Parameters** | `text` (string, required), `direction` ("up"/"down"/"left"/"right"), `amount` (int, default 5), `max_scrolls` (int, default 8), `nth` (int, default 1), `scroll_x`/`scroll_y` (optional), OCR region `x`/`y`/`width`/`height` (optional), `grayscale` (bool, default true) |
| **Returns** | `{"success": bool, "found": string, "box": {...}, "center_x": int, "center_y": int, "scroll_count": int, "direction": string, "amount": int, "pass": string, "visible_text": string}` |
| **RobotGo Calls** | `robotgo.CaptureImg(...)`, `robotgo.Move(scroll_x, scroll_y)`, `robotgo.ScrollDir(amount, direction)` repeated up to `max_scrolls` |
| **Failsafe** | ✓ Checked before each scroll |
| **Early Stop** | Stops when two consecutive viewport fingerprints match, which prevents endless scrolling after the end of a list/page |

### OCR Caching

The OCR layer keeps a lightweight single-entry cache keyed by a fast hash of the captured image. When consecutive calls inspect the same unchanged viewport, `ReadImage()` returns the cached OCR result immediately instead of rerunning Tesseract. This specifically reduces redundant work in flows like `find_elements` → `find_and_click` on the same screen.

### type_text

| Aspect | Details |
|--------|---------|
| **Purpose** | Type text via keyboard into focused element |
| **Parameters** | `text` (string, max 10,000 chars) |
| **Returns** | `{"success": bool, "characters_typed": int}` |
| **RobotGo Call** | `robotgo.TypeStr(text)` |

### press_key

| Aspect | Details |
|--------|---------|
| **Purpose** | Press a single key (uses allowlist validation) |
| **Parameters** | `key` (string) |
| **Returns** | `{"success": bool, "key": string}` |
| **RobotGo Call** | `robotgo.KeyTap(key)` |

### take_screenshot

| Aspect | Details |
|--------|---------|
| **Purpose** | Capture screen as PNG, returned as image content |
| **Parameters** | `x`, `y`, `width`, `height` (all optional) |
| **Returns** | Text block: `{"success": bool, "filepath": string, "width": int, "height": int}` + image/png content block |
| **RobotGo Calls** | `robotgo.CaptureImg()`, `robotgo.SavePng()` |
| **Cleanup** | Temp file deleted after read unless `GHOST_MCP_KEEP_SCREENSHOTS=1` |

### find_elements

| Aspect | Details |
|--------|---------|
| **Purpose** | Capture screen region, run Tesseract OCR, return all text elements with bounding boxes, center coordinates, and element types |
| **Parameters** | `x`, `y`, `width`, `height` (all optional — defaults to full screen), `element_type` (optional string: `button`, `input`, `checkbox`, `radio`, `dropdown`, `toggle`, `slider`, `label`, `heading`, `link`, `value`, `text`) |
| **Returns** | `{"success": bool, "element_count": int, "region": {...}, "elements": [{text, type, x, y, width, height, center_x, center_y, confidence}]}` |
| **Dependencies** | Tesseract OCR (`gosseract`), `TESSDATA_PREFIX` must be set |
| **Coordinates** | Element positions are absolute screen coordinates, ready to use with `click_at` |

### find_and_click

| Aspect | Details |
|--------|---------|
| **Purpose** | Full-screen OCR scan, find nth matching word, click its center |
| **Parameters** | `text` (string, required), `button` (default "left"), `nth` (int, default 1), `element_type` (optional string: `button`, `input`, `checkbox`, `radio`, `dropdown`, `toggle`, `slider`, `label`, `heading`, `link`, `value`, `text`) |
| **Returns** | `{"success": bool, "found": string, "x": int, "y": int, "button": string, "occurrence": int}` |
| **Match logic** | Case-insensitive substring match against each OCR word, filtered by element type if specified |
| **Dependencies** | Tesseract OCR (`gosseract`), `TESSDATA_PREFIX` must be set |
| **RobotGo Calls** | `robotgo.CaptureImg()`, `robotgo.Move()`, `robotgo.Click()` |

## Dependencies

### Direct Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/mark3labs/mcp-go` | MCP protocol implementation |
| `github.com/go-vgo/robotgo` | OS-level mouse/keyboard/screen automation (CGo) |
| `github.com/otiai10/gosseract/v2` | Tesseract OCR bindings (CGo) |

### Standard Library

| Package | Purpose |
|---------|---------|
| `context` | Request context propagation |
| `encoding/base64` | Screenshot encoding |
| `fmt` | Formatting |
| `os` | File operations, env vars, stderr |
| `os/signal` | Signal handling |
| `path/filepath` | Path manipulation |
| `runtime` | Platform detection |
| `syscall` | Signal constants |
| `time` | Timestamps |

## Security Considerations

### 1. Failsafe Position

The (0,0) failsafe prevents infinite loops but:
- Users should avoid placing important UI elements at (0,0)
- AI should be instructed not to move to (0,0) intentionally

### 2. Stdio-Only Transport

- No network exposure
- Only accessible to processes that can spawn the binary
- No authentication (relies on client config security)

### 3. Permission Requirements

- Requires accessibility permissions on macOS
- May require admin on Windows for some operations
- Linux requires X11 access

### 4. Screenshot Data

- Screenshots returned as `image/png` content blocks (not base64 in JSON)
- Temp file deleted after read unless `GHOST_MCP_KEEP_SCREENSHOTS=1`
- No persistent storage of captured data by default

## Testing Strategy

### Unit Tests (`main_test.go`)

Tests focus on:
1. **Parameter extraction** - Type conversion, missing params
2. **Handler logic** - Response format, error handling
3. **Failsafe** - Shutdown triggering
4. **Logging** - No panics in logging functions

### Integration Testing

Manual testing required for:
- Actual mouse movement
- Keyboard input
- Screen capture
- Cross-platform behavior

### Test Limitations

- RobotGo requires display/graphics environment
- CI/CD needs virtual display (Xvfb, etc.)
- Some tests skipped without display

## Extension Points

### Adding New Tools

1. Implement handler function:
   ```go
   func handleNewTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
   ```

2. Register in `registerTools()`:
   ```go
   mcpServer.AddTool(mcp.NewTool(
       "new_tool",
       mcp.WithDescription("..."),
       mcp.WithString("param", mcp.Required()),
   ), handleNewTool)
   ```

### Adding New Transports

The MCP SDK supports other transports:
- HTTP/SSE for remote servers
- WebSocket for bidirectional communication

Modify `main()` to use alternative transport instead of `ServeStdio()`.

## Performance Considerations

### Memory

- Screenshots held in memory during base64 encoding
- Large screens may require significant memory
- Temp file cleanup is immediate

### Latency

- Mouse movement is instant (`robotgo.Move`) — no animation delay
- Keyboard typing has inherent OS latency
- Screenshot capture and OCR are blocking; OCR on a full screen can take ~1–2 s

### Throughput

- Sequential request handling (stdio limitation)
- No request queuing or batching
- Suitable for interactive AI use, not high-volume automation

## Debugging

### Enable Debug Logging

```json
{
  "env": {
    "GHOST_MCP_DEBUG": "1"
  }
}
```

### Log Output

Debug logs show:
- Incoming requests
- Parameter values
- RobotGo calls
- Response data

### Common Debug Scenarios

1. **Tool not found**: Check tool name in registration
2. **Parameter errors**: Verify parameter types in schema
3. **RobotGo failures**: Check platform permissions
4. **Protocol errors**: Ensure no stdout pollution
