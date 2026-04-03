// audit_hooks_test.go — Tests for RegisterHooks and extractToolResultError.
package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// =============================================================================
// extractToolResultError
// =============================================================================

func TestExtractToolResultError_Nil(t *testing.T) {
	got := extractToolResultError(nil)
	if got != "unknown tool error" {
		t.Errorf("expected %q, got %q", "unknown tool error", got)
	}
}

func TestExtractToolResultError_EmptyContent(t *testing.T) {
	result := &mcp.CallToolResult{}
	got := extractToolResultError(result)
	if got != "unknown tool error" {
		t.Errorf("expected %q, got %q", "unknown tool error", got)
	}
}

func TestExtractToolResultError_TextContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Text: "something exploded"}},
	}
	got := extractToolResultError(result)
	if got != "something exploded" {
		t.Errorf("expected %q, got %q", "something exploded", got)
	}
}

func TestExtractToolResultError_NonTextContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{mcp.ImageContent{Data: "abc", MIMEType: "image/png"}},
	}
	got := extractToolResultError(result)
	if got != "tool error (non-text content)" {
		t.Errorf("expected %q, got %q", "tool error (non-text content)", got)
	}
}

// =============================================================================
// RegisterHooks — invoke callbacks directly via exported hook slices
// =============================================================================

func TestRegisterHooks_BeforeCallTool(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	req := &mcp.CallToolRequest{}
	req.Params.Name = "find_and_click"
	req.Params.Arguments = map[string]any{"text": "OK"}

	for _, fn := range hooks.OnBeforeCallTool {
		fn(context.Background(), nil, req)
	}

	entries := readEntries(t, al)
	found := false
	for _, e := range entries {
		if e.Event == EventToolCall && e.Tool == "find_and_click" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TOOL_CALL entry for find_and_click, got entries: %v", entries)
	}
}

func TestRegisterHooks_AfterCallTool_Success(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	req := &mcp.CallToolRequest{}
	req.Params.Name = "move_mouse"
	result := &mcp.CallToolResult{IsError: false}

	for _, fn := range hooks.OnAfterCallTool {
		fn(context.Background(), nil, req, result)
	}

	entries := readEntries(t, al)
	found := false
	for _, e := range entries {
		if e.Event == EventToolSuccess && e.Tool == "move_mouse" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TOOL_SUCCESS entry, got entries: %v", entries)
	}
}

func TestRegisterHooks_AfterCallTool_Screenshot(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	req := &mcp.CallToolRequest{}
	req.Params.Name = "take_screenshot"
	result := &mcp.CallToolResult{IsError: false}

	for _, fn := range hooks.OnAfterCallTool {
		fn(context.Background(), nil, req, result)
	}

	entries := readEntries(t, al)
	found := false
	for _, e := range entries {
		if e.Event == EventScreenshot {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SCREENSHOT_REQUESTED entry, got entries: %v", entries)
	}
}

func TestRegisterHooks_AfterCallTool_Failure(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	req := &mcp.CallToolRequest{}
	req.Params.Name = "click"
	result := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{Text: "coords out of range"}},
	}

	for _, fn := range hooks.OnAfterCallTool {
		fn(context.Background(), nil, req, result)
	}

	entries := readEntries(t, al)
	found := false
	for _, e := range entries {
		if e.Event == EventToolFailure && e.Tool == "click" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected TOOL_FAILURE entry, got entries: %v", entries)
	}
}

func TestRegisterHooks_OnError_AuthFailure(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	for _, fn := range hooks.OnError {
		fn(context.Background(), nil, mcp.MethodToolsCall, nil, ErrAuthFailed)
	}

	entries := readEntries(t, al)
	found := false
	for _, e := range entries {
		if e.Event == EventAuthFailure {
			found = true
		}
	}
	if !found {
		t.Errorf("expected AUTH_FAILURE entry, got entries: %v", entries)
	}
}

func TestRegisterHooks_OnError_Other(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	for _, fn := range hooks.OnError {
		fn(context.Background(), nil, mcp.MethodToolsCall, nil, errors.New("unexpected error"))
	}

	entries := readEntries(t, al)
	found := false
	for _, e := range entries {
		if e.Event == EventRequestError {
			found = true
		}
	}
	if !found {
		t.Errorf("expected REQUEST_ERROR entry, got entries: %v", entries)
	}
}

func TestRegisterHooks_OnError_NilIgnored(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	// nil error must not panic or log anything
	for _, fn := range hooks.OnError {
		fn(context.Background(), nil, mcp.MethodToolsCall, nil, nil)
	}
}

func TestRegisterHooks_AfterInitialize_NilRequest(t *testing.T) {
	al := newTestLogger(t)
	hooks := &server.Hooks{}
	RegisterHooks(hooks, al)

	// nil request must not panic
	for _, fn := range hooks.OnAfterInitialize {
		fn(context.Background(), nil, nil, nil)
	}
}
