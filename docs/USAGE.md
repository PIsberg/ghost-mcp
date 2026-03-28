# Ghost MCP Usage Guide

This guide demonstrates how to use Ghost MCP to automate UI interactions through the Model Context Protocol (MCP). It includes step-by-step examples with the interactive test fixture.

## Table of Contents

- [Quick Start](#quick-start)
- [Starting the Test Fixture](#starting-the-test-fixture)
- [Using Ghost MCP Tools](#using-ghost-mcp-tools)
- [Interactive Test Fixture Features](#interactive-test-fixture-features)
- [Example Workflows](#example-workflows)
- [API Reference](#api-reference)

## Quick Start

### Prerequisites

- Go 1.22 or later
- GCC/MinGW (required for robotgo on Windows)
- A display environment (Linux requires X11, Windows/macOS have it by default)

### Installation

1. Clone the repository:
```bash
git clone https://github.com/PIsberg/ghost-mcp.git
cd ghost-mcp
```

2. Download dependencies:
```bash
go mod download
```

3. Build the binary:
```bash
# Windows
go build -o ghost-mcp.exe .

# macOS/Linux
go build -o ghost-mcp .
```

## Starting the Test Fixture

The test fixture is an interactive web application that simulates a GUI for testing UI automation.

### Option 1: Using Test Runner (Recommended)

```bash
# Windows
test_runner.bat fixture

# macOS/Linux
chmod +x test_runner.sh
./test_runner.sh fixture
```

The fixture will be available at: **http://localhost:8765**

### Option 2: Direct Command

```bash
go run test_fixture/fixture_server.go
```

### Option 3: Custom Port

```bash
# Windows
set FIXTURE_PORT=9000
go run test_fixture/fixture_server.go

# macOS/Linux
export FIXTURE_PORT=9000
go run test_fixture/fixture_server.go
```

## Using Ghost MCP Tools

Ghost MCP provides the following tools for UI automation:

> **OCR dependency**: `read_screen_text` requires [Tesseract OCR](https://github.com/tesseract-ocr/tesseract) to be installed on the system. The other six tools work without Tesseract.

### 1. **Get Screen Size**

Returns the dimensions of the display.

**Request:**
```json
{
  "tool": "get_screen_size",
  "arguments": {}
}
```

**Response:**
```json
{
  "width": 1920,
  "height": 1080
}
```

**Use Case:** Query display resolution before positioning elements on screen.

---

### 2. **Move Mouse**

Moves the mouse cursor to specified coordinates.

**Request:**
```json
{
  "tool": "move_mouse",
  "arguments": {
    "x": 400,
    "y": 300
  }
}
```

**Response:**
```json
{
  "message": "Mouse moved to position (400, 300)"
}
```

**Use Case:** Position the mouse over a button or input field before clicking.

**Example Workflow:**
1. Move mouse to button location
2. Click to interact with the element
3. Wait for element to respond

---

### 3. **Click**

Performs a mouse click at the current cursor position.

**Request:**
```json
{
  "tool": "click",
  "arguments": {
    "button": "left"
  }
}
```

**Parameters:**
- `button`: `"left"`, `"right"`, or `"middle"` (default: `"left"`)

**Response:**
```json
{
  "message": "Clicked with left button"
}
```

**Use Case:** Activate buttons, checkboxes, radio buttons, and links.

**Full Button Click Example:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 250, "y": 150}},
  {"tool": "click", "arguments": {"button": "left"}}
]
```

---

### 4. **Type Text**

Types text into the currently focused text field.

**Request:**
```json
{
  "tool": "type_text",
  "arguments": {
    "text": "Hello, Ghost MCP!"
  }
}
```

**Response:**
```json
{
  "message": "Typed 16 characters"
}
```

**Use Case:** Fill out text input fields and text areas.

**Full Input Example:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 250}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "type_text", "arguments": {"text": "Test input data"}}
]
```

---

### 5. **Take Screenshot**

Captures the current screen and saves it to a file.

**Request:**
```json
{
  "tool": "take_screenshot",
  "arguments": {
    "filepath": "./screenshot.png"
  }
}
```

**Response:**
```json
{
  "message": "Screenshot saved to ./screenshot.png"
}
```

**Use Case:** Capture screen state for verification and documentation.

**Screenshot Series Example:**
```json
[
  {"tool": "take_screenshot", "arguments": {"filepath": "./step1-initial.png"}},
  {"tool": "move_mouse", "arguments": {"x": 250, "y": 150}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./step2-after-click.png"}}
]
```

---

### 6. **Read Screen Text** (OCR)

Captures a screen region, runs OCR, and returns the text content with word-level bounding boxes. Use the returned `x`/`y` coordinates to click specific words without needing image recognition.

**Request:**
```json
{
  "tool": "read_screen_text",
  "arguments": {
    "x": 0,
    "y": 0,
    "width": 800,
    "height": 600
  }
}
```

**Parameters** (all optional — default is full screen):
- `x`: X coordinate of region (default: 0)
- `y`: Y coordinate of region (default: 0)
- `width`: Width of region (default: full screen width)
- `height`: Height of region (default: full screen height)

**Response:**
```json
{
  "success": true,
  "text": "File Edit View Help\nGhost MCP Automation\nClick Submit to Continue",
  "words": [
    {"text": "File",   "x": 10,  "y": 20,  "width": 30, "height": 15, "confidence": 98.5},
    {"text": "Submit", "x": 450, "y": 320, "width": 60, "height": 20, "confidence": 97.1}
  ],
  "region": {"x": 0, "y": 0, "width": 800, "height": 600}
}
```

**Use Case:** Let the AI read what is on screen and locate UI elements by their label text.

**OCR-driven click example:**
```json
[
  {"tool": "read_screen_text", "arguments": {}},
  // AI finds "Submit" word at x:450, y:320 from the response
  {"tool": "move_mouse", "arguments": {"x": 450, "y": 320}},
  {"tool": "click", "arguments": {"button": "left"}}
]
```

**Requirements:** Tesseract OCR must be installed:
```bash
# Windows (Chocolatey)
choco install tesseract

# macOS
brew install tesseract

# Ubuntu/Debian
sudo apt install tesseract-ocr
```

---

### 7. **Press Key**

Presses a single key or key combination.

**Request:**
```json
{
  "tool": "key_press",
  "arguments": {
    "key": "enter"
  }
}
```

**Supported Keys:**
- `"enter"`, `"escape"`, `"backspace"`, `"tab"`
- `"home"`, `"end"`, `"pageup"`, `"pagedown"`
- `"up"`, `"down"`, `"left"`, `"right"`
- Single characters: `"a"`, `"1"`, `"@"`, etc.

**Response:**
```json
{
  "message": "Pressed key: enter"
}
```

**Use Case:** Navigate forms, confirm dialogs, submit forms.

**Form Submission Example:**
```json
[
  {"tool": "type_text", "arguments": {"text": "myusername"}},
  {"tool": "key_press", "arguments": {"key": "tab"}},
  {"tool": "type_text", "arguments": {"text": "mypassword"}},
  {"tool": "key_press", "arguments": {"key": "enter"}}
]
```

---

## Interactive Test Fixture Features

The test fixture at http://localhost:8765 includes the following interactive elements for testing:

### Initial Fixture Page

When you first load the test fixture, you'll see the main interface with all available test areas:

![Initial Test Fixture](./screenshots/01-initial-fixture.png)

### Button Click Tests

Four colored buttons for testing click automation:
- **Primary** (purple gradient)
- **Success** (green gradient)
- **Warning** (pink/red gradient)
- **Info** (cyan gradient)

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 250, "y": 150}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./button-clicked.png"}}
]
```

**Expected Result:** Button animates with a click flash effect, event log updates.

**Fixture Screenshots:**

Primary Button Clicked:
![Primary Button Clicked](./screenshots/02-primary-button-clicked.png)

Success Button Clicked:
![Success Button Clicked](./screenshots/03-success-button-clicked.png)

Warning Button Clicked:
![Warning Button Clicked](./screenshots/04-warning-button-clicked.png)

Info Button Clicked:
![Info Button Clicked](./screenshots/05-info-button-clicked.png)

---

### Input Fields

Two text input areas for testing text automation:
- **Single-line input** - For short text entry
- **Multi-line textarea** - For longer content

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 250}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "type_text", "arguments": {"text": "Testing text input automation"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./text-input.png"}}
]
```

**Expected Result:** Text appears in the input field, counter shows character count.

**Fixture Screenshots:**

Text Input Filled:
![Text Input Filled](./screenshots/06-text-input-filled.png)

Textarea Filled:
![Textarea Filled](./screenshots/07-textarea-filled.png)

---

### Selection Controls

#### Checkboxes (3 options)
Toggle checkboxes to test state management:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 300, "y": 350}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./checkbox-checked.png"}}
]
```

**Expected Result:** Checkbox toggles, event log records the state change.

**Fixture Screenshot:**

Checkboxes Toggled:
![Checkboxes Toggled](./screenshots/08-checkbox-checked.png)

#### Radio Buttons (3 choices: A, B, C)
Select one option from multiple choices:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 350, "y": 380}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./radio-selected.png"}}
]
```

