//go:build !integration

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestHandleGhostMCPGuide_ReturnsMessages(t *testing.T) {
	result, err := handleGhostMCPGuide(context.Background(), mcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	msg := result.Messages[0]
	if msg.Role != mcp.RoleUser {
		t.Errorf("expected role %q, got %q", mcp.RoleUser, msg.Role)
	}
	tc, ok := msg.Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", msg.Content)
	}
	if tc.Text == "" {
		t.Error("expected non-empty guide text")
	}
	if tc.Type != "text" {
		t.Errorf("expected type %q, got %q", "text", tc.Type)
	}
}

func TestGhostMCPGuide_ContainsKeyTools(t *testing.T) {
	tools := []string{
		"find_and_click",
		"find_elements",
		"find_click_and_type",
		"execute_workflow",
		"learn_screen",
		"wait_for_text",
		"find_and_click_all",
		"clear_learned_view",
	}
	for _, tool := range tools {
		if !strings.Contains(ghostMCPGuide, tool) {
			t.Errorf("guide missing tool: %s", tool)
		}
	}
}

func TestGhostMCPGuide_ContainsScenarios(t *testing.T) {
	sections := []string{
		"Scenario A",
		"Scenario B",
		"Scenario C",
		"Scenario D",
		"Learning Mode",
		"Safety",
	}
	for _, s := range sections {
		if !strings.Contains(ghostMCPGuide, s) {
			t.Errorf("guide missing section: %s", s)
		}
	}
}

func TestRegisterPrompts_Registers(t *testing.T) {
	mcpSrv := server.NewMCPServer("test", "1.0")
	// registerPrompts must not panic and the server must be usable afterward
	registerPrompts(mcpSrv)
}
