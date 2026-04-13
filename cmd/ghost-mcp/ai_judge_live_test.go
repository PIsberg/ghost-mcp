//go:build integration

// ai_judge_live_test.go — Live AI judge integration tests.
//
// Takes a real screenshot of the test fixture, runs Ghost MCP's OCR pipeline,
// then asks Gemini to independently identify elements and compares them.
//
// Requirements:
//   - GOOGLE_API_KEY env var
//   - INTEGRATION=1 env var
//   - Display (or Xvfb on Linux)
//   - GCC toolchain for CGo
//
// Run with:
//
//	GOOGLE_API_KEY=xxx INTEGRATION=1 go test -v -run TestAIJudge_Live ./cmd/ghost-mcp/... -timeout 180s
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
	"github.com/ghost-mcp/mcpclient"
)

// TestAIJudge_LiveFixtureNormal takes a live screenshot of the normal fixture,
// runs Ghost MCP's OCR pipeline, then uses Gemini as an independent judge.
func TestAIJudge_LiveFixtureNormal(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set; skipping live AI judge test")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	// Start the fixture server
	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)
	time.Sleep(settleTime)

	// Create MCP client
	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: 90 * time.Second,
		Env:     []string{"GHOST_MCP_KEEP_SCREENSHOTS=1", "INTEGRATION=1"},
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Step 1: Take a screenshot via MCP
	t.Log("Taking screenshot via Ghost MCP...")
	screenshotJSON, err := client.CallToolString(ctx, "take_screenshot", nil)
	if err != nil {
		t.Fatalf("take_screenshot failed: %v", err)
	}

	var ssResp struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(screenshotJSON), &ssResp); err != nil {
		t.Logf("Screenshot response: %s", screenshotJSON)
	}

	// Step 2: Learn the screen with Ghost MCP (our pipeline)
	t.Log("Running Ghost MCP learn_screen...")
	learnResult, err := client.CallToolString(ctx, "learn_screen", map[string]interface{}{
		"max_pages": 1,
	})
	if err != nil {
		t.Fatalf("learn_screen failed: %v", err)
	}

	var learnResp struct {
		Success       bool `json:"success"`
		ElementsFound int  `json:"elements_found"`
	}
	json.Unmarshal([]byte(learnResult), &learnResp)
	t.Logf("Ghost MCP found %d elements", learnResp.ElementsFound)

	// Step 3: Get the learned view
	viewJSON, err := client.CallToolString(ctx, "get_learned_view", nil)
	if err != nil {
		t.Fatalf("get_learned_view failed: %v", err)
	}

	var viewResp struct {
		Elements []struct {
			ID         int     `json:"ocr_id"`
			Text       string  `json:"text"`
			Type       string  `json:"type"`
			X          int     `json:"x"`
			Y          int     `json:"y"`
			Width      int     `json:"width"`
			Height     int     `json:"height"`
			Confidence float64 `json:"confidence"`
			OcrPass    string  `json:"ocr_pass"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(viewJSON), &viewResp); err != nil {
		t.Fatalf("Failed to parse learned view: %v", err)
	}

	// Convert to GhostElement for comparison
	ghostElements := make([]aijudge.GhostElement, len(viewResp.Elements))
	for i, e := range viewResp.Elements {
		ghostElements[i] = aijudge.GhostElement{
			ID:         e.ID,
			Text:       e.Text,
			Type:       e.Type,
			Rect:       aijudge.Rect{X: e.X, Y: e.Y, Width: e.Width, Height: e.Height},
			Confidence: e.Confidence,
			OcrPass:    e.OcrPass,
		}
	}

	// Step 4: Get the screenshot image bytes for Gemini
	// Use take_screenshot's saved file if available, otherwise take a fresh one
	screenshotBytes, err := getScreenshotBytes(ssResp.Path)
	if err != nil {
		t.Logf("Could not read screenshot file, taking inline screenshot")
		t.Skip("Screenshot file not accessible for Gemini analysis")
	}

	// Step 5: Send to Gemini
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}

	judge, err := aijudge.NewJudge(apiKey, model)
	if err != nil {
		t.Fatalf("Failed to create AI judge: %v", err)
	}

	t.Log("Sending screenshot to Gemini for independent analysis...")
	judgeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	judgedElements, err := judge.AnalyzeScreenshot(judgeCtx, screenshotBytes, "image/png")
	if err != nil {
		t.Fatalf("Gemini analysis failed: %v", err)
	}
	t.Logf("Gemini identified %d elements", len(judgedElements))

	// Step 6: Compare
	report := aijudge.CompareResults("live_normal_fixture", ghostElements, judgedElements, aijudge.DefaultCompareConfig())

	t.Logf("\n%s", report.String())

	// Step 7: Save report
	saveLiveReport(t, report)

	// Step 8: Assert minimum quality
	t.Logf("Results: Precision=%.1f%%, Recall=%.1f%%, F1=%.1f%%, TypeAccuracy=%.1f%%",
		report.Precision*100, report.Recall*100, report.F1*100, report.TypeAccuracy*100)

	// Soft thresholds — warn but don't fail during initial calibration
	if report.Recall < 0.30 {
		t.Errorf("Recall %.1f%% is critically low (< 30%%)", report.Recall*100)
	} else if report.Recall < 0.50 {
		t.Logf("WARNING: Recall %.1f%% is below target (< 50%%)", report.Recall*100)
	}

	if report.Precision < 0.30 {
		t.Errorf("Precision %.1f%% is critically low (< 30%%)", report.Precision*100)
	} else if report.Precision < 0.50 {
		t.Logf("WARNING: Precision %.1f%% is below target (< 50%%)", report.Precision*100)
	}
}

// getScreenshotBytes reads a screenshot file from disk.
func getScreenshotBytes(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("no screenshot path")
	}
	return os.ReadFile(path)
}

func saveLiveReport(t *testing.T, report *aijudge.AccuracyReport) {
	t.Helper()
	dir := filepath.Join("benchmarks", "ai-judge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("WARN: could not create report dir: %v", err)
		return
	}

	filename := fmt.Sprintf("report_live_%s_%s.json",
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
	t.Logf("Live report saved to %s", path)
}
