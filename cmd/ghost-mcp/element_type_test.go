package main

import (
	"testing"

	"github.com/ghost-mcp/internal/learner"
	"github.com/ghost-mcp/internal/ocr"
)

// TestIsValidElementType tests the element type validation function
func TestIsValidElementType(t *testing.T) {
	validTypes := []string{
		"button", "input", "checkbox", "radio", "dropdown",
		"toggle", "slider", "label", "heading", "link", "value", "text",
	}

	for _, typ := range validTypes {
		if !isValidElementType(typ) {
			t.Errorf("isValidElementType(%q) = false, want true", typ)
		}
	}

	invalidTypes := []string{
		"invalid", "BUTTON", "Input", "", "checkboxes", "btn",
	}

	for _, typ := range invalidTypes {
		if isValidElementType(typ) {
			t.Errorf("isValidElementType(%q) = true, want false", typ)
		}
	}
}

// TestParseElementType tests the element type parsing function
func TestParseElementType(t *testing.T) {
	tests := []struct {
		input    string
		expected learner.ElementType
	}{
		{"button", learner.ElementTypeButton},
		{"input", learner.ElementTypeInput},
		{"checkbox", learner.ElementTypeCheckbox},
		{"radio", learner.ElementTypeRadio},
		{"dropdown", learner.ElementTypeDropdown},
		{"toggle", learner.ElementTypeToggle},
		{"slider", learner.ElementTypeSlider},
		{"label", learner.ElementTypeLabel},
		{"heading", learner.ElementTypeHeading},
		{"link", learner.ElementTypeLink},
		{"value", learner.ElementTypeValue},
		{"text", learner.ElementTypeText},
		{"invalid", learner.ElementTypeUnknown},
		{"", learner.ElementTypeUnknown},
	}

	for _, tt := range tests {
		result := parseElementType(tt.input)
		if result != tt.expected {
			t.Errorf("parseElementType(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestMatchesElementType tests the element type filtering logic
func TestMatchesElementType(t *testing.T) {
	// Create test words with different characteristics
	tests := []struct {
		name          string
		wordText      string
		wordWidth     int
		wordHeight    int
		filterType    string
		shouldMatch   bool
	}{
		{
			name:        "button matches button filter",
			wordText:    "Submit",
			wordWidth:   80,
			wordHeight:  30,
			filterType:  "button",
			shouldMatch: true,
		},
		{
			name:        "label matches label filter",
			wordText:    "Email:",
			wordWidth:   50,
			wordHeight:  20,
			filterType:  "label",
			shouldMatch: true,
		},
		{
			name:        "button does not match label filter",
			wordText:    "Submit",
			wordWidth:   80,
			wordHeight:  30,
			filterType:  "label",
			shouldMatch: false,
		},
		{
			name:        "empty filter matches everything",
			wordText:    "Submit",
			wordWidth:   80,
			wordHeight:  30,
			filterType:  "",
			shouldMatch: true,
		},
		{
			name:        "heading matches heading filter",
			wordText:    "Welcome to Our Site",
			wordWidth:   200,
			wordHeight:  40,
			filterType:  "heading",
			shouldMatch: true,
		},
		{
			name:        "value matches value filter",
			wordText:    "$99.99",
			wordWidth:   60,
			wordHeight:  20,
			filterType:  "value",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			word := ocr.Word{
				Text:       tt.wordText,
				Width:      tt.wordWidth,
				Height:     tt.wordHeight,
				Confidence: 90.0,
			}

			result := matchesElementType(word, tt.filterType)
			if result != tt.shouldMatch {
				t.Errorf("matchesElementType(word, %q) = %v, want %v",
					tt.filterType, result, tt.shouldMatch)
			}
		})
	}
}
