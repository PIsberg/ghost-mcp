# Ghost MCP Usage Guide

This guide demonstrates how to use Ghost MCP to automate UI interactions through the Model Context Protocol (MCP). It includes step-by-step examples with the interactive test fixture.

## Table of Contents

- [Quick Start](#quick-start)
- [Starting the Test Fixture](#starting-the-test-fixture)
- [Using Ghost MCP Tools](#using-ghost-mcp-tools)
- [Interactive Test Fixture](#interactive-test-fixture)
- [OCR Text Payloads](#ocr-text-payloads)
- [Example Workflows](#example-workflows)
- [API Reference](#api-reference)

## Quick Start

### Prerequisites

- Go 1.22 or later
- GCC/MinGW (required for robotgo on Windows)
- A display environment (Linux requires X11, Windows/macOS have it by default)
- [Tesseract OCR](https://github.com/tesseract-ocr/tesseract) (required for `read_screen_text` only)

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
go build -o ghost-mcp.exe ./cmd/ghost-mcp/

# macOS/Linux
go build -o ghost-mcp ./cmd/ghost-mcp/
```

## Starting the Test Fixture

The test fixture is an interactive web application that simulates a GUI for testing UI automation.

```bash
# Windows
test_runner.bat fixture

# macOS/Linux
./test_runner.sh fixture
```

The fixture will be available at: **http://localhost:8765**

## Using Ghost MCP Tools

Ghost MCP provides seven tools for UI automation.

> **OCR dependency**: `read_screen_text` requires [Tesseract OCR](https://github.com/tesseract-ocr/tesseract). The other six tools work without it.

---

### 1. **Get Screen Size**

Returns the dimensions of the primary display.

```json
{ "tool": "get_screen_size", "arguments": {} }
```

```json
{ "width": 1920, "height": 1080 }
```

---

### 2. **Move Mouse**

Moves the mouse cursor to specified coordinates.

```json
{ "tool": "move_mouse", "arguments": { "x": 400, "y": 300 } }
```

```json
{ "success": true, "x": 400, "y": 300 }
```

---

### 3. **Click**

Performs a mouse click at the current cursor position.

```json
{ "tool": "click", "arguments": { "button": "left" } }
```

`button`: `"left"` (default), `"right"`, or `"middle"`

---

### 4. **Type Text**

Types text via the keyboard into the focused element.

```json
{ "tool": "type_text", "arguments": { "text": "Hello, Ghost MCP!" } }
```

```json
{ "success": true, "characters_typed": 17 }
```

---

### 5. **Press Key**

Presses a single key on the keyboard.

```json
{ "tool": "press_key", "arguments": { "key": "enter" } }
```

Supported keys include: `enter`, `tab`, `esc`, `space`, `backspace`, `delete`, `home`, `end`, `pageup`, `pagedown`, `up`, `down`, `left`, `right`, `ctrl`, `alt`, `shift`, `f1`–`f12`, and single characters.

---

### 6. **Take Screenshot**

Captures a screen region and returns a base64-encoded PNG.

```json
{
  "tool": "take_screenshot",
  "arguments": { "x": 0, "y": 0, "width": 1920, "height": 1080 }
}
```

```json
{
  "success": true,
  "base64": "iVBORw0KGgo...",
  "width": 1920,
  "height": 1080
}
```

All parameters are optional — omitting them captures the full screen.

---

### 7. **Read Screen Text** (OCR)

Captures a screen region, runs OCR, and returns the text with word-level bounding boxes. The coordinates in the response can be passed directly to `move_mouse` to click specific words.

```json
{
  "tool": "read_screen_text",
  "arguments": { "x": 0, "y": 0, "width": 800, "height": 400 }
}
```

```json
{
  "success": true,
  "text": "File  Edit  View  Help\nGhost MCP Automation\nClick Submit to Continue",
  "words": [
    { "text": "File",   "x": 10,  "y": 20,  "width": 28, "height": 14, "confidence": 98.5 },
    { "text": "Edit",   "x": 58,  "y": 20,  "width": 26, "height": 14, "confidence": 97.2 },
    { "text": "Submit", "x": 110, "y": 68,  "width": 52, "height": 16, "confidence": 96.8 }
  ],
  "region": { "x": 0, "y": 0, "width": 800, "height": 400 }
}
```

All parameters are optional — omitting them reads the full screen.

**Install Tesseract:**
```bash
choco install tesseract        # Windows
brew install tesseract         # macOS
sudo apt install tesseract-ocr # Ubuntu/Debian
```

---

## Interactive Test Fixture

The fixture at `http://localhost:8765` contains every control type Ghost MCP can interact with.

### Full Page Overview

![Full fixture page](./screenshots/00-full-page.png)

---

### Top of Page — Buttons and Input Fields

The top section shows the status bar, four coloured buttons, and text input fields.

![Top of fixture](./screenshots/01-initial-fixture.png)

**Button click workflow:**
```json
[
  { "tool": "move_mouse", "arguments": { "x": 183, "y": 98 } },
  { "tool": "click",      "arguments": { "button": "left" } },
  { "tool": "take_screenshot", "arguments": {} }
]
```

**Text input workflow:**
```json
[
  { "tool": "move_mouse", "arguments": { "x": 400, "y": 150 } },
  { "tool": "click",      "arguments": { "button": "left" } },
  { "tool": "type_text",  "arguments": { "text": "Automated input" } }
]
```

---

### Selection Controls — Checkboxes, Radio Buttons, Dropdown, Slider

![Selection controls](./screenshots/06-text-input-filled.png)

**Checkbox toggle:**
```json
[
  { "tool": "move_mouse", "arguments": { "x": 205, "y": 240 } },
  { "tool": "click",      "arguments": { "button": "left" } }
]
```

**Dropdown selection:**
```json
[
  { "tool": "move_mouse", "arguments": { "x": 245, "y": 340 } },
  { "tool": "click",      "arguments": { "button": "left" } }
]
```

---

### Lower Controls — Slider, Color Picker, Click Counter, Hover Zone

![Lower controls](./screenshots/08-checkbox-checked.png)

**Click counter:**
```json
[
  { "tool": "move_mouse", "arguments": { "x": 397, "y": 308 } },
  { "tool": "click",      "arguments": { "button": "left" } },
  { "tool": "click",      "arguments": { "button": "left" } },
  { "tool": "click",      "arguments": { "button": "left" } }
]
```

---

### Keyboard Test, Event Log, and Test Results

![Keyboard and event log](./screenshots/04-lower-controls.png)

**Keyboard test workflow:**
```json
[
  { "tool": "move_mouse", "arguments": { "x": 300, "y": 190 } },
  { "tool": "click",      "arguments": { "button": "left" } },
  { "tool": "press_key",  "arguments": { "key": "enter" } },
  { "tool": "press_key",  "arguments": { "key": "tab" } }
]
```

---

### OCR Text Recognition Panel

The OCR panel contains high-contrast text for reliable OCR testing.

![OCR Test Panel](./screenshots/05-ocr-panel.png)

---

### Test Results Summary

After running through all interactions, the results summary shows which tests passed.

![Test Results](./screenshots/17-test-results.png)

---

## OCR Text Payloads

This section shows what `read_screen_text` returns when pointed at different parts of the fixture.

### Full fixture page (top viewport)

Calling `read_screen_text` with no arguments on the fixture page returns something like:

```json
{
  "success": true,
  "text": "Ghost MCP Test Fixture\nInteractive GUI for testing MCP UI automation tools\nINIT: Test fixture ready for MCP automation\nMouse: ...\nButton Click Tests\nPRIMARY  SUCCESS  WARNING  INFO\nInput Tests\nType here or use MCP type_text...\nCLEAR\nMulti-line text area...\nSelection Tests\nCheckboxes:\nOption 1  Option 2  Option 3\nRadio Buttons:\nChoice A  Choice B  Choice C\nDropdown Test\n-- Select an option --",
  "words": [
    { "text": "Ghost",      "x": 350, "y": 10,  "width": 45, "height": 18, "confidence": 98.2 },
    { "text": "MCP",        "x": 400, "y": 10,  "width": 32, "height": 18, "confidence": 97.9 },
    { "text": "Test",       "x": 436, "y": 10,  "width": 30, "height": 18, "confidence": 98.5 },
    { "text": "Fixture",    "x": 470, "y": 10,  "width": 48, "height": 18, "confidence": 97.1 },
    { "text": "PRIMARY",    "x": 149, "y": 88,  "width": 65, "height": 14, "confidence": 96.4 },
    { "text": "SUCCESS",    "x": 232, "y": 88,  "width": 65, "height": 14, "confidence": 97.0 },
    { "text": "WARNING",    "x": 315, "y": 88,  "width": 65, "height": 14, "confidence": 95.8 },
    { "text": "INFO",       "x": 398, "y": 88,  "width": 35, "height": 14, "confidence": 98.1 }
  ],
  "region": { "x": 0, "y": 0, "width": 1707, "height": 932 }
}
```

### OCR test panel (white background area)

Targeting just the white OCR test panel at the bottom of the fixture gives a clean, high-confidence result:

```json
{
  "success": true,
  "text": "OCR Test Area\nHello World\nGhost MCP Automation\nClick Submit to Continue\nFile  Edit  View  Help",
  "words": [
    { "text": "OCR",        "x": 10,  "y": 8,   "width": 28, "height": 20, "confidence": 99.1 },
    { "text": "Test",       "x": 42,  "y": 8,   "width": 28, "height": 20, "confidence": 99.3 },
    { "text": "Area",       "x": 74,  "y": 8,   "width": 30, "height": 20, "confidence": 99.2 },
    { "text": "Hello",      "x": 10,  "y": 40,  "width": 38, "height": 16, "confidence": 99.5 },
    { "text": "World",      "x": 52,  "y": 40,  "width": 38, "height": 16, "confidence": 99.4 },
    { "text": "Ghost",      "x": 10,  "y": 64,  "width": 38, "height": 16, "confidence": 99.2 },
    { "text": "MCP",        "x": 52,  "y": 64,  "width": 28, "height": 16, "confidence": 99.0 },
    { "text": "Automation", "x": 84,  "y": 64,  "width": 72, "height": 16, "confidence": 98.9 },
    { "text": "Click",      "x": 10,  "y": 88,  "width": 34, "height": 16, "confidence": 99.3 },
    { "text": "Submit",     "x": 48,  "y": 88,  "width": 44, "height": 16, "confidence": 99.1 },
    { "text": "to",         "x": 96,  "y": 88,  "width": 14, "height": 16, "confidence": 98.8 },
    { "text": "Continue",   "x": 114, "y": 88,  "width": 58, "height": 16, "confidence": 99.0 },
    { "text": "File",       "x": 10,  "y": 116, "width": 22, "height": 14, "confidence": 99.2 },
    { "text": "Edit",       "x": 62,  "y": 116, "width": 22, "height": 14, "confidence": 99.1 },
    { "text": "View",       "x": 122, "y": 116, "width": 28, "height": 14, "confidence": 99.0 },
    { "text": "Help",       "x": 182, "y": 116, "width": 24, "height": 14, "confidence": 99.3 }
  ],
  "region": { "x": 140, "y": 2680, "width": 600, "height": 200 }
}
```

### OCR-driven click example

Using the word positions from `read_screen_text` to click "Submit":

```json
[
  {
    "tool": "read_screen_text",
    "arguments": { "x": 140, "y": 2680, "width": 600, "height": 200 }
  },
  { "comment": "AI finds 'Submit' at x:48+140=188, y:88+2680=2768 (region offset)" },
  { "tool": "move_mouse", "arguments": { "x": 188, "y": 2768 } },
  { "tool": "click",      "arguments": { "button": "left" } }
]
```

> Note: `read_screen_text` returns coordinates **relative to the region origin** (the `x`/`y` parameters). Add the region offset to get absolute screen coordinates.

---

## Example Workflows

### Complete automation workflow

```json
[
  { "tool": "get_screen_size", "arguments": {} },

  { "tool": "move_mouse",  "arguments": { "x": 183, "y": 98 } },
  { "tool": "click",       "arguments": { "button": "left" } },
  { "tool": "take_screenshot", "arguments": {} },

  { "tool": "move_mouse",  "arguments": { "x": 400, "y": 150 } },
  { "tool": "click",       "arguments": { "button": "left" } },
  { "tool": "type_text",   "arguments": { "text": "Automated test data" } },

  { "tool": "move_mouse",  "arguments": { "x": 205, "y": 240 } },
  { "tool": "click",       "arguments": { "button": "left" } },

  { "tool": "take_screenshot", "arguments": {} }
]
```

### OCR-driven navigation

Let the AI read the screen and decide where to click:

```json
[
  { "tool": "read_screen_text", "arguments": {} },
  { "comment": "AI reads text and word positions, then moves to the right button" },
  { "tool": "move_mouse", "arguments": { "x": 183, "y": 98 } },
  { "tool": "click",      "arguments": { "button": "left" } }
]
```

### Form navigation with keyboard

```json
[
  { "tool": "move_mouse", "arguments": { "x": 400, "y": 150 } },
  { "tool": "click",      "arguments": { "button": "left" } },
  { "tool": "type_text",  "arguments": { "text": "Username" } },
  { "tool": "press_key",  "arguments": { "key": "tab" } },
  { "tool": "type_text",  "arguments": { "text": "Password123" } },
  { "tool": "press_key",  "arguments": { "key": "enter" } }
]
```

---

## API Reference

### Tool: get_screen_size

**Arguments:** none

**Returns:** `{ "width": 1920, "height": 1080 }`

---

### Tool: move_mouse

**Arguments:**
- `x` (number, required): X-coordinate in pixels
- `y` (number, required): Y-coordinate in pixels

**Returns:** `{ "success": true, "x": 400, "y": 300 }`

---

### Tool: click

**Arguments:**
- `button` (string, required): `"left"`, `"right"`, or `"middle"`

**Returns:** `{ "success": true, "button": "left", "x": 400, "y": 300 }`

---

### Tool: type_text

**Arguments:**
- `text` (string, required): Text to type (max 10,000 characters)

**Returns:** `{ "success": true, "characters_typed": 16 }`

---

### Tool: press_key

**Arguments:**
- `key` (string, required): Key name (e.g. `"enter"`, `"tab"`, `"esc"`, `"ctrl"`)

**Returns:** `{ "success": true, "key": "enter" }`

---

### Tool: take_screenshot

**Arguments** (all optional):
- `x` (number): X coordinate of region (default: 0)
- `y` (number): Y coordinate of region (default: 0)
- `width` (number): Width of region (default: full screen)
- `height` (number): Height of region (default: full screen)

**Returns:**
```json
{
  "success": true,
  "filepath": "/tmp/ghost-mcp-screenshot-1234.png",
  "base64": "iVBORw0KGgo...",
  "width": 1920,
  "height": 1080
}
```

---

### Tool: read_screen_text

Reads text from a screen region using OCR. Requires Tesseract OCR.

**Arguments** (all optional):
- `x` (number): X coordinate of region (default: 0)
- `y` (number): Y coordinate of region (default: 0)
- `width` (number): Width of region (default: full screen)
- `height` (number): Height of region (default: full screen)

**Returns:**
```json
{
  "success": true,
  "text": "Full extracted text with newlines",
  "words": [
    {
      "text": "Submit",
      "x": 450, "y": 320,
      "width": 60, "height": 20,
      "confidence": 97.1
    }
  ],
  "region": { "x": 0, "y": 0, "width": 1920, "height": 1080 }
}
```

Word coordinates are **relative to the region origin** — add the region's `x`/`y` to get absolute screen positions.

---

## Best Practices

### 1. Read before clicking
Use `read_screen_text` to locate buttons by label rather than hardcoding coordinates:
```json
{ "tool": "read_screen_text", "arguments": {} }
```

### 2. Verify with screenshots
Take a screenshot before and after key actions to confirm the UI changed as expected.

### 3. Avoid the failsafe position
Don't move the mouse to (0, 0) — this triggers an emergency shutdown.

### 4. Use regions for faster OCR
Narrow the region to where the text you need is located for quicker, more accurate results:
```json
{ "tool": "read_screen_text", "arguments": { "x": 0, "y": 0, "width": 800, "height": 100 } }
```

### 5. Convert word coordinates
`read_screen_text` returns coordinates relative to the region. Always add the region offset:
```
absolute_x = word.x + region.x
absolute_y = word.y + region.y
```

---

## Troubleshooting

### Mouse Not Responding
- **macOS**: Grant Terminal accessibility permissions (System Preferences → Security & Privacy → Accessibility)
- **Linux**: Check X11 permissions with `xhost +`
- **Windows**: Try running as Administrator

### Tesseract Not Found
```bash
# Windows (Chocolatey)
choco install tesseract

# macOS
brew install tesseract

# Ubuntu/Debian
sudo apt install tesseract-ocr
```

### Fixture Port Already in Use
```bash
# Use a different port
export FIXTURE_PORT=9000
go run ./cmd/ghost-mcp/test_fixture/
```

### GCC Not Found (build error)
```bash
# Windows (Chocolatey)
choco install mingw

# macOS
xcode-select --install

# Ubuntu/Debian
sudo apt install gcc libx11-dev xorg-dev libxtst-dev libpng-dev
```

### No Display Available (Linux)
```bash
sudo apt install xvfb
export DISPLAY=:99
Xvfb :99 &
```

---

## See Also

- [TESTING.md](TESTING.md) - Comprehensive testing guide
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture
