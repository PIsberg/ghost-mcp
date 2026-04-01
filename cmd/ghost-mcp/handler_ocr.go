package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/ocr"
	"github.com/ghost-mcp/internal/validate"
	"github.com/go-vgo/robotgo"
	"github.com/mark3labs/mcp-go/mcp"
)

var (
	prepareParallelOCRImageSet = ocr.PrepareParallelImageSet
	readPreparedOCRImage       = ocr.ReadPreparedBytes
	findTextWithScrolling      = scrollSearchForText
	waitForTextCaptureImage    = func(x, y, width, height int) (image.Image, error) { return robotgo.CaptureImg(x, y, width, height) }
	waitForTextSleep           = time.Sleep
)

const (
	waitForTextInitialDelay = 200 * time.Millisecond
	waitForTextPollInterval = 100 * time.Millisecond
)



func handleFindAndClick(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling find_and_click request")

	searchText, err := getStringParam(request, "text")
	if err != nil {
		logging.Error("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}
	if searchText == "" {
		return mcp.NewToolResultError("text must not be empty"), nil
	}

	button, err := getStringParam(request, "button")
	if err != nil {
		button = "left"
	}
	validButtons := map[string]bool{"left": true, "right": true, "middle": true}
	if !validButtons[button] {
		return mcp.NewToolResultError(fmt.Sprintf("invalid button '%s', must be 'left', 'right', or 'middle'", button)), nil
	}

	nth := 1
	if n, err := getIntParam(request, "nth"); err == nil {
		if n < 1 {
			return mcp.NewToolResultError("nth must be 1 or greater"), nil
		}
		nth = n
	}

	screenW, screenH := robotgo.GetScreenSize()

	// Optional region — defaults to full screen.
	regionX := 0
	regionY := 0
	regionW := screenW
	regionH := screenH
	if v, err := getIntParam(request, "x"); err == nil {
		regionX = v
	}
	if v, err := getIntParam(request, "y"); err == nil {
		regionY = v
	}
	if v, err := getIntParam(request, "width"); err == nil {
		regionW = v
	}
	if v, err := getIntParam(request, "height"); err == nil {
		regionH = v
	}

	if err := validate.ScreenRegion(regionX, regionY, regionW, regionH, screenW, screenH); err != nil {
		logging.Error("Screen region validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid screen region: %v", err)), nil
	}

	logging.Info("find_and_click: OCR region (%d,%d) %dx%d for text %q", regionX, regionY, regionW, regionH, searchText)

	// Capture region for OCR.
	img, captureErr := robotgo.CaptureImg(regionX, regionY, regionW, regionH)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	saveScreenshotIfKept(img, "ghost-mcp-findclick")

	grayscale := getBoolParam(request, "grayscale", true)

	minX, minY, maxX, maxY, found, passName := parallelFindText(ctx, img, searchText, nth, grayscale)
	if !found {
		logging.Info("Text %q (occurrence %d) not found on screen", searchText, nth)
		return mcp.NewToolResultError(buildFindTextFailureMessage(img, searchText, nth, regionX, regionY, regionW, regionH, grayscale)), nil
	}

	// Calculate center of the merged button bounds
	cx := regionX + (minX+maxX)/2
	cy := regionY + (minY+maxY)/2

	if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("found text but center out of bounds: %v", err)), nil
	}

	logging.Info("ACTION: Found %q (occurrence %d) via %s pass at box (%d,%d)-(%d,%d), clicking center (%d,%d) with %s",
		searchText, nth, passName, minX, minY, maxX, maxY, cx, cy, button)
	robotgo.Move(cx, cy)

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	robotgo.Click(button, false)
	applyClickDelay(request)

	finalX, finalY := robotgo.GetMousePos()
	if finalX != cx || finalY != cy {
		logging.Info("WARNING: cursor moved after click: requested (%d,%d) actual (%d,%d)", cx, cy, finalX, finalY)
	}
	logging.Info("ACTION COMPLETE: find_and_click %q at (%d, %d)", searchText, finalX, finalY)

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"requested_x":%d,"requested_y":%d,"actual_x":%d,"actual_y":%d,"button":%q,"occurrence":%d}`,
		searchText, minX, minY, maxX-minX, maxY-minY, cx, cy, finalX, finalY, button, nth,
	)), nil
}

// findButtonBounds finds the full bounding box of a button by merging adjacent
// words that match searchText. This handles multi-word buttons like "Save Changes"
// by returning the combined bounding box of all matching words on the same line.
// Returns the merged bounds relative to the OCR image, or false if not found.
func findButtonBounds(ocrResult *ocr.Result, searchText string, nth int) (minX, minY, maxX, maxY int, found bool) {
	needle := strings.ToLower(strings.TrimSpace(searchText))
	matchCount := 0

	for i, w := range ocrResult.Words {
		minX, minY = w.X, w.Y
		maxX, maxY = w.X+w.Width, w.Y+w.Height
		avgHeight := w.Height
		avgWidth := w.Width
		verticalThreshold := avgHeight / 3
		maxHGap := avgWidth / 2
		phrase := strings.ToLower(strings.TrimSpace(w.Text))
		matched := strings.Contains(phrase, needle)

		for j := i + 1; j < len(ocrResult.Words); j++ {
			next := ocrResult.Words[j]
			nextCenterY := next.Y + next.Height/2
			currCenterY := minY + (maxY-minY)/2
			if abs(nextCenterY-currCenterY) > verticalThreshold {
				continue
			}
			hGap := next.X - maxX
			if hGap < 0 {
				continue
			}
			if hGap > maxHGap {
				break
			}

			if next.X < minX {
				minX = next.X
			}
			if next.Y < minY {
				minY = next.Y
			}
			if next.X+next.Width > maxX {
				maxX = next.X + next.Width
			}
			if next.Y+next.Height > maxY {
				maxY = next.Y + next.Height
			}
			phrase += " " + strings.ToLower(strings.TrimSpace(next.Text))
			if strings.Contains(phrase, needle) {
				matched = true
			}
		}

		if matched {
			matchCount++
			if matchCount == nth {
				return minX, minY, maxX, maxY, true
			}
		}
	}
	return 0, 0, 0, 0, false
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

type ocrCandidate struct {
	text  string
	score int
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			insertCost := curr[j-1] + 1
			deleteCost := prev[j] + 1
			replaceCost := prev[j-1] + cost
			curr[j] = min(insertCost, min(deleteCost, replaceCost))
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func closestOCRMatches(ocrResult *ocr.Result, searchText string, limit int) []string {
	if ocrResult == nil || limit <= 0 {
		return nil
	}

	needle := strings.ToLower(strings.TrimSpace(searchText))
	if needle == "" {
		return nil
	}

	seen := make(map[string]bool)
	candidates := make([]ocrCandidate, 0, len(ocrResult.Words))
	for i, w := range ocrResult.Words {
		phrase := strings.TrimSpace(w.Text)
		if phrase == "" {
			continue
		}
		candidateText := phrase
		addCandidate := func(text string) {
			normalized := strings.ToLower(strings.TrimSpace(text))
			if normalized == "" || seen[normalized] {
				return
			}
			seen[normalized] = true
			score := levenshteinDistance(needle, normalized)
			if strings.Contains(normalized, needle) || strings.Contains(needle, normalized) {
				score -= 4
			}
			candidates = append(candidates, ocrCandidate{text: text, score: score})
		}
		addCandidate(candidateText)

		maxX := w.X + w.Width
		avgHeight := w.Height
		avgWidth := w.Width
		verticalThreshold := avgHeight / 3
		maxHGap := avgWidth / 2
		for j := i + 1; j < len(ocrResult.Words) && j < i+4; j++ {
			next := ocrResult.Words[j]
			nextCenterY := next.Y + next.Height/2
			currCenterY := w.Y + w.Height/2
			if abs(nextCenterY-currCenterY) > verticalThreshold {
				continue
			}
			hGap := next.X - maxX
			if hGap < 0 {
				continue
			}
			if hGap > maxHGap {
				break
			}
			candidateText += " " + strings.TrimSpace(next.Text)
			maxX = next.X + next.Width
			addCandidate(candidateText)
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return len(candidates[i].text) < len(candidates[j].text)
		}
		return candidates[i].score < candidates[j].score
	})

	out := make([]string, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		out = append(out, candidate.text)
		if len(out) == limit {
			break
		}
	}
	return out
}

func formatFindTextFailureMessage(searchText string, nth int, regionX, regionY, regionW, regionH int, matches []string) string {
	msg := fmt.Sprintf(
		`text %q not found on screen (occurrence %d). Search region: x=%d y=%d width=%d height=%d.`,
		searchText, nth, regionX, regionY, regionW, regionH,
	)
	if len(matches) > 0 {
		msg += fmt.Sprintf(" Closest OCR matches: %q.", matches)
	}

	// Add specific actionable suggestions
	msg += ` TRY THESE: (a) Search for LABEL text like "Text Input:" or "Email:" (not placeholder). (b) Use find_elements to see visible text. (c) Use scroll_until_text for off-screen content.`

	return msg
}

// findLabelCandidates extracts words ending with ":" which are likely field labels
func findLabelCandidates(ocrResult *ocr.Result, limit int) []string {
	labels := []string{}
	for _, w := range ocrResult.Words {
		text := strings.TrimSpace(w.Text)
		if strings.HasSuffix(text, ":") && len(text) > 1 {
			labels = append(labels, text)
			if len(labels) >= limit {
				break
			}
		}
	}
	return labels
}

func buildFindTextFailureMessage(img image.Image, searchText string, nth int, regionX, regionY, regionW, regionH int, grayscale bool) string {
	ocrResult, err := ocr.ReadImage(img, ocr.Options{Color: !grayscale})
	if err != nil || ocrResult == nil {
		return formatFindTextFailureMessage(searchText, nth, regionX, regionY, regionW, regionH, nil)
	}

	matches := closestOCRMatches(ocrResult, searchText, 5)
	
	// Find label candidates (words ending with ":")
	labels := findLabelCandidates(ocrResult, 5)
	
	msg := formatFindTextFailureMessage(searchText, nth, regionX, regionY, regionW, regionH, matches)
	
	if len(labels) > 0 {
		msg += fmt.Sprintf(" DETECTED LABELS: %q - try searching for one of these!", labels)
	}
	
	return msg
}

// findAndClickWord removed in favor of direct clickFoundBounds logic

// handleFindElements discovers all text elements on screen with their bounding boxes.
// Use this to get an overview of clickable elements before targeting specific ones.
func handleFindElements(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling find_elements request")

	screenW, screenH := robotgo.GetScreenSize()

	// Optional region parameters
	regionX, regionY := 0, 0
	regionW, regionH := screenW, screenH
	if x, err := getIntParam(request, "x"); err == nil {
		regionX = x
	}
	if y, err := getIntParam(request, "y"); err == nil {
		regionY = y
	}
	if w, err := getIntParam(request, "width"); err == nil {
		regionW = w
	}
	if h, err := getIntParam(request, "height"); err == nil {
		regionH = h
	}

	// Capture the region
	img, captureErr := robotgo.CaptureImg(regionX, regionY, regionW, regionH)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	saveScreenshotIfKept(img, "ghost-mcp-findelements")

	// Run OCR with color mode for best element detection
	ocrResult, ocrErr := ocr.ReadImage(img, ocr.Options{Color: true})
	if ocrErr != nil {
		logging.Error("OCR failed: %v", ocrErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", ocrErr)), nil
	}

	// Group words into clickable elements (buttons, links, labels)
	// Filter by confidence and minimum size to avoid noise
	elements := make([]map[string]interface{}, 0)
	for _, w := range ocrResult.Words {
		if w.Confidence < 50 {
			continue // Skip low-confidence detections
		}
		if w.Width < 20 || w.Height < 10 {
			continue // Skip tiny text (likely noise)
		}

		elements = append(elements, map[string]interface{}{
			"text":       w.Text,
			"x":          regionX + w.X,
			"y":          regionY + w.Y,
			"width":      w.Width,
			"height":     w.Height,
			"center_x":   regionX + w.X + w.Width/2,
			"center_y":   regionY + w.Y + w.Height/2,
			"confidence": w.Confidence,
		})
	}

	logging.Info("find_elements: found %d elements in region (%d,%d) %dx%d", len(elements), regionX, regionY, regionW, regionH)

	// Build JSON response
	elementsJSON := "["
	for i, e := range elements {
		if i > 0 {
			elementsJSON += ","
		}
		elementsJSON += fmt.Sprintf(
			`{"text":%q,"x":%d,"y":%d,"width":%d,"height":%d,"center_x":%d,"center_y":%d,"confidence":%.1f}`,
			e["text"], e["x"], e["y"], e["width"], e["height"], e["center_x"], e["center_y"], e["confidence"],
		)
	}
	elementsJSON += "]"

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"element_count":%d,"region":{"x":%d,"y":%d,"width":%d,"height":%d},"elements":%s}`,
		len(elements), regionX, regionY, regionW, regionH, elementsJSON,
	)), nil
}

