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

	"github.com/otiai10/gosseract/v2"
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
// Constants tests
// =============================================================================

// TestPageSegMode_SparseText documents why PSM_SPARSE_TEXT is used for UI
// screenshots. UI screens contain scattered labels, buttons, and menu items
// rather than structured prose, so Tesseract's default PSM_AUTO (which assumes
// document-style columns and paragraphs) misses or misplaces UI text.
// PSM_SPARSE_TEXT (11) finds text wherever it appears with no layout assumptions.
func TestPageSegMode_SparseText(t *testing.T) {
	// PSM_SPARSE_TEXT == 11 per gosseract's PageSegMode iota.
	// We assert on the integer value so a future gosseract upgrade that shifts
	// the iota would be caught here.
	const wantPSM = 11
	if int(gosseract.PSM_SPARSE_TEXT) != wantPSM {
		t.Errorf("gosseract.PSM_SPARSE_TEXT = %d; want %d — iota may have shifted", int(gosseract.PSM_SPARSE_TEXT), wantPSM)
	}
}

// TestScaleFactor_AtLeastThree ensures the scale factor is high enough to
// bring a 96 DPI screen capture into Tesseract's optimal ~288–300 DPI range.
// Lowering this below 3 degrades recognition of short UI text (button labels,
// menu items). See: https://tesseract-ocr.github.io/tessdoc/ImproveQuality
func TestScaleFactor_AtLeastThree(t *testing.T) {
	if ScaleFactor < 3 {
		t.Errorf("ScaleFactor = %d; want >= 3 for reliable UI text recognition (96 DPI × 3 ≈ 288 DPI, Tesseract's optimal range)", ScaleFactor)
	}
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
