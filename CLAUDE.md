# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

Ghost MCP is an MCP (Model Context Protocol) server written in Go that exposes OS-level UI automation to AI clients like Claude. It uses the `robotgo` library to control mouse, keyboard, and screen on Windows, macOS, and Linux.

## Build Commands

```bash
# Build for current platform (run from repo root)
go build -o ghost-mcp.exe ./cmd/ghost-mcp/   # Windows
go build -o ghost-mcp ./cmd/ghost-mcp/        # macOS/Linux

# Cross-compile release builds
GOOS=windows GOARCH=amd64 go build -o ghost-mcp.exe -ldflags="-s -w" ./cmd/ghost-mcp/
GOOS=darwin  GOARCH=amd64 go build -o ghost-mcp     -ldflags="-s -w" ./cmd/ghost-mcp/
GOOS=linux   GOARCH=amd64 go build -o ghost-mcp     -ldflags="-s -w" ./cmd/ghost-mcp/
```

## Test Commands

```bash
# Unit tests only (no display required)
go test -v -short ./cmd/ghost-mcp/... ./internal/...

# Integration tests (requires display and GCC)
INTEGRATION=1 go test -v -run Integration ./cmd/ghost-mcp/...   # Linux/macOS
set INTEGRATION=1 && go test -v -run Integration ./cmd/ghost-mcp/...  # Windows

# With race detector
go test -race -short ./cmd/ghost-mcp/...

# Platform test runners
test_runner.bat              # Windows: unit tests
test_runner.bat integration  # Windows: integration tests
./test_runner.sh             # Linux/macOS: unit tests
./test_runner.sh integration # Linux/macOS: integration tests
./test_runner.sh fixture     # Start the test fixture web server
```

## Benchmark Commands

```bash
# Run all benchmark packages (no display required)
go test -bench=. -benchmem ./internal/validate/...
go test -bench=. -benchmem ./internal/audit/...
go test -bench=. -benchmem ./internal/learner/...
go test -bench=. -benchmem ./internal/ocr/...

# Generate an HTML report with charts and historical comparison
go run ./cmd/bench-report/                              # runs all packages, opens browser
go run ./cmd/bench-report/ -no-run                     # regenerate HTML from stored results
go run ./cmd/bench-report/ -benchtime=2s -count=5      # more stable numbers
go run ./cmd/bench-report/ -compare=3                  # show only last 3 runs per package

# Report is written to benchmarks/report.html
# JSON results are stored in benchmarks/results/ (committed to git for history)
```

See [`docs/BENCHMARKING.md`](docs/BENCHMARKING.md) for the full guide including CGo setup and dependency comparison workflow.

## Lint / Format

```bash
go fmt ./...
go vet ./...
gofmt -l .   # List unformatted files
```

## Architecture

### Request Flow

```
AI Client (Claude) ←→[stdio JSON-RPC]←→ Ghost MCP Server ←→[CGo]←→ RobotGo ←→ OS
```

The server exposes tools across several files:
- `cmd/ghost-mcp/main.go`: `get_screen_size`, `move_mouse`, `click`, `click_at`, `type_text`, `press_key`, `take_screenshot`
- `cmd/ghost-mcp/tools_ocr.go`: `find_and_click`, `find_and_click_all`, `find_elements`, `find_click_and_type`, `wait_for_text`
- `cmd/ghost-mcp/tools_learning.go`: `learn_screen`, `get_learned_view`, `clear_learned_view`, `set_learning_mode`

### Learning Mode

Learning mode enables the server to build a full internal picture of the current GUI or webpage **before** acting on it. Enable it by setting `GHOST_MCP_LEARNING=1` or by calling `set_learning_mode` at runtime.

**How it works:**

1. On the first `find_and_click` or `find_elements` call (or explicitly via `learn_screen`), the server:
   - Takes a screenshot of the full screen (or a specified region)
   - Runs two OCR passes (normal + inverted) to catch both dark-on-light and light-on-dark text
   - Scrolls down and repeats until content repeats or `max_pages` is reached
   - Scrolls back to the original position
   - Stores all discovered elements in an internal `View` indexed by scroll page

2. Subsequent calls use the learned view for **region-targeted lookups**:
   - `find_and_click`: narrows the OCR scan to the element's known bounding box (10–25× faster)
   - If the element was on a non-zero scroll page, the tool scrolls there first
   - `find_elements`: includes off-page elements from the learned view in the response

3. Call `clear_learned_view` after navigation or significant UI changes to rebuild.

**Key files:**
- `internal/learner/learner.go` — pure-Go data structures (`Element`, `View`, `Learner`), thread-safe lookup
- `cmd/ghost-mcp/handler_learning.go` — screen discovery algorithm, MCP handlers, `autoLearnIfNeeded()`
- `cmd/ghost-mcp/tools_learning.go` — MCP tool registrations

