// ghost-mcp: MCP Server for OS-level UI automation
//
// This server exposes mouse, keyboard, and screen reading capabilities
// as MCP tools that AI clients can use to control legacy applications.
//
// CRITICAL: All logging MUST go to stderr because stdout is used for
// the MCP JSON-RPC protocol. Writing to stdout will corrupt the protocol.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/ghost-mcp/internal/audit"
	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/ocr"
	"github.com/ghost-mcp/internal/transport"
	"github.com/ghost-mcp/internal/validate"
	"github.com/ghost-mcp/internal/visual"
	"github.com/go-vgo/robotgo"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	FailsafeX = 0
	FailsafeY = 0
)

var (
	scrollCaptureImage = func(x, y, width, height int) (image.Image, error) {
		return robotgo.CaptureImg(x, y, width, height)
	}
	scrollReadImage = ocr.ReadImage
	scrollSleep     = time.Sleep
)

const (
	ServerName    = "ghost-mcp"
	ServerVersion = "1.0.0"
)

// TokenEnvVar is the environment variable for the authentication token.
const TokenEnvVar = "GHOST_MCP_TOKEN"

const scrollToolDescription = `Move the mouse to (x, y) and scroll the mouse wheel. Use for scrolling lists, pages, and dropdowns.

Scroll 'down' to reveal content below, 'up' to go back up. For horizontal content use 'left' or 'right'.

The response includes visible_text plus smart feedback:
- viewport_changed: false means the scroll likely did not reveal anything new
- boundary_likely: true means you probably hit the top/bottom of the page
- text_found: true means optional search_text is already visible after the scroll

AMOUNT GUIDANCE — use small increments to avoid overshooting:
- amount=3 (default): ~1/4 screen — use for fine positioning
- amount=5: ~1/2 screen — use to reveal the next section
- amount=10: ~full screen — jumps far, easy to overshoot; only use for large pages

SEARCH WORKFLOW:
- Prefer scroll_until_text for page search tasks
- If the target text is not currently visible, do NOT start with find_elements or take_screenshot; try scroll_until_text first
- If using scroll directly, keep amount at 3-5 while searching
- Do not keep alternating up/down unless you are backtracking one step after overshooting
- Stop when boundary_likely=true or viewport_changed=false

x and y are optional and default to the screen centre, which is correct for most page scrolling. Only specify them when scrolling a specific widget (e.g. a side panel or dropdown list).`

const scrollUntilTextToolDescription = `Search a long page more intelligently by scrolling in bounded steps until text becomes visible.

Use this first when you are hunting for a label, button, input placeholder, or section title that is probably off-screen above or below the current viewport.

DO NOT do this first:
- find_elements on the full page
- take_screenshot of the full page
- repeated manual scroll + screenshot loops

HOW IT STOPS:
- found: the requested text is visible in the post-scroll OCR
- boundary: scrolling stopped changing the viewport, so you likely hit the page edge
- max_steps: the search budget was exhausted

SMART DEFAULTS:
- direction=down
- amount=5
- max_steps=8

GUIDANCE:
- Keep amount at 3-5 for search. Larger jumps overshoot and create oscillation.
- Use x/y only when scrolling a specific panel or dropdown; otherwise omit them.
- If this returns found=false with stop_reason=boundary, do not keep scrolling in the same direction.
- If this finds the target, use the returned match_box/requested_x/requested_y for the next click or focus step before doing broader rediscovery.`

const screenshotToolDescription = `Capture the screen and return it as an image.

🚫 DO NOT take a screenshot before clicking — use find_and_click instead.
🚫 DO NOT take a screenshot after every click to verify — use wait_for_text instead.
🚫 DO NOT take a screenshot to find a button's coordinates — use find_and_click or find_elements instead.
🚫 DO NOT use screenshots as the first step for long-page text search — use scroll_until_text first.

WHEN TO USE take_screenshot:
- The task explicitly requires seeing the visual layout (e.g. "describe the screen", "what color is the button").
- The target has no text (icon, image, progress bar) so OCR cannot locate it.
- Debugging: scroll_until_text, find_and_click, or find_elements returned unexpected results and you need to see what is on screen.

SPEED TIPS:
- Use quality=85 (JPEG) for a ~10× smaller image — much faster for the model to process.
- Use region parameters (x, y, width, height) to capture only the relevant area.`

// =============================================================================
// GLOBAL STATE
// =============================================================================

type serverState struct {
	shutdownChan   chan struct{}
	isShuttingDown bool
}

var state = &serverState{
	shutdownChan: make(chan struct{}),
}

// =============================================================================
// FAILSAFE
// =============================================================================

func checkFailsafe() error {
	x, y := robotgo.GetMousePos()
	if x == FailsafeX && y == FailsafeY {
		logging.Error("FAILSAFE TRIGGERED: Mouse at (%d, %d). Initiating shutdown.", x, y)
		initiateShutdown()
		return fmt.Errorf("failsafe triggered: mouse at origin (%d, %d)", x, y)
	}
	return nil
}

func initiateShutdown() {
	if state.isShuttingDown {
		return
	}
	state.isShuttingDown = true
	logging.Info("Initiating graceful shutdown...")
	close(state.shutdownChan)
}

// =============================================================================
// MCP TOOL HANDLERS
// =============================================================================

