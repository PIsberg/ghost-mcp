//go:build integration

// integration_test.go - Integration tests for Ghost MCP
//
// These tests verify the full integration between the MCP server
// and the test fixture GUI. They require:
// 1. GCC/MinGW installed (for robotgo)
// 2. A display environment
// 3. The test fixture server running
//
// Run with: go test -v -run Integration ./...
//
// WARNING: These tests control your mouse and keyboard!
// Do not run while working on important tasks.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ghost-mcp/mcpclient"
)

// Test configuration
const (
	fixtureURL  = "http://localhost:8765"
	fixturePort = "8765"
	testTimeout = 60 * time.Second
	settleTime  = 500 * time.Millisecond // Time for UI to settle after actions
)

// skipIfNoDisplay skips tests if no display is available
func skipIfNoDisplay(t *testing.T) {
	// Check for common CI/CD environment variables
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping in CI environment")
	}

	// Check for display (Linux/macOS)
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		// On Windows, this check doesn't apply
		if os.Getenv("OS") != "Windows_NT" {
			t.Skip("No display available")
		}
	}
}

// skipIfNoGCC skips tests if GCC is not available
func skipIfNoGCC(t *testing.T) {
	_, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("GCC not found in PATH, skipping integration tests")
	}
}

// startFixtureServer starts the test fixture HTTP server
func startFixtureServer(t *testing.T) (*exec.Cmd, func()) {
	cmd := exec.Command("go", "run", "test_fixture/fixture_server.go")
	cmd.Env = append(os.Environ(), fmt.Sprintf("FIXTURE_PORT=%s", fixturePort))

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start fixture server: %v", err)
	}

	// Wait for server to be ready
	for i := 0; i < 20; i++ {
		resp, err := http.Get(fixtureURL)
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	cleanup := func() {
		cmd.Process.Kill()
		cmd.Wait()
	}

	return cmd, cleanup
}

// waitForFixture waits for the fixture server to respond
func waitForFixture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Fixture server not responding")
		default:
			resp, err := http.Get(fixtureURL)
			if err == nil {
				resp.Body.Close()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// TestIntegration_GetScreenSize tests the get_screen_size tool
func TestIntegration_GetScreenSize(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	width, height, err := client.GetScreenSize(ctx)
	if err != nil {
		t.Fatalf("GetScreenSize failed: %v", err)
	}

	if width <= 0 || height <= 0 {
		t.Errorf("Invalid screen size: %dx%d", width, height)
	}

	t.Logf("Screen size: %dx%d", width, height)
}

// TestIntegration_MoveMouse tests mouse movement
func TestIntegration_MoveMouse(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Get screen size to calculate safe positions
	width, height, err := client.GetScreenSize(ctx)
	if err != nil {
		t.Fatalf("Failed to get screen size: %v", err)
	}

	// Test positions (avoiding failsafe at 0,0)
	testPositions := []struct {
		x, y int
		desc string
	}{
		{100, 100, "top-left area"},
		{width / 2, height / 2, "center"},
		{width - 100, height - 100, "bottom-right area"},
		{width / 2, 100, "top-center"},
	}

	for _, pos := range testPositions {
		t.Run(pos.desc, func(t *testing.T) {
			err := client.MoveMouse(ctx, pos.x, pos.y)
			if err != nil {
				t.Errorf("MoveMouse failed: %v", err)
			}
			time.Sleep(settleTime)
		})
	}
}

// TestIntegration_Click tests mouse clicks
func TestIntegration_Click(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Move to a safe area first
	width, height, _ := client.GetScreenSize(ctx)
	client.MoveMouse(ctx, width/2, height/2)
	time.Sleep(settleTime)

	// Test different buttons
	buttons := []string{"left", "right", "middle"}
	for _, button := range buttons {
		t.Run(button, func(t *testing.T) {
			err := client.Click(ctx, button)
			if err != nil {
				t.Errorf("Click(%s) failed: %v", button, err)
			}
			time.Sleep(settleTime)
		})
	}
}

// TestIntegration_TypeText tests text typing
func TestIntegration_TypeText(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	testStrings := []string{
		"Hello, World!",
		"Ghost MCP Test",
		"12345",
		"Special chars: !@#$%",
	}

	for _, text := range testStrings {
		t.Run(text, func(t *testing.T) {
			err := client.TypeText(ctx, text)
			if err != nil {
				t.Errorf("TypeText failed: %v", err)
			}
			time.Sleep(settleTime)
		})
	}
}

// TestIntegration_PressKey tests key presses
func TestIntegration_PressKey(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	keys := []string{"enter", "tab", "esc", "space"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			err := client.PressKey(ctx, key)
			if err != nil {
				t.Errorf("PressKey(%s) failed: %v", key, err)
			}
			time.Sleep(settleTime)
		})
	}
}

