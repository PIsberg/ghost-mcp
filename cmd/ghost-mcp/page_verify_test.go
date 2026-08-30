package main

// Regression tests for issue #155: drift the server cannot observe (a page
// reload restoring a mid-page position, the user scrolling by hand) must not
// produce blind mis-clicks. After navigating to a learned page the viewport
// is verified against the stored snapshot; a recognisable mismatch is
// resynced, an unrecognisable screen clears the view and errors.

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/learner"
)

// paintPattern fills a pseudo-random black/white pattern on the 9x8 cell grid
// that computeDHash samples, so different kinds yield dHashes ~32 bits apart
// (uniform shapes like half-fills collapse to near-identical hashes, which is
// what made an earlier version of these tests meaningless).
func paintPattern(kind int, w, h int) image.Image {
	cellW, cellH := w/9, h/8
	blackCell := func(cx, cy int) bool {
		v := uint32(cx*73856093) ^ uint32(cy*19349663) ^ uint32((kind+1)*83492791)
		v ^= v >> 13
		v *= 2654435761
		v ^= v >> 16
		return v&1 == 0
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if blackCell(x/cellW, y/cellH) {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	return img
}

// TestPaintPatternsAreDHashDistinct guards the harness itself: every pattern
// pair must be farther apart than the match threshold, or the drift tests
// prove nothing.
func TestPaintPatternsAreDHashDistinct(t *testing.T) {
	const w, h = 144, 96
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			d := hammingDistance(computeDHash(paintPattern(i, w, h)), computeDHash(paintPattern(j, w, h)))
			if d <= learnedPageMatchMaxDist {
				t.Fatalf("patterns %d and %d are only %d bits apart (threshold %d)", i, j, d, learnedPageMatchMaxDist)
			}
		}
	}
}

// driftHarness simulates a real page whose scroll position can drift away
// from the Learner's tracked offset. Capture returns the page image for the
// simulated true position.
type driftHarness struct {
	pos     int // true scroll offset in ticks
	byRange func(pos int) image.Image
	scrolls []string
}

func installDriftHarness(t *testing.T, h *driftHarness) {
	t.Helper()
	origLearner := globalLearner
	origScrollDir := uiScrollDir
	origCheckFailsafe := uiCheckFailsafe
	origCaptureImage := uiCaptureImage
	t.Cleanup(func() {
		globalLearner = origLearner
		uiScrollDir = origScrollDir
		uiCheckFailsafe = origCheckFailsafe
		uiCaptureImage = origCaptureImage
	})

	uiCheckFailsafe = func() error { return nil }
	uiScrollDir = func(amount int, direction string) {
		if direction == "down" {
			h.pos += amount
		} else if direction == "up" {
			h.pos -= amount
		}
		h.scrolls = append(h.scrolls, direction)
	}
	uiCaptureImage = func(x, y, w, hh int) (image.Image, error) {
		return h.byRange(h.pos), nil
	}
}

func driftView(t *testing.T, w, h int) *learner.View {
	t.Helper()
	pages := make([]learner.PageSnapshot, 3)
	for i := 0; i < 3; i++ {
		jpg := encodeJPEG(paintPattern(i, w, h))
		if jpg == nil {
			t.Fatal("encodeJPEG failed")
		}
		pages[i] = learner.PageSnapshot{Index: i, CumulativeScrollTicks: i * 10, Width: w, Height: h, JPEG: jpg}
	}
	return &learner.View{
		Elements: []learner.Element{
			{ID: 7, Text: "Footer", X: 30, Y: 40, Width: 20, Height: 10, PageIndex: 1},
		},
		Pages:            pages,
		PageCount:        3,
		ScrollAmountUsed: 10,
		CapturedAt:       time.Now(),
		ScreenW:          w,
		ScreenH:          h,
	}
}

func TestResolveVisualTarget_ResyncsAfterUnobservedDrift(t *testing.T) {
	const w, hh = 144, 96
	h := &driftHarness{
		pos: 10, // the user scrolled one page down by hand; the tracker says 0
		byRange: func(pos int) image.Image {
			switch {
			case pos < 5:
				return paintPattern(0, w, hh)
			case pos < 15:
				return paintPattern(1, w, hh)
			default:
				return paintPattern(2, w, hh)
			}
		},
	}
	installDriftHarness(t, h)
	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(driftView(t, w, hh))

	x, y, found, err := resolveVisualTarget(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("element 7 should be found")
	}
	if x != 40 || y != 45 {
		t.Errorf("coords (%d,%d), want (40,45)", x, y)
	}
	// Navigation overshot to page 2 because of the unobserved drift, then the
	// verifier recognised page 2 and scrolled back up to page 1.
	if h.pos != 10 {
		t.Errorf("true position after resync = %d ticks, want 10 (page 1)", h.pos)
	}
	if got := globalLearner.CurrentScrollTicks(); got != 10 {
		t.Errorf("tracked ticks = %d, want 10", got)
	}
}

func TestResolveVisualTarget_ClearsViewWhenScreenUnrecognisable(t *testing.T) {
	const w, hh = 144, 96
	h := &driftHarness{
		pos: 0,
		byRange: func(pos int) image.Image {
			return paintPattern(3, w, hh) // matches no learned page
		},
	}
	installDriftHarness(t, h)
	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(driftView(t, w, hh))

	_, _, found, err := resolveVisualTarget(7)
	if err == nil {
		t.Fatal("expected an out-of-sync error for an unrecognisable screen")
	}
	if !found {
		t.Fatal("the element itself was in the view; found should be true")
	}
	if !strings.Contains(err.Error(), "out of sync") {
		t.Errorf("unexpected error text: %v", err)
	}
	if globalLearner.HasView() {
		t.Error("stale view should have been cleared")
	}
}
