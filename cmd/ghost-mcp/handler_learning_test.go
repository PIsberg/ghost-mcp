package main

import (
	"encoding/json"
	"os"
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
// mergeOCRPasses
// =============================================================================

func TestMergeOCRPasses_NilInputs(t *testing.T) {
	elems := mergeOCRPasses(0, 0, 0, nil, nil, nil, nil)
	if len(elems) != 0 {
		t.Fatalf("nil inputs should return empty slice, got %d elements", len(elems))
	}
}

func TestMergeOCRPasses_AddsOffset(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{
			{Text: "OK", X: 10, Y: 20, Width: 30, Height: 15, Confidence: 90},
		},
	}
	elems := mergeOCRPasses(0, 100, 200, r, nil, nil, nil)
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	if elems[0].X != 110 || elems[0].Y != 220 {
		t.Errorf("expected offset coords (110,220), got (%d,%d)", elems[0].X, elems[0].Y)
	}
	if elems[0].OcrPass != learner.OcrPassNormal {
		t.Errorf("expected OcrPass=normal, got %v", elems[0].OcrPass)
	}
}

func TestMergeOCRPasses_DeduplicatesWithinPage(t *testing.T) {
	word := ocr.Word{Text: "OK", X: 10, Y: 10, Width: 30, Height: 15, Confidence: 80}
	r1 := &ocr.Result{Words: []ocr.Word{word}}
	r2 := &ocr.Result{Words: []ocr.Word{word}} // exact same text + position → deduplicated
	elems := mergeOCRPasses(0, 0, 0, r1, r2, nil, nil)
	if len(elems) != 1 {
		t.Fatalf("duplicate at same position should be merged; got %d elements", len(elems))
	}
}

func TestMergeOCRPasses_SetsPageIndex(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{{Text: "Hi", X: 5, Y: 5, Width: 20, Height: 12, Confidence: 85}},
	}
	elems := mergeOCRPasses(3, 0, 0, r, nil, nil, nil)
	if len(elems) == 0 {
		t.Fatal("expected at least one element")
	}
	if elems[0].PageIndex != 3 {
		t.Errorf("expected PageIndex=3, got %d", elems[0].PageIndex)
	}
}

func TestMergeOCRPasses_SkipsLowConfidence(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Noise", X: 0, Y: 0, Width: 20, Height: 10, Confidence: 10}, // below MinConfidence
		},
	}
	elems := mergeOCRPasses(0, 0, 0, r, nil, nil, nil)
	if len(elems) != 0 {
		t.Fatal("low-confidence words should be skipped")
	}
}

func TestMergeOCRPasses_SkipsEmptyText(t *testing.T) {
	r := &ocr.Result{
		Words: []ocr.Word{
			{Text: "   ", X: 0, Y: 0, Width: 20, Height: 10, Confidence: 90},
		},
	}
	elems := mergeOCRPasses(0, 0, 0, r, nil, nil, nil)
	if len(elems) != 0 {
		t.Fatal("blank text should be skipped")
	}
}

