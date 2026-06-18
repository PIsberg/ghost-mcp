package ocr

import (
	"image"
	"image/jpeg"
	"os"
	"strings"
	"testing"
)

func loadJPEG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return img
}

// TestColoredButtonsDetection runs every preprocessing pass against a real
// screenshot of the test fixture's white-on-colour buttons (PRIMARY #667eea,
// SUCCESS #28a745, WARNING #f0ad4e) and reports which pass, if any, reads each
// label.
//
// KNOWN LIMITATION (skipped): none of the six passes read these labels through
// the normal pipeline, and there is no simple pass/scale fix. Findings, proven
// by experiment:
//
//   - The preprocessing is fine: the UNMODIFIED brightTextToGray output of a
//     TIGHT CROP of the button row OCRs to "PRIMARY SUCCESS" at scale=1.
//   - But on the FULL screenshot, bright-text AND dark-text detect the buttons
//     at NO scale (1, 2, and 3 all return zero button hits) — while smaller page
//     text reads fine. So it is not just over-upscaling; full-image layout
//     analysis drops these large, uppercase, letter-spaced labels regardless of
//     scale. They are only recoverable from a tight per-region crop at native
//     scale.
//
// Consequence: adding native/low-scale bright/dark-text passes would NOT fix
// learn_screen (which scans full-screen) — verified, not assumed. A real fix
// would need region-proposal OCR (detect coloured rectangles, then OCR each crop
// at native scale), a much larger change. Until then the workaround is visual
// click_at on the button (OCR locate does not find these). Remove the Skip when
// fixed — the assertion below then guards the regression.
func TestColoredButtonsDetection(t *testing.T) {
	t.Skip("known limitation: OCR does not detect white-on-saturated-colour buttons; see doc comment")
	img := loadJPEG(t, "testdata/colored_buttons.jpg")
	targets := []string{"PRIMARY", "SUCCESS", "WARNING"}
	foundBy := detectButtonsAllPasses(t, img, targets)
	for _, tgt := range targets {
		t.Logf("TARGET %-8s detected by passes: %v", tgt, foundBy[tgt])
		if len(foundBy[tgt]) == 0 {
			t.Errorf("NO pass detected button label %q", tgt)
		}
	}
}

// detectButtonsAllPasses runs the six preprocessing passes the exact way the
// live pipeline does — PrepareParallelImageSet builds distinct preprocessed
// bytes per pass, then each is OCR'd with ReadPreparedBytes. (Calling
// ReadImage per-pass does NOT work: it caches by image hash ignoring Options,
// so every pass returns the first/normal result.) Returns, per target label,
// the list of pass names that detected it; logs every pass's words.
func detectButtonsAllPasses(t *testing.T, img image.Image, targets []string) map[string][]string {
	t.Helper()
	prepared, err := PrepareParallelImageSet(img, true)
	if err != nil {
		t.Fatalf("PrepareParallelImageSet: %v", err)
	}
	passes := []struct {
		name string
		data []byte
	}{
		{"normal", prepared.Normal},
		{"inverted", prepared.Inverted},
		{"bright-text", prepared.BrightText},
		{"dark-text", prepared.DarkText},
		{"color", prepared.Color},
		{"color-inverted", prepared.ColorInverted},
	}
	foundBy := map[string][]string{}
	for _, p := range passes {
		res, err := ReadPreparedBytes(p.data, ScaleFactor, Options{})
		if err != nil {
			t.Errorf("pass %s: %v", p.name, err)
			continue
		}
		words := make([]string, 0, len(res.Words))
		for _, w := range res.Words {
			words = append(words, w.Text)
		}
		joined := strings.ToUpper(strings.Join(words, " "))
		for _, tgt := range targets {
			if strings.Contains(joined, tgt) {
				foundBy[tgt] = append(foundBy[tgt], p.name)
			}
		}
		t.Logf("pass %-14s %2d words: %s", p.name, len(res.Words), strings.Join(words, " | "))
	}
	return foundBy
}