// TestIntegration_TakeScreenshot tests screenshot capture
func TestIntegration_TakeScreenshot(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	filepath, base64Data, width, height, err := client.TakeScreenshot(ctx)
	if err != nil {
		t.Fatalf("TakeScreenshot failed: %v", err)
	}

	if width <= 0 || height <= 0 {
		t.Errorf("Invalid screenshot dimensions: %dx%d", width, height)
	}

	// Verify base64 data is valid
	_, err = base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		t.Errorf("Invalid base64 data: %v", err)
	}

	t.Logf("Screenshot: %dx%d, saved to %s", width, height, filepath)
}

// TestIntegration_FullWorkflow tests a complete automation workflow against the fixture.
func TestIntegration_FullWorkflow(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{Timeout: testTimeout})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	time.Sleep(settleTime)

	// Click the Primary button by text label — no coordinate guessing.
	result, err := client.FindAndClick(ctx, "Primary", mcpclient.FindAndClickOptions{})
	if err != nil {
		t.Fatalf("FindAndClick('Primary') failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("FindAndClick('Primary') reported failure: %+v", result)
	}
	t.Logf("✓ Clicked Primary at (%d, %d)", result.ActualX, result.ActualY)

	// Click the text input field and type into it.
	inputResult, err := client.FindAndClick(ctx, "Type here", mcpclient.FindAndClickOptions{})
	if err != nil {
		t.Fatalf("FindAndClick('Type here') failed: %v", err)
	}
	if !inputResult.Success {
		t.Fatalf("FindAndClick('Type here') reported failure")
	}

	if err := client.TypeText(ctx, "Integration test"); err != nil {
		t.Fatalf("TypeText failed: %v", err)
	}

	t.Log("✓ Full workflow completed successfully")
}