**Expected Result:** Radio button selects, previous selection clears automatically.

**Fixture Screenshot:**

Radio Button Selected:
![Radio Button Selected](./screenshots/09-radio-selected.png)

---

### Dropdown (Select)

Test dropdown/select element interaction:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 450}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./dropdown-opened.png"}},
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 480}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./option-selected.png"}}
]
```

**Expected Result:** Dropdown opens, option selects, result displays in test summary.

**Fixture Screenshot:**

Dropdown Option Selected:
![Dropdown Option Selected](./screenshots/10-dropdown-selected.png)

---

### Slider (Range Input)

Test slider/range control automation:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 350, "y": 520}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "move_mouse", "arguments": {"x": 450, "y": 520}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./slider-adjusted.png"}}
]
```

**Expected Result:** Slider moves, percentage value updates in real-time.

**Fixture Screenshot:**

Slider Adjusted:
![Slider Adjusted](./screenshots/11-slider-adjusted.png)

---

### Color Picker

Test color selection automation:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 300, "y": 580}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./color-changed.png"}}
]
```

**Expected Result:** Color box changes to selected color, animation plays.

**Fixture Screenshot:**

Color Changed:
![Color Changed](./screenshots/12-color-changed.png)

---

### Click Counter

Button with persistent click counting:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 650}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./counter-updated.png"}}
]
```

