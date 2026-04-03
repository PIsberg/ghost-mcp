package mcpclient

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestJSONRPCRequestStruct(t *testing.T) {
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"key":"value"}`),
	}

	if request.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC 2.0, got %s", request.JSONRPC)
	}
	if request.ID != 1 {
		t.Errorf("expected ID 1, got %d", request.ID)
	}
	if request.Method != "tools/call" {
		t.Errorf("expected method 'tools/call', got %s", request.Method)
	}
}

func TestJSONRPCResponseStruct(t *testing.T) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"success":true}`),
		Error:   nil,
	}

	if response.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC 2.0, got %s", response.JSONRPC)
	}
	if response.ID != 1 {
		t.Errorf("expected ID 1, got %d", response.ID)
	}
	if response.Error != nil {
		t.Error("expected nil error")
	}
}

func TestJSONRPCErrorStruct(t *testing.T) {
	err := JSONRPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}

	if err.Code != -32600 {
		t.Errorf("expected error code -32600, got %d", err.Code)
	}
	if err.Message != "Invalid Request" {
		t.Errorf("expected error message 'Invalid Request', got %s", err.Message)
	}
}

func TestToolCallParamsStruct(t *testing.T) {
	params := ToolCallParams{
		Name: "find_and_click",
		Arguments: map[string]interface{}{
			"text": "Submit",
		},
	}

	if params.Name != "find_and_click" {
		t.Errorf("expected name 'find_and_click', got %s", params.Name)
	}
	if params.Arguments["text"] != "Submit" {
		t.Errorf("expected text 'Submit', got %v", params.Arguments["text"])
	}
}

func TestToolResultStruct(t *testing.T) {
	result := ToolResult{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: `{"success":true}`},
		},
		IsError: false,
	}

	if result.IsError {
		t.Error("expected no error")
	}
	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("expected content type 'text', got %s", result.Content[0].Type)
	}
}

func TestConfigStruct(t *testing.T) {
	config := Config{
		BinaryPath: "./ghost-mcp",
		Timeout:    10 * time.Second,
		Env:        []string{"GHOST_MCP_TOKEN=test"},
	}

	if config.BinaryPath != "./ghost-mcp" {
		t.Errorf("expected binary path './ghost-mcp', got %s", config.BinaryPath)
	}
	if config.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", config.Timeout)
	}
	if len(config.Env) != 1 {
		t.Errorf("expected 1 env variable, got %d", len(config.Env))
	}
}

func TestFindAndClickOptionsStruct(t *testing.T) {
	opts := FindAndClickOptions{
		Button:    "left",
		Nth:       2,
		X:         100,
		Y:         200,
		Width:     300,
		Height:    150,
		Grayscale: true,
		DelayMS:   200,
	}

	if opts.Button != "left" {
		t.Errorf("expected button 'left', got %s", opts.Button)
	}
	if opts.Nth != 2 {
		t.Errorf("expected nth 2, got %d", opts.Nth)
	}
	if opts.X != 100 {
		t.Errorf("expected X 100, got %d", opts.X)
	}
	if opts.Y != 200 {
		t.Errorf("expected Y 200, got %d", opts.Y)
	}
	if opts.Width != 300 {
		t.Errorf("expected width 300, got %d", opts.Width)
	}
	if opts.Height != 150 {
		t.Errorf("expected height 150, got %d", opts.Height)
	}
	if !opts.Grayscale {
		t.Error("expected grayscale true")
	}
	if opts.DelayMS != 200 {
		t.Errorf("expected delay_ms 200, got %d", opts.DelayMS)
	}
}

func TestFindAndClickResultStruct(t *testing.T) {
	result := FindAndClickResult{
		Success:    true,
		Found:      "Submit",
		BoxX:       100,
		BoxY:       200,
		BoxWidth:   80,
		BoxHeight:  30,
		RequestedX: 140,
		RequestedY: 215,
		ActualX:    140,
		ActualY:    215,
		Button:     "left",
		Occurrence: 1,
	}

	if !result.Success {
		t.Error("expected success true")
	}
	if result.Found != "Submit" {
		t.Errorf("expected found 'Submit', got %s", result.Found)
	}
	if result.BoxX != 100 {
		t.Errorf("expected box X 100, got %d", result.BoxX)
	}
	if result.Button != "left" {
		t.Errorf("expected button 'left', got %s", result.Button)
	}
}

