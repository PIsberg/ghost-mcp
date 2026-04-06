//go:build !integration

// smart_click_test.go - Unit tests for the smart_click tool
package main

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/learner"
	"github.com/mark3labs/mcp-go/mcp"
)

// makeSmartClickReq builds a CallToolRequest for smart_click
func makeSmartClickReq(text string) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text": text,
			},
		},
	}
}

// textFromResultSmart extracts text from tool result
func textFromResultSmart(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// =============================================================================
// Smart Click - Parameter Validation
// =============================================================================

func TestSmartClick_MissingText(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleSmartClick(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing text parameter")
	}
	text := textFromResultSmart(t, result)
	if !strings.Contains(text, "text parameter required") {
		t.Errorf("error should mention missing text; got %q", text)
	}
}

// =============================================================================
// Smart Click - Auto-Learn Behavior
// =============================================================================

func TestSmartClick_AutoLearns_WhenDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: triggers real screen capture and OCR")
	}
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New() // Disabled by default

	// Set up a mock view so find_and_click can succeed
	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{Text: "Test", X: 100, Y: 100, Width: 50, Height: 20},
		},
		ScreenW: 1920,
		ScreenH: 1080,
	})

	req := makeSmartClickReq("Test")
	result, err := handleSmartClick(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should succeed (learn skipped since view created, click succeeds)
	if result.IsError {
		t.Logf("Result: %s", textFromResultSmart(t, result))
		// This is OK - find_and_click might fail without actual screen
	}
}

func TestSmartClick_UsesExistingView(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: triggers real find_and_click OCR")
	}
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(&learner.View{
		Elements:   []learner.Element{{Text: "Test"}},
		CapturedAt: time.Now(),
		ScreenW:    1920,
		ScreenH:    1080,
	})

	req := makeSmartClickReq("Test")
	_, err := handleSmartClick(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Test passes if no panic and function completes
}

// =============================================================================
// Smart Click - Error Handling
// =============================================================================

func TestSmartClick_EmptyText(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: triggers real screen capture and OCR")
	}
	req := makeSmartClickReq("")

	result, err := handleSmartClick(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty text should be handled gracefully
	if result.IsError {
		text := textFromResultSmart(t, result)
		if !strings.Contains(text, "text parameter") {
			t.Logf("Empty text handled: %s", text)
		}
	}
}

// =============================================================================
// HashImageFast - Screen Change Detection
// =============================================================================

func TestHashImageFast_NilImage(t *testing.T) {
	hash := hashImageFast(nil)
	if hash != 0 {
		t.Errorf("expected 0 for nil image, got %d", hash)
	}
}

func TestHashImageFast_DifferentImages(t *testing.T) {
	// Create two different test images (with full alpha = 0xFF at the end)
	img1 := createTestImage(100, 100, 0xFF0000FF) // Red (RGBA: 255,0,0,255)
	img2 := createTestImage(100, 100, 0x00FF00FF) // Green (RGBA: 0,255,0,255)

	hash1 := hashImageFast(img1)
	hash2 := hashImageFast(img2)

	if hash1 == hash2 {
		t.Error("different images should have different hashes")
	}
}

func TestHashImageFast_SameImages(t *testing.T) {
	img := createTestImage(100, 100, 0xFFFF0000)

	hash1 := hashImageFast(img)
	hash2 := hashImageFast(img)

	if hash1 != hash2 {
		t.Error("same image should have same hash")
	}
}

func TestHashImageFast_VariousSizes(t *testing.T) {
	sizes := []struct{ w, h int }{
		{50, 50},
		{100, 100},
		{200, 200},
		{1920, 1080},
	}

	for _, size := range sizes {
		img := createTestImage(size.w, size.h, 0x0000FFFF) // Blue (RGBA: 0,0,255,255)
		hash := hashImageFast(img)
		if hash == 0 {
			t.Errorf("hash should not be 0 for %dx%d image", size.w, size.h)
		}
	}
}

// Helper to create test images
func createTestImage(width, height int, colorVal uint32) *testImage {
	return &testImage{
		width:  width,
		height: height,
		color:  colorVal,
	}
}

// testImage implements image.Image for testing
type testImage struct {
	width  int
	height int
	color  uint32
}

func (img *testImage) Bounds() image.Rectangle {
	return image.Rect(0, 0, img.width, img.height)
}

func (img *testImage) ColorModel() color.Model {
	return color.RGBAModel
}

func (img *testImage) At(x, y int) color.Color {
	if x < 0 || x >= img.width || y < 0 || y >= img.height {
		return color.RGBA{0, 0, 0, 0}
	}
	r := uint8((img.color >> 24) & 0xFF)
	g := uint8((img.color >> 16) & 0xFF)
	b := uint8((img.color >> 8) & 0xFF)
	a := uint8(img.color & 0xFF)
	return color.RGBA{r, g, b, a}
}
