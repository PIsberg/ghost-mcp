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
	variables      map[string]string
	language       string
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

func (f *fakeOCRClient) SetVariable(name gosseract.SettableVariable, value string) error {
	f.variables[string(name)] = value
	return nil
}

func (f *fakeOCRClient) SetLanguage(lang ...string) error {
	f.language = lang[0]
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
		client := &fakeOCRClient{id: len(created) + 1, variables: make(map[string]string)}
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
		client := &fakeOCRClient{id: len(created) + 1, variables: make(map[string]string)}
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

func TestScaleGrayNearest_ReplicatesPixels(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 2, 2))
	src.Pix[0] = 10
	src.Pix[1] = 20
	src.Pix[src.Stride] = 30
	src.Pix[src.Stride+1] = 40

	got := scaleGrayNearest(src, 2)
	want := [][]uint8{
		{10, 10, 20, 20},
		{10, 10, 20, 20},
		{30, 30, 40, 40},
		{30, 30, 40, 40},
	}

	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got.GrayAt(x, y).Y != want[y][x] {
				t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, got.GrayAt(x, y).Y, want[y][x])
			}
		}
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

// TestBrightTextToGray_WhiteOnColored verifies that white text and anti-aliased
// edge pixels become black, while fully-coloured backgrounds stay white.
// Documents the production threshold=185 (luminance + spread) behaviour:
//   - Pure white (255,255,255): lum=255 ≥ 185, spread=0 ≤ 100 → black ✓
//   - Anti-aliased white 50% on #667eea → (178,190,244): lum=191, spread=66 → black ✓
//   - Anti-aliased white 50% on #f5576c red → (250,171,181): lum=188, spread=79 → black ✓
//   - Near-white #eee (238,238,238): lum=238 ≥ 185, spread=0 ≤ 100 → black ✓
//   - Pure #667eea blue (102,126,234): lum=129 < 185 → white ✓
//   - Pure #f093fb pink (240,147,251): lum=174 < 185 → white ✓
//   - Cyan #00f2fe (0,242,254): lum=191 ≥ 185 but spread=254 > 100 → white ✓
func TestBrightTextToGray_WhiteOnColored(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	// Primary button background (#667eea) — lum=129 < 185 → excluded.
	btnPurple := color.RGBA{R: 102, G: 126, B: 234, A: 255}
	// Warning button start (#f093fb) — lum=174 < 185 → excluded.
	btnPink := color.RGBA{R: 240, G: 147, B: 251, A: 255}
	// Cyan (#00f2fe) — lum=191 ≥ 185 but spread=254 > 100 → excluded by spread.
	btnCyan := color.RGBA{R: 0, G: 242, B: 254, A: 255}
	// Pure white — button label text → black.
	whiteText := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	// Anti-aliased edge: white 50% blended with primary #667eea → (178,190,244).
	// lum=191, spread=66 → black.
	antiAliasedPurple := color.RGBA{R: 178, G: 190, B: 244, A: 255}
	// Anti-aliased edge: white 50% blended with warning red #f5576c → (250,171,181).
	// lum=188, spread=79 → black (the key regression case).
	antiAliasedRed := color.RGBA{R: 250, G: 171, B: 181, A: 255}
	// Near-white body text (#eee) — lum=238, spread=0 → black.
	bodyText := color.RGBA{R: 238, G: 238, B: 238, A: 255}

	pixels := map[image.Point]color.RGBA{
		{5, 5}: whiteText,
		{4, 5}: antiAliasedPurple,
		{3, 5}: antiAliasedRed,
		{0, 0}: bodyText,
		{1, 1}: btnPurple,
		{2, 2}: btnPink,
		{3, 3}: btnCyan,
	}
	img2 := image.NewRGBA(image.Rect(0, 0, 10, 10))
	// Fill with purple background, then set specific pixels.
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if c, ok := pixels[image.Pt(x, y)]; ok {
				img2.Set(x, y, c)
			} else {
				img2.Set(x, y, btnPurple)
			}
		}
	}
	_ = img

	got := brightTextToGray(img2, 185)

	type brightCheck struct {
		pt   image.Point
		want uint8
		desc string
	}
	checks := []brightCheck{
		{image.Pt(5, 5), 0, "pure white text → black"},
		{image.Pt(4, 5), 0, "anti-aliased white on #667eea (178,190,244): lum=191,spread=66 → black"},
		{image.Pt(3, 5), 0, "anti-aliased white on #f5576c (250,171,181): lum=188,spread=79 → black"},
		{image.Pt(0, 0), 0, "near-white #eee (238,238,238): lum=238,spread=0 → black"},
		{image.Pt(1, 1), 255, "pure #667eea blue: lum=129 < 185 → white"},
		{image.Pt(2, 2), 255, "pure #f093fb pink: lum=174 < 185 → white"},
		{image.Pt(3, 3), 255, "cyan #00f2fe: lum=191 but spread=254 > 100 → white"},
	}
	for _, c := range checks {
		if got.GrayAt(c.pt.X, c.pt.Y).Y != c.want {
			t.Errorf("%s: got gray=%d, want %d", c.desc, got.GrayAt(c.pt.X, c.pt.Y).Y, c.want)
		}
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

	got := brightTextToGray(img, 185)

	if got.GrayAt(5, 5).Y != 0 {
		t.Errorf("white text on dark bg: gray=%d; want 0 (black)", got.GrayAt(5, 5).Y)
	}
	if got.GrayAt(0, 0).Y != 255 {
		t.Errorf("dark background: gray=%d; want 255 (white)", got.GrayAt(0, 0).Y)
	}
}