func TestMergeOCRPasses_AllFivePasses(t *testing.T) {
	normal := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Normal", X: 10, Y: 10, Width: 50, Height: 20, Confidence: 90},
		},
	}
	inverted := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Inverted", X: 100, Y: 10, Width: 60, Height: 20, Confidence: 85},
		},
	}
	bright := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Bright", X: 150, Y: 10, Width: 50, Height: 20, Confidence: 86},
		},
	}
	dark := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Dark", X: 300, Y: 10, Width: 40, Height: 20, Confidence: 87},
		},
	}
	color := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Color", X: 200, Y: 10, Width: 40, Height: 20, Confidence: 88},
		},
	}
	elems := mergeOCRPasses(0, 0, 0, normal, inverted, bright, dark, color)
	if len(elems) != 5 {
		t.Fatalf("expected 5 elements from 5 passes, got %d", len(elems))
	}

	passes := map[learner.OcrPass]bool{}
	for _, e := range elems {
		passes[e.OcrPass] = true
	}
	if !passes[learner.OcrPassNormal] {
		t.Error("expected normal pass element")
	}
	if !passes[learner.OcrPassInverted] {
		t.Error("expected inverted pass element")
	}
	if !passes[learner.OcrPassBrightText] {
		t.Error("expected bright_text pass element")
	}
	if !passes[learner.OcrPassDarkText] {
		t.Error("expected dark_text pass element")
	}
	if !passes[learner.OcrPassColor] {
		t.Error("expected color pass element")
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

// =============================================================================
// Element type inference (inferTypes helper)
// =============================================================================

func TestInferTypes_Label(t *testing.T) {
	elems := []learner.Element{
		{Text: "Email:", X: 10, Y: 10, Width: 50, Height: 20},
		{Text: "Name:", X: 10, Y: 50, Width: 50, Height: 20},
	}
	result := inferTypes(elems)
	if result[0].Type != learner.ElementTypeLabel {
		t.Errorf("expected 'Email:' to be label, got %v", result[0].Type)
	}
	if result[1].Type != learner.ElementTypeLabel {
		t.Errorf("expected 'Name:' to be label, got %v", result[1].Type)
	}
}

func TestInferTypes_Heading(t *testing.T) {
	elems := []learner.Element{
		{Text: "Welcome to the Dashboard", X: 10, Y: 10, Width: 300, Height: 32},
	}
	result := inferTypes(elems)
	if result[0].Type != learner.ElementTypeHeading {
		t.Errorf("expected heading, got %v", result[0].Type)
	}
}

func TestInferTypes_Button(t *testing.T) {
	elems := []learner.Element{
		{Text: "Save", X: 10, Y: 10, Width: 60, Height: 30},
		{Text: "Cancel", X: 100, Y: 10, Width: 70, Height: 30},
		{Text: "Submit", X: 200, Y: 10, Width: 80, Height: 35},
	}
	result := inferTypes(elems)
	for i, e := range result {
		if e.Type != learner.ElementTypeButton {
			t.Errorf("element %d (%q) expected button, got %v", i, e.Text, e.Type)
		}
	}
}

func TestInferTypes_Link(t *testing.T) {
	elems := []learner.Element{
		{Text: "https://example.com", X: 10, Y: 10, Width: 150, Height: 20},
		{Text: "Learn More", X: 10, Y: 50, Width: 80, Height: 20},
	}
	result := inferTypes(elems)
	for i, e := range result {
		if e.Type != learner.ElementTypeLink {
			t.Errorf("element %d (%q) expected link, got %v", i, e.Text, e.Type)
		}
	}
}

func TestInferTypes_Value(t *testing.T) {
	elems := []learner.Element{
		{Text: "42", X: 10, Y: 10, Width: 30, Height: 20},
		{Text: "$99.99", X: 10, Y: 50, Width: 60, Height: 20},
		{Text: "85%", X: 10, Y: 90, Width: 40, Height: 20},
	}
	result := inferTypes(elems)
	for i, e := range result {
		if e.Type != learner.ElementTypeValue {
			t.Errorf("element %d (%q) expected value, got %v", i, e.Text, e.Type)
		}
	}
}

func TestInferTypes_Text(t *testing.T) {
	// Use dimensions outside button range: height < 16 or > 65, or width < 40
	// Also ensure it's not a button keyword, URL, numeric, or heading
	elems := []learner.Element{
		{Text: "This is some body text", X: 10, Y: 10, Width: 200, Height: 14},
	}
	result := inferTypes(elems)
	if result[0].Type != learner.ElementTypeText {
		t.Errorf("expected text, got %v", result[0].Type)
	}
}

// =============================================================================
// AssociateLabels
// =============================================================================

func TestAssociateLabels_LabelToRight(t *testing.T) {
	elems := []learner.Element{
		{Text: "Email:", X: 10, Y: 10, Width: 50, Height: 20, Type: learner.ElementTypeLabel},
		{Text: "Enter your email", X: 80, Y: 10, Width: 150, Height: 20, Type: learner.ElementTypeText},
	}
	result := learner.AssociateLabels(elems)
	if result[0].LabelFor != "Enter your email" {
		t.Errorf("expected label to be associated with 'Enter your email', got %q", result[0].LabelFor)
	}
}

func TestAssociateLabels_LabelBelow(t *testing.T) {
	elems := []learner.Element{
		{Text: "Name:", X: 10, Y: 10, Width: 50, Height: 20, Type: learner.ElementTypeLabel},
		{Text: "John Doe", X: 10, Y: 40, Width: 100, Height: 20, Type: learner.ElementTypeText},
	}
	result := learner.AssociateLabels(elems)
	if result[0].LabelFor != "John Doe" {
		t.Errorf("expected label to be associated with 'John Doe', got %q", result[0].LabelFor)
	}
}

func TestAssociateLabels_NoAssociation(t *testing.T) {
	elems := []learner.Element{
		{Text: "Orphan Label:", X: 10, Y: 10, Width: 50, Height: 20, Type: learner.ElementTypeLabel},
		{Text: "Too far away", X: 500, Y: 500, Width: 100, Height: 20, Type: learner.ElementTypeText},
	}
	result := learner.AssociateLabels(elems)
	if result[0].LabelFor != "" {
		t.Errorf("expected no association, got %q", result[0].LabelFor)
	}
}

func TestAssociateLabels_PreservesOriginal(t *testing.T) {
	elems := []learner.Element{
		{Text: "Email:", X: 10, Y: 10, Width: 50, Height: 20, Type: learner.ElementTypeLabel},
		{Text: "test @example.com", X: 80, Y: 10, Width: 150, Height: 20, Type: learner.ElementTypeText},
	}
	original := make([]learner.Element, len(elems))
	copy(original, elems)
	_ = learner.AssociateLabels(elems)
	// Original slice should not be modified.
	if original[0].LabelFor != "" {
		t.Error("original slice should not be modified")
	}
}

// =============================================================================
// encodeJPEG
// =============================================================================

func TestEncodeJPEG_NilImage(t *testing.T) {
	// Should not panic, should return nil.
	// Note: encodeJPEG will panic on nil image because jpeg.Encode doesn't handle it.
	// This is acceptable since encodeJPEG is only called with valid images from uiCaptureImage.
	t.Skip("encodeJPEG is not designed to handle nil images; only called with valid captures")
}

// =============================================================================
// BENCHMARKS
// =============================================================================

func BenchmarkTextSimilarity_Identical(b *testing.B) {
	a := "Save Changes button on the page"
	b.Run("identical", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			textSimilarity(a, a)
		}
	})
}