func handleGetScreenSize(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling get_screen_size request")
	width, height := robotgo.GetScreenSize()
	scale := getDPIScale()
	logging.Info("Screen size: %dx%d, DPI scale: %.2f", width, height, scale)
	return mcp.NewToolResultText(fmt.Sprintf(
		`{"width": %d, "height": %d, "scale_factor": %.2f}`,
		width, height, scale,
	)), nil
}

func handleMoveMouse(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling move_mouse request")

	x, err := getIntParam(request, "x")
	if err != nil {
		logging.Error("Invalid x parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid x parameter: %v", err)), nil
	}
	y, err := getIntParam(request, "y")
	if err != nil {
		logging.Error("Invalid y parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid y parameter: %v", err)), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		logging.Error("Coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	// Log current position
	currentX, currentY := robotgo.GetMousePos()
	logging.Info("ACTION: Moving mouse from (%d, %d) to (%d, %d)", currentX, currentY, x, y)

	robotgo.Move(x, y)

	if os.Getenv("GHOST_MCP_VISUAL") == "1" {
		visual.PulseCursor(x, y)
	}

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	finalX, finalY := robotgo.GetMousePos()
	logging.Info("ACTION COMPLETE: Mouse now at (%d, %d)", finalX, finalY)
	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "x": %d, "y": %d}`, finalX, finalY)), nil
}

func handleClick(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling click request")

	button, err := getStringParam(request, "button")
	if err != nil {
		logging.Error("Invalid button parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid button parameter: %v", err)), nil
	}

	validButtons := map[string]bool{"left": true, "right": true, "middle": true}
	if !validButtons[button] {
		err := fmt.Errorf("invalid button '%s', must be 'left', 'right', or 'middle'", button)
		logging.Error("%v", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	x, y := robotgo.GetMousePos()
	logging.Info("ACTION: Performing %s click at (%d, %d)", button, x, y)

	// Show visual feedback if enabled
	if os.Getenv("GHOST_MCP_VISUAL") == "1" {
		visual.PulseCursor(x, y)
	}

	robotgo.Click(button, false)
	applyClickDelay(request)

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	finalX, finalY := robotgo.GetMousePos()
	logging.Info("ACTION COMPLETE: %s click executed at (%d, %d)", button, finalX, finalY)
	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "button": "%s", "x": %d, "y": %d}`, button, finalX, finalY)), nil
}

func handleClickAt(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling click_at request")

	x, err := getIntParam(request, "x")
	if err != nil {
		logging.Error("Invalid x parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid x parameter: %v", err)), nil
	}
	y, err := getIntParam(request, "y")
	if err != nil {
		logging.Error("Invalid y parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid y parameter: %v", err)), nil
	}

	button, err := getStringParam(request, "button")
	if err != nil {
		button = "left"
	}

	validButtons := map[string]bool{"left": true, "right": true, "middle": true}
	if !validButtons[button] {
		err := fmt.Errorf("invalid button '%s', must be 'left', 'right', or 'middle'", button)
		logging.Error("%v", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		logging.Error("Coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	logging.Info("ACTION: Moving mouse to (%d, %d) for %s click", x, y, button)
	robotgo.Move(x, y)

	if os.Getenv("GHOST_MCP_VISUAL") == "1" {
		visual.PulseCursor(x, y)
	}

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	robotgo.Click(button, false)
	applyClickDelay(request)

	finalX, finalY := robotgo.GetMousePos()
	if finalX != x || finalY != y {
		logging.Info("WARNING: cursor moved after click: requested (%d,%d) actual (%d,%d)", x, y, finalX, finalY)
	}
	logging.Info("ACTION COMPLETE: %s click at (%d, %d)", button, finalX, finalY)

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "button": "%s", "requested_x": %d, "requested_y": %d, "actual_x": %d, "actual_y": %d}`,
		button, x, y, finalX, finalY,
	)), nil
}

func handleDoubleClick(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling double_click request")

	x, err := getIntParam(request, "x")
	if err != nil {
		logging.Error("Invalid x parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid x parameter: %v", err)), nil
	}
	y, err := getIntParam(request, "y")
	if err != nil {
		logging.Error("Invalid y parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid y parameter: %v", err)), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		logging.Error("Coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	logging.Info("ACTION: Moving mouse to (%d, %d) for double-click", x, y)
	robotgo.Move(x, y)

	if os.Getenv("GHOST_MCP_VISUAL") == "1" {
		visual.PulseCursor(x, y)
	}

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	robotgo.Click("left", true)
	applyClickDelay(request)

	finalX, finalY := robotgo.GetMousePos()
	if finalX != x || finalY != y {
		logging.Info("WARNING: cursor moved after double-click: requested (%d,%d) actual (%d,%d)", x, y, finalX, finalY)
	}
	logging.Info("ACTION COMPLETE: Double-click at (%d, %d)", x, y)

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "requested_x": %d, "requested_y": %d, "actual_x": %d, "actual_y": %d}`,
		x, y, finalX, finalY,
	)), nil
}

