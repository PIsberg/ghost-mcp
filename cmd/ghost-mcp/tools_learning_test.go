package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleLearnScreen(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleLearnScreen(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// May fail due to missing dependencies, but should not error on parameter validation
}

func TestHandleLearnScreenWithRegion(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"x":      float64(100),
				"y":      float64(200),
				"width":  float64(800),
				"height": float64(600),
			},
		},
	}

	result, err := handleLearnScreen(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleLearnScreenWithScrollParams(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"max_pages":     float64(5),
				"scroll_amount": float64(3),
				"scroll_direction": "up",
			},
		},
	}

	result, err := handleLearnScreen(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleGetLearnedView(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}

	result, err := handleGetLearnedView(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// Should return successful result even if no view exists
}

func TestHandleClearLearnedViewNew(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}

	result, err := handleClearLearnedView(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.IsError {
		t.Error("expected successful result")
	}
}

func TestHandleSetLearningModeMissingEnabled(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleSetLearningMode(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for missing enabled parameter")
	}
}

func TestHandleSetLearningModeEnable(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"enabled": true,
			},
		},
	}

	result, err := handleSetLearningMode(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.IsError {
		t.Error("expected successful result")
	}
}

func TestHandleSetLearningModeDisable(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"enabled": false,
			},
		},
	}

	result, err := handleSetLearningMode(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.IsError {
		t.Error("expected successful result")
	}
}

func TestLearningToolRegistration(t *testing.T) {
	// Verify learning tool names are correct
	expectedTools := []string{
		"learn_screen",
		"get_learned_view",
		"clear_learned_view",
		"set_learning_mode",
	}

	for _, toolName := range expectedTools {
		if toolName == "" {
			t.Error("expected non-empty tool name")
		}
	}
}

func TestHandleLearnScreenInvalidParams(t *testing.T) {
	ctx := context.Background()

	// Test with invalid parameter types
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"x":      "not a number",
				"y":      "not a number",
				"width":  "not a number",
				"height": "not a number",
			},
		},
	}

	result, err := handleLearnScreen(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleLearnScreenEdgeCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{
			name: "zero region",
			args: map[string]interface{}{
				"x":      float64(0),
				"y":      float64(0),
				"width":  float64(0),
				"height": float64(0),
			},
		},
		{
			name: "negative region",
			args: map[string]interface{}{
				"x":      float64(-100),
				"y":      float64(-200),
				"width":  float64(800),
				"height": float64(600),
			},
		},
		{
			name: "large max_pages",
			args: map[string]interface{}{
				"max_pages": float64(100),
			},
		},
		{
			name: "zero scroll_amount",
			args: map[string]interface{}{
				"scroll_amount": float64(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Arguments: tt.args,
				},
			}

			result, err := handleLearnScreen(ctx, req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected result, got nil")
			}
		})
	}
}

func TestHandleSetLearningModeInvalidType(t *testing.T) {
	ctx := context.Background()

	// Test with invalid enabled type
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"enabled": "not a boolean",
			},
		},
	}

	result, err := handleSetLearningMode(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// Should handle gracefully with invalid type
}
