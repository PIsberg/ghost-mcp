package aijudge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

// =============================================================================
// Gemini Vision Judge
// =============================================================================

// Judge wraps the Gemini API for GUI element analysis.
type Judge struct {
	client *genai.Client
	model  string
}

// NewJudge creates a new AI judge using the given API key.
// Model defaults to "gemini-2.5-flash" if empty.
func NewJudge(apiKey string, model string) (*Judge, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("aijudge: GOOGLE_API_KEY is required")
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("aijudge: failed to create Gemini client: %w", err)
	}

	return &Judge{client: client, model: model}, nil
}

// analysisPrompt is the system prompt for GUI element identification.
const analysisPrompt = `You are a GUI element identification expert. Analyze the provided screenshot and identify ALL visible GUI elements.

For each element, provide:
- "text": The visible text content of the element (exactly as displayed). For non-text elements (icons, color swatches), describe briefly.
- "type": One of: "button", "label", "input", "heading", "link", "checkbox", "radio", "dropdown", "toggle", "slider", "icon", "text", "value"
- "rect": Approximate bounding box as {"x": ..., "y": ..., "width": ..., "height": ...} in pixels from top-left.

Type definitions:
- "button": Clickable action element (Submit, Cancel, OK, etc.)
- "label": Form field label (usually followed by an input, often ends with ":")
- "input": Text input field, text area, or search box
- "heading": Section or page title (larger/bolder text)
- "link": Hyperlink or navigation text
- "checkbox": Checkbox control with label
- "radio": Radio button control with label
- "dropdown": Dropdown/select control
- "toggle": Toggle/switch control
- "slider": Slider/range control
- "icon": Non-text visual element
- "text": General body text, descriptions, paragraphs
- "value": Numeric display, status indicator, counter

Rules:
1. Include EVERY visible text element, no matter how small.
2. Be precise with bounding box estimates - they should tightly wrap the element.
3. Classify each element by its VISUAL appearance and context, not just its text.
4. For composite elements (e.g., a labeled checkbox), report the checkbox and label separately.
5. Do NOT include browser chrome, window decorations, or OS UI elements.

Respond with ONLY a valid JSON array. No markdown, no explanation, no code fences. Example:
[{"text":"Submit","type":"button","rect":{"x":100,"y":200,"width":80,"height":30}}]`

// AnalyzeScreenshot sends a screenshot to Gemini and returns the identified elements.
func (j *Judge) AnalyzeScreenshot(ctx context.Context, imageBytes []byte, mimeType string) ([]JudgedElement, error) {
	if mimeType == "" {
		mimeType = "image/png"
	}

	parts := []*genai.Part{
		{Text: analysisPrompt},
		{InlineData: &genai.Blob{
			MIMEType: mimeType,
			Data:     imageBytes,
		}},
	}

	config := &genai.GenerateContentConfig{
		Temperature:      genai.Ptr(float32(0.0)), // Maximally deterministic output
		TopP:             genai.Ptr(float32(0.8)),
		MaxOutputTokens:  32768, // GUI screenshots can yield 60+ elements; 8192 truncated mid-array
		ResponseMIMEType: "application/json",
	}

	content := []*genai.Content{{Parts: parts}}

	// Retry with exponential backoff for transient errors (503, 429)
	var result *genai.GenerateContentResponse
	var lastErr error
	retryDelays := []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second}

	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		result, lastErr = j.client.Models.GenerateContent(ctx, j.model, content, config)
		if lastErr == nil {
			break
		}

		errStr := lastErr.Error()
		isRetryable := strings.Contains(errStr, "503") ||
			strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "UNAVAILABLE") ||
			strings.Contains(errStr, "RESOURCE_EXHAUSTED")

		if !isRetryable || attempt >= len(retryDelays) {
			return nil, fmt.Errorf("aijudge: Gemini API call failed: %w", lastErr)
		}

		delay := retryDelays[attempt]
		fmt.Fprintf(os.Stderr, "aijudge: retryable error (attempt %d/%d), waiting %v: %v\n",
			attempt+1, len(retryDelays), delay, lastErr)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("aijudge: context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("aijudge: Gemini API call failed after retries: %w", lastErr)
	}

	// Extract text from the response
	text := extractResponseText(result)
	if text == "" {
		return nil, fmt.Errorf("aijudge: empty response from Gemini")
	}

	// Parse the JSON array from the response
	elements, err := parseJudgeResponse(text)
	if err != nil {
		return nil, fmt.Errorf("aijudge: failed to parse response: %w\nraw response: %s", err, truncate(text, 500))
	}

	return elements, nil
}

// extractResponseText pulls the text content from a Gemini response.
func extractResponseText(result *genai.GenerateContentResponse) string {
	if result == nil || len(result.Candidates) == 0 {
		return ""
	}
	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return ""
	}

	var texts []string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "")
}

// parseJudgeResponse extracts a JSON array of JudgedElement from the response text.
// It handles cases where the model wraps the JSON in markdown code fences.
func parseJudgeResponse(text string) ([]JudgedElement, error) {
	text = strings.TrimSpace(text)

	// Strip markdown code fences if present
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		// Find the opening and closing fence
		start := 1 // skip first line (```)
		end := len(lines) - 1
		if end > start && strings.HasPrefix(lines[end], "```") {
			end--
		}
		if start <= end {
			text = strings.Join(lines[start:end+1], "\n")
		}
	}

	text = strings.TrimSpace(text)

	// Find the JSON array boundaries
	startIdx := strings.Index(text, "[")
	if startIdx == -1 {
		return nil, fmt.Errorf("no JSON array found in response")
	}
	endIdx := strings.LastIndex(text, "]")

	// If the response was truncated mid-array (no closing ]), try to recover
	// the complete elements parsed so far by trimming back to the last complete
	// object and synthesizing a closing bracket.
	if endIdx == -1 || endIdx <= startIdx {
		recovered, ok := recoverTruncatedArray(text[startIdx:])
		if !ok {
			return nil, fmt.Errorf("response appears truncated and could not be recovered (likely hit MaxOutputTokens)")
		}
		var elements []JudgedElement
		if err := json.Unmarshal([]byte(recovered), &elements); err != nil {
			return nil, fmt.Errorf("JSON parse error after recovery: %w", err)
		}
		return elements, nil
	}

	jsonStr := text[startIdx : endIdx+1]

	var elements []JudgedElement
	if err := json.Unmarshal([]byte(jsonStr), &elements); err != nil {
		return nil, fmt.Errorf("JSON parse error: %w", err)
	}

	return elements, nil
}

// recoverTruncatedArray attempts to salvage a JSON array that was cut off
// mid-element. It walks the input tracking brace depth and trims back to the
// last point where depth returned to 1 (i.e. between top-level objects), then
// appends a closing bracket.
func recoverTruncatedArray(s string) (string, bool) {
	depth := 0
	inStr := false
	escaped := false
	lastGoodEnd := -1 // index just after a top-level object's closing brace
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if inStr {
			if r == '\\' {
				escaped = true
			} else if r == '"' {
				inStr = false
			}
			continue
		}
		switch r {
		case '"':
			inStr = true
		case '[':
			depth++
		case '{':
			depth++
		case ']':
			depth--
		case '}':
			depth--
			if depth == 1 {
				lastGoodEnd = i + 1
			}
		}
	}
	if lastGoodEnd <= 0 {
		return "", false
	}
	return s[:lastGoodEnd] + "]", true
}

// truncate shortens a string with an ellipsis if it exceeds maxLen.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
