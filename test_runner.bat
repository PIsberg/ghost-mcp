@echo off
REM test_runner.bat - Windows test runner for Ghost MCP
REM
REM This script builds and runs all tests for the Ghost MCP server.
REM
REM Usage:
REM   test_runner.bat              - Run unit tests only
REM   test_runner.bat integration  - Run integration tests (requires GCC)
REM   test_runner.bat all          - Run all tests
REM   test_runner.bat fixture      - Start test fixture server only

setlocal enabledelayedexpansion

echo.
echo ========================================
echo    Ghost MCP Test Runner
echo ========================================
echo.

REM Check for Go
where go >nul 2>&1
if %errorlevel% neq 0 (
    echo [ERROR] Go is not installed or not in PATH
    echo Download from: https://go.dev/dl/
    exit /b 1
)

echo [INFO] Go version:
go version
echo.

REM Parse arguments
set TEST_TYPE=%1
if "%TEST_TYPE%"=="" set TEST_TYPE=unit

REM Build the main binary first
echo [STEP 1] Building ghost-mcp...
go build -o ghost-mcp.exe -ldflags="-s -w" .
if %errorlevel% neq 0 (
    echo.
    echo [ERROR] Build failed!
    echo.
    echo Common issues:
    echo   - GCC/MinGW not installed (required for robotgo)
    echo     Install with: choco install mingw
    echo   - Missing dependencies
    echo     Run: go mod download
    echo.
    exit /b 1
)
echo [OK] Build successful
echo.

if "%TEST_TYPE%"=="fixture" (
    echo [STEP 2] Starting test fixture server...
    echo.
    echo Test Fixture URL: http://localhost:8765
    echo Press Ctrl+C to stop
    echo.
    go run test_fixture/fixture_server.go
    exit /b 0
)

REM Run unit tests
if "%TEST_TYPE%"=="unit" (
    echo [STEP 2] Running unit tests...
    echo.
    go test -v -short ./...
    goto :summary
)

REM Run integration tests
if "%TEST_TYPE%"=="integration" (
    echo [STEP 2] Running integration tests...
    echo.
    echo WARNING: Integration tests will control your mouse and keyboard!
    echo Do not run while working on important tasks.
    echo.
    set /p CONFIRM="Continue? (y/n): "
    if /i not "!CONFIRM!"=="y" (
        echo [INFO] Integration tests cancelled
        exit /b 0
    )
    echo.
    
    REM Check for GCC
    where gcc >nul 2>&1
    if %errorlevel% neq 0 (
        echo [ERROR] GCC not found in PATH
        echo Integration tests require GCC/MinGW for robotgo
        echo.
        echo Install MinGW:
        echo   Option 1: choco install mingw
        echo   Option 2: Download from https://www.mingw-w64.org/
        echo.
        exit /b 1
    )
    
    REM Start fixture server in background
    echo [INFO] Starting test fixture server...
    start /B go run test_fixture/fixture_server.go
    timeout /t 3 /nobreak >nul
    
    echo [INFO] Running integration tests...
    echo.
    set INTEGRATION=1
    go test -v -run Integration ./...
    goto :summary
)

REM Run all tests
if "%TEST_TYPE%"=="all" (
    echo [STEP 2] Running all tests...
    echo.
    
    echo --- Unit Tests ---
    echo.
    go test -v -short ./...
    echo.
    
    echo --- Integration Tests ---
    echo.
    echo WARNING: Integration tests will control your mouse and keyboard!
    echo.
    
    where gcc >nul 2>&1
    if %errorlevel% neq 0 (
        echo [SKIP] GCC not found, skipping integration tests
        goto :summary
    )
    
    REM Start fixture server
    echo [INFO] Starting test fixture server...
    start /B go run test_fixture/fixture_server.go
    timeout /t 3 /nobreak >nul
    
    set INTEGRATION=1
    go test -v -run Integration ./...
    goto :summary
)

echo [ERROR] Unknown test type: %TEST_TYPE%
echo.
echo Usage:
echo   test_runner.bat              - Run unit tests only
echo   test_runner.bat integration  - Run integration tests
echo   test_runner.bat all          - Run all tests
echo   test_runner.bat fixture      - Start fixture server
exit /b 1

:summary
echo.
echo ========================================
echo    Test Run Complete
echo ========================================
echo.
