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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/aijudge"
	"github.com/ghost-mcp/mcpclient"
)

// Controlled fixture browser geometry for AI judge tests. We launch Edge in
// --kiosk mode (true fullscreen, no chrome, takes focus), so the entire
// primary monitor IS the fixture page — no other windows can pollute the
// capture. The actual screen bounds are queried at runtime via get_screen_size.
var (
	fixtureBrowserX      = 0
	fixtureBrowserY      = 0
	fixtureBrowserWidth  = 0 // filled in from get_screen_size
	fixtureBrowserHeight = 0
)

// launchControlledBrowser opens the fixture URL in Microsoft Edge's app mode
// at a known position and size, so the screenshot region is deterministic and
// other windows on screen can't pollute the capture.
func launchControlledBrowser(t *testing.T, url string) func() {
	t.Helper()
	args := []string{
		"--kiosk", url,
		"--new-window",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-features=Translate",
	}
	// Use a unique temp dir for each test run to avoid profile corruption
	userDataDir, err := os.MkdirTemp(os.TempDir(), "ghost-mcp-aijudge-edge-*")
	if err != nil {
		t.Skipf("Failed to create temp dir for browser user data: %v", err)
	}
	args = append(args, "--user-data-dir="+userDataDir)
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}
	var browserPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			browserPath = c
			break
		}
	}
	if browserPath == "" {
		t.Skip("No suitable browser (Edge/Chrome) found for controlled fixture window")
	}
	cmd := exec.Command(browserPath, args...)
	if err := cmd.Start(); err != nil {
		t.Skipf("Failed to launch controlled browser: %v", err)
	}
	t.Logf("Launched %s in --kiosk mode (pid=%d)", filepath.Base(browserPath), cmd.Process.Pid)

	return func() {
		_ = cmd.Process.Kill()
		// Add timeout to prevent blocking on hang
		done := make(chan struct{})
		go func() {
			_, _ = cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// Process didn't exit in time, force kill
			_ = cmd.Process.Kill()
		}
		// Clean up the temp user data dir
		_ = os.RemoveAll(userDataDir)
	}
}

// waitForBrowserReady polls until the browser has rendered the page.
// It attempts to take a screenshot and verifies that we get a non-empty result.
func waitForBrowserReady(ctx context.Context, client *mcpclient.Client, t *testing.T) {
	t.Helper()
	const (
		maxWait    = 10 * time.Second
		pollInterval = 500 * time.Millisecond
	)

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if _, err := client.CallToolString(ctx, "take_screenshot", nil); err == nil {
			t.Log("Browser rendered page (screenshot successful)")
			return
		}
		time.Sleep(pollInterval)
	}
	// If we get here, the browser didn't render in time, but we'll proceed anyway
	// and let the test fail later with a more meaningful error.
	t.Logf("WARNING: Browser did not render within %v, proceeding anyway", maxWait)
}

