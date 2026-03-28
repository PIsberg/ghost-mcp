// ghost-mcp: Tests for the MCP server tools
//
// These tests verify the parameter extraction and tool handling logic.
// Note: Actual UI automation tests require a display and are platform-dependent,
// so we focus on testing the request/response handling logic.
package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ghost-mcp/internal/audit"
	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/validate"
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

	// Content[0] is the JSON metadata, Content[1] is the PNG image
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response); err != nil {
		t.Fatalf("Failed to parse result JSON: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || !success {
		t.Error("Expected success to be true")
	}

	if _, ok := response["filepath"]; !ok {
		t.Error("Response missing 'filepath' field")
	}

	// Image data is in Content[1] as ImageContent
	if len(result.Content) < 2 {
		t.Error("Expected at least 2 content items (metadata + image)")
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
	logging.Info("Test info message")
	logging.Error("Test error message")
	logging.Debug("Test debug message")

	// Enable debug mode and test again
	t.Setenv("GHOST_MCP_DEBUG", "1")
	logging.Debug("Test debug message with debug enabled")
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
	t.Setenv(audit.EnvVar, t.TempDir())
	al, _ := audit.New()
	defer al.Close()
	validator := makeTokenValidator("valid-token", al)
	if err := validator(context.Background(), 1, nil); err != nil {
		t.Errorf("Expected valid token to pass, got error: %v", err)
	}
}

// TestMakeTokenValidator_WrongToken tests that a mismatched token is rejected
func TestMakeTokenValidator_WrongToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "wrong-token")
	t.Setenv(audit.EnvVar, t.TempDir())
	al, _ := audit.New()
	defer al.Close()
	validator := makeTokenValidator("expected-token", al)
	if err := validator(context.Background(), 1, nil); err == nil {
		t.Error("Expected error for mismatched token, got nil")
	}
}

// TestMakeTokenValidator_MissingToken tests that a cleared token is rejected
func TestMakeTokenValidator_MissingToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "")
	t.Setenv(audit.EnvVar, t.TempDir())
	al, _ := audit.New()
	defer al.Close()
	validator := makeTokenValidator("expected-token", al)
	if err := validator(context.Background(), 1, nil); err == nil {
		t.Error("Expected error for missing token, got nil")
	}
}

// TestCreateServer_WithToken tests that createServer succeeds with a valid token
func TestCreateServer_WithToken(t *testing.T) {
	t.Setenv(TokenEnvVar, "test-token")
	t.Setenv(audit.EnvVar, t.TempDir())
	al, err := audit.New()
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer al.Close()
	srv := createServer("test-token", al)
	if srv == nil {
		t.Fatal("Expected non-nil server")
	}
}

// =============================================================================
// GETINTPARAM — whole-number enforcement
// =============================================================================

func TestGetIntParam_WholeFloat(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"n": float64(42)}}}
	v, err := getIntParam(req, "n")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if v != 42 {
		t.Errorf("Expected 42, got %d", v)
	}
}

func TestGetIntParam_FractionalFloat_Rejected(t *testing.T) {
	for _, f := range []float64{1.5, 0.1, 42.9, -1.1, 100.001} {
		req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"n": f}}}
		if _, err := getIntParam(req, "n"); err == nil {
			t.Errorf("getIntParam with %v: expected error for fractional float", f)
		}
	}
}

func TestGetIntParam_NegativeWholeFloat(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"n": float64(-5)}}}
	v, err := getIntParam(req, "n")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if v != -5 {
		t.Errorf("Expected -5, got %d", v)
	}
}

func TestGetIntParam_ZeroFloat(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"n": float64(0)}}}
	v, err := getIntParam(req, "n")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if v != 0 {
		t.Errorf("Expected 0, got %d", v)
	}
}

// =============================================================================
// HANDLER VALIDATION — no CGo; only tests the validation branch
// =============================================================================

func TestHandleTypeText_EmptyText(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"text": ""}}}
	result, err := handleTypeText(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error result for empty text")
	}
}

func TestHandleTypeText_TooLong(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]interface{}{"text": strings.Repeat("a", validate.MaxTextLength+1)},
	}}
	result, err := handleTypeText(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error result for oversized text")
	}
}

func TestHandlePressKey_UnknownKey(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"key": "not_a_real_key"}}}
	result, err := handlePressKey(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error result for unknown key")
	}
}

