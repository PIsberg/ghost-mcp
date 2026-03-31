// ocr_test.go — Tests for the OCR package.
package ocr

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/otiai10/gosseract/v2"
)

type fakeOCRClient struct {
	id             int
	setImageBytes  [][]byte
	getBoxesCalls  int
	pageSegModes   []gosseract.PageSegMode
	boundingBoxes  []gosseract.BoundingBox
	boundingBoxErr error
}

func (f *fakeOCRClient) SetImage(string) error { return nil }

func (f *fakeOCRClient) SetImageFromBytes(data []byte) error {
	f.setImageBytes = append(f.setImageBytes, append([]byte(nil), data...))
	return nil
}

func (f *fakeOCRClient) SetPageSegMode(mode gosseract.PageSegMode) error {
	f.pageSegModes = append(f.pageSegModes, mode)
	return nil
}

func (f *fakeOCRClient) GetBoundingBoxes(gosseract.PageIteratorLevel) ([]gosseract.BoundingBox, error) {
	f.getBoxesCalls++
	return f.boundingBoxes, f.boundingBoxErr
}

func (f *fakeOCRClient) Close() error { return nil }

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

func TestPrimeClientPool_WarmsFourClients(t *testing.T) {
	originalNewClient := newOCRClient
	originalPool := clientPool
	originalWarmOnce := warmClientOnce
	t.Cleanup(func() {
		newOCRClient = originalNewClient
		clientPool = originalPool
		warmClientOnce = originalWarmOnce
	})

	created := make([]*fakeOCRClient, 0, pooledClientWarmCount)
	newOCRClient = func() ocrClient {
		client := &fakeOCRClient{id: len(created) + 1}
		created = append(created, client)
		return client
	}
	clientPool = sync.Pool{
		New: func() any {
			c := newOCRClient()
			_ = c.SetPageSegMode(gosseract.PSM_SPARSE_TEXT)
			return c
		},
	}
	warmClientOnce = sync.Once{}

	primeClientPool()

	if len(created) != pooledClientWarmCount {
		t.Fatalf("created %d clients, want %d", len(created), pooledClientWarmCount)
	}
	for i, client := range created {
		if len(client.pageSegModes) != 1 || client.pageSegModes[0] != gosseract.PSM_SPARSE_TEXT {
			t.Fatalf("client %d page seg modes = %v, want [PSM_SPARSE_TEXT]", i, client.pageSegModes)
		}
		if client.getBoxesCalls != 1 {
			t.Fatalf("client %d warm calls = %d, want 1", i, client.getBoxesCalls)
		}
		if len(client.setImageBytes) != 1 || len(client.setImageBytes[0]) == 0 {
			t.Fatalf("client %d warm image bytes not set", i)
		}
	}
}

func TestGetPooledClient_ReusesPrimedClient(t *testing.T) {
	originalNewClient := newOCRClient
	originalPool := clientPool
	originalWarmOnce := warmClientOnce
	t.Cleanup(func() {
		newOCRClient = originalNewClient
		clientPool = originalPool
		warmClientOnce = originalWarmOnce
	})

	created := make([]*fakeOCRClient, 0, pooledClientWarmCount)
	newOCRClient = func() ocrClient {
		client := &fakeOCRClient{id: len(created) + 1}
		created = append(created, client)
		return client
	}
	clientPool = sync.Pool{
		New: func() any {
			c := newOCRClient()
			_ = c.SetPageSegMode(gosseract.PSM_SPARSE_TEXT)
			return c
		},
	}
	warmClientOnce = sync.Once{}

	first := getPooledClient()
	putPooledClient(first)
	second := getPooledClient()

	firstFake, ok1 := first.(*fakeOCRClient)
	secondFake, ok2 := second.(*fakeOCRClient)
	if !ok1 || !ok2 {
		t.Fatalf("unexpected client types: %T and %T", first, second)
	}
	if firstFake != secondFake {
		t.Fatalf("expected pooled client reuse, got ids %d and %d", firstFake.id, secondFake.id)
	}
}

// =============================================================================
// Preprocessing tests
// =============================================================================

// TestToGrayscaleContrast_UniformImage checks that a solid-colour image is
// returned as-is (no stretch, to avoid amplifying noise).
func TestToGrayscaleContrast_UniformImage(t *testing.T) {
	// Solid mid-grey — span < 10, so stretch should be skipped.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}
	got := toGrayscaleContrast(img)
	// All pixels should remain near 128 (not stretched to 0 or 255).
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			l := got.GrayAt(x, y).Y
			if l < 120 || l > 136 {
				t.Errorf("pixel (%d,%d): gray = %d; want ~128 (no contrast stretch for uniform image)", x, y, l)
			}
		}
	}
}