// TestAIJudge_LiveFixtureNormal takes a live screenshot of the normal fixture,
// runs Ghost MCP's OCR pipeline, then uses Gemini as an independent judge.
func TestAIJudge_LiveFixtureNormal(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set; skipping live AI judge test")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	// Start the fixture server (do not rely on its auto-opened browser — we
	// launch our own controlled window below).
	_, cleanup := startFixtureServer(t)
	defer cleanup()
	// Wait briefly for HTTP to be reachable; skip waitForFixture's focus dance.
	time.Sleep(1 * time.Second)

	// Launch a controlled Edge --app window at known position and size.
	browserCleanup := launchControlledBrowser(t, fixtureURL)
	defer browserCleanup()

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

	// Wait for browser to render before taking screenshots.
	waitForBrowserReady(ctx, client, t)

	// Discover screen size — kiosk fills the primary monitor.
	if sizeJSON, err := client.CallToolString(ctx, "get_screen_size", nil); err == nil {
		var sz struct{ Width, Height int }
		if err := json.Unmarshal([]byte(sizeJSON), &sz); err == nil && sz.Width > 0 {
			fixtureBrowserWidth = sz.Width
			fixtureBrowserHeight = sz.Height
			t.Logf("Screen size: %dx%d", sz.Width, sz.Height)
		}
	}
	if fixtureBrowserWidth == 0 {
		fixtureBrowserWidth = 1707
		fixtureBrowserHeight = 1067
	}

	// Step 1: Take a fullscreen screenshot (kiosk browser should fill the screen).
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
		t.Logf("take_screenshot raw response (unmarshal failed: %v): %s", err, truncateForLog(screenshotJSON, 400))
	} else {
		t.Logf("take_screenshot path=%q size=%dx%d", ssResp.Path, ssResp.Width, ssResp.Height)
	}

	// Step 2: Learn the screen with Ghost MCP (same region as the screenshot)
	t.Log("Running Ghost MCP learn_screen...")
	learnResult, err := client.CallToolString(ctx, "learn_screen", map[string]interface{}{
		"max_pages": 1,
		"x":         fixtureBrowserX,
		"y":         fixtureBrowserY,
		"width":     fixtureBrowserWidth,
		"height":    fixtureBrowserHeight,
	})
	if err != nil {
		t.Fatalf("learn_screen failed: %v", err)
	}

	var learnResp struct {
		Success       bool `json:"success"`
		ElementsFound int  `json:"elements_found"`
	}
	if err := json.Unmarshal([]byte(learnResult), &learnResp); err != nil {
		t.Logf("WARNING: Failed to parse learn_screen response: %v", err)
	} else {
		t.Logf("Ghost MCP found %d elements", learnResp.ElementsFound)
	}

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

	// Sanity check: did we actually capture the fixture page? If the controlled
	// browser failed to take over the screen (other windows blocking, multi-
	// monitor setup, focus stealing prevented), Ghost MCP will be OCRing
	// unrelated content and the resulting score is meaningless.
	if !containsAny(ghostElements, "PRIMARY", "SUCCESS", "WARNING") {
		t.Skipf("Fixture canary not found in OCR output (%d elements). The controlled browser likely failed to claim the screen — run on a clean desktop or VM.", len(ghostElements))
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
	judgeCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
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

// TestAIJudge_LiveFixtureChallenge navigates to the challenge (dark-theme) page
// and runs the full Ghost MCP + Gemini comparison pipeline.
// This stress-tests the 6-pass OCR pipeline on gradient buttons and white-on-dark text.
func TestAIJudge_LiveFixtureChallenge(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set; skipping challenge fixture live test")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	// Start the fixture server and navigate to challenge page
	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)
	time.Sleep(settleTime)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: 90 * time.Second,
		Env:     []string{"GHOST_MCP_KEEP_SCREENSHOTS=1", "INTEGRATION=1"},
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Wait for browser to render before taking screenshots.
	waitForBrowserReady(ctx, client, t)

	// Navigate to the challenge page (dark theme)
	t.Log("Clicking challenge page link...")
	_, err = client.CallToolString(ctx, "find_and_click", map[string]interface{}{
		"text": "Challenge page",
	})
	if err != nil {
		t.Fatalf("Failed to navigate to challenge page: %v", err)
	}
	time.Sleep(settleTime)

	// Take screenshot
	screenshotJSON, err := client.CallToolString(ctx, "take_screenshot", nil)
	if err != nil {
		t.Fatalf("take_screenshot failed: %v", err)
	}
	var ssResp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(screenshotJSON), &ssResp); err != nil {
		t.Logf("WARNING: Failed to parse screenshot response: %v", err)
	}

	// Save a copy as fixture_challenge.png for future offline tests
	if ssResp.Path != "" {
		targetPath := filepath.Join("internal", "aijudge", "testdata", "fixture_challenge.png")
		if err := copyFile(ssResp.Path, targetPath); err == nil {
			t.Logf("Saved challenge screenshot to %s", targetPath)
		}
	}

	// Learn the screen
	t.Log("Running Ghost MCP learn_screen on challenge page...")
	learnResult, err := client.CallToolString(ctx, "learn_screen", map[string]interface{}{
		"max_pages": 1,
	})
	if err != nil {
		t.Fatalf("learn_screen failed: %v", err)
	}
	var learnResp struct {
		ElementsFound int `json:"elements_found"`
	}
	if err := json.Unmarshal([]byte(learnResult), &learnResp); err != nil {
		t.Logf("WARNING: Failed to parse learn_screen response on challenge page: %v", err)
	} else {
		t.Logf("Ghost MCP found %d elements on challenge page", learnResp.ElementsFound)
	}

	// Get learned elements
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

	// Send screenshot to Gemini
	screenshotBytes, err := getScreenshotBytes(ssResp.Path)
	if err != nil {
		t.Skip("Screenshot file not accessible for Gemini analysis")
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}
	judge, err := aijudge.NewJudge(apiKey, model)
	if err != nil {
		t.Fatalf("Failed to create AI judge: %v", err)
	}

	t.Log("Sending challenge screenshot to Gemini (dark-theme, gradient buttons)...")
	judgeCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	judgedElements, err := judge.AnalyzeScreenshot(judgeCtx, screenshotBytes, "image/png")
	if err != nil {
		t.Fatalf("Gemini analysis failed: %v", err)
	}
	t.Logf("Gemini identified %d elements on challenge page", len(judgedElements))

	report := aijudge.CompareResults("live_challenge_fixture", ghostElements, judgedElements, aijudge.DefaultCompareConfig())
	t.Logf("\n%s", report.String())
	saveLiveReport(t, report)

	t.Logf("Challenge Results: Precision=%.1f%%, Recall=%.1f%%, F1=%.1f%%",
		report.Precision*100, report.Recall*100, report.F1*100)

	// Challenge page is harder — lower thresholds, but still check for catastrophic failure
	if report.Recall < 0.20 {
		t.Errorf("Challenge recall %.1f%% is critically low (< 20%%) — OCR pipeline failing on dark theme", report.Recall*100)
	} else if report.Recall < 0.40 {
		t.Logf("WARNING: Challenge recall %.1f%% is below target (< 40%%) — dark-theme OCR needs improvement", report.Recall*100)
	}
}

// containsAny returns true if any element's text contains any of the needles
// (case-insensitive substring match).
func containsAny(elems []aijudge.GhostElement, needles ...string) bool {
	for _, e := range elems {
		lower := strings.ToLower(e.Text)
		for _, n := range needles {
			if strings.Contains(lower, strings.ToLower(n)) {
				return true
			}
		}
	}
	return false
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// copyFile copies a file from src to dst, creating parent directories as needed.
// Uses io.Copy for memory efficiency with large files (e.g. multi-MB screenshots).
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
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