### Internal Packages

- `internal/logging` — stderr logging helpers (`logging.Info/Error/Debug()`). **Never use `fmt.Println`; stdout is the MCP JSON-RPC wire.**
- `internal/validate` — input validation before any robotgo call: `validate.Coords()`, `validate.ScreenRegion()`, `validate.Text()`, `validate.Key()`
- `internal/audit` — tamper-evident audit logging. Each JSONL entry carries a SHA-256 hash chain. Configured via `GHOST_MCP_AUDIT_LOG` (directory); defaults to `<UserConfigDir>/ghost-mcp/audit/`. Files rotate daily.
- `internal/transport` — transport mode selection: stdio (default) or HTTP/SSE (`GHOST_MCP_TRANSPORT=http`). HTTP mode requires Bearer token auth and uses `GHOST_MCP_HTTP_ADDR` / `GHOST_MCP_HTTP_BASE_URL`.
- `internal/ocr` — Tesseract OCR via `gosseract`. Requires Tesseract libraries installed (vcpkg on Windows). Supports grayscale, inverted, and color preprocessing passes.
- `internal/learner` — learning mode core logic. Pure Go (no CGo), fully unit-tested, thread-safe.
- `internal/visual`, `internal/cursor` — visual click feedback animations (mouse-circle effects).

### Critical Design Constraints

- **stdout is reserved for the MCP JSON-RPC protocol.** All logging must go to stderr. Use `logging.Info()`, `logging.Error()`, `logging.Debug()` from `internal/logging` — never `fmt.Println`.
- **Failsafe mechanism**: moving the mouse to (0,0) triggers an emergency shutdown. `checkFailsafe()` is called after every mouse movement. Do not bypass this.
- **Parameter validation**: use `internal/validate` functions after parameter extraction, before any OS call.
- **Authentication**: set `GHOST_MCP_TOKEN` to require a token. In stdio mode the token is checked via an MCP hook; in HTTP mode it's a Bearer header.
- **Debug logging**: controlled by the `GHOST_MCP_DEBUG=1` environment variable.
- **Learning mode**: controlled by `GHOST_MCP_LEARNING=1`. When enabled, the first OCR tool call auto-scans the screen and subsequent calls use the cached view. Can also be toggled at runtime via `set_learning_mode`.

### Testing the Learning Mode Feature

**Quick test (no display required):**
```bash
go test -v -run TestAccuracy ./cmd/ghost-mcp/...
```
This demonstrates the **133% accuracy improvement** from multi-pass OCR.

**Full integration test (requires display):**
```powershell
$env:INTEGRATION = "1"
go test -v -tags=integration -run TestIntegration_FindAndClickButton ./cmd/ghost-mcp/...
```
Current accuracy: **100%** (4/4 buttons found and clicked).

See [`docs/LEARNING_MODE_TESTING.md`](docs/LEARNING_MODE_TESTING.md) for complete testing guide.

### Testing Architecture

- `cmd/ghost-mcp/main_test.go` — unit tests for parameter extraction, handlers, logging, and failsafe
- `cmd/ghost-mcp/handler_learning_test.go` — unit tests for learning mode helpers and handlers (no robotgo required)
- `cmd/ghost-mcp/integration_test.go` — full MCP server tests using the helper client in `mcpclient/`
- `cmd/ghost-mcp/integration_learning_test.go` — learning mode integration tests against the fixture page
- `cmd/ghost-mcp/test_fixture/` — a Go HTTP server + HTML/JS page that renders interactive UI elements for integration tests to automate against. Includes a scrollable "Learning Mode Test Section" below the fold for scroll-discovery tests.
- `internal/*/..._test.go` — unit and fuzz tests for each internal package (run with `go test ./internal/...`)

Integration tests are gated behind the `INTEGRATION=1` env var because they require a real display (or Xvfb on Linux) and a GCC toolchain for CGo.

### Platform Notes

- **Windows**: Requires MinGW-w64 GCC for the robotgo C bindings.
- **macOS**: Requires Xcode Command Line Tools and accessibility permissions. CI skips the build on macOS 15+ due to a `CGDisplayCreateImageForRect` deprecation in robotgo.
- **Linux**: Requires `libx11-dev`, `xorg-dev`, `libxtst-dev`, `libpng-dev`. Integration tests in CI use Xvfb.

### CI/CD

GitHub Actions (`.github/workflows/test.yml`) runs a matrix over Ubuntu/Windows/macOS × Go 1.22/1.23. Coverage is uploaded to Codecov from the Linux/Go 1.23 job.