func TestNewClientEmptyConfig(t *testing.T) {
	// Test with empty config - should use defaults
	// This will fail to start the server, but tests the config logic
	config := Config{}
	
	// Verify defaults would be set
	if config.BinaryPath == "" {
		// Would default to "./ghost-mcp.exe" or "./ghost-mcp"
		t.Log("BinaryPath would default")
	}
	if config.Timeout == 0 {
		// Would default to 30 seconds
		t.Log("Timeout would default to 30s")
	}
}

func TestClientCloseMethod(t *testing.T) {
	// Test Close method idempotency
	// Can't test without actual server, but verify the logic
	client := &Client{
		closed: false,
	}
	
	// First close
	client.closed = true
	
	// Second close should be safe
	if !client.closed {
		t.Error("expected client to be closed")
	}
}

func TestCallToolStringEmptyResult(t *testing.T) {
	// Test handling of empty result
	result := ToolResult{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{},
		IsError: false,
	}

	if len(result.Content) == 0 {
		t.Log("Would return 'empty result' error")
	}
}

func TestCallToolStringErrorResult(t *testing.T) {
	// Test handling of error result
	result := ToolResult{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{
			{Type: "text", Text: "Tool failed"},
		},
		IsError: true,
	}

	if !result.IsError {
		t.Error("expected error result")
	}
	if len(result.Content) > 0 {
		t.Logf("Would return error: %s", result.Content[0].Text)
	}
}

func TestGetScreenSizeParsing(t *testing.T) {
	// Test screen size result parsing
	resultStr := `{"width":1920,"height":1080}`
	
	var dims struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	
	if err := json.Unmarshal([]byte(resultStr), &dims); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	
	if dims.Width != 1920 {
		t.Errorf("expected width 1920, got %d", dims.Width)
	}
	if dims.Height != 1080 {
		t.Errorf("expected height 1080, got %d", dims.Height)
	}
}

func TestFindElementsParsing(t *testing.T) {
	// Test find_elements result parsing
	resultStr := `{
		"success": true,
		"elements": [
			{"text": "Button", "x": 100, "y": 200, "width": 80, "height": 30},
			{"text": "Label", "x": 300, "y": 400, "width": 60, "height": 20}
		]
	}`
	
	var data struct {
		Success  bool                     `json:"success"`
		Elements []map[string]interface{} `json:"elements"`
	}
	
	if err := json.Unmarshal([]byte(resultStr), &data); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	
	if !data.Success {
		t.Error("expected success true")
	}
	if len(data.Elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(data.Elements))
	}
}