// handleFindAndClickAll finds and clicks multiple text labels in sequence.
// This is an atomic operation — either all clicks succeed or it returns an error.
// Use this when you need to click multiple buttons (e.g., "Primary", "Success", "Warning")
// without verification loops between each click.
func handleFindAndClickAll(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling find_and_click_all request")

	texts, err := getStringArrayParam(request, "texts")
	if err != nil {
		logging.Error("Invalid texts parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid texts parameter: %v", err)), nil
	}
	if len(texts) == 0 {
		return mcp.NewToolResultError("texts array must not be empty"), nil
	}

	button, _ := getStringParam(request, "button")
	if button == "" {
		button = "left"
	}

	delayMS := 100
	if d, err := getIntParam(request, "delay_ms"); err == nil {
		delayMS = d
	}

	screenW, screenH := robotgo.GetScreenSize()

	// Capture screen once for all lookups
	img, captureErr := robotgo.CaptureImg(0, 0, screenW, screenH)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	saveScreenshotIfKept(img, "ghost-mcp-findclickall")

	grayscale := getBoolParam(request, "grayscale", true)
	prepared, prepareErr := prepareParallelOCRImageSet(img, grayscale)
	if prepareErr != nil {
		logging.Error("find_and_click_all preprocessing failed: %v", prepareErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR preprocessing failed: %v", prepareErr)), nil
	}

	// Click each text in sequence
	clicks := make([]map[string]interface{}, 0, len(texts))
	for _, text := range texts {
		minX, minY, maxX, maxY, found, passName := parallelFindPreparedText(ctx, prepared, text, 1, grayscale)
		if !found {
			logging.Info("find_and_click_all: text %q not found, stopping", text)
			return mcp.NewToolResultError(fmt.Sprintf("text %q not found on screen", text)), nil
		}

		cx := (minX + maxX) / 2
		cy := (minY + maxY) / 2

		logging.Info("ACTION: Clicking %q via %s pass at (%d, %d)", text, passName, cx, cy)
		robotgo.Move(cx, cy)
		time.Sleep(10 * time.Millisecond) // Small delay for mouse movement
		robotgo.Click(button, false)

		if delayMS > 0 {
			time.Sleep(time.Duration(min(delayMS, 10000)) * time.Millisecond)
		}

		finalX, finalY := robotgo.GetMousePos()
		clicks = append(clicks, map[string]interface{}{
			"text":      text,
			"box":       map[string]int{"x": minX, "y": minY, "width": maxX - minX, "height": maxY - minY},
			"clicked_x": cx,
			"clicked_y": cy,
			"actual_x":  finalX,
			"actual_y":  finalY,
			"button":    button,
		})
	}

	logging.Info("ACTION COMPLETE: find_and_click_all clicked %d buttons", len(clicks))

	// Build JSON response
	clicksJSON := "["
	for i, c := range clicks {
		if i > 0 {
			clicksJSON += ","
		}
		clicksJSON += fmt.Sprintf(
			`{"text":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"clicked_x":%d,"clicked_y":%d,"actual_x":%d,"actual_y":%d,"button":%q}`,
			c["text"],
			c["box"].(map[string]int)["x"], c["box"].(map[string]int)["y"],
			c["box"].(map[string]int)["width"], c["box"].(map[string]int)["height"],
			c["clicked_x"], c["clicked_y"], c["actual_x"], c["actual_y"], c["button"],
		)
	}
	clicksJSON += "]"

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"clicked_count":%d,"clicks":%s}`,
		len(clicks), clicksJSON,
	)), nil
}

// handleWaitForText waits for text to appear or disappear from the screen.
// Use this to verify UI state changes after clicking a button.
func handleWaitForText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling wait_for_text request")

	text, err := getStringParam(request, "text")
	if err != nil {
		logging.Error("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}

	timeoutMS := 5000
	if t, err := getIntParam(request, "timeout_ms"); err == nil {
		timeoutMS = t
	}
	if timeoutMS > 30000 {
		timeoutMS = 30000
	}

	visible := getBoolParam(request, "visible", true)

	regionX, regionY := 0, 0
	regionW, regionH := robotgo.GetScreenSize()
	if x, err := getIntParam(request, "x"); err == nil {
		regionX = x
	}
	if y, err := getIntParam(request, "y"); err == nil {
		regionY = y
	}
	if w, err := getIntParam(request, "width"); err == nil {
		regionW = w
	}
	if h, err := getIntParam(request, "height"); err == nil {
		regionH = h
	}

	startTime := time.Now()
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if waitForTextInitialDelay > 0 {
		initialDelay := waitForTextInitialDelay
		if initialDelay > timeout {
			initialDelay = timeout
		}
		waitForTextSleep(initialDelay)
	}

	for time.Since(startTime) < time.Duration(timeoutMS)*time.Millisecond {
		img, captureErr := waitForTextCaptureImage(regionX, regionY, regionW, regionH)
		if captureErr == nil {
			grayscale := getBoolParam(request, "grayscale", true)
			_, _, _, _, found, passName := parallelFindText(ctx, img, text, 1, grayscale)
			if visible && found {
				logging.Info("wait_for_text: text %q appeared via %s pass after %v", text, passName, time.Since(startTime))
				return mcp.NewToolResultText(fmt.Sprintf(
					`{"success":true,"text":%q,"visible":true,"waited_ms":%d}`,
					text, time.Since(startTime).Milliseconds(),
				)), nil
			}
			if !visible && !found {
				logging.Info("wait_for_text: text %q disappeared after %v", text, time.Since(startTime))
				return mcp.NewToolResultText(fmt.Sprintf(
					`{"success":true,"text":%q,"visible":false,"waited_ms":%d}`,
					text, time.Since(startTime).Milliseconds(),
				)), nil
			}
		}

		waitForTextSleep(waitForTextPollInterval)
	}

	logging.Info("wait_for_text: timeout waiting for %q (visible=%v)", text, visible)
	return mcp.NewToolResultError(fmt.Sprintf("timeout waiting for text %q (visible=%v) after %dms", text, visible, timeoutMS)), nil
}

// getStringArrayParam extracts a string array parameter from the request
func getStringArrayParam(request mcp.CallToolRequest, name string) ([]string, error) {
	args := request.Params.Arguments
	if args == nil {
		return nil, fmt.Errorf("missing required parameter: %s", name)
	}

	// Type assert to map[string]interface{}
	argsMap, ok := args.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid arguments format")
	}

	val, ok := argsMap[name]
	if !ok {
		return nil, fmt.Errorf("missing required parameter: %s", name)
	}

	// Case 1: Already an array []interface{}
	if arr, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			} else {
				return nil, fmt.Errorf("parameter %s must be an array of strings", name)
			}
		}
		return result, nil
	}

	// Case 2: JSON string that needs parsing
	if str, ok := val.(string); ok {
		var arr []string
		if err := json.Unmarshal([]byte(str), &arr); err != nil {
			return nil, fmt.Errorf("parameter %s must be a valid JSON array string (e.g., [\"Button1\", \"Button2\"]): %v", name, err)
		}
		return arr, nil
	}

	return nil, fmt.Errorf("parameter %s must be an array or JSON array string", name)
}

// findNearbyLabel searches for common label patterns near input fields when the
// exact searchText is not found. It looks for labels ending with ":" or containing
// the searchText as a substring. Returns the bounds of the found label and its text.
func findNearbyLabel(ocrResult *ocr.Result, searchText string) (minX, minY, maxX, maxY int, foundText string, found bool) {
	needle := strings.ToLower(strings.TrimSpace(searchText))
	
	// First try to find labels that contain the search text
	for _, w := range ocrResult.Words {
		wordLower := strings.ToLower(strings.TrimSpace(w.Text))
		if wordLower == "" {
			continue
		}
		// Look for labels ending with colon (common pattern: "Label:")
		if strings.HasSuffix(wordLower, ":") && strings.Contains(wordLower, needle) {
			return w.X, w.Y, w.X + w.Width, w.Y + w.Height, w.Text, true
		}
		// Look for exact substring match
		if strings.Contains(wordLower, needle) && w.Width >= 30 {
			return w.X, w.Y, w.X + w.Width, w.Y + w.Height, w.Text, true
		}
	}
	
	// Try merging adjacent words on the same line to find multi-word labels
	for i, w := range ocrResult.Words {
		phrase := strings.ToLower(strings.TrimSpace(w.Text))
		minX, minY = w.X, w.Y
		maxX, maxY = w.X + w.Width, w.Y + w.Height
		avgHeight := w.Height
		verticalThreshold := avgHeight / 2
		maxHGap := avgHeight // Allow larger gaps for labels
		
		for j := i + 1; j < len(ocrResult.Words) && j < i+5; j++ {
			next := ocrResult.Words[j]
			nextCenterY := next.Y + next.Height/2
			currCenterY := minY + (maxY-minY)/2
			if abs(nextCenterY-currCenterY) > verticalThreshold {
				continue
			}
			hGap := next.X - maxX
			if hGap < 0 || hGap > maxHGap {
				continue
			}
			
			// Extend bounds
			maxX = next.X + next.Width
			if next.Y < minY {
				minY = next.Y
			}
			if next.Y + next.Height > maxY {
				maxY = next.Y + next.Height
			}
			
			phrase += " " + strings.ToLower(strings.TrimSpace(next.Text))
		}
		
		// Check if merged phrase contains our search text
		if strings.Contains(phrase, needle) && len(phrase) > len(needle) {
			// Re-scan to get actual bounds
			for k := i; k < len(ocrResult.Words) && k < i+5; k++ {
				checkWord := ocrResult.Words[k]
				checkPhrase := strings.ToLower(strings.TrimSpace(checkWord.Text))
				if strings.Contains(checkPhrase, needle) {
					// Find all words that form this label
					labelMinX, labelMinY := checkWord.X, checkWord.Y
					labelMaxX, labelMaxY := checkWord.X + checkWord.Width, checkWord.Y + checkWord.Height
					foundText = checkWord.Text
					
					for m := k + 1; m < len(ocrResult.Words) && m < k+5; m++ {
						adjWord := ocrResult.Words[m]
						adjCenterY := adjWord.Y + adjWord.Height/2
						labelCenterY := labelMinY + (labelMaxY-labelMinY)/2
						if abs(adjCenterY-labelCenterY) > verticalThreshold {
							break
						}
						hGap := adjWord.X - labelMaxX
						if hGap < 0 || hGap > maxHGap {
							break
						}
						labelMaxX = adjWord.X + adjWord.Width
						if adjWord.Y < labelMinY {
							labelMinY = adjWord.Y
						}
						if adjWord.Y + adjWord.Height > labelMaxY {
							labelMaxY = adjWord.Y + adjWord.Height
						}
						foundText += " " + adjWord.Text
					}
					return labelMinX, labelMinY, labelMaxX, labelMaxY, foundText, true
				}
			}
		}
	}
	
	return 0, 0, 0, 0, "", false
}

// handleFindClickAndType searches for a bounding box around `text`, calculating
// click target coordinates (applying `x_offset` and `y_offset`), moves mouse,
// clicks, optionally waits, types `type_text`, and optionally presses enter.
func handleFindClickAndType(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling find_click_and_type request")

	searchText, err := getStringParam(request, "text")
	if err != nil {
		logging.Error("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}
	if searchText == "" {
		return mcp.NewToolResultError("text must not be empty"), nil
	}

	typeText, err := getStringParam(request, "type_text")
	if err != nil {
		logging.Error("Invalid type_text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid type_text parameter: %v", err)), nil
	}

	xOffset := 0
	if offset, err := getIntParam(request, "x_offset"); err == nil {
		xOffset = offset
	}
	yOffset := 0
	if offset, err := getIntParam(request, "y_offset"); err == nil {
		yOffset = offset
	}

	pressEnter := getBoolParam(request, "press_enter", false)

	delayMS := 100
	if d, err := getIntParam(request, "delay_ms"); err == nil {
		if d >= 0 && d <= 10000 {
			delayMS = d
		}
	}

	nth := 1
	if n, err := getIntParam(request, "nth"); err == nil {
		if n >= 1 {
			nth = n
		}
	}

	screenW, screenH := robotgo.GetScreenSize()
	regionX, regionY, regionW, regionH := 0, 0, screenW, screenH
	if v, err := getIntParam(request, "x"); err == nil {
		regionX = v
	}
	if v, err := getIntParam(request, "y"); err == nil {
		regionY = v
	}
	if v, err := getIntParam(request, "width"); err == nil {
		regionW = v
	}
	if v, err := getIntParam(request, "height"); err == nil {
		regionH = v
	}

	if err := validate.ScreenRegion(regionX, regionY, regionW, regionH, screenW, screenH); err != nil {
		logging.Error("Screen region validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid screen region: %v", err)), nil
	}

	img, captureErr := robotgo.CaptureImg(regionX, regionY, regionW, regionH)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	saveScreenshotIfKept(img, "ghost-mcp-findclicktype")

	grayscale := getBoolParam(request, "grayscale", true)
	scrollDirection, _ := getStringParam(request, "scroll_direction")
	scrollAmount := 5
	if v, err := getIntParam(request, "scroll_amount"); err == nil {
		if v <= 0 {
			return mcp.NewToolResultError("scroll_amount must be positive"), nil
		}
		scrollAmount = v
	}
	maxScrolls := 0
	if scrollDirection != "" {
		maxScrolls = 8
	}
	if v, err := getIntParam(request, "max_scrolls"); err == nil {
		if v <= 0 {
			return mcp.NewToolResultError("max_scrolls must be positive"), nil
		}
		maxScrolls = v
	}
	if maxScrolls > 0 && scrollDirection == "" {
		return mcp.NewToolResultError("scroll_direction is required when max_scrolls is set"), nil
	}
	scrollX := screenW / 2
	scrollY := screenH / 2
	if v, err := getIntParam(request, "scroll_x"); err == nil {
		scrollX = v
	}
	if v, err := getIntParam(request, "scroll_y"); err == nil {
		scrollY = v
	}

	minX, minY, maxX, maxY, found, passName := parallelFindText(ctx, img, searchText, nth, grayscale)
	scrollCount := 0
	if !found && scrollDirection != "" {
		scrollResult, searchErr := findTextWithScrolling(ctx, scrollSearchConfig{
			SearchText: searchText,
			Direction:  scrollDirection,
			Amount:     scrollAmount,
			MaxScrolls: maxScrolls,
			Nth:        nth,
			ScrollX:    scrollX,
			ScrollY:    scrollY,
			RegionX:    regionX,
			RegionY:    regionY,
			RegionW:    regionW,
			RegionH:    regionH,
			Grayscale:  grayscale,
		})
		if searchErr != nil {
			return mcp.NewToolResultError(searchErr.Error()), nil
		}
		minX, minY, maxX, maxY, found, passName = scrollResult.MinX, scrollResult.MinY, scrollResult.MaxX, scrollResult.MaxY, scrollResult.Found, scrollResult.PassName
		scrollCount = scrollResult.ScrollCount
	}
	
	// If still not found, try to find a nearby label that matches
	labelFound := false
	if !found {
		// Run OCR to get all elements
		ocrResult, ocrErr := ocr.ReadImage(img, ocr.Options{Color: !grayscale})
		if ocrErr == nil && ocrResult != nil {
			labelMinX, labelMinY, labelMaxX, labelMaxY, labelFoundText, labelFoundResult := findNearbyLabel(ocrResult, searchText)
			if labelFoundResult {
				minX, minY, maxX, maxY = labelMinX, labelMinY, labelMaxX, labelMaxY
				found = true
				passName = "label"
				labelFound = true
				logging.Info("find_click_and_type: found label %q for search text %q", labelFoundText, searchText)
			}
		}
	}
	
	if !found {
		logging.Info("find_click_and_type: text %q (occurrence %d) not found", searchText, nth)
		return mcp.NewToolResultError(buildFindTextFailureMessage(img, searchText, nth, regionX, regionY, regionW, regionH, grayscale)), nil
	}

	// Calculate click target - for labels, click to the right and below the label
	cx := regionX + (minX+maxX)/2 + xOffset
	cy := regionY + (minY+maxY)/2 + yOffset
	
	// If we found a label (not the exact text), click to the right of it
	if labelFound && xOffset == 0 && yOffset == 0 {
		cx = regionX + maxX + 50 // Click 50px to the right of the label
		cy = regionY + (minY+maxY)/2
	}

	if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
		logging.Error("Target coordinate out of bounds: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("target coordinate out of bounds: %v", err)), nil
	}

	logging.Info("ACTION: Found %q, calculated target (%d,%d) applying offset (%d,%d)", searchText, cx, cy, xOffset, yOffset)
	robotgo.Move(cx, cy)

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	robotgo.Click("left", false)

	if delayMS > 0 {
		time.Sleep(time.Duration(delayMS) * time.Millisecond)
	}

	if err := validate.Text(typeText); err != nil {
		logging.Error("Text validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text: %v", err)), nil
	}

	displayText := typeText
	if len(displayText) > 50 {
		displayText = displayText[:47] + "..."
	}
	logging.Info("ACTION: Typing text %q", displayText)
	robotgo.TypeStr(typeText)

	if pressEnter {
		logging.Info("ACTION: Pressing enter after typing")
		robotgo.KeyTap("enter")
	}

	finalX, finalY := robotgo.GetMousePos()
	foundText := searchText
	if labelFound {
		foundText = searchText // Keep original search text in response
	}
	logging.Info("ACTION COMPLETE: find_click_and_type %q -> typed %d characters", searchText, len(typeText))

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"clicked_x":%d,"clicked_y":%d,"actual_x":%d,"actual_y":%d,"characters_typed":%d,"enter_pressed":%t,"pass":%q,"scroll_count":%d,"label_found":%t}`,
		foundText, minX, minY, maxX-minX, maxY-minY, cx, cy, finalX, finalY, len(typeText), pressEnter, passName, scrollCount, labelFound,
	)), nil
}

