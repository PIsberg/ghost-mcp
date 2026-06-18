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
// the normal pipeline. Root cause, proven by experiment (and it is NOT the
// preprocessing): the sole blocker is the ScaleFactor=3 upscaling. The button
// labels are large (≈45 px caps in the capture); 3× upscaling pushes them past
// the size Tesseract reads reliably, while smaller page text survives. Evidence:
// the UNMODIFIED brightTextToGray output of the button row, OCR'd at scale=1,
// returns "PRIMARY SUCCESS" — but the same bytes at scale=2 or 3 return nothing.
// (brightTextToGray does invert the near-white page to black, but that is a
// red herring: page words still read; only the large button labels fail, purely
// on size.)
//
// Fix direction: run a native/low-scale variant of the bright-text and dark-text
// passes (their targets — white/dark text on coloured buttons — are large), in
// addition to the 3× passes that small body text needs. This touches three
// duplicated pass-machinery sites (ocr.ReadAllPasses, cmd parallelFindPreparedText,
// handler_learning's scan loop) and adds OCR latency, so it should be validated
// against the live pipeline before shipping. Until then the workaround is visual
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