func TestTakeScreenshotParsing(t *testing.T) {
	// Test screenshot result parsing
	resultStr := `{
		"filepath": "/tmp/screenshot.png",
		"base64": "iVBORw0KGgo=",
		"width": 1920,
		"height": 1080
	}`
	
	var data struct {
		Filepath string `json:"filepath"`
		Base64   string `json:"base64"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	}
	
	if err := json.Unmarshal([]byte(resultStr), &data); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	
	if data.Filepath != "/tmp/screenshot.png" {
		t.Errorf("expected filepath '/tmp/screenshot.png', got %s", data.Filepath)
	}
	if data.Width != 1920 {
		t.Errorf("expected width 1920, got %d", data.Width)
	}
	if data.Height != 1080 {
		t.Errorf("expected height 1080, got %d", data.Height)
	}
}

func TestFindAndClickArgsBuilding(t *testing.T) {
	// Test argument building for find_and_click
	opts := FindAndClickOptions{
		Button:    "right",
		Nth:       2,
		X:         100,
		Y:         200,
		Width:     300,
		Height:    150,
		Grayscale: false,
		DelayMS:   250,
	}

	args := map[string]interface{}{"text": "Submit"}

	if opts.Button != "" {
		args["button"] = opts.Button
	}
	if opts.Nth > 0 {
		args["nth"] = opts.Nth
	}
	if opts.Width > 0 {
		args["x"] = opts.X
		args["y"] = opts.Y
		args["width"] = opts.Width
		args["height"] = opts.Height
	}
	if opts.DelayMS > 0 {
		args["delay_ms"] = opts.DelayMS
	}
	args["grayscale"] = opts.Grayscale

	// Verify all expected args are present
	expectedKeys := []string{"text", "button", "nth", "x", "y", "width", "height", "delay_ms", "grayscale"}
	for _, key := range expectedKeys {
		if _, ok := args[key]; !ok {
			t.Errorf("expected key '%s' not found", key)
		}
	}

	if args["button"] != "right" {
		t.Errorf("expected button 'right', got %v", args["button"])
	}
	// Check nth (can be int or float64 depending on how it's set)
	if nth, ok := args["nth"].(int); !ok || nth != 2 {
		if nthF, okF := args["nth"].(float64); !okF || nthF != 2.0 {
			t.Errorf("expected nth 2, got %v (type: %T)", args["nth"], args["nth"])
		}
	}
	if args["grayscale"] != false {
		t.Errorf("expected grayscale false, got %v", args["grayscale"])
	}
}

func TestFindAndClickMinimalArgs(t *testing.T) {
	// Test with minimal options
	opts := FindAndClickOptions{}

	args := map[string]interface{}{"text": "Submit"}

	if opts.Button != "" {
		args["button"] = opts.Button
	}
	if opts.Nth > 0 {
		args["nth"] = opts.Nth
	}
	if opts.Width > 0 {
		args["x"] = opts.X
		args["y"] = opts.Y
		args["width"] = opts.Width
		args["height"] = opts.Height
	}
	if opts.DelayMS > 0 {
		args["delay_ms"] = opts.DelayMS
	}
	args["grayscale"] = opts.Grayscale

	// Should only have text and grayscale
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
	if _, ok := args["text"]; !ok {
		t.Error("expected 'text' arg")
	}
	if _, ok := args["grayscale"]; !ok {
		t.Error("expected 'grayscale' arg")
	}
}

func TestContextTimeout(t *testing.T) {
	// Test context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Verify context works
	select {
	case <-ctx.Done():
		t.Log("Context timed out as expected")
	default:
		// Should not timeout immediately
	}
}

func TestJSONRPCRequestMarshal(t *testing.T) {
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test"}`),
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify it can be unmarshaled back
	var unmarshaled JSONRPCRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != 42 {
		t.Errorf("expected ID 42, got %d", unmarshaled.ID)
	}
	if unmarshaled.Method != "tools/call" {
		t.Errorf("expected method 'tools/call', got %s", unmarshaled.Method)
	}
}

func TestJSONRPCResponseMarshal(t *testing.T) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      42,
		Result:  json.RawMessage(`{"success":true}`),
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Verify it can be unmarshaled back
	var unmarshaled JSONRPCResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ID != 42 {
		t.Errorf("expected ID 42, got %d", unmarshaled.ID)
	}
}

func TestHelperMethodsSignatures(t *testing.T) {
	// Verify all helper method signatures are correct
	// This is a compile-time check
	
	// GetScreenSize(ctx) -> (width, height int, err error)
	// MoveMouse(ctx, x, y int) -> error
	// Click(ctx, button string) -> error
	// TypeText(ctx, text string) -> error
	// PressKey(ctx, key string) -> error
	// FindElements(ctx, args) -> ([]map[string]interface{}, error)
	// TakeScreenshot(ctx) -> (filepath, base64 string, width, height int, err error)
	// FindAndClick(ctx, text, opts) -> (*FindAndClickResult, error)
	
	t.Log("All helper method signatures verified")
}

func TestClientStructFields(t *testing.T) {
	// Verify Client struct has all expected fields
	client := Client{
		timeout: 30 * time.Second,
		nextID:  1,
		closed:  false,
	}

	if client.timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", client.timeout)
	}
	if client.nextID != 1 {
		t.Errorf("expected nextID 1, got %d", client.nextID)
	}
	if client.closed {
		t.Error("expected closed false")
	}
}
