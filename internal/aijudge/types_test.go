package aijudge

import (
	"math"
	"strings"
	"testing"
)

// =============================================================================
// IoU tests
// =============================================================================

func TestComputeIoU_ExactOverlap(t *testing.T) {
	a := Rect{X: 10, Y: 10, Width: 50, Height: 30}
	b := Rect{X: 10, Y: 10, Width: 50, Height: 30}

	iou := computeIoU(a, b)
	if iou != 1.0 {
		t.Errorf("expected IoU 1.0 for identical rects, got %.4f", iou)
	}
}

func TestComputeIoU_NoOverlap(t *testing.T) {
	a := Rect{X: 0, Y: 0, Width: 10, Height: 10}
	b := Rect{X: 100, Y: 100, Width: 10, Height: 10}

	iou := computeIoU(a, b)
	if iou != 0.0 {
		t.Errorf("expected IoU 0.0 for non-overlapping rects, got %.4f", iou)
	}
}

func TestComputeIoU_PartialOverlap(t *testing.T) {
	a := Rect{X: 0, Y: 0, Width: 20, Height: 20}
	b := Rect{X: 10, Y: 10, Width: 20, Height: 20}

	// Intersection: 10x10 = 100
	// Union: 400 + 400 - 100 = 700
	expected := 100.0 / 700.0
	iou := computeIoU(a, b)
	if math.Abs(iou-expected) > 0.001 {
		t.Errorf("expected IoU %.4f, got %.4f", expected, iou)
	}
}

func TestComputeIoU_ContainedRect(t *testing.T) {
	outer := Rect{X: 0, Y: 0, Width: 100, Height: 100}
	inner := Rect{X: 25, Y: 25, Width: 50, Height: 50}

	// Intersection: 50x50 = 2500
	// Union: 10000 + 2500 - 2500 = 10000
	expected := 2500.0 / 10000.0
	iou := computeIoU(outer, inner)
	if math.Abs(iou-expected) > 0.001 {
		t.Errorf("expected IoU %.4f, got %.4f", expected, iou)
	}
}

func TestComputeIoU_EdgeTouch(t *testing.T) {
	a := Rect{X: 0, Y: 0, Width: 10, Height: 10}
	b := Rect{X: 10, Y: 0, Width: 10, Height: 10} // touches right edge

	iou := computeIoU(a, b)
	if iou != 0.0 {
		t.Errorf("expected IoU 0.0 for edge-touching rects, got %.4f", iou)
	}
}

func TestComputeIoU_ZeroArea(t *testing.T) {
	a := Rect{X: 0, Y: 0, Width: 0, Height: 10}
	b := Rect{X: 0, Y: 0, Width: 10, Height: 10}

	iou := computeIoU(a, b)
	if iou != 0.0 {
		t.Errorf("expected IoU 0.0 for zero-area rect, got %.4f", iou)
	}
}

// =============================================================================
// Text similarity tests
// =============================================================================

func TestTextSimilarity_ExactMatch(t *testing.T) {
	score := textSimilarity("Submit", "Submit")
	if score != 1.0 {
		t.Errorf("exact match should be 1.0, got %.4f", score)
	}
}

func TestTextSimilarity_CaseInsensitive(t *testing.T) {
	score := textSimilarity("SUBMIT", "submit")
	if score != 1.0 {
		t.Errorf("case insensitive match should be 1.0, got %.4f", score)
	}
}

func TestTextSimilarity_Containment(t *testing.T) {
	score := textSimilarity("Click Submit", "Submit")
	if score < 0.5 {
		t.Errorf("containment should score >= 0.5, got %.4f", score)
	}
}

func TestTextSimilarity_NoMatch(t *testing.T) {
	score := textSimilarity("Primary", "Dropdown")
	if score > 0.3 {
		t.Errorf("unrelated strings should score low, got %.4f", score)
	}
}

func TestTextSimilarity_EmptyString(t *testing.T) {
	if textSimilarity("", "hello") != 0.0 {
		t.Error("empty string should score 0")
	}
	if textSimilarity("hello", "") != 0.0 {
		t.Error("empty string should score 0")
	}
}

func TestTextSimilarity_OCRError(t *testing.T) {
	// Common OCR misread: "PR|MARY" for "PRIMARY"
	score := textSimilarity("primary", "pr|mary")
	// Should have non-zero trigram similarity
	if score == 0.0 {
		t.Error("OCR error variant should have non-zero similarity")
	}
}

// =============================================================================
// Trigram tests
// =============================================================================