func handleScroll(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling scroll request")

	screenW, screenH := robotgo.GetScreenSize()

	// x and y are optional — default to screen centre so callers can omit them
	// for standard page scrolling.
	x := screenW / 2
	y := screenH / 2
	if v, err := getIntParam(request, "x"); err == nil {
		x = v
	}
	if v, err := getIntParam(request, "y"); err == nil {
		y = v
	}

	direction, err := getStringParam(request, "direction")
	if err != nil {
		logging.Error("Invalid direction parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid direction parameter: %v", err)), nil
	}

	validDirections := map[string]bool{"up": true, "down": true, "left": true, "right": true}
	if !validDirections[direction] {
		err := fmt.Errorf("invalid direction '%s', must be 'up', 'down', 'left', or 'right'", direction)
		logging.Error("%v", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	amount := 3
	if a, err := getIntParam(request, "amount"); err == nil {
		if a <= 0 {
			return mcp.NewToolResultError("amount must be positive"), nil
		}
		amount = a
	}

	searchText := ""
	if s, err := getStringParam(request, "search_text"); err == nil {
		searchText = s
	}

	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		logging.Error("Coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	logging.Info("ACTION: Moving mouse to (%d, %d) then scrolling %s by %d", x, y, direction, amount)
	robotgo.Move(x, y)

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	feedback, err := performScrollAndAnalyze(screenW, screenH, direction, amount, searchText)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	logging.Info(
		"ACTION COMPLETE: Scrolled %s by %d at (%d, %d); viewport_changed=%t boundary_likely=%t text_found=%t",
		direction, amount, x, y, feedback.ViewportChanged, feedback.BoundaryLikely, feedback.TextFound,
	)

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "x": %d, "y": %d, "direction": %q, "amount": %d, "search_text": %q, "text_found": %t, "visible_text": %q, "visible_text_hash": %q, "viewport_changed": %t, "boundary_likely": %t}`,
		x, y, direction, amount, searchText, feedback.TextFound, feedback.VisibleText, feedback.VisibleTextHash, feedback.ViewportChanged, feedback.BoundaryLikely,
	)), nil
}

type scrollOCRSnapshot struct {
	VisibleText string
	Normalized  string
	Hash        string
	OCRResult   *ocr.Result
}

type scrollMatch struct {
	BoxX       int
	BoxY       int
	BoxWidth   int
	BoxHeight  int
	RequestedX int
	RequestedY int
}

type scrollFeedback struct {
	VisibleText     string
	VisibleTextHash string
	ViewportChanged bool
	BoundaryLikely  bool
	TextFound       bool
}

func normalizeVisibleText(text string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r):
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func hashVisibleText(text string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return fmt.Sprintf("%016x", h.Sum64())
}

func captureScrollSnapshot(screenW, screenH int) scrollOCRSnapshot {
	stripY := screenH / 4
	stripH := screenH / 2
	if img, captureErr := scrollCaptureImage(0, stripY, screenW, stripH); captureErr == nil {
		if ocrResult, ocrErr := scrollReadImage(img, ocr.Options{}); ocrErr == nil {
			normalized := normalizeVisibleText(ocrResult.Text)
			return scrollOCRSnapshot{
				VisibleText: ocrResult.Text,
				Normalized:  normalized,
				Hash:        hashVisibleText(normalized),
				OCRResult:   ocrResult,
			}
		} else {
			logging.Debug("scroll OCR failed (non-fatal): %v", ocrErr)
		}
	} else {
		logging.Debug("scroll capture failed (non-fatal): %v", captureErr)
	}
	return scrollOCRSnapshot{}
}

func buildScrollFeedback(before, after scrollOCRSnapshot, direction, searchText string) scrollFeedback {
	viewportChanged := before.Hash != after.Hash
	boundaryLikely := (direction == "up" || direction == "down") && !viewportChanged && after.Normalized != ""
	textFound := false
	if searchText != "" && after.Normalized != "" {
		textFound = strings.Contains(after.Normalized, normalizeVisibleText(searchText))
	}
	return scrollFeedback{
		VisibleText:     after.VisibleText,
		VisibleTextHash: after.Hash,
		ViewportChanged: viewportChanged,
		BoundaryLikely:  boundaryLikely,
		TextFound:       textFound,
	}
}

func findScrollMatch(snapshot scrollOCRSnapshot, searchText string) (scrollMatch, bool) {
	if snapshot.OCRResult == nil || searchText == "" {
		return scrollMatch{}, false
	}
	needle := normalizeVisibleText(searchText)
	if needle == "" {
		return scrollMatch{}, false
	}
	tokens := strings.Fields(needle)
	anchor := needle
	if len(tokens) > 0 {
		anchor = tokens[0]
	}
	minX, minY, maxX, maxY, found := findButtonBounds(snapshot.OCRResult, anchor, 1)
	if !found {
		return scrollMatch{}, false
	}
	return scrollMatch{
		BoxX:       minX,
		BoxY:       minY,
		BoxWidth:   maxX - minX,
		BoxHeight:  maxY - minY,
		RequestedX: (minX + maxX) / 2,
		RequestedY: (minY + maxY) / 2,
	}, true
}

func performScrollAndAnalyze(screenW, screenH int, direction string, amount int, searchText string) (scrollFeedback, error) {
	before := captureScrollSnapshot(screenW, screenH)
	robotgo.ScrollDir(amount, direction)
	after := captureScrollSnapshot(screenW, screenH)
	return buildScrollFeedback(before, after, direction, searchText), nil
}

func handleScrollUntilText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling scroll_until_text request")

	text, err := getStringParam(request, "text")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	x := screenW / 2
	y := screenH / 2
	if v, err := getIntParam(request, "x"); err == nil {
		x = v
	}
	if v, err := getIntParam(request, "y"); err == nil {
		y = v
	}
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	direction := "down"
	if v, err := getStringParam(request, "direction"); err == nil {
		direction = v
	}
	if direction != "up" && direction != "down" {
		return mcp.NewToolResultError("direction must be 'up' or 'down'"), nil
	}

	amount := 5
	if v, err := getIntParam(request, "amount"); err == nil {
		if v <= 0 {
			return mcp.NewToolResultError("amount must be positive"), nil
		}
		amount = v
	}
	maxSteps := 8
	if v, err := getIntParam(request, "max_steps"); err == nil {
		if v <= 0 {
			return mcp.NewToolResultError("max_steps must be positive"), nil
		}
		maxSteps = v
	}
	delayMS := 150
	if v, err := getIntParam(request, "delay_ms"); err == nil {
		if v < 0 {
			return mcp.NewToolResultError("delay_ms must be non-negative"), nil
		}
		delayMS = v
	}

	initial := captureScrollSnapshot(screenW, screenH)
	if strings.Contains(initial.Normalized, normalizeVisibleText(text)) {
		match, hasMatch := findScrollMatch(initial, text)
		return mcp.NewToolResultText(fmt.Sprintf(
			`{"success": true, "found": true, "text": %q, "direction": %q, "amount": %d, "steps_taken": 0, "max_steps": %d, "stop_reason": "already_visible", "boundary_likely": false, "visible_text": %q, "visible_text_hash": %q, "match_found": %t, "match_box": {"x": %d, "y": %d, "width": %d, "height": %d}, "requested_x": %d, "requested_y": %d}`,
			text, direction, amount, maxSteps, initial.VisibleText, initial.Hash, hasMatch, match.BoxX, match.BoxY, match.BoxWidth, match.BoxHeight, match.RequestedX, match.RequestedY,
		)), nil
	}

	lastFeedback := scrollFeedback{
		VisibleText:     initial.VisibleText,
		VisibleTextHash: initial.Hash,
	}
	lastSnapshot := initial
	stopReason := "max_steps"
	for step := 1; step <= maxSteps; step++ {
		robotgo.Move(x, y)
		if err := checkFailsafe(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		before := captureScrollSnapshot(screenW, screenH)
		robotgo.ScrollDir(amount, direction)
		lastSnapshot = captureScrollSnapshot(screenW, screenH)
		lastFeedback = buildScrollFeedback(before, lastSnapshot, direction, text)
		if lastFeedback.TextFound {
			stopReason = "found"
			match, hasMatch := findScrollMatch(lastSnapshot, text)
			return mcp.NewToolResultText(fmt.Sprintf(
				`{"success": true, "found": true, "text": %q, "direction": %q, "amount": %d, "steps_taken": %d, "max_steps": %d, "stop_reason": %q, "boundary_likely": %t, "visible_text": %q, "visible_text_hash": %q, "match_found": %t, "match_box": {"x": %d, "y": %d, "width": %d, "height": %d}, "requested_x": %d, "requested_y": %d}`,
				text, direction, amount, step, maxSteps, stopReason, lastFeedback.BoundaryLikely, lastFeedback.VisibleText, lastFeedback.VisibleTextHash, hasMatch, match.BoxX, match.BoxY, match.BoxWidth, match.BoxHeight, match.RequestedX, match.RequestedY,
			)), nil
		}
		if lastFeedback.BoundaryLikely {
			stopReason = "boundary"
			return mcp.NewToolResultText(fmt.Sprintf(
				`{"success": true, "found": false, "text": %q, "direction": %q, "amount": %d, "steps_taken": %d, "max_steps": %d, "stop_reason": %q, "boundary_likely": true, "visible_text": %q, "visible_text_hash": %q}`,
				text, direction, amount, step, maxSteps, stopReason, lastFeedback.VisibleText, lastFeedback.VisibleTextHash,
			)), nil
		}
		if delayMS > 0 {
			scrollSleep(time.Duration(delayMS) * time.Millisecond)
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "found": false, "text": %q, "direction": %q, "amount": %d, "steps_taken": %d, "max_steps": %d, "stop_reason": %q, "boundary_likely": %t, "visible_text": %q, "visible_text_hash": %q}`,
		text, direction, amount, maxSteps, maxSteps, stopReason, lastFeedback.BoundaryLikely, lastFeedback.VisibleText, lastFeedback.VisibleTextHash,
	)), nil
}

