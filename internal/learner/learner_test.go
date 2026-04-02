package learner

import (
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Enable / Disable / IsEnabled
// =============================================================================

func TestLearner_EnableDisable(t *testing.T) {
	l := New()
	if l.IsEnabled() {
		t.Fatal("new learner should be disabled")
	}
	l.Enable()
	if !l.IsEnabled() {
		t.Fatal("learner should be enabled after Enable()")
	}
	l.Disable()
	if l.IsEnabled() {
		t.Fatal("learner should be disabled after Disable()")
	}
}

// =============================================================================
// View management: SetView / GetView / ClearView / HasView
// =============================================================================

func TestLearner_ViewLifecycle(t *testing.T) {
	l := New()

	if l.HasView() {
		t.Fatal("new learner should have no view")
	}
	if v := l.GetView(); v != nil {
		t.Fatal("GetView should return nil before SetView")
	}

	v := &View{
		Elements:   []Element{{Text: "OK", X: 10, Y: 20, Width: 50, Height: 20, PageIndex: 0}},
		PageCount:  1,
		CapturedAt: time.Now(),
		ScreenW:    1920,
		ScreenH:    1080,
	}
	l.SetView(v)

	if !l.HasView() {
		t.Fatal("HasView should return true after SetView")
	}
	got := l.GetView()
	if got == nil {
		t.Fatal("GetView should return view after SetView")
	}
	if len(got.Elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(got.Elements))
	}

	l.ClearView()
	if l.HasView() {
		t.Fatal("HasView should return false after ClearView")
	}
	if l.GetView() != nil {
		t.Fatal("GetView should return nil after ClearView")
	}
}

// =============================================================================
// AllElements
// =============================================================================

func TestLearner_AllElements_Empty(t *testing.T) {
	l := New()
	if elems := l.AllElements(); elems != nil {
		t.Fatal("AllElements should return nil when no view")
	}
}

func TestLearner_AllElements_ReturnsCopy(t *testing.T) {
	l := New()
	l.SetView(&View{
		Elements: []Element{
			{Text: "Save", PageIndex: 0},
			{Text: "Cancel", PageIndex: 0},
		},
	})

	got := l.AllElements()
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	// Mutating the returned slice must not affect the stored view.
	got[0].Text = "mutated"
	if l.GetView().Elements[0].Text == "mutated" {
		t.Fatal("AllElements should return an independent copy")
	}
}

// =============================================================================
// FindElement
// =============================================================================

func TestLearner_FindElement_NilView(t *testing.T) {
	l := New()
	if e := l.FindElement("Save"); e != nil {
		t.Fatal("FindElement should return nil when no view")
	}
}

func TestLearner_FindElement_ExactMatch(t *testing.T) {
	l := New()
	l.SetView(&View{Elements: []Element{
		{Text: "Save", X: 100, Y: 200, Width: 60, Height: 30, PageIndex: 0},
	}})
	e := l.FindElement("Save")
	if e == nil {
		t.Fatal("expected match")
	}
	if e.Text != "Save" {
		t.Fatalf("expected Save, got %q", e.Text)
	}
}

func TestLearner_FindElement_CaseInsensitive(t *testing.T) {
	l := New()
	l.SetView(&View{Elements: []Element{
		{Text: "SUBMIT", PageIndex: 0},
	}})
	if e := l.FindElement("submit"); e == nil {
		t.Fatal("expected case-insensitive match")
	}
}

func TestLearner_FindElement_SubstringMatch(t *testing.T) {
	l := New()
	l.SetView(&View{Elements: []Element{
		{Text: "Save Changes", PageIndex: 0},
	}})
	if e := l.FindElement("save"); e == nil {
		t.Fatal("expected substring match")
	}
}

func TestLearner_FindElement_NoMatch(t *testing.T) {
	l := New()
	l.SetView(&View{Elements: []Element{
		{Text: "Cancel", PageIndex: 0},
	}})
	if e := l.FindElement("NonExistent"); e != nil {
		t.Fatal("expected no match")
	}
}

func TestLearner_FindElement_PrefersExact(t *testing.T) {
	l := New()
	l.SetView(&View{Elements: []Element{
		{Text: "Save Changes", PageIndex: 0},
		{Text: "Save", PageIndex: 0},
	}})
	e := l.FindElement("Save")
	if e == nil {
		t.Fatal("expected match")
	}
	if e.Text != "Save" {
		t.Fatalf("exact match should win over prefix; got %q", e.Text)
	}
}

