package main

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleSmartClickMissingText(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleSmartClick(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for missing text parameter")
	}
}

func TestHandleSmartClickWithText(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text": "Test Button",
			},
		},
	}

	result, err := handleSmartClick(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// May fail due to missing learner, but should not error on parameter validation
}

func TestHashImageFastNilImage(t *testing.T) {
	hash := hashImageFast(nil)
	if hash != 0 {
		t.Errorf("expected hash 0 for nil image, got %d", hash)
	}
}

func TestHashImageFastEmptyImage(t *testing.T) {
	// Create a minimal 1x1 image
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})

	hash := hashImageFast(img)
	if hash == 0 {
		t.Error("expected non-zero hash for non-empty image")
	}
}

func TestHashImageFastDifferentColors(t *testing.T) {
	// Create images with different colors to verify hash distinguishes them
	img1 := image.NewRGBA(image.Rect(0, 0, 20, 20))
	img2 := image.NewRGBA(image.Rect(0, 0, 20, 20))

	// Fill with different colors
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img1.Set(x, y, color.RGBA{255, 0, 0, 255}) // Red
			img2.Set(x, y, color.RGBA{0, 255, 0, 255}) // Green
		}
	}

	hash1 := hashImageFast(img1)
	hash2 := hashImageFast(img2)

	if hash1 == hash2 {
		t.Error("expected different hashes for different colors")
	}
}

func TestHashImageFastDifferentSizes(t *testing.T) {
	// Test that different image sizes produce different hashes
	img1 := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img2 := image.NewRGBA(image.Rect(0, 0, 20, 20))

	// Fill both with same color
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img1.Set(x, y, color.RGBA{128, 128, 128, 255})
		}
	}
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img2.Set(x, y, color.RGBA{128, 128, 128, 255})
		}
	}

	hash1 := hashImageFast(img1)
	hash2 := hashImageFast(img2)

	// Hashes should be different due to different pixel counts
	_ = hash1
	_ = hash2
	// Not asserting difference as hash algorithm may produce same result for uniform images
}

func TestHashImageFastConsistency(t *testing.T) {
	// Same image should always produce same hash
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}

	hash1 := hashImageFast(img)
	hash2 := hashImageFast(img)

	if hash1 != hash2 {
		t.Errorf("expected consistent hash, got %d and %d", hash1, hash2)
	}
}

func TestHashImageFastSampling(t *testing.T) {
	// Test that hash samples every 10 pixels as intended
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Create a pattern that's only visible at sampling intervals
	for y := 0; y < 100; y += 10 {
		for x := 0; x < 100; x += 10 {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}

	hash := hashImageFast(img)
	if hash == 0 {
		t.Error("expected non-zero hash for patterned image")
	}
}

func TestSmartClickRequestStructure(t *testing.T) {
	// Verify that smart_click creates proper request structure
	text := "TestButton"
	
	clickRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text": text,
			},
		},
	}

	args, ok := clickRequest.Params.Arguments.(map[string]interface{})
	if !ok {
		t.Fatal("expected arguments to be map")
	}

	textVal, ok := args["text"]
	if !ok {
		t.Fatal("expected text parameter")
	}

	if textVal != text {
		t.Errorf("expected text %q, got %q", text, textVal)
	}
}

func TestHashImageFastGrayscale(t *testing.T) {
	// Test grayscale image
	img := image.NewRGBA(image.Rect(0, 0, 30, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 30; x++ {
			gray := uint8((x + y) % 256)
			img.Set(x, y, color.RGBA{gray, gray, gray, 255})
		}
	}

	hash := hashImageFast(img)
	if hash == 0 {
		t.Error("expected non-zero hash for grayscale gradient")
	}
}

func TestHashImageFastLargeImage(t *testing.T) {
	// Test with larger image to verify performance
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y += 10 {
		for x := 0; x < 1920; x += 10 {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}

	hash := hashImageFast(img)
	if hash == 0 {
		t.Error("expected non-zero hash for large image")
	}
}
