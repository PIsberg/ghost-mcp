package main

// Tests for CV shape elements in the learned view (issue #158): empty input
// boxes must enter the view typed as "input", and inferTypes must not clobber
// shape-assigned types (the pre-existing bug that turned every icon into
// "unknown" because re-inference ran on their empty text).

import (
	"image"
	"testing"

	"github.com/ghost-mcp/internal/learner"
)

func TestAppendShapeElements_AssignsInputType(t *testing.T) {
	rects := []image.Rectangle{image.Rect(100, 80, 460, 116)}
	elems := appendShapeElements(nil, rects, 1, 10, 20, learner.ElementTypeInput)

	if len(elems) != 1 {
		t.Fatalf("elements = %d, want 1", len(elems))
	}
	e := elems[0]
	if e.Type != learner.ElementTypeInput {
		t.Errorf("type = %q, want input", e.Type)
	}
	if e.X != 110 || e.Y != 100 {
		t.Errorf("offset not applied: (%d,%d), want (110,100)", e.X, e.Y)
	}
	if e.PageIndex != 1 {
		t.Errorf("page = %d, want 1", e.PageIndex)
	}
}

func TestAppendShapeElements_SkipsOverlapWithOCRText(t *testing.T) {
	existing := []learner.Element{
		{Text: "Type here...", X: 120, Y: 90, Width: 200, Height: 20, PageIndex: 0},
	}
	rects := []image.Rectangle{image.Rect(100, 80, 460, 116)} // overlaps the placeholder
	elems := appendShapeElements(existing, rects, 0, 0, 0, learner.ElementTypeInput)

	if len(elems) != 1 {
		t.Fatalf("overlapping shape should be skipped, got %d elements", len(elems))
	}
}

func TestInferTypes_PreservesShapeAssignedTypes(t *testing.T) {
	in := []learner.Element{
		{Text: "", Width: 360, Height: 36, Type: learner.ElementTypeInput},
		{Text: "", Width: 40, Height: 40, Type: learner.ElementTypeIcon},
		{Text: "Submit", Width: 120, Height: 30}, // text: inferred normally
	}
	out := inferTypes(in)

	if out[0].Type != learner.ElementTypeInput {
		t.Errorf("input type clobbered to %q", out[0].Type)
	}
	if out[1].Type != learner.ElementTypeIcon {
		t.Errorf("icon type clobbered to %q", out[1].Type)
	}
	if out[2].Type != learner.ElementTypeButton {
		t.Errorf("text element not inferred: %q", out[2].Type)
	}
}