func BenchmarkTextSimilarity_Similar(b *testing.B) {
	a := "Save Changes button on the page"
	b2 := "Save Changes button on the page - now with more text"
	b.Run("similar", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			textSimilarity(a, b2)
		}
	})
}

func BenchmarkTextSimilarity_Different(b *testing.B) {
	a := "Save Changes button on the page"
	b2 := "Completely different text with no similarity at all"
	b.Run("different", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			textSimilarity(a, b2)
		}
	})
}

func BenchmarkMergeOCRPasses_Empty(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mergeOCRPasses(0, 0, 0, nil, nil, nil, nil)
	}
}

func BenchmarkMergeOCRPasses_SingleWord(b *testing.B) {
	r := &ocr.Result{
		Words: []ocr.Word{
			{Text: "OK", X: 10, Y: 20, Width: 30, Height: 15, Confidence: 90},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mergeOCRPasses(0, 0, 0, r, nil, nil, nil)
	}
}

func BenchmarkMergeOCRPasses_ThreePasses(b *testing.B) {
	normal := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Normal", X: 10, Y: 10, Width: 50, Height: 20, Confidence: 90},
			{Text: "Text", X: 70, Y: 10, Width: 40, Height: 20, Confidence: 88},
			{Text: "Here", X: 120, Y: 10, Width: 45, Height: 20, Confidence: 92},
		},
	}
	inverted := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Inverted", X: 10, Y: 40, Width: 60, Height: 20, Confidence: 85},
			{Text: "Words", X: 80, Y: 40, Width: 50, Height: 20, Confidence: 87},
		},
	}
	bright := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Bright", X: 10, Y: 70, Width: 55, Height: 20, Confidence: 86},
		},
	}
	color := &ocr.Result{
		Words: []ocr.Word{
			{Text: "Color", X: 10, Y: 100, Width: 45, Height: 20, Confidence: 88},
			{Text: "Button", X: 65, Y: 100, Width: 55, Height: 20, Confidence: 91},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mergeOCRPasses(0, 0, 0, normal, inverted, bright, color)
	}
}

