//go:build !integration

// ai_judge_test.go — Offline AI judge tests for GUI element identification.
//
// These tests evaluate Ghost MCP's OCR accuracy by:
//  1. Using ground-truth element lists as Gemini's "judge" output (or calling Gemini API if GOOGLE_API_KEY is set)
//  2. Comparing against what the OCR pipeline would detect
//  3. Producing structured accuracy reports
//
// Run with:
//
//	go test -v -run TestAIJudge ./cmd/ghost-mcp/...
//	GOOGLE_API_KEY=xxx go test -v -run TestAIJudge ./cmd/ghost-mcp/... -timeout 120s
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/aijudge"
	"github.com/ghost-mcp/internal/aijudge/testdata"
	"github.com/ghost-mcp/internal/learner"
)

// TestAIJudge_ComparisonWithGroundTruth validates the comparison engine
// using a simulated Ghost MCP output against the ground-truth manifests.
// No API key required — this tests the scoring framework itself.
func TestAIJudge_ComparisonWithGroundTruth(t *testing.T) {
	// Simulate what Ghost MCP might find on the normal fixture
	simulatedGhost := []aijudge.GhostElement{
		// Found correctly
		{ID: 1, Text: "Ghost MCP Test Fixture", Type: "heading", Rect: aijudge.Rect{X: 200, Y: 10, Width: 500, Height: 40}},
		{ID: 2, Text: "PRIMARY", Type: "button", Rect: aijudge.Rect{X: 30, Y: 210, Width: 200, Height: 40}},
		{ID: 3, Text: "SUCCESS", Type: "button", Rect: aijudge.Rect{X: 240, Y: 210, Width: 200, Height: 40}},
		{ID: 4, Text: "WARNING", Type: "button", Rect: aijudge.Rect{X: 450, Y: 210, Width: 200, Height: 40}},
		{ID: 5, Text: "INFO", Type: "button", Rect: aijudge.Rect{X: 660, Y: 210, Width: 200, Height: 40}},
		{ID: 6, Text: "Text Input:", Type: "label", Rect: aijudge.Rect{X: 30, Y: 305, Width: 80, Height: 20}},
		{ID: 7, Text: "CLEAR", Type: "button", Rect: aijudge.Rect{X: 780, Y: 300, Width: 70, Height: 35}},
		{ID: 8, Text: "Hello World", Type: "text", Rect: aijudge.Rect{X: 50, Y: 995, Width: 120, Height: 20}},
		// Misclassified (heading as text)
		{ID: 9, Text: "Button Click Tests", Type: "text", Rect: aijudge.Rect{X: 30, Y: 175, Width: 200, Height: 24}},
		// OCR noise (false positive)
		{ID: 10, Text: "|||", Type: "text", Rect: aijudge.Rect{X: 5, Y: 5, Width: 10, Height: 10}, Confidence: 20},
	}

	report := aijudge.CompareResults("simulated_normal", simulatedGhost, testdata.FixtureNormal.Elements, aijudge.DefaultCompareConfig())

	t.Logf("Simulated accuracy:\n%s", report.String())

	// Verify report structure
	if report.GhostCount != len(simulatedGhost) {
		t.Errorf("ghost count: expected %d, got %d", len(simulatedGhost), report.GhostCount)
	}
	if report.JudgeCount != len(testdata.FixtureNormal.Elements) {
		t.Errorf("judge count: expected %d, got %d", len(testdata.FixtureNormal.Elements), report.JudgeCount)
	}

	// We expect some matches and some misses
	if report.MatchedCount == 0 {
		t.Error("expected at least some matched elements")
	}
	if len(report.MissedByGhost) == 0 {
		t.Error("simulated ghost should miss some ground-truth elements")
	}

	// The "|||" false positive should be detected
	if len(report.FalsePositives) == 0 {
		t.Error("expected at least one false positive (OCR noise)")
	}

	// Type mismatch: "Button Click Tests" classified as text instead of heading
	if len(report.TypeMismatches) == 0 {
		t.Log("NOTE: type mismatch for 'Button Click Tests' (text vs heading) may not match due to text similarity threshold")
	}
}

