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
		{Text: "Save Draft", PageIndex: 1},        // suffix match on "save" → score 400
		{Text: "Save", PageIndex: 0},              // exact match → score 1000
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
		{"Save Changes", "save", 500},    // prefix
		{"Auto Save", "save", 400},       // suffix
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

// =============================================================================
// InferElementType
// =============================================================================

func TestInferElementType_Label(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
		want   ElementType
	}{
		{"Email:", 50, 20, ElementTypeLabel},
		{"Name:", 50, 20, ElementTypeLabel},
		{"Phone Number:", 100, 20, ElementTypeLabel},
		{"Full width colon:", 100, 20, ElementTypeLabel},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != tc.want {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want %v", tc.text, tc.width, tc.height, got, tc.want)
		}
	}
}

func TestInferElementType_Heading(t *testing.T) {
	got := InferElementType("Welcome to the Dashboard", 300, 32)
	if got != ElementTypeHeading {
		t.Errorf("expected heading, got %v", got)
	}
}

func TestInferElementType_Button(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
		want   ElementType
	}{
		{"Save", 60, 30, ElementTypeButton},
		{"Cancel", 70, 30, ElementTypeButton},
		{"Submit", 80, 35, ElementTypeButton},
		{"OK", 50, 25, ElementTypeButton},
		{"Log In", 70, 30, ElementTypeButton},
		{"Sign Up", 70, 30, ElementTypeButton},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != tc.want {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want %v", tc.text, tc.width, tc.height, got, tc.want)
		}
	}
}

func TestInferElementType_Link(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
		want   ElementType
	}{
		{"https://example.com", 150, 20, ElementTypeLink},
		{"http://test.org", 120, 20, ElementTypeLink},
		{"www.example.com", 130, 20, ElementTypeLink},
		{"Learn More", 80, 20, ElementTypeLink},
		{"Click here", 70, 20, ElementTypeLink},
		{"Read more", 70, 20, ElementTypeLink},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != tc.want {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want %v", tc.text, tc.width, tc.height, got, tc.want)
		}
	}
}

func TestInferElementType_Value(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
		want   ElementType
	}{
		{"42", 30, 20, ElementTypeValue},
		{"$99.99", 60, 20, ElementTypeValue},
		{"85%", 40, 20, ElementTypeValue},
		{"1,234.56", 70, 20, ElementTypeValue},
		{"-100", 50, 20, ElementTypeValue},
		{"+50", 40, 20, ElementTypeValue},
		{"1/2", 30, 20, ElementTypeValue},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != tc.want {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want %v", tc.text, tc.width, tc.height, got, tc.want)
		}
	}
}

func TestInferElementType_Text(t *testing.T) {
	// Use dimensions outside button range: height < 16 or > 65, or width < 40
	// Also ensure it's not a button keyword, URL, numeric, or heading
	got := InferElementType("This is some body text", 200, 14)
	if got != ElementTypeText {
		t.Errorf("expected text, got %v", got)
	}
}

func TestInferElementType_Input(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
	}{
		// Original patterns
		{"Enter your email", 150, 20},
		{"Type here", 100, 20},
		{"email...", 120, 20},
		{"password...", 120, 20},
		{"username...", 120, 20},
		{"email", 120, 20},
		{"username", 120, 20},
		{"message", 120, 20},
		// New patterns — fixture placeholders
		{"Type here or use MCP type_text...", 300, 20},
		{"Multi-line text area...", 300, 24},
		// New patterns — common web forms
		{"Enter text", 150, 20},
		{"Write your message", 300, 20},
		{"First name", 150, 20},
		{"Full name", 150, 20},
		{"Search here", 200, 20},
		{"Enter here", 150, 20},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != ElementTypeInput {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want input", tc.text, tc.width, tc.height, got)
		}
	}
}

func TestInferElementType_Checkbox(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
	}{
		// Unchecked symbols (uncheckedSymbols constant)
		{"☐ I agree", 150, 20},
		{"□ unchecked item", 100, 20},
		{"◻ empty box", 100, 20},
		{"▢ tick box", 100, 20},
		// Checked symbols (checkedSymbols constant)
		{"☑ Accept terms", 150, 20},
		{"☒ dismissed", 100, 20},
		{"✓ Yes", 50, 20},
		{"✔ completed", 100, 20},
		{"✗ declined", 100, 20},
		{"✘ invalid", 100, 20},
		// Bracket patterns
		{"[ ] Option", 100, 20},
		{"[x] Selected", 100, 20},
		{"[X] Done", 100, 20},
		{"(x) confirm", 100, 20},
		{"(X) opted in", 100, 20},
		{"( ) pending", 100, 20},
		// Agreement / opt-in text
		{"Remember me", 100, 20},
		{"Subscribe", 80, 20},
		{"I agree to the terms", 200, 20},
		{"Agree to terms", 150, 20},
		{"opt in for updates", 150, 20},
		{"opt out of emails", 150, 20},
		// State description text
		{"mark as complete", 150, 20},
		{"tick this box", 150, 20},
		{"check this to confirm", 200, 20},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != ElementTypeCheckbox {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want checkbox", tc.text, tc.width, tc.height, got)
		}
	}
}

