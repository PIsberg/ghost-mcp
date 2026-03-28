// ghost-mcp: Tests for the MCP server tools
//
// These tests verify the parameter extraction and tool handling logic.
// Note: Actual UI automation tests require a display and are platform-dependent,
// so we focus on testing the request/response handling logic.
package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// =============================================================================
// PARAMETER EXTRACTION TESTS
// =============================================================================

// TestGetStringParamMissing tests that missing string parameters return an error
func TestGetStringParamMissing(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	_, err := getStringParam(request, "missing_param")
	if err == nil {
		t.Error("Expected error for missing parameter, got nil")
	}

	expectedMsg := "missing required parameter: missing_param"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestGetStringParamInvalidType tests that non-string parameters return an error
func TestGetStringParamInvalidType(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"test_param": 123, // Integer instead of string
			},
		},
	}

	_, err := getStringParam(request, "test_param")
	if err == nil {
		t.Error("Expected error for invalid type, got nil")
	}
}

// TestGetStringParamValid tests that valid string parameters are extracted correctly
func TestGetStringParamValid(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"test_param": "hello world",
			},
		},
	}

	val, err := getStringParam(request, "test_param")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if val != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", val)
	}
}

// TestGetIntParamMissing tests that missing integer parameters return an error
func TestGetIntParamMissing(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	_, err := getIntParam(request, "missing_param")
	if err == nil {
		t.Error("Expected error for missing parameter, got nil")
	}
}

// TestGetIntParamFloat64 tests that float64 values (JSON numbers) are converted to int
func TestGetIntParamFloat64(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"test_param": float64(42),
			},
		},
	}

	val, err := getIntParam(request, "test_param")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if val != 42 {
		t.Errorf("Expected 42, got %d", val)
	}
}

// TestGetIntParamInt64 tests that int64 values are converted to int
func TestGetIntParamInt64(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"test_param": int64(100),
			},
		},
	}

	val, err := getIntParam(request, "test_param")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if val != 100 {
		t.Errorf("Expected 100, got %d", val)
	}
}

// TestGetIntParamInvalidType tests that invalid types return an error
func TestGetIntParamInvalidType(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"test_param": "not a number",
			},
		},
	}

	_, err := getIntParam(request, "test_param")
	if err == nil {
		t.Error("Expected error for invalid type, got nil")
	}
}

// =============================================================================
// TOOL HANDLER TESTS
// =============================================================================

// TestHandleGetScreenSize tests the get_screen_size tool handler
func TestHandleGetScreenSize(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	ctx := context.Background()
	result, err := handleGetScreenSize(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Parse the JSON response
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}

	// Verify response contains width and height
	if _, ok := response["width"]; !ok {
		t.Error("Response missing 'width' field")
	}
	if _, ok := response["height"]; !ok {
		t.Error("Response missing 'height' field")
	}
}

// TestHandleMoveMouseValid tests the move_mouse tool with valid parameters
func TestHandleMoveMouseValid(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"x": float64(100),
				"y": float64(200),
			},
		},
	}

	ctx := context.Background()
	result, err := handleMoveMouse(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Parse the JSON response
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}

	// Verify response
	if success, ok := response["success"].(bool); !ok || !success {
		t.Error("Expected success to be true")
	}
}

// TestHandleMoveMouseMissingX tests move_mouse with missing x parameter
func TestHandleMoveMouseMissingX(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"y": float64(200),
			},
		},
	}

	ctx := context.Background()
	result, err := handleMoveMouse(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Should return an error result
	if !result.IsError {
		t.Error("Expected error result for missing x parameter")
	}
}

// TestHandleMoveMouseMissingY tests move_mouse with missing y parameter
func TestHandleMoveMouseMissingY(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"x": float64(100),
			},
		},
	}

	ctx := context.Background()
	result, err := handleMoveMouse(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for missing y parameter")
	}
}

// TestHandleClickValid tests the click tool with valid parameters
func TestHandleClickValid(t *testing.T) {
	testCases := []struct {
		button string
	}{
		{"left"},
		{"right"},
		{"middle"},
	}

	for _, tc := range testCases {
		t.Run(tc.button, func(t *testing.T) {
			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Arguments: map[string]interface{}{
						"button": tc.button,
					},
				},
			}

			ctx := context.Background()
			result, err := handleClick(ctx, request)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			// Parse the JSON response
			var response map[string]interface{}
			if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
				t.Fatalf("Failed to parse result JSON: %v", err)
			}

			if success, ok := response["success"].(bool); !ok || !success {
				t.Error("Expected success to be true")
			}
		})
	}
}

// TestHandleClickInvalidButton tests click with an invalid button value
func TestHandleClickInvalidButton(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"button": "invalid_button",
			},
		},
	}

	ctx := context.Background()
	result, err := handleClick(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for invalid button")
	}
}

// TestHandleClickMissingButton tests click with missing button parameter
func TestHandleClickMissingButton(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	ctx := context.Background()
	result, err := handleClick(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for missing button")
	}
}

// TestHandleTypeTextValid tests the type_text tool with valid parameters
func TestHandleTypeTextValid(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text": "Hello, World!",
			},
		},
	}

	ctx := context.Background()
	result, err := handleTypeText(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Parse the JSON response
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || !success {
		t.Error("Expected success to be true")
	}

	charCount, ok := response["characters_typed"].(float64)
	if !ok || int(charCount) != 13 {
		t.Errorf("Expected 13 characters typed, got %v", charCount)
	}
}

// TestHandleTypeTextMissingText tests type_text with missing text parameter
func TestHandleTypeTextMissingText(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	ctx := context.Background()
	result, err := handleTypeText(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for missing text")
	}
}