func handleTypeText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling type_text request")

	text, err := getStringParam(request, "text")
	if err != nil {
		logging.Error("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}

	if err := validate.Text(text); err != nil {
		logging.Error("Text validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text: %v", err)), nil
	}

	pressEnter := getBoolParam(request, "press_enter", false)

	// Truncate long text for logging
	displayText := text
	if len(text) > 50 {
		displayText = text[:47] + "..."
	}
	logging.Info("ACTION: Typing text: %q", displayText)
	robotgo.TypeStr(text)

	if pressEnter {
		logging.Info("ACTION: Pressing enter after typing")
		robotgo.KeyTap("enter")
	}

	logging.Info("ACTION COMPLETE: Typed %d characters", len(text))
	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "characters_typed": %d, "enter_pressed": %t}`, len(text), pressEnter)), nil
}

func handleClickAndType(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling click_and_type request")

	x, err := getIntParam(request, "x")
	if err != nil {
		logging.Error("Invalid x parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid x parameter: %v", err)), nil
	}
	y, err := getIntParam(request, "y")
	if err != nil {
		logging.Error("Invalid y parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid y parameter: %v", err)), nil
	}

	text, err := getStringParam(request, "text")
	if err != nil {
		logging.Error("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
	}

	if err := validate.Text(text); err != nil {
		logging.Error("Text validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text: %v", err)), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		logging.Error("Coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	pressEnter := getBoolParam(request, "press_enter", false)

	logging.Info("ACTION: Moving mouse to (%d, %d) for click and type", x, y)
	robotgo.Move(x, y)

	if os.Getenv("GHOST_MCP_VISUAL") == "1" {
		visual.PulseCursor(x, y)
	}

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	robotgo.Click("left", false)
	applyClickDelay(request)

	displayText := text
	if len(text) > 50 {
		displayText = text[:47] + "..."
	}
	logging.Info("ACTION: Typing text: %q", displayText)
	robotgo.TypeStr(text)

	if pressEnter {
		logging.Info("ACTION: Pressing enter after typing")
		robotgo.KeyTap("enter")
	}

	finalX, finalY := robotgo.GetMousePos()
	logging.Info("ACTION COMPLETE: Click and type at (%d, %d)", finalX, finalY)

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "x": %d, "y": %d, "characters_typed": %d, "enter_pressed": %t}`,
		finalX, finalY, len(text), pressEnter,
	)), nil
}

