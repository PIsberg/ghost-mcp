//go:build integration

// test_fixture_safety_test.go - Integration tests for safety features and loop protection
package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/mcpclient"
)

// =============================================================================
// CALL LIMIT TRACKING TESTS
// =============================================================================

// TestCallLimitTracking_VerifyGlobalLimit tests that the 25-call limit is enforced
func TestCallLimitTracking_VerifyGlobalLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}

	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// Make 24 successful calls (should all work)
	for i := 0; i < 24; i++ {
		result, err := client.CallToolString(ctx, "get_screen_size", nil)
		if err != nil {
			t.Fatalf("Call %d failed unexpectedly: %v", i+1, err)
		}

		// Verify response contains screen size data
		if !strings.Contains(result, "width") || !strings.Contains(result, "height") {
			t.Errorf("Call %d: Invalid response format: %s", i+1, result)
		}
	}

	// 25th call should trigger the limit
	result, err := client.CallToolString(ctx, "get_screen_size", nil)
	if err == nil {
		t.Error("Expected error on 25th call, but got success")
	}

	// Verify error message mentions the limit
	if !strings.Contains(result, "MAXIMUM TOOL CALLS REACHED") &&
		!strings.Contains(result, "25") {
		t.Errorf("Expected limit warning in response, got: %s", result)
	}

	t.Logf("✓ Global call limit enforced at 25 calls")
}

// TestCallLimitTracking_ConsecutiveFailures tests the 3-strike failure detection
func TestCallLimitTracking_ConsecutiveFailures(t *testing.T) {
	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// Make 3 consecutive failing calls with same text
	for i := 0; i < 3; i++ {
		result, err := client.CallTool(ctx, "find_and_click", map[string]interface{}{
			"text": "NONEXISTENT_BUTTON_XYZ123",
		})

		if err != nil {
			t.Fatalf("Call %d failed to execute: %v", i+1, err)
		}

		// Parse the error response
		var response struct {
			Error                string `json:"error"`
			ConsecutiveFailures  int    `json:"consecutive_failures"`
			RemainingCalls       int    `json:"remaining_calls"`
		}

		if len(result.Content) > 0 {
			json.Unmarshal([]byte(result.Content[0].Text), &response)
		}

		// Verify consecutive_failures counter increments
		expectedFailures := i + 1
		if response.ConsecutiveFailures != expectedFailures {
			t.Errorf("Call %d: Expected consecutive_failures=%d, got %d",
				i+1, expectedFailures, response.ConsecutiveFailures)
		}

		t.Logf("Call %d: consecutive_failures=%d, remaining_calls=%d",
			i+1, response.ConsecutiveFailures, response.RemainingCalls)
	}

	// 4th call should have GIVE UP recommendation
	result, err := client.CallTool(ctx, "find_and_click", map[string]interface{}{
		"text": "NONEXISTENT_BUTTON_XYZ123",
	})

	if err != nil {
		t.Fatalf("Call 4 failed to execute: %v", err)
	}

	// Check for GIVE UP recommendation
	if !strings.Contains(result.Content[0].Text, "GIVE UP RECOMMENDATION") {
		t.Errorf("Expected GIVE UP recommendation after 3 failures, got: %s", result.Content[0].Text)
	}

	t.Logf("✓ Consecutive failure detection working (3 strikes)")
}

// =============================================================================
// REPEATED CLICK DETECTION TESTS
// =============================================================================

