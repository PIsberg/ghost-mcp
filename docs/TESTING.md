# Ghost MCP Testing Guide

This document describes how to test the Ghost MCP server, including unit tests, integration tests, and the interactive test fixture.

## Table of Contents

- [Overview](#overview)
- [Test Types](#test-types)
- [Running Tests](#running-tests)
- [Benchmarks](#benchmarks)
- [Test Fixture](#test-fixture)
- [Writing New Tests](#writing-new-tests)
- [Troubleshooting](#troubleshooting)

## Overview

Ghost MCP uses a multi-layered testing approach:

```
┌─────────────────────────────────────────────────────────────┐
│                    Test Pyramid                              │
│                                                              │
│                      ╱─────╲                                 │
│                     ╱       ╲                                │
│                    ╱  E2E    ╲                               │
│                   ╱───────────╲                              │
│                  ╱ Integration ╲                             │
│                 ╱───────────────╲                            │
│                ╱     Unit        ╲                           │
│               ╱───────────────────╲                          │
│              ╱─────────────────────╲                         │
│             ╱───────────────────────╲                        │
│            ╱─────────────────────────╲                       │
│           ╱───────────────────────────╲                      │
│          ╱─────────────────────────────╲                     │
│         ╱───────────────────────────────╲                    │
│        ╱─────────────────────────────────╲                   │
│       ╱───────────────────────────────────╲                  │
│      ╱────────────────────────────────────╲                 │
│     ╱──────────────────────────────────────╲                │
│    ╱────────────────────────────────────────╲               │
│   ╱──────────────────────────────────────────╲              │
│  ╱────────────────────────────────────────────╲             │
│ ╱──────────────────────────────────────────────╲            │
│╱────────────────────────────────────────────────╲           │
└─────────────────────────────────────────────────────────────┘
```

1. **Unit Tests** - Test individual functions and handlers
2. **Integration Tests** - Test MCP client ↔ server communication
3. **E2E Tests** - Test full automation workflows with the fixture GUI

## Test Types

### Unit Tests (`main_test.go`)

Test the MCP tool handlers and parameter extraction logic without actually controlling hardware.

**Characteristics:**
- Fast execution (< 1 second)
- No external dependencies
- No display required
- Can run in CI/CD

**What they test:**
- Parameter extraction (`getStringParam`, `getIntParam`)
- Handler response formats
- Error handling
- Logging functions

### Integration Tests (`integration_test.go`)

Test the full MCP server by actually calling tools through the stdio transport.

**Characteristics:**
- Moderate execution (5-30 seconds)
- Requires GCC/MinGW (for robotgo)
- Requires a display environment
- Controls mouse and keyboard

**What they test:**
- Full tool invocation
- RobotGo integration
- Response parsing
- Error conditions

### E2E Tests (via test fixture)

Test complete automation workflows against a real GUI application.

**Characteristics:**
- Slowest execution (30+ seconds)
- Requires fixture server running
- Full UI automation
- Validates real-world scenarios

**What they test:**
- Button clicking
- Text input
- Form interactions
- Screen capture

## Running Tests

### Quick Start

```bash
# Windows
test_runner.bat

# macOS/Linux
chmod +x test_runner.sh
./test_runner.sh
```

### Running Specific Test Types

#### Unit Tests Only

```bash
# Using test runner
test_runner.bat unit
./test_runner.sh unit

# Direct go test
go test -v -short ./...
```

#### Integration Tests

```bash
# Using test runner
test_runner.bat integration
./test_runner.sh integration

# Direct go test
set INTEGRATION=1  # Windows
export INTEGRATION=1  # macOS/Linux
go test -v -tags integration -run TestIntegration ./...
```

#### All Tests

```bash
test_runner.bat all
./test_runner.sh all
```

#### Test Fixture Server Only

```bash
test_runner.bat fixture
./test_runner.sh fixture
```

Then open http://localhost:8765 in your browser.

## Benchmarks

Ghost MCP now includes fixture-backed OCR benchmarks that run against static screenshots instead of the live desktop. This keeps the measurements repeatable while still exercising the real OCR pipeline.

### Benchmark Targets

- `internal/ocr/benchmark_fixture_test.go`
  Measures `ReadImage` on the saved OCR panel screenshot in grayscale and color modes.
- `cmd/ghost-mcp/benchmark_fixture_test.go`
  Measures `parallelFindText` on the saved fixture screenshot in grayscale and color-only modes.

### Run Current Branch Benchmarks

```powershell
$env:PATH="$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\bin;$env:PATH"
$env:TESSDATA_PREFIX="$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\share\tessdata"

go test -run '^$' -bench 'BenchmarkReadImage_FixturePanel' -benchmem ./internal/ocr
go test -run '^$' -bench 'BenchmarkParallelFindText_FixtureButtons' -benchmem ./cmd/ghost-mcp
```

### Compare Against `main`

```powershell
.\benchmarks\compare-ocr.ps1
```

The comparison runner:

- Creates a temporary worktree for `origin/main`
- Copies the benchmark files into that worktree so the same harness runs on both code versions
- Saves timestamped JSON + raw text outputs under `benchmarks/results/history/`
- Refreshes `benchmarks/results/latest-current.json`
- Refreshes `benchmarks/results/latest-baseline.json`
- Refreshes `benchmarks/results/latest-comparison.json`

### Notes

- The cross-branch comparison currently uses the shared `internal/ocr` benchmarks because those APIs exist on both `origin/main` and the optimized branch.
- The `cmd/ghost-mcp` benchmarks are still saved in the current-branch result files, so future runs on branches that contain the benchmark harness can be compared directly via the same JSON history.

### Running Individual Tests

```bash
# Run specific unit test
go test -v -run TestGetStringParamValid

# Run specific integration test
set INTEGRATION=1
go test -v -run TestIntegration_GetScreenSize

# Run tests with coverage
go test -cover ./...

# Run tests with race detector
go test -race -short ./...
```

## Test Fixture

The test fixture is an interactive web-based GUI application designed for testing UI automation.

### Starting the Fixture

```bash
# Build and run
go run test_fixture/fixture_server.go

# Or use test runner
test_runner.bat fixture
./test_runner.sh fixture
```

The fixture will be available at: **http://localhost:8765**

### Fixture Features

| Feature | Description |
|---------|-------------|
| **Button Grid** | 4 colored buttons for click testing |
| **Text Input** | Single-line and multi-line text fields |
| **Checkboxes** | 3 checkboxes for toggle testing |
| **Radio Buttons** | 3 radio buttons for selection testing |
| **Dropdown** | Select element for dropdown testing |
| **Slider** | Range input for slider testing |
| **Color Picker** | Color-changing box for visual verification |
| **Click Counter** | Button with click counter |
| **Hover Zone** | Area that detects mouse hover |
| **Event Log** | Real-time log of all interactions |
| **Test Results** | Summary of test outcomes |

### Using the Fixture with MCP

Once the fixture is running, you can control it through the MCP server:

```json
// Move mouse to button area
{"tool": "move_mouse", "arguments": {"x": 400, "y": 300}}

// Click the button
{"tool": "click", "arguments": {"button": "left"}}

// Type in the input field
{"tool": "move_mouse", "arguments": {"x": 400, "y": 400}}
{"tool": "click", "arguments": {"button": "left"}}
{"tool": "type_text", "arguments": {"text": "Hello, Ghost MCP!"}}

// Take a screenshot
{"tool": "take_screenshot"}
```

### Fixture State API

The fixture exposes its state via JavaScript for programmatic verification:

```javascript
// In browser console
const state = window.getFixtureState();
console.log(state);

// Returns:
{
  buttonsClicked: ["primary", "success"],
  inputTyped: true,
  checkboxesToggled: [...],
  radioSelected: "a",
  dropdownChanged: true,
  sliderAdjusted: true,
  keysPressed: [...],
  totalClicks: 5,
  hoverActive: false
}
```

## Writing New Tests

### Adding Unit Tests

```go
// main_test.go

func TestHandleYourTool(t *testing.T) {
    // Create request
    request := mcp.CallToolRequest{
        Params: mcp.CallToolRequestParams{
            Arguments: map[string]interface{}{
                "param": "value",
            },
        },
    }

    // Call handler
    ctx := context.Background()
    result, err := handleYourTool(ctx, request)

    // Verify
    if err != nil {
        t.Errorf("Unexpected error: %v", err)
    }

    if result.IsError {
        t.Error("Expected success, got error")
    }

    // Parse and verify response
    var response YourResponseType
    json.Unmarshal([]byte(result.Content[0].Text), &response)
    if response.ExpectedField != expectedValue {
        t.Errorf("Expected %v, got %v", expectedValue, response.ExpectedField)
    }
}
```

### Adding Integration Tests

```go
// integration_test.go

func TestIntegration_YourFeature(t *testing.T) {
    if os.Getenv("INTEGRATION") != "1" {
        t.Skip("Integration tests not enabled")
    }

    skipIfNoGCC(t)
    skipIfNoDisplay(t)

    client, err := mcpclient.NewClient(mcpclient.Config{
        Timeout: testTimeout,
    })
    if err != nil {
        t.Fatalf("Failed to create client: %v", err)
    }
    defer client.Close()

    ctx := context.Background()

    // Your test logic
    err = client.YourTool(ctx, "param")
    if err != nil {
        t.Errorf("YourTool failed: %v", err)
    }
}
```

### Test Best Practices

1. **Always check for INTEGRATION env var** in integration tests
2. **Use skipIfNoGCC and skipIfNoDisplay** helpers
3. **Always defer client.Close()** to clean up processes
4. **Use settleTime delays** after UI actions
5. **Avoid failsafe position (0,0)** in mouse tests
6. **Clean up temporary files** after screenshots

## Troubleshooting

### Common Issues

#### "GCC not found"

```bash
# Windows (Chocolatey)
choco install mingw -y

# Windows (MSYS2)
pacman -S mingw-w64-x86_64-gcc

# macOS
xcode-select --install

# Ubuntu/Debian
sudo apt install gcc libx11-dev xorg-dev libxtst-dev libpng-dev

# Fedora
sudo dnf install gcc libX11-devel libXtst-devel libpng-devel
```

#### "No display available"

Integration tests require a display:

- **Linux**: Ensure X11 is running, or use Xvfb for headless
- **CI/CD**: Use Xvfb or similar virtual display
- **Windows/macOS**: Display is usually available by default

#### "Binary not found"

Build the binary first:

```bash
go build -o ghost-mcp.exe .  # Windows
go build -o ghost-mcp .      # macOS/Linux
```

#### Tests hang or timeout

- Check that no other process is blocking stdin/stdout
- Ensure the fixture server isn't already running on port 8765
- Try increasing the timeout in integration tests

#### Mouse/keyboard not responding

- **macOS**: Grant accessibility permissions
- **Linux**: Check X11 permissions (`xhost +`)
- **Windows**: Run as Administrator if needed

### Debug Mode

Enable verbose logging:

```bash
# Set environment variable
set GHOST_MCP_LOG_LEVEL=DEBUG  # Windows
export GHOST_MCP_LOG_LEVEL=DEBUG  # macOS/Linux

# Run tests
go test -v ./...
```

### Test Output Interpretation

```
=== RUN   TestIntegration_GetScreenSize
--- PASS: TestIntegration_GetScreenSize (0.52s)
    integration_test.go:123: Screen size: 1920x1080
```

- `=== RUN` - Test started
- `--- PASS` or `--- FAIL` - Test result
- Indented lines - Test output (t.Logf)

### Performance Tips

- Run unit tests frequently (fast feedback)
- Run integration tests before commits
- Use `-short` flag to skip slow tests during development
- Run full suite in CI/CD

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]

    steps:
    - uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.22'

    - name: Install dependencies (Linux)
      if: matrix.os == 'ubuntu-latest'
      run: |
        sudo apt update
        sudo apt install -y gcc libx11-dev xorg-dev libxtst-dev libpng-dev xvfb

    - name: Install dependencies (macOS)
      if: matrix.os == 'macos-latest'
      run: xcode-select --install

    - name: Download dependencies
      run: go mod download

    - name: Run unit tests
      run: go test -v -short ./...

    - name: Run integration tests (with virtual display)
      if: matrix.os == 'ubuntu-latest'
      run: |
        Xvfb :99 &
        export DISPLAY=:99
        export INTEGRATION=1
        go test -v -tags integration -run TestIntegration ./...
```

## Test Coverage

Generate coverage report:

```bash
# Run tests with coverage
go test -cover ./...

# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View HTML report
go tool cover -html=coverage.out
```

Target coverage:
- Unit tests: > 80%
- Integration tests: Critical paths covered