func TestLearner_FindElement_EmptyQuery(t *testing.T) {
	l := New()
	l.SetView(&View{Elements: []Element{{Text: "OK", PageIndex: 0}}})
	if e := l.FindElement(""); e != nil {
		t.Fatal("empty query should return nil")
	}
}

// =============================================================================
// FindAllElements
// =============================================================================

func TestLearner_FindAllElements_OrderedByScore(t *testing.T) {
	l := New()
	l.SetView(&View{Elements: []Element{
		{Text: "Save Draft", PageIndex: 1},   // suffix match on "save" → score 400
		{Text: "Save", PageIndex: 0},          // exact match → score 1000
		{Text: "Auto-Save Enabled", PageIndex: 0}, // substring → score 100
	}})
	results := l.FindAllElements("save")
	if len(results) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(results))
	}
	if results[0].Text != "Save" {
		t.Errorf("first result should be exact match; got %q", results[0].Text)
	}
}

func TestLearner_FindAllElements_NilView(t *testing.T) {
	l := New()
	if r := l.FindAllElements("x"); r != nil {
		t.Fatal("expected nil for no view")
	}
}

// =============================================================================
// ScoreMatch
// =============================================================================

func TestScoreMatch(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		wantMin  int
	}{
		{"save", "save", 1000},
		{"Save Changes", "save", 500},   // prefix
		{"Auto Save", "save", 400},      // suffix
		{"Please save now", "save", 100}, // substring
		{"delete", "save", 0},
		{"anything", "", 0},
	}
	for _, tc := range tests {
		got := ScoreMatch(tc.haystack, tc.needle)
		if got < tc.wantMin {
			t.Errorf("ScoreMatch(%q, %q) = %d, want >= %d", tc.haystack, tc.needle, got, tc.wantMin)
		}
		if tc.wantMin == 0 && got != 0 {
			t.Errorf("ScoreMatch(%q, %q) = %d, want 0", tc.haystack, tc.needle, got)
		}
	}
}

// =============================================================================
// DeduplicateElements
// =============================================================================

func TestDeduplicateElements_RemovesSamePageOverlap(t *testing.T) {
	elems := []Element{
		{Text: "OK", X: 10, Y: 10, Width: 60, Height: 30, Confidence: 90, PageIndex: 0},
		{Text: "OK", X: 12, Y: 11, Width: 58, Height: 28, Confidence: 80, PageIndex: 0}, // overlaps
	}
	got := DeduplicateElements(elems)
	if len(got) != 1 {
		t.Fatalf("expected 1 after dedup, got %d", len(got))
	}
	if got[0].Confidence != 90 {
		t.Error("should keep higher-confidence copy")
	}
}

func TestDeduplicateElements_KeepsDifferentPages(t *testing.T) {
	elems := []Element{
		{Text: "Header", X: 10, Y: 10, Width: 60, Height: 30, PageIndex: 0},
		{Text: "Header", X: 10, Y: 10, Width: 60, Height: 30, PageIndex: 1},
	}
	got := DeduplicateElements(elems)
	if len(got) != 2 {
		t.Fatalf("same text on different pages should be kept; got %d", len(got))
	}
}

func TestDeduplicateElements_KeepsNonOverlapping(t *testing.T) {
	elems := []Element{
		{Text: "Save", X: 100, Y: 100, Width: 60, Height: 30, PageIndex: 0},
		{Text: "Cancel", X: 200, Y: 100, Width: 80, Height: 30, PageIndex: 0},
	}
	got := DeduplicateElements(elems)
	if len(got) != 2 {
		t.Fatalf("distinct elements should both be kept; got %d", len(got))
	}
}

func TestDeduplicateElements_Empty(t *testing.T) {
	got := DeduplicateElements(nil)
	if got != nil {
		t.Fatal("nil input should return nil")
	}
}

// =============================================================================
// Concurrent safety
// =============================================================================

func TestLearner_ConcurrentAccess(t *testing.T) {
	l := New()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%4 == 0 {
				l.Enable()
			} else if i%4 == 1 {
				l.Disable()
			} else if i%4 == 2 {
				l.SetView(&View{Elements: []Element{{Text: "btn", PageIndex: 0}}})
			} else {
				l.FindElement("btn")
				l.IsEnabled()
				l.HasView()
			}
		}(i)
	}
	wg.Wait()
}