func TestHandlePressKey_InjectionAttempt(t *testing.T) {
	for _, key := range []string{"; rm -rf /", "$(whoami)", "../../../etc/passwd", "ctrl+alt+del"} {
		req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{"key": key}}}
		result, err := handlePressKey(nil, req)
		if err != nil {
			t.Fatalf("Handler returned unexpected Go error for %q: %v", key, err)
		}
		if !result.IsError {
			t.Errorf("Expected tool error for injection attempt %q", key)
		}
	}
}

func TestHandleMoveMouse_FractionalCoords(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{
		Arguments: map[string]interface{}{"x": 100.5, "y": float64(200)},
	}}
	result, err := handleMoveMouse(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for fractional x coordinate")
	}
}

// =============================================================================
// CLICK_AT TESTS
// =============================================================================

// TestHandleClickAt_InvalidButton tests click_at with an invalid button value
func TestHandleClickAt_InvalidButton(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(100), "y": float64(200), "button": "invalid",
	}}}
	result, err := handleClickAt(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for invalid button")
	}
}

// TestHandleClickAt_MissingX tests click_at with missing x parameter
func TestHandleClickAt_MissingX(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"y": float64(200),
	}}}
	result, err := handleClickAt(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for missing x parameter")
	}
}

// TestHandleClickAt_MissingY tests click_at with missing y parameter
func TestHandleClickAt_MissingY(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(100),
	}}}
	result, err := handleClickAt(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for missing y parameter")
	}
}

// TestHandleClickAt_FractionalCoords tests click_at rejects fractional coordinates
func TestHandleClickAt_FractionalCoords(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": 100.7, "y": float64(200),
	}}}
	result, err := handleClickAt(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for fractional x coordinate")
	}
}

// TestHandleClickAt_NegativeCoords tests click_at rejects out-of-bounds coordinates
func TestHandleClickAt_NegativeCoords(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(-1), "y": float64(200),
	}}}
	result, err := handleClickAt(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for negative x coordinate")
	}
}

// =============================================================================
// DOUBLE_CLICK TESTS
// =============================================================================

// TestHandleDoubleClick_MissingX tests double_click with missing x parameter
func TestHandleDoubleClick_MissingX(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"y": float64(200),
	}}}
	result, err := handleDoubleClick(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for missing x parameter")
	}
}

// TestHandleDoubleClick_MissingY tests double_click with missing y parameter
func TestHandleDoubleClick_MissingY(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(100),
	}}}
	result, err := handleDoubleClick(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for missing y parameter")
	}
}

// TestHandleDoubleClick_NegativeCoords tests double_click rejects negative coordinates
func TestHandleDoubleClick_NegativeCoords(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(-1), "y": float64(200),
	}}}
	result, err := handleDoubleClick(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for negative x coordinate")
	}
}

// TestHandleDoubleClick_FractionalCoords tests double_click rejects fractional coordinates
func TestHandleDoubleClick_FractionalCoords(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": 100.5, "y": float64(200),
	}}}
	result, err := handleDoubleClick(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for fractional x coordinate")
	}
}

// =============================================================================
// SCROLL TESTS
// =============================================================================

// TestHandleScroll_InvalidDirection tests scroll with an invalid direction
func TestHandleScroll_InvalidDirection(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(100), "y": float64(200), "direction": "sideways",
	}}}
	result, err := handleScroll(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for invalid direction")
	}
}

// TestHandleScroll_MissingDirection tests scroll with missing direction
func TestHandleScroll_MissingDirection(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(100), "y": float64(200),
	}}}
	result, err := handleScroll(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for missing direction")
	}
}

// TestHandleScroll_MissingX tests scroll with missing x parameter
func TestHandleScroll_MissingX(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"y": float64(200), "direction": "down",
	}}}
	result, err := handleScroll(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for missing x parameter")
	}
}

// TestHandleScroll_ZeroAmount tests scroll rejects zero amount
func TestHandleScroll_ZeroAmount(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(100), "y": float64(200), "direction": "down", "amount": float64(0),
	}}}
	result, err := handleScroll(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for zero amount")
	}
}

// TestHandleScroll_NegativeCoords tests scroll rejects out-of-bounds coordinates
func TestHandleScroll_NegativeCoords(t *testing.T) {
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]interface{}{
		"x": float64(-1), "y": float64(200), "direction": "down",
	}}}
	result, err := handleScroll(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for negative x coordinate")
	}
}
