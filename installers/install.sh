#!/bin/bash
#
# Ghost MCP - Linux Installation Script
#
# This script installs dependencies, builds ghost-mcp, generates an auth token,
# sets environment variables, and displays the MCP client configuration.
#
# Usage: ./install.sh
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# =============================================================================
# Helper Functions
# =============================================================================

print_header() {
    echo ""
    echo -e "${CYAN}$(printf '─%.0s' {1..60})${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}$(printf '─%.0s' {1..60})${NC}"
}

print_banner() {
    clear
    echo ""
    echo -e "${MAGENTA}  ██████╗ ██╗  ██╗ ██████╗ ███████╗████████╗    ███╗   ███╗ ██████╗██████╗ ${NC}"
    echo -e "${MAGENTA}  ██╔════╝ ██║  ██║██╔═══██╗██╔════╝╚══██╔══╝    ████╗ ████║██╔════╝██╔══██╗${NC}"
    echo -e "${MAGENTA}  ██║  ███╗███████║██║   ██║███████╗   ██║       ██╔████╔██║██║     ██████╔╝${NC}"
    echo -e "${MAGENTA}  ██║   ██║██╔══██║██║   ██║╚════██║   ██║       ██║╚██╔╝██║██║     ██╔═══╝ ${NC}"
    echo -e "${MAGENTA}  ╚██████╔╝██║  ██║╚██████╔╝███████║   ██║       ██║ ╚═╝ ██║╚██████╗██║     ${NC}"
    echo -e "${MAGENTA}   ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚══════╝   ╚═╝       ╚═╝     ╚═╝ ╚═════╝╚═╝     ${NC}"
    echo ""
    echo -e "${GRAY}  OS-level UI automation for AI agents - Linux Installer${NC}"
    echo ""
}

prompt_choice() {
    local question="$1"
    shift
    local options=("$@")
    
    echo ""
    echo -e "${YELLOW}$question${NC}"
    for i in "${!options[@]}"; do
        echo -e "  [$((i+1))] ${options[$i]}"
    done
    
    while true; do
        read -p "Enter choice (1-${#options[@]}): " choice
        if [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le "${#options[@]}" ]; then
            echo $((choice - 1))
            return
        fi
    done
}

generate_token() {
    openssl rand -hex 32
}

# =============================================================================
# Dependency Installation
# =============================================================================

