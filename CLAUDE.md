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
go test -v -short ./cmd/ghost-mcp/...

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

The server exposes six tools: `get_screen_size`, `move_mouse`, `click`, `type_text`, `press_key`, `take_screenshot`. Source lives in `cmd/ghost-mcp/`.

### Critical Design Constraints

- **stdout is reserved for the MCP JSON-RPC protocol.** All logging must go to stderr. Use `logInfo()`, `logError()`, `logDebug()` — never `fmt.Println`.
- **Failsafe mechanism**: moving the mouse to (0,0) triggers an emergency shutdown. `checkFailsafe()` is called after every mouse movement. Do not bypass this.
- **Parameter extraction**: JSON numbers arrive as `float64`. Use `getIntParam()` and `getStringParam()` helpers rather than direct type assertions.
- **Debug logging**: controlled by the `GHOST_MCP_DEBUG=1` environment variable.

### Testing Architecture

- `cmd/ghost-mcp/main_test.go` — unit tests for parameter extraction, handlers, logging, and failsafe
- `cmd/ghost-mcp/integration_test.go` — full MCP server tests using the helper client in `mcpclient/`
- `cmd/ghost-mcp/test_fixture/` — a Go HTTP server + HTML/JS page that renders interactive UI elements for integration tests to automate against

Integration tests are gated behind the `INTEGRATION=1` env var because they require a real display (or Xvfb on Linux) and a GCC toolchain for CGo.

### Platform Notes

- **Windows**: Requires MinGW-w64 GCC for the robotgo C bindings.
- **macOS**: Requires Xcode Command Line Tools and accessibility permissions. CI skips the build on macOS 15+ due to a `CGDisplayCreateImageForRect` deprecation in robotgo.
- **Linux**: Requires `libx11-dev`, `xorg-dev`, `libxtst-dev`, `libpng-dev`. Integration tests in CI use Xvfb.

### CI/CD

GitHub Actions (`.github/workflows/test.yml`) runs a matrix over Ubuntu/Windows/macOS × Go 1.22/1.23. Coverage is uploaded to Codecov from the Linux/Go 1.23 job.
