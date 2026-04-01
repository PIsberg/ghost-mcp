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

	minX, minY, maxX, _, found := findButtonBounds(result, "Save", 1)
	if !found {
		t.Fatal("Expected to find 'Save' button")
	}
	// With smart matching, adjacent words may merge if within gap threshold
	// Just verify we found the right word and bounds are reasonable
	if minX != 100 || minY != 50 {
		t.Errorf("Expected min bounds (100,50), got (%d,%d)", minX, minY)
	}
	if maxX < 160 || maxX > 300 {
		t.Errorf("Expected maxX between 160-300, got %d", maxX)
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

	minX, minY, maxX, _, found := findButtonBounds(result, "Save", 1)
	if !found {
		t.Fatal("Expected to find 'Save Changes' button")
	}
	// Should merge "Save" and "Changes" into one bounding box
	// With smart matching (maxHGap = avgWidth * 2), 5px gap easily merges
	if minX != 100 {
		t.Errorf("Expected merged minX 100, got %d", minX)
	}
	if maxX < 245 || maxX > 370 {
		t.Errorf("Expected merged maxX around 245-370, got %d", maxX)
	}
	if minY != 50 {
		t.Errorf("Expected Y bounds 50, got %d", minY)
	}

	// "Cancel" should be found separately
	minX2, _, maxX2, _, found2 := findButtonBounds(result, "Cancel", 1)
	if !found2 {
		t.Fatal("Expected to find 'Cancel' button separately")
	}
	if minX2 != 300 || maxX2 != 370 {
		t.Errorf("Expected Cancel bounds 300-370, got %d-%d", minX2, maxX2)
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

	// Each button should be found - with smart matching, adjacent buttons may merge
	// if within the horizontal gap threshold (maxHGap = avgWidth * 2)
	tests := []struct {
		text       string
		expectMinX int
		expectMaxX int
	}{
		{"Primary", 100, 180}, // May merge right to ~550 with adjacent buttons
		{"Success", 230, 310}, // May merge right to ~550
		{"Warning", 360, 440}, // May merge right to ~550
		{"Info", 490, 550},    // Rightmost button, should be exact
	}

	for _, tt := range tests {
		minX, minY, maxX, maxY, found := findButtonBounds(result, tt.text, 1)
		if !found {
			t.Errorf("Expected to find '%s' button", tt.text)
			continue
		}
		// With smart matching, buttons may merge if within gap threshold
		// Just verify the button was found and bounds are reasonable
		if minX < tt.expectMinX-50 || minX > tt.expectMinX+50 {
			t.Errorf("%s: expected minX around %d, got %d", tt.text, tt.expectMinX, minX)
		}
		if maxX < tt.expectMaxX || maxX > 600 {
			t.Errorf("%s: expected maxX >= %d and <= 600, got %d", tt.text, tt.expectMaxX, maxX)
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
	// Verify at least the matching variant (color) was used
	// Other variants may not complete due to context cancellation
	if !got["color"] {
		t.Fatalf("expected 'color' variant to be used, got %v", got)
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
	// Verify at least the matching variant (bright) was used
	// Other variants may not complete due to context cancellation
	if !got["bright"] {
		t.Fatalf("expected 'bright' variant to be used, got %v", got)
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

// =============================================================================
// REGION CACHE TESTS
// =============================================================================

// TestRegionCache_BasicPutGet tests basic cache put and get operations
func TestRegionCache_BasicPutGet(t *testing.T) {
	cache := &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}

	// Put an entry
	cache.Put("save", 100, 200, 80, 40, 1920, 1080)

	// Get the entry
	entry, found := cache.Get("save", 1920, 1080)
	if !found {
		t.Fatal("Expected to find cached entry")
	}
	if entry.X != 100 || entry.Y != 200 || entry.Width != 80 || entry.Height != 40 {
		t.Errorf("Expected (100,200) 80x40, got (%d,%d) %dx%d", entry.X, entry.Y, entry.Width, entry.Height)
	}
	if entry.ScreenW != 1920 || entry.ScreenH != 1080 {
		t.Errorf("Expected screen size 1920x1080, got %dx%d", entry.ScreenW, entry.ScreenH)
	}
}

// TestRegionCache_CaseInsensitive tests that cache lookups are case-insensitive
func TestRegionCache_CaseInsensitive(t *testing.T) {
	cache := &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}

	// Put with lowercase
	cache.Put("save", 100, 200, 80, 40, 1920, 1080)

	// Get with different cases
	cases := []string{"SAVE", "Save", "save", "  SAVE  ", "SaVe"}
	for _, c := range cases {
		_, found := cache.Get(c, 1920, 1080)
		if !found {
			t.Errorf("Expected to find entry with key %q", c)
		}
	}
}

// TestRegionCache_ScreenResolutionMismatch tests that cache misses on screen resolution change
func TestRegionCache_ScreenResolutionMismatch(t *testing.T) {
	cache := &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}

	cache.Put("save", 100, 200, 80, 40, 1920, 1080)

	// Try to get with different screen resolution
	_, found := cache.Get("save", 2560, 1440)
	if found {
		t.Error("Expected cache miss due to screen resolution mismatch")
	}
}

// TestRegionCache_Eviction tests LRU eviction when cache is full
func TestRegionCache_Eviction(t *testing.T) {
	cache := &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 3,
		maxAge:  24 * time.Hour,
	}

	// Add 3 entries
	cache.Put("first", 10, 10, 50, 50, 1920, 1080)
	time.Sleep(10 * time.Millisecond)
	cache.Put("second", 20, 20, 50, 50, 1920, 1080)
	time.Sleep(10 * time.Millisecond)
	cache.Put("third", 30, 30, 50, 50, 1920, 1080)

	// Add 4th entry, should evict "first"
	cache.Put("fourth", 40, 40, 50, 50, 1920, 1080)

	if len(cache.entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(cache.entries))
	}

	_, foundFirst := cache.entries["first"]
	_, foundFourth := cache.entries["fourth"]

	if foundFirst {
		t.Error("Expected 'first' to be evicted")
	}
	if !foundFourth {
		t.Error("Expected 'fourth' to exist")
	}
	if cache.stats.Evictions != 1 {
		t.Errorf("Expected 1 eviction, got %d", cache.stats.Evictions)
	}
}

// TestRegionCache_StatsTracking tests that cache statistics are properly tracked
func TestRegionCache_StatsTracking(t *testing.T) {
	cache := &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}

	// Initial stats
	stats := cache.GetStats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("Expected initial stats to be 0, got hits=%d, misses=%d", stats.Hits, stats.Misses)
	}

	// Record hits and misses
	cache.RecordHit()
	cache.RecordHit()
	cache.RecordMiss()

	stats = cache.GetStats()
	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
}