// TestToGrayscaleContrast_StretchesContrast verifies that a two-tone image
// (pure black and pure white pixels) has its extremes mapped to 0 and 255.
func TestToGrayscaleContrast_StretchesContrast(t *testing.T) {
	// Left half black, right half white.
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.Black)
		}
		for x := 10; x < 20; x++ {
			img.Set(x, y, color.White)
		}
	}
	got := toGrayscaleContrast(img)
	darkPx := got.GrayAt(5, 5).Y
	lightPx := got.GrayAt(15, 5).Y
	if darkPx != 0 {
		t.Errorf("dark pixel after stretch = %d; want 0", darkPx)
	}
	if lightPx != 255 {
		t.Errorf("light pixel after stretch = %d; want 255", lightPx)
	}
}

// TestToGrayscaleContrast_ColoredBackground simulates a colored button
// (e.g. blue background with white text). After grayscale + contrast stretch
// the text pixels should be significantly brighter than the background.
func TestToGrayscaleContrast_ColoredBackground(t *testing.T) {
	// Blue button background: R=0 G=100 B=200
	// White text pixels: R=255 G=255 B=255
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	blueBtn := color.RGBA{R: 0, G: 100, B: 200, A: 255}
	whiteText := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if x == 5 && y == 5 {
				img.Set(x, y, whiteText) // one "text" pixel
			} else {
				img.Set(x, y, blueBtn)
			}
		}
	}
	got := toGrayscaleContrast(img)
	textPx := got.GrayAt(5, 5).Y
	bgPx := got.GrayAt(0, 0).Y
	if textPx <= bgPx {
		t.Errorf("text pixel (%d) should be brighter than background pixel (%d) after contrast stretch", textPx, bgPx)
	}
	// After full stretch the text pixel (brightest) should map close to 255.
	if textPx < 250 {
		t.Errorf("text pixel = %d; want ~255 (brightest pixel maps to 255 after stretch)", textPx)
	}
}

// The client is now dynamically instantiated for true concurrency.
// TestGetClient_ReturnsSameInstance removed.

// TestInvertGray verifies that invertGray flips every pixel value.
func TestInvertGray(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 3, 1))
	img.Pix[0] = 0
	img.Pix[1] = 128
	img.Pix[2] = 255

	invertGray(img)

	if img.Pix[0] != 255 {
		t.Errorf("Pix[0]: got %d want 255", img.Pix[0])
	}
	if img.Pix[1] != 127 {
		t.Errorf("Pix[1]: got %d want 127", img.Pix[1])
	}
	if img.Pix[2] != 0 {
		t.Errorf("Pix[2]: got %d want 0", img.Pix[2])
	}
}

// TestOptions_Inverted_WhiteTextOnDark simulates the button scenario:
// white text (255) on a dark coloured background (~100). After normal
// grayscale+contrast the text and background are both high-valued on a
// page with a white background — Tesseract can't see it. After inversion
// the text becomes dark (0) on a lighter button background, which is what
// Tesseract is trained on.
func TestOptions_Inverted_WhiteTextOnDark(t *testing.T) {
	// Simulate: mostly-white page (200,200,200), dark button region (80,80,200),
	// white button text (255,255,255).
	img := image.NewRGBA(image.Rect(0, 0, 30, 10))
	pageColor := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	btnColor := color.RGBA{R: 80, G: 80, B: 200, A: 255}
	textColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 30; x++ {
			if x >= 10 && x < 20 {
				if x == 15 {
					img.Set(x, y, textColor) // "text" pixel in button centre
				} else {
					img.Set(x, y, btnColor) // button background
				}
			} else {
				img.Set(x, y, pageColor) // page background
			}
		}
	}

	gray := toGrayscaleContrast(img)

	// Before inversion: text pixel and page background are near 255 → indistinct.
	textBefore := gray.GrayAt(15, 5).Y
	bgBefore := gray.GrayAt(0, 5).Y
	// Both should be high (near 255) — that is the problem we are solving.
	if textBefore < 200 || bgBefore < 200 {
		t.Logf("Before inversion: text=%d, page bg=%d (both should be near 255)", textBefore, bgBefore)
	}

	invertGray(gray)

	// After inversion: text pixel should be near 0 (dark), button background
	// should be lighter than the page background.
	textAfter := gray.GrayAt(15, 5).Y
	btnAfter := gray.GrayAt(12, 5).Y
	if textAfter >= btnAfter {
		t.Errorf("After inversion: text (%d) should be darker than button background (%d)", textAfter, btnAfter)
	}
	if textAfter > 50 {
		t.Errorf("After inversion: text pixel = %d; want near 0 (dark text on lighter background)", textAfter)
	}
}

