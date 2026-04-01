//go:build !integration

// handler_ocr_test.go - Unit tests for OCR handler functions
package main

import (
	"context"
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/ocr"
	"github.com/mark3labs/mcp-go/mcp"
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

func TestFindButtonBounds_MultiWordPhraseSearch(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Type", X: 100, Y: 50, Width: 40, Height: 24, Confidence: 95},
			{Text: "here", X: 145, Y: 50, Width: 42, Height: 24, Confidence: 95},
			{Text: "or", X: 192, Y: 50, Width: 18, Height: 24, Confidence: 95},
			{Text: "use", X: 215, Y: 50, Width: 34, Height: 24, Confidence: 95},
		},
	}

	minX, minY, maxX, maxY, found := findButtonBounds(result, "Type here or use", 1)
	if !found {
		t.Fatal("Expected to find multi-word phrase across adjacent OCR words")
	}
	if minX != 100 || minY != 50 || maxX != 249 || maxY != 74 {
		t.Fatalf("Expected merged bounds (100,50)-(249,74), got (%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
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

func TestClosestOCRMatches_PrioritizesNearbyPhrases(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Type", X: 100, Y: 50, Width: 40, Height: 24, Confidence: 95},
			{Text: "here", X: 145, Y: 50, Width: 42, Height: 24, Confidence: 95},
			{Text: "localhost:8765", X: 100, Y: 100, Width: 110, Height: 24, Confidence: 95},
			{Text: "Clear", X: 220, Y: 100, Width: 38, Height: 24, Confidence: 95},
		},
	}

	got := closestOCRMatches(result, "Type here or use", 3)
	if len(got) == 0 {
		t.Fatal("expected closest OCR matches")
	}
	if got[0] != "Type here" {
		t.Fatalf("first closest match = %q, want %q", got[0], "Type here")
	}
}

func TestFormatFindTextFailureMessage_IncludesCandidatesAndRegion(t *testing.T) {
	msg := formatFindTextFailureMessage("Type here or use", 1, 10, 20, 300, 100, []string{"Type here", "localhost:8765"})

	if !strings.Contains(msg, `Search region: x=10 y=20 width=300 height=100`) {
		t.Fatalf("expected region details in failure message: %s", msg)
	}
	if !strings.Contains(msg, `Closest OCR matches`) {
		t.Fatalf("expected closest OCR matches in failure message: %s", msg)
	}
	if !strings.Contains(msg, `scroll_until_text`) {
		t.Fatalf("expected scroll_until_text guidance in failure message: %s", msg)
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

// TestHandleFindClickAndTypeMissingText tests find_click_and_type with missing text parameter
func TestHandleFindClickAndTypeMissingText(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"type_text": "hello",
			},
		},
	}
	ctx := context.TODO()
	result, err := handleFindClickAndType(ctx, request)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected error result for missing text")
	}
}

// TestHandleFindClickAndTypeMissingTypeText tests find_click_and_type with missing type_text parameter
func TestHandleFindClickAndTypeMissingTypeText(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text": "Login",
			},
		},
	}
	ctx := context.TODO()
	result, err := handleFindClickAndType(ctx, request)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected error result for missing type_text")
	}
}

func TestParallelFindText_PreparesVariantsOnceAndUsesPreparedBytes(t *testing.T) {
	originalPrepare := prepareParallelOCRImageSet
	originalRead := readPreparedOCRImage
	t.Cleanup(func() {
		prepareParallelOCRImageSet = originalPrepare
		readPreparedOCRImage = originalRead
	})

	var prepareCalls int
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	prepared := &ocr.PreparedImageSet{
		Normal:     []byte("normal"),
		Inverted:   []byte("inverted"),
		BrightText: []byte("bright"),
		Color:      []byte("color"),
	}
	prepareParallelOCRImageSet = func(got image.Image, grayscale bool) (*ocr.PreparedImageSet, error) {
		prepareCalls++
		if got != img {
			t.Fatalf("prepare called with unexpected image pointer")
		}
		if !grayscale {
			t.Fatalf("expected grayscale=true")
		}
		return prepared, nil
	}

	seen := make(chan string, 4)
	readPreparedOCRImage = func(imgBytes []byte, scaleFactor int) (*ocr.Result, error) {
		seen <- string(imgBytes)
		if scaleFactor != ocr.ScaleFactor {
			t.Fatalf("scaleFactor = %d, want %d", scaleFactor, ocr.ScaleFactor)
		}
		if string(imgBytes) == "color" {
			return &ocr.Result{Words: []ocr.Word{{Text: "Submit", X: 10, Y: 10, Width: 40, Height: 20, Confidence: 99}}}, nil
		}
		return &ocr.Result{}, nil
	}

	_, _, _, _, found, pass := parallelFindText(context.Background(), img, "Submit", 1, true)
	if !found {
		t.Fatal("expected text to be found")
	}
	if pass != "color" {
		t.Fatalf("pass = %q, want color", pass)
	}
	if prepareCalls != 1 {
		t.Fatalf("prepareCalls = %d, want 1", prepareCalls)
	}

	// parallelFindText returns as soon as the first match is found, but goroutines
	// that passed the ctx.Done() check before cancellation may still be executing
	// their readPreparedOCRImage call. Give them time to finish before draining
	// the buffered channel — closing it while they are still writing would panic.
	time.Sleep(10 * time.Millisecond)
	got := make(map[string]bool)
loop:
	for {
		select {
		case name := <-seen:
			got[name] = true
		default:
			break loop
		}
	}
	if !got["normal"] || !got["inverted"] || !got["bright"] || !got["color"] {
		t.Fatalf("prepared bytes used = %v", got)
	}
}

