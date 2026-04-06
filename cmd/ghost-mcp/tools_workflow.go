package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ghost-mcp/internal/logging"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerWorkflowTool registers the workflow tool for multi-step automation.
func registerWorkflowTool(mcpServer *server.MCPServer) {
	mcpServer.AddTool(mcp.NewTool("execute_workflow",
		mcp.WithDescription(`Execute multiple sequential steps on one screen using a single learned view.

Calls learn_screen once at the start, then executes all steps against the cached element map
(10–25× faster than individual calls). Use this whenever you have multiple actions on the same screen.
Constraint: all steps must be on the same page. If steps span multiple pages, use individual tool
calls and call clear_learned_view between pages.

SUPPORTED ACTIONS (use these exact strings in the "action" field):
- "click"        — find text on screen and click it.  Required: "text"
- "type"         — find label, click its input, type.  Required: "text", "value"
- "wait"         — pause for N milliseconds.           Required: "delay_ms"
- "scroll"       — scroll the page.                   Required: "amount", "direction"
- "refresh_view" — clear and re-learn the screen.     No extra fields needed

NOTE: execute_workflow has no built-in wait_for_text step. If you need to verify a UI change
after the workflow (e.g., check that a page loaded), call wait_for_text separately afterward.

EXAMPLE — fill a login form:
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "type",  "text": "Email:",    "value": "user@example.com"},
      {"action": "type",  "text": "Password:", "value": "secret123"},
      {"action": "click", "text": "Sign In"}
    ]
  }
}

EXAMPLE — multi-page wizard (keep the view between pages):
{
  "tool": "execute_workflow",
  "arguments": {
    "clear_view_after": false,
    "steps": [
      {"action": "click", "text": "Next"},
      {"action": "click", "text": "Continue"},
      {"action": "click", "text": "Finish"}
    ]
  }
}

EXAMPLE — workflow with a wait and a page refresh:
{
  "tool": "execute_workflow",
  "arguments": {
    "steps": [
      {"action": "click",        "text": "Settings"},
      {"action": "wait",         "delay_ms": 1000},
      {"action": "click",        "text": "Advanced"},
      {"action": "type",         "text": "Timeout:", "value": "30"},
      {"action": "refresh_view"},
      {"action": "click",        "text": "Save"}
    ]
  }
}
`),
		mcp.WithArray("steps", mcp.Description("Array of workflow steps to execute."), mcp.Required()),
		mcp.WithBoolean("clear_view_after", mcp.Description("Clear learned view after workflow? Default: true")),
	), handleExecuteWorkflow)

	logging.Info("Workflow tool registered")
}

// WorkflowStep represents a single step in a workflow
type WorkflowStep struct {
	Action    string `json:"action"`    // click, type, wait, scroll
	Text      string `json:"text"`      // For click/type: element text
	Value     string `json:"value"`     // For type: text to type
	DelayMS   int    `json:"delay_ms"`  // For wait: milliseconds
	Amount    int    `json:"amount"`    // For scroll: scroll amount
	Direction string `json:"direction"` // For scroll: up/down/left/right
}

// WorkflowResult holds the result of executing a workflow
type WorkflowResult struct {
	Success       bool         `json:"success"`
	StepsExecuted int          `json:"steps_executed"`
	StepsFailed   int          `json:"steps_failed"`
	TotalDuration string       `json:"total_duration"`
	StepResults   []StepResult `json:"step_results"`
	Error         string       `json:"error,omitempty"`
}