func handlePressKey(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling press_key request")

	key, err := getStringParam(request, "key")
	if err != nil {
		logging.Error("Invalid key parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid key parameter: %v", err)), nil
	}

	if err := validate.Key(key); err != nil {
		logging.Error("Key validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid key: %v", err)), nil
	}

	logging.Info("ACTION: Pressing key: %s", key)
	robotgo.KeyTap(key)
	logging.Info("ACTION COMPLETE: Key %s pressed", key)
	return mcp.NewToolResultText(fmt.Sprintf(`{"success": true, "key": "%s"}`, key)), nil
}

func handleTakeScreenshot(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling take_screenshot request")

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
		logging.Error("Screenshot region validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid screenshot region: %v", err)), nil
	}

	// quality=0 → PNG (lossless). quality=1–100 → JPEG at that quality.
	// JPEG 85 is typically 10× smaller than PNG for screen content, significantly
	// reducing the number of tokens the model processes and cutting transfer time.
	quality, _ := getIntParam(request, "quality")
	if quality < 0 {
		quality = 0
	}
	if quality > 100 {
		quality = 100
	}

	logging.Info("Taking screenshot at (%d, %d) size %dx%d quality=%d", x, y, width, height, quality)

	img, captureErr := robotgo.CaptureImg(x, y, width, height)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	// Encode directly into a memory buffer — no temp file write or read.
	// Previous pipeline: SavePng→disk, ReadFile←disk, Remove added ~200–400 ms
	// of unnecessary file I/O on every screenshot call.
	var buf bytes.Buffer
	var mimeType string

	if quality > 0 {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			logging.Error("Failed to encode screenshot as JPEG: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to encode screenshot: %v", err)), nil
		}
		mimeType = "image/jpeg"
	} else {
		// BestSpeed (level 1) is significantly faster than the default (level 6)
		// with only a modest increase in file size — the right trade-off for
		// interactive screenshots where the image is consumed immediately.
		enc := &png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&buf, img); err != nil {
			logging.Error("Failed to encode screenshot as PNG: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("failed to encode screenshot: %v", err)), nil
		}
		mimeType = "image/png"
	}

	logging.Info("Screenshot encoded: %s %d bytes", mimeType, buf.Len())

	// Save to disk only when explicitly requested for debugging.
	if os.Getenv("GHOST_MCP_KEEP_SCREENSHOTS") == "1" {
		screenshotDir := os.Getenv("GHOST_MCP_SCREENSHOT_DIR")
		if screenshotDir == "" {
			screenshotDir = os.TempDir()
		}
		if mkErr := os.MkdirAll(screenshotDir, 0755); mkErr == nil {
			ext := "png"
			if quality > 0 {
				ext = "jpg"
			}
			fpath := filepath.Join(screenshotDir, fmt.Sprintf("ghost-mcp-screenshot-%d.%s", time.Now().UnixNano(), ext))
			if writeErr := os.WriteFile(fpath, buf.Bytes(), 0644); writeErr == nil {
				logging.Info("Screenshot kept at: %s", fpath)
			}
		}
	}

	return mcp.NewToolResultImage(
		fmt.Sprintf(`{"success": true, "width": %d, "height": %d, "format": %q, "bytes": %d}`, width, height, mimeType, buf.Len()),
		base64.StdEncoding.EncodeToString(buf.Bytes()),
		mimeType,
	), nil
}

// =============================================================================
// CLICK DELAY HELPER
// =============================================================================

// defaultClickDelayMs is the default post-click pause.
// Browsers and most apps need a few milliseconds to process a click event
// and update the DOM/UI before a screenshot would reflect the change.
// 100 ms is imperceptible to the user but enough for virtually all UIs.
const defaultClickDelayMs = 100

// applyClickDelay sleeps for the caller-requested delay (delay_ms param).
// Falls back to defaultClickDelayMs when the parameter is absent.
// Set delay_ms=0 to skip the delay entirely for latency-sensitive flows.
func applyClickDelay(request mcp.CallToolRequest) {
	ms := defaultClickDelayMs
	if v, err := getIntParam(request, "delay_ms"); err == nil {
		if v >= 0 && v <= 10000 {
			ms = v
		}
	}
	if ms > 0 {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

// =============================================================================
// PARAMETER EXTRACTION
// =============================================================================

func getStringParam(request mcp.CallToolRequest, name string) (string, error) {
	val, ok := request.GetArguments()[name]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", name)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", name)
	}
	return str, nil
}

