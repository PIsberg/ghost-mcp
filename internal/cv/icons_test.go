package cv

// Tests for the shape detectors (issue #158): FindInputBoxes must locate
// empty text-entry fields that OCR cannot see, and FindIcons must keep its
// pre-refactor behaviour on icon-sized components.

import (
	"image"
	"image/color"
	"testing"
)

// drawRectOutline draws a 2px border of the given color on img.
func drawRectOutline(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for t := 0; t < 2; t++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, r.Min.Y+t, c)
			img.SetRGBA(x, r.Max.Y-1-t, c)
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			img.SetRGBA(r.Min.X+t, y, c)
			img.SetRGBA(r.Max.X-1-t, y, c)
		}
	}
}

// fillRect fills r with the given color.
func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func whiteCanvas(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, img.Bounds(), color.RGBA{255, 255, 255, 255})
	return img
}

func TestFindInputBoxes_DetectsEmptyField(t *testing.T) {
	img := whiteCanvas(600, 200)
	// A browser-style empty input: light-gray 2px border, white interior.
	field := image.Rect(100, 80, 460, 116) // 360x36 — single-line input shape
	drawRectOutline(img, field, color.RGBA{118, 118, 118, 255})

	boxes := FindInputBoxes(img)
	if len(boxes) == 0 {
		t.Fatal("expected the empty input field to be detected, got none")
	}
	// The detected box must cover the field's center.
	center := image.Pt((field.Min.X+field.Max.X)/2, (field.Min.Y+field.Max.Y)/2)
	covered := false
	for _, b := range boxes {
		if center.In(b) {
			covered = true
			break
		}
	}
	if !covered {
		t.Fatalf("no detected box covers the field center %v: %v", center, boxes)
	}
}

func TestFindInputBoxes_RejectsDarkFilledBox(t *testing.T) {
	img := whiteCanvas(600, 200)
	// A filled dark button of input-like proportions must be rejected: its
	// interior is not light, so it is not a text-entry field.
	fillRect(img, image.Rect(100, 80, 460, 116), color.RGBA{60, 60, 200, 255})

	for _, b := range FindInputBoxes(img) {
		if image.Pt(280, 98).In(b) {
			t.Fatalf("dark filled box was misdetected as an input field: %v", b)
		}
	}
}

func TestFindInputBoxes_RejectsIconSizedBox(t *testing.T) {
	img := whiteCanvas(300, 200)
	// A square outline (icon/checkbox shape) fails the 3:1 aspect requirement.
	drawRectOutline(img, image.Rect(100, 80, 140, 120), color.RGBA{118, 118, 118, 255})

	if boxes := FindInputBoxes(img); len(boxes) != 0 {
		t.Fatalf("square outline misdetected as input field: %v", boxes)
	}
}

func TestFindIcons_StillDetectsIconSizedComponent(t *testing.T) {
	img := whiteCanvas(300, 200)
	// A solid 40x40 blob is exactly the icon shape the detector targets.
	fillRect(img, image.Rect(100, 80, 140, 120), color.RGBA{30, 30, 30, 255})

	icons := FindIcons(img)
	if len(icons) == 0 {
		t.Fatal("expected the 40x40 blob to be detected as an icon")
	}
	center := image.Pt(120, 100)
	covered := false
	for _, r := range icons {
		if center.In(r) {
			covered = true
			break
		}
	}
	if !covered {
		t.Fatalf("no icon rect covers the blob center: %v", icons)
	}
}

func TestFindInputBoxes_NilImage(t *testing.T) {
	if boxes := FindInputBoxes(nil); boxes != nil {
		t.Fatalf("nil image should yield nil, got %v", boxes)
	}
}