// TestRegionCache_UpdateExisting tests updating an existing entry
func TestRegionCache_UpdateExisting(t *testing.T) {
	cache := &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}

	// Initial put
	cache.Put("save", 100, 200, 80, 40, 1920, 1080)
	time.Sleep(10 * time.Millisecond)

	// Update
	cache.Put("save", 150, 250, 90, 45, 1920, 1080)

	entry, found := cache.Get("save", 1920, 1080)
	if !found {
		t.Fatal("Expected to find updated entry")
	}
	if entry.X != 150 || entry.Y != 250 || entry.Width != 90 || entry.Height != 45 {
		t.Errorf("Expected updated values, got (%d,%d) %dx%d", entry.X, entry.Y, entry.Width, entry.Height)
	}
	if entry.HitCount != 1 {
		t.Errorf("Expected HitCount=1 after update, got %d", entry.HitCount)
	}
}

// TestRegionCache_Clear tests clearing all cache entries
func TestRegionCache_Clear(t *testing.T) {
	cache := &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}

	cache.Put("save", 100, 200, 80, 40, 1920, 1080)
	cache.Put("cancel", 200, 200, 80, 40, 1920, 1080)

	if len(cache.entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(cache.entries))
	}

	cache.Clear()

	if len(cache.entries) != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", len(cache.entries))
	}
}