func getIntParam(request mcp.CallToolRequest, name string) (int, error) {
	val, ok := request.GetArguments()[name]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: %s", name)
	}
	if f, ok := val.(float64); ok {
		if f != math.Trunc(f) {
			return 0, fmt.Errorf("parameter %s must be a whole number, got %v", name, f)
		}
		return int(f), nil
	}
	if i, ok := val.(int); ok {
		return i, nil
	}
	if i, ok := val.(int64); ok {
		return int(i), nil
	}
	return 0, fmt.Errorf("parameter %s must be an integer", name)
}

// getBoolParam reads a boolean parameter. If the parameter is absent, it
// returns defaultVal. JSON booleans arrive as bool; the function also handles
// the string literals "true"/"false" for robustness.
func getBoolParam(request mcp.CallToolRequest, name string, defaultVal bool) bool {
	val, ok := request.GetArguments()[name]
	if !ok {
		return defaultVal
	}
	if b, ok := val.(bool); ok {
		return b
	}
	if s, ok := val.(string); ok {
		return s == "true"
	}
	return defaultVal
}

// =============================================================================
// TOKEN AUTHENTICATION
// =============================================================================

func validateStartupToken() (string, error) {
	token := os.Getenv(TokenEnvVar)
	if token == "" {
		return "", fmt.Errorf("%s environment variable is not set", TokenEnvVar)
	}
	return token, nil
}

func makeTokenValidator(expectedToken string, al *audit.Logger) func(ctx context.Context, id any, message any) error {
	return func(ctx context.Context, id any, message any) error {
		if os.Getenv(TokenEnvVar) != expectedToken {
			logging.Error("Authentication failed: %s mismatch or missing", TokenEnvVar)
			al.Log(audit.EventAuthFailure, "", fmt.Sprintf("invalid or missing %s", TokenEnvVar), nil)
			return fmt.Errorf("%w: invalid or missing %s", audit.ErrAuthFailed, TokenEnvVar)
		}
		return nil
	}
}

// =============================================================================
// SERVER SETUP
// =============================================================================

func createServer(token string, al *audit.Logger) *server.MCPServer {
	logging.Info("Creating MCP server: %s v%s", ServerName, ServerVersion)

	hooks := &server.Hooks{}
	hooks.AddOnRequestInitialization(makeTokenValidator(token, al))
	audit.RegisterHooks(hooks, al)

	mcpServer := server.NewMCPServer(
		ServerName,
		ServerVersion,
		server.WithResourceCapabilities(true, true),
		server.WithHooks(hooks),
	)
	registerTools(mcpServer)
	return mcpServer
}

