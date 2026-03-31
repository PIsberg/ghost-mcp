//go:build integration

// integration_fixture_test.go - Integration tests for all fixture GUI elements
//
// These tests verify the MCP server can interact with every GUI element
// in the test fixture HTML page.
//
// Run with: INTEGRATION=1 go test -v -run TestFixture ./cmd/ghost-mcp
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/mcpclient"
)

// =============================================================================
// FIXTURE TEST HELPERS
// =============================================================================

// ensureFixtureServer starts the fixture server only if it isn't already responding.
// Returns a cleanup function — a no-op if the server was already running externally.
func ensureFixtureServer(t *testing.T) func() {
	t.Helper()
	resp, err := http.Get(fixtureURL)
	if err == nil {
		resp.Body.Close()
		return func() {} // already running, don't stop it
	}
	_, cleanup := startFixtureServer(t)
	return cleanup
}

func setupFixtureTest(t *testing.T) (*mcpclient.Client, context.Context, func()) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}
	skipIfNoGCC(t)

	serverCleanup := ensureFixtureServer(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: 30 * time.Second,
	})
	if err != nil {
		serverCleanup()
		t.Fatalf("Failed to create MCP client: %v", err)
	}

	ctx := context.Background()
	time.Sleep(500 * time.Millisecond)

	cleanup := func() {
		client.Close()
		serverCleanup()
	}
	return client, ctx, cleanup
}

// verifyLastAction waits for the Last Action display to contain expectedText.
func verifyLastAction(t *testing.T, ctx context.Context, client *mcpclient.Client, expectedText string) {
	t.Helper()
	result, err := client.CallToolString(ctx, "wait_for_text", map[string]interface{}{
		"text":       expectedText,
		"timeout_ms": 3000,
	})
	if err != nil {
		t.Errorf("wait_for_text(%q) error: %v", expectedText, err)
		return
	}
	if !strings.Contains(result, `"visible":true`) {
		t.Errorf("Expected %q in Last Action, but not found: %s", expectedText, result)
	}
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func scrollFixtureToTop(t *testing.T, ctx context.Context, client *mcpclient.Client) {
	t.Helper()
	for i := 0; i < 4; i++ {
		if err := client.PressKey(ctx, "home"); err == nil {
			time.Sleep(150 * time.Millisecond)
		}
	}
}

// =============================================================================
// BUTTON TESTS
// =============================================================================

// TestFixture_Button_Primary clicks the Primary button and verifies the Last Action.
func TestFixture_Button_Primary(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Primary", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click Primary button: %v", err)
	}
	if !result.Success {
		t.Error("FindAndClick did not report success")
	}

	verifyLastAction(t, ctx, client, "Clicked PRIMARY")
	t.Logf("✓ Clicked Primary at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_Success clicks the Success button and verifies the Last Action.
func TestFixture_Button_Success(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Success", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click Success button: %v", err)
	}
	if !result.Success {
		t.Error("FindAndClick did not report success")
	}

	verifyLastAction(t, ctx, client, "Clicked SUCCESS")
	t.Logf("✓ Clicked Success at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_Warning clicks the Warning button and verifies the Last Action.
func TestFixture_Button_Warning(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Warning", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click Warning button: %v", err)
	}
	if !result.Success {
		t.Error("FindAndClick did not report success")
	}

	verifyLastAction(t, ctx, client, "Clicked WARNING")
	t.Logf("✓ Clicked Warning at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_Info clicks the Info button (white text on cyan gradient —
// requires the color OCR fallback) and verifies the Last Action.
func TestFixture_Button_Info(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Info", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click Info button: %v", err)
	}
	if !result.Success {
		t.Error("FindAndClick did not report success")
	}

	verifyLastAction(t, ctx, client, "Clicked INFO")
	t.Logf("✓ Clicked Info at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_AllButtons clicks all four buttons in sequence and verifies each.
func TestFixture_Button_AllButtons(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	cases := []struct{ label, expected string }{
		{"Primary", "Clicked PRIMARY"},
		{"Success", "Clicked SUCCESS"},
		{"Warning", "Clicked WARNING"},
		{"Info", "Clicked INFO"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			result, err := client.FindAndClick(ctx, tc.label, mcpclient.FindAndClickOptions{Button: "left"})
			if err != nil {
				t.Errorf("Failed to click %s: %v", tc.label, err)
				return
			}
			if !result.Success {
				t.Errorf("FindAndClick did not report success for %s", tc.label)
				return
			}
			verifyLastAction(t, ctx, client, tc.expected)
			t.Logf("✓ Clicked %s at (%d, %d)", tc.label, result.ActualX, result.ActualY)
		})
	}
}