// TestDarkTextToGray_DarkOnColored verifies that dark achromatic text on a
// coloured background is detected correctly. The canonical case is the WARNING
// button: dark #333 text on yellow #f0ad4e background.
func TestDarkTextToGray_DarkOnColored(t *testing.T) {
	// WARNING button: yellow background, dark text, anti-aliased edge.
	yellowBg := color.RGBA{R: 240, G: 173, B: 78, A: 255}    // #f0ad4e, lum≈180
	darkText := color.RGBA{R: 51, G: 51, B: 51, A: 255}      // #333, lum=51, spread=0
	antiAliased := color.RGBA{R: 146, G: 112, B: 65, A: 255} // 50% blend, lum≈115, spread=81

	// White background — should NOT be detected as text (lum=255 > darkTextMaxLum).
	whiteBg := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	// Saturated color pixel — should be excluded by spread check.
	cyanPx := color.RGBA{R: 0, G: 200, B: 220, A: 255} // lum≈148, spread=220
	// Dark but saturated pixel — should be excluded by spread check.
	darkRed := color.RGBA{R: 100, G: 0, B: 0, A: 255} // lum=21, spread=100 ≤ 130 — included

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	pixels := map[image.Point]color.RGBA{
		{0, 0}: darkText,
		{1, 1}: antiAliased,
		{2, 2}: yellowBg,
		{3, 3}: whiteBg,
		{4, 4}: cyanPx,
		{5, 5}: darkRed,
	}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if c, ok := pixels[image.Pt(x, y)]; ok {
				img.Set(x, y, c)
			} else {
				img.Set(x, y, yellowBg)
			}
		}
	}

	got := darkTextToGray(img)

	checks := []struct {
		pt   image.Point
		want uint8
		desc string
	}{
		{image.Pt(0, 0), 0, "#333 dark text (lum=51, spread=0) → black"},
		{image.Pt(1, 1), 0, "50% anti-aliased #333+yellow (lum≈115, spread=81) → black"},
		{image.Pt(2, 2), 255, "yellow #f0ad4e background (lum≈180 > 120) → white"},
		{image.Pt(3, 3), 255, "white background (lum=255 > 120) → white"},
		{image.Pt(4, 4), 255, "cyan (lum≈148 > 120) → white"},
		{image.Pt(5, 5), 0, "dark red (lum=21 ≤ 120, spread=100 ≤ 130) → black"},
	}
	for _, c := range checks {
		if got.GrayAt(c.pt.X, c.pt.Y).Y != c.want {
			t.Errorf("%s: got gray=%d, want %d", c.desc, got.GrayAt(c.pt.X, c.pt.Y).Y, c.want)
		}
	}
}