// saveScreenshotIfKept centralizes the logic to write a debug screenshot to disk
// if GHOST_MCP_KEEP_SCREENSHOTS is explicitly enabled. Otherwise, it bypasses disk I/O.
func saveScreenshotIfKept(img image.Image, prefix string) {
	if os.Getenv("GHOST_MCP_KEEP_SCREENSHOTS") == "1" {
		screenshotDir := os.Getenv("GHOST_MCP_SCREENSHOT_DIR")
		if screenshotDir == "" {
			screenshotDir = os.TempDir()
		}
		filename := fmt.Sprintf("%s-%d.png", prefix, time.Now().UnixNano())
		fpath := filepath.Join(screenshotDir, filename)
		if saveErr := robotgo.SavePng(img, fpath); saveErr != nil {
			logging.Error("Failed to keep screenshot: %v", saveErr)
		} else {
			logging.Info("OCR screenshot kept at: %s", fpath)
		}
	}
}

// parallelFindText concurrently executes up to 4 OCR passes (Normal, Inverted, BrightText, Color)
// against the provided image and races them to find the first matching bounding box.
func parallelFindText(ctx context.Context, img image.Image, searchText string, nth int, grayscale bool) (int, int, int, int, bool, string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	prepared, err := prepareParallelOCRImageSet(img, grayscale)
	if err != nil {
		logging.Error("parallelFindText preprocessing failed: %v", err)
		return 0, 0, 0, 0, false, ""
	}

	return parallelFindPreparedText(ctx, prepared, searchText, nth, grayscale)
}