install_dependencies() {
    local need_minGW=false
    local need_tesseract=false
    local want_ocr=false
    
    print_header "Step 0 - Check Dependencies"
    
    # Check for GCC
    echo -e "${GRAY}  Checking for GCC compiler...${NC}"
    if command -v gcc &> /dev/null; then
        echo -e "${GREEN}  GCC found: $(gcc --version | head -n 1)${NC}"
    else
        echo -e "${YELLOW}  GCC not found. RobotGo requires a C compiler.${NC}"
        need_minGW=true
    fi
    
    # Ask about OCR support
    echo ""
    echo -e "${CYAN}  OCR Support (read_screen_text tool):${NC}"
    echo -e "${GRAY}  - Reads text directly from screen (faster than screenshots)${NC}"
    echo -e "${GRAY}  - Requires Tesseract OCR development libraries (~500MB)${NC}"
    echo -e "${GRAY}  - Installation takes 10-15 minutes${NC}"
    echo ""
    read -p "  Enable OCR support? [y/N] " ocr_response
    
    if [[ "$ocr_response" =~ ^[Yy]$ ]]; then
        want_ocr=true
        # Check for Tesseract dev libraries
        if pkg-config --exists tesseract 2>/dev/null; then
            echo -e "${GREEN}  Tesseract development libraries found.${NC}"
        else
            echo -e "${YELLOW}  Tesseract development libraries not found.${NC}"
            need_tesseract=true
        fi
    fi
    
    if [ "$need_minGW" = true ] || [ "$need_tesseract" = true ]; then
        echo ""
        if [ "$need_minGW" = true ]; then
            echo -e "${YELLOW}  Missing: GCC compiler${NC}"
        fi
        if [ "$need_tesseract" = true ]; then
            echo -e "${YELLOW}  Missing: Tesseract OCR development libraries${NC}"
        fi
        echo ""
        
        choice=$(prompt_choice "How do you want to install missing dependencies?" \
            "Install automatically (recommended)" \
            "Skip - I will install dependencies manually later")
        
        if [ "$choice" -eq 0 ]; then
            # Detect package manager
            if command -v apt-get &> /dev/null; then
                PKG_MANAGER="apt"
            elif command -v dnf &> /dev/null; then
                PKG_MANAGER="dnf"
            elif command -v yum &> /dev/null; then
                PKG_MANAGER="yum"
            elif command -v pacman &> /dev/null; then
                PKG_MANAGER="pacman"
            elif command -v zypper &> /dev/null; then
                PKG_MANAGER="zypper"
            else
                echo -e "${RED}  Could not detect package manager.${NC}"
                echo -e "${GRAY}  Please install dependencies manually.${NC}"
                exit 1
            fi
            
            echo -e "${GRAY}  Detected package manager: $PKG_MANAGER${NC}"
            
            if [ "$need_minGW" = true ]; then
                echo ""
                echo -e "${GRAY}  Installing GCC...${NC}"
                case $PKG_MANAGER in
                    apt)
                        sudo apt-get update
                        sudo apt-get install -y build-essential
                        ;;
                    dnf|yum)
                        sudo $PKG_MANAGER install -y gcc gcc-c++ make
                        ;;
                    pacman)
                        sudo pacman -S --noconfirm base-devel
                        ;;
                    zypper)
                        sudo zypper install -y gcc gcc-c++ make
                        ;;
                esac
            fi
            
            if [ "$need_tesseract" = true ] && [ "$want_ocr" = true ]; then
                echo ""
                echo -e "${GRAY}  Installing Tesseract OCR development libraries...${NC}"
                case $PKG_MANAGER in
                    apt)
                        sudo apt-get install -y tesseract-ocr libleptonica-dev libtesseract-dev
                        ;;
                    dnf|yum)
                        sudo $PKG_MANAGER install -y tesseract tesseract-devel leptonica-devel
                        ;;
                    pacman)
                        sudo pacman -S --noconfirm tesseract tesseract-data-eng
                        ;;
                    zypper)
                        sudo zypper install -y tesseract-ocr tesseract-ocr-devel
                        ;;
                esac
            fi
            
            # Verify GCC
            if [ "$need_minGW" = true ]; then
                if command -v gcc &> /dev/null; then
                    echo -e "${GREEN}  GCC found: $(gcc --version | head -n 1)${NC}"
                else
                    echo -e "${YELLOW}  Warning: GCC still not found. You may need to restart your shell.${NC}"
                    choice=$(prompt_choice "Continue anyway?" \
                        "Yes - try to build" \
                        "No - exit and restart shell")
                    if [ "$choice" -eq 1 ]; then
                        echo -e "${GRAY}  Exiting. Please re-run install.sh after restarting your shell.${NC}"
                        exit 0
                    fi
                fi
            fi
        else
            echo -e "${YELLOW}  Skipping dependency installation.${NC}"
            echo -e "${GRAY}  The build step will fail. Install missing dependencies manually.${NC}"
        fi
    else
        echo -e "${GREEN}  All dependencies found!${NC}"
    fi
    
    export WANT_OCR="$want_ocr"
}

# =============================================================================
# Build Binary
# =============================================================================