func TestInferElementType_Radio(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
	}{
		// Empty radio symbols (radioEmptySymbols constant)
		{"○ Option A", 100, 20},
		{"◯ empty option", 100, 20},
		{"◎ ringed option", 100, 20},
		// Selected radio symbols (radioSelectedSymbols constant)
		{"● Selected", 100, 20},
		{"◉ Option B", 100, 20},
		{"⊙ option C", 100, 20},
		{"⊚ chosen", 100, 20},
		// ASCII-art radio patterns
		{"(*) selected option", 100, 20},
		{"( • ) choice", 100, 20},
		{"(•) point", 100, 20},
		// Phrase patterns
		{"Option 1", 100, 20},
		{"Choice A", 100, 20},
		{"select option A", 150, 20},
		{"select this option", 150, 20},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != ElementTypeRadio {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want radio", tc.text, tc.width, tc.height, got)
		}
	}
}

// =============================================================================
// IsCheckedSymbol
// =============================================================================

func TestIsCheckedSymbol_CheckedStates(t *testing.T) {
	checked := []string{
		// checkedSymbols: ☑☒✓✔✗✘
		"☑", "☒", "✓", "✔", "✗", "✘",
		// radioSelectedSymbols: ●◉⊙⊚
		"●", "◉", "⊙", "⊚",
		// pattern-based
		"[x]", "[X]", "(*)", "(•)",
		// embedded in text
		"☑ Accept terms",
		"(*) Selected option",
		"✓ confirmed",
	}
	for _, s := range checked {
		if !IsCheckedSymbol(s) {
			t.Errorf("IsCheckedSymbol(%q) = false, want true", s)
		}
	}
}

func TestIsCheckedSymbol_UncheckedStates(t *testing.T) {
	unchecked := []string{
		// uncheckedSymbols: ☐□◻▢
		"☐", "□", "◻", "▢",
		// radioEmptySymbols: ○◯◎
		"○", "◯", "◎",
		// ASCII empty
		"[ ]", "( )",
		// plain label text
		"Remember me",
		"Option 1",
	}
	for _, s := range unchecked {
		if IsCheckedSymbol(s) {
			t.Errorf("IsCheckedSymbol(%q) = true, want false", s)
		}
	}
}

func TestInferElementType_Dropdown(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
	}{
		{"Select...", 120, 20},
		{"Choose...", 120, 20},
		{"▼ Select option", 150, 20},
		{"-- Select --", 120, 20},
		{"Pick one", 80, 20},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != ElementTypeDropdown {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want dropdown", tc.text, tc.width, tc.height, got)
		}
	}
}

func TestInferElementType_Toggle(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
	}{
		{"ON", 50, 20},
		{"OFF", 50, 20},
		{"Enabled", 70, 20},
		{"Disabled", 70, 20},
		{"Active", 70, 20},
		{"Inactive", 70, 20},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != ElementTypeToggle {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want toggle", tc.text, tc.width, tc.height, got)
		}
	}
}

func TestInferElementType_Slider(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		height int
	}{
		{"Volume 50%", 100, 20},
		{"Brightness: 75%", 120, 20},
		{"Zoom: 100%", 80, 20},
		{"Speed", 60, 20},
		{"Opacity", 70, 20},
		{"Range: 0-100", 100, 20},
		{"Level", 60, 20},
		{"Contrast 80%", 100, 20},
	}
	for _, tc := range tests {
		got := InferElementType(tc.text, tc.width, tc.height)
		if got != ElementTypeSlider {
			t.Errorf("InferElementType(%q, %d, %d) = %v, want slider", tc.text, tc.width, tc.height, got)
		}
	}
}

func TestInferElementType_Unknown(t *testing.T) {
	got := InferElementType("", 0, 0)
	if got != ElementTypeUnknown {
		t.Errorf("expected unknown for empty text, got %v", got)
	}
}