// TestRepeatedClickDetection_WarnsAfterFiveClicks tests click repetition warning
func TestRepeatedClickDetection_WarnsAfterFiveClicks(t *testing.T) {
	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// Get screen size for valid coordinates
	sizeResult, err := client.CallToolString(ctx, "get_screen_size", nil)
	if err != nil {
		t.Fatalf("Failed to get screen size: %v", err)
	}

	var screenSize struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	json.Unmarshal([]byte(sizeResult), &screenSize)

	// Use center of screen for testing
	x := screenSize.Width / 2
	y := screenSize.Height / 2

	// Click 5 times at same coordinates
	for i := 0; i < 5; i++ {
		result, err := client.CallTool(ctx, "click_at", map[string]interface{}{
			"x":        x,
			"y":        y,
			"button":   "left",
			"delay_ms": 100,
		})

		if err != nil {
			t.Fatalf("Click %d failed: %v", i+1, err)
		}

		// First 4 clicks should not have warning
		if i < 4 {
			if strings.Contains(result.Content[0].Text, "warning") {
				t.Errorf("Click %d: Unexpected warning (should appear on click 5+)", i+1)
			}
		}

		t.Logf("Click %d at (%d,%d): OK", i+1, x, y)
		time.Sleep(100 * time.Millisecond)
	}

	// 6th click should trigger warning
	result, err := client.CallTool(ctx, "click_at", map[string]interface{}{
		"x":        x,
		"y":        y,
		"button":   "left",
		"delay_ms": 100,
	})

	if err != nil {
		t.Fatalf("Click 6 failed: %v", err)
	}

	// Check for warning in response
	if !strings.Contains(result.Content[0].Text, "warning") ||
		!strings.Contains(result.Content[0].Text, "should_stop") {
		t.Errorf("Expected click warning on 6th click, got: %s", result.Content[0].Text)
	}

	t.Logf("✓ Repeated click detection working (warns after 5 clicks)")
}

// =============================================================================
// CLICK_UNTIL_TEXT_APPEARS TESTS
// =============================================================================

// TestClickUntilTextAppears_Success tests successful text detection
func TestClickUntilTextAppears_Success(t *testing.T) {
	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// First, use find_elements to find some visible text
	elementsResult, err := client.CallToolString(ctx, "find_elements", nil)
	if err != nil {
		t.Skipf("Cannot test click_until_text_appears: find_elements failed: %v", err)
	}

	// Parse elements to find a clickable one
	var elementsData struct {
		Success    bool `json:"success"`
		ElementCount int `json:"element_count"`
		Elements   []struct {
			Text      string `json:"text"`
			CenterX   int    `json:"center_x"`
			CenterY   int    `json:"center_y"`
		} `json:"elements"`
	}
	json.Unmarshal([]byte(elementsResult), &elementsData)

	if elementsData.ElementCount == 0 {
		t.Skip("No elements found on screen for testing")
	}

	// Use the first element's coordinates
	elem := elementsData.Elements[0]

	// Test: Click and wait for text that's already visible (should succeed immediately)
	result, err := client.CallTool(ctx, "click_until_text_appears", map[string]interface{}{
		"x":               elem.CenterX,
		"y":               elem.CenterY,
		"wait_for_text":   elem.Text,
		"timeout_ms":      2000,
		"max_clicks":      1,
		"button":          "left",
	})

	if err != nil {
		t.Fatalf("click_until_text_appears failed: %v", err)
	}

	// Parse response
	var response struct {
		Success  bool   `json:"success"`
		Text     string `json:"text"`
		Clicks   int    `json:"clicks"`
		WaitedMs int    `json:"waited_ms"`
		Found    bool   `json:"found"`
	}
	json.Unmarshal([]byte(result.Content[0].Text), &response)

	if !response.Success || !response.Found {
		t.Errorf("Expected success, got: %+v", response)
	}

	if response.Clicks != 1 {
		t.Errorf("Expected 1 click, got %d", response.Clicks)
	}

	t.Logf("✓ click_until_text_appears: Found %q in %dms with %d click(s)",
		response.Text, response.WaitedMs, response.Clicks)
}

// TestClickUntilTextAppears_Timeout tests timeout when text never appears
func TestClickUntilTextAppears_Timeout(t *testing.T) {
	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// Use coordinates that won't have our test text
	result, err := client.CallTool(ctx, "click_until_text_appears", map[string]interface{}{
		"x":               100,
		"y":               100,
		"wait_for_text":   "NONEXISTENT_TEXT_XYZ789",
		"timeout_ms":      1000, // Short timeout for test
		"max_clicks":      2,    // Limited clicks
		"button":          "left",
	})

	if err != nil {
		t.Fatalf("click_until_text_appears failed to execute: %v", err)
	}

	// Parse response
	var response struct {
		Success  bool   `json:"success"`
		Text     string `json:"text"`
		Clicks   int    `json:"clicks"`
		WaitedMs int    `json:"waited_ms"`
		Found    bool   `json:"found"`
		Error    string `json:"error"`
	}
	json.Unmarshal([]byte(result.Content[0].Text), &response)

	if response.Success || response.Found {
		t.Errorf("Expected failure (text doesn't exist), got success")
	}

	if response.Clicks > 2 {
		t.Errorf("Expected max 2 clicks, got %d", response.Clicks)
	}

	if !strings.Contains(response.Error, "did not appear") {
		t.Errorf("Expected timeout error, got: %s", response.Error)
	}

	t.Logf("✓ click_until_text_appears: Correctly timed out after %d clicks", response.Clicks)
}

