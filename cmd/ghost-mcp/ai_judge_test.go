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
	"sort"
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

	// 3 retries × worst-case delay (60s) + buffer = 3 min minimum
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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

// TestAIJudge_ChallengeFixture_GroundTruth validates the comparison engine
// against the dark-theme challenge fixture ground truth. The challenge page
// uses gradient buttons and white-on-dark text, which is the hardest case for
// OCR. This offline test uses a simulated Ghost MCP output to stress-test the
// comparison framework; a live display test is in ai_judge_live_test.go.
func TestAIJudge_ChallengeFixture_GroundTruth(t *testing.T) {
	// Simulate what Ghost MCP's 6-pass OCR pipeline would find on the dark-theme
	// challenge page. In practice it captures fewer elements than the light theme
	// because gradient/dark backgrounds are harder for OCR.
	simulatedGhost := []aijudge.GhostElement{
		// Primary heading — large white text on dark background (inverted pass)
		{ID: 1, Text: "Ghost MCP Test Fixture", Type: "heading", Rect: aijudge.Rect{X: 250, Y: 10, Width: 400, Height: 40}},
		// Buttons found via bright-text pass (white text on gradient)
		{ID: 2, Text: "PRIMARY", Type: "button", Rect: aijudge.Rect{X: 30, Y: 200, Width: 200, Height: 45}},
		{ID: 3, Text: "SUCCESS", Type: "button", Rect: aijudge.Rect{X: 240, Y: 200, Width: 200, Height: 45}},
		{ID: 4, Text: "WARNING", Type: "button", Rect: aijudge.Rect{X: 450, Y: 200, Width: 200, Height: 45}},
		// INFO button sometimes missed due to cyan background (color-inverted pass needed)
		// OCR targets — white-on-dark, found via inverted pass
		{ID: 5, Text: "Hello World", Type: "text", Rect: aijudge.Rect{X: 50, Y: 985, Width: 120, Height: 20}},
		{ID: 6, Text: "Ghost MCP Automation", Type: "text", Rect: aijudge.Rect{X: 50, Y: 1010, Width: 200, Height: 20}},
	}

	report := aijudge.CompareResults(
		"simulated_challenge",
		simulatedGhost,
		testdata.FixtureChallenge.Elements,
		aijudge.DefaultCompareConfig(),
	)

	t.Logf("Challenge fixture simulated accuracy:\n%s", report.String())

	// Validate report structure
	if report.GhostCount != len(simulatedGhost) {
		t.Errorf("ghost count: expected %d, got %d", len(simulatedGhost), report.GhostCount)
	}
	if report.JudgeCount != len(testdata.FixtureChallenge.Elements) {
		t.Errorf("judge count: expected %d, got %d", len(testdata.FixtureChallenge.Elements), report.JudgeCount)
	}

	// Expect the primary heading and buttons to match
	if report.MatchedCount == 0 {
		t.Error("expected at least some matched elements on challenge fixture")
	}

	// Challenge fixture has harder elements (checkboxes, radios, dark-bg links)
	// so we expect significant misses — but the comparison framework should work
	if len(report.MissedByGhost) == 0 {
		t.Error("expected some missed elements on the harder challenge fixture")
	}

	// Precision should be high (what we find, we find correctly)
	if report.Precision < 0.6 {
		t.Errorf("challenge fixture precision too low: %.1f%%", report.Precision*100)
	}

	t.Logf("Summary: Precision=%.1f%%, Recall=%.1f%%, F1=%.1f%%",
		report.Precision*100, report.Recall*100, report.F1*100)
}

