package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ghost-mcp/internal/learner"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleGetAnnotatedView_NoView(t *testing.T) {
	// Reset the global learner
	globalLearner.ClearView()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleGetAnnotatedView(context.TODO(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return a text message saying no view learned
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	if !contains(tc.Text, "No view has been learned yet") {
		t.Errorf("expected error message in text, got %q", tc.Text)
	}
}

func TestHandleGetAnnotatedView_WithView(t *testing.T) {
	// Mock robotgo might fail in CI, so we might need to skip if it fails actually capturing.
	// But let's at least test the logic up to the capture point if possible.

	// We'll skip this if we can't get screen size (indicates no display)
	// _, _ = robotgo.GetScreenSize() // This might panic or return 0,0

	globalLearner.SetView(&learner.View{
		Elements: []learner.Element{
			{ID: 1, Text: "OK", X: 10, Y: 10, Width: 50, Height: 20},
		},
		CapturedAt: time.Now(),
		ScreenW:    1920,
		ScreenH:    1080,
	})
	defer globalLearner.ClearView()

	// We skip the actual tool call because of robotgo dependency in CI
	t.Skip("Skipping integration test for handleGetAnnotatedView due to robotgo dependency")
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