// TestHandlePressKeyValid tests the press_key tool with valid parameters
func TestHandlePressKeyValid(t *testing.T) {
	testCases := []string{"enter", "tab", "esc", "ctrl", "alt", "shift"}

	for _, key := range testCases {
		t.Run(key, func(t *testing.T) {
			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Arguments: map[string]interface{}{
						"key": key,
					},
				},
			}

			ctx := context.Background()
			result, err := handlePressKey(ctx, request)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			// Parse the JSON response
			var response map[string]interface{}
			if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
				t.Fatalf("Failed to parse result JSON: %v", err)
			}

			if success, ok := response["success"].(bool); !ok || !success {
				t.Error("Expected success to be true")
			}
		})
	}
}

// TestHandlePressKeyMissingKey tests press_key with missing key parameter
func TestHandlePressKeyMissingKey(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	ctx := context.Background()
	result, err := handlePressKey(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for missing key")
	}
}

// TestHandleTakeScreenshotValid tests the take_screenshot tool
func TestHandleTakeScreenshotValid(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{},
		},
	}

	ctx := context.Background()
	result, err := handleTakeScreenshot(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Parse the JSON response
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}

	// Verify response contains expected fields
	if success, ok := response["success"].(bool); !ok || !success {
		t.Error("Expected success to be true")
	}

	if _, ok := response["filepath"]; !ok {
		t.Error("Response missing 'filepath' field")
	}

	if _, ok := response["base64"]; !ok {
		t.Error("Response missing 'base64' field")
	}
}

// TestHandleTakeScreenshotWithRegion tests take_screenshot with custom region
func TestHandleTakeScreenshotWithRegion(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"x":      float64(0),
				"y":      float64(0),
				"width":  float64(100),
				"height": float64(100),
			},
		},
	}

	ctx := context.Background()
	result, err := handleTakeScreenshot(ctx, request)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Parse the JSON response
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || !success {
		t.Error("Expected success to be true")
	}
}

// =============================================================================
// FAILSAFE TESTS
// =============================================================================

// TestInitiateShutdown tests that shutdown can be initiated
func TestInitiateShutdown(t *testing.T) {
	// Reset state
	state = &serverState{
		shutdownChan: make(chan struct{}),
	}

	initiateShutdown()

	if !state.isShuttingDown {
		t.Error("Expected isShuttingDown to be true after initiateShutdown")
	}

	// Verify channel is closed
	select {
	case _, ok := <-state.shutdownChan:
		if ok {
			t.Error("Expected shutdownChan to be closed")
		}
	default:
		t.Error("Expected shutdownChan to be closed")
	}
}

// TestInitiateShutdownIdempotent tests that multiple shutdown calls are safe
func TestInitiateShutdownIdempotent(t *testing.T) {
	// Reset state
	state = &serverState{
		shutdownChan: make(chan struct{}),
	}

	// Call shutdown multiple times
	initiateShutdown()
	initiateShutdown()
	initiateShutdown()

	// Should still be in shutdown state (no panic)
	if !state.isShuttingDown {
		t.Error("Expected isShuttingDown to be true")
	}
}

// =============================================================================
// LOGGING TESTS
// =============================================================================

// TestLoggingFunctions tests that logging functions don't panic
func TestLoggingFunctions(t *testing.T) {
	// These should not panic
	logInfo("Test info message")
	logError("Test error message")
	logDebug("Test debug message")

	// Enable debug mode and test again
	t.Setenv("GHOST_MCP_DEBUG", "1")
	logDebug("Test debug message with debug enabled")
}

// =============================================================================
// TOKEN AUTHENTICATION TESTS
// =============================================================================

// TestValidateStartupToken_Missing tests that missing token returns error
func TestValidateStartupToken_Missing(t *testing.T) {
	t.Setenv(TokenEnvVar, "")
	_, err := validateStartupToken()
	if err == nil {
		t.Error("Expected error when token is not set, got nil")
	}
}

// TestValidateStartupToken_Present tests that a set token is returned correctly
func TestValidateStartupToken_Present(t *testing.T) {
	t.Setenv(TokenEnvVar, "my-secret-token")
	token, err := validateStartupToken()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if token != "my-secret-token" {
		t.Errorf("Expected 'my-secret-token', got '%s'", token)
	}
}

// TestMakeTokenValidator_ValidToken tests that a matching token passes validation
func TestMakeTokenValidator_ValidToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "valid-token")
	validator := makeTokenValidator("valid-token")
	if err := validator(context.Background(), 1, nil); err != nil {
		t.Errorf("Expected valid token to pass, got error: %v", err)
	}
}

// TestMakeTokenValidator_WrongToken tests that a mismatched token is rejected
func TestMakeTokenValidator_WrongToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "wrong-token")
	validator := makeTokenValidator("expected-token")
	if err := validator(context.Background(), 1, nil); err == nil {
		t.Error("Expected error for mismatched token, got nil")
	}
}

// TestMakeTokenValidator_MissingToken tests that a cleared token is rejected
func TestMakeTokenValidator_MissingToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "")
	validator := makeTokenValidator("expected-token")
	if err := validator(context.Background(), 1, nil); err == nil {
		t.Error("Expected error for missing token, got nil")
	}
}

// TestCreateServer_WithToken tests that createServer succeeds with a valid token
func TestCreateServer_WithToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "test-token")
	t.Setenv(AuditEnvVar, t.TempDir())
	al, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer al.Close()
	srv := createServer("test-token", al)
	if srv == nil {
		t.Fatal("Expected non-nil server")
	}
}
