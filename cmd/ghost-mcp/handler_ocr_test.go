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
	if minX != 100 || maxX != 245 {
		t.Errorf("Expected merged X bounds 100-245, got %d-%d", minX, maxX)
	}
	if minY != 50 || maxY != 80 {
		t.Errorf("Expected Y bounds 50-80, got %d-%d", minY, maxY)
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