func TestTrigramSimilarity_Identical(t *testing.T) {
	score := trigramSimilarity("button", "button")
	if score != 1.0 {
		t.Errorf("identical strings should have trigram similarity 1.0, got %.4f", score)
	}
}

func TestTrigramSimilarity_Similar(t *testing.T) {
	score := trigramSimilarity("button", "buttan")
	if score < 0.4 {
		t.Errorf("similar strings should have moderate trigram similarity, got %.4f", score)
	}
}

func TestTrigramSimilarity_Short(t *testing.T) {
	// Strings shorter than 3 chars can't produce trigrams
	score := trigramSimilarity("ab", "xy")
	if score != 0.0 {
		t.Errorf("strings shorter than 3 chars should score 0, got %.4f", score)
	}
}

// =============================================================================
// normalizeType tests
// =============================================================================

func TestNormalizeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"button", "button"},
		{"btn", "button"},
		{"Button", "button"},
		{"HEADING", "heading"},
		{"h1", "heading"},
		{"header", "heading"},
		{"select", "dropdown"},
		{"combobox", "dropdown"},
		{"drop_down", "dropdown"},
		{"check_box", "checkbox"},
		{"radio_button", "radio"},
		{"text_input", "input"},
		{"hyperlink", "link"},
		{"anchor", "link"},
		{"switch", "toggle"},
		{"range_input", "slider"},
		{"image", "icon"},
		{"paragraph", "text"},
		{"static_text", "text"},
		{"number", "value"},
		{"something_custom", "something_custom"}, // pass-through
	}

	for _, tt := range tests {
		got := normalizeType(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeType(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// =============================================================================
// CompareResults tests
// =============================================================================

func TestCompareResults_ExactMatch(t *testing.T) {
	ghost := []GhostElement{
		{ID: 1, Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
		{ID: 2, Text: "Cancel", Type: "button", Rect: Rect{200, 100, 60, 30}},
	}
	judge := []JudgedElement{
		{Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
		{Text: "Cancel", Type: "button", Rect: Rect{200, 100, 60, 30}},
	}

	report := CompareResults("test", ghost, judge, DefaultCompareConfig())

	if report.MatchedCount != 2 {
		t.Errorf("expected 2 matches, got %d", report.MatchedCount)
	}
	if report.Precision != 1.0 {
		t.Errorf("expected precision 1.0, got %.2f", report.Precision)
	}
	if report.Recall != 1.0 {
		t.Errorf("expected recall 1.0, got %.2f", report.Recall)
	}
	if report.F1 != 1.0 {
		t.Errorf("expected F1 1.0, got %.2f", report.F1)
	}
	if report.TypeAccuracy != 1.0 {
		t.Errorf("expected type accuracy 1.0, got %.2f", report.TypeAccuracy)
	}
}

func TestCompareResults_MissedElements(t *testing.T) {
	ghost := []GhostElement{
		{ID: 1, Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
	}
	judge := []JudgedElement{
		{Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
		{Text: "Cancel", Type: "button", Rect: Rect{200, 100, 60, 30}},
		{Text: "Reset", Type: "button", Rect: Rect{300, 100, 60, 30}},
	}

	report := CompareResults("test", ghost, judge, DefaultCompareConfig())

	if report.MatchedCount != 1 {
		t.Errorf("expected 1 match, got %d", report.MatchedCount)
	}
	if len(report.MissedByGhost) != 2 {
		t.Errorf("expected 2 missed, got %d", len(report.MissedByGhost))
	}
	if report.Recall > 0.34 {
		t.Errorf("recall should be ~0.33, got %.2f", report.Recall)
	}
	if report.Precision != 1.0 {
		t.Errorf("precision should be 1.0, got %.2f", report.Precision)
	}
}

func TestCompareResults_ExtraElements(t *testing.T) {
	ghost := []GhostElement{
		{ID: 1, Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
		{ID: 2, Text: "Cancel", Type: "button", Rect: Rect{200, 100, 60, 30}},
		{ID: 3, Text: "Garbage OCR Noise", Type: "text", Rect: Rect{50, 50, 100, 10}},
	}
	judge := []JudgedElement{
		{Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
		{Text: "Cancel", Type: "button", Rect: Rect{200, 100, 60, 30}},
	}

	report := CompareResults("test", ghost, judge, DefaultCompareConfig())

	if report.MatchedCount != 2 {
		t.Errorf("expected 2 matches, got %d", report.MatchedCount)
	}
	if len(report.FalsePositives) != 1 {
		t.Errorf("expected 1 false positive, got %d", len(report.FalsePositives))
	}
	if report.Recall != 1.0 {
		t.Errorf("recall should be 1.0, got %.2f", report.Recall)
	}
}

func TestCompareResults_TypeMismatch(t *testing.T) {
	ghost := []GhostElement{
		{ID: 1, Text: "Submit", Type: "heading", Rect: Rect{100, 100, 60, 30}}, // wrong type
	}
	judge := []JudgedElement{
		{Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
	}

	report := CompareResults("test", ghost, judge, DefaultCompareConfig())

	if report.MatchedCount != 1 {
		t.Errorf("expected 1 match (text matches), got %d", report.MatchedCount)
	}
	if report.TypeAccuracy != 0.0 {
		t.Errorf("type accuracy should be 0.0, got %.2f", report.TypeAccuracy)
	}
	if len(report.TypeMismatches) != 1 {
		t.Errorf("expected 1 type mismatch, got %d", len(report.TypeMismatches))
	}
}

func TestCompareResults_EmptyBoth(t *testing.T) {
	report := CompareResults("empty", nil, nil, DefaultCompareConfig())

	if report.Precision != 1.0 || report.Recall != 1.0 || report.F1 != 1.0 {
		t.Errorf("empty lists should produce perfect scores")
	}
}

func TestCompareResults_FuzzyTextMatch(t *testing.T) {
	ghost := []GhostElement{
		{ID: 1, Text: "Click Me!", Type: "button", Rect: Rect{100, 100, 80, 30}},
	}
	judge := []JudgedElement{
		{Text: "Click Me", Type: "button", Rect: Rect{100, 100, 80, 30}}, // missing !
	}

	report := CompareResults("test", ghost, judge, DefaultCompareConfig())

	if report.MatchedCount != 1 {
		t.Errorf("should fuzzy-match 'Click Me!' and 'Click Me', got %d matches", report.MatchedCount)
	}
}

func TestCompareResults_WithIoUThreshold(t *testing.T) {
	ghost := []GhostElement{
		{ID: 1, Text: "Submit", Type: "button", Rect: Rect{100, 100, 60, 30}},
	}
	judge := []JudgedElement{
		{Text: "Submit", Type: "button", Rect: Rect{500, 500, 60, 30}}, // far away
	}

	cfg := CompareConfig{MinTextScore: 0.5, MinIoU: 0.1}
	report := CompareResults("test", ghost, judge, cfg)

	// Should NOT match because IoU is 0
	if report.MatchedCount != 0 {
		t.Errorf("expected 0 matches with IoU threshold, got %d", report.MatchedCount)
	}
}

// =============================================================================
// Report rendering tests
// =============================================================================

func TestAccuracyReport_String(t *testing.T) {
	report := &AccuracyReport{
		FixtureName:  "test_fixture",
		GhostCount:   10,
		JudgeCount:   12,
		MatchedCount: 8,
		Precision:    0.80,
		Recall:       0.667,
		F1:           0.727,
		TypeAccuracy: 0.875,
		MissedByGhost: []JudgedElement{
			{Text: "Missed1", Type: "button"},
		},
		FalsePositives: []GhostElement{
			{Text: "FP1", Type: "text", Confidence: 45},
		},
		TypeMismatches: []TypeMismatch{
			{Text: "Ambiguous", GhostType: "heading", JudgeType: "button"},
		},
	}

	s := report.String()
	if s == "" {
		t.Fatal("report string should not be empty")
	}
	if !containsStr(s, "test_fixture") {
		t.Error("report should contain fixture name")
	}
	if !containsStr(s, "80.0%") {
		t.Errorf("report should contain precision, got:\n%s", s)
	}
	if !containsStr(s, "Missed1") {
		t.Error("report should list missed elements")
	}
	if !containsStr(s, "FP1") {
		t.Error("report should list false positives")
	}
	if !containsStr(s, "Ambiguous") {
		t.Error("report should list type mismatches")
	}
}

func TestRect_Center(t *testing.T) {
	r := Rect{X: 10, Y: 20, Width: 40, Height: 60}
	cx, cy := r.Center()
	if cx != 30 || cy != 50 {
		t.Errorf("center of (10,20,40,60) should be (30,50), got (%d,%d)", cx, cy)
	}
}

func TestRect_Area(t *testing.T) {
	r := Rect{X: 0, Y: 0, Width: 10, Height: 20}
	if r.Area() != 200 {
		t.Errorf("area should be 200, got %d", r.Area())
	}
}

func TestRect_Area_Zero(t *testing.T) {
	r := Rect{X: 0, Y: 0, Width: 0, Height: 20}
	if r.Area() != 0 {
		t.Errorf("zero-width rect should have area 0, got %d", r.Area())
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && strings.Contains(s, substr)
}
