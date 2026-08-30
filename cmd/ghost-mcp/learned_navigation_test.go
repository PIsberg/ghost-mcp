package main

// Regression tests for learned-view navigation (issues #153, #154, #155, #157):
// visual_id resolution must scroll to the element's captured page, navigation
// must be delta-based against the tracked scroll position, and the
// stopped-changing heuristic must survive a single repaint race.

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/learner"
	"github.com/ghost-mcp/internal/ocr"
	"github.com/mark3labs/mcp-go/mcp"
)

// setupNavView installs a fresh enabled learner with one element on page 2
// (5 ticks per page => target offset 10) and swaps the scroll seams to record
// instead of moving the real mouse wheel.
func setupNavView(t *testing.T) *[]string {
	t.Helper()

	origLearner := globalLearner
	origScrollDir := uiScrollDir
	origCheckFailsafe := uiCheckFailsafe
	t.Cleanup(func() {
		globalLearner = origLearner
		uiScrollDir = origScrollDir
		uiCheckFailsafe = origCheckFailsafe
	})

	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{ID: 7, Text: "Footer", X: 100, Y: 200, Width: 40, Height: 20, PageIndex: 2},
		},
		ScrollAmountUsed: 5,
		CapturedAt:       time.Now(),
		ScreenW:          1920,
		ScreenH:          1080,
	})

	scrolls := &[]string{}
	uiScrollDir = func(amount int, direction string) {
		*scrolls = append(*scrolls, direction)
		_ = amount
	}
	uiCheckFailsafe = func() error { return nil }
	return scrolls
}

func TestResolveVisualTarget_ScrollsToElementPage(t *testing.T) {
	origScrollDir := uiScrollDir
	origLearner := globalLearner
	origCheckFailsafe := uiCheckFailsafe
	t.Cleanup(func() {
		globalLearner = origLearner
		uiScrollDir = origScrollDir
		uiCheckFailsafe = origCheckFailsafe
	})

	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{ID: 7, Text: "Footer", X: 100, Y: 200, Width: 40, Height: 20, PageIndex: 2},
		},
		ScrollAmountUsed: 5,
		CapturedAt:       time.Now(),
		ScreenW:          1920,
		ScreenH:          1080,
	})

	type scrollCall struct {
		amount    int
		direction string
	}
	var calls []scrollCall
	uiScrollDir = func(amount int, direction string) {
		calls = append(calls, scrollCall{amount, direction})
	}
	uiCheckFailsafe = func() error { return nil }

	x, y, found, err := resolveVisualTarget(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected element 7 to be found")
	}
	if x != 120 || y != 210 {
		t.Errorf("center = (%d,%d), want (120,210)", x, y)
	}
	// Page 2 at 5 ticks/page from origin => one downward navigation of 10.
	if len(calls) != 1 || calls[0].direction != "down" || calls[0].amount != 10 {
		t.Fatalf("scroll calls = %+v, want one down/10", calls)
	}

	// Second resolution of the same element: viewport already there, no scroll.
	_, _, _, err = resolveVisualTarget(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("second resolve scrolled again: calls = %+v", calls)
	}

	// A tracked scroll elsewhere (e.g. the scroll tool moving up 4 ticks)
	// must be compensated by delta, not by a blind full re-scroll.
	recordTrackedScroll(4, "up")
	_, _, _, err = resolveVisualTarget(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 || calls[1].direction != "down" || calls[1].amount != 4 {
		t.Fatalf("drift compensation calls = %+v, want second down/4", calls)
	}
}

func TestResolveVisualTarget_UnknownID(t *testing.T) {
	scrolls := setupNavView(t)

	_, _, found, err := resolveVisualTarget(999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected unknown visual_id to report found=false")
	}
	if len(*scrolls) != 0 {
		t.Fatalf("unknown id must not scroll, got %v", *scrolls)
	}
}

func TestScrollToLearnedPage_NoViewIsNoop(t *testing.T) {
	scrolls := setupNavView(t)
	globalLearner.ClearView()

	if err := scrollToLearnedPage(3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*scrolls) != 0 {
		t.Fatalf("no-view navigation must not scroll, got %v", *scrolls)
	}
}

func TestRecordTrackedScroll_Directions(t *testing.T) {
	origLearner := globalLearner
	t.Cleanup(func() { globalLearner = origLearner })
	globalLearner = learner.New()
	globalLearner.SetView(&learner.View{ScrollAmountUsed: 5, CapturedAt: time.Now()})

	recordTrackedScroll(7, "down")
	recordTrackedScroll(3, "up")
	recordTrackedScroll(9, "left") // horizontal: ignored
	if got := globalLearner.CurrentScrollTicks(); got != 4 {
		t.Fatalf("tracked ticks = %d, want 4", got)
	}
}

// TestHandleScrollUntilText_SurvivesSingleRepaintRace reproduces issue #157:
// one unchanged capture right after a scroll (a repaint race) must not abort
// the search. The old single-strike heuristic returned "viewport stopped
// changing" here; the search must instead continue and find the text.
func TestHandleScrollUntilText_SurvivesSingleRepaintRace(t *testing.T) {
	originalGetScreenSize := uiGetScreenSize
	originalCaptureImage := uiCaptureImage
	originalReadImage := uiReadImage
	originalFindText := uiFindText
	originalMoveMouse := uiMoveMouse
	originalScrollDir := uiScrollDir
	originalCheckFailsafe := uiCheckFailsafe
	t.Cleanup(func() {
		uiGetScreenSize = originalGetScreenSize
		uiCaptureImage = originalCaptureImage
		uiReadImage = originalReadImage
		uiFindText = originalFindText
		uiMoveMouse = originalMoveMouse
		uiScrollDir = originalScrollDir
		uiCheckFailsafe = originalCheckFailsafe
	})

	uiGetScreenSize = func() (int, int) { return 1280, 720 }
	uiCheckFailsafe = func() error { return nil }
	uiMoveMouse = func(x, y int) {}
	uiScrollDir = func(amount int, direction string) {}

	// Captures: frame A, frame A again (repaint race), then a new frame B.
	var captureCalls int
	uiCaptureImage = func(x, y, width, height int) (image.Image, error) {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		if captureCalls >= 2 {
			img.Set(0, 0, color.RGBA{200, 0, 0, 255})
		}
		captureCalls++
		return img, nil
	}
	uiReadImage = func(image.Image, ocr.Options) (*ocr.Result, error) {
		return &ocr.Result{Text: "some page text"}, nil
	}
	var findCalls int
	uiFindText = func(context.Context, image.Image, string, int, bool, string) (int, int, int, int, bool, string) {
		findCalls++
		if findCalls == 3 {
			return 10, 20, 110, 60, true, "normal"
		}
		return 0, 0, 0, 0, false, ""
	}

	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"text":        "Target",
		"direction":   "down",
		"max_scrolls": float64(4),
	}}}

	result, err := handleScrollUntilText(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		if strings.Contains(text, "viewport stopped changing") {
			t.Fatal("single repaint race aborted the search (issue #157 regression)")
		}
		t.Fatalf("unexpected error result: %s", text)
	}
}