// TestIntegration_ScreenshotRegion tests region-specific screenshots
func TestIntegration_ScreenshotRegion(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Take screenshot of top-left quadrant
	result, err := client.CallToolString(ctx, "take_screenshot", map[string]interface{}{
		"x":      0,
		"y":      0,
		"width":  400,
		"height": 300,
	})
	if err != nil {
		t.Fatalf("TakeScreenshot (region) failed: %v", err)
	}

	var data struct {
		Success bool   `json:"success"`
		Base64  string `json:"base64"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	if !data.Success {
		t.Error("Screenshot reported failure")
	}

	if data.Width != 400 || data.Height != 300 {
		t.Errorf("Expected 400x300, got %dx%d", data.Width, data.Height)
	}

	t.Logf("Region screenshot: %dx%d", data.Width, data.Height)
}

// TestIntegration_ErrorHandling tests error cases
func TestIntegration_ErrorHandling(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Test invalid button
	result, err := client.CallTool(ctx, "click", map[string]interface{}{
		"button": "invalid_button",
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Error("Expected error for invalid button")
	}

	// Test missing parameters
	result, err = client.CallTool(ctx, "move_mouse", map[string]interface{}{})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Error("Expected error for missing parameters")
	}

	t.Log("Error handling verified")
}

// TestIntegration_ConcurrentCalls tests that concurrent tool calls work
func TestIntegration_ConcurrentCalls(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	results := make(chan error, 5)

	// Make concurrent calls
	for i := 0; i < 5; i++ {
		go func(n int) {
			_, _, err := client.GetScreenSize(ctx)
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < 5; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent call %d failed: %v", i, err)
		}
	}

	t.Log("Concurrent calls completed successfully")
}

// TestIntegration_Failsafe tests that the failsafe mechanism exists
// Note: We don't actually trigger it, just verify the server handles edge cases
func TestIntegration_Failsafe(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Move to position (1, 1) - very close to failsafe but not triggering
	err = client.MoveMouse(ctx, 1, 1)
	if err != nil {
		t.Fatalf("MoveMouse near failsafe failed: %v", err)
	}
	time.Sleep(settleTime)

	// Verify server is still responsive
	_, _, err = client.GetScreenSize(ctx)
	if err != nil {
		t.Errorf("Server unresponsive after near-failsafe move: %v", err)
	}

	t.Log("Failsafe boundary test passed")
}

// TestIntegration_ToolDiscovery verifies all expected tools are available
func TestIntegration_ToolDiscovery(t *testing.T) {
	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	expectedTools := []string{
		"get_screen_size",
		"move_mouse",
		"click",
		"click_at",
		"double_click",
		"scroll",
		"scroll_until_text",
		"type_text",
		"press_key",
		"take_screenshot",
		"read_screen_text",
		"find_and_click",
		"find_and_click_all",
		"wait_for_text",
		"find_elements",
	}

	for _, tool := range expectedTools {
		t.Run(tool, func(t *testing.T) {
			// Call with empty/invalid params to verify tool exists
			result, err := client.CallTool(ctx, tool, map[string]interface{}{})
			if err != nil {
				t.Errorf("Tool %s call failed: %v", tool, err)
				return
			}

			// Tool should exist (may return error for invalid params, that's OK)
			t.Logf("Tool %s exists (isError: %v)", tool, result.IsError)
		})
	}
}

// TestIntegration_ReadScreenTextResultFormat verifies read_screen_text response shape.
func TestIntegration_ReadScreenTextResultFormat(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	result, err := client.CallToolString(ctx, "read_screen_text", nil)
	if err != nil {
		t.Fatalf("read_screen_text failed: %v", err)
	}

	var data struct {
		Success bool   `json:"success"`
		Text    string `json:"text"`
		Words   []struct {
			Text       string  `json:"text"`
			X          int     `json:"x"`
			Y          int     `json:"y"`
			Width      int     `json:"width"`
			Height     int     `json:"height"`
			Confidence float64 `json:"confidence"`
		} `json:"words"`
		Region struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"region"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("Result is not valid JSON: %v\nraw: %s", err, result)
	}

	if !data.Success {
		t.Error("read_screen_text did not report success")
	}
	if data.Region.Width <= 0 || data.Region.Height <= 0 {
		t.Errorf("Region dimensions invalid: %dx%d", data.Region.Width, data.Region.Height)
	}
	// words slice must be present (may be empty if Tesseract not installed)
	if data.Words == nil {
		t.Error("words field must not be null")
	}

	t.Logf("✓ read_screen_text: %d words extracted, region %dx%d",
		len(data.Words), data.Region.Width, data.Region.Height)
}

// TestIntegration_ReadScreenTextRegion verifies the x/y/width/height parameters.
func TestIntegration_ReadScreenTextRegion(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	result, err := client.CallToolString(ctx, "read_screen_text", map[string]interface{}{
		"x":      0,
		"y":      0,
		"width":  400,
		"height": 200,
	})
	if err != nil {
		t.Fatalf("read_screen_text (region) failed: %v", err)
	}

	var data struct {
		Success bool `json:"success"`
		Region  struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"region"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if !data.Success {
		t.Error("read_screen_text did not report success for region call")
	}
	if data.Region.Width != 400 || data.Region.Height != 200 {
		t.Errorf("Expected region 400x200, got %dx%d", data.Region.Width, data.Region.Height)
	}

	t.Logf("✓ Region read_screen_text returned correct dimensions")
}

// TestIntegration_ReadScreenTextFixture reads the test fixture page and looks for known text.
func TestIntegration_ReadScreenTextFixture(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Give the browser time to render the fixture page.
	time.Sleep(settleTime)

	text, words, err := client.ReadScreenText(ctx, nil)
	if err != nil {
		t.Fatalf("ReadScreenText failed: %v", err)
	}

	t.Logf("Extracted %d words from fixture page", len(words))
	t.Logf("First 200 chars of text: %.200s", text)

	// If Tesseract found anything, each word must have valid coordinates.
	for i, w := range words {
		txt, _ := w["text"].(string)
		x, _ := w["x"].(float64)
		y, _ := w["y"].(float64)
		width, _ := w["width"].(float64)
		height, _ := w["height"].(float64)
		if width <= 0 || height <= 0 {
			t.Errorf("Word %d (%q) has invalid size: %.0fx%.0f", i, txt, width, height)
		}
		if x < 0 || y < 0 {
			t.Errorf("Word %d (%q) has negative coords: (%.0f, %.0f)", i, txt, x, y)
		}
	}
}

// TestIntegration_ReadScreenTextInvalidRegion verifies error on out-of-bounds region.
func TestIntegration_ReadScreenTextInvalidRegion(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	result, err := client.CallTool(ctx, "read_screen_text", map[string]interface{}{
		"x":      -100,
		"y":      0,
		"width":  400,
		"height": 300,
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Error("Expected error result for negative x coordinate")
	}

	t.Log("✓ Invalid region correctly rejected")
}

// TestIntegration_FindAndClickButton verifies find_and_click against the fixture's
// plain-text buttons (dark text, easily readable by grayscale OCR).
func TestIntegration_FindAndClickButton(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{Timeout: testTimeout})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	time.Sleep(settleTime)

	for _, label := range []string{"Primary", "Success", "Warning"} {
		t.Run(label, func(t *testing.T) {
			result, err := client.FindAndClick(ctx, label, mcpclient.FindAndClickOptions{})
			if err != nil {
				t.Fatalf("FindAndClick(%q) error: %v", label, err)
			}
			if !result.Success {
				t.Fatalf("FindAndClick(%q) not found", label)
			}
			t.Logf("✓ Clicked %q at (%d, %d)", label, result.ActualX, result.ActualY)
			time.Sleep(settleTime)
		})
	}
}

// TestIntegration_FindAndClickColoredButton verifies that find_and_click can locate
// the Info button, which has white text on a cyan/blue gradient background that is
// invisible to grayscale OCR. This exercises the color-mode fallback in the 3-pass OCR.
func TestIntegration_FindAndClickColoredButton(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{Timeout: testTimeout})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	time.Sleep(settleTime)

	result, err := client.FindAndClick(ctx, "Info", mcpclient.FindAndClickOptions{})
	if err != nil {
		t.Fatalf("FindAndClick('Info') error: %v", err)
	}
	if !result.Success {
		t.Fatal("FindAndClick('Info') not found — color OCR fallback may have failed")
	}
	t.Logf("✓ Clicked Info button at (%d, %d)", result.ActualX, result.ActualY)
}

// TestIntegration_FindAndClickAll verifies find_and_click_all clicks multiple buttons
// in one atomic OCR scan.
func TestIntegration_FindAndClickAll(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{Timeout: testTimeout})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	time.Sleep(settleTime)

	result, err := client.CallToolString(ctx, "find_and_click_all", map[string]interface{}{
		"texts":    `["Primary", "Success", "Warning"]`,
		"delay_ms": 150,
	})
	if err != nil {
		t.Fatalf("find_and_click_all error: %v", err)
	}

	var data struct {
		Success      bool `json:"success"`
		ClickedCount int  `json:"clicked_count"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("Failed to parse result: %v\nraw: %s", err, result)
	}
	if !data.Success {
		t.Fatalf("find_and_click_all reported failure: %s", result)
	}
	if data.ClickedCount != 3 {
		t.Errorf("Expected 3 clicks, got %d", data.ClickedCount)
	}
	t.Logf("✓ find_and_click_all clicked %d buttons", data.ClickedCount)
}