// TestBrightTextToGray_WhiteOnColored verifies that pure-white pixels become
// black (text detected) and coloured / near-white pixels become white (background).
// This documents the threshold=240 design choice:
//   - Pure white button text (255,255,255) → black ✓
//   - Body text colour #eee (238,238,238) → white (238 < 240, not captured) ✓
//   - Coloured button background, e.g. primary #667eea (102,126,234) → white ✓
//     (R=102 < 240 fails the "all channels ≥ threshold" check)
func TestBrightTextToGray_WhiteOnColored(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	// Primary button blue (#667eea) — coloured background, should become white.
	btnColor := color.RGBA{R: 102, G: 126, B: 234, A: 255}
	// Pure white — button label text, should become black.
	whiteText := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	// Near-white body text (#eee = 238,238,238) — must NOT be captured (238 < 240).
	bodyText := color.RGBA{R: 238, G: 238, B: 238, A: 255}

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			switch {
			case x == 5 && y == 5:
				img.Set(x, y, whiteText)
			case x == 0 && y == 0:
				img.Set(x, y, bodyText)
			default:
				img.Set(x, y, btnColor)
			}
		}
	}

	got := brightTextToGray(img, 240)

	// Pure white text pixel → black (0 = text detected).
	if got.GrayAt(5, 5).Y != 0 {
		t.Errorf("white text pixel: gray=%d; want 0 (black)", got.GrayAt(5, 5).Y)
	}
	// Coloured button background → white (255 = background, not captured).
	if got.GrayAt(1, 1).Y != 255 {
		t.Errorf("blue background pixel: gray=%d; want 255 (white)", got.GrayAt(1, 1).Y)
	}
	// Near-white body text #eee (238) → white (238 < 240, should not be captured).
	if got.GrayAt(0, 0).Y != 255 {
		t.Errorf("body text #eee pixel: gray=%d; want 255 (238 < threshold 240, not captured)", got.GrayAt(0, 0).Y)
	}
}

// TestBrightTextToGray_DarkBackground verifies dark page backgrounds become white
// (background), ensuring white button labels stand out on dark-themed pages.
func TestBrightTextToGray_DarkBackground(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	// Dark page background (#1a1a2e) — should become white background.
	darkBg := color.RGBA{R: 26, G: 26, B: 46, A: 255}
	// Pure white button text — should become black.
	whiteText := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if x == 5 && y == 5 {
				img.Set(x, y, whiteText)
			} else {
				img.Set(x, y, darkBg)
			}
		}
	}

	got := brightTextToGray(img, 240)

	if got.GrayAt(5, 5).Y != 0 {
		t.Errorf("white text on dark bg: gray=%d; want 0 (black)", got.GrayAt(5, 5).Y)
	}
	if got.GrayAt(0, 0).Y != 255 {
		t.Errorf("dark background: gray=%d; want 255 (white)", got.GrayAt(0, 0).Y)
	}
}

// TestOptions_DefaultIsGrayscale verifies the zero value of Options selects
// grayscale mode (Color == false), preserving backward-compatible behaviour.
func TestOptions_DefaultIsGrayscale(t *testing.T) {
	var opts Options
	if opts.Color {
		t.Error("Options zero value: Color = true; want false (grayscale is the default)")
	}
}

// TestOptions_ColorMode verifies that Options{Color: true} is accepted by
// ReadFile without error (the pipeline skips grayscale + contrast stretch and
// passes a colour image to Tesseract instead).
func TestOptions_ColorMode(t *testing.T) {
	img := whiteImage(200, 100)
	path := writePNG(t, img)

	_, err := ReadFile(path, Options{Color: true})
	// A missing Tesseract install would return an error containing "tessdata";
	// that is the only expected failure. Any other error is a bug.
	if err != nil && !strings.Contains(err.Error(), "tessdata") {
		t.Errorf("ReadFile with Color=true returned unexpected error: %v", err)
	}
}

