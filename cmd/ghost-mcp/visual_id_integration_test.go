//go:build integration

// visual_id_integration_test.go - Comprehensive validation of the Visual ID workflow.
//
// This test proves that Ghost MCP can successfully navigate the entire test fixture
// using ONLY numeric IDs, bypassing traditional coordinate or label-based automation.
//
// Run with: INTEGRATION=1 go test -v -run TestVisualIDWorkflow ./cmd/ghost-mcp
package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/mcpclient"
)

func TestVisualIDWorkflow(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("Integration tests not enabled. Set INTEGRATION=1 to run.")
	}

	skipIfNoGCC(t)
	skipIfNoDisplay(t)

	// 1. Setup Fixture Server
	_, cleanup := startFixtureServer(t)
	defer cleanup()
	waitForFixture(t)

	client, err := mcpclient.NewClient(mcpclient.Config{
		Timeout: 90 * time.Second,
		Env:     []string{"GHOST_MCP_KEEP_SCREENSHOTS=1", "INTEGRATION=1"},
	})
	if err != nil {
		t.Fatalf("Failed to create MCP client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	time.Sleep(1 * time.Second) // Let UI settle

	// 2. Perform Capture (Scan)
	t.Log("STEP 1: Scanning screen with learn_screen...")
	_, err = client.CallToolString(ctx, "learn_screen", map[string]interface{}{
		"max_pages": 5,
	})
	if err != nil {
		t.Fatalf("learn_screen failed: %v", err)
	}

	// 3. Load Machine-Map (get_learned_view)
	t.Log("STEP 2: Loading machine-readable map (JSON)...")
	learnedViewJSON, err := client.CallToolString(ctx, "get_learned_view", nil)
	if err != nil {
		t.Fatalf("get_learned_view failed: %v", err)
	}

	// For debugging in constrained environments
	if strings.Contains(learnedViewJSON, `"element_count":0`) || strings.Contains(learnedViewJSON, `"elements":[]`) {
		t.Logf("DEBUG: No elements found. Full JSON: %s", learnedViewJSON)
	} else {
		// Log some of the found elements to see what OCR is producing
		displayLen := 1000
		if len(learnedViewJSON) < displayLen {
			displayLen = len(learnedViewJSON)
		}
		t.Logf("DEBUG: JSON size %d. First %d chars: %s", len(learnedViewJSON), displayLen, learnedViewJSON[:displayLen])
	}

	var learnedData struct {
		Elements []struct {
			OcrID int    `json:"ocr_id"`
			Text  string `json:"text"`
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(learnedViewJSON), &learnedData); err != nil {
		t.Fatalf("Failed to parse learned view JSON: %v", err)
	}

	// Helper to find element's internal ID by text (ocr_id maps to visual_id internally)
	findID := func(text string) int {
		normalizedTarget := strings.ToLower(strings.TrimSpace(text))
		for _, e := range learnedData.Elements {
			normalizedElement := strings.ToLower(strings.TrimSpace(e.Text))
			// Exact substring check
			if strings.Contains(normalizedElement, normalizedTarget) {
				return e.OcrID
			}
			// Fuzzy check (handles OCR misreads like "PR|MARY")
			dist := levenshteinDistance(normalizedTarget, normalizedElement)
			threshold := float64(len(normalizedTarget)) * 0.4 // allow 40% distance (higher because element might have extra text)
			if float64(dist) <= threshold || strings.Contains(normalizedTarget, normalizedElement) {
				return e.OcrID
			}
		}
		return -1
	}

	// levenshteinDistance is a simple helper for fuzzy matching
	_ = levenshteinDistance // ensure it's available or use a copy if not in scope

	// 4. Interaction - Buttons (Click by visual_id)
	buttons := []struct {
		label    string
		expected string
	}{
		{"Primary", "Clicked PRIMARY"},
		{"Success", "Clicked SUCCESS"},
		{"Warning", "Clicked WARNING"},
		{"Info", "Clicked INFO"},
	}

	for _, btn := range buttons {
		id := findID(btn.label)
		if id == -1 {
			t.Errorf("Could not find ID for button %q", btn.label)
			continue
		}

		t.Logf("Clicking %q using visual_id %d...", btn.label, id)
		_, err := client.CallToolString(ctx, "click_at", map[string]interface{}{
			"visual_id": id,
		})
		if err != nil {
			t.Errorf("click_at(visual_id=%d) failed: %v", id, err)
			continue
		}

		verifyLastAction(t, ctx, client, btn.expected)
		time.Sleep(200 * time.Millisecond)
	}

	// 5. Interaction - Input (Type by visual_id)
	inputID := findID("Type here")
	if inputID != -1 {
		t.Logf("Typing into input field using visual_id %d...", inputID)
		_, err := client.CallToolString(ctx, "click_and_type", map[string]interface{}{
			"visual_id": inputID,
			"text":      "ID-based typing works!",
		})
		if err != nil {
			t.Errorf("click_and_type(visual_id=%d) failed: %v", inputID, err)
		} else {
			verifyLastAction(t, ctx, client, "Typed")
		}
	} else {
		t.Error("Could not find ID for text input")
	}

	// 6. Interaction - Hover (Hover by visual_id)
	hoverID := findID("Move mouse over this")
	if hoverID != -1 {
		t.Logf("Hovering over zone using visual_id %d...", hoverID)
		_, err := client.CallToolString(ctx, "move_mouse", map[string]interface{}{
			"visual_id": hoverID,
		})
		if err != nil {
			t.Errorf("move_mouse(visual_id=%d) failed: %v", hoverID, err)
		} else {
			time.Sleep(500 * time.Millisecond)
			verifyLastAction(t, ctx, client, "Mouse in hover zone")
		}
	} else {
		t.Error("Could not find ID for hover zone")
	}

	t.Log("✓ Comprehensive Visual ID Workflow validation complete!")
}