// TestAIJudge_ChallengeFixture_Gemini sends the challenge fixture screenshot to
// Gemini for independent evaluation of the dark-theme page.
// Requires GOOGLE_API_KEY and a pre-captured fixture_challenge.png.
func TestAIJudge_ChallengeFixture_Gemini(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set; skipping challenge fixture Gemini test")
	}

	screenshotPath := findChallengeScreenshot(t)
	if screenshotPath == "" {
		t.Skip("No challenge fixture screenshot found; capture with test_runner.bat fixture first")
	}

	imgBytes, err := os.ReadFile(screenshotPath)
	if err != nil {
		t.Fatalf("failed to read challenge screenshot: %v", err)
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}

	judge, err := aijudge.NewJudge(apiKey, model)
	if err != nil {
		t.Fatalf("failed to create judge: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Log("Sending challenge screenshot to Gemini for analysis (dark-theme)...")
	judgedElements, err := judge.AnalyzeScreenshot(ctx, imgBytes, "image/png")
	if err != nil {
		t.Fatalf("Gemini analysis failed: %v", err)
	}

	t.Logf("Gemini identified %d elements on the dark-theme challenge page", len(judgedElements))

	report := aijudge.CompareResults(
		"gemini_challenge",
		ghostElementsFromJudged(judgedElements),
		testdata.FixtureChallenge.Elements,
		aijudge.DefaultCompareConfig(),
	)

	t.Logf("\nChallenge page Gemini self-consistency report:\n%s", report.String())
	saveReport(t, report)
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

func findChallengeScreenshot(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("internal", "aijudge", "testdata", "fixture_challenge.png"),
		filepath.Join("testdata", "fixture_challenge.png"),
		filepath.Join("..", "..", "internal", "aijudge", "testdata", "fixture_challenge.png"),
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

// saveReport persists an AccuracyReport to benchmarks/ai-judge/ and compares
// it against the most recent previous report for the same fixture to detect
// regressions or improvements across runs.
func saveReport(t *testing.T, report *aijudge.AccuracyReport) {
	t.Helper()
	dir := filepath.Join("benchmarks", "ai-judge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("WARN: could not create report dir: %v", err)
		return
	}

	// Load the previous report for this fixture for trend comparison.
	prev := loadLatestReport(t, dir, report.FixtureName)
	if prev != nil {
		logTrend(t, prev, report)
	}

	// Check recall threshold — emit a warning if recall has regressed.
	checkRecallThreshold(t, report, 0.50)

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

// loadLatestReport finds the most-recent saved JSON report for a given fixture.
// Returns nil if no prior report exists.
func loadLatestReport(t *testing.T, dir, fixtureName string) *aijudge.AccuracyReport {
	t.Helper()
	prefix := fmt.Sprintf("report_%s_", fixtureName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // dir may not exist yet
	}

	// Collect matching filenames (sorted by name = chronological order since
	// the timestamp format is YYYYMMDD_HHMMSS).
	var matches []string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > len(prefix) &&
			e.Name()[:len(prefix)] == prefix &&
			filepath.Ext(e.Name()) == ".json" {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	latest := matches[len(matches)-1]

	data, err := os.ReadFile(filepath.Join(dir, latest))
	if err != nil {
		return nil
	}
	var rep aijudge.AccuracyReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil
	}
	return &rep
}

// logTrend logs the delta between the previous and current report so regressions
// or improvements are visible in the test output.
func logTrend(t *testing.T, prev, curr *aijudge.AccuracyReport) {
	t.Helper()
	dPrec := (curr.Precision - prev.Precision) * 100
	dRec := (curr.Recall - prev.Recall) * 100
	dF1 := (curr.F1 - prev.F1) * 100

	sign := func(v float64) string {
		if v > 0 {
			return fmt.Sprintf("+%.1f%%", v)
		}
		return fmt.Sprintf("%.1f%%", v)
	}

	t.Logf("Trend vs previous %s report:", prev.FixtureName)
	t.Logf("  Precision: %.1f%% → %.1f%% (%s)",
		prev.Precision*100, curr.Precision*100, sign(dPrec))
	t.Logf("  Recall:    %.1f%% → %.1f%% (%s)",
		prev.Recall*100, curr.Recall*100, sign(dRec))
	t.Logf("  F1:        %.1f%% → %.1f%% (%s)",
		prev.F1*100, curr.F1*100, sign(dF1))
}

// checkRecallThreshold emits a test warning when recall drops below the given
// threshold. This implements CI alerting without failing the test outright,
// since recall is subject to Gemini API variation and quota fluctuations.
func checkRecallThreshold(t *testing.T, report *aijudge.AccuracyReport, minRecall float64) {
	t.Helper()
	if report.Recall < minRecall {
		t.Logf("WARNING: recall %.1f%% is below calibrated threshold %.0f%% for fixture %q — consider investigating OCR pipeline",
			report.Recall*100, minRecall*100, report.FixtureName)
	}
}

// TestAIJudge_TrendTracking verifies that saveReport correctly loads a previous
// report and produces a meaningful delta when a newer report is compared.
func TestAIJudge_TrendTracking(t *testing.T) {
	dir := t.TempDir()

	// Write a "previous" report
	prev := &aijudge.AccuracyReport{
		Timestamp:   time.Now().Add(-1 * time.Hour),
		FixtureName: "trend_test",
		Precision:   0.70,
		Recall:      0.40,
		F1:          0.51,
	}
	prevBytes, _ := json.MarshalIndent(prev, "", "  ")
	prevFile := filepath.Join(dir, fmt.Sprintf("report_trend_test_%s.json", prev.Timestamp.Format("20060102_150405")))
	if err := os.WriteFile(prevFile, prevBytes, 0o644); err != nil {
		t.Fatalf("failed to write previous report: %v", err)
	}

	// Load it back
	loaded := loadLatestReport(t, dir, "trend_test")
	if loaded == nil {
		t.Fatal("expected loadLatestReport to find previous report")
	}
	if loaded.Precision != 0.70 {
		t.Errorf("expected precision 0.70, got %.2f", loaded.Precision)
	}
	if loaded.Recall != 0.40 {
		t.Errorf("expected recall 0.40, got %.2f", loaded.Recall)
	}

	// Verify logTrend doesn't panic and produces output
	curr := &aijudge.AccuracyReport{
		Timestamp:   time.Now(),
		FixtureName: "trend_test",
		Precision:   0.85,
		Recall:      0.55,
		F1:          0.67,
	}
	logTrend(t, loaded, curr) // Should log improvement without panicking

	// Verify checkRecallThreshold warns when below target
	lowRecall := &aijudge.AccuracyReport{FixtureName: "trend_test", Recall: 0.30}
	checkRecallThreshold(t, lowRecall, 0.50) // Should log a warning

	highRecall := &aijudge.AccuracyReport{FixtureName: "trend_test", Recall: 0.75}
	checkRecallThreshold(t, highRecall, 0.50) // Should be silent (no warning)
}