// TestReadImage_WhiteImage verifies ReadImage works end-to-end with an
// in-memory image (no file I/O path).
func TestReadImage_WhiteImage(t *testing.T) {
	skipIfNoTesseract(t)

	img := whiteImage(200, 100)
	result, err := ReadImage(img, Options{})
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Words == nil {
		t.Error("Expected non-nil Words slice")
	}
	if len(result.Words) != 0 {
		t.Errorf("Expected no words from white image, got %d", len(result.Words))
	}
}

// TestReadImage_ColorMode verifies ReadImage accepts Color=true without error.
func TestReadImage_ColorMode(t *testing.T) {
	img := whiteImage(200, 100)
	_, err := ReadImage(img, Options{Color: true})
	if err != nil && !strings.Contains(err.Error(), "tessdata") {
		t.Errorf("ReadImage with Color=true returned unexpected error: %v", err)
	}
}

// TestReadImage_InvertedMode verifies ReadImage accepts Inverted=true without error.
func TestReadImage_InvertedMode(t *testing.T) {
	img := whiteImage(200, 100)
	_, err := ReadImage(img, Options{Inverted: true})
	if err != nil && !strings.Contains(err.Error(), "tessdata") {
		t.Errorf("ReadImage with Inverted=true returned unexpected error: %v", err)
	}
}

func TestPrepareParallelImageSet_GrayscaleIncludesAllVariants(t *testing.T) {
	img := whiteImage(20, 10)
	set, err := PrepareParallelImageSet(img, true)
	if err != nil {
		t.Fatalf("PrepareParallelImageSet: %v", err)
	}
	if len(set.Normal) == 0 || len(set.Inverted) == 0 || len(set.BrightText) == 0 || len(set.Color) == 0 {
		t.Fatalf("expected all grayscale variants to be populated: %+v", map[string]int{
			"normal":      len(set.Normal),
			"inverted":    len(set.Inverted),
			"bright_text": len(set.BrightText),
			"color":       len(set.Color),
		})
	}
}

func TestPrepareParallelImageSet_ColorOnlyReusesBytes(t *testing.T) {
	img := whiteImage(20, 10)
	set, err := PrepareParallelImageSet(img, false)
	if err != nil {
		t.Fatalf("PrepareParallelImageSet: %v", err)
	}
	if len(set.Normal) == 0 || len(set.Color) == 0 {
		t.Fatal("expected normal/color bytes to be populated")
	}
	if &set.Normal[0] != &set.Color[0] {
		t.Fatal("expected non-grayscale path to reuse the same prepared bytes for normal and color")
	}
	if len(set.Inverted) != 0 || len(set.BrightText) != 0 {
		t.Fatal("expected inverted and bright-text variants to stay empty when grayscale=false")
	}
}

// BenchmarkToGrayscaleContrast_RGBA benchmarks the fast path (input is
// *image.RGBA, the type returned by robotgo screen captures).
func BenchmarkToGrayscaleContrast_RGBA(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	// Fill with non-uniform data so the contrast stretch actually runs.
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toGrayscaleContrast(img)
	}
}

// =============================================================================
// Error handling tests
// =============================================================================

func TestReadFile_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.png")
	_, err := ReadFile(path, Options{})
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

// Test disabled: Tesseract C++ API on Windows swallows invalid image format
// errors and returns an empty string without an error code via gosseract.
/*
func TestReadFile_NotAPNG(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "bad-*.png")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	f.WriteString("not a valid image file")
	f.Close()

	_, err = ReadFile(f.Name(), Options{})
	if err == nil {
		t.Error("Expected error for invalid image, got nil")
	}
}
*/

// =============================================================================
// Result structure tests (require Tesseract)
// =============================================================================

func skipIfNoTesseract(t *testing.T) {
	t.Helper()
	// Try a minimal OCR call; if Tesseract data files are missing it errors.
	img := whiteImage(100, 100)
	path := writePNG(t, img)
	_, err := ReadFile(path, Options{})
	if err != nil && strings.Contains(err.Error(), "tessdata") {
		t.Skip("Tesseract data files not available")
	}
}

func TestReadFile_WhiteImage_ReturnsEmptyText(t *testing.T) {
	skipIfNoTesseract(t)

	img := whiteImage(200, 100)
	path := writePNG(t, img)

	result, err := ReadFile(path, Options{})
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

	result, err := ReadFile(path, Options{})
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
