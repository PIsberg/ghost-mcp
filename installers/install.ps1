#Requires -Version 5.1
<#
.SYNOPSIS
    Ghost MCP - Windows installation script.

.DESCRIPTION
    Builds ghost-mcp.exe, generates a random auth token, sets persistent user
    environment variables, and prints the Claude Desktop settings.json snippet.

.EXAMPLE
    # Run from the repo root in an elevated (or normal user) PowerShell:
    .\install.ps1
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# --- check admin rights ------------------------------------------------------

function Test-Administrator {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Request-Elevation {
    Write-Host ""
    Write-Host "  This installer needs administrator privileges to:" -ForegroundColor Yellow
    Write-Host "   - Install MinGW (C compiler for RobotGo)" -ForegroundColor Gray
    Write-Host "   - Write to Program Files" -ForegroundColor Gray
    Write-Host ""
    
    $response = Read-Host "  Restart as Administrator? [Y/n]"
    if ($response -match '^[Nn]$') {
        Write-Host "  Exiting. Please re-run this script as Administrator." -ForegroundColor Red
        exit 1
    }
    
    $arguments = "-ExecutionPolicy Bypass -File `"$PSCommandPath`""
    Start-Process powershell -Verb RunAs -ArgumentList $arguments -Wait
    exit 0
}

if (-not (Test-Administrator)) {
    Write-Host ""
    Write-Host "  WARNING: Not running as Administrator!" -ForegroundColor Red
    Request-Elevation
}

# --- helpers -----------------------------------------------------------------

function Write-Header {
    param([string]$Text)
    $line = '-' * 60
    Write-Host ""
    Write-Host $line -ForegroundColor Cyan
    Write-Host "  $Text" -ForegroundColor Cyan
    Write-Host $line -ForegroundColor Cyan
}

function Prompt-Choice {
    param([string]$Question, [string[]]$Options)
    Write-Host ""
    Write-Host $Question -ForegroundColor Yellow
    for ($i = 0; $i -lt $Options.Count; $i++) {
        Write-Host "  [$($i+1)] $($Options[$i])"
    }
    do {
        $raw = Read-Host "Enter choice (1-$($Options.Count))"
        $n   = $raw -as [int]
    } while ($n -lt 1 -or $n -gt $Options.Count)
    return $n - 1
}

function Generate-Token {
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    return ([System.BitConverter]::ToString($bytes) -replace '-','').ToLower()
}

function Set-UserEnv {
    param([string]$Name, [string]$Value)
    [System.Environment]::SetEnvironmentVariable($Name, $Value, 'User')
    # also set in current process so subsequent reads are fresh
    [System.Environment]::SetEnvironmentVariable($Name, $Value, 'Process')
}

# --- check dependencies ------------------------------------------------------

function Test-GccInstalled {
    try {
        $null = Get-Command gcc -ErrorAction Stop
        return $true
    } catch {
        return $false
    }
}

function Install-Chocolatey {
    Write-Host ""
    Write-Host "  Installing Chocolatey package manager..." -ForegroundColor Yellow
    
    # Download and run Chocolatey installer
    $chocoInstallScript = "$env:TEMP\choco_install.ps1"
    Invoke-WebRequest -Uri "https://chocolatey.org/install.ps1" -OutFile $chocoInstallScript
    
    & powershell -ExecutionPolicy Bypass -File $chocoInstallScript
    
    # Refresh environment
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
    
    Remove-Item $chocoInstallScript -Force
    Write-Host "  Chocolatey installed successfully." -ForegroundColor Green
}

function Install-MinGW {
    Write-Host ""
    Write-Host "  Installing MinGW-w64 via Chocolatey..." -ForegroundColor Yellow
    Write-Host "  This may take a few minutes..." -ForegroundColor Gray
    
    & choco install mingw -y --no-progress
    
    # Refresh PATH
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
    
    Write-Host "  MinGW installed successfully." -ForegroundColor Green
}

function Install-Tesseract {
    Write-Host ""
    Write-Host "  Installing Tesseract OCR development libraries..." -ForegroundColor Yellow
    Write-Host "  This may take 10-15 minutes..." -ForegroundColor Gray

    # Check if vcpkg is installed
    $vcpkgPath = "$env:USERPROFILE\vcpkg"
    if (-not (Test-Path $vcpkgPath)) {
        Write-Host "  Installing vcpkg package manager..." -ForegroundColor Gray
        git clone https://github.com/Microsoft/vcpkg.git $vcpkgPath
        Push-Location $vcpkgPath
        & .\bootstrap-vcpkg.bat -disableMetrics
        Pop-Location
        Write-Host "  vcpkg installed." -ForegroundColor Green
    }

    # Install Tesseract and Leptonica via vcpkg using the MinGW triplet
    Write-Host "  Installing Tesseract OCR and Leptonica (this takes a while)..." -ForegroundColor Gray
    Push-Location $vcpkgPath
    & .\vcpkg install tesseract:x64-mingw-dynamic leptonica:x64-mingw-dynamic --disable-metrics
    Pop-Location

    # gosseract links against -ltesseract but vcpkg installs libtesseract55.dll.a
    $libDir = "$vcpkgPath\installed\x64-mingw-dynamic\lib"
    $tessLib = Get-Item "$libDir\libtesseract*.dll.a" -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($tessLib -and -not (Test-Path "$libDir\libtesseract.dll.a")) {
        Copy-Item $tessLib.FullName "$libDir\libtesseract.dll.a"
        Write-Host "  Created libtesseract.dll.a alias." -ForegroundColor Gray
    }

    # Download English language data — Tesseract cannot run without it
    $tessdataDir = "$vcpkgPath\installed\x64-mingw-dynamic\share\tessdata"
    $engData = "$tessdataDir\eng.traineddata"
    if (-not (Test-Path $engData)) {
        Write-Host "  Downloading Tesseract English language data..." -ForegroundColor Gray
        Invoke-WebRequest "https://github.com/tesseract-ocr/tessdata_fast/raw/main/eng.traineddata" -OutFile $engData
        Write-Host "  Language data downloaded." -ForegroundColor Green
    } else {
        Write-Host "  Tesseract language data already present." -ForegroundColor Green
    }

    # Use forward-slash paths — CGO_CPPFLAGS applies to both C and C++ (needed for tessbridge.cpp)
    $vcpkgInstall = ($vcpkgPath + "/installed/x64-mingw-dynamic").Replace('\', '/')
    [System.Environment]::SetEnvironmentVariable("CGO_CPPFLAGS", "-I$vcpkgInstall/include", "User")
    [System.Environment]::SetEnvironmentVariable("CGO_LDFLAGS", "-L$vcpkgInstall/lib", "User")
    [System.Environment]::SetEnvironmentVariable("CGO_ENABLED", "1", "User")

    # TESSDATA_PREFIX must point to the directory that directly contains eng.traineddata
    $tessPrefix = "$vcpkgPath\installed\x64-mingw-dynamic\share\tessdata"
    [System.Environment]::SetEnvironmentVariable("TESSDATA_PREFIX", $tessPrefix, "User")

    # Add vcpkg bin to PATH so the runtime DLLs are found when running ghost-mcp.exe
    $vcpkgBin = "$vcpkgPath\installed\x64-mingw-dynamic\bin"
    $userPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
    if ($userPath -notlike "*$vcpkgBin*") {
        [System.Environment]::SetEnvironmentVariable("PATH", "$userPath;$vcpkgBin", "User")
    }

    # Set for current session
    $env:CGO_CPPFLAGS    = "-I$vcpkgInstall/include"
    $env:CGO_LDFLAGS     = "-L$vcpkgInstall/lib"
    $env:CGO_ENABLED     = "1"
    $env:TESSDATA_PREFIX = $tessPrefix
    $env:PATH            = "$env:PATH;$vcpkgBin"

    Write-Host "  Tesseract development libraries installed." -ForegroundColor Green
    Write-Host "  CGO flags configured for vcpkg." -ForegroundColor Green
}

# --- banner ------------------------------------------------------------------

Clear-Host
Write-Host ""
Write-Host "  ██████╗ ██╗  ██╗ ██████╗ ███████╗████████╗    ███╗   ███╗ ██████╗██████╗ " -ForegroundColor Magenta
Write-Host "  ██╔════╝ ██║  ██║██╔═══██╗██╔════╝╚══██╔══╝    ████╗ ████║██╔════╝██╔══██╗" -ForegroundColor Magenta
Write-Host "  ██║  ███╗███████║██║   ██║███████╗   ██║       ██╔████╔██║██║     ██████╔╝" -ForegroundColor Magenta
Write-Host "  ██║   ██║██╔══██║██║   ██║╚════██║   ██║       ██║╚██╔╝██║██║     ██╔═══╝ " -ForegroundColor Magenta
Write-Host "  ╚██████╔╝██║  ██║╚██████╔╝███████║   ██║       ██║ ╚═╝ ██║╚██████╗██║     " -ForegroundColor Magenta
Write-Host "   ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚══════╝   ╚═╝       ╚═╝     ╚═╝ ╚═════╝╚═╝     " -ForegroundColor Magenta
Write-Host ""
Write-Host "  OS-level UI automation for AI agents - Windows Installer" -ForegroundColor Gray
Write-Host ""

# --- step 0: check/install dependencies --------------------------------------

Write-Header "Step 0 - Check Dependencies"

Write-Host "  Checking for MinGW (GCC) compiler..." -ForegroundColor Gray

$needTesseract = $false
$needMinGW = $false

if (Test-GccInstalled) {
    Write-Host "  GCC found: $(gcc --version | Select-Object -First 1)" -ForegroundColor Green
} else {
    Write-Host "  GCC not found. RobotGo requires a C compiler." -ForegroundColor Yellow
    $needMinGW = $true
}

# Check if Tesseract is installed via vcpkg (x64-mingw-dynamic triplet)
$vcpkgTessInclude = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\include\tesseract"
if (Test-Path $vcpkgTessInclude) {
    Write-Host "  Tesseract OCR found in vcpkg." -ForegroundColor Green
} else {
    Write-Host "  Tesseract OCR not found. Required for read_screen_text tool." -ForegroundColor Yellow
    $needTesseract = $true
}

if ($needMinGW -or $needTesseract) {
    Write-Host ""
    if ($needMinGW)     { Write-Host "  Missing: MinGW-w64 (GCC compiler)" -ForegroundColor Yellow }
    if ($needTesseract) { Write-Host "  Missing: Tesseract OCR (x64-mingw-dynamic via vcpkg)" -ForegroundColor Yellow }
    Write-Host ""

    $chocoIdx = Prompt-Choice "How do you want to install missing dependencies?" @(
        "Install automatically (recommended)",
        "Skip - I will install dependencies manually later"
    )

    if ($chocoIdx -eq 0) {
        # Check if Chocolatey is installed (for MinGW)
        try {
            $null = Get-Command choco -ErrorAction Stop
            $chocoInstalled = $true
        } catch {
            $chocoInstalled = $false
        }

        if (-not $chocoInstalled) {
            Install-Chocolatey
        }

        if ($needMinGW) {
            Install-MinGW
        }

        # Verify GCC installation
        if ($needMinGW) {
            if (Test-GccInstalled) {
                Write-Host "  GCC found: $(gcc --version | Select-Object -First 1)" -ForegroundColor Green
            } else {
                Write-Host "  Warning: GCC still not found. You may need to restart PowerShell." -ForegroundColor Yellow
                $continueIdx = Prompt-Choice "Continue anyway?" @("Yes - try to build", "No - exit and restart PowerShell")
                if ($continueIdx -eq 1) {
                    Write-Host "  Exiting. Please re-run install.ps1 after restarting PowerShell." -ForegroundColor Gray
                    exit 0
                }
            }
        }

        if ($needTesseract) {
            Install-Tesseract
        }
    } else {
        Write-Host "  Skipping dependency installation." -ForegroundColor Yellow
        Write-Host "  The build step will fail. Install missing dependencies manually." -ForegroundColor Gray
    }
} else {
    Write-Host "  All dependencies found!" -ForegroundColor Green
}

# --- step 1: locate / build the binary ---------------------------------------

Write-Header "Step 1 - Binary"

$scriptDir = $PSScriptRoot
if (-not $scriptDir) { $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path }

# Resolve the project root (parent of installers directory)
if ($scriptDir -like "*\installers") {
    $projectRoot = Split-Path $scriptDir -Parent
} else {
    $projectRoot = $scriptDir
}

$binaryPath = Join-Path $projectRoot "ghost-mcp.exe"

if (Test-Path $binaryPath) {
    Write-Host "  Found existing binary: $binaryPath" -ForegroundColor Green
    $rebuild = Prompt-Choice "Rebuild from source?" @("No - keep existing binary", "Yes - rebuild now")
    $buildFailed = $false  # existing binary is fine
} else {
    Write-Host "  Binary not found. Will build from source." -ForegroundColor Yellow
    $rebuild = 1   # force build
    $buildFailed = $false  # will be set in build block
}

# Resolve the project root (parent of installers directory)
if ($scriptDir -like "*\installers") {
    $projectRoot = Split-Path $scriptDir -Parent
} else {
    $projectRoot = $scriptDir
}

if ($rebuild -eq 1) {
    Write-Host ""
    Write-Host "  Running: go mod tidy" -ForegroundColor Gray
    Push-Location $projectRoot
    & go mod tidy
    Pop-Location
    
    Write-Host ""
    
    # Set CGO flags for vcpkg (x64-mingw-dynamic) — use forward slashes; CGO_CPPFLAGS covers C++ includes
    $vcpkgInstall = ($env:USERPROFILE + "/vcpkg/installed/x64-mingw-dynamic").Replace('\', '/')
    $env:CGO_CPPFLAGS = "-I$vcpkgInstall/include"
    $env:CGO_LDFLAGS  = "-L$vcpkgInstall/lib"

    Push-Location $projectRoot
    try {
        $buildFailed = $false
        & go build -o ghost-mcp.exe -ldflags="-s -w" ./cmd/ghost-mcp/
        if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }
        Write-Host "  Build succeeded: $binaryPath" -ForegroundColor Green

        # Copy runtime DLLs next to the exe so it works without vcpkg in PATH
        $vcpkgBin = "$env:USERPROFILE\vcpkg\installed\x64-mingw-dynamic\bin"
        if (Test-Path $vcpkgBin) {
            Write-Host "  Copying runtime DLLs..." -ForegroundColor Gray
            Copy-Item "$vcpkgBin\*.dll" $projectRoot -Force
            Write-Host "  DLLs copied to: $projectRoot" -ForegroundColor Green
        }
    } catch {
        Write-Host ""
        Write-Host "  Build failed!" -ForegroundColor Red
        Write-Host ""
        Write-Host "  This is likely due to missing MinGW or Tesseract OCR libraries." -ForegroundColor Yellow
        Write-Host "  Re-run the installer from Step 0 to install missing dependencies." -ForegroundColor Gray
        Write-Host ""
        $skipBuildIdx = Prompt-Choice "How do you want to proceed?" @(
            "Exit and fix dependencies manually",
            "Continue anyway (binary not available)"
        )
        if ($skipBuildIdx -eq 0) {
            Write-Host "  Exiting." -ForegroundColor Gray
            exit 1
        }
        $buildFailed = $true
    } finally {
        Pop-Location
    }
}

# Check if build failed and no binary exists
if ($buildFailed -and -not (Test-Path $binaryPath)) {
    Write-Host ""
    Write-Host "  ERROR: Build failed and no binary is available." -ForegroundColor Red
    Write-Host "  Installation cannot continue without a valid binary." -ForegroundColor Red
    Write-Host ""
    Write-Host "  Options:" -ForegroundColor Yellow
    Write-Host "   1. Install dependencies and re-run this script as Administrator" -ForegroundColor Gray
    Write-Host "   2. Build manually: go build -o ghost-mcp.exe ./cmd/ghost-mcp/" -ForegroundColor Gray
    Write-Host ""
    exit 1
}

# Skip remaining steps if no binary (shouldn't happen, but safety check)
if (-not (Test-Path $binaryPath)) {
    Write-Host ""
    Write-Host "  ERROR: No binary found at: $binaryPath" -ForegroundColor Red
    Write-Host "  Installation cannot continue." -ForegroundColor Red
    exit 1
}

# --- step 2: transport mode --------------------------------------------------

Write-Header "Step 2 - Transport Mode"

Write-Host "  stdio   - Claude Desktop launches ghost-mcp as a subprocess (recommended)" -ForegroundColor Gray
Write-Host "  HTTP/SSE - ghost-mcp runs as a persistent HTTP server (advanced)" -ForegroundColor Gray

$transportIdx = Prompt-Choice "Which transport mode?" @("stdio (recommended)", "HTTP / SSE")
$transport    = @("stdio", "http")[$transportIdx]

$httpAddr    = "localhost:8080"
$httpBaseUrl = ""

if ($transport -eq "http") {
    Write-Host ""
    $addrInput = Read-Host "  HTTP bind address [default: localhost:8080]"
    if ($addrInput.Trim() -ne "") { $httpAddr = $addrInput.Trim() }

    $baseInput = Read-Host "  Public base URL   [default: http://$httpAddr]"
    if ($baseInput.Trim() -ne "") {
        $httpBaseUrl = $baseInput.Trim()
    } else {
        $httpBaseUrl = "http://$httpAddr"
    }
}

# --- step 3: auth token ------------------------------------------------------

Write-Header "Step 3 - Auth Token"

$existingToken = [System.Environment]::GetEnvironmentVariable("GHOST_MCP_TOKEN", "User")

if ($existingToken) {
    Write-Host "  Existing token found in user environment." -ForegroundColor Yellow
    $tokenIdx = Prompt-Choice "What do you want to do?" @(
        "Keep existing token",
        "Generate a new token (old token will stop working)"
    )
    if ($tokenIdx -eq 0) {
        $token = $existingToken
    } else {
        $token = Generate-Token
        Write-Host "  New token generated." -ForegroundColor Green
    }
} else {
    $token = Generate-Token
    Write-Host "  Token generated." -ForegroundColor Green
}

# --- step 4: optional settings -----------------------------------------------

Write-Header "Step 4 - Optional Settings"

$logLevelIdx = Prompt-Choice "Select log verbosity:" @("INFO (Narrative of agent actions)", "DEBUG (Full tool payloads)")
$logLevel    = if ($logLevelIdx -eq 1) { "DEBUG" } else { "INFO" }

$auditLog   = [System.IO.Path]::Combine(
    [System.Environment]::GetFolderPath("ApplicationData"),
    "ghost-mcp", "audit"
)
$auditInput = Read-Host "  Audit log directory [default: $auditLog]"
if ($auditInput.Trim() -ne "") { $auditLog = $auditInput.Trim() }

$screenshotDir = [System.IO.Path]::Combine(
    [System.Environment]::GetFolderPath("LocalApplicationData"),
    "Temp"
)
$screenshotInput = Read-Host "  Screenshot directory [default: %TEMP%]"
if ($screenshotInput.Trim() -ne "") { $screenshotDir = $screenshotInput.Trim() }

$visualInput = Read-Host "  Show visual feedback for actions? (y/N) - draws cursor effects"
$visual      = if ($visualInput -match '^[Yy]') { "1" } else { "0" }

# --- step 5: persist environment variables -----------------------------------

Write-Header "Step 5 - Setting Environment Variables"

$envVars = [ordered]@{
    GHOST_MCP_TOKEN         = $token
    GHOST_MCP_TRANSPORT     = $transport
    GHOST_MCP_LOG_LEVEL     = $logLevel
    GHOST_MCP_AUDIT_LOG     = $auditLog
    GHOST_MCP_VISUAL        = $visual
    GHOST_MCP_SCREENSHOT_DIR = $screenshotDir
}

if ($transport -eq "http") {
    $envVars["GHOST_MCP_HTTP_ADDR"]     = $httpAddr
    $envVars["GHOST_MCP_HTTP_BASE_URL"] = $httpBaseUrl
}

foreach ($kv in $envVars.GetEnumerator()) {
    Set-UserEnv -Name $kv.Key -Value $kv.Value
    $display = if ($kv.Key -eq "GHOST_MCP_TOKEN") { "****" } else { $kv.Value }
    Write-Host ("  {0,-30} = {1}" -f $kv.Key, $display) -ForegroundColor Green
}

Write-Host ""
Write-Host "  Variables saved to the current user's persistent environment." -ForegroundColor Gray
Write-Host "  New terminals and processes will pick them up automatically." -ForegroundColor Gray

# --- step 6: build settings.json ---------------------------------------------

Write-Header "Step 6 - Claude Desktop settings.json"

# Build the env block for the JSON
$envBlock = [ordered]@{
    GHOST_MCP_TOKEN         = $token
    GHOST_MCP_TRANSPORT     = $transport
    GHOST_MCP_LOG_LEVEL     = $logLevel
    GHOST_MCP_AUDIT_LOG     = $auditLog
    GHOST_MCP_VISUAL        = $visual
    GHOST_MCP_SCREENSHOT_DIR = $screenshotDir
}
if ($transport -eq "http") {
    $envBlock["GHOST_MCP_HTTP_ADDR"]     = $httpAddr
    $envBlock["GHOST_MCP_HTTP_BASE_URL"] = $httpBaseUrl
}

# Escape backslashes for JSON
$binaryPathJson = $binaryPath.Replace('\', '\\')
$auditLogJson   = $auditLog.Replace('\', '\\')

# Build env JSON lines
$envLines = $envBlock.GetEnumerator() | ForEach-Object {
    $v = if ($_.Key -eq "GHOST_MCP_AUDIT_LOG") { $auditLogJson } else { $_.Value }
    "        `"$($_.Key)`": `"$v`""
}
$envJson = $envLines -join ",`n"

if ($transport -eq "stdio") {
    $serverBlock = @"
    "ghost-mcp": {
      "command": "$binaryPathJson",
      "args": [],
      "env": {
$envJson
      }
    }
"@
} else {
    # HTTP/SSE - no command/args; client connects to the running server
    $serverBlock = @"
    "ghost-mcp": {
      "url": "$httpBaseUrl/sse",
      "headers": {
        "Authorization": "Bearer $token"
      }
    }
"@
}

$settingsJson = @"
{
  "mcpServers": {
$serverBlock
  }
}
"@

# --- display results ---------------------------------------------------------

Write-Host ""
Write-Host "  Config file location (manual):" -ForegroundColor Yellow
Write-Host "    $env:APPDATA\Claude\mcp.json" -ForegroundColor White
Write-Host ""
Write-Host "  Copy the settings.json below:" -ForegroundColor Cyan
Write-Host ""
$settingsJson
Write-Host ""

# --- step 7: start the service -----------------------------------------------

Write-Header "Step 7 - Start the Service"

if ($transport -eq "stdio") {
    Write-Host "  In stdio mode, Claude Desktop launches ghost-mcp automatically." -ForegroundColor Gray
    Write-Host "  No separate service needs to be started." -ForegroundColor Gray
    $startService = 0
} else {
    $startServiceIdx = Prompt-Choice "Start ghost-mcp HTTP/SSE server now?" @(
        "Yes - start the server in the background",
        "No  - I will start it manually later"
    )
    $startService = $startServiceIdx
}

if ($startService -eq 0 -and $transport -eq "http") {
    Write-Host ""
    Write-Host "  Starting ghost-mcp HTTP server..." -ForegroundColor Yellow
    Write-Host "  Listen address: $httpAddr" -ForegroundColor Gray
    Write-Host "  SSE endpoint:   $httpBaseUrl/sse" -ForegroundColor Gray
    Write-Host ""

    # Start the server as a background process
    $logDir = Join-Path $env:APPDATA "ghost-mcp\logs"
    if (-not (Test-Path $logDir)) {
        New-Item -ItemType Directory -Path $logDir | Out-Null
    }
    $logFile = Join-Path $logDir "ghost-mcp-$(Get-Date -Format 'yyyyMMdd-HHmmss').log"

    Start-Process -FilePath $binaryPath -WindowStyle Hidden -RedirectStandardOutput $logFile -RedirectStandardError $logFile

    Write-Host "  Server started successfully!" -ForegroundColor Green
    Write-Host "  Log file: $logFile" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  To stop the server:" -ForegroundColor Cyan
    Write-Host "    Stop-Process -Name 'ghost-mcp' -Force" -ForegroundColor White
    Write-Host ""
    Write-Host "  To view logs:" -ForegroundColor Cyan
    Write-Host "    Get-Content -Tail 50 -Wait $logFile" -ForegroundColor White
}

# --- final summary -----------------------------------------------------------

Write-Header "Done"

Write-Host ""
Write-Host "  Binary   : $binaryPath" -ForegroundColor White
Write-Host "  Transport: $transport" -ForegroundColor White
Write-Host ""
Write-Host "  +-- YOUR SECRET TOKEN (keep this safe!) ---------------------" -ForegroundColor Red
Write-Host "  |" -ForegroundColor Red
Write-Host "  |   $token" -ForegroundColor Yellow
Write-Host "  |" -ForegroundColor Red
Write-Host "  +-----------------------------------------------------------" -ForegroundColor Red
Write-Host ""

if ($transport -eq "stdio") {
    Write-Host "  Next steps:" -ForegroundColor Cyan
    Write-Host "   1. Restart Claude Desktop so it picks up the new MCP config." -ForegroundColor White
    Write-Host "   2. The ghost-mcp tools will appear in the Claude tool list." -ForegroundColor White
    Write-Host "   3. Move the mouse to (0, 0) at any time to trigger the failsafe." -ForegroundColor White
} elseif ($startService -eq 0) {
    Write-Host "  Next steps:" -ForegroundColor Cyan
    Write-Host "   [OK] Server is running on $httpAddr" -ForegroundColor Green
    Write-Host "   1. Restart Claude Desktop so it picks up the new MCP config." -ForegroundColor White
    Write-Host "   2. The ghost-mcp tools will appear in the Claude tool list." -ForegroundColor White
    Write-Host "   3. Move the mouse to (0, 0) at any time to trigger the failsafe." -ForegroundColor White
} else {
    Write-Host "  Next steps:" -ForegroundColor Cyan
    Write-Host "   1. Start the server:  ghost-mcp.exe" -ForegroundColor White
    Write-Host "   2. It will listen on $httpAddr" -ForegroundColor White
    Write-Host "   3. Restart Claude Desktop so it picks up the new MCP config." -ForegroundColor White
    Write-Host "   4. The ghost-mcp tools will appear in the Claude tool list." -ForegroundColor White
    Write-Host "   5. Move the mouse to (0, 0) at any time to trigger the failsafe." -ForegroundColor White
}

Write-Host ""