build_binary() {
    print_header "Step 1 - Binary"
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    BINARY_PATH="$SCRIPT_DIR/ghost-mcp"
    
    if [ -f "$BINARY_PATH" ]; then
        echo -e "${GREEN}  Found existing binary: $BINARY_PATH${NC}"
        choice=$(prompt_choice "Rebuild from source?" \
            "No - keep existing binary" \
            "Yes - rebuild now")
        if [ "$choice" -eq 0 ]; then
            return
        fi
    else
        echo -e "${YELLOW}  Binary not found. Will build from source.${NC}"
    fi
    
    echo ""
    
    # Tidy modules first
    echo -e "${GRAY}  Running: go mod tidy${NC}"
    go mod tidy
    
    # Note: robotgo v1.0.0+ has GetText built-in by default, no build tags needed
    echo -e "${GRAY}  Running: go build -o ghost-mcp ./cmd/ghost-mcp/${NC}"
    echo -e "${GRAY}  Building (OCR support included by default in robotgo v1.0.0+)...${NC}"
    
    cd "$SCRIPT_DIR"
    
    if go build -o ghost-mcp -ldflags="-s -w" ./cmd/ghost-mcp/; then
        echo -e "${GREEN}  Build succeeded: $BINARY_PATH${NC}"
        echo -e "${GREEN}  OCR support enabled (read_screen_text tool available)${NC}"
    else
        echo ""
        echo -e "${RED}  Build failed!${NC}"
        echo ""
        echo -e "${YELLOW}  This is likely due to missing Tesseract OCR development headers.${NC}"
        echo -e "${GRAY}  The read_screen_text tool requires Tesseract + Leptonica headers.${NC}"
        echo ""
        
        choice=$(prompt_choice "How do you want to proceed?" \
            "Build WITHOUT OCR support (read_screen_text tool will be disabled)" \
            "Build WITH OCR support (requires Tesseract dev libraries)" \
            "Exit and fix dependencies manually")
        
        if [ "$choice" -eq 0 ]; then
            # Build without OCR
            echo -e "${GRAY}  Building without OCR support...${NC}"
            if go build -o ghost-mcp -ldflags="-s -w" -tags noocr ./cmd/ghost-mcp/; then
                echo -e "${GREEN}  Build succeeded (OCR disabled): $BINARY_PATH${NC}"
            else
                echo -e "${RED}  Build failed again.${NC}"
                echo ""
                echo -e "${YELLOW}  Options:${NC}"
                echo -e "${GRAY}   1. Install dependencies and re-run this script${NC}"
                echo -e "${GRAY}   2. Build manually: go build -tags noocr -o ghost-mcp ./cmd/ghost-mcp/${NC}"
                echo ""
                exit 1
            fi
        elif [ "$choice" -eq 1 ]; then
            # Build with OCR
            echo -e "${GRAY}  Building with OCR support...${NC}"
            
            # Tidy modules first
            echo -e "${GRAY}  Running: go mod tidy${NC}"
            go mod tidy
            
            if go build -o ghost-mcp -ldflags="-s -w" ./cmd/ghost-mcp/; then
                echo -e "${GREEN}  Build succeeded (OCR enabled): $BINARY_PATH${NC}"
                echo -e "${GREEN}  read_screen_text tool is now available!${NC}"
            else
                echo -e "${RED}  Build with OCR failed.${NC}"
                echo ""
                choice=$(prompt_choice "Try building without OCR instead?" \
                    "Yes - build without OCR" \
                    "No - exit")
                if [ "$choice" -eq 0 ]; then
                    if go build -o ghost-mcp -ldflags="-s -w" ./cmd/ghost-mcp/; then
                        echo -e "${GREEN}  Build succeeded (OCR disabled): $BINARY_PATH${NC}"
                    else
                        echo -e "${RED}  Build failed.${NC}"
                        exit 1
                    fi
                else
                    exit 1
                fi
            fi
        else
            echo -e "${GRAY}  Exiting.${NC}"
            exit 1
        fi
    fi
}

# =============================================================================
# Transport Mode
# =============================================================================