func TestInferElementType_ButtonKeywords(t *testing.T) {
	keywords := []string{
		"ok", "yes", "no", "cancel", "close", "dismiss", "done",
		"submit", "send", "save", "delete", "remove", "back", "next",
		"continue", "confirm", "apply", "accept", "login", "sign in",
		"create", "new", "add", "edit", "update", "copy", "paste",
		"search", "filter", "sort", "refresh", "reset", "expand",
		"collapse", "toggle", "show", "hide", "select all",
	}
	for _, kw := range keywords {
		got := InferElementType(kw, 60, 30)
		if got != ElementTypeButton {
			t.Errorf("keyword %q should be button, got %v", kw, got)
		}
	}
}

// =============================================================================
// AssociateLabels
// =============================================================================

func TestAssociateLabels_LabelToRight(t *testing.T) {
	elems := []Element{
		{Text: "Email:", X: 10, Y: 10, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "Enter your email", X: 80, Y: 10, Width: 150, Height: 20, Type: ElementTypeText},
	}
	result := AssociateLabels(elems)
	if result[0].LabelFor != "Enter your email" {
		t.Errorf("expected label to be associated with 'Enter your email', got %q", result[0].LabelFor)
	}
}

func TestAssociateLabels_LabelBelow(t *testing.T) {
	elems := []Element{
		{Text: "Name:", X: 10, Y: 10, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "John Doe", X: 10, Y: 40, Width: 100, Height: 20, Type: ElementTypeText},
	}
	result := AssociateLabels(elems)
	if result[0].LabelFor != "John Doe" {
		t.Errorf("expected label to be associated with 'John Doe', got %q", result[0].LabelFor)
	}
}

func TestAssociateLabels_NoAssociation(t *testing.T) {
	elems := []Element{
		{Text: "Orphan Label:", X: 10, Y: 10, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "Too far away", X: 500, Y: 500, Width: 100, Height: 20, Type: ElementTypeText},
	}
	result := AssociateLabels(elems)
	if result[0].LabelFor != "" {
		t.Errorf("expected no association, got %q", result[0].LabelFor)
	}
}

func TestAssociateLabels_PreservesOriginal(t *testing.T) {
	elems := []Element{
		{Text: "Email:", X: 10, Y: 10, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "test @example.com", X: 80, Y: 10, Width: 150, Height: 20, Type: ElementTypeText},
	}
	original := make([]Element, len(elems))
	copy(original, elems)
	_ = AssociateLabels(elems)
	// Original slice should not be modified.
	if original[0].LabelFor != "" {
		t.Error("original slice should not be modified")
	}
}

func TestAssociateLabels_MultipleLabels(t *testing.T) {
	elems := []Element{
		{Text: "First:", X: 10, Y: 10, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "John", X: 80, Y: 10, Width: 100, Height: 20, Type: ElementTypeText},
		{Text: "Last:", X: 10, Y: 50, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "Doe", X: 80, Y: 50, Width: 100, Height: 20, Type: ElementTypeText},
	}
	result := AssociateLabels(elems)
	if result[0].LabelFor != "John" {
		t.Errorf("first label should associate with 'John', got %q", result[0].LabelFor)
	}
	if result[2].LabelFor != "Doe" {
		t.Errorf("second label should associate with 'Doe', got %q", result[2].LabelFor)
	}
}