// TestIntegration_FindElements verifies find_elements returns the fixture's button labels
// with valid coordinates.
func TestIntegration_FindElements(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{Timeout: testTimeout})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	time.Sleep(settleTime)

	result, err := client.CallToolString(ctx, "find_elements", nil)
	if err != nil {
		t.Fatalf("find_elements error: %v", err)
	}

	var data struct {
		Success      bool `json:"success"`
		ElementCount int  `json:"element_count"`
		Elements     []struct {
			Text       string  `json:"text"`
			CenterX    int     `json:"center_x"`
			CenterY    int     `json:"center_y"`
			Width      int     `json:"width"`
			Height     int     `json:"height"`
			Confidence float64 `json:"confidence"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("Failed to parse result: %v\nraw: %s", err, result)
	}
	if !data.Success {
		t.Fatalf("find_elements reported failure")
	}
	if data.ElementCount == 0 {
		t.Fatal("find_elements returned no elements")
	}

	// Every element must have valid coordinates.
	for _, el := range data.Elements {
		if el.CenterX <= 0 || el.CenterY <= 0 {
			t.Errorf("Element %q has invalid center: (%d, %d)", el.Text, el.CenterX, el.CenterY)
		}
		if el.Width <= 0 || el.Height <= 0 {
			t.Errorf("Element %q has invalid size: %dx%d", el.Text, el.Width, el.Height)
		}
	}

	t.Logf("✓ find_elements returned %d elements", data.ElementCount)
}

// TestIntegration_WaitForText verifies wait_for_text detects text already on screen.
func TestIntegration_WaitForText(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{Timeout: testTimeout})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	time.Sleep(settleTime)

	// "Button Click Tests" heading is always visible on the fixture page.
	result, err := client.CallToolString(ctx, "wait_for_text", map[string]interface{}{
		"text":       "Button Click Tests",
		"timeout_ms": 5000,
	})
	if err != nil {
		t.Fatalf("wait_for_text error: %v", err)
	}

	var data struct {
		Success  bool `json:"success"`
		Visible  bool `json:"visible"`
		WaitedMS int  `json:"waited_ms"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("Failed to parse result: %v\nraw: %s", err, result)
	}
	if !data.Success || !data.Visible {
		t.Fatalf("wait_for_text did not find 'Button Click Tests': %s", result)
	}
	t.Logf("✓ wait_for_text found text in %dms", data.WaitedMS)
}