configure_transport() {
    print_header "Step 2 - Transport Mode"
    
    echo -e "${GRAY}  stdio   - Claude Desktop launches ghost-mcp as a subprocess (recommended)${NC}"
    echo -e "${GRAY}  HTTP/SSE - ghost-mcp runs as a persistent HTTP server (advanced)${NC}"
    
    choice=$(prompt_choice "Which transport mode?" \
        "stdio (recommended)" \
        "HTTP / SSE")
    
    if [ "$choice" -eq 0 ]; then
        TRANSPORT="stdio"
    else
        TRANSPORT="http"
        
        echo ""
        read -p "  HTTP bind address [default: localhost:8080]: " addr_input
        HTTP_ADDR="${addr_input:-localhost:8080}"
        
        read -p "  Public base URL [default: http://$HTTP_ADDR]: " base_input
        HTTP_BASE_URL="${base_input:-http://$HTTP_ADDR}"
    fi
}

# =============================================================================
# Auth Token
# =============================================================================

configure_auth() {
    print_header "Step 3 - Auth Token"
    
    EXISTING_TOKEN="${GHOST_MCP_TOKEN:-}"
    
    if [ -n "$EXISTING_TOKEN" ]; then
        echo -e "${YELLOW}  Existing token found in environment.${NC}"
        choice=$(prompt_choice "What do you want to do?" \
            "Keep existing token" \
            "Generate a new token (old token will stop working)")
        if [ "$choice" -eq 0 ]; then
            TOKEN="$EXISTING_TOKEN"
        else
            TOKEN=$(generate_token)
            echo -e "${GREEN}  New token generated.${NC}"
        fi
    else
        TOKEN=$(generate_token)
        echo -e "${GREEN}  Token generated.${NC}"
    fi
}

# =============================================================================
# Optional Settings
# =============================================================================

configure_settings() {
    print_header "Step 4 - Optional Settings"
    
    read -p "  Enable debug logging? (y/N) " debug_input
    if [[ "$debug_input" =~ ^[Yy]$ ]]; then
        DEBUG="1"
    else
        DEBUG="0"
    fi
    
    AUDIT_LOG="$HOME/.config/ghost-mcp/audit"
    read -p "  Audit log directory [default: $AUDIT_LOG]: " audit_input
    AUDIT_LOG="${audit_input:-$AUDIT_LOG}"
    
    SCREENSHOT_DIR="/tmp"
    read -p "  Screenshot directory [default: /tmp]: " screenshot_input
    SCREENSHOT_DIR="${screenshot_input:-/tmp}"
    
    read -p "  Show visual feedback for actions? (y/N) - draws cursor effects " visual_input
    if [[ "$visual_input" =~ ^[Yy]$ ]]; then
        VISUAL="1"
    else
        VISUAL="0"
    fi
}

# =============================================================================
# Set Environment Variables
# =============================================================================

