package main

import (
	"context"
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

	result, ocrErr := ocr.ReadFile(fpath)
	if ocrErr != nil {
		logging.Error("OCR failed: %v", ocrErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", ocrErr)), nil
	}

	logging.Info("OCR extracted %d words", len(result.Words))

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

	ocrResult, ocrErr := ocr.ReadFile(fpath)
	if ocrErr != nil {
		logging.Error("OCR failed: %v", ocrErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", ocrErr)), nil
	}

	// Find the nth word whose text contains searchText (case-insensitive)
	needle := strings.ToLower(searchText)
	matchCount := 0
	for _, w := range ocrResult.Words {
		if strings.Contains(strings.ToLower(w.Text), needle) {
			matchCount++
			if matchCount == nth {
				// OCR coords are relative to the captured region; translate to screen coords.
				cx := regionX + w.X + w.Width/2
				cy := regionY + w.Y + w.Height/2

				if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("found text but center out of bounds: %v", err)), nil
				}

				logging.Info("ACTION: Found %q (occurrence %d) at (%d,%d), clicking center (%d,%d) with %s",
					w.Text, nth, w.X, w.Y, cx, cy, button)
				robotgo.Move(cx, cy)

				if err := checkFailsafe(); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				robotgo.Click(button, false)
				logging.Info("ACTION COMPLETE: find_and_click %q at (%d, %d)", searchText, cx, cy)

				return mcp.NewToolResultText(fmt.Sprintf(
					`{"success":true,"found":%q,"x":%d,"y":%d,"button":%q,"occurrence":%d}`,
					w.Text, cx, cy, button, nth,
				)), nil
			}
		}
	}

	logging.Info("Text %q (occurrence %d) not found on screen", searchText, nth)
	return mcp.NewToolResultError(fmt.Sprintf("text %q not found on screen (occurrence %d, %d words scanned)", searchText, nth, len(ocrResult.Words))), nil
}