func TestAssociateLabels_LabelSkipsOtherLabels(t *testing.T) {
	elems := []Element{
		{Text: "Label1:", X: 10, Y: 10, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "Label2:", X: 80, Y: 10, Width: 50, Height: 20, Type: ElementTypeLabel},
		{Text: "Value", X: 150, Y: 10, Width: 100, Height: 20, Type: ElementTypeText},
	}
	result := AssociateLabels(elems)
	// Label1 should associate with Value, not Label2
	if result[0].LabelFor != "Value" {
		t.Errorf("label should skip over other labels, got %q", result[0].LabelFor)
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkFindElement(b *testing.B) {
	l := New()
	l.Enable()
	l.SetView(&View{
		Elements: []Element{
			{Text: "Save", X: 100, Y: 200, Width: 60, Height: 30, PageIndex: 0},
			{Text: "Cancel", X: 200, Y: 200, Width: 70, Height: 30, PageIndex: 0},
			{Text: "Submit", X: 300, Y: 200, Width: 80, Height: 30, PageIndex: 0},
			{Text: "Delete", X: 400, Y: 200, Width: 75, Height: 30, PageIndex: 0},
			{Text: "Edit", X: 500, Y: 200, Width: 50, Height: 30, PageIndex: 0},
		},
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.FindElement("Save")
	}
}

func BenchmarkFindElement_LargeView(b *testing.B) {
	l := New()
	l.Enable()
	elements := make([]Element, 100)
	for i := 0; i < 100; i++ {
		elements[i] = Element{
			Text:      "Element",
			X:         i * 10,
			Y:         i * 10,
			Width:     50,
			Height:    20,
			PageIndex: i / 10,
		}
	}
	l.SetView(&View{Elements: elements})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.FindElement("Element")
	}
}

func BenchmarkFindAllElements(b *testing.B) {
	l := New()
	l.Enable()
	l.SetView(&View{
		Elements: []Element{
			{Text: "Save", PageIndex: 0},
			{Text: "Save Draft", PageIndex: 0},
			{Text: "Auto-Save", PageIndex: 1},
			{Text: "Cancel", PageIndex: 0},
			{Text: "Submit", PageIndex: 0},
		},
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.FindAllElements("save")
	}
}

func BenchmarkScoreMatch(b *testing.B) {
	haystack := "Save Changes button on the page"
	needle := "save"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScoreMatch(haystack, needle)
	}
}

func BenchmarkDeduplicateElements(b *testing.B) {
	elements := []Element{
		{Text: "OK", X: 10, Y: 10, Width: 60, Height: 30, Confidence: 90, PageIndex: 0},
		{Text: "OK", X: 12, Y: 11, Width: 58, Height: 28, Confidence: 80, PageIndex: 0},
		{Text: "Cancel", X: 100, Y: 10, Width: 70, Height: 30, Confidence: 85, PageIndex: 0},
		{Text: "Submit", X: 200, Y: 10, Width: 80, Height: 35, Confidence: 92, PageIndex: 0},
		{Text: "Submit", X: 202, Y: 12, Width: 78, Height: 33, Confidence: 88, PageIndex: 0},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DeduplicateElements(elements)
	}
}

func BenchmarkInferElementType_AllTypes(b *testing.B) {
	tests := []struct {
		text   string
		width  int
		height int
	}{
		{"Email:", 50, 20},
		{"Save", 60, 30},
		{"Welcome", 300, 32},
		{"https://example.com", 150, 20},
		{"$99.99", 60, 20},
		{"Body text", 200, 14},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tt := range tests {
			InferElementType(tt.text, tt.width, tt.height)
		}
	}
}

func BenchmarkAssociateLabels_LargeForm(b *testing.B) {
	elements := make([]Element, 20)
	for i := 0; i < 10; i++ {
		elements[i*2] = Element{
			Text:   "Label",
			X:      10,
			Y:      10 + i*40,
			Width:  50,
			Height: 20,
			Type:   ElementTypeLabel,
		}
		elements[i*2+1] = Element{
			Text:   "Value",
			X:      80,
			Y:      10 + i*40,
			Width:  150,
			Height: 20,
			Type:   ElementTypeText,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AssociateLabels(elements)
	}
}

// =============================================================================
// GetPageScreenshot
// =============================================================================

func TestGetPageScreenshot_NoView(t *testing.T) {
	l := New()
	l.Enable()
	if got := l.GetPageScreenshot(0); got != nil {
		t.Errorf("expected nil with no view, got %v", got)
	}
}

func TestGetPageScreenshot_PageExists(t *testing.T) {
	l := New()
	l.Enable()
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	l.SetView(&View{
		Pages: []PageSnapshot{{Index: 0, JPEG: jpeg}},
	})
	got := l.GetPageScreenshot(0)
	if len(got) != len(jpeg) {
		t.Errorf("expected %d bytes, got %d", len(jpeg), len(got))
	}
}

func TestGetPageScreenshot_PageMissing(t *testing.T) {
	l := New()
	l.Enable()
	l.SetView(&View{
		Pages: []PageSnapshot{{Index: 0, JPEG: []byte{1, 2, 3}}},
	})
	if got := l.GetPageScreenshot(1); got != nil {
		t.Errorf("expected nil for missing page, got %v", got)
	}
}

// =============================================================================
// isRadioText — symbol coverage
// =============================================================================

func TestIsRadioText_Symbols(t *testing.T) {
	for _, sym := range []string{"○", "●", "◉", "◎"} {
		if !isRadioText(sym) {
			t.Errorf("isRadioText(%q) = false, want true", sym)
		}
	}
}

// =============================================================================
// isSliderText — symbol coverage
// =============================================================================

func TestIsSliderText_Symbols(t *testing.T) {
	for _, sym := range []string{"─●", "▬●", "│●"} {
		if !isSliderText(sym) {
			t.Errorf("isSliderText(%q) = false, want true", sym)
		}
	}
}

// =============================================================================
// isNumericValue — currency symbol coverage
// =============================================================================

func TestIsNumericValue_Currency(t *testing.T) {
	cases := []string{"€99", "£50", "¥1000", "₹500", "$9.99"}
	for _, s := range cases {
		if !isNumericValue(s) {
			t.Errorf("isNumericValue(%q) = false, want true", s)
		}
	}
}
