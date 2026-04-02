package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/learner"
	"github.com/ghost-mcp/internal/ocr"
	"github.com/mark3labs/mcp-go/mcp"
)

// makeLearnReq builds a CallToolRequest with the given argument map.
func makeLearnReq(args map[string]interface{}) mcp.CallToolRequest {
	if args == nil {
		args = map[string]interface{}{}
	}
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: args},
	}
}

// textFromResult extracts the text content from the first content item.
func textFromResult(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// =============================================================================
// textSimilarity
// =============================================================================

func TestTextSimilarity_Identical(t *testing.T) {
	if s := textSimilarity("hello world", "hello world"); s != 1.0 {
		t.Fatalf("identical strings should give 1.0, got %f", s)
	}
}

func TestTextSimilarity_Disjoint(t *testing.T) {
	s := textSimilarity("abcdef", "zyxwvu")
	if s > 0.1 {
		t.Fatalf("completely different strings should have low similarity, got %f", s)
	}
}

func TestTextSimilarity_ShortStrings(t *testing.T) {
	// Strings shorter than 3 runes cannot form trigrams, so dissimilar short
	// strings return 0. (Identical strings still return 1.0 via early exit.)
	if s := textSimilarity("ab", "cd"); s != 0.0 {
		t.Fatalf("dissimilar short strings should return 0, got %f", s)
	}
	if s := textSimilarity("ab", "ab"); s != 1.0 {
		t.Fatalf("identical short strings should return 1.0, got %f", s)
	}
}

func TestTextSimilarity_HighForSimilar(t *testing.T) {
	a := "Save Changes button on the page"
	b := "Save Changes button on the page - now with more text"
	s := textSimilarity(a, b)
	if s < 0.5 {
		t.Fatalf("highly similar strings should score >0.5, got %f", s)
	}
}

// =============================================================================
// mergeOCRResults
// =============================================================================

func TestMergeOCRResults_NilInputs(t *testing.T) {
	elems := mergeOCRResults(0, 0, 0, nil, nil)
	if len(elems) != 0 {
		t.Fatalf("nil inputs should return empty slice, got %d elements", len(elems))
	}
}

func TestMergeOCRResults_AddsOffset(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{
			{Text: "OK", X: 10, Y: 20, Width: 30, Height: 15, Confidence: 90},
		},
	}
	elems := mergeOCRResults(0, 100, 200, r, nil)
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	if elems[0].X != 110 || elems[0].Y != 220 {
		t.Errorf("expected offset coords (110,220), got (%d,%d)", elems[0].X, elems[0].Y)
	}
}

func TestMergeOCRResults_DeduplicatesWithinPage(t *testing.T) {
	word := ocr.Word{Text: "OK", X: 10, Y: 10, Width: 30, Height: 15, Confidence: 80}
	r1 := &ocr.Result{Words: []ocr.Word{word}}
	r2 := &ocr.Result{Words: []ocr.Word{word}} // exact same text + position → deduplicated
	elems := mergeOCRResults(0, 0, 0, r1, r2)
	if len(elems) != 1 {
		t.Fatalf("duplicate at same position should be merged; got %d elements", len(elems))
	}
}

func TestMergeOCRResults_SetsPageIndex(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{{Text: "Hi", X: 5, Y: 5, Width: 20, Height: 12, Confidence: 85}},
	}
	elems := mergeOCRResults(3, 0, 0, r, nil)
	if len(elems) == 0 {
		t.Fatal("expected at least one element")
	}
	if elems[0].PageIndex != 3 {
		t.Errorf("expected PageIndex=3, got %d", elems[0].PageIndex)
	}
}

func TestMergeOCRResults_SkipsLowConfidence(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Noise", X: 0, Y: 0, Width: 20, Height: 10, Confidence: 10}, // below MinConfidence
		},
	}
	elems := mergeOCRResults(0, 0, 0, r, nil)
	if len(elems) != 0 {
		t.Fatal("low-confidence words should be skipped")
	}
}

func TestMergeOCRResults_SkipsEmptyText(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{
			{Text: "   ", X: 0, Y: 0, Width: 20, Height: 10, Confidence: 90},
		},
	}
	elems := mergeOCRResults(0, 0, 0, r, nil)
	if len(elems) != 0 {
		t.Fatal("blank text should be skipped")
	}
}

// =============================================================================
// learnerRegionHint
// =============================================================================

func TestLearnerRegionHint_Disabled(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New() // disabled by default

	_, _, _, _, _, ok := learnerRegionHint("Save", 1920, 1080)
	if ok {
		t.Fatal("should return no hint when learner is disabled")
	}
}

func TestLearnerRegionHint_NoView(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.Enable()

	_, _, _, _, _, ok := learnerRegionHint("Save", 1920, 1080)
	if ok {
		t.Fatal("should return no hint when no view exists")
	}
}

func TestLearnerRegionHint_MatchOnPage0(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{Text: "Save", X: 500, Y: 300, Width: 80, Height: 30, PageIndex: 0},
		},
		ScrollAmountUsed: 5,
		CapturedAt:       time.Now(),
		ScreenW:          1920,
		ScreenH:          1080,
	})

	x, y, _, _, scrolls, ok := learnerRegionHint("Save", 1920, 1080)
	if !ok {
		t.Fatal("expected hint to be found")
	}
	if scrolls != 0 {
		t.Errorf("page 0 element should need 0 scrolls, got %d", scrolls)
	}
	// Region top-left should be padded above/left of the element.
	if x > 500 || y > 300 {
		t.Errorf("region (%d,%d) should be before element at (500,300)", x, y)
	}
}