// =============================================================================
// INPUT TESTS
// =============================================================================

// TestFixture_Input_TextField clicks the text input and types into it.
func TestFixture_Input_TextField(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	_, err := client.FindAndClick(ctx, "Type here", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click text input: %v", err)
	}

	if err := client.TypeText(ctx, "Hello from integration test!"); err != nil {
		t.Fatalf("Failed to type text: %v", err)
	}

	verifyLastAction(t, ctx, client, "Typed")
	t.Log("✓ Typed text into input field")
}

// TestFixture_FindClickAndType verifies the atomic find_click_and_type tool.
func TestFixture_FindClickAndType(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Clear the input field first from previous tests
	_, _ = client.CallToolString(ctx, "find_and_click", map[string]interface{}{
		"text":   "Clear",
		"button": "left",
		"nth":    1,
	})

	time.Sleep(3 * time.Second)

	// find "Type here", click, type text and press enter
	result, err := client.CallToolString(ctx, "find_click_and_type", map[string]interface{}{
		"text":        "Type here",
		"type_text":   "test find click and type",
		"press_enter": true,
	})
	if err != nil {
		t.Fatalf("Failed find_click_and_type: %v", err)
	}

	if !strings.Contains(result, `"success":true`) {
		t.Fatalf("Tool returned error: %s", result)
	}

	verifyLastAction(t, ctx, client, "Typed")
	t.Log("✓ Successfully tested find_click_and_type")
}

// TestFixture_Input_TextArea clicks the textarea and types multi-line text.
func TestFixture_Input_TextArea(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	_, err := client.FindAndClick(ctx, "Multi-line", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click textarea: %v", err)
	}

	if err := client.TypeText(ctx, "Line 1\nLine 2\nLine 3"); err != nil {
		t.Fatalf("Failed to type in textarea: %v", err)
	}

	verifyLastAction(t, ctx, client, "Typed")
	t.Log("✓ Typed multi-line text into textarea")
}

// TestFixture_Input_Clear types text then clicks the Clear button to erase it.
func TestFixture_Input_Clear(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Type some text first so there is something to clear.
	if _, err := client.FindAndClick(ctx, "Type here", mcpclient.FindAndClickOptions{}); err != nil {
		t.Fatalf("Failed to click text input: %v", err)
	}
	if err := client.TypeText(ctx, "will be cleared"); err != nil {
		t.Fatalf("TypeText failed: %v", err)
	}

	// "Clear" is the first occurrence on the page (Input Tests panel);
	// "Clear Log" is lower down in the Event Log panel.
	result, err := client.FindAndClick(ctx, "Clear", mcpclient.FindAndClickOptions{Nth: 1})
	if err != nil {
		t.Fatalf("Failed to click Clear button: %v", err)
	}
	if !result.Success {
		t.Fatalf("FindAndClick('Clear') reported failure")
	}

	verifyLastAction(t, ctx, client, "Cleared all inputs")
	t.Logf("✓ Clear button at (%d, %d) cleared inputs", result.ActualX, result.ActualY)
}

// =============================================================================
// CHECKBOX TESTS
// =============================================================================

// TestFixture_Checkbox_Option1 clicks the first checkbox and verifies it.
func TestFixture_Checkbox_Option1(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Option 1", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click Option 1 checkbox: %v", err)
	}

	// Last Action: "chk1 checked" or "chk1 unchecked" depending on prior state.
	verifyLastAction(t, ctx, client, "chk1")
	t.Logf("✓ Clicked Option 1 at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Checkbox_AllOptions clicks all three checkboxes and verifies each.
func TestFixture_Checkbox_AllOptions(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	cases := []struct{ label, lastActionFragment string }{
		{"Option 1", "chk1"},
		{"Option 2", "chk2"},
		{"Option 3", "chk3"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			result, err := client.FindAndClick(ctx, tc.label, mcpclient.FindAndClickOptions{Button: "left"})
			if err != nil {
				t.Errorf("Failed to click %s: %v", tc.label, err)
				return
			}
			verifyLastAction(t, ctx, client, tc.lastActionFragment)
			t.Logf("✓ Clicked %s at (%d, %d)", tc.label, result.ActualX, result.ActualY)
		})
	}
}

