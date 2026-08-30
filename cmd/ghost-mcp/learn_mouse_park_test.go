package main

// Regression test found live on 2026-08-30: learn_screen scrolled at whatever
// position the cursor happened to be, so with the cursor over another window
// the wheel events scrolled that window and every "page" captured identically.
// The learn paths must park the cursor inside the scan region before the
// first scroll.

import (
	"image"
	"image/color"
	"testing"
)

func TestLearnScreenSync_ParksCursorInScanRegionBeforeScrolling(t *testing.T) {
	origGetScreenSize := uiGetScreenSize
	origCaptureImage := uiCaptureImage
	origMoveMouse := uiMoveMouse
	origScrollDir := uiScrollDir
	origCheckFailsafe := uiCheckFailsafe
	t.Cleanup(func() {
		uiGetScreenSize = origGetScreenSize
		uiCaptureImage = origCaptureImage
		uiMoveMouse = origMoveMouse
		uiScrollDir = origScrollDir
		uiCheckFailsafe = origCheckFailsafe
	})

	uiGetScreenSize = func() (int, int) { return 800, 600 }
	uiCheckFailsafe = func() error { return nil }

	// Distinct images per capture so repeat detection never stops the scan.
	var captureCalls int
	uiCaptureImage = func(x, y, width, height int) (image.Image, error) {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for px := 0; px < width; px++ {
			img.Set(px, captureCalls%height, color.RGBA{uint8(captureCalls * 40), 0, 0, 255})
		}
		captureCalls++
		return img, nil
	}

	type event struct {
		kind string
		x, y int
	}
	var events []event
	uiMoveMouse = func(x, y int) { events = append(events, event{"move", x, y}) }
	uiScrollDir = func(amount int, direction string) { events = append(events, event{"scroll", 0, 0}) }

	if _, err := learnScreenSync(learnCfg{MaxPages: 2}); err != nil {
		t.Fatalf("learnScreenSync failed: %v", err)
	}

	if len(events) == 0 || events[0].kind != "move" {
		t.Fatalf("expected a mouse move before any scroll, events = %+v", events)
	}
	if events[0].x != 400 || events[0].y != 300 {
		t.Errorf("cursor parked at (%d,%d), want scan-region center (400,300)", events[0].x, events[0].y)
	}
	sawScroll := false
	for _, e := range events {
		if e.kind == "scroll" {
			sawScroll = true
			break
		}
	}
	if !sawScroll {
		t.Fatal("expected the scan to scroll at least once with MaxPages=2")
	}
}