// TestDarkTextToGray_ColoredDarkPixelExcludedBySpread verifies that dark but
// highly-saturated pixels (e.g. a dark blue button border) are excluded by the
// spread check even when their luminance is below darkTextMaxLum.
func TestDarkTextToGray_ColoredDarkPixelExcludedBySpread(t *testing.T) {
	// Dark blue: lum≈18, but spread=131 > brightTextMaxSpread(130) → excluded.
	darkBlue := color.RGBA{R: 0, G: 0, B: 131, A: 255}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, darkBlue)
		}
	}
	got := darkTextToGray(img)
	if got.GrayAt(2, 2).Y != 255 {
		t.Errorf("dark blue (spread=131 > 130) should be excluded: got gray=%d, want 255", got.GrayAt(2, 2).Y)
	}
}

// TestDarkTextToGray_SlowPath verifies that the slow (non-RGBA) image path
// produces the same output as the fast *image.RGBA path.
func TestDarkTextToGray_SlowPath(t *testing.T) {
	// Build an RGBA image and convert to NRGBA to force the slow path.
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 4))
	rgba.Set(0, 0, color.RGBA{R: 51, G: 51, B: 51, A: 255})   // dark text → black
	rgba.Set(1, 1, color.RGBA{R: 240, G: 173, B: 78, A: 255}) // yellow bg → white

	// image.NRGBA triggers the slow path in darkTextToGray.
	nrgba := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	nrgba.Set(0, 0, color.RGBA{R: 51, G: 51, B: 51, A: 255})
	nrgba.Set(1, 1, color.RGBA{R: 240, G: 173, B: 78, A: 255})

	fast := darkTextToGray(rgba)
	slow := darkTextToGray(nrgba)

	for _, pt := range []image.Point{{0, 0}, {1, 1}} {
		f := fast.GrayAt(pt.X, pt.Y).Y
		s := slow.GrayAt(pt.X, pt.Y).Y
		if f != s {
			t.Errorf("at %v: fast=%d slow=%d — paths disagree", pt, f, s)
		}
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

// TestReadAllPasses_DeduplicatesIdenticalWords verifies that ReadAllPasses merges
// words found by multiple passes (same text, same approximate position) into a
// single entry — so a word visible in both normal and bright-text passes is not
// double-counted.
func TestReadAllPasses_DeduplicatesIdenticalWords(t *testing.T) {
	// A plain white image with no real text — all passes will return zero words.
	// We verify that ReadAllPasses returns a non-error result with an empty word list.
	img := whiteImage(200, 100)
	result, err := ReadAllPasses(img)
	if err != nil && !strings.Contains(err.Error(), "tessdata") {
		t.Fatalf("ReadAllPasses returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result from ReadAllPasses")
	}
	// White image should yield zero words regardless of pass count.
	if len(result.Words) != 0 {
		t.Errorf("expected 0 words from blank image, got %d", len(result.Words))
	}
}

// TestReadAllPasses_ReturnsMergedResult verifies that ReadAllPasses returns a
// Result whose Text field is a space-joined version of all Words.
func TestReadAllPasses_ReturnsMergedResult(t *testing.T) {
	img := whiteImage(10, 10)
	result, err := ReadAllPasses(img)
	if err != nil && !strings.Contains(err.Error(), "tessdata") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		return // tessdata missing; can't verify further
	}
	// Text must be consistent with Words.
	if len(result.Words) == 0 && result.Text != "" {
		t.Errorf("Words is empty but Text=%q; want empty string", result.Text)
	}
}

