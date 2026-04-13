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

// FixtureNormal defines the expected elements on index.html (light theme).
// These are manually curated from the HTML source.
var FixtureNormal = GroundTruth{
	Name:  "normal_fixture",
	Image: "fixture_normal.png",
	Elements: []aijudge.JudgedElement{
		// Page title
		{Text: "Ghost MCP Test Fixture", Type: "heading", Rect: aijudge.Rect{X: 200, Y: 10, Width: 500, Height: 40}},
		{Text: "Interactive GUI for testing MCP UI automation tools", Type: "text", Rect: aijudge.Rect{X: 250, Y: 55, Width: 400, Height: 20}},

		// Navigation
		{Text: "Normal page", Type: "link", Rect: aijudge.Rect{X: 380, Y: 80, Width: 80, Height: 16}},
		{Text: "Challenge page", Type: "link", Rect: aijudge.Rect{X: 470, Y: 80, Width: 100, Height: 16}},

		// Status bar
		{Text: "Waiting for interaction...", Type: "text", Rect: aijudge.Rect{X: 50, Y: 105, Width: 200, Height: 16}},

		// Last action display
		{Text: "Last Action: None", Type: "text", Rect: aijudge.Rect{X: 200, Y: 140, Width: 300, Height: 20}},

		// Button panel heading
		{Text: "Button Click Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 175, Width: 200, Height: 24}},

		// Buttons
		{Text: "PRIMARY", Type: "button", Rect: aijudge.Rect{X: 30, Y: 210, Width: 200, Height: 40}},
		{Text: "SUCCESS", Type: "button", Rect: aijudge.Rect{X: 240, Y: 210, Width: 200, Height: 40}},
		{Text: "WARNING", Type: "button", Rect: aijudge.Rect{X: 450, Y: 210, Width: 200, Height: 40}},
		{Text: "INFO", Type: "button", Rect: aijudge.Rect{X: 660, Y: 210, Width: 200, Height: 40}},

		// Input panel
		{Text: "Input Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 270, Width: 150, Height: 24}},
		{Text: "Text Input:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 305, Width: 80, Height: 20}},
		{Text: "CLEAR", Type: "button", Rect: aijudge.Rect{X: 780, Y: 300, Width: 70, Height: 35}},
		{Text: "Text Area:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 345, Width: 80, Height: 20}},

		// Selection panel
		{Text: "Selection Tests", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 420, Width: 180, Height: 24}},
		{Text: "Checkboxes:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 455, Width: 100, Height: 18}},
		{Text: "Option 1", Type: "checkbox", Rect: aijudge.Rect{X: 30, Y: 475, Width: 100, Height: 20}},
		{Text: "Option 2", Type: "checkbox", Rect: aijudge.Rect{X: 140, Y: 475, Width: 100, Height: 20}},
		{Text: "Option 3", Type: "checkbox", Rect: aijudge.Rect{X: 250, Y: 475, Width: 100, Height: 20}},
		{Text: "Radio Buttons:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 510, Width: 110, Height: 18}},
		{Text: "Choice A", Type: "radio", Rect: aijudge.Rect{X: 30, Y: 535, Width: 100, Height: 20}},
		{Text: "Choice B", Type: "radio", Rect: aijudge.Rect{X: 140, Y: 535, Width: 100, Height: 20}},
		{Text: "Choice C", Type: "radio", Rect: aijudge.Rect{X: 250, Y: 535, Width: 100, Height: 20}},

		// Dropdown
		{Text: "Dropdown Test", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 575, Width: 160, Height: 24}},
		{Text: "-- Select an option --", Type: "dropdown", Rect: aijudge.Rect{X: 30, Y: 610, Width: 200, Height: 35}},

		// Slider
		{Text: "Slider Test", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 665, Width: 140, Height: 24}},
		{Text: "50%", Type: "value", Rect: aijudge.Rect{X: 400, Y: 710, Width: 50, Height: 25}},

		// OCR test area
		{Text: "OCR Text Recognition", Type: "heading", Rect: aijudge.Rect{X: 30, Y: 900, Width: 250, Height: 24}},
		{Text: "OCR Test Area", Type: "heading", Rect: aijudge.Rect{X: 50, Y: 960, Width: 200, Height: 28}},
		{Text: "Hello World", Type: "text", Rect: aijudge.Rect{X: 50, Y: 995, Width: 120, Height: 20}},
		{Text: "Ghost MCP Automation", Type: "text", Rect: aijudge.Rect{X: 50, Y: 1020, Width: 200, Height: 20}},
		{Text: "Click Submit to Continue", Type: "text", Rect: aijudge.Rect{X: 50, Y: 1045, Width: 230, Height: 20}},
		{Text: "File", Type: "text", Rect: aijudge.Rect{X: 50, Y: 1075, Width: 30, Height: 18}},
		{Text: "Edit", Type: "text", Rect: aijudge.Rect{X: 100, Y: 1075, Width: 30, Height: 18}},
		{Text: "View", Type: "text", Rect: aijudge.Rect{X: 150, Y: 1075, Width: 30, Height: 18}},
		{Text: "Help", Type: "text", Rect: aijudge.Rect{X: 200, Y: 1075, Width: 30, Height: 18}},
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
