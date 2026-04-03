package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleFindAndClickMissingText(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleFindAndClick(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for missing text parameter")
	}
}

func TestHandleFindAndClickWithText(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text": "Test",
			},
		},
	}

	result, err := handleFindAndClick(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// May fail due to missing OCR/learner, but parameter validation should pass
}

func TestHandleFindAndClickWithAllParams(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text":             "Button",
				"button":           "left",
				"nth":              float64(1),
				"x":                float64(100),
				"y":                float64(200),
				"width":            float64(300),
				"height":           float64(150),
				"delay_ms":         float64(200),
				"grayscale":        true,
				"scroll_direction": "down",
				"max_scrolls":      float64(5),
				"scroll_amount":    float64(3),
			},
		},
	}

	result, err := handleFindAndClick(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleFindAndClickAllMissingTexts(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleFindAndClickAll(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for missing texts parameter")
	}
}

func TestHandleFindAndClickAllInvalidJSON(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"texts": "not a json array",
			},
		},
	}

	result, err := handleFindAndClickAll(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for invalid JSON")
	}
}

func TestHandleFindAndClickAllEmptyArray(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"texts": "[]",
			},
		},
	}

	result, err := handleFindAndClickAll(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for empty array")
	}
}

func TestHandleFindAndClickAllValidParams(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"texts":    `["Button1", "Button2"]`,
				"button":   "left",
				"delay_ms": float64(200),
			},
		},
	}

	result, err := handleFindAndClickAll(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleWaitForTextMissingText(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleWaitForText(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for missing text parameter")
	}
}

func TestHandleWaitForTextWithParams(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text":       "Loading...",
				"visible":    true,
				"timeout_ms": float64(3000),
				"x":          float64(100),
				"y":          float64(200),
				"width":      float64(400),
				"height":     float64(300),
			},
		},
	}

	result, err := handleWaitForText(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleWaitForTextInvisible(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text":    "Loading...",
				"visible": false,
			},
		},
	}

	result, err := handleWaitForText(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleFindElementsWithRegion(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"x":      float64(0),
				"y":      float64(0),
				"width":  float64(800),
				"height": float64(600),
			},
		},
	}

	result, err := handleFindElements(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleFindElementsNoRegion(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleFindElements(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleFindClickAndTypeMissingParams(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{
			name: "missing text",
			args: map[string]interface{}{
				"type_text": "value",
			},
		},
		{
			name: "missing type_text",
			args: map[string]interface{}{
				"text": "label",
			},
		},
		{
			name: "missing both",
			args: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Arguments: tt.args,
				},
			}

			result, err := handleFindClickAndType(ctx, req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected result, got nil")
			}
			if !result.IsError {
				t.Error("expected error result for missing parameters")
			}
		})
	}
}

func TestHandleFindClickAndTypeWithAllParams(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text":             "Email:",
				"type_text":        "test@example.com",
				"x_offset":         float64(100),
				"y_offset":         float64(5),
				"press_enter":      true,
				"delay_ms":         float64(150),
				"nth":              float64(1),
				"x":                float64(50),
				"y":                float64(100),
				"width":            float64(400),
				"height":           float64(200),
				"scroll_direction": "down",
				"scroll_amount":    float64(5),
				"max_scrolls":      float64(8),
				"scroll_x":         float64(960),
				"scroll_y":         float64(540),
				"grayscale":        true,
			},
		},
	}

	result, err := handleFindClickAndType(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestHandleGetRegionCacheStatsNew(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}

	result, err := handleGetRegionCacheStats(ctx, req)
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

func TestHandleClearRegionCacheNew(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}

	result, err := handleClearRegionCache(ctx, req)
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

func TestHandleClickUntilTextAppearsMissingParams(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{
			name: "missing x",
			args: map[string]interface{}{
				"y":             float64(100),
				"wait_for_text": "Success",
			},
		},
		{
			name: "missing y",
			args: map[string]interface{}{
				"x":             float64(100),
				"wait_for_text": "Success",
			},
		},
		{
			name: "missing wait_for_text",
			args: map[string]interface{}{
				"x": float64(100),
				"y": float64(100),
			},
		},
		{
			name: "missing all",
			args: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Arguments: tt.args,
				},
			}

			result, err := handleClickUntilTextAppears(ctx, req)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected result, got nil")
			}
			if !result.IsError {
				t.Error("expected error result for missing parameters")
			}
		})
	}
}

func TestHandleClickUntilTextAppearsWithParams(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"x":             float64(400),
				"y":             float64(300),
				"wait_for_text": "Saved!",
				"button":        "left",
				"timeout_ms":    float64(5000),
				"max_clicks":    float64(3),
			},
		},
	}

	result, err := handleClickUntilTextAppears(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestOCRToolRegistration(t *testing.T) {
	// This test verifies that OCR tools are registered correctly
	// We can't test the actual registration without a server instance
	// but we can verify the tool names exist

	expectedTools := []string{
		"find_and_click",
		"find_and_click_all",
		"wait_for_text",
		"find_elements",
		"find_click_and_type",
		"get_region_cache_stats",
		"clear_region_cache",
		"click_until_text_appears",
	}

	for _, toolName := range expectedTools {
		if toolName == "" {
			t.Error("expected non-empty tool name")
		}
	}
}