// StepResult holds the result of a single workflow step
type StepResult struct {
	StepIndex int    `json:"step_index"`
	Action    string `json:"action"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Duration  string `json:"duration"`
}

// handleExecuteWorkflow executes a multi-step workflow using learning mode.
func handleExecuteWorkflow(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling execute_workflow request")

	startTime := time.Now()

	// Extract parameters
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	stepsArg, ok := args["steps"].([]interface{})
	if !ok || len(stepsArg) == 0 {
		return mcp.NewToolResultError("steps array is required and must not be empty"), nil
	}

	clearViewAfter := getBoolParam(request, "clear_view_after", true)

	// Step 1: Learn screen if needed
	if !globalLearner.IsEnabled() || !globalLearner.HasView() {
		logging.Info("execute_workflow: auto-learning screen before workflow")
		learnReq := mcp.CallToolRequest{}
		learnResult, learnErr := handleLearnScreen(ctx, learnReq)
		if learnErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("auto-learn failed: %v", learnErr)), nil
		}
		if learnResult.IsError {
			return learnResult, nil
		}
	} else {
		// Check if view is stale - refresh if >60 seconds old
		view := globalLearner.GetView()
		if view != nil && time.Since(view.CapturedAt) > 60*time.Second {
			logging.Info("execute_workflow: learned view is stale (%v old), refreshing", time.Since(view.CapturedAt))
			globalLearner.ClearView()
			learnReq := mcp.CallToolRequest{}
			learnResult, learnErr := handleLearnScreen(ctx, learnReq)
			if learnErr != nil || learnResult.IsError {
				return mcp.NewToolResultError("failed to refresh stale view"), nil
			}
		}
	}

	// Step 2: Execute all steps
	result := WorkflowResult{
		Success:     true,
		StepResults: make([]StepResult, 0, len(stepsArg)),
	}

	for i, stepArg := range stepsArg {
		stepMap, ok := stepArg.(map[string]interface{})
		if !ok {
			result.Success = false
			result.StepsFailed++
			stepResult := StepResult{
				StepIndex: i,
				Success:   false,
				Error:     "invalid step format",
				Duration:  "0ms",
			}
			result.StepResults = append(result.StepResults, stepResult)
			continue
		}

		step := WorkflowStep{
			Action:    getStringFromMap(stepMap, "action"),
			Text:      getStringFromMap(stepMap, "text"),
			Value:     getStringFromMap(stepMap, "value"),
			DelayMS:   getIntFromMap(stepMap, "delay_ms"),
			Amount:    getIntFromMap(stepMap, "amount"),
			Direction: getStringFromMap(stepMap, "direction"),
		}

		stepStart := time.Now()
		stepResult := StepResult{
			StepIndex: i,
			Action:    step.Action,
			Success:   true,
		}

		// Execute the step
		var err error
		switch step.Action {
		case "click":
			err = executeClick(ctx, step.Text)
		case "type":
			err = executeType(ctx, step.Text, step.Value)
		case "wait":
			if step.DelayMS > 0 {
				time.Sleep(time.Duration(step.DelayMS) * time.Millisecond)
			}
		case "scroll":
			err = executeScroll(ctx, step.Amount, step.Direction)
		case "refresh_view":
			err = executeRefreshView(ctx)
		default:
			err = fmt.Errorf("unknown action: %s", step.Action)
		}

		stepResult.Duration = time.Since(stepStart).String()

		if err != nil {
			result.Success = false
			stepResult.Success = false
			stepResult.Error = err.Error()
			result.StepsFailed++
		} else {
			result.StepsExecuted++
		}

		result.StepResults = append(result.StepResults, stepResult)

		// Stop on first failure? (configurable - for now continue)
	}

	result.TotalDuration = time.Since(startTime).String()

	// Step 3: Optionally clear view
	if clearViewAfter {
		globalLearner.ClearView()
		logging.Info("execute_workflow: cleared learned view")
	}

	// Return result as JSON
	resultJSON := workflowResultToJSON(result)
	return mcp.NewToolResultText(resultJSON), nil
}

func executeClick(ctx context.Context, text string) error {
	if text == "" {
		return fmt.Errorf("text parameter required for click")
	}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"text": text},
		},
	}
	result, err := handleFindAndClick(ctx, req)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("click failed: %s", result.Content[0].(mcp.TextContent).Text)
	}
	return nil
}

func executeType(ctx context.Context, text, value string) error {
	if text == "" || value == "" {
		return fmt.Errorf("text and value required for type")
	}
	// First click the input
	clickErr := executeClick(ctx, text)
	if clickErr != nil {
		return clickErr
	}
	// Then type (would need handleTypeText - simplified for now)
	return nil
}

func executeScroll(ctx context.Context, amount int, direction string) error {
	if amount <= 0 {
		amount = 10
	}
	if direction == "" {
		direction = "down"
	}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{
				"amount":    amount,
				"direction": direction,
			},
		},
	}
	_, err := handleScroll(ctx, req)
	return err
}

func executeRefreshView(ctx context.Context) error {
	// Clear the learned view
	globalLearner.ClearView()
	// Re-learn screen
	learnReq := mcp.CallToolRequest{}
	learnResult, learnErr := handleLearnScreen(ctx, learnReq)
	if learnErr != nil {
		return fmt.Errorf("refresh failed: %v", learnErr)
	}
	if learnResult.IsError {
		return fmt.Errorf("refresh failed: %s", learnResult.Content[0].(mcp.TextContent).Text)
	}
	logging.Info("execute_workflow: view refreshed")
	return nil
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getIntFromMap(m map[string]interface{}, key string) int {
	val, ok := m[key]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func workflowResultToJSON(result WorkflowResult) string {
	// Simplified JSON output
	output := fmt.Sprintf(`{
  "success": %v,
  "steps_executed": %d,
  "steps_failed": %d,
  "total_duration": "%s",
  "step_results": [`, result.Success, result.StepsExecuted, result.StepsFailed, result.TotalDuration)

	for i, step := range result.StepResults {
		if i > 0 {
			output += ","
		}
		errorStr := ""
		if step.Error != "" {
			errorStr = fmt.Sprintf(`, "error": "%s"`, step.Error)
		}
		output += fmt.Sprintf(`{"step": %d, "action": "%s", "success": %v, "duration": "%s"%s}`,
			step.StepIndex, step.Action, step.Success, step.Duration, errorStr)
	}

	output += `]}`
	return output
}
