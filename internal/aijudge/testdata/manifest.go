// Package testdata contains ground-truth element definitions for the
// Ghost MCP test fixture pages. These are used by the AI judge tests to
// verify OCR accuracy without needing a live display.
package testdata

import (
	"github.com/ghost-mcp/internal/aijudge"
)

// GroundTruth pairs a fixture image filename with the expected elements.
type GroundTruth struct {
	Name     string
	Image    string // relative path to PNG screenshot
	Elements []aijudge.JudgedElement
}

// FixtureNormal defines the expected elements visible in fixture_normal.png
// (a single-viewport screenshot of index.html, light theme). Elements below the
// captured viewport — most notably the OCR Test Area panel — are intentionally
// omitted because they are not present in the screenshot. Coordinates are
// approximate; CompareResults matches on text similarity (IoU defaults to 0).
var FixtureNormal = GroundTruth{
	Name:  "normal_fixture",
	Image: "fixture_normal.png",
	Elements: []aijudge.JudgedElement{
		// Page title + subtitle
		{Text: "Ghost MCP Test Fixture", Type: "heading", Rect: aijudge.Rect{X: 200, Y: 10, Width: 500, Height: 40}},
		{Text: "Interactive GUI for testing MCP UI automation tools", Type: "text", Rect: aijudge.Rect{X: 220, Y: 78, Width: 440, Height: 20}},

		// Navigation
		{Text: "Normal page", Type: "link", Rect: aijudge.Rect{X: 380, Y: 108, Width: 90, Height: 18}},
		{Text: "Challenge page", Type: "link", Rect: aijudge.Rect{X: 490, Y: 108, Width: 110, Height: 18}},

		// Status bar (initial state after init() — JS sets statusText to "Ready")
		{Text: "Ready", Type: "text", Rect: aijudge.Rect{X: 240, Y: 146, Width: 50, Height: 18}},
		{Text: "Mouse:", Type: "label", Rect: aijudge.Rect{X: 920, Y: 146, Width: 50, Height: 18}},
		{Text: "--,--", Type: "value", Rect: aijudge.Rect{X: 980, Y: 146, Width: 45, Height: 18}},

		// Last action display. Gemini sometimes splits this into label + value
		// across runs; substring matching handles either form when listed as one.
		{Text: "Last Action: None", Type: "text", Rect: aijudge.Rect{X: 380, Y: 178, Width: 200, Height: 20}},

		// Button Click Tests panel
		{Text: "Button Click Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 230, Width: 200, Height: 24}},
		{Text: "PRIMARY", Type: "button", Rect: aijudge.Rect{X: 30, Y: 270, Width: 200, Height: 45}},
		{Text: "SUCCESS", Type: "button", Rect: aijudge.Rect{X: 240, Y: 270, Width: 200, Height: 45}},
		{Text: "WARNING", Type: "button", Rect: aijudge.Rect{X: 450, Y: 270, Width: 200, Height: 45}},
		{Text: "INFO", Type: "button", Rect: aijudge.Rect{X: 660, Y: 270, Width: 200, Height: 45}},

		// Input Tests panel
		{Text: "Input Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 350, Width: 150, Height: 24}},
		{Text: "Text Input:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 390, Width: 80, Height: 20}},
		{Text: "Type here or use MCP type_text...", Type: "input", Rect: aijudge.Rect{X: 120, Y: 388, Width: 600, Height: 36}},
		{Text: "CLEAR", Type: "button", Rect: aijudge.Rect{X: 780, Y: 388, Width: 70, Height: 35}},
		{Text: "Text Area:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 432, Width: 80, Height: 20}},
		{Text: "Multi-line text area...", Type: "input", Rect: aijudge.Rect{X: 120, Y: 432, Width: 730, Height: 60}},

		// Selection Tests panel
		{Text: "Selection Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 530, Width: 180, Height: 24}},
		{Text: "Checkboxes:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 565, Width: 100, Height: 18}},
		{Text: "Option 1", Type: "checkbox", Rect: aijudge.Rect{X: 30, Y: 590, Width: 100, Height: 20}},
		{Text: "Option 2", Type: "checkbox", Rect: aijudge.Rect{X: 140, Y: 590, Width: 100, Height: 20}},
		{Text: "Option 3", Type: "checkbox", Rect: aijudge.Rect{X: 250, Y: 590, Width: 100, Height: 20}},
		{Text: "Radio Buttons:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 625, Width: 110, Height: 18}},
		{Text: "Choice A", Type: "radio", Rect: aijudge.Rect{X: 30, Y: 650, Width: 100, Height: 20}},
		{Text: "Choice B", Type: "radio", Rect: aijudge.Rect{X: 140, Y: 650, Width: 100, Height: 20}},
		{Text: "Choice C", Type: "radio", Rect: aijudge.Rect{X: 250, Y: 650, Width: 100, Height: 20}},

		// Dropdown
		{Text: "Dropdown Test", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 700, Width: 160, Height: 24}},
		{Text: "-- Select an option --", Type: "dropdown", Rect: aijudge.Rect{X: 30, Y: 740, Width: 200, Height: 35}},

		// Slider
		{Text: "Slider Test", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 800, Width: 140, Height: 24}},
		{Text: "50", Type: "value", Rect: aijudge.Rect{X: 425, Y: 855, Width: 50, Height: 25}},

		// Color Picker Test — six small color buttons next to the preview swatch
		{Text: "Color Picker Test", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 905, Width: 200, Height: 24}},
		{Text: "Color swatch", Type: "icon", Rect: aijudge.Rect{X: 30, Y: 945, Width: 80, Height: 80}},
		{Text: "Color swatch", Type: "icon", Rect: aijudge.Rect{X: 130, Y: 965, Width: 36, Height: 36}},
		{Text: "Color swatch", Type: "icon", Rect: aijudge.Rect{X: 175, Y: 965, Width: 36, Height: 36}},
		{Text: "Color swatch", Type: "icon", Rect: aijudge.Rect{X: 220, Y: 965, Width: 36, Height: 36}},
		{Text: "Color swatch", Type: "icon", Rect: aijudge.Rect{X: 265, Y: 965, Width: 36, Height: 36}},
		{Text: "Color swatch", Type: "icon", Rect: aijudge.Rect{X: 310, Y: 965, Width: 36, Height: 36}},

		// Click Counter
		{Text: "Click Counter", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 1040, Width: 160, Height: 24}},
		{Text: "0", Type: "value", Rect: aijudge.Rect{X: 425, Y: 1080, Width: 30, Height: 50}},
		{Text: "Total clicks on this button", Type: "text", Rect: aijudge.Rect{X: 320, Y: 1135, Width: 240, Height: 18}},
		{Text: "Click Me!", Type: "button", Rect: aijudge.Rect{X: 30, Y: 1165, Width: 820, Height: 40}},

		// Hover Detection Zone
		{Text: "Hover Detection Zone", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 1245, Width: 240, Height: 24}},
		{Text: "Move mouse over this area...", Type: "text", Rect: aijudge.Rect{X: 320, Y: 1295, Width: 250, Height: 18}},

		// Keyboard Test
		{Text: "Keyboard Test", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 1370, Width: 160, Height: 24}},
		{Text: "Press any key to test keyboard input:", Type: "text", Rect: aijudge.Rect{X: 30, Y: 1405, Width: 320, Height: 18}},
		{Text: "Focus here and press keys...", Type: "input", Rect: aijudge.Rect{X: 30, Y: 1430, Width: 820, Height: 36}},

		// Event Log
		{Text: "Event Log", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 1505, Width: 120, Height: 24}},
		// logEvent() prepends new entries without removing the placeholder, so
		// after init() the log contains two rows: the runtime "INIT" entry on top
		// and the original "--:--:-- System" placeholder beneath it.
		// NOTE: The timestamp on the INIT row is dynamic (e.g. "23:43:34") and
		// varies per run, so we omit it from ground truth. The "INIT" label and
		// surrounding static text are sufficient for matching.
		{Text: "INIT", Type: "text", Rect: aijudge.Rect{X: 125, Y: 1545, Width: 40, Height: 16}},
		{Text: "Test fixture ready for MCP automation", Type: "text", Rect: aijudge.Rect{X: 175, Y: 1545, Width: 320, Height: 16}},
		{Text: "--:--:--", Type: "text", Rect: aijudge.Rect{X: 42, Y: 1565, Width: 75, Height: 16}},
		{Text: "System", Type: "text", Rect: aijudge.Rect{X: 125, Y: 1565, Width: 60, Height: 16}},
		{Text: "Test fixture initialized. Waiting for interaction...", Type: "text", Rect: aijudge.Rect{X: 195, Y: 1565, Width: 400, Height: 16}},
		{Text: "CLEAR LOG", Type: "button", Rect: aijudge.Rect{X: 30, Y: 1620, Width: 110, Height: 35}},
	},
}

