package ocr

// Regression test for issue #158: white/dark labels on saturated-colour
// buttons are invisible to every full-image OCR pass (the skipped
// TestColoredButtonsDetection documents that), but the region-proposal
// pipeline — cv.FindColorButtons rectangles OCR'd as tight crops at native
// scale — must read them from the same real screenshot.

import (
	"strings"
	"testing"

	"github.com/ghost-mcp/internal/cv"
)

func TestColoredButtons_RegionProposal(t *testing.T) {
	img := loadJPEG(t, "testdata/colored_buttons.jpg")

	rects := cv.FindColorButtons(img)
	if len(rects) == 0 {
		t.Fatal("FindColorButtons found no candidate rectangles on the fixture screenshot")
	}
	t.Logf("FindColorButtons: %d candidates: %v", len(rects), rects)

	res, err := ReadColorButtonRegions(img, rects)
	if err != nil {
		t.Fatalf("ReadColorButtonRegions: %v", err)
	}
	joined := strings.ToUpper(res.Text)
	t.Logf("region OCR text: %s", joined)

	for _, target := range []string{"PRIMARY", "SUCCESS", "WARNING"} {
		if !strings.Contains(joined, target) {
			t.Errorf("region-proposal OCR did not read %q", target)
		}
	}

	// Words must carry full-image coordinates (inside the image bounds and
	// inside one of the proposed rectangles), so learn_screen can store them
	// as clickable elements.
	for _, w := range res.Words {
		inside := false
		for _, r := range rects {
			if w.X >= r.Min.X-5 && w.X <= r.Max.X+5 && w.Y >= r.Min.Y-5 && w.Y <= r.Max.Y+5 {
				inside = true
				break
			}
		}
		if !inside {
			t.Errorf("word %q at (%d,%d) lies outside every proposed rectangle", w.Text, w.X, w.Y)
		}
	}
}