// Helper function to parse tool result JSON
func parseToolResult[T any](result string, out *T) error {
	return json.Unmarshal([]byte(result), out)
}

// TestMain runs setup/teardown for all integration tests
func TestMain(m *testing.M) {
	// Check if we should run integration tests
	if os.Getenv("INTEGRATION") != "1" {
		fmt.Println("Skipping integration tests (set INTEGRATION=1 to run)")
		os.Exit(0)
		return
	}

	// Verify binary exists
	binaryPath := "./ghost-mcp.exe"
	if _, err := os.Stat("./ghost-mcp"); err == nil {
		binaryPath = "./ghost-mcp"
	} else if _, err := os.Stat("./ghost-mcp.exe"); err != nil {
		fmt.Println("Error: ghost-mcp binary not found. Run 'go build' first.")
		os.Exit(1)
	}

	// Set up authentication token for integration tests.
	// The server subprocess inherits this via os.Environ() in mcpclient.NewClient.
	if os.Getenv("GHOST_MCP_TOKEN") == "" {
		os.Setenv("GHOST_MCP_TOKEN", "ghost-mcp-integration-test-token")
	}

	// Set environment for tests
	os.Setenv("GHOST_MCP_BINARY", binaryPath)

	// Run tests
	code := m.Run()
	os.Exit(code)
}

// ensureBinaryExists checks if the ghost-mcp binary exists
func ensureBinaryExists(t *testing.T) string {
	paths := []string{"./ghost-mcp.exe", "./ghost-mcp"}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try to build
	t.Log("Binary not found, attempting to build...")
	cmd := exec.Command("go", "build", "-o", "ghost-mcp.exe", ".")
	cmd.Dir = filepath.Join(getProjectRoot(t))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build ghost-mcp: %v\n%s", err, string(output))
	}
	return filepath.Join(cmd.Dir, "ghost-mcp.exe")
}