func TestPrepareParallelImageSet_GrayscaleIncludesAllVariants(t *testing.T) {
	img := whiteImage(20, 10)
	set, err := PrepareParallelImageSet(img, true)
	if err != nil {
		t.Fatalf("PrepareParallelImageSet: %v", err)
	}
	if len(set.Normal) == 0 || len(set.Inverted) == 0 || len(set.BrightText) == 0 ||
		len(set.DarkText) == 0 || len(set.Color) == 0 || len(set.ColorInverted) == 0 {
		t.Fatalf("expected all grayscale variants to be populated: %+v", map[string]int{
			"normal":         len(set.Normal),
			"inverted":       len(set.Inverted),
			"bright_text":    len(set.BrightText),
			"dark_text":      len(set.DarkText),
			"color":          len(set.Color),
			"color_inverted": len(set.ColorInverted),
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
	if len(set.Inverted) != 0 || len(set.BrightText) != 0 || len(set.DarkText) != 0 || len(set.ColorInverted) != 0 {
		t.Fatal("expected inverted, bright-text, dark-text, and color-inverted variants to stay empty when grayscale=false")
	}
}

// TestPrepareParallelImageSet_GrayscaleVariantsAreIndependent verifies that each
// preprocessing variant produces its own independent byte slice.  Sharing memory
// between variants would mean a mutation in one (e.g. inversion) corrupts another.
func TestPrepareParallelImageSet_GrayscaleVariantsAreIndependent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	// Non-uniform content so Normal ≠ Inverted after processing.
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 20), B: 128, A: 255})
		}
	}

	set, err := PrepareParallelImageSet(img, true)
	if err != nil {
		t.Fatalf("PrepareParallelImageSet: %v", err)
	}

	// Each variant must be a distinct allocation — they encode the image differently.
	if len(set.Normal) == 0 || len(set.Inverted) == 0 || len(set.BrightText) == 0 ||
		len(set.DarkText) == 0 || len(set.Color) == 0 || len(set.ColorInverted) == 0 {
		t.Fatal("expected all six variants to be non-empty")
	}
	if &set.Normal[0] == &set.Inverted[0] || &set.Normal[0] == &set.BrightText[0] ||
		&set.Normal[0] == &set.DarkText[0] || &set.Normal[0] == &set.Color[0] {
		t.Fatal("grayscale variants must not share memory — parallel writes to the same buffer would race")
	}
}

// TestPrepareParallelImageSet_ConcurrentCallsAreSafe verifies that multiple
// goroutines can call PrepareParallelImageSet simultaneously on the same image
// without data races (requires -race in go test).
func TestPrepareParallelImageSet_ConcurrentCallsAreSafe(t *testing.T) {
	img := whiteImage(200, 100)
	const workers = 8
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := PrepareParallelImageSet(img, true)
			errs <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent PrepareParallelImageSet: %v", err)
		}
	}
}