**Expected Result:** Counter increments to 3, test result shows "✓".

**Fixture Screenshot:**

Counter Incremented:
![Counter Incremented](./screenshots/13-counter-incremented.png)

---

### Hover Detection

Zone that detects mouse hover events:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 750}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./hover-detected.png"}},
  {"tool": "move_mouse", "arguments": {"x": 100, "y": 100}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./hover-exited.png"}}
]
```

**Expected Result:** Zone highlights when hovered, unhighlights when mouse leaves.

**Fixture Screenshot:**

Hover Detected:
![Hover Detected](./screenshots/14-hover-detected.png)

---

### Keyboard Input

Test keyboard key press automation:

**Test Scenario:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 820}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "type_text", "arguments": {"text": "hello"}},
  {"tool": "key_press", "arguments": {"key": "backspace"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./keyboard-input.png"}}
]
```

**Expected Result:** Text types, backspace removes last character, test result shows key count.

**Fixture Screenshot:**

Keyboard Input Tested:
![Keyboard Input Tested](./screenshots/15-keyboard-input.png)

---

### Event Log

Real-time log of all interactions on the page.

**Features:**
- **Timestamp** - When the event occurred
- **Event Type** - BUTTON_CLICK, INPUT, CHECKBOX, etc.
- **Details** - Specific information about the event
- **Auto-scroll** - Shows most recent events at top
- **Capacity** - Keeps last 50 events

**Clearing Log:**
```json
[
  {"tool": "move_mouse", "arguments": {"x": 300, "y": 900}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./log-cleared.png"}}
]
```

**Fixture Screenshot:**

Event Log with All Interactions:
![Event Log](./screenshots/16-event-log.png)

---

## Example Workflows

### Complete User Registration Flow

This example demonstrates a complete workflow using multiple Ghost MCP tools:

```json
[
  {"tool": "take_screenshot", "arguments": {"filepath": "./01-initial-page.png"}},
  
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 250}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "type_text", "arguments": {"text": "Test input data"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./02-text-entered.png"}},
  
  {"tool": "move_mouse", "arguments": {"x": 250, "y": 150}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./03-button-clicked.png"}},
  
  {"tool": "move_mouse", "arguments": {"x": 300, "y": 350}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./04-checkbox-toggled.png"}},
  
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 450}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./05-dropdown-opened.png"}},
  
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 480}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./06-option-selected.png"}},
  
  {"tool": "take_screenshot", "arguments": {"filepath": "./07-final-state.png"}}
]
```

### Screenshot Series Verification

Compare multiple states to verify automation:

```json
[
  {"tool": "take_screenshot", "arguments": {"filepath": "./before.png"}},
  
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 650}},
  {"tool": "click", "arguments": {"button": "left"}},
  
  {"tool": "take_screenshot", "arguments": {"filepath": "./after-click-1.png"}},
  
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "click", "arguments": {"button": "left"}},
  
  {"tool": "take_screenshot", "arguments": {"filepath": "./after-click-3.png"}}
]
```

### Form Navigation with Keyboard

Navigate between form fields using Tab and Enter:

```json
[
  {"tool": "move_mouse", "arguments": {"x": 400, "y": 250}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "type_text", "arguments": {"text": "Username"}},
  
  {"tool": "key_press", "arguments": {"key": "tab"}},
  {"tool": "type_text", "arguments": {"text": "Password123"}},
  
  {"tool": "key_press", "arguments": {"key": "tab"}},
  {"tool": "key_press", "arguments": {"key": "enter"}},
  
  {"tool": "take_screenshot", "arguments": {"filepath": "./form-submitted.png"}}
]
```

---

## Test Results Summary

After running through all the interactive tests, the fixture displays a comprehensive summary showing which tests passed:

![Test Results Summary](./screenshots/17-test-results.png)