func parallelFindPreparedText(ctx context.Context, prepared *ocr.PreparedImageSet, searchText string, nth int, grayscale bool) (int, int, int, int, bool, string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type match struct {
		minX, minY, maxX, maxY int
		pass                   string
	}
	matches := make(chan match, 4)
	var wg sync.WaitGroup

	runPass := func(imgBytes []byte, name string) {
		defer wg.Done()

		select {
		case <-ctx.Done():
			return // Another pass already found it
		default:
		}

		ocrResult, err := readPreparedOCRImage(imgBytes, ocr.ScaleFactor)
		if err == nil && ocrResult != nil {
			if bMinX, bMinY, bMaxX, bMaxY, bFound := findButtonBounds(ocrResult, searchText, nth); bFound {
				select {
				case matches <- match{bMinX, bMinY, bMaxX, bMaxY, name}:
					cancel() // Stop the other passes
				case <-ctx.Done():
				}
			}
		}
	}

	wg.Add(1)
	go runPass(prepared.Normal, "normal")

	if grayscale {
		wg.Add(3)
		go runPass(prepared.Inverted, "inverted")
		go runPass(prepared.BrightText, "bright-text")
		go runPass(prepared.Color, "color")
	}

	go func() {
		wg.Wait()
		close(matches)
	}()

	if m, ok := <-matches; ok {
		return m.minX, m.minY, m.maxX, m.maxY, true, m.pass
	}
	return 0, 0, 0, 0, false, ""
}