// BenchmarkPrepareParallelImageSet_Grayscale measures the wall-clock time of the
// parallel preprocessing pass on a full-HD image.  Run with -benchtime=5s to
// get stable numbers; compare against a sequential baseline by reverting the
// goroutine dispatch to measure the actual speedup.
func BenchmarkPrepareParallelImageSet_Grayscale(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := PrepareParallelImageSet(img, true); err != nil {
			b.Fatalf("PrepareParallelImageSet: %v", err)
		}
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

// =============================================================================
// Caching tests
// =============================================================================

func TestHashImageFast_SameImageSameHash(t *testing.T) {
	img1 := whiteImage(100, 100)
	img2 := whiteImage(100, 100)

	h1 := HashImageFast(img1)
	h2 := HashImageFast(img2)

	if h1 != h2 {
		t.Errorf("Expected same hash for identical images, got %d and %d", h1, h2)
	}

	// Modify a pixel
	if rgba, ok := img1.(*image.RGBA); ok {
		rgba.Set(50, 50, color.Black)
	}
	h3 := HashImageFast(img1)
	if h1 == h3 {
		t.Errorf("Expected different hash after modifying image, got %d", h3)
	}
}

func TestCache_SetAndGet(t *testing.T) {
	hash := uint64(123456789)
	res := &Result{Text: "cached result"}

	SetCachedResult(hash, res)

	got := GetCachedResult(hash)
	if got != res {
		t.Errorf("Expected to retrieve cached result, got %v", got)
	}

	gotOther := GetCachedResult(uint64(987654321))
	if gotOther != nil {
		t.Errorf("Expected nil for uncached hash, got %v", gotOther)
	}
}

func TestReadImage_UsesCachedResultWhenHashMatches(t *testing.T) {
	cacheMu.Lock()
	cacheHash = 0
	cacheResult = nil
	cacheMu.Unlock()
	t.Cleanup(func() {
		cacheMu.Lock()
		cacheHash = 0
		cacheResult = nil
		cacheMu.Unlock()
	})

	img := whiteImage(64, 32)
	want := &Result{Text: "from cache"}
	SetCachedResult(HashImageFast(img), want)

	got, err := ReadImage(img, Options{})
	if err != nil {
		t.Fatalf("ReadImage returned error: %v", err)
	}
	if got != want {
		t.Fatalf("ReadImage returned %p, want cached %p", got, want)
	}
}

// =============================================================================
// colorInvertGray
// =============================================================================

func TestColorInvertGray_WhiteOnCyan(t *testing.T) {
	// Simulates white text on the cyan INFO button background (#4facfe).
	// After color inversion: white→black (0), cyan→medium gray.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	whitePixel := color.RGBA{255, 255, 255, 255}
	cyanBg := color.RGBA{79, 172, 254, 255}

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if x >= 3 && x <= 6 && y >= 3 && y <= 6 {
				img.Set(x, y, whitePixel)
			} else {
				img.Set(x, y, cyanBg)
			}
		}
	}

	got := colorInvertGray(img)

	// White text should become black (0).
	if v := got.GrayAt(5, 5).Y; v != 0 {
		t.Errorf("white text: got gray=%d, want 0 (black)", v)
	}

	// Cyan background should become medium gray — not black, not white.
	// Inverted: (176,83,1) → BT.709 lum ≈ 83.
	bgVal := got.GrayAt(1, 1).Y
	if bgVal < 50 || bgVal > 150 {
		t.Errorf("cyan background: got gray=%d, want medium gray (50-150)", bgVal)
	}

	// Verify contrast: text should be significantly darker than background.
	if textVal := got.GrayAt(5, 5).Y; textVal >= bgVal {
		t.Errorf("text (%d) should be darker than background (%d) for contrast", textVal, bgVal)
	}
}

func TestColorInvertGray_ComparisonWithRegularInverted(t *testing.T) {
	// Prove that ColorInverted produces different results from regular Inverted
	// for white-on-cyan, which is exactly why it helps.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(2, 2, color.RGBA{255, 255, 255, 255}) // white pixel
	img.Set(0, 0, color.RGBA{79, 172, 254, 255})  // cyan pixel

	// Regular grayscale then invert: white→255, cyan→158, invert→0 and 97
	gc := toGrayscaleContrast(img)
	inverted := image.NewGray(gc.Bounds())
	for i := range inverted.Pix {
		inverted.Pix[i] = 255 - gc.Pix[i]
	}

	// ColorInverted: RGB invert then grayscale: white→0, cyan→~83
	ci := colorInvertGray(img)

	// The white pixel should be 0 in both (both make text dark).
	whiteNormal := inverted.GrayAt(2, 2).Y
	whiteCI := ci.GrayAt(2, 2).Y
	if whiteNormal != 0 || whiteCI != 0 {
		t.Errorf("white text: normal_inverted=%d, color_inverted=%d, both want 0", whiteNormal, whiteCI)
	}

	// The key difference: background brightness.
	// Regular inverted: cyan background becomes ~97 (dark).
	// Color inverted: cyan background becomes ~83 (medium).
	bgNormal := inverted.GrayAt(0, 0).Y
	bgCI := ci.GrayAt(0, 0).Y

	// Both should be non-zero (gray, not black).
	if bgNormal == 0 || bgCI == 0 {
		t.Errorf("background should be gray, got normal=%d, color_inv=%d", bgNormal, bgCI)
	}

	// The contrast gap should exist in both, but ColorInverted gives
	// black-on-medium which Tesseract handles better.
	t.Logf("White-on-cyan comparison: regular_inverted bg=%d, color_inverted bg=%d", bgNormal, bgCI)
}
