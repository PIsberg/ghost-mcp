#!/bin/bash
# test_runner.sh - Unix/macOS test runner for Ghost MCP
#
# This script builds and runs all tests for the Ghost MCP server.
#
# Usage:
#   ./test_runner.sh              - Run unit tests only
#   ./test_runner.sh integration  - Run integration tests (requires GCC)
#   ./test_runner.sh all          - Run all tests
#   ./test_runner.sh fixture      - Start test fixture server only

set -e

echo ""
echo "========================================"
echo "   Ghost MCP Test Runner"
echo "========================================"
echo ""

# Check for Go
if ! command -v go &> /dev/null; then
    echo "[ERROR] Go is not installed or not in PATH"
    echo "Download from: https://go.dev/dl/"
    exit 1
fi

echo "[INFO] Go version:"
go version
echo ""

# Parse arguments
TEST_TYPE="${1:-unit}"

# Detect binary name
BINARY="ghost-mcp"
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]; then
    BINARY="ghost-mcp.exe"
fi

# Build the main binary first
echo "[STEP 1] Building $BINARY..."
go build -o "$BINARY" -ldflags="-s -w" .
if [ $? -ne 0 ]; then
    echo ""
    echo "[ERROR] Build failed!"
    echo ""
    echo "Common issues:"
    echo "  - GCC not installed (required for robotgo)"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo "    macOS: xcode-select --install"
    elif [[ "$OSTYPE" == "linux-gnu" ]]; then
        echo "    Linux: sudo apt install gcc libx11-dev xorg-dev libxtst-dev libpng-dev"
    fi
    echo "  - Missing dependencies"
    echo "    Run: go mod download"
    echo ""
    exit 1
fi
echo "[OK] Build successful"
echo ""

if [ "$TEST_TYPE" == "fixture" ]; then
    echo "[STEP 2] Starting test fixture server..."
    echo ""
    echo "Test Fixture URL: http://localhost:8765"
    echo "Press Ctrl+C to stop"
    echo ""
    go run test_fixture/fixture_server.go
    exit 0
fi

# Run unit tests
if [ "$TEST_TYPE" == "unit" ]; then
    echo "[STEP 2] Running unit tests..."
    echo ""
    go test -v -short ./...
    goto_summary
fi

# Run integration tests
if [ "$TEST_TYPE" == "integration" ]; then
    echo "[STEP 2] Running integration tests..."
    echo ""
    echo "WARNING: Integration tests will control your mouse and keyboard!"
    echo "Do not run while working on important tasks."
    echo ""
    read -p "Continue? (y/n): " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "[INFO] Integration tests cancelled"
        exit 0
    fi
    echo ""
    
    # Check for GCC
    if ! command -v gcc &> /dev/null; then
        echo "[ERROR] GCC not found in PATH"
        echo "Integration tests require GCC for robotgo"
        echo ""
        if [[ "$OSTYPE" == "darwin"* ]]; then
            echo "Install Xcode Command Line Tools:"
            echo "  xcode-select --install"
        elif [[ "$OSTYPE" == "linux-gnu" ]]; then
            echo "Install GCC and X11 dev libraries:"
            echo "  sudo apt install gcc libx11-dev xorg-dev libxtst-dev libpng-dev"
        fi
        echo ""
        exit 1
    fi
    
    # Start fixture server in background
    echo "[INFO] Starting test fixture server..."
    go run test_fixture/fixture_server.go &
    FIXTURE_PID=$!
    sleep 3
    
    # Ensure cleanup on exit
    trap "kill $FIXTURE_PID 2>/dev/null || true" EXIT
    
    echo "[INFO] Running integration tests..."
    echo ""
    INTEGRATION=1 go test -v -run Integration ./...
    goto_summary
fi

# Run all tests
if [ "$TEST_TYPE" == "all" ]; then
    echo "[STEP 2] Running all tests..."
    echo ""
    
    echo "--- Unit Tests ---"
    echo ""
    go test -v -short ./...
    echo ""
    
    echo "--- Integration Tests ---"
    echo ""
    echo "WARNING: Integration tests will control your mouse and keyboard!"
    echo ""
    
    if ! command -v gcc &> /dev/null; then
        echo "[SKIP] GCC not found, skipping integration tests"
        goto_summary
    fi
    
    # Start fixture server
    echo "[INFO] Starting test fixture server..."
    go run test_fixture/fixture_server.go &
    FIXTURE_PID=$!
    sleep 3
    
    # Ensure cleanup on exit
    trap "kill $FIXTURE_PID 2>/dev/null || true" EXIT
    
    INTEGRATION=1 go test -v -run Integration ./...
    goto_summary
fi

echo "[ERROR] Unknown test type: $TEST_TYPE"
echo ""
echo "Usage:"
echo "  $0              - Run unit tests only"
echo "  $0 integration  - Run integration tests"
echo "  $0 all          - Run all tests"
echo "  $0 fixture      - Start fixture server"
exit 1

goto_summary() {
    echo ""
    echo "========================================"
    echo "   Test Run Complete"
    echo "========================================"
    echo ""
}