set_environment() {
    print_header "Step 5 - Setting Environment Variables"
    
    # Create config directory
    CONFIG_DIR="$HOME/.config/ghost-mcp"
    mkdir -p "$CONFIG_DIR"
    
    # Write environment file
    ENV_FILE="$CONFIG_DIR/env"
    cat > "$ENV_FILE" << EOF
# Ghost MCP Environment Variables
# Source this file or add to your shell profile

export GHOST_MCP_TOKEN="$TOKEN"
export GHOST_MCP_TRANSPORT="$TRANSPORT"
export GHOST_MCP_DEBUG="$DEBUG"
export GHOST_MCP_AUDIT_LOG="$AUDIT_LOG"
export GHOST_MCP_VISUAL="$VISUAL"
export GHOST_MCP_SCREENSHOT_DIR="$SCREENSHOT_DIR"
EOF

    if [ "$TRANSPORT" = "http" ]; then
        echo "export GHOST_MCP_HTTP_ADDR=\"$HTTP_ADDR\"" >> "$ENV_FILE"
        echo "export GHOST_MCP_HTTP_BASE_URL=\"$HTTP_BASE_URL\"" >> "$ENV_FILE"
    fi
    
    echo -e "${GREEN}  Environment variables saved to: $ENV_FILE${NC}"
    echo ""
    echo -e "${GRAY}  To load these in your current shell:${NC}"
    echo -e "${CYAN}    source $ENV_FILE${NC}"
    echo ""
    echo -e "${GRAY}  To load automatically, add to your ~/.bashrc or ~/.zshrc:${NC}"
    echo -e "${CYAN}    source $ENV_FILE${NC}"
    echo ""
    
    # Also export for current session
    export GHOST_MCP_TOKEN="$TOKEN"
    export GHOST_MCP_TRANSPORT="$TRANSPORT"
    export GHOST_MCP_DEBUG="$DEBUG"
    export GHOST_MCP_AUDIT_LOG="$AUDIT_LOG"
    export GHOST_MCP_VISUAL="$VISUAL"
    export GHOST_MCP_SCREENSHOT_DIR="$SCREENSHOT_DIR"
    
    if [ "$TRANSPORT" = "http" ]; then
        export GHOST_MCP_HTTP_ADDR="$HTTP_ADDR"
        export GHOST_MCP_HTTP_BASE_URL="$HTTP_BASE_URL"
    fi
}

# =============================================================================
# Generate settings.json
# =============================================================================

generate_settings() {
    print_header "Step 6 - MCP Client Configuration"
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    BINARY_PATH="$SCRIPT_DIR/ghost-mcp"
    
    echo ""
    echo -e "${YELLOW}  Config file location (manual):${NC}"
    echo -e "${WHITE}    ~/.config/Claude/mcp.json${NC}"
    echo ""
    echo -e "${CYAN}  Copy the settings.json below:${NC}"
    echo ""
    
    # Build JSON
    if [ "$TRANSPORT" = "stdio" ]; then
        cat << EOF
{
  "mcpServers": {
    "ghost-mcp": {
      "command": "$BINARY_PATH",
      "args": [],
      "env": {
        "GHOST_MCP_TOKEN": "$TOKEN",
        "GHOST_MCP_TRANSPORT": "$TRANSPORT",
        "GHOST_MCP_DEBUG": "$DEBUG",
        "GHOST_MCP_AUDIT_LOG": "$AUDIT_LOG",
        "GHOST_MCP_VISUAL": "$VISUAL",
        "GHOST_MCP_SCREENSHOT_DIR": "$SCREENSHOT_DIR"
      }
    }
  }
}
EOF
    else
        cat << EOF
{
  "mcpServers": {
    "ghost-mcp": {
      "url": "$HTTP_BASE_URL/sse",
      "headers": {
        "Authorization": "Bearer $TOKEN"
      }
    }
  }
}
EOF
    fi
    
    echo ""
}

# =============================================================================
# Start Service (HTTP mode only)
# =============================================================================

