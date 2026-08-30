package learner

// Tests for the tracked scroll offset (issue #155) and full-element lookup
// by ID (issue #153): the offset must follow recorded scrolls and reset with
// the view lifecycle, and GetElementByID must expose PageIndex so callers can
// navigate before clicking.

import "testing"

func TestRecordScrollAndCurrentTicks(t *testing.T) {
	l := New()
	if got := l.CurrentScrollTicks(); got != 0 {
		t.Fatalf("initial ticks = %d, want 0", got)
	}
	l.RecordScroll(7)
	l.RecordScroll(5)
	l.RecordScroll(-3)
	if got := l.CurrentScrollTicks(); got != 9 {
		t.Fatalf("ticks = %d, want 9", got)
	}
}

func TestSetViewResetsScrollTicks(t *testing.T) {
	l := New()
	l.RecordScroll(12)
	l.SetView(&View{ScrollAmountUsed: 5})
	if got := l.CurrentScrollTicks(); got != 0 {
		t.Fatalf("ticks after SetView = %d, want 0 (fresh view is at its own origin)", got)
	}
}

func TestClearViewResetsScrollTicks(t *testing.T) {
	l := New()
	l.SetView(&View{ScrollAmountUsed: 5})
	l.RecordScroll(4)
	l.ClearView()
	if got := l.CurrentScrollTicks(); got != 0 {
		t.Fatalf("ticks after ClearView = %d, want 0", got)
	}
}

func TestGetElementByID(t *testing.T) {
	l := New()
	l.SetView(&View{
		Elements: []Element{
			{ID: 1, Text: "OK", X: 10, Y: 20, Width: 30, Height: 40, PageIndex: 0},
			{ID: 2, Text: "Footer", X: 50, Y: 60, Width: 70, Height: 80, PageIndex: 3},
		},
	})

	e, ok := l.GetElementByID(2)
	if !ok {
		t.Fatal("expected element 2 to be found")
	}
	if e.PageIndex != 3 || e.Text != "Footer" || e.X != 50 {
		t.Fatalf("unexpected element: %+v", e)
	}

	if _, ok := l.GetElementByID(99); ok {
		t.Fatal("expected element 99 to be absent")
	}
}

func TestGetElementByID_NoView(t *testing.T) {
	l := New()
	if _, ok := l.GetElementByID(1); ok {
		t.Fatal("expected lookup without a view to report not found")
	}
}