// =============================================================================
// RADIO BUTTON TESTS
// =============================================================================

// TestFixture_Radio_AllChoices clicks all three radio buttons and verifies each.
func TestFixture_Radio_AllChoices(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	cases := []struct{ label, lastActionFragment string }{
		{"Choice A", "Selected radio a"},
		{"Choice B", "Selected radio b"},
		{"Choice C", "Selected radio c"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			result, err := client.FindAndClick(ctx, tc.label, mcpclient.FindAndClickOptions{Button: "left"})
			if err != nil {
				t.Errorf("Failed to click %s: %v", tc.label, err)
				return
			}
			verifyLastAction(t, ctx, client, tc.lastActionFragment)
			t.Logf("✓ Clicked %s at (%d, %d)", tc.label, result.ActualX, result.ActualY)
		})
	}
}

// =============================================================================
// DROPDOWN TEST
// =============================================================================

// TestFixture_Dropdown_Select opens the dropdown and selects the first option
// using the ArrowDown key (fires onchange, no need to press Enter).
func TestFixture_Dropdown_Select(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Click the select element to focus it.
	_, err := client.FindAndClick(ctx, "Select an option", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click dropdown: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Arrow down moves to Option 1 and fires onchange → "Dropdown: option1"
	if err := client.PressKey(ctx, "down"); err != nil {
		t.Fatalf("Failed to press down key: %v", err)
	}

	verifyLastAction(t, ctx, client, "Dropdown:")
	t.Log("✓ Dropdown selection verified")
}

// =============================================================================
// SLIDER TEST
// =============================================================================

// TestFixture_Slider_Keyboard focuses the slider via find_and_click on its value
// label, then uses arrow keys to change the value.
// Note: dragging the slider track is not currently possible (no drag tool).
func TestFixture_Slider_Keyboard(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// The slider value label "50%" is next to the slider track.
	// Clicking it doesn't focus the range input, but we can use find_elements
	// to locate the "Slider Test" panel and tab to the slider input.
	// Simplest cross-platform approach: click the slider track region by
	// locating the "Slider" heading and clicking below it.
	result, err := client.CallToolString(ctx, "find_elements", nil)
	if err != nil {
		t.Fatalf("find_elements failed: %v", err)
	}

	var elemData struct {
		Elements []struct {
			Text    string `json:"text"`
			CenterX int    `json:"center_x"`
			CenterY int    `json:"center_y"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(result), &elemData); err != nil {
		t.Fatalf("Failed to parse elements: %v", err)
	}

	// Find the "Slider" heading to determine the y position of the slider panel.
	var sliderHeadingY int
	for _, el := range elemData.Elements {
		if strings.Contains(strings.ToLower(el.Text), "slider") {
			sliderHeadingY = el.CenterY
			break
		}
	}
	if sliderHeadingY == 0 {
		t.Skip("Could not locate Slider panel — skipping slider test")
	}

	// Click roughly where the slider track is (below the heading, center of screen width).
	screenW, _, err := client.GetScreenSize(ctx)
	if err != nil {
		t.Fatalf("GetScreenSize failed: %v", err)
	}
	if err := client.MoveMouse(ctx, screenW/2, sliderHeadingY+50); err != nil {
		t.Fatalf("MoveMouse failed: %v", err)
	}
	if err := client.Click(ctx, "left"); err != nil {
		t.Fatalf("Click failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Arrow right increments the slider value.
	if err := client.PressKey(ctx, "right"); err != nil {
		t.Fatalf("PressKey failed: %v", err)
	}

	verifyLastAction(t, ctx, client, "Slider:")
	t.Log("✓ Slider keyboard interaction verified")
}

// =============================================================================
// COLOR PICKER TEST
// =============================================================================

// TestFixture_ColorPicker clicks a color button using take_screenshot + click_at.
// Color picker buttons are icon-only (no text labels) so OCR cannot find them;
// this test uses a screenshot to locate the color controls area, then clicks
// the first color button by its approximate position within the panel.
func TestFixture_ColorPicker(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Find the "Color Picker" heading to anchor the panel position.
	result, err := client.CallToolString(ctx, "find_elements", nil)
	if err != nil {
		t.Fatalf("find_elements failed: %v", err)
	}

	var elemData struct {
		Elements []struct {
			Text    string `json:"text"`
			CenterX int    `json:"center_x"`
			CenterY int    `json:"center_y"`
			Width   int    `json:"width"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(result), &elemData); err != nil {
		t.Fatalf("Failed to parse elements: %v", err)
	}

	var panelX, panelY, panelW int
	for _, el := range elemData.Elements {
		if strings.Contains(strings.ToLower(el.Text), "color") {
			panelX = el.CenterX - el.Width/2
			panelY = el.CenterY
			panelW = el.Width
			break
		}
	}
	if panelY == 0 {
		t.Skip("Could not locate Color Picker panel — skipping color picker test")
	}

	// The color buttons are in a row roughly 60px below the heading.
	// The first color button (#ff4444) is near the left edge of the controls area.
	// Take a small screenshot to visually confirm the region before clicking.
	_, err = client.CallToolString(ctx, "take_screenshot", map[string]interface{}{
		"x":       panelX,
		"y":       panelY + 40,
		"width":   panelW,
		"height":  80,
		"quality": 85,
	})
	if err != nil {
		t.Logf("Screenshot failed (non-fatal): %v", err)
	}

	// Click approximate position of the first color button.
	colorBtnX := panelX + 120 // color box (~100px) + gap, then first color button
	colorBtnY := panelY + 70
	_, err = client.CallToolString(ctx, "click_at", map[string]interface{}{
		"x": colorBtnX,
		"y": colorBtnY,
	})
	if err != nil {
		t.Fatalf("click_at failed: %v", err)
	}

	verifyLastAction(t, ctx, client, "Color:")
	t.Logf("✓ Color picker button clicked at approx (%d, %d)", colorBtnX, colorBtnY)
}

// =============================================================================
// CLICK COUNTER TEST
// =============================================================================

// TestFixture_Counter_Button clicks the counter button multiple times
// and verifies the counter increments via Last Action.
func TestFixture_Counter_Button(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	for i := 1; i <= 3; i++ {
		result, err := client.FindAndClick(ctx, "Click Me", mcpclient.FindAndClickOptions{Button: "left"})
		if err != nil {
			t.Fatalf("Click %d: failed to click counter button: %v", i, err)
		}
		if !result.Success {
			t.Errorf("Click %d: FindAndClick did not report success", i)
		}
	}

	verifyLastAction(t, ctx, client, "Counter:")
	t.Log("✓ Counter button clicked 3 times")
}

// =============================================================================
// HOVER ZONE TEST
// =============================================================================

// TestFixture_Hover_Zone moves the mouse over the hover detection zone (without
// clicking) and verifies the onmouseenter event fires via Last Action.
func TestFixture_Hover_Zone(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Locate the hover zone text with find_elements.
	result, err := client.CallToolString(ctx, "find_elements", nil)
	if err != nil {
		t.Fatalf("find_elements failed: %v", err)
	}

	var elemData struct {
		Elements []struct {
			Text    string `json:"text"`
			CenterX int    `json:"center_x"`
			CenterY int    `json:"center_y"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(result), &elemData); err != nil {
		t.Fatalf("Failed to parse elements: %v", err)
	}

	var hoverX, hoverY int
	for _, el := range elemData.Elements {
		if strings.Contains(strings.ToLower(el.Text), "move") {
			hoverX = el.CenterX
			hoverY = el.CenterY
			break
		}
	}
	if hoverY == 0 {
		t.Skip("Could not locate hover zone text — skipping hover test")
	}

	// Move the mouse into the hover zone (triggers onmouseenter, no click needed).
	if err := client.MoveMouse(ctx, hoverX, hoverY); err != nil {
		t.Fatalf("MoveMouse failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond) // give the browser time to fire the event

	verifyLastAction(t, ctx, client, "Mouse in hover zone")
	t.Logf("✓ Hover zone triggered at (%d, %d)", hoverX, hoverY)
}

// =============================================================================
// KEYBOARD INPUT TEST
// =============================================================================

// TestFixture_Keyboard_Input clicks the keyboard test input, types a distinctive
// string, and verifies it appears in the input field.
func TestFixture_Keyboard_Input(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	_, err := client.FindAndClick(ctx, "Focus here and press keys", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click keyboard test input: %v", err)
	}

	const testInput = "ghost"
	if err := client.TypeText(ctx, testInput); err != nil {
		t.Fatalf("TypeText failed: %v", err)
	}

	// The typed text appears in the input field and is readable by OCR.
	result, err := client.CallToolString(ctx, "wait_for_text", map[string]interface{}{
		"text":       testInput,
		"timeout_ms": 3000,
	})
	if err != nil {
		t.Fatalf("wait_for_text failed: %v", err)
	}
	if !strings.Contains(result, `"visible":true`) {
		t.Errorf("Typed text %q not found on screen: %s", testInput, result)
	}
	t.Logf("✓ Keyboard input %q verified on screen", testInput)
}

// =============================================================================
// EVENT LOG TEST
// =============================================================================

// TestFixture_ClearLog clicks the Clear Log button and verifies the log resets.
func TestFixture_ClearLog(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Click the Clear Log button (distinct from the Input Tests "Clear" button).
	result, err := client.FindAndClick(ctx, "Clear Log", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("Failed to click Clear Log button: %v", err)
	}
	if !result.Success {
		t.Fatalf("FindAndClick('Clear Log') reported failure")
	}

	// After clearing, the event log shows "Log cleared".
	logResult, err := client.CallToolString(ctx, "wait_for_text", map[string]interface{}{
		"text":       "Log cleared",
		"timeout_ms": 3000,
	})
	if err != nil {
		t.Fatalf("wait_for_text failed: %v", err)
	}
	if !strings.Contains(logResult, `"visible":true`) {
		t.Errorf("Expected 'Log cleared' in event log: %s", logResult)
	}
	t.Logf("✓ Clear Log button at (%d, %d) reset the event log", result.ActualX, result.ActualY)
}

// =============================================================================
// SCROLL TESTS
// =============================================================================

// TestFixture_ScrollUntilText_ClearLog verifies the bounded page-search helper
// can locate a lower-page control without oscillating through manual scroll loops.
func TestFixture_ScrollUntilText_ClearLog(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	scrollFixtureToTop(t, ctx, client)

	result, err := client.CallToolString(ctx, "scroll_until_text", map[string]interface{}{
		"text":      "Clear Log",
		"direction": "down",
		"amount":    5,
		"max_steps": 8,
		"delay_ms":  100,
	})
	if err != nil {
		t.Fatalf("scroll_until_text failed: %v", err)
	}

	var data struct {
		Success        bool   `json:"success"`
		Found          bool   `json:"found"`
		Text           string `json:"text"`
		StepsTaken     int    `json:"steps_taken"`
		MaxSteps       int    `json:"max_steps"`
		StopReason     string `json:"stop_reason"`
		BoundaryLikely bool   `json:"boundary_likely"`
		VisibleText    string `json:"visible_text"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("scroll_until_text returned invalid JSON: %v\nraw: %s", err, result)
	}
	if !data.Success {
		t.Fatalf("scroll_until_text did not report success: %s", result)
	}
	if !data.Found {
		t.Fatalf("expected to find Clear Log while scrolling, got: %s", result)
	}
	if data.StopReason != "found" && data.StopReason != "already_visible" {
		t.Fatalf("unexpected stop_reason %q in: %s", data.StopReason, result)
	}
	if !containsIgnoreCase(data.VisibleText, "Clear Log") {
		t.Fatalf("post-scroll OCR does not contain Clear Log: %s", result)
	}

	clickResult, err := client.FindAndClick(ctx, "Clear Log", mcpclient.FindAndClickOptions{Button: "left"})
	if err != nil {
		t.Fatalf("FindAndClick('Clear Log') after scroll_until_text failed: %v", err)
	}
	if !clickResult.Success {
		t.Fatalf("FindAndClick('Clear Log') did not report success")
	}
	verifyLastAction(t, ctx, client, "Log cleared")
	t.Logf("✓ scroll_until_text found Clear Log in %d step(s)", data.StepsTaken)
}

// TestFixture_Scroll_StructuredFeedback verifies scroll returns the additional
// search-friendly fields needed to avoid blind up/down oscillation.
func TestFixture_Scroll_StructuredFeedback(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	scrollFixtureToTop(t, ctx, client)

	result, err := client.CallToolString(ctx, "scroll", map[string]interface{}{
		"direction":   "down",
		"amount":      5,
		"search_text": "Clear Log",
	})
	if err != nil {
		t.Fatalf("scroll failed: %v", err)
	}

	var data struct {
		Success         bool   `json:"success"`
		Direction       string `json:"direction"`
		Amount          int    `json:"amount"`
		SearchText      string `json:"search_text"`
		TextFound       bool   `json:"text_found"`
		VisibleText     string `json:"visible_text"`
		VisibleTextHash string `json:"visible_text_hash"`
		ViewportChanged bool   `json:"viewport_changed"`
		BoundaryLikely  bool   `json:"boundary_likely"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		t.Fatalf("scroll returned invalid JSON: %v\nraw: %s", err, result)
	}
	if !data.Success {
		t.Fatalf("scroll did not report success: %s", result)
	}
	if data.Direction != "down" {
		t.Fatalf("Direction = %q; want down", data.Direction)
	}
	if data.Amount != 5 {
		t.Fatalf("Amount = %d; want 5", data.Amount)
	}
	if data.SearchText != "Clear Log" {
		t.Fatalf("SearchText = %q; want Clear Log", data.SearchText)
	}
	if data.VisibleTextHash == "" {
		t.Fatalf("VisibleTextHash must not be empty: %s", result)
	}
	if data.VisibleText == "" {
		t.Fatalf("VisibleText must not be empty: %s", result)
	}
	if !data.ViewportChanged && !data.BoundaryLikely {
		t.Fatalf("expected either viewport change or boundary detection: %s", result)
	}

	t.Logf("✓ scroll returned structured feedback (changed=%t boundary=%t found=%t)", data.ViewportChanged, data.BoundaryLikely, data.TextFound)
}

// =============================================================================
// OCR PANEL TEST
// =============================================================================

// TestFixture_OCR_Panel reads the OCR test panel and verifies known words are present.
func TestFixture_OCR_Panel(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	text, _, err := client.ReadScreenText(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to read screen: %v", err)
	}

	for _, word := range []string{"Ghost", "MCP", "Hello", "World"} {
		if containsIgnoreCase(text, word) {
			t.Logf("✓ Found expected word: %s", word)
		} else {
			t.Errorf("Expected word %q not found in OCR output", word)
		}
	}
}

// =============================================================================
// COMPREHENSIVE WORKFLOW TEST
// =============================================================================

// TestFixture_CompleteWorkflow exercises every interactive element on the fixture page.
func TestFixture_CompleteWorkflow(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	t.Log("=== Starting Complete Fixture Workflow ===")

	step := func(name string, fn func() error) {
		t.Helper()
		if err := fn(); err != nil {
			t.Errorf("Step %q failed: %v", name, err)
		} else {
			t.Logf("✓ %s", name)
		}
	}

	// Buttons
	for _, btn := range []string{"Primary", "Success", "Warning", "Info"} {
		btn := btn
		step("Button: "+btn, func() error {
			r, err := client.FindAndClick(ctx, btn, mcpclient.FindAndClickOptions{})
			if err != nil || !r.Success {
				return err
			}
			return nil
		})
	}

	// Text input
	step("Text input", func() error {
		if _, err := client.FindAndClick(ctx, "Type here", mcpclient.FindAndClickOptions{}); err != nil {
			return err
		}
		return client.TypeText(ctx, "workflow test")
	})

	// Clear
	step("Clear input", func() error {
		_, err := client.FindAndClick(ctx, "Clear", mcpclient.FindAndClickOptions{Nth: 1})
		return err
	})

	// Checkboxes
	for _, opt := range []string{"Option 1", "Option 2", "Option 3"} {
		opt := opt
		step("Checkbox: "+opt, func() error {
			_, err := client.FindAndClick(ctx, opt, mcpclient.FindAndClickOptions{})
			return err
		})
	}

	// Radio buttons
	for _, choice := range []string{"Choice A", "Choice B", "Choice C"} {
		choice := choice
		step("Radio: "+choice, func() error {
			_, err := client.FindAndClick(ctx, choice, mcpclient.FindAndClickOptions{})
			return err
		})
	}

	// Dropdown
	step("Dropdown: open and select", func() error {
		if _, err := client.FindAndClick(ctx, "Select an option", mcpclient.FindAndClickOptions{}); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
		return client.PressKey(ctx, "down")
	})

	// Click counter
	step("Counter button", func() error {
		_, err := client.FindAndClick(ctx, "Click Me", mcpclient.FindAndClickOptions{})
		return err
	})

	// Keyboard input
	step("Keyboard input", func() error {
		if _, err := client.FindAndClick(ctx, "Focus here and press keys", mcpclient.FindAndClickOptions{}); err != nil {
			return err
		}
		return client.TypeText(ctx, "test")
	})

	// Clear log
	step("Clear Log", func() error {
		_, err := client.FindAndClick(ctx, "Clear Log", mcpclient.FindAndClickOptions{})
		return err
	})

	t.Log("=== Complete Workflow Finished ===")
}