func TestWaitForTextPollInterval_Is100ms(t *testing.T) {
	if waitForTextPollInterval != 100*time.Millisecond {
		t.Fatalf("waitForTextPollInterval = %v, want 100ms", waitForTextPollInterval)
	}
}

func TestWaitForTextInitialDelay_Is200ms(t *testing.T) {
	if waitForTextInitialDelay != 200*time.Millisecond {
		t.Fatalf("waitForTextInitialDelay = %v, want 200ms", waitForTextInitialDelay)
	}
}

func TestHandleWaitForText_UsesConfiguredPollInterval(t *testing.T) {
	originalCapture := waitForTextCaptureImage
	originalPrepare := prepareParallelOCRImageSet
	originalReadPrepared := readPreparedOCRImage
	originalSleep := waitForTextSleep
	t.Cleanup(func() {
		waitForTextCaptureImage = originalCapture
		prepareParallelOCRImageSet = originalPrepare
		readPreparedOCRImage = originalReadPrepared
		waitForTextSleep = originalSleep
	})

	waitForTextCaptureImage = func(x, y, width, height int) (image.Image, error) {
		return image.NewRGBA(image.Rect(0, 0, 2, 2)), nil
	}
	prepareParallelOCRImageSet = func(image.Image, bool) (*ocr.PreparedImageSet, error) {
		return &ocr.PreparedImageSet{
			Normal:     []byte("normal"),
			Inverted:   []byte("inverted"),
			BrightText: []byte("bright"),
			Color:      []byte("color"),
		}, nil
	}
	readPreparedOCRImage = func([]byte, int) (*ocr.Result, error) {
		return &ocr.Result{}, nil
	}

	var slept []time.Duration
	waitForTextSleep = func(d time.Duration) {
		slept = append(slept, d)
		time.Sleep(5 * time.Millisecond)
	}

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text":       "NeverAppears",
				"timeout_ms": float64(210),
			},
		},
	}

	result, err := handleWaitForText(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected timeout error result")
	}
	if len(slept) == 0 {
		t.Fatal("expected at least one sleep")
	}
	if slept[0] != waitForTextInitialDelay {
		t.Fatalf("first sleep duration = %v, want %v", slept[0], waitForTextInitialDelay)
	}
	for _, d := range slept[1:] {
		if d != waitForTextPollInterval {
			t.Fatalf("poll sleep duration = %v, want %v", d, waitForTextPollInterval)
		}
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "timeout waiting for text") {
		t.Fatalf("unexpected result text: %s", text)
	}
}

func TestParallelFindPreparedText_UsesPreparedBytes(t *testing.T) {
	originalRead := readPreparedOCRImage
	t.Cleanup(func() {
		readPreparedOCRImage = originalRead
	})

	prepared := &ocr.PreparedImageSet{
		Normal:     []byte("normal"),
		Inverted:   []byte("inverted"),
		BrightText: []byte("bright"),
		Color:      []byte("color"),
	}

	seen := make(chan string, 4)
	readPreparedOCRImage = func(imgBytes []byte, scaleFactor int) (*ocr.Result, error) {
		seen <- string(imgBytes)
		if scaleFactor != ocr.ScaleFactor {
			t.Fatalf("scaleFactor = %d, want %d", scaleFactor, ocr.ScaleFactor)
		}
		if string(imgBytes) == "bright" {
			return &ocr.Result{Words: []ocr.Word{{Text: "Submit", X: 10, Y: 10, Width: 40, Height: 20, Confidence: 99}}}, nil
		}
		return &ocr.Result{}, nil
	}

	_, _, _, _, found, pass := parallelFindPreparedText(context.Background(), prepared, "Submit", 1, true)
	if !found {
		t.Fatal("expected text to be found")
	}
	if pass != "bright-text" {
		t.Fatalf("pass = %q, want bright-text", pass)
	}

	time.Sleep(10 * time.Millisecond)
	got := make(map[string]bool)
loop:
	for {
		select {
		case name := <-seen:
			got[name] = true
		default:
			break loop
		}
	}
	if !got["normal"] || !got["inverted"] || !got["bright"] || !got["color"] {
		t.Fatalf("prepared bytes used = %v", got)
	}
}

func TestParallelFindText_PreprocessFailureReturnsNotFound(t *testing.T) {
	originalPrepare := prepareParallelOCRImageSet
	t.Cleanup(func() {
		prepareParallelOCRImageSet = originalPrepare
	})

	prepareParallelOCRImageSet = func(image.Image, bool) (*ocr.PreparedImageSet, error) {
		return nil, fmt.Errorf("boom")
	}

	_, _, _, _, found, pass := parallelFindText(context.Background(), image.NewRGBA(image.Rect(0, 0, 1, 1)), "Submit", 1, true)
	if found {
		t.Fatal("expected not found when preprocessing fails")
	}
	if pass != "" {
		t.Fatalf("pass = %q, want empty", pass)
	}
}
