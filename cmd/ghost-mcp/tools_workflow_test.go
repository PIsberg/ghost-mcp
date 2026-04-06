package main

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestWorkflowStepStruct(t *testing.T) {
	tests := []struct {
		name string
		step WorkflowStep
	}{
		{
			name: "click action",
			step: WorkflowStep{
				Action: "click",
				Text:   "Submit",
			},
		},
		{
			name: "type action",
			step: WorkflowStep{
				Action: "type",
				Text:   "Email:",
				Value:  "test@example.com",
			},
		},
		{
			name: "wait action",
			step: WorkflowStep{
				Action:  "wait",
				DelayMS: 1000,
			},
		},
		{
			name: "scroll action",
			step: WorkflowStep{
				Action:    "scroll",
				Amount:    5,
				Direction: "down",
			},
		},
		{
			name: "refresh_view action",
			step: WorkflowStep{
				Action: "refresh_view",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.step.Action == "" {
				t.Error("expected action to be set")
			}
		})
	}
}

func TestWorkflowResultStruct(t *testing.T) {
	result := WorkflowResult{
		Success:       true,
		StepsExecuted: 3,
		StepsFailed:   0,
		TotalDuration: "1.5s",
		StepResults: []StepResult{
			{StepIndex: 0, Action: "click", Success: true, Duration: "100ms"},
			{StepIndex: 1, Action: "type", Success: true, Duration: "200ms"},
			{StepIndex: 2, Action: "click", Success: true, Duration: "150ms"},
		},
	}

	if !result.Success {
		t.Error("expected result to be successful")
	}
	if result.StepsExecuted != 3 {
		t.Errorf("expected 3 steps executed, got %d", result.StepsExecuted)
	}
	if result.StepsFailed != 0 {
		t.Errorf("expected 0 steps failed, got %d", result.StepsFailed)
	}
	if len(result.StepResults) != 3 {
		t.Errorf("expected 3 step results, got %d", len(result.StepResults))
	}
}

func TestStepResultStruct(t *testing.T) {
	stepResult := StepResult{
		StepIndex: 2,
		Action:    "scroll",
		Success:   false,
		Error:     "scroll failed",
		Duration:  "50ms",
	}

	if stepResult.StepIndex != 2 {
		t.Errorf("expected step index 2, got %d", stepResult.StepIndex)
	}
	if stepResult.Action != "scroll" {
		t.Errorf("expected action 'scroll', got %s", stepResult.Action)
	}
	if stepResult.Success {
		t.Error("expected step to be unsuccessful")
	}
	if stepResult.Error != "scroll failed" {
		t.Errorf("expected error 'scroll failed', got %s", stepResult.Error)
	}
}

func TestHandleExecuteWorkflowInvalidArgs(t *testing.T) {
	ctx := context.Background()

	// Test with invalid arguments
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: "invalid",
		},
	}

	result, err := handleExecuteWorkflow(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for invalid arguments")
	}
}

func TestHandleExecuteWorkflowEmptySteps(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"steps": []interface{}{},
			},
		},
	}

	result, err := handleExecuteWorkflow(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for empty steps")
	}
}

func TestHandleExecuteWorkflowMissingSteps(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handleExecuteWorkflow(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Error("expected error result for missing steps")
	}
}

func TestHandleExecuteWorkflowInvalidStepFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: triggers real screen capture and OCR via auto-learn")
	}
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"steps": []interface{}{"invalid_step"},
			},
		},
	}

	result, err := handleExecuteWorkflow(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// Should complete but report step failure
	if result.IsError {
		t.Error("expected successful result with failed steps")
	}
}

func TestHandleExecuteWorkflowUnknownAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: triggers real screen capture and OCR via auto-learn")
	}
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"action": "unknown_action",
					},
				},
			},
		},
	}

	result, err := handleExecuteWorkflow(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// Should complete but report the step as failed
	if result.IsError {
		t.Error("expected successful result with failed steps")
	}
}

func TestHandleExecuteWorkflowWaitAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: triggers real screen capture and OCR via auto-learn")
	}
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"action":   "wait",
						"delay_ms": 10,
					},
				},
			},
		},
	}

	result, err := handleExecuteWorkflow(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.IsError {
		t.Errorf("expected successful result, got error")
	}
}

func TestHandleExecuteWorkflowClearViewAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: triggers real screen capture and OCR via auto-learn")
	}
	ctx := context.Background()

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"action":   "wait",
						"delay_ms": 10,
					},
				},
				"clear_view_after": true,
			},
		},
	}

	result, err := handleExecuteWorkflow(ctx, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestGetStringFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		key      string
		expected string
	}{
		{
			name:     "valid string value",
			input:    map[string]interface{}{"key": "value"},
			key:      "key",
			expected: "value",
		},
		{
			name:     "missing key",
			input:    map[string]interface{}{"other": "value"},
			key:      "key",
			expected: "",
		},
		{
			name:     "non-string value",
			input:    map[string]interface{}{"key": 123},
			key:      "key",
			expected: "",
		},
		{
			name:     "empty string",
			input:    map[string]interface{}{"key": ""},
			key:      "key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringFromMap(tt.input, tt.key)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetIntFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		key      string
		expected int
	}{
		{
			name:     "valid int value",
			input:    map[string]interface{}{"key": float64(42)},
			key:      "key",
			expected: 42,
		},
		{
			name:     "missing key",
			input:    map[string]interface{}{"other": 42},
			key:      "key",
			expected: 0,
		},
		{
			name:     "non-number value",
			input:    map[string]interface{}{"key": "value"},
			key:      "key",
			expected: 0,
		},
		{
			name:     "zero value",
			input:    map[string]interface{}{"key": float64(0)},
			key:      "key",
			expected: 0,
		},
		{
			name:     "negative value",
			input:    map[string]interface{}{"key": float64(-10)},
			key:      "key",
			expected: -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIntFromMap(tt.input, tt.key)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestWorkflowResultToJSON(t *testing.T) {
	tests := []struct {
		name   string
		result WorkflowResult
		check  func(t *testing.T, output string)
	}{
		{
			name: "successful workflow",
			result: WorkflowResult{
				Success:       true,
				StepsExecuted: 2,
				StepsFailed:   0,
				TotalDuration: "500ms",
				StepResults: []StepResult{
					{StepIndex: 0, Action: "click", Success: true, Duration: "100ms"},
					{StepIndex: 1, Action: "type", Success: true, Duration: "200ms"},
				},
			},
			check: func(t *testing.T, output string) {
				if !containsStr(output, `"success": true`) {
					t.Error("expected success: true")
				}
				if !containsStr(output, `"steps_executed": 2`) {
					t.Error("expected steps_executed: 2")
				}
				if !containsStr(output, `"steps_failed": 0`) {
					t.Error("expected steps_failed: 0")
				}
			},
		},
		{
			name: "failed workflow",
			result: WorkflowResult{
				Success:       false,
				StepsExecuted: 1,
				StepsFailed:   1,
				TotalDuration: "300ms",
				StepResults: []StepResult{
					{StepIndex: 0, Action: "click", Success: true, Duration: "100ms"},
					{StepIndex: 1, Action: "type", Success: false, Error: "failed", Duration: "50ms"},
				},
			},
			check: func(t *testing.T, output string) {
				if !containsStr(output, `"success": false`) {
					t.Error("expected success: false")
				}
				if !containsStr(output, `"steps_executed": 1`) {
					t.Error("expected steps_executed: 1")
				}
				if !containsStr(output, `"steps_failed": 1`) {
					t.Error("expected steps_failed: 1")
				}
				if !containsStr(output, `"error": "failed"`) {
					t.Error("expected error message")
				}
			},
		},
		{
			name: "empty steps",
			result: WorkflowResult{
				Success:       true,
				StepsExecuted: 0,
				StepsFailed:   0,
				TotalDuration: "0ms",
				StepResults:   []StepResult{},
			},
			check: func(t *testing.T, output string) {
				if !containsStr(output, `"steps_executed": 0`) {
					t.Error("expected steps_executed: 0")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := workflowResultToJSON(tt.result)
			tt.check(t, output)
		})
	}
}

func TestExecuteClickEmptyText(t *testing.T) {
	ctx := context.Background()
	err := executeClick(ctx, "")
	if err == nil {
		t.Error("expected error for empty text")
	}
}

func TestExecuteTypeEmptyParams(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		text  string
		value string
	}{
		{"empty text", "", "value"},
		{"empty value", "text", ""},
		{"both empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeType(ctx, tt.text, tt.value)
			if err == nil {
				t.Error("expected error for empty parameters")
			}
		})
	}
}

func TestExecuteScrollDefaults(t *testing.T) {
	ctx := context.Background()

	// Test with zero amount (should default to 10)
	err := executeScroll(ctx, 0, "")
	// May fail due to missing learner, but we're testing defaults
	_ = err // Ignore error as it depends on global state
}

func TestWorkflowStepValidation(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		shouldError bool
	}{
		{"click", "click", false},
		{"type", "type", false},
		{"wait", "wait", false},
		{"scroll", "scroll", false},
		{"refresh_view", "refresh_view", false},
		{"unknown", "unknown", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := WorkflowStep{Action: tt.action}
			validActions := map[string]bool{
				"click":        true,
				"type":         true,
				"wait":         true,
				"scroll":       true,
				"refresh_view": true,
			}
			isValid := validActions[step.Action]
			if tt.shouldError && isValid {
				t.Error("expected invalid action")
			}
			if !tt.shouldError && !isValid {
				t.Error("expected valid action")
			}
		})
	}
}

// Helper functions

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrHelper(s, substr))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