The summary panel shows:
- ✓ **Buttons Clicked** - Number of button interactions detected
- ✓ **Input Typed** - Character count from text inputs
- ✓ **Checkbox Toggled** - Number of checkboxes checked
- ✓ **Dropdown Changed** - Selected dropdown value
- ✓ **Slider Adjusted** - Final slider percentage value
- ✓ **Keys Pressed** - Number of keyboard events detected

---

## API Reference

### Tool: get_screen_size

Gets the dimensions of the display.

**Arguments:** (none)

**Returns:**
```json
{
  "width": 1920,
  "height": 1080
}
```

---

### Tool: move_mouse

Moves the mouse cursor to the specified coordinates.

**Arguments:**
- `x` (number, required): X-coordinate in pixels
- `y` (number, required): Y-coordinate in pixels

**Returns:**
```json
{
  "message": "Mouse moved to position (400, 300)"
}
```

---

### Tool: click

Performs a mouse click.

**Arguments:**
- `button` (string, optional): `"left"`, `"right"`, or `"middle"` (default: `"left"`)

**Returns:**
```json
{
  "message": "Clicked with left button"
}
```

---

### Tool: type_text

Types text into the focused element.

**Arguments:**
- `text` (string, required): Text to type

**Returns:**
```json
{
  "message": "Typed 16 characters"
}
```

---

### Tool: take_screenshot

Captures the screen to a file.

**Arguments:**
- `filepath` (string, required): Path where screenshot should be saved

**Returns:**
```json
{
  "message": "Screenshot saved to ./screenshot.png"
}
```

---

### Tool: press_key

Presses a key or key combination.

**Arguments:**
- `key` (string, required): Key name or character

**Returns:**
```json
{
  "success": true,
  "key": "enter"
}
```

---

### Tool: read_screen_text

Reads text from a screen region using OCR. Requires Tesseract OCR to be installed.

**Arguments** (all optional):
- `x` (number): X coordinate of region (default: 0)
- `y` (number): Y coordinate of region (default: 0)
- `width` (number): Width of region (default: full screen width)
- `height` (number): Height of region (default: full screen height)

**Returns:**
```json
{
  "success": true,
  "text": "Full extracted text including newlines",
  "words": [
    {
      "text": "Submit",
      "x": 450,
      "y": 320,
      "width": 60,
      "height": 20,
      "confidence": 97.1
    }
  ],
  "region": {"x": 0, "y": 0, "width": 1920, "height": 1080}
}
```

The `words` array gives pixel coordinates for each recognised word so the AI can move the mouse to a specific word and click it.

---

## Best Practices

### 1. Always Capture Initial State
Take a screenshot before making changes for comparison:
```json
{"tool": "take_screenshot", "arguments": {"filepath": "./step0-initial.png"}}
```

### 2. Add Delays Between Actions
Some UI updates may need time to render. Add wait time in your client:
```json
[
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./after-click.png"}}
]
```

### 3. Use Descriptive File Paths
Include step numbers and descriptions:
```
./step01-initial-page.png
./step02-button-clicked.png
./step03-form-filled.png
./step04-submitted.png
```

### 4. Avoid Mouse Failsafe Position
Don't move mouse to (0, 0) as it may trigger system failsafe:
```json
{"tool": "move_mouse", "arguments": {"x": 100, "y": 100}}
```

### 5. Verify State Changes
Use the event log to confirm interactions:
```json
[
  {"tool": "move_mouse", "arguments": {"x": 300, "y": 350}},
  {"tool": "click", "arguments": {"button": "left"}},
  {"tool": "take_screenshot", "arguments": {"filepath": "./verify-checkbox.png"}}
]
```

### 6. Clean Up Screenshots
Remove screenshot files after verification to avoid accumulation.

---

## Troubleshooting

### Mouse Not Responding
- **macOS**: Grant Terminal accessibility permissions (System Preferences → Security & Privacy → Accessibility)
- **Linux**: Check X11 permissions with `xhost +`
- **Windows**: Try running as Administrator

### Fixture Port Already in Use
```bash
# Check what's using port 8765
# Windows: netstat -ano | findstr :8765
# Linux/macOS: lsof -i :8765

# Use a different port
export FIXTURE_PORT=9000
go run test_fixture/fixture_server.go
```

### GCC Not Found
Install compiler:
```bash
# Windows (Chocolatey)
choco install mingw

# macOS
xcode-select --install

# Ubuntu/Debian
sudo apt install gcc libx11-dev xorg-dev libxtst-dev libpng-dev
```

### No Display Available (Linux)
Use virtual display:
```bash
sudo apt install xvfb
export DISPLAY=:99
Xvfb :99 &
```

---

## See Also

- [TESTING.md](TESTING.md) - Comprehensive testing guide
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture
- [README.md](README.md) - Project overview