start_service() {
    print_header "Step 7 - Start the Service"
    
    if [ "$TRANSPORT" = "stdio" ]; then
        echo -e "${GRAY}  In stdio mode, Claude Desktop launches ghost-mcp automatically.${NC}"
        echo -e "${GRAY}  No separate service needs to be started.${NC}"
        START_SERVICE=0
    else
        choice=$(prompt_choice "Start ghost-mcp HTTP/SSE server now?" \
            "Yes - start the server in the background" \
            "No  - I will start it manually later")
        START_SERVICE=$choice
    fi
    
    if [ "$START_SERVICE" -eq 0 ] && [ "$TRANSPORT" = "http" ]; then
        echo ""
        echo -e "${YELLOW}  Starting ghost-mcp HTTP server...${NC}"
        echo -e "${GRAY}  Listen address: $HTTP_ADDR${NC}"
        echo -e "${GRAY}  SSE endpoint:   $HTTP_BASE_URL/sse${NC}"
        echo ""
        
        # Create log directory
        LOG_DIR="$HOME/.local/share/ghost-mcp/logs"
        mkdir -p "$LOG_DIR"
        LOG_FILE="$LOG_DIR/ghost-mcp-$(date +%Y%m%d-%H%M%S).log"
        
        # Start in background
        nohup ./ghost-mcp > "$LOG_FILE" 2>&1 &
        SERVER_PID=$!
        
        echo -e "${GREEN}  Server started successfully!${NC}"
        echo -e "${GRAY}  PID: $SERVER_PID${NC}"
        echo -e "${GRAY}  Log file: $LOG_FILE${NC}"
        echo ""
        echo -e "${CYAN}  To stop the server:${NC}"
        echo -e "${WHITE}    kill $SERVER_PID${NC}"
        echo ""
        echo -e "${CYAN}  To view logs:${NC}"
        echo -e "${WHITE}    tail -f $LOG_FILE${NC}"
    fi
}

# =============================================================================
# Final Summary
# =============================================================================

print_summary() {
    print_header "Done"
    
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    
    echo ""
    echo -e "  Binary   : ${WHITE}$SCRIPT_DIR/ghost-mcp${NC}"
    echo -e "  Transport: ${WHITE}$TRANSPORT${NC}"
    echo ""
    echo -e "${RED}  ┌─ YOUR SECRET TOKEN (keep this safe!) ──────────────────────${NC}"
    echo -e "${RED}  │${NC}"
    echo -e "${RED}  │   ${YELLOW}$TOKEN${NC}"
    echo -e "${RED}  │${NC}"
    echo -e "${RED}  └─────────────────────────────────────────────────────────────${NC}"
    echo ""
    
    if [ "$TRANSPORT" = "stdio" ]; then
        echo -e "${CYAN}  Next steps:${NC}"
        echo -e "${WHITE}   1. Copy the settings.json above to ~/.config/Claude/mcp.json${NC}"
        echo -e "${WHITE}   2. Restart Claude Desktop so it picks up the new MCP config.${NC}"
        echo -e "${WHITE}   3. The ghost-mcp tools will appear in the Claude tool list.${NC}"
        echo -e "${WHITE}   4. Move the mouse to (0, 0) at any time to trigger the failsafe.${NC}"
    elif [ "$START_SERVICE" -eq 0 ]; then
        echo -e "${CYAN}  Next steps:${NC}"
        echo -e "${GREEN}   [OK] Server is running on $HTTP_ADDR${NC}"
        echo -e "${WHITE}   1. Copy the settings.json above to ~/.config/Claude/mcp.json${NC}"
        echo -e "${WHITE}   2. Restart Claude Desktop so it picks up the new MCP config.${NC}"
        echo -e "${WHITE}   3. The ghost-mcp tools will appear in the Claude tool list.${NC}"
        echo -e "${WHITE}   4. Move the mouse to (0, 0) at any time to trigger the failsafe.${NC}"
    else
        echo -e "${CYAN}  Next steps:${NC}"
        echo -e "${WHITE}   1. Start the server:  ./ghost-mcp${NC}"
        echo -e "${WHITE}   2. It will listen on $HTTP_ADDR${NC}"
        echo -e "${WHITE}   3. Copy the settings.json above to ~/.config/Claude/mcp.json${NC}"
        echo -e "${WHITE}   4. Restart Claude Desktop so it picks up the new MCP config.${NC}"
        echo -e "${WHITE}   5. The ghost-mcp tools will appear in the Claude tool list.${NC}"
        echo -e "${WHITE}   6. Move the mouse to (0, 0) at any time to trigger the failsafe.${NC}"
    fi
    
    echo ""
}

# =============================================================================
# Main
# =============================================================================

print_banner
install_dependencies
build_binary
configure_transport
configure_auth
configure_settings
set_environment
generate_settings
start_service
print_summary
