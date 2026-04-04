package main

import (
	"context"
	"fmt"
	"image"
	"time"

	"github.com/ghost-mcp/internal/logging"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerSmartClickTool registers the smart_click tool.
func registerSmartClickTool(mcpServer *server.MCPServer) {
	mcpServer.AddTool(mcp.NewTool("smart_click",
		mcp.WithDescription(`Convenience wrapper: runs learn_screen then find_and_click in one call.

Use for a single click on an unfamiliar screen when you don't want to manage the
learn/act/clear lifecycle manually.

For more than one action on the same screen, use learn_screen explicitly so you
control when clear_learned_view is called.
`),
		mcp.WithString("text", mcp.Description("Text of the button/link to click."), mcp.Required()),
	), handleSmartClick)

	logging.Info("Smart click tool registered")
}

// handleSmartClick combines learn_screen + find_and_click in one tool.
// This is the RECOMMENDED tool for AI that forgets to use learning mode.
//
// Workflow:
// 1. Automatically calls learn_screen if no view exists or screen changed
// 2. Uses learned view to find the element
// 3. Clicks the element
//
// This eliminates the need for AI to remember the multi-step workflow.
func handleSmartClick(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling smart_click request")

	text, err := getStringParam(request, "text")
	if err != nil {
		return mcp.NewToolResultError("text parameter required"), nil
	}

	// Step 1: Check if we need to learn screen
	needLearn := !globalLearner.IsEnabled() || !globalLearner.HasView()

	// Step 2: If we have a view, check if screen changed significantly
	if !needLearn && globalLearner.HasView() {
		view := globalLearner.GetView()
		if view != nil && view.CapturedAt.Before(time.Now().Add(-30*time.Second)) {
			// View is older than 30 seconds, might be stale
			logging.Debug("smart_click: view is %v old, re-learning recommended", time.Since(view.CapturedAt))
		}
	}

	// Step 3: Learn if needed
	if needLearn {
		logging.Info("smart_click: auto-learning screen before click")
		learnResult, learnErr := handleLearnScreen(ctx, request)
		if learnErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("auto-learn failed: %v", learnErr)), nil
		}
		if learnResult.IsError {
			return learnResult, nil
		}
	}

	// Step 4: Click using learned view
	clickRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"text": text,
			},
		},
	}

	return handleFindAndClick(ctx, clickRequest)
}

// hashImageFast computes a fast hash for screen change detection
func hashImageFast(img image.Image) uint64 {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	var hash uint64 = 5381
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 10 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 10 {
			r, g, b, _ := img.At(x, y).RGBA()
			// Use weighted sum to distinguish colors with same total brightness
			// Red gets weight 1, green gets weight 2, blue gets weight 4
			hash = ((hash << 5) + hash) + uint64(r+g*2+b*4)
		}
	}
	return hash
}