func registerTools(mcpServer *server.MCPServer) {
	logging.Info("Registering tools...")

	mcpServer.AddTool(mcp.NewTool("get_screen_size",
		mcp.WithDescription(`Get the screen resolution and DPI scale factor.

Returns {width, height, scale_factor}.

- width / height: screen dimensions in logical pixels. All ghost-mcp coordinates (mouse, screenshots, OCR) use this same logical pixel space, so you do NOT need to apply the scale factor yourself.
- scale_factor: the OS display scaling ratio (e.g. 1.5 = 150% / "High DPI"). Informational only — useful to understand why an app's own coordinate reporter (e.g. a browser's window.devicePixelRatio or a game's cursor position) might differ from ghost-mcp coordinates.

(0,0) is the top-left corner. Only call this when you specifically need the screen dimensions — most tasks do not require it.`),
	), handleGetScreenSize)

	mcpServer.AddTool(mcp.NewTool("move_mouse",
		mcp.WithDescription(`Move the mouse cursor to absolute screen coordinates. (0,0) is the top-left corner.

⚠️ NOT FOR CLICKING BUTTONS: If you want to click a button or link by its text label, use find_and_click instead — it locates the target by OCR and clicks in one call with no coordinate guessing.

Use move_mouse only when you already have exact coordinates (e.g. from find_elements center_x/center_y) and need to hover before a click, or when dragging.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels from the left edge of the screen. Must be within screen bounds."), mcp.Required()),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels from the top edge of the screen. Must be within screen bounds."), mcp.Required()),
	), handleMoveMouse)

	mcpServer.AddTool(mcp.NewTool("click",
		mcp.WithDescription(`Click the mouse button at the current cursor position. Call move_mouse first to position the cursor.

⚠️ NOT FOR CLICKING BUTTONS BY LABEL: Use find_and_click instead — it locates and clicks in one call without needing move_mouse first.

Use this tool only when you have already moved the mouse to exact coordinates and need to click at the current position (e.g. after a drag, or a hover-then-click sequence). Use right-click to open context menus.`),
		mcp.WithString("button", mcp.Description("Mouse button to click: 'left' for normal clicks and selecting items, 'right' for context menus, 'middle' for middle-click."), mcp.Required()),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after the click for the UI to update (default: 100). Set to 0 to skip. Max: 10000.")),
	), handleClick)

	mcpServer.AddTool(mcp.NewTool("click_at",
		mcp.WithDescription(`Move the mouse to (x, y) and click in one atomic operation. Preferred over separate move_mouse + click calls.

⚠️ NOT FOR CLICKING BUTTONS BY LABEL: Use find_and_click instead — it finds the text on screen and clicks without needing you to supply coordinates.

Use click_at only when you already have exact pixel coordinates (e.g. center_x/center_y from find_elements, or a known fixed coordinate). Do not guess coordinates — guessing will miss.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels from the left edge of the screen."), mcp.Required()),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels from the top edge of the screen."), mcp.Required()),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after the click for the UI to update (default: 100). Set to 0 to skip. Max: 10000.")),
	), handleClickAt)

	mcpServer.AddTool(mcp.NewTool("double_click",
		mcp.WithDescription(`Move the mouse to (x, y) and perform a double-click. Use for opening files, activating items, or any UI that requires double-click.

Use coordinates from find_elements (center_x/center_y) or a known fixed position. Do not guess coordinates.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels from the left edge of the screen."), mcp.Required()),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels from the top edge of the screen."), mcp.Required()),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after the click for the UI to update (default: 100). Set to 0 to skip. Max: 10000.")),
	), handleDoubleClick)

	mcpServer.AddTool(mcp.NewTool("scroll",
		mcp.WithDescription(scrollToolDescription),
		mcp.WithNumber("x", mcp.Description("X coordinate to scroll at (pixels from left edge). Defaults to screen centre.")),
		mcp.WithNumber("y", mcp.Description("Y coordinate to scroll at (pixels from top edge). Defaults to screen centre.")),
		mcp.WithString("direction", mcp.Description("Scroll direction: 'up', 'down', 'left', or 'right'."), mcp.Required()),
		mcp.WithNumber("amount", mcp.Description("Number of scroll steps (default: 3 ≈ 1/4 screen). amount=5 ≈ half screen. amount=10 jumps far — avoid for precise navigation.")),
		mcp.WithString("search_text", mcp.Description("Optional text to look for in the post-scroll OCR. When provided, the response includes text_found=true if that text is visible.")),
	), handleScroll)

	mcpServer.AddTool(mcp.NewTool("scroll_until_text",
		mcp.WithDescription(scrollUntilTextToolDescription),
		mcp.WithString("text", mcp.Description("Text to search for while scrolling (case-insensitive substring match)."), mcp.Required()),
		mcp.WithString("direction", mcp.Description("Search direction: 'down' (default) or 'up'.")),
		mcp.WithNumber("amount", mcp.Description("Scroll step size for each move. Use 3-5 for search (default: 5).")),
		mcp.WithNumber("max_steps", mcp.Description("Maximum number of scroll attempts before stopping (default: 8).")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait between scroll attempts so the UI can settle (default: 150).")),
		mcp.WithNumber("x", mcp.Description("X coordinate to scroll at (defaults to screen centre).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate to scroll at (defaults to screen centre).")),
	), handleScrollUntilText)

	mcpServer.AddTool(mcp.NewTool("type_text",
		mcp.WithDescription(`Type text as keyboard input into the currently focused element. Click the target input field first to ensure it has focus before typing.

NORMAL WORKFLOW:
1. find_and_click {"text": "placeholder or label text"} → focuses the field
2. type_text {"text": "your text"}

IF THE FIELD HAS NO DETECTABLE TEXT (e.g. dark/empty placeholder):
- Find a labeled button immediately next to the field (e.g. "Clear" button beside the input)
- find_and_click {"text": "Clear"} → response includes box: {x, y, width, height}
- The input field is to the LEFT: click_at {"x": box.x - 200, "y": box.y + box.height/2}
- Then type_text

For special characters or control sequences (Enter, Tab, Ctrl+C), use press_key instead. To verify the text was entered, use wait_for_text or read_screen_text on the input region — not a full screenshot.`),
		mcp.WithString("text", mcp.Description("The exact text string to type. Supports Unicode. Do not include control characters — use press_key for Enter, Tab, Backspace etc."), mcp.Required()),
		mcp.WithBoolean("press_enter", mcp.Description("If true, automatically presses the Enter key immediately after the text is typed (default: false).")),
	), handleTypeText)

	mcpServer.AddTool(mcp.NewTool("click_and_type",
		mcp.WithDescription(`Move the mouse to (x, y), click to focus, and then type text. This is a single atomic operation that replaces separate click_at and type_text calls.

Use this tool when you already have the absolute pixel coordinates (e.g. from read_screen_text or find_elements) and need to interact with an input field. Do not guess coordinates.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels from the left edge of the screen."), mcp.Required()),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels from the top edge of the screen."), mcp.Required()),
		mcp.WithString("text", mcp.Description("The exact text string to type. Supports Unicode."), mcp.Required()),
		mcp.WithBoolean("press_enter", mcp.Description("If true, automatically presses the Enter key immediately after the text is typed (default: false).")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after the click before typing begins. This gives the UI time to focus the input field (default: 100). Max: 10000.")),
	), handleClickAndType)

	mcpServer.AddTool(mcp.NewTool("press_key",
		mcp.WithDescription(`Press a single keyboard key or key combination. Use for control keys, navigation, and shortcuts.

Common uses: 'enter' to confirm/submit, 'tab' to move between fields, 'esc' to cancel/close, 'backspace'/'delete' to erase, arrow keys to navigate lists. For shortcuts use modifier keys: 'ctrl', 'alt', 'shift', 'cmd' (macOS).`),
		mcp.WithString("key", mcp.Description("Key name: 'enter', 'tab', 'esc', 'space', 'backspace', 'delete', 'up', 'down', 'left', 'right', 'ctrl', 'alt', 'shift', 'win' (Windows key), 'cmd' (macOS), 'f1'–'f12', or any single letter/digit."), mcp.Required()),
	), handlePressKey)

	mcpServer.AddTool(mcp.NewTool("take_screenshot",
		mcp.WithDescription(screenshotToolDescription),
		mcp.WithNumber("x", mcp.Description("X coordinate of the top-left corner of the capture region in pixels (default: 0).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of the top-left corner of the capture region in pixels (default: 0).")),
		mcp.WithNumber("width", mcp.Description("Width of the capture region in pixels (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of the capture region in pixels (default: full screen height).")),
		mcp.WithNumber("quality", mcp.Description("Image quality: 0 = PNG lossless (default, largest, slowest for model to process). 1–100 = JPEG at that quality (85 recommended — ~10× smaller than PNG, significantly faster). Use PNG when you need to read small text; use JPEG=85 for general visual confirmation.")),
	), handleTakeScreenshot)

	registerOCRTools(mcpServer)

	logging.Info("All tools registered successfully")
}