func TestLearnerRegionHint_ScrollPageElement(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{Text: "Footer", X: 100, Y: 200, Width: 150, Height: 40, PageIndex: 2},
		},
		ScrollAmountUsed: 5,
		CapturedAt:       time.Now(),
		ScreenW:          1920,
		ScreenH:          1080,
	})

	_, _, _, _, scrolls, ok := learnerRegionHint("Footer", 1920, 1080)
	if !ok {
		t.Fatal("expected hint to be found")
	}
	// page_index=2 * scroll_amount=5 = 10 scroll ticks
	if scrolls != 10 {
		t.Errorf("expected 10 scrolls for page 2 with amount 5, got %d", scrolls)
	}
}

func TestLearnerRegionHint_NoMatch(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.Enable()
	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{Text: "Cancel", X: 100, Y: 100, Width: 80, Height: 30, PageIndex: 0},
		},
		CapturedAt: time.Now(),
		ScreenW:    1920,
		ScreenH:    1080,
	})

	_, _, _, _, _, ok := learnerRegionHint("NonExistentButton", 1920, 1080)
	if ok {
		t.Fatal("should return no hint for unmatched text")
	}
}

// =============================================================================
// extractText
// =============================================================================

func TestExtractText_Nil(t *testing.T) {
	if s := extractText(nil); s != "" {
		t.Fatalf("nil result should yield empty string, got %q", s)
	}
}

func TestExtractText_Normal(t *testing.T) {
	r := &ocr.Result{Text: "  Hello World  "}
	if s := extractText(r); s != "Hello World" {
		t.Fatalf("expected trimmed text, got %q", s)
	}
}

// =============================================================================
// handleLearnScreen — validation only (no robotgo calls)
// =============================================================================

func TestHandleLearnScreen_InvalidScrollDirection(t *testing.T) {
	req := makeLearnReq(map[string]interface{}{
		"scroll_direction": "sideways",
	})
	result, err := handleLearnScreen(nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid scroll_direction")
	}
	text := textFromResult(t, result)
	if !strings.Contains(text, "sideways") {
		t.Errorf("error should mention the invalid value; got %q", text)
	}
}

// =============================================================================
// handleGetLearnedView
// =============================================================================

func TestHandleGetLearnedView_NoView(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()

	result, err := handleGetLearnedView(nil, makeLearnReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("get_learned_view should not error on missing view")
	}
	text := textFromResult(t, result)
	if !strings.Contains(text, `"learned":false`) {
		t.Errorf("expected learned:false in response; got %s", text)
	}
}

func TestHandleGetLearnedView_WithView(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{Text: "OK", X: 10, Y: 10, Width: 50, Height: 20, PageIndex: 0},
		},
		PageCount:  1,
		CapturedAt: time.Now(),
		ScreenW:    1280,
		ScreenH:    800,
	})

	result, err := handleGetLearnedView(nil, makeLearnReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	text := textFromResult(t, result)

	if !strings.Contains(text, `"learned":true`) {
		t.Errorf("expected learned:true; got %s", text)
	}
	if !strings.Contains(text, `"OK"`) {
		t.Errorf("expected element text in response; got %s", text)
	}

	// Verify it's valid JSON.
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, text)
	}
	if pc, ok := v["page_count"].(float64); !ok || pc != 1 {
		t.Errorf("expected page_count=1, got %v", v["page_count"])
	}
}

// =============================================================================
// handleClearLearnedView
// =============================================================================

func TestHandleClearLearnedView(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.SetView(&learner.View{
		Elements:   []learner.Element{{Text: "X"}},
		CapturedAt: time.Now(),
	})

	result, err := handleClearLearnedView(nil, makeLearnReq(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("unexpected error result")
	}
	if globalLearner.HasView() {
		t.Fatal("view should be cleared after handleClearLearnedView")
	}
}

// =============================================================================
// handleSetLearningMode
// =============================================================================

func TestHandleSetLearningMode_Enable(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()

	req := makeLearnReq(map[string]interface{}{"enabled": true})
	result, err := handleSetLearningMode(nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if !globalLearner.IsEnabled() {
		t.Fatal("learner should be enabled")
	}
	text := textFromResult(t, result)
	if !strings.Contains(text, `"learning_mode":true`) {
		t.Errorf("expected learning_mode:true in response; got %s", text)
	}
}

func TestHandleSetLearningMode_Disable(t *testing.T) {
	orig := globalLearner
	defer func() { globalLearner = orig }()
	globalLearner = learner.New()
	globalLearner.Enable()

	req := makeLearnReq(map[string]interface{}{"enabled": false})
	result, err := handleSetLearningMode(nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("unexpected error")
	}
	if globalLearner.IsEnabled() {
		t.Fatal("learner should be disabled")
	}
	text := textFromResult(t, result)
	if !strings.Contains(text, `"learning_mode":false`) {
		t.Errorf("expected learning_mode:false in response; got %s", text)
	}
}