// FixtureChallenge defines the expected elements on challenge.html (dark theme, gradients).
// This is the harder test case for OCR — text on dark/gradient backgrounds.
var FixtureChallenge = GroundTruth{
	Name:  "challenge_fixture",
	Image: "fixture_challenge.png",
	Elements: []aijudge.JudgedElement{
		// Page title (neon cyan on dark)
		{Text: "Ghost MCP Test Fixture", Type: "heading", Rect: aijudge.Rect{X: 250, Y: 10, Width: 400, Height: 40}},

		// Navigation
		{Text: "Normal page", Type: "link", Rect: aijudge.Rect{X: 350, Y: 65, Width: 80, Height: 16}},
		{Text: "Challenge page", Type: "link", Rect: aijudge.Rect{X: 450, Y: 65, Width: 100, Height: 16}},

		// Status bar
		{Text: "Waiting for interaction...", Type: "text", Rect: aijudge.Rect{X: 50, Y: 95, Width: 200, Height: 16}},

		// Last action
		{Text: "Last Action: None", Type: "text", Rect: aijudge.Rect{X: 200, Y: 130, Width: 300, Height: 20}},

		// Buttons (gradient backgrounds — toughest for OCR)
		{Text: "Button Click Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 165, Width: 250, Height: 24}},
		{Text: "PRIMARY", Type: "button", Rect: aijudge.Rect{X: 30, Y: 200, Width: 200, Height: 45}},
		{Text: "SUCCESS", Type: "button", Rect: aijudge.Rect{X: 240, Y: 200, Width: 200, Height: 45}},
		{Text: "WARNING", Type: "button", Rect: aijudge.Rect{X: 450, Y: 200, Width: 200, Height: 45}},
		{Text: "INFO", Type: "button", Rect: aijudge.Rect{X: 660, Y: 200, Width: 200, Height: 45}},

		// Input (white text on dark background)
		{Text: "Input Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 265, Width: 150, Height: 24}},
		{Text: "Text Input:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 300, Width: 80, Height: 20}},
		{Text: "CLEAR", Type: "button", Rect: aijudge.Rect{X: 780, Y: 295, Width: 70, Height: 35}},

		// Checkboxes, radios (white text on dark)
		{Text: "Selection Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 415, Width: 180, Height: 24}},
		{Text: "Option 1", Type: "checkbox", Rect: aijudge.Rect{X: 30, Y: 470, Width: 100, Height: 20}},
		{Text: "Option 2", Type: "checkbox", Rect: aijudge.Rect{X: 140, Y: 470, Width: 100, Height: 20}},
		{Text: "Option 3", Type: "checkbox", Rect: aijudge.Rect{X: 250, Y: 470, Width: 100, Height: 20}},
		{Text: "Choice A", Type: "radio", Rect: aijudge.Rect{X: 30, Y: 530, Width: 100, Height: 20}},
		{Text: "Choice B", Type: "radio", Rect: aijudge.Rect{X: 140, Y: 530, Width: 100, Height: 20}},
		{Text: "Choice C", Type: "radio", Rect: aijudge.Rect{X: 250, Y: 530, Width: 100, Height: 20}},

		// OCR targets (white on dark)
		{Text: "OCR Test Area", Type: "heading", Rect: aijudge.Rect{X: 50, Y: 950, Width: 200, Height: 28}},
		{Text: "Hello World", Type: "text", Rect: aijudge.Rect{X: 50, Y: 985, Width: 120, Height: 20}},
		{Text: "Ghost MCP Automation", Type: "text", Rect: aijudge.Rect{X: 50, Y: 1010, Width: 200, Height: 20}},
	},
}