// =============================================================================
// SCROLL-AND-SEARCH TESTS
// =============================================================================

// TestFindAndClick_WithScrollDirection tests scroll-and-search mode
func TestFindAndClick_WithScrollDirection(t *testing.T) {
	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// Try to find text that may require scrolling
	// Use a common word that might be off-screen
	result, err := client.CallTool(ctx, "find_and_click", map[string]interface{}{
		"text":              "Click",
		"scroll_direction":  "down",
		"max_scrolls":       3,
		"scroll_amount":     5,
	})

	if err != nil {
		t.Fatalf("find_and_click with scroll failed: %v", err)
	}

	// Response should indicate whether it scrolled
	responseText := result.Content[0].Text

	// Either it found the text (success) or it didn't (failure with candidates)
	if strings.Contains(responseText, `"success":true`) {
		t.Logf("✓ find_and_click with scroll: Found text successfully")
	} else {
		// Check that candidates were provided to help AI decide next action
		if !strings.Contains(responseText, "candidates") {
			t.Error("Expected candidates array in failure response")
		}
		if !strings.Contains(responseText, "suggestion") {
			t.Error("Expected suggestion field in failure response")
		}
		t.Logf("✓ find_and_click with scroll: Provided helpful failure response")
	}
}

// =============================================================================
// MULTI-PAGE SEARCH TESTS
// =============================================================================

// TestFindAndClick_MultiPageSearch tests select_best mode
func TestFindAndClick_MultiPageSearch(t *testing.T) {
	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// Test multi-page search with select_best
	// This simulates searching across multiple "pages" (tabs, paginated content)
	result, err := client.CallTool(ctx, "find_and_click", map[string]interface{}{
		"text":           "Click",
		"next_page_keys": "Page_Down",
		"max_pages":      2,
		"select_best":    true,
	})

	if err != nil {
		t.Fatalf("find_and_click multi-page failed: %v", err)
	}

	responseText := result.Content[0].Text

	// Check response format
	if strings.Contains(responseText, `"success":true`) {
		// Should include page number where found
		if !strings.Contains(responseText, "page") {
			t.Error("Expected 'page' field in multi-page success response")
		}
		// Should include score for select_best mode
		if !strings.Contains(responseText, "score") {
			t.Error("Expected 'score' field in select_best response")
		}
		t.Logf("✓ Multi-page search with select_best: Found on page with score")
	} else {
		t.Logf("✓ Multi-page search: Correctly reported not found")
	}
}

// =============================================================================
// VIEWPORT AWARENESS TESTS
// =============================================================================

// TestViewportAwareness_ScrollSuggestion tests that scroll suggestions are provided
func TestViewportAwareness_ScrollSuggestion(t *testing.T) {
	client, cleanup := newFixtureClient(t)
	defer cleanup()

	ctx := context.Background()

	// Search for text that exists but may be off-screen
	result, err := client.CallTool(ctx, "find_and_click", map[string]interface{}{
		"text": "Click",
	})

	if err != nil {
		t.Fatalf("find_and_click failed: %v", err)
	}

	responseText := result.Content[0].Text

	// Check for suggestion field
	if strings.Contains(responseText, `"suggestion"`) {
		// Verify suggestion is one of the expected values
		validSuggestions := []string{
			"scroll_may_help",
			"text_continues_off_screen",
			"try_different_search_term",
			"no_matches_found",
		}

		foundValid := false
		for _, valid := range validSuggestions {
			if strings.Contains(responseText, valid) {
				foundValid = true
				break
			}
		}

		if !foundValid && !strings.Contains(responseText, `"success":true`) {
			t.Errorf("Invalid suggestion value in response")
		}
		t.Logf("✓ Viewport awareness: Suggestion field present")
	} else if !strings.Contains(responseText, `"success":true`) {
		t.Error("Expected suggestion field in failure response")
	}
}