// getProjectRoot returns the project root directory
func getProjectRoot(t *testing.T) string {
	// Simple heuristic: look for go.mod
	dir, _ := os.Getwd()
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// TestIntegration_ValidateBinaryExists ensures the binary is built before tests
func TestIntegration_ValidateBinaryExists(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	binaryPath := ensureBinaryExists(t)
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("Binary not found at %s", binaryPath)
	}
	t.Logf("Found binary at: %s", binaryPath)
}

// TestIntegration_ScreenSizeBasic is a simple sanity check
func TestIntegration_ScreenSizeBasic(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	width, height, err := client.GetScreenSize(ctx)
	if err != nil {
		t.Fatalf("GetScreenSize failed: %v", err)
	}

	t.Logf("✓ Screen size: %dx%d", width, height)

	// Basic sanity checks
	if width < 640 {
		t.Errorf("Width %d seems too small", width)
	}
	if height < 480 {
		t.Errorf("Height %d seems too small", height)
	}
}

// TestIntegration_ScreenSizeResultFormat verifies the result format
func TestIntegration_ScreenSizeResultFormat(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	result, err := client.CallToolString(ctx, "get_screen_size", nil)
	if err != nil {
		t.Fatalf("CallToolString failed: %v", err)
	}

	// Verify JSON format
	var dims map[string]interface{}
	if err := json.Unmarshal([]byte(result), &dims); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	// Check required fields
	if _, ok := dims["width"]; !ok {
		t.Error("Result missing 'width' field")
	}
	if _, ok := dims["height"]; !ok {
		t.Error("Result missing 'height' field")
	}

	t.Logf("✓ Result format valid: %s", result)
}

// TestIntegration_MouseMovement verifies mouse can be moved and position reported
func TestIntegration_MouseMovement(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Move to a test position
	testX, testY := 200, 200
	err = client.MoveMouse(ctx, testX, testY)
	if err != nil {
		t.Fatalf("MoveMouse failed: %v", err)
	}
	time.Sleep(settleTime)

	// The move_mouse tool returns the actual position
	result, err := client.CallToolString(ctx, "move_mouse", map[string]interface{}{
		"x": testX,
		"y": testY,
	})
	if err != nil {
		t.Fatalf("MoveMouse verification failed: %v", err)
	}

	// Parse result
	var moveResult struct {
		Success bool `json:"success"`
		X       int  `json:"x"`
		Y       int  `json:"y"`
	}
	if err := json.Unmarshal([]byte(result), &moveResult); err != nil {
		t.Fatalf("Failed to parse move result: %v", err)
	}

	if !moveResult.Success {
		t.Error("Move reported failure")
	}

	t.Logf("✓ Mouse moved to (%d, %d)", moveResult.X, moveResult.Y)
}

// TestIntegration_ClickResultFormat verifies click result format
func TestIntegration_ClickResultFormat(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	result, err := client.CallToolString(ctx, "click", map[string]interface{}{
		"button": "left",
	})
	if err != nil {
		t.Fatalf("Click failed: %v", err)
	}

	// Parse and verify format
	var clickResult map[string]interface{}
	if err := json.Unmarshal([]byte(result), &clickResult); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if success, ok := clickResult["success"].(bool); !ok || !success {
		t.Error("Click did not report success")
	}
	if _, ok := clickResult["button"]; !ok {
		t.Error("Result missing 'button' field")
	}

	t.Logf("✓ Click result format valid")
}

// TestIntegration_TypeTextResultFormat verifies type_text result format
func TestIntegration_TypeTextResultFormat(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	testText := "Test message"
	result, err := client.CallToolString(ctx, "type_text", map[string]interface{}{
		"text": testText,
	})
	if err != nil {
		t.Fatalf("TypeText failed: %v", err)
	}

	// Parse and verify
	var typeResult struct {
		Success         bool `json:"success"`
		CharactersTyped int  `json:"characters_typed"`
	}
	if err := json.Unmarshal([]byte(result), &typeResult); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if !typeResult.Success {
		t.Error("TypeText did not report success")
	}
	if typeResult.CharactersTyped != len(testText) {
		t.Errorf("Expected %d chars, got %d", len(testText), typeResult.CharactersTyped)
	}

	t.Logf("✓ TypeText result format valid (%d characters)", typeResult.CharactersTyped)
}

