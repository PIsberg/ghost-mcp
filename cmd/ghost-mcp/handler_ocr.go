package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/ocr"
	"github.com/ghost-mcp/internal/validate"
	"github.com/go-vgo/robotgo"
	"github.com/mark3labs/mcp-go/mcp"
)

func handleReadScreenText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling read_screen_text request")

	screenW, screenH := robotgo.GetScreenSize()
	x, _ := getIntParam(request, "x")
	y, _ := getIntParam(request, "y")
	width := screenW
	height := screenH
	if w, err := getIntParam(request, "width"); err == nil {
		width = w
	}
	if h, err := getIntParam(request, "height"); err == nil {
		height = h
	}

	if err := validate.ScreenRegion(x, y, width, height, screenW, screenH); err != nil {
		logging.Error("Screen region validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid screen region: %v", err)), nil
	}

	logging.Info("Capturing screen for OCR at (%d, %d) size %dx%d", x, y, width, height)

	img, captureErr := robotgo.CaptureImg(x, y, width, height)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	// Determine screenshot directory
	screenshotDir := os.Getenv("GHOST_MCP_SCREENSHOT_DIR")
	if screenshotDir == "" {
		screenshotDir = os.TempDir()
	}

	filename := fmt.Sprintf("ghost-mcp-ocr-%d.png", time.Now().UnixNano())
	fpath := filepath.Join(screenshotDir, filename)

	if saveErr := robotgo.SavePng(img, fpath); saveErr != nil {
		logging.Error("Failed to save screenshot for OCR: %v", saveErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save screenshot: %v", saveErr)), nil
	}
	if os.Getenv("GHOST_MCP_KEEP_SCREENSHOTS") != "1" {
		defer os.Remove(fpath)
	} else {
		logging.Info("OCR screenshot kept at: %s", fpath)
	}

	grayscale := getBoolParam(request, "grayscale", true)
	result, ocrErr := ocr.ReadFile(fpath, ocr.Options{Color: !grayscale})
	if ocrErr != nil {
		logging.Error("OCR failed: %v", ocrErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", ocrErr)), nil
	}

	logging.Info("OCR extracted %d words (grayscale=%v)", len(result.Words), grayscale)

	// Build JSON response manually to avoid encoding/json import churn.
	wordsJSON := "["
	for i, w := range result.Words {
		if i > 0 {
			wordsJSON += ","
		}
		wordsJSON += fmt.Sprintf(
			`{"text":%q,"x":%d,"y":%d,"width":%d,"height":%d,"confidence":%.1f}`,
			w.Text, w.X, w.Y, w.Width, w.Height, w.Confidence,
		)
	}
	wordsJSON += "]"

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"text":%q,"words":%s,"region":{"x":%d,"y":%d,"width":%d,"height":%d}}`,
		result.Text, wordsJSON, x, y, width, height,
	)), nil
}

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

	screenshotDir := os.Getenv("GHOST_MCP_SCREENSHOT_DIR")
	if screenshotDir == "" {
		screenshotDir = os.TempDir()
	}

	filename := fmt.Sprintf("ghost-mcp-findclick-%d.png", time.Now().UnixNano())
	fpath := filepath.Join(screenshotDir, filename)

	if saveErr := robotgo.SavePng(img, fpath); saveErr != nil {
		logging.Error("Failed to save screenshot for OCR: %v", saveErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save screenshot: %v", saveErr)), nil
	}
	if os.Getenv("GHOST_MCP_KEEP_SCREENSHOTS") != "1" {
		defer os.Remove(fpath)
	}

	grayscale := getBoolParam(request, "grayscale", true)

	// Pass 1: normal preprocessing (dark text on light backgrounds).
	ocrResult, ocrErr := ocr.ReadFile(fpath, ocr.Options{Color: !grayscale})
	if ocrErr != nil {
		logging.Error("OCR failed: %v", ocrErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", ocrErr)), nil
	}

	if result := findAndClickWord(ocrResult, searchText, nth, regionX, regionY, screenW, screenH, button, request); result != nil {
		return result, nil
	}

	// Pass 2 (inverted): white text on dark/coloured backgrounds becomes dark
	// text on a lighter background — the pattern Tesseract is trained on.
	// CSS buttons with white text on gradient backgrounds are the classic case.
	// Only runs in grayscale mode; colour mode already preserves all info.
	if grayscale {
		logging.Info("find_and_click: %q not found on normal pass, retrying with inverted image", searchText)
		invertedResult, invertedErr := ocr.ReadFile(fpath, ocr.Options{Inverted: true})
		if invertedErr == nil {
			if result := findAndClickWord(invertedResult, searchText, nth, regionX, regionY, screenW, screenH, button, request); result != nil {
				return result, nil
			}
			logging.Info("find_and_click: %q not found on inverted pass either (%d words)", searchText, len(invertedResult.Words))
		}
	}

	// Pass 3 (color mode): disable grayscale to preserve color contrast.
	// Some colored buttons (green/blue/red with white text) are detected better
	// when color information is preserved rather than converted to grayscale.
	if grayscale {
		logging.Info("find_and_click: %q not found on inverted pass, retrying with color mode", searchText)
		colorResult, colorErr := ocr.ReadFile(fpath, ocr.Options{Color: true})
		if colorErr == nil {
			if result := findAndClickWord(colorResult, searchText, nth, regionX, regionY, screenW, screenH, button, request); result != nil {
				return result, nil
			}
			logging.Info("find_and_click: %q not found on color pass either (%d words)", searchText, len(colorResult.Words))
		}
	}

	logging.Info("Text %q (occurrence %d) not found on screen", searchText, nth)
	return mcp.NewToolResultError(fmt.Sprintf("text %q not found on screen (occurrence %d). TIP: Use read_screen_text first to see all detected text with exact coordinates, or set grayscale=false for colored buttons.", searchText, nth)), nil
}

// findButtonBounds finds the full bounding box of a button by merging adjacent
// words that match searchText. This handles multi-word buttons like "Save Changes"
// by returning the combined bounding box of all matching words on the same line.
// Returns the merged bounds relative to the OCR image, or false if not found.
func findButtonBounds(ocrResult *ocr.Result, searchText string, nth int) (minX, minY, maxX, maxY int, found bool) {
	needle := strings.ToLower(searchText)
	matchCount := 0

	for i, w := range ocrResult.Words {
		if !strings.Contains(strings.ToLower(w.Text), needle) {
			continue
		}

		matchCount++
		if matchCount == nth {
			// Start with the matched word's bounds
			minX, minY = w.X, w.Y
			maxX, maxY = w.X+w.Width, w.Y+w.Height

			// Look for adjacent words on the same horizontal line that are part
			// of the same button label. Only merge words that are very close
			// (within typical word spacing, not separate buttons).
			avgHeight := w.Height
			avgWidth := w.Width
			verticalThreshold := avgHeight / 3      // Must be very close vertically
			maxHGap := avgWidth / 2                 // Max gap between words in same label

			// Scan forward to merge adjacent words on same line
			for j := i + 1; j < len(ocrResult.Words); j++ {
				next := ocrResult.Words[j]
				// Check if next word is horizontally aligned (within threshold)
				nextCenterY := next.Y + next.Height/2
				currCenterY := minY + (maxY-minY)/2
				if abs(nextCenterY-currCenterY) > verticalThreshold {
					continue // Not on same line
				}
				// Check if it's close horizontally (within typical word spacing)
				hGap := next.X - maxX
				if hGap >= 0 && hGap <= maxHGap {
					// Merge this word
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
				} else if hGap > maxHGap {
					// Gap is too large - this is a different button/element
					break
				}
			}

			return minX, minY, maxX, maxY, true
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

// findAndClickWord searches ocrResult for the nth case-insensitive occurrence
// of searchText, clicks the center of the full button (merged bounding box for
// multi-word buttons), and returns the MCP result. Returns nil if the text is
// not found (caller should try a different OCR pass or give up).
func findAndClickWord(ocrResult *ocr.Result, searchText string, nth, regionX, regionY, screenW, screenH int, button string, request mcp.CallToolRequest) *mcp.CallToolResult {
	minX, minY, maxX, maxY, found := findButtonBounds(ocrResult, searchText, nth)
	if !found {
		return nil
	}

	// Calculate center of the merged button bounds
	// OCR coords are relative to the captured region; translate to screen coords.
	cx := regionX + (minX+maxX)/2
	cy := regionY + (minY+maxY)/2

	if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
		result := mcp.NewToolResultError(fmt.Sprintf("found text but center out of bounds: %v", err))
		return result
	}

	logging.Info("ACTION: Found %q (occurrence %d) at box (%d,%d)-(%d,%d), clicking center (%d,%d) with %s",
		searchText, nth, minX, minY, maxX, maxY, cx, cy, button)
	robotgo.Move(cx, cy)

	if err := checkFailsafe(); err != nil {
		result := mcp.NewToolResultError(err.Error())
		return result
	}

	robotgo.Click(button, false)
	applyClickDelay(request)

	finalX, finalY := robotgo.GetMousePos()
	if finalX != cx || finalY != cy {
		logging.Info("WARNING: cursor moved after click: requested (%d,%d) actual (%d,%d)", cx, cy, finalX, finalY)
	}
	logging.Info("ACTION COMPLETE: find_and_click %q at (%d, %d)", searchText, finalX, finalY)

	result := mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"requested_x":%d,"requested_y":%d,"actual_x":%d,"actual_y":%d,"button":%q,"occurrence":%d}`,
		searchText, minX, minY, maxX-minX, maxY-minY, cx, cy, finalX, finalY, button, nth,
	))
	return result
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

	screenshotDir := os.Getenv("GHOST_MCP_SCREENSHOT_DIR")
	if screenshotDir == "" {
		screenshotDir = os.TempDir()
	}

	filename := fmt.Sprintf("ghost-mcp-findclickall-%d.png", time.Now().UnixNano())
	fpath := filepath.Join(screenshotDir, filename)

	if saveErr := robotgo.SavePng(img, fpath); saveErr != nil {
		logging.Error("Failed to save screenshot: %v", saveErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save screenshot: %v", saveErr)), nil
	}
	if os.Getenv("GHOST_MCP_KEEP_SCREENSHOTS") != "1" {
		defer os.Remove(fpath)
	}

	// Run OCR once
	ocrResult, ocrErr := ocr.ReadFile(fpath, ocr.Options{Color: true})
	if ocrErr != nil {
		logging.Error("OCR failed: %v", ocrErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", ocrErr)), nil
	}

	// Click each text in sequence
	clicks := make([]map[string]interface{}, 0, len(texts))
	for _, text := range texts {
		minX, minY, maxX, maxY, found := findButtonBounds(ocrResult, text, 1)
		if !found {
			// Try inverted OCR for this text
			invertedResult, invertedErr := ocr.ReadFile(fpath, ocr.Options{Inverted: true})
			if invertedErr == nil {
				minX, minY, maxX, maxY, found = findButtonBounds(invertedResult, text, 1)
			}
		}

		if !found {
			logging.Info("find_and_click_all: text %q not found, stopping", text)
			return mcp.NewToolResultError(fmt.Sprintf("text %q not found on screen", text)), nil
		}

		cx := (minX + maxX) / 2
		cy := (minY + maxY) / 2

		logging.Info("ACTION: Clicking %q at (%d, %d)", text, cx, cy)
		robotgo.Move(cx, cy)
		time.Sleep(10 * time.Millisecond) // Small delay for mouse movement
		robotgo.Click(button, false)

		if delayMS > 0 {
			time.Sleep(time.Duration(min(delayMS, 10000)) * time.Millisecond)
		}

		finalX, finalY := robotgo.GetMousePos()
		clicks = append(clicks, map[string]interface{}{
			"text":        text,
			"box":         map[string]int{"x": minX, "y": minY, "width": maxX - minX, "height": maxY - minY},
			"clicked_x":   cx,
			"clicked_y":   cy,
			"actual_x":    finalX,
			"actual_y":    finalY,
			"button":      button,
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
	intervalMS := 500 // Check every 500ms

	for time.Since(startTime) < time.Duration(timeoutMS)*time.Millisecond {
		img, captureErr := robotgo.CaptureImg(regionX, regionY, regionW, regionH)
		if captureErr == nil {
			ocrResult, ocrErr := ocr.ReadImage(img, ocr.Options{})
			if ocrErr == nil {
				_, _, _, _, found := findButtonBounds(ocrResult, text, 1)
				if visible && found {
					logging.Info("wait_for_text: text %q appeared after %v", text, time.Since(startTime))
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
		}

		time.Sleep(time.Duration(intervalMS) * time.Millisecond)
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
