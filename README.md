# Ghost MCP Server

**The Ghost in the Machine** - An MCP (Model Context Protocol) server that exposes OS-level UI automation capabilities to AI clients.

**License:** Free for personal and hobby use ([PolyForm Noncommercial](LICENSE)). Commercial use requires a license — contact [Peter Isberg](mailto:isberg.peter+gm@gmail.com).


## Overview

Ghost MCP allows AI assistants like Claude to control your computer's mouse, keyboard, and screen. This enables automation of legacy applications, GUI testing, and any task that requires interacting with applications that don't have APIs.

![ghost-mcp-1](https://github.com/user-attachments/assets/2e36118e-8dcd-4cdf-b7c2-552ca66bff80)

## Features

- 🖱️ **Mouse Control**: Move cursor, click (left/right/middle)
- ⌨️ **Keyboard Control**: Type text, press individual keys
- 📸 **Screen Capture**: Take screenshots with optional region selection
- 🛡️ **Failsafe**: Emergency shutdown by moving mouse to top-left corner (0,0)
- 📝 **Proper Logging**: All logs go to stderr, keeping stdout clean for MCP protocol

## Available Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `get_screen_size` | Returns primary monitor dimensions | None |
| `move_mouse` | Moves cursor to coordinates | `x`, `y` |
| `click` | Clicks at current position | `button` (left/right/middle) |
| `type_text` | Types text via keyboard | `text` |
| `press_key` | Presses a single key | `key` |
| `take_screenshot` | Captures screen as base64 PNG | `x`, `y`, `width`, `height` (optional) |

## Prerequisites

- **Go 1.22+** - [Download Go](https://go.dev/dl/)
- **RobotGo Dependencies** - See platform-specific requirements below

### Platform-Specific Dependencies

#### Windows

**Required: MinGW-w64 GCC Compiler**

RobotGo requires a C compiler. Install MinGW-w64:

**Option A: Using Chocolatey (Recommended)**
```powershell
# Install Chocolatey first (if not installed)
# Run PowerShell as Administrator
Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

# Install MinGW
choco install mingw -y
```

**Option B: Manual Install**
1. Download MinGW-w64 from [https://www.mingw-w64.org/](https://www.mingw-w64.org/)
2. Add `C:\Program Files\mingw-w64\bin` (or your install path) to PATH
3. Verify installation: `gcc --version`

**Option C: Using MSYS2**
```powershell
# Install MSYS2 from https://www.msys2.org/
# Then run in MSYS2 terminal:
pacman -S mingw-w64-x86_64-gcc
```

#### macOS
```bash
# Install Xcode Command Line Tools
xcode-select --install

# Grant accessibility permissions (required for automation)
# System Settings → Privacy & Security → Accessibility → Add Terminal/your IDE
```

#### Linux
```bash
# Ubuntu/Debian
sudo apt-get install libx11-dev xorg-dev libxtst-dev libpng-dev

# Fedora
sudo dnf install libX11-devel libXtst-devel libpng-devel

# Arch Linux
sudo pacman -S libx11 libxtst libpng
```

## Installation

### 1. Clone and Build

```bash
# Clone or navigate to the project directory
cd ghost-mcp

# Download dependencies
go mod download

# Build the binary
go build -o ghost-mcp.exe .    # Windows
go build -o ghost-mcp .        # macOS/Linux
```

### 2. Verify the Build

```bash
# Run tests
go test -v ./...

# Check the binary exists
ls -la ghost-mcp*  # or dir ghost-mcp* on Windows
```

## Configuration

### MCP Client Configuration (mcp.json)

Add this to your MCP client configuration to connect to Ghost MCP:

#### For Claude Desktop (Windows)
```json
{
  "mcpServers": {
    "ghost-mcp": {
      "command": "C:\\path\\to\\ghost-mcp.exe",
      "args": [],
      "env": {
        "GHOST_MCP_DEBUG": "1"
      }
    }
  }
}
```

#### For Claude Desktop (macOS/Linux)
```json
{
  "mcpServers": {
    "ghost-mcp": {
      "command": "/absolute/path/to/ghost-mcp",
      "args": [],
      "env": {
        "GHOST_MCP_DEBUG": "1"
      }
    }
  }
}
```

### Configuration File Locations

| Platform | Config Location |
|----------|-----------------|
| Windows | `%APPDATA%\Claude\mcp.json` |
| macOS | `~/Library/Application Support/Claude/mcp.json` |
| Linux | `~/.config/Claude/mcp.json` |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GHOST_MCP_DEBUG` | Enable debug logging | `0` (disabled) |

## Usage

### Starting the Server

The server runs via stdio and is typically started by your MCP client automatically. To test manually:

```bash
# Run the server (will wait for stdin/stdout communication)
./ghost-mcp

# With debug logging
GHOST_MCP_DEBUG=1 ./ghost-mcp
```

### Example Tool Calls

Once connected, AI clients can use the tools like this:

```json
// Get screen size
{
  "tool": "get_screen_size"
}
// Response: {"width": 1920, "height": 1080}

// Move mouse to center of screen
{
  "tool": "move_mouse",
  "arguments": {"x": 960, "y": 540}
}
// Response: {"success": true, "x": 960, "y": 540}

// Left click
{
  "tool": "click",
  "arguments": {"button": "left"}
}
// Response: {"success": true, "button": "left", "x": 960, "y": 540}

// Type text
{
  "tool": "type_text",
  "arguments": {"text": "Hello, World!"}
}
// Response: {"success": true, "characters_typed": 13}

// Press Enter key
{
  "tool": "press_key",
  "arguments": {"key": "enter"}
}
// Response: {"success": true, "key": "enter"}

// Take screenshot
{
  "tool": "take_screenshot"
}
// Response: {"success": true, "filepath": "...", "base64": "...", "width": 1920, "height": 1080}
```

## Safety Features

### 🛡️ Failsafe Mechanism

Ghost MCP includes an emergency shutdown feature:

- **Trigger**: Move your mouse to the absolute top-left corner of the screen (coordinates 0,0)
- **Effect**: The server will log an error and initiate graceful shutdown
- **Purpose**: Prevents runaway AI automation loops from causing damage

```
⚠️ WARNING: Do not move your mouse to (0,0) during normal operation!
```

### Logging Safety

All application logs are written to **stderr**, never to stdout. This ensures:
- MCP JSON-RPC protocol on stdout remains clean
- No corruption of messages between client and server
- Debug output doesn't interfere with tool responses

## Troubleshooting

### Common Issues

#### "Permission Denied" or "Access Denied"
- **macOS**: Grant accessibility permissions in System Settings
- **Linux**: Ensure you have X11 permissions (`xhost +` for testing)
- **Windows**: Run as Administrator if needed

#### "RobotGo initialization failed"
- Verify platform-specific dependencies are installed (see Prerequisites)
- Check that no other application is blocking accessibility APIs

#### MCP Client Can't Connect
- Verify the binary path in mcp.json is correct
- Ensure the binary is executable (`chmod +x ghost-mcp` on Unix)
- Check that no firewall is blocking stdio communication

#### Screenshots Fail
- Ensure sufficient disk space in temp directory
- Verify write permissions to system temp folder

### Debug Mode

Enable verbose logging for troubleshooting:

```json
{
  "mcpServers": {
    "ghost-mcp": {
      "command": "/path/to/ghost-mcp",
      "env": {
        "GHOST_MCP_DEBUG": "1"
      }
    }
  }
}
```

## Development

### Running Tests

Ghost MCP includes comprehensive tests including a test fixture GUI for integration testing.

```bash
# Windows
test_runner.bat              # Unit tests only
test_runner.bat integration  # Integration tests (requires GCC)
test_runner.bat all          # All tests
test_runner.bat fixture      # Start fixture server

# macOS/Linux
chmod +x test_runner.sh
./test_runner.sh
./test_runner.sh integration
./test_runner.sh all
./test_runner.sh fixture

# Direct go test
go test -v -short ./...                 # Unit tests
set INTEGRATION=1 && go test -v ./...   # Integration tests (Windows)
export INTEGRATION=1 && go test -v ./... # Integration tests (Unix)
```

For detailed testing documentation, see [TESTING.md](./TESTING.md).

### Building for Release

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o ghost-mcp.exe -ldflags="-s -w" .

# macOS
GOOS=darwin GOARCH=amd64 go build -o ghost-mcp -ldflags="-s -w" .

# Linux
GOOS=linux GOARCH=amd64 go build -o ghost-mcp -ldflags="-s -w" .
```

## Architecture

See [ARCHITECTURE.md](./ARCHITECTURE.md) for detailed information about:
- Server structure and components
- MCP protocol implementation
- Tool handling flow
- Failsafe mechanism design

## Documentation

| Document | Description |
|----------|-------------|
| [README.md](./README.md) | Getting started and usage |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System design and internals |
| [TESTING.md](./TESTING.md) | Testing guide and fixture docs |

## License

See LICENSE file for details.

## Contributing

Contributions welcome! Please ensure:
1. All tests pass (`go test ./...`)
2. Code follows Go formatting (`go fmt ./...`)
3. No linting errors (`go vet ./...`)
4. Documentation is updated for new features

## Acknowledgments

- [MCP SDK](https://github.com/mark3labs/mcp-go) - Model Context Protocol implementation
- [RobotGo](https://github.com/go-vgo/robotgo) - Cross-platform automation library