// TestIntegration_PressKeyResultFormat verifies press_key result format
func TestIntegration_PressKeyResultFormat(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	testKey := "enter"
	result, err := client.CallToolString(ctx, "press_key", map[string]interface{}{
		"key": testKey,
	})
	if err != nil {
		t.Fatalf("PressKey failed: %v", err)
	}

	// Parse and verify
	var keyResult struct {
		Success bool   `json:"success"`
		Key     string `json:"key"`
	}
	if err := json.Unmarshal([]byte(result), &keyResult); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if !keyResult.Success {
		t.Error("PressKey did not report success")
	}
	if keyResult.Key != testKey {
		t.Errorf("Expected key '%s', got '%s'", testKey, keyResult.Key)
	}

	t.Logf("✓ PressKey result format valid (key: %s)", keyResult.Key)
}

// TestIntegration_TakeScreenshotResultFormat verifies take_screenshot result format
func TestIntegration_TakeScreenshotResultFormat(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	result, err := client.CallToolString(ctx, "take_screenshot", nil)
	if err != nil {
		t.Fatalf("TakeScreenshot failed: %v", err)
	}

	// Parse and verify
	var screenshotResult struct {
		Success  bool   `json:"success"`
		Filepath string `json:"filepath"`
		Base64   string `json:"base64"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	}
	if err := json.Unmarshal([]byte(result), &screenshotResult); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if !screenshotResult.Success {
		t.Error("TakeScreenshot did not report success")
	}
	if screenshotResult.Base64 == "" {
		t.Error("Base64 data is empty")
	}
	if screenshotResult.Width <= 0 || screenshotResult.Height <= 0 {
		t.Errorf("Invalid dimensions: %dx%d", screenshotResult.Width, screenshotResult.Height)
	}

	// Verify base64 is valid
	_, err = base64.StdEncoding.DecodeString(screenshotResult.Base64)
	if err != nil {
		t.Errorf("Invalid base64 data: %v", err)
	}

	t.Logf("✓ TakeScreenshot result format valid (%dx%d)", screenshotResult.Width, screenshotResult.Height)
}

// TestIntegration_CompleteToolSuite runs all tools and reports summary
func TestIntegration_CompleteToolSuite(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: testTimeout,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	results := make(map[string]bool)

	// Test each tool
	t.Run("get_screen_size", func(t *testing.T) {
		_, _, err := client.GetScreenSize(ctx)
		results["get_screen_size"] = err == nil
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
	})

	t.Run("move_mouse", func(t *testing.T) {
		err := client.MoveMouse(ctx, 100, 100)
		results["move_mouse"] = err == nil
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
	})

	t.Run("click", func(t *testing.T) {
		err := client.Click(ctx, "left")
		results["click"] = err == nil
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
	})

	t.Run("type_text", func(t *testing.T) {
		err := client.TypeText(ctx, "test")
		results["type_text"] = err == nil
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
	})

	t.Run("press_key", func(t *testing.T) {
		err := client.PressKey(ctx, "enter")
		results["press_key"] = err == nil
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
	})

	t.Run("take_screenshot", func(t *testing.T) {
		_, _, _, _, err := client.TakeScreenshot(ctx)
		results["take_screenshot"] = err == nil
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
	})

	t.Run("read_screen_text", func(t *testing.T) {
		_, _, err := client.ReadScreenText(ctx, nil)
		results["read_screen_text"] = err == nil
		if err != nil {
			t.Errorf("Failed: %v", err)
		}
	})

	// Summary
	t.Log("\n=== Tool Suite Summary ===")
	passed := 0
	failed := 0
	for tool, ok := range results {
		status := "✓ PASS"
		if !ok {
			status = "✗ FAIL"
			failed++
		} else {
			passed++
		}
		t.Logf("%s: %s", status, tool)
	}
	t.Logf("\nTotal: %d passed, %d failed", passed, failed)

	if failed > 0 {
		t.Errorf("%d tools failed", failed)
	}
}