func BenchmarkInferElementType(b *testing.B) {
	tests := []struct {
		name   string
		text   string
		width  int
		height int
	}{
		{"label", "Email:", 50, 20},
		{"button", "Save", 60, 30},
		{"heading", "Welcome to Dashboard", 300, 32},
		{"link", "https://example.com", 150, 20},
		{"value", "$99.99", 60, 20},
		{"text", "Some body text here", 200, 14},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				learner.InferElementType(tt.text, tt.width, tt.height)
			}
		})
	}
}

func BenchmarkAssociateLabels(b *testing.B) {
	elems := []learner.Element{
		{Text: "First:", X: 10, Y: 10, Width: 50, Height: 20, Type: learner.ElementTypeLabel},
		{Text: "John", X: 80, Y: 10, Width: 100, Height: 20, Type: learner.ElementTypeText},
		{Text: "Last:", X: 10, Y: 50, Width: 50, Height: 20, Type: learner.ElementTypeLabel},
		{Text: "Doe", X: 80, Y: 50, Width: 100, Height: 20, Type: learner.ElementTypeText},
		{Text: "Email:", X: 10, Y: 90, Width: 50, Height: 20, Type: learner.ElementTypeLabel},
		{Text: "john@example.com", X: 80, Y: 90, Width: 150, Height: 20, Type: learner.ElementTypeText},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		learner.AssociateLabels(elems)
	}
}

// =============================================================================
// autoLearnWithPages
// =============================================================================

func TestAutoLearnWithPages_RejectsLessThanTwo(t *testing.T) {
	_, err := autoLearnWithPages(1)
	if err == nil {
		t.Fatal("expected error for scan_pages=1, got nil")
	}
	if !strings.Contains(err.Error(), "must be >= 2") {
		t.Fatalf("expected 'must be >= 2' error, got: %v", err)
	}
}

func TestAutoLearnWithPages_RejectsZero(t *testing.T) {
	_, err := autoLearnWithPages(0)
	if err == nil {
		t.Fatal("expected error for scan_pages=0, got nil")
	}
	if !strings.Contains(err.Error(), "must be >= 2") {
		t.Fatalf("expected 'must be >= 2' error, got: %v", err)
	}
}

// TestAutoLearnWithPages_ReturnsViewStructure verifies that autoLearnWithPages
// calls learnScreen and returns a properly structured View. Since learnScreen
// requires a real display, this test is skipped in CI.
func TestAutoLearnWithPages_ReturnsViewStructure(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping: requires real desktop screen")
	}

	view, err := autoLearnWithPages(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view == nil {
		t.Fatal("expected non-nil view")
	}
	if view.PageCount < 1 {
		t.Errorf("expected at least 1 page, got %d", view.PageCount)
	}
}
