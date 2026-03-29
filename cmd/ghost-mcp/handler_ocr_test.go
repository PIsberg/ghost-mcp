//go:build !integration

// handler_ocr_test.go - Unit tests for OCR handler functions
package main

import (
	"testing"

	"github.com/ghost-mcp/internal/ocr"
)

// =============================================================================
// FIND_BUTTON_BOUNDS TESTS
// =============================================================================

// TestFindButtonBounds_SingleWord tests finding a single-word button
func TestFindButtonBounds_SingleWord(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Save", X: 100, Y: 50, Width: 60, Height: 30, Confidence: 95},
			{Text: "Cancel", X: 200, Y: 50, Width: 70, Height: 30, Confidence: 95},
		},
	}

	minX, minY, maxX, maxY, found := findButtonBounds(result, "Save", 1)
	if !found {
		t.Fatal("Expected to find 'Save' button")
	}
	if minX != 100 || minY != 50 || maxX != 160 || maxY != 80 {
		t.Errorf("Expected bounds (100,50)-(160,80), got (%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

// TestFindButtonBounds_MultiWord tests finding a multi-word button
func TestFindButtonBounds_MultiWord(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Save", X: 100, Y: 50, Width: 60, Height: 30, Confidence: 95},
			{Text: "Changes", X: 165, Y: 50, Width: 80, Height: 30, Confidence: 95},
			{Text: "Cancel", X: 300, Y: 50, Width: 70, Height: 30, Confidence: 95},
		},
	}

	minX, minY, maxX, maxY, found := findButtonBounds(result, "Save", 1)
	if !found {
		t.Fatal("Expected to find 'Save Changes' button")
	}
	// Should merge "Save" and "Changes" into one bounding box
	// Gap between "Save" (ends at 160) and "Changes" (starts at 165) is 5px
	// maxHGap = 60/2 = 30, so 5px gap should merge
	if minX != 100 || maxX != 245 {
		t.Errorf("Expected merged X bounds 100-245, got %d-%d", minX, maxX)
	}
	if minY != 50 || maxY != 80 {
		t.Errorf("Expected Y bounds 50-80, got %d-%d", minY, maxY)
	}
	
	// "Cancel" should NOT be merged (gap from 245 to 300 = 55px > maxHGap of 30)
	// Verify by searching for "Cancel" separately
	minX2, minY2, maxX2, maxY2, found2 := findButtonBounds(result, "Cancel", 1)
	if !found2 {
		t.Fatal("Expected to find 'Cancel' button separately")
	}
	if minX2 != 300 || maxX2 != 370 {
		t.Errorf("Expected Cancel bounds 300-370, got %d-%d", minX2, maxX2)
	}
	if minY2 != 50 || maxY2 != 80 {
		t.Errorf("Expected Cancel Y bounds 50-80, got %d-%d", minY2, maxY2)
	}
}

// TestFindButtonBounds_NthOccurrence tests finding the nth occurrence of a button
func TestFindButtonBounds_NthOccurrence(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Delete", X: 100, Y: 50, Width: 70, Height: 30, Confidence: 95},
			{Text: "Delete", X: 100, Y: 150, Width: 70, Height: 30, Confidence: 95},
			{Text: "Delete", X: 100, Y: 250, Width: 70, Height: 30, Confidence: 95},
		},
	}

	// Find 2nd occurrence
	minX, minY, maxX, maxY, found := findButtonBounds(result, "Delete", 2)
	if !found {
		t.Fatal("Expected to find 2nd 'Delete' button")
	}
	if minX != 100 || minY != 150 || maxX != 170 || maxY != 180 {
		t.Errorf("Expected bounds (100,150)-(170,180), got (%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

// TestFindButtonBounds_NotFound tests when button text is not present
func TestFindButtonBounds_NotFound(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Save", X: 100, Y: 50, Width: 60, Height: 30, Confidence: 95},
			{Text: "Cancel", X: 200, Y: 50, Width: 70, Height: 30, Confidence: 95},
		},
	}

	_, _, _, _, found := findButtonBounds(result, "Submit", 1)
	if found {
		t.Error("Expected not to find 'Submit' button")
	}
}

// TestFindButtonBounds_CaseInsensitive tests case-insensitive matching
func TestFindButtonBounds_CaseInsensitive(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "SAVE", X: 100, Y: 50, Width: 60, Height: 30, Confidence: 95},
		},
	}

	minX, minY, maxX, maxY, found := findButtonBounds(result, "save", 1)
	if !found {
		t.Fatal("Expected to find 'SAVE' with lowercase search")
	}
	if minX != 100 || minY != 50 || maxX != 160 || maxY != 80 {
		t.Errorf("Expected bounds (100,50)-(160,80), got (%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

// TestFindButtonBounds_PartialMatch tests partial text matching
func TestFindButtonBounds_PartialMatch(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Submitting...", X: 100, Y: 50, Width: 100, Height: 30, Confidence: 95},
		},
	}

	minX, minY, maxX, maxY, found := findButtonBounds(result, "Submit", 1)
	if !found {
		t.Fatal("Expected to find 'Submitting...' with partial match 'Submit'")
	}
	if minX != 100 || minY != 50 || maxX != 200 || maxY != 80 {
		t.Errorf("Expected bounds (100,50)-(200,80), got (%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

// TestFindButtonBounds_FixtureButtons tests the fixture button layout where
// Primary, Success, Warning, Info buttons are on the same row but separated
func TestFindButtonBounds_FixtureButtons(t *testing.T) {
	// Simulating fixture layout: buttons spaced ~130px apart
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Primary", X: 100, Y: 200, Width: 80, Height: 35, Confidence: 90},
			{Text: "Success", X: 230, Y: 200, Width: 80, Height: 35, Confidence: 90},
			{Text: "Warning", X: 360, Y: 200, Width: 80, Height: 35, Confidence: 90},
			{Text: "Info", X: 490, Y: 200, Width: 60, Height: 35, Confidence: 90},
		},
	}

	// Each button should be found separately
	tests := []struct {
		text       string
		expectX    int
		expectMaxX int
	}{
		{"Primary", 100, 180},
		{"Success", 230, 310},
		{"Warning", 360, 440},
		{"Info", 490, 550},
	}

	for _, tt := range tests {
		minX, minY, maxX, maxY, found := findButtonBounds(result, tt.text, 1)
		if !found {
			t.Errorf("Expected to find '%s' button", tt.text)
			continue
		}
		if minX != tt.expectX {
			t.Errorf("%s: expected minX=%d, got %d", tt.text, tt.expectX, minX)
		}
		if maxX != tt.expectMaxX {
			t.Errorf("%s: expected maxX=%d, got %d", tt.text, tt.expectMaxX, maxX)
		}
		if minY != 200 || maxY != 235 {
			t.Errorf("%s: expected Y bounds 200-235, got %d-%d", tt.text, minY, maxY)
		}
	}
}

// TestAbs tests the abs helper function
func TestAbs(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{-5, 5},
		{0, 0},
		{5, 5},
		{-100, 100},
	}

	for _, tt := range tests {
		result := abs(tt.input)
		if result != tt.expected {
			t.Errorf("abs(%d) = %d, expected %d", tt.input, result, tt.expected)
		}
	}
}