// =============================================================================
// STARTUP CONFIG LOGGING
// =============================================================================

func logEnvConfig() {
	type envVar struct {
		name     string
		secret   bool   // mask value
		fallback string // shown when unset
	}
	vars := []envVar{
		{TokenEnvVar, true, "(not set — server will exit)"},
		{"GHOST_MCP_TRANSPORT", false, "stdio"},
		{"GHOST_MCP_DEBUG", false, "0 (off)"},
		{"GHOST_MCP_VISUAL", false, "0 (off)"},
		{"GHOST_MCP_KEEP_SCREENSHOTS", false, "0 (screenshots deleted after use)"},
		{"GHOST_MCP_SCREENSHOT_DIR", false, os.TempDir()},
		{"GHOST_MCP_AUDIT_LOG", false, "<UserConfigDir>/ghost-mcp/audit"},
		{"TESSDATA_PREFIX", false, "(not set — OCR will fail)"},
		{"GHOST_MCP_HTTP_ADDR", false, "localhost:8080"},
		{"GHOST_MCP_HTTP_BASE_URL", false, ""},
	}
	logging.Info("--- Configuration ---")
	for _, v := range vars {
		val := os.Getenv(v.name)
		if val == "" {
			logging.Info("  %-30s = %s", v.name, v.fallback)
		} else if v.secret {
			logging.Info("  %-30s = %s****", v.name, val[:min(8, len(val))])
		} else {
			logging.Info("  %-30s = %s", v.name, val)
		}
	}
	logging.Info("---------------------")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// =============================================================================
// MAIN
// =============================================================================

func main() {
	logging.Info("Starting %s v%s...", ServerName, ServerVersion)
	logging.Info("Platform: %s/%s", runtime.GOOS, runtime.GOARCH)

	token, err := validateStartupToken()
	if err != nil {
		logging.Error("Authentication not configured: %v", err)
		logging.Error("Ghost MCP requires a secret token to prevent unauthorized access.")
		logging.Error("Set %s to a random secret string in your environment:", TokenEnvVar)
		logging.Error("  Linux/macOS: export %s=$(openssl rand -hex 32)", TokenEnvVar)
		logging.Error("  Windows:     $env:%s = -join ((1..32)|%%{'{0:x}' -f (Get-Random -Max 256)})", TokenEnvVar)
		logging.Error("Then add it to your MCP client config under the 'env' key.")
		os.Exit(1)
	}
	logging.Info("Token authentication enabled (%s is set)", TokenEnvVar)

	// Print configuration so it's visible in logs at startup.
	logEnvConfig()

	auditLog, auditErr := audit.New()
	if auditErr != nil {
		logging.Error("Audit log unavailable: %v", auditErr)
	}
	defer auditLog.Close()
	auditLog.Log(audit.EventServerStart, "", "", map[string]interface{}{
		"version":  ServerVersion,
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
	})

	logging.Info("Failsafe position: (%d, %d) - Move mouse here to trigger emergency shutdown", FailsafeX, FailsafeY)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logging.Info("Received signal: %v", sig)
		initiateShutdown()
	}()

	cfg, err := transport.Load()
	if err != nil {
		logging.Error("Transport configuration error: %v", err)
		os.Exit(1)
	}
	logging.Info("Transport: %s", cfg.Mode)

	mcpServer := createServer(token, auditLog)

	switch cfg.Mode {
	case transport.Stdio:
		logging.Info("Starting stdio transport...")
		logging.Info("IMPORTANT: All application logs are written to stderr")
		logging.Info("stdout is reserved for MCP JSON-RPC protocol")
		if err = server.ServeStdio(mcpServer); err != nil {
			auditLog.Log(audit.EventServerStop, "", err.Error(), nil)
			logging.Error("Server error: %v", err)
			os.Exit(1)
		}

	case transport.HTTP:
		logging.Info("Starting HTTP/SSE transport...")
		if err = transport.ServeHTTP(state.shutdownChan, mcpServer, cfg, token, auditLog); err != nil {
			auditLog.Log(audit.EventServerStop, "", err.Error(), nil)
			logging.Error("HTTP server error: %v", err)
			os.Exit(1)
		}
	}

	auditLog.Log(audit.EventServerStop, "", "", nil)
	logging.Info("Server stopped gracefully")
}
