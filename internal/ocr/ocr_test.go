// ocr_test.go — Tests for the OCR package.
package ocr

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePNG creates a temporary PNG file and returns its path.
func writePNG(t *testing.T, img image.Image) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "ocr-test-*.png")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return f.Name()
}

// whiteImage returns a plain white image of the given dimensions.
func whiteImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	return img
}

// =============================================================================
// Error handling tests
// =============================================================================

func TestReadFile_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.png")
	_, err := ReadFile(path)
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

func TestReadFile_NotAPNG(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "bad-*.png")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	f.WriteString("not a valid image file")
	f.Close()

	_, err = ReadFile(f.Name())
	if err == nil {
		t.Error("Expected error for invalid image, got nil")
	}
}

// =============================================================================
// Result structure tests (require Tesseract)
// =============================================================================

func skipIfNoTesseract(t *testing.T) {
	t.Helper()
	// Try a minimal OCR call; if Tesseract data files are missing it errors.
	img := whiteImage(100, 100)
	path := writePNG(t, img)
	_, err := ReadFile(path)
	if err != nil && strings.Contains(err.Error(), "tessdata") {
		t.Skip("Tesseract data files not available")
	}
}

func TestReadFile_WhiteImage_ReturnsEmptyText(t *testing.T) {
	skipIfNoTesseract(t)

	img := whiteImage(200, 100)
	path := writePNG(t, img)

	result, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	// A blank white image should yield no words
	if len(result.Words) != 0 {
		t.Errorf("Expected no words from white image, got %d", len(result.Words))
	}
}

func TestReadFile_ResultFields(t *testing.T) {
	skipIfNoTesseract(t)

	img := whiteImage(200, 100)
	path := writePNG(t, img)

	result, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Text field should be a string (may be empty or whitespace for a blank image)
	_ = result.Text

	// Words slice should be initialised (not nil — it's make([]Word, 0, ...))
	if result.Words == nil {
		t.Error("Expected non-nil Words slice")
	}
}

func TestReadFile_WordFields(t *testing.T) {
	skipIfNoTesseract(t)

	// We can only verify field types since real text rendering requires fonts/drawing.
	// This test documents the expected structure and verifies the zero value is sane.
	w := Word{
		Text:       "hello",
		X:          10,
		Y:          20,
		Width:      30,
		Height:     15,
		Confidence: 98.5,
	}
	if w.Text != "hello" {
		t.Errorf("Expected 'hello', got %q", w.Text)
	}
	if w.X != 10 || w.Y != 20 {
		t.Errorf("Unexpected position: (%d, %d)", w.X, w.Y)
	}
	if w.Width != 30 || w.Height != 15 {
		t.Errorf("Unexpected size: %dx%d", w.Width, w.Height)
	}
	if w.Confidence != 98.5 {
		t.Errorf("Unexpected confidence: %f", w.Confidence)
	}
}