// TestNormalizeText tests the normalizeText function
func TestNormalizeText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Save", "save"},
		{"SAVE", "save"},
		{"  Save  ", "save"},
		{"Save Changes", "save changes"},
		{"", ""},
	}

	for _, tt := range tests {
		result := normalizeText(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeText(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestHandleGetRegionCacheStats tests the get_region_cache_stats handler
func TestHandleGetRegionCacheStats(t *testing.T) {
	// Reset cache for test
	originalCache := regionCache
	regionCache = &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}
	t.Cleanup(func() {
		regionCache = originalCache
	})

	// Add some data
	regionCache.Put("test", 100, 100, 50, 50, 1920, 1080)
	regionCache.RecordHit()
	regionCache.RecordMiss()

	// Call handler
	result, err := handleGetRegionCacheStats(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Parse result
	if len(result.Content) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("Expected TextContent")
	}

	// Verify it's valid JSON with expected fields
	if !strings.Contains(textContent.Text, "entries") {
		t.Error("Result should contain 'entries' field")
	}
	if !strings.Contains(textContent.Text, "hits") {
		t.Error("Result should contain 'hits' field")
	}
	if !strings.Contains(textContent.Text, "hit_rate") {
		t.Error("Result should contain 'hit_rate' field")
	}
}

// TestHandleClearRegionCache tests the clear_region_cache handler
func TestHandleClearRegionCache(t *testing.T) {
	// Reset cache for test
	originalCache := regionCache
	regionCache = &RegionCache{
		entries: make(map[string]*RegionCacheEntry),
		maxSize: 100,
		maxAge:  24 * time.Hour,
	}
	t.Cleanup(func() {
		regionCache = originalCache
	})

	// Add some data
	regionCache.Put("test", 100, 100, 50, 50, 1920, 1080)

	// Call handler
	result, err := handleClearRegionCache(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Verify cache is cleared
	if len(regionCache.entries) != 0 {
		t.Errorf("Expected cache to be cleared, got %d entries", len(regionCache.entries))
	}

	// Parse result
	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("Expected TextContent")
	}

	if !strings.Contains(textContent.Text, `"success":true`) {
		t.Error("Result should indicate success")
	}
}

// =============================================================================
// SMART MATCHING TESTS
// =============================================================================

// TestScoreMatch_ExactMatch tests exact match scoring
func TestScoreMatch_ExactMatch(t *testing.T) {
	needleWords := []string{"click"}
	score := scoreMatch("click", "click", needleWords)
	if score != 1000 {
		t.Errorf("Exact match should score 1000, got %d", score)
	}
}

// TestScoreMatch_PrefixMatch tests prefix match scoring
func TestScoreMatch_PrefixMatch(t *testing.T) {
	needleWords := []string{"click"}
	score := scoreMatch("click me!", "click", needleWords)
	if score != 500 {
		t.Errorf("Prefix match should score 500, got %d", score)
	}
}

// TestScoreMatch_StandaloneWord tests standalone word scoring
func TestScoreMatch_StandaloneWord(t *testing.T) {
	needleWords := []string{"click"}

	// "button click" ends with "click" so scores as suffix match (400)
	score := scoreMatch("button click", "click", needleWords)
	if score != 400 {
		t.Errorf("Suffix match should score 400, got %d", score)
	}
}

// TestScoreMatch_SubstringInsideWord tests substring inside another word
func TestScoreMatch_SubstringInsideWord(t *testing.T) {
	needleWords := []string{"click"}

	// "button click tests" contains "click" as a standalone word (300)
	score := scoreMatch("button click tests", "click", needleWords)
	if score != 300 {
		t.Errorf("Standalone word in phrase should score 300, got %d", score)
	}
}

// TestScoreMatch_NoMatch tests no match scoring
func TestScoreMatch_NoMatch(t *testing.T) {
	needleWords := []string{"click"}
	score := scoreMatch("submit", "click", needleWords)
	if score != 0 {
		t.Errorf("No match should score 0, got %d", score)
	}
}

// TestFindButtonBounds_PrefersStandaloneWord tests that standalone words are preferred
func TestFindButtonBounds_PrefersStandaloneWord(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Button", X: 100, Y: 50, Width: 60, Height: 30, Confidence: 95},
			{Text: "Click", X: 165, Y: 50, Width: 50, Height: 30, Confidence: 95}, // Part of "Button Click Tests"
			{Text: "Tests", X: 220, Y: 50, Width: 50, Height: 30, Confidence: 95},
			{Text: "Click", X: 100, Y: 200, Width: 60, Height: 40, Confidence: 95}, // Standalone "Click" button
		},
	}

	_, minY, _, _, found := findButtonBounds(result, "Click", 1)
	if !found {
		t.Fatal("Expected to find 'Click' button")
	}

	// Should find the standalone "Click" at (100, 200), not the one in "Button Click Tests"
	if minY != 200 {
		t.Errorf("Expected standalone 'Click' at Y=200, got Y=%d (found 'Button Click Tests' instead)", minY)
	}
}

// TestFindButtonBounds_MultiWordButton tests multi-word button matching
func TestFindButtonBounds_MultiWordButton(t *testing.T) {
	result := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Save", X: 100, Y: 50, Width: 60, Height: 30, Confidence: 95},
			{Text: "Changes", X: 165, Y: 50, Width: 80, Height: 30, Confidence: 95},
		},
	}

	minX, minY, maxX, maxY, found := findButtonBounds(result, "Save Changes", 1)
	if !found {
		t.Fatal("Expected to find 'Save Changes' button")
	}
	if minX != 100 || maxX != 245 {
		t.Errorf("Expected merged bounds (100,50)-(245,80), got (%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

// TestIsWordBoundary tests word boundary detection
func TestIsWordBoundary(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{' ', true},
		{'-', true},
		{'_', true},
		{'.', true},
		{',', true},
		{'!', true},
		{'?', true},
		{':', true},
		{';', true},
		{'a', false},
		{'Z', false},
		{'1', false},
	}

	for _, tt := range tests {
		result := isWordBoundary(tt.char)
		if result != tt.expected {
			t.Errorf("isWordBoundary(%q) = %v, want %v", tt.char, result, tt.expected)
		}
	}
}