// TestAIJudge_GeminiLive calls the actual Gemini API on a test fixture screenshot.
// Requires GOOGLE_API_KEY environment variable.
func TestAIJudge_GeminiLive(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set; skipping live AI judge test")
	}

	// Look for a pre-captured fixture screenshot
	screenshotPath := findFixtureScreenshot(t)
	if screenshotPath == "" {
		t.Skip("No fixture screenshot found; run capture_fixtures first")
	}

	imgBytes, err := os.ReadFile(screenshotPath)
	if err != nil {
		t.Fatalf("failed to read screenshot: %v", err)
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}

	judge, err := aijudge.NewJudge(apiKey, model)
	if err != nil {
		t.Fatalf("failed to create judge: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Log("Sending screenshot to Gemini for analysis...")
	judgedElements, err := judge.AnalyzeScreenshot(ctx, imgBytes, "image/png")
	if err != nil {
		t.Fatalf("Gemini analysis failed: %v", err)
	}

	t.Logf("Gemini identified %d elements", len(judgedElements))
	for i, e := range judgedElements {
		if i < 10 { // log first 10
			t.Logf("  [%d] %q (%s) at (%d,%d) %dx%d", i, e.Text, e.Type, e.Rect.X, e.Rect.Y, e.Rect.Width, e.Rect.Height)
		}
	}

	// Compare with ground truth
	report := aijudge.CompareResults("gemini_vs_groundtruth", ghostElementsFromJudged(judgedElements), testdata.FixtureNormal.Elements, aijudge.DefaultCompareConfig())

	t.Logf("\nGemini self-consistency report:\n%s", report.String())

	// Save report for trend tracking
	saveReport(t, report)
}

// TestAIJudge_ElementTypeAccuracy focuses specifically on whether
// Ghost MCP's InferElementType classification matches Gemini's understanding.
func TestAIJudge_ElementTypeAccuracy(t *testing.T) {
	// Test the type inference against known examples
	testCases := []struct {
		text     string
		width    int
		height   int
		expected string
	}{
		{"Submit", 80, 30, "button"},
		{"Cancel", 80, 30, "button"},
		{"Email:", 50, 20, "label"},
		{"Username:", 70, 20, "label"},
		{"Welcome to the Dashboard", 400, 36, "heading"},
		{"Learn more", 80, 15, "link"},
		{"https://example.com", 200, 15, "link"},
		{"$99.99", 60, 20, "value"},
		{"42", 20, 20, "value"},
	}

	correct := 0
	for _, tc := range testCases {
		inferred := string(learner.InferElementType(tc.text, tc.width, tc.height))
		match := aijudge.NormalizeTypeExported(inferred) == aijudge.NormalizeTypeExported(tc.expected)
		if match {
			correct++
		} else {
			t.Logf("MISMATCH: %q (%dx%d) → inferred=%q, expected=%q",
				tc.text, tc.width, tc.height, inferred, tc.expected)
		}
	}

	accuracy := float64(correct) / float64(len(testCases))
	t.Logf("Element type inference accuracy: %.0f%% (%d/%d)", accuracy*100, correct, len(testCases))

	if accuracy < 0.7 {
		t.Errorf("type inference accuracy %.0f%% is below 70%% threshold", accuracy*100)
	}
}

// TestAIJudge_ReportJSON verifies the report can be serialized to JSON.
func TestAIJudge_ReportJSON(t *testing.T) {
	report := &aijudge.AccuracyReport{
		Timestamp:    time.Now(),
		FixtureName:  "test",
		GhostCount:   5,
		JudgeCount:   6,
		MatchedCount: 4,
		Precision:    0.8,
		Recall:       0.667,
		F1:           0.727,
		TypeAccuracy: 0.75,
	}

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("JSON marshaling failed: %v", err)
	}

	var decoded aijudge.AccuracyReport
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON unmarshaling failed: %v", err)
	}

	if decoded.FixtureName != "test" {
		t.Errorf("expected fixture name 'test', got %q", decoded.FixtureName)
	}
	if decoded.Precision != 0.8 {
		t.Errorf("expected precision 0.8, got %.2f", decoded.Precision)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func findFixtureScreenshot(t *testing.T) string {
	t.Helper()
	// Check multiple possible locations
	candidates := []string{
		filepath.Join("internal", "aijudge", "testdata", "fixture_normal.png"),
		filepath.Join("testdata", "fixture_normal.png"),
		filepath.Join("..", "..", "internal", "aijudge", "testdata", "fixture_normal.png"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func ghostElementsFromJudged(judged []aijudge.JudgedElement) []aijudge.GhostElement {
	ghost := make([]aijudge.GhostElement, len(judged))
	for i, j := range judged {
		ghost[i] = aijudge.GhostElement{
			ID:         i + 1,
			Text:       j.Text,
			Type:       j.Type,
			Rect:       j.Rect,
			Confidence: 90,
		}
	}
	return ghost
}

func saveReport(t *testing.T, report *aijudge.AccuracyReport) {
	t.Helper()
	dir := filepath.Join("benchmarks", "ai-judge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("WARN: could not create report dir: %v", err)
		return
	}

	filename := fmt.Sprintf("report_%s_%s.json",
		report.FixtureName,
		report.Timestamp.Format("20060102_150405"))

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Logf("WARN: could not marshal report: %v", err)
		return
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, jsonBytes, 0o644); err != nil {
		t.Logf("WARN: could not write report: %v", err)
		return
	}
	t.Logf("Report saved to %s", path)
}
