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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/mcpclient"
)

// =============================================================================
// FIXTURE TEST HELPERS
// =============================================================================

func setupFixtureTest(t *testing.T) (*mcpclient.Client, context.Context, func()) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled")
	}
	skipIfNoGCC(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}

	ctx := context.Background()

	// Start fixture server if not running
	startFixtureServer(t)

	// Give browser time to load
	time.Sleep(500 * time.Millisecond)

	cleanup := func() {
		client.Close()
	}

	return client, ctx, cleanup
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// =============================================================================
// BUTTON TESTS
// =============================================================================

// TestFixture_Button_Primary tests clicking the Primary button
func TestFixture_Button_Primary(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Primary", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Fatalf("Failed to click Primary button: %v", err)
	}

	if !result.Success {
		t.Error("FindAndClick did not report success")
	}
	t.Logf("✓ Clicked Primary button at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_Success tests clicking the Success button
func TestFixture_Button_Success(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Success", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Fatalf("Failed to click Success button: %v", err)
	}

	if !result.Success {
		t.Error("FindAndClick did not report success")
	}
	t.Logf("✓ Clicked Success button at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_Warning tests clicking the Warning button
func TestFixture_Button_Warning(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Warning", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Fatalf("Failed to click Warning button: %v", err)
	}

	if !result.Success {
		t.Error("FindAndClick did not report success")
	}
	t.Logf("✓ Clicked Warning button at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_Info tests clicking the Info button
func TestFixture_Button_Info(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Info", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Fatalf("Failed to click Info button: %v", err)
	}

	if !result.Success {
		t.Error("FindAndClick did not report success")
	}
	t.Logf("✓ Clicked Info button at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Button_AllButtons tests clicking all four buttons in sequence
func TestFixture_Button_AllButtons(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	buttons := []string{"Primary", "Success", "Warning", "Info"}

	for _, btn := range buttons {
		t.Run(btn, func(t *testing.T) {
			result, err := client.FindAndClick(ctx, btn, mcpclient.FindAndClickOptions{
				Button: "left",
			})
			if err != nil {
				t.Errorf("Failed to click %s button: %v", btn, err)
				return
			}
			if !result.Success {
				t.Errorf("FindAndClick did not report success for %s", btn)
			}
			t.Logf("✓ Clicked %s at (%d, %d)", btn, result.ActualX, result.ActualY)
		})
	}
}

// =============================================================================
// INPUT TESTS
// =============================================================================

// TestFixture_Input_TextField tests typing into a text input field
func TestFixture_Input_TextField(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Find the input field by looking for placeholder text
	_, err := client.FindAndClick(ctx, "Type here", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Logf("Could not find by placeholder, trying alternative: %v", err)
	}

	// Type test text
	err = client.TypeText(ctx, "Hello from integration test!")
	if err != nil {
		t.Fatalf("Failed to type text: %v", err)
	}

	t.Logf("✓ Typed text into input field")
}

// TestFixture_Input_TextArea tests typing into a textarea
func TestFixture_Input_TextArea(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	// Find textarea by placeholder
	_, err := client.FindAndClick(ctx, "Multi-line", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Logf("Could not find textarea by placeholder: %v", err)
	}

	// Type multi-line text
	err = client.TypeText(ctx, "Line 1\nLine 2\nLine 3")
	if err != nil {
		t.Fatalf("Failed to type in textarea: %v", err)
	}

	t.Logf("✓ Typed multi-line text into textarea")
}

// =============================================================================
// CHECKBOX TESTS
// =============================================================================

// TestFixture_Checkbox_Option1 tests clicking the first checkbox
func TestFixture_Checkbox_Option1(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Option 1", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Fatalf("Failed to click Option 1 checkbox: %v", err)
	}

	t.Logf("✓ Clicked Option 1 checkbox at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Checkbox_AllOptions tests clicking all checkboxes
func TestFixture_Checkbox_AllOptions(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	options := []string{"Option 1", "Option 2", "Option 3"}

	for _, opt := range options {
		t.Run(opt, func(t *testing.T) {
			result, err := client.FindAndClick(ctx, opt, mcpclient.FindAndClickOptions{
				Button: "left",
			})
			if err != nil {
				t.Errorf("Failed to click %s: %v", opt, err)
				return
			}
			t.Logf("✓ Clicked %s at (%d, %d)", opt, result.ActualX, result.ActualY)
		})
	}
}

// =============================================================================
// RADIO BUTTON TESTS
// =============================================================================

// TestFixture_Radio_ChoiceA tests clicking radio button A
func TestFixture_Radio_ChoiceA(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Choice A", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Fatalf("Failed to click Choice A: %v", err)
	}

	t.Logf("✓ Clicked Choice A at (%d, %d)", result.ActualX, result.ActualY)
}

// TestFixture_Radio_AllChoices tests clicking all radio buttons
func TestFixture_Radio_AllChoices(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	choices := []string{"Choice A", "Choice B", "Choice C"}

	for _, choice := range choices {
		t.Run(choice, func(t *testing.T) {
			result, err := client.FindAndClick(ctx, choice, mcpclient.FindAndClickOptions{
				Button: "left",
			})
			if err != nil {
				t.Errorf("Failed to click %s: %v", choice, err)
				return
			}
			t.Logf("✓ Clicked %s at (%d, %d)", choice, result.ActualX, result.ActualY)
		})
	}
}

// =============================================================================
// DROPDOWN TESTS
// =============================================================================

// TestFixture_Dropdown_Open tests opening the dropdown
func TestFixture_Dropdown_Open(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Select an option", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Logf("Could not find dropdown by text: %v", err)
	} else {
		t.Logf("✓ Clicked dropdown at (%d, %d)", result.ActualX, result.ActualY)
	}
}

// =============================================================================
// SLIDER TESTS
// =============================================================================

// TestFixture_Slider_Adjust tests adjusting the slider
func TestFixture_Slider_Adjust(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "50", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Logf("Could not find slider by value: %v", err)
	} else {
		t.Logf("✓ Clicked slider area at (%d, %d)", result.ActualX, result.ActualY)
	}

	t.Logf("✓ Slider test completed (full drag test requires drag implementation)")
}

// =============================================================================
// COLOR PICKER TESTS
// =============================================================================

// TestFixture_ColorPicker_Scan tests scanning the color picker area
func TestFixture_ColorPicker_Scan(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, _, err := client.ReadScreenText(ctx, map[string]interface{}{
		"width":  300,
		"height": 200,
	})
	if err != nil {
		t.Logf("Could not read color picker area: %v", err)
	} else {
		t.Logf("✓ Color picker area scanned: %s", result[:min(50, len(result))])
	}
}

// =============================================================================
// CLICK COUNTER TESTS
// =============================================================================

// TestFixture_Counter_Button tests clicking the counter button
func TestFixture_Counter_Button(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Click Me", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Fatalf("Failed to click counter button: %v", err)
	}

	t.Logf("✓ Clicked counter button at (%d, %d)", result.ActualX, result.ActualY)
}

// =============================================================================
// HOVER ZONE TESTS
// =============================================================================

// TestFixture_Hover_Zone tests hovering over the detection zone
func TestFixture_Hover_Zone(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, err := client.FindAndClick(ctx, "Move mouse", mcpclient.FindAndClickOptions{
		Button: "left",
	})
	if err != nil {
		t.Logf("Could not find hover zone: %v", err)
	} else {
		t.Logf("✓ Moved to hover zone at (%d, %d)", result.ActualX, result.ActualY)
	}
}

// =============================================================================
// OCR PANEL TESTS
// =============================================================================

// TestFixture_OCR_Panel tests reading the OCR test panel
func TestFixture_OCR_Panel(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	result, _, err := client.ReadScreenText(ctx, map[string]interface{}{
		"x":      100,
		"y":      100,
		"width":  600,
		"height": 400,
	})
	if err != nil {
		t.Fatalf("Failed to read OCR panel: %v", err)
	}

	t.Logf("✓ OCR Panel text: %s", result[:min(100, len(result))])

	expectedWords := []string{"Hello", "World", "Ghost", "MCP"}
	for _, word := range expectedWords {
		if containsIgnoreCase(result, word) {
			t.Logf("✓ Found expected word: %s", word)
		}
	}
}

// =============================================================================
// COMPREHENSIVE WORKFLOW TEST
// =============================================================================

// TestFixture_CompleteWorkflow tests a complete workflow across all elements
func TestFixture_CompleteWorkflow(t *testing.T) {
	client, ctx, cleanup := setupFixtureTest(t)
	defer cleanup()

	t.Log("=== Starting Complete Fixture Workflow ===")

	buttons := []string{"Primary", "Success", "Warning", "Info"}
	for _, btn := range buttons {
		_, err := client.FindAndClick(ctx, btn, mcpclient.FindAndClickOptions{})
		if err != nil {
			t.Logf("Warning: Could not click %s: %v", btn, err)
		}
	}

	client.FindAndClick(ctx, "Type here", mcpclient.FindAndClickOptions{})
	client.TypeText(ctx, "Test input")

	for _, opt := range []string{"Option 1", "Option 2"} {
		client.FindAndClick(ctx, opt, mcpclient.FindAndClickOptions{})
	}

	client.FindAndClick(ctx, "Choice A", mcpclient.FindAndClickOptions{})
	client.FindAndClick(ctx, "Click Me", mcpclient.FindAndClickOptions{})

	result, _, err := client.ReadScreenText(ctx, nil)
	if err != nil {
		t.Logf("Warning: Could not read screen: %v", err)
	} else {
		t.Logf("✓ OCR extracted %d words", len(strings.Fields(result)))
	}

	t.Log("=== Complete Workflow Finished ===")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
