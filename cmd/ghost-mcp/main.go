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
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

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

const (
	ServerName    = "ghost-mcp"
	ServerVersion = "1.0.0"
)

// TokenEnvVar is the environment variable for the authentication token.
const TokenEnvVar = "GHOST_MCP_TOKEN"

// =============================================================================
// GLOBAL STATE
// =============================================================================

type serverState struct {
	shutdownChan   chan struct{}
	isShuttingDown bool
}

type scrollSearchConfig struct {
	SearchText  string
	Direction   string
	Amount      int
	MaxScrolls  int
	Nth         int
	ScrollX     int
	ScrollY     int
	RegionX     int
	RegionY     int
	RegionW     int
	RegionH     int
	Grayscale   bool
	ElementType string
}

type scrollSearchResult struct {
	MinX, MinY, MaxX, MaxY int
	Found                  bool
	PassName               string
	VisibleText            string
	ScrollCount            int
	RepeatedViewport       bool
}

var state = &serverState{
	shutdownChan: make(chan struct{}),
}

var (
	uiGetScreenSize = robotgo.GetScreenSize
	uiMoveMouse     = func(x, y int) { robotgo.Move(x, y) }
	uiScrollDir     = func(amount int, direction string) { robotgo.ScrollDir(amount, direction) }
	uiCaptureImage  = func(x, y, w, h int) (image.Image, error) {
		img, err := robotgo.CaptureImg(x, y, w, h)
		if err != nil {
			return nil, err
		}

		// Detect blank/failed screenshots
		if isUniform(img) {
			logging.Error("uiCaptureImage: Captured image is completely uniform (all pixels same color). This often indicates a screen capture failure or per-app protection.")
		}

		return img, nil
	}
	uiReadImage     = ocr.ReadImage
	uiCheckFailsafe = checkFailsafe
	uiFindText      = parallelFindText
)

func isUniform(img image.Image) bool {
	bounds := img.Bounds()
	if bounds.Empty() {
		return true
	}
	r0, g0, b0, a0 := img.At(bounds.Min.X, bounds.Min.Y).RGBA()
	for py := bounds.Min.Y; py < bounds.Max.Y; py++ {
		for px := bounds.Min.X; px < bounds.Max.X; px++ {
			r, g, b, a := img.At(px, py).RGBA()
			if r != r0 || g != g0 || b != b0 || a != a0 {
				return false
			}
		}
	}
	return true
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

func handleMoveMouse(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling move_mouse request")

	x, errX := getIntParam(request, "x")
	y, errY := getIntParam(request, "y")
	vid, errVID := getIntParam(request, "visual_id")

	if errVID == nil {
		foundX, foundY, found := globalLearner.GetElementCoords(vid)
		if !found {
			return mcp.NewToolResultError(fmt.Sprintf("visual_id %d not found in current view", vid)), nil
		}
		x, y = foundX, foundY
	} else if errX != nil || errY != nil {
		return mcp.NewToolResultError("either 'visual_id' or both 'x' and 'y' must be provided"), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid position: %v", err)), nil
	}

	robotgo.Move(x, y)
	return mcp.NewToolResultText(fmt.Sprintf("Moved mouse to (%d, %d)", x, y)), nil
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

	x, errX := getIntParam(request, "x")
	y, errY := getIntParam(request, "y")
	vid, errVID := getIntParam(request, "visual_id")

	if errVID == nil {
		foundX, foundY, found := globalLearner.GetElementCoords(vid)
		if !found {
			return mcp.NewToolResultError(fmt.Sprintf("visual_id %d not found in current view", vid)), nil
		}
		x, y = foundX, foundY
	} else if errX != nil || errY != nil {
		return mcp.NewToolResultError("either 'visual_id' or both 'x' and 'y' must be provided"), nil
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

	// Check for repeated clicks at same location
	clickWarning := tracker.recordClick(x, y, button, true)
	if clickWarning.ShouldStop {
		logging.Error("REPEATED CLICK WARNING: %s", clickWarning.Reason)
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

	response := fmt.Sprintf(
		`{"success": true, "button": "%s", "requested_x": %d, "requested_y": %d, "actual_x": %d, "actual_y": %d}`,
		button, x, y, finalX, finalY,
	)

	// Add warning if clicking same spot too many times
	if clickWarning.ShouldStop {
		response = fmt.Sprintf(`%s,"warning":{"should_stop":true,"reason":%q,"click_count":%d,"message":"You've clicked this spot %d times in 30 seconds. Verify this is correct."}`,
			response[:len(response)-1], clickWarning.Reason, clickWarning.ClickCount, clickWarning.ClickCount)
	}

	return mcp.NewToolResultText(response + "}"), nil
}

func handleDoubleClick(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling double_click request")

	x, errX := getIntParam(request, "x")
	y, errY := getIntParam(request, "y")
	vid, errVID := getIntParam(request, "visual_id")

	if errVID == nil {
		foundX, foundY, found := globalLearner.GetElementCoords(vid)
		if !found {
			return mcp.NewToolResultError(fmt.Sprintf("visual_id %d not found in current view", vid)), nil
		}
		x, y = foundX, foundY
	} else if errX != nil || errY != nil {
		return mcp.NewToolResultError("either 'visual_id' or both 'x' and 'y' must be provided"), nil
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid double_click position: %v", err)), nil
	}

	robotgo.Move(x, y)
	robotgo.Click("left", true) // true = double click
	delay, _ := getIntParam(request, "delay_ms")
	if delay <= 0 {
		delay = 100
	}
	time.Sleep(time.Duration(delay) * time.Millisecond)

	return mcp.NewToolResultText(fmt.Sprintf("Double-clicked at (%d, %d)", x, y)), nil
}

func handleScroll(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling scroll request")

	screenW, screenH := uiGetScreenSize()

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

	// Default amount=15 for faster page coverage (was 3)
	// On most systems: amount=10 ≈ half page, amount=15 ≈ 2/3 page, amount=30 ≈ full page
	amount := 15
	if a, err := getIntParam(request, "amount"); err == nil {
		if a <= 0 {
			return mcp.NewToolResultError("amount must be positive"), nil
		}
		amount = a
	}

	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		logging.Error("Coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	logging.Info("ACTION: Moving mouse to (%d, %d) then scrolling %s by %d", x, y, direction, amount)
	uiMoveMouse(x, y)

	if err := uiCheckFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	uiScrollDir(amount, direction)
	logging.Info("ACTION COMPLETE: Scrolled %s by %d at (%d, %d)", direction, amount, x, y)

	// Run a quick OCR pass on the centre half of the screen so the AI knows
	// what is now visible without needing a separate screenshot + find_elements call.
	visibleText := ""
	stripY := screenH / 4
	stripH := screenH / 2
	if img, captureErr := uiCaptureImage(0, stripY, screenW, stripH); captureErr == nil {
		if ocrResult, ocrErr := uiReadImage(img, ocr.Options{}); ocrErr == nil {
			visibleText = ocrResult.Text
		} else {
			logging.Debug("scroll OCR failed (non-fatal): %v", ocrErr)
		}
	} else {
		logging.Debug("scroll capture failed (non-fatal): %v", captureErr)
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success": true, "x": %d, "y": %d, "direction": "%s", "amount": %d, "visible_text": %q, "note": "Use visible_text to check content BEFORE calling more tools. Only call find_elements if you need to interact with something not in visible_text."}`,
		x, y, direction, amount, visibleText,
	)), nil
}

func scrollSearchForText(ctx context.Context, cfg scrollSearchConfig) (*scrollSearchResult, error) {
	var lastHash uint64
	var lastVisibleText string

	for attempt := 0; attempt <= cfg.MaxScrolls; attempt++ {
		img, captureErr := uiCaptureImage(cfg.RegionX, cfg.RegionY, cfg.RegionW, cfg.RegionH)
		if captureErr != nil {
			return nil, fmt.Errorf("failed to capture screen: %w", captureErr)
		}

		currentHash := ocr.HashImageFast(img)

		// Abort immediately before doing expensive OCR if the viewport hasn't changed
		if attempt > 0 && currentHash == lastHash {
			return &scrollSearchResult{
				VisibleText:      lastVisibleText,
				ScrollCount:      attempt,
				RepeatedViewport: true,
			}, nil
		}
		lastHash = currentHash

		visibleText := ""
		if ocrResult, ocrErr := uiReadImage(img, ocr.Options{Color: !cfg.Grayscale}); ocrErr == nil {
			visibleText = ocrResult.Text
		} else {
			logging.Debug("scrollSearchForText visible-text OCR failed (non-fatal): %v", ocrErr)
		}
		lastVisibleText = visibleText

		minX, minY, maxX, maxY, found, passName := uiFindText(ctx, img, cfg.SearchText, cfg.Nth, cfg.Grayscale, cfg.ElementType)
		if found {
			return &scrollSearchResult{
				MinX:        minX,
				MinY:        minY,
				MaxX:        maxX,
				MaxY:        maxY,
				Found:       true,
				PassName:    passName,
				VisibleText: visibleText,
				ScrollCount: attempt,
			}, nil
		}

		if attempt == cfg.MaxScrolls {
			return &scrollSearchResult{
				VisibleText: visibleText,
				ScrollCount: attempt,
			}, nil
		}

		uiMoveMouse(cfg.ScrollX, cfg.ScrollY)
		if err := uiCheckFailsafe(); err != nil {
			return nil, err
		}
		uiScrollDir(cfg.Amount, cfg.Direction)
	}

	return nil, fmt.Errorf("unreachable scrollSearchForText state")
}

func handleScrollUntilText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling scroll_until_text request")

	searchText, err := getStringParam(request, "text")
	if err != nil {
		logging.Error("Invalid text parameter: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid text parameter: %v", err)), nil
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

	// Default amount=15 for faster page coverage (was 5)
	amount := 15
	if a, err := getIntParam(request, "amount"); err == nil {
		if a <= 0 {
			return mcp.NewToolResultError("amount must be positive"), nil
		}
		amount = a
	}

	maxScrolls := 8
	if n, err := getIntParam(request, "max_scrolls"); err == nil {
		if n <= 0 {
			return mcp.NewToolResultError("max_scrolls must be positive"), nil
		}
		maxScrolls = n
	}

	nth := 1
	if n, err := getIntParam(request, "nth"); err == nil {
		if n <= 0 {
			return mcp.NewToolResultError("nth must be >= 1"), nil
		}
		nth = n
	}

	grayscale := getBoolParam(request, "grayscale", true)

	screenW, screenH := uiGetScreenSize()

	scrollX := screenW / 2
	scrollY := screenH / 2
	if v, err := getIntParam(request, "scroll_x"); err == nil {
		scrollX = v
	}
	if v, err := getIntParam(request, "scroll_y"); err == nil {
		scrollY = v
	}
	if err := validate.Coords(scrollX, scrollY, screenW, screenH); err != nil {
		logging.Error("Scroll coordinate validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid scroll coordinates: %v", err)), nil
	}

	regionX, regionY := 0, 0
	regionW, regionH := screenW, screenH
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

	result, searchErr := scrollSearchForText(ctx, scrollSearchConfig{
		SearchText: searchText,
		Direction:  direction,
		Amount:     amount,
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
		logging.Error("scroll_until_text failed: %v", searchErr)
		return mcp.NewToolResultError(searchErr.Error()), nil
	}

	if result.Found {
		cx := regionX + (result.MinX+result.MaxX)/2
		cy := regionY + (result.MinY+result.MaxY)/2
		logging.Info("scroll_until_text: found %q after %d scrolls via %s pass", searchText, result.ScrollCount, result.PassName)
		return mcp.NewToolResultText(fmt.Sprintf(
			`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"center_x":%d,"center_y":%d,"scroll_count":%d,"direction":%q,"amount":%d,"pass":%q,"visible_text":%q}`,
			searchText, regionX+result.MinX, regionY+result.MinY, result.MaxX-result.MinX, result.MaxY-result.MinY, cx, cy, result.ScrollCount, direction, amount, result.PassName, result.VisibleText,
		)), nil
	}
	if result.RepeatedViewport {
		logging.Info("scroll_until_text: viewport repeated after %d scrolls while searching for %q", result.ScrollCount, searchText)
		return mcp.NewToolResultError(fmt.Sprintf(
			`text %q not found after %d scrolls; viewport stopped changing, likely reached the end. Last visible_text: %q`,
			searchText, result.ScrollCount, result.VisibleText,
		)), nil
	}

	logging.Info("scroll_until_text: text %q not found after max_scrolls=%d", searchText, maxScrolls)
	return mcp.NewToolResultError(fmt.Sprintf(
		`text %q not found after %d scrolls (%s by %d). Last visible_text: %q`,
		searchText, maxScrolls, direction, amount, result.VisibleText,
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

	x, errX := getIntParam(request, "x")
	y, errY := getIntParam(request, "y")
	vid, errVID := getIntParam(request, "visual_id")

	if errVID == nil {
		foundX, foundY, found := globalLearner.GetElementCoords(vid)
		if !found {
			return mcp.NewToolResultError(fmt.Sprintf("visual_id %d not found in current view", vid)), nil
		}
		x, y = foundX, foundY
	} else if errX != nil || errY != nil {
		return mcp.NewToolResultError("either 'visual_id' or both 'x' and 'y' must be provided"), nil
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
		server.WithInstructions(ghostMCPGuide),
	)
	registerTools(mcpServer)
	registerPrompts(mcpServer)
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
		mcp.WithDescription(`Move the mouse cursor to absolute screen coordinates or a visual_id badge.

Two modes:
- Coordinates: move_mouse(x=350, y=780)
- Badge ID:    move_mouse(visual_id=12) — read the badge number from the get_annotated_view image.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("visual_id", mcp.Description("The badge number you see in the get_annotated_view image (e.g. [12] means visual_id=12). If provided, x/y are ignored.")),
	), handleMoveMouse)

	mcpServer.AddTool(mcp.NewTool("click",
		mcp.WithDescription(`Click the mouse button at the current cursor position. Call move_mouse first to position the cursor.

⚠️ NOT FOR CLICKING BUTTONS BY LABEL: Use find_and_click instead — it locates and clicks in one call without needing move_mouse first.

Use this tool only when you have already moved the mouse to exact coordinates and need to click at the current position (e.g. after a drag, or a hover-then-click sequence). Use right-click to open context menus.`),
		mcp.WithString("button", mcp.Description("Mouse button to click: 'left' for normal clicks and selecting items, 'right' for context menus, 'middle' for middle-click."), mcp.Required()),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after the click for the UI to update (default: 100). Set to 0 to skip. Max: 10000.")),
	), handleClick)

	mcpServer.AddTool(mcp.NewTool("click_at",
		mcp.WithDescription(`Move the mouse and click in one atomic operation.

Two modes:
- Coordinates: click_at(x=350, y=780) — use when get_learned_view found the element.
- Badge ID:    click_at(visual_id=12)  — use when you read badge [12] from the get_annotated_view image.

Choose coordinates when OCR found your target. Choose visual_id when you identified
the badge number by looking at the annotated screenshot.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("visual_id", mcp.Description("The badge number you see in the get_annotated_view image (e.g. [12] means visual_id=12). If provided, x/y are ignored.")),
		mcp.WithString("button", mcp.Description("Mouse button: 'left' (default), 'right', or 'middle'.")),
		mcp.WithNumber("delay_ms", mcp.Description("Milliseconds to wait after the click for the UI to update (default: 100). Set to 0 to skip. Max: 10000.")),
	), handleClickAt)

	mcpServer.AddTool(mcp.NewTool("double_click",
		mcp.WithDescription(`Move the mouse and double-click. Use for opening files or activating items.

Two modes:
- Coordinates: double_click(x=350, y=780)
- Badge ID:    double_click(visual_id=12) — read the badge number from the get_annotated_view image.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("visual_id", mcp.Description("The badge number you see in the get_annotated_view image (e.g. [12] means visual_id=12). If provided, x/y are ignored.")),
		mcp.WithNumber("delay_ms", mcp.Description("Wait after double-click (default: 100). Max: 10000.")),
	), handleDoubleClick)

	mcpServer.AddTool(mcp.NewTool("scroll",
		mcp.WithDescription(`Move the mouse to (x, y) and scroll the mouse wheel.

NO-PEEK RULE: Do NOT use this tool manually to "peek" at the UI after learn_screen.
Instead, use click_at(id=N). The server will automatically scroll to the
indexed target for you.

── WHEN TO USE ────────────────────────────────────────────────────────────────
- To scroll for human-only visual inspection (e.g. reading an article).
- When learn_screen is NOT in use.

AMOUNT GUIDANCE:
- amount=15 (default): ~2/3 screen
- amount=5: ~1/4 screen
- amount=30: ~full screen`),
		mcp.WithNumber("x", mcp.Description("X coordinate to scroll at (pixels from left edge). Defaults to screen centre.")),
		mcp.WithNumber("y", mcp.Description("Y coordinate to scroll at (pixels from top edge). Defaults to screen centre.")),
		mcp.WithString("direction", mcp.Description("Scroll direction: 'up', 'down', 'left', or 'right'."), mcp.Required()),
		mcp.WithNumber("amount", mcp.Description("Number of scroll steps (default: 15 ≈ 2/3 screen). amount=5 for fine control. amount=30 for very long pages.")),
	), handleScroll)

	mcpServer.AddTool(mcp.NewTool("scroll_until_text",
		mcp.WithDescription(`BOUNDED SEARCH TOOL: scrolls and OCR-searches for text in one call. Use this instead of manually chaining scroll + find_elements + screenshots.

🎯 WHEN TO USE:
- You need to find text that is likely off-screen in a scrollable page, list, or panel
- You want a bounded search with fewer tool calls and no manual scroll loop

HOW IT WORKS:
1. OCR-searches the current viewport first
2. If not found, scrolls in the requested direction
3. Repeats search up to max_scrolls times
4. Stops early if the viewport text stops changing (end reached)
5. Returns the found text box and center coordinates`),
		mcp.WithString("text", mcp.Description("Text to search for while scrolling (case-insensitive substring match)."), mcp.Required()),
		mcp.WithString("direction", mcp.Description("Scroll direction: 'up', 'down', 'left', or 'right'."), mcp.Required()),
		mcp.WithNumber("amount", mcp.Description("Scroll steps per attempt (default: 5).")),
		mcp.WithNumber("max_scrolls", mcp.Description("Maximum number of scroll attempts (default: 8).")),
		mcp.WithNumber("nth", mcp.Description("Which occurrence to match if the text appears multiple times (default: 1).")),
		mcp.WithNumber("scroll_x", mcp.Description("X coordinate to scroll at (default: screen center).")),
		mcp.WithNumber("scroll_y", mcp.Description("Y coordinate to scroll at (default: screen center).")),
		mcp.WithNumber("x", mcp.Description("X coordinate of the OCR search region (default: 0 = full screen).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of the OCR search region (default: 0 = full screen).")),
		mcp.WithNumber("width", mcp.Description("Width of the OCR search region (default: full screen).")),
		mcp.WithNumber("height", mcp.Description("Height of the OCR search region (default: full screen).")),
		mcp.WithBoolean("grayscale", mcp.Description("Use grayscale OCR (default: true).")),
	), handleScrollUntilText)

	mcpServer.AddTool(mcp.NewTool("type_text",
		mcp.WithDescription(`Type text as keyboard input into the currently focused element. 

NORMAL WORKFLOW:
1. find_and_click {"text": "label"} → focuses the field
2. type_text {"text": "your text"}`),
		mcp.WithString("text", mcp.Description("The exact text string to type."), mcp.Required()),
		mcp.WithBoolean("press_enter", mcp.Description("If true, automatically presses the Enter key immediately after typing (default: false).")),
	), handleTypeText)

	mcpServer.AddTool(mcp.NewTool("click_and_type",
		mcp.WithDescription(`Click to focus a field and type text.

Two modes:
- Coordinates: click_and_type(x=350, y=780, text="hello")
- Badge ID:    click_and_type(visual_id=12, text="hello") — read [12] from the annotated image.`),
		mcp.WithNumber("x", mcp.Description("X coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("y", mcp.Description("Y coordinate in pixels. Required if 'visual_id' is not provided.")),
		mcp.WithNumber("visual_id", mcp.Description("The badge number you see in the get_annotated_view image (e.g. [12] means visual_id=12). If provided, x/y are ignored.")),
		mcp.WithString("text", mcp.Description("The exact text string to type."), mcp.Required()),
		mcp.WithBoolean("press_enter", mcp.Description("If true, automatically presses Enter after typing (default: false).")),
		mcp.WithNumber("delay_ms", mcp.Description("Wait after click before typing (default: 100).")),
	), handleClickAndType)

	mcpServer.AddTool(mcp.NewTool("press_key",
		mcp.WithDescription(`Press a single keyboard key or key combination. 
Common keys: 'enter', 'tab', 'esc', 'backspace', 'up', 'down'. Modifiers: 'ctrl', 'alt', 'shift'.`),
		mcp.WithString("key", mcp.Description("Key name (e.g. 'enter', 'tab', 'shift', 'a', '1')."), mcp.Required()),
	), handlePressKey)

	mcpServer.AddTool(mcp.NewTool("take_screenshot",
		mcp.WithDescription(`Capture a raw, un-annotated screenshot. (FOR VISUAL-ONLY TASKS).

🚫 DO NOT CALL THIS FOR UI ANALYSIS. If you are trying to find buttons, 
read text, or identify IDs, you are making a MAJOR REASONING ERROR. 

── USE get_annotated_view INSTEAD ─────────────────────────────────────────────
get_annotated_view provides the essential ID badges ([5], [12]) required for 
interaction. take_screenshot gives you a "blind" image with No IDs.`),
		mcp.WithNumber("x", mcp.Description("X coordinate of the top-left corner of the capture region in pixels (default: 0).")),
		mcp.WithNumber("y", mcp.Description("Y coordinate of the top-left corner of the capture region in pixels (default: 0).")),
		mcp.WithNumber("width", mcp.Description("Width of the capture region in pixels (default: full screen width).")),
		mcp.WithNumber("height", mcp.Description("Height of the capture region in pixels (default: full screen height).")),
		mcp.WithNumber("quality", mcp.Description("Image quality: 0 = PNG (fastest), 1-100 = JPEG (recommended 85 for speed).")),
	), handleTakeScreenshot)

	registerOCRTools(mcpServer)
	registerLearningTools(mcpServer)
	registerSmartClickTool(mcpServer)
	registerWorkflowTool(mcpServer)

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
		{"GHOST_MCP_LEARNING", false, "1 (learning mode on by default; set 0 to disable)"},
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

func main() {
	logging.Info("Starting %s v%s...", ServerName, ServerVersion)
	logging.Info("Platform: %s/%s", runtime.GOOS, runtime.GOARCH)

	// Step 1: Initialize environment (auto-configure Tesseract/DLLs on Windows)
	SetupWindowsEnv()

	// Step 2: Check Tesseract health (lite check)
	if version, err := ocr.CheckTesseract(); err != nil {
		logging.Error("OCR engine initialization check failed: %v", err)
		logging.Error("OCR tools (find_and_click, learn_screen, etc.) will not work correctly.")
	} else {
		logging.Info("OCR engine initialized: Tesseract %s (OK)", version)
		// Warm the client pool in the background to avoid startup hang
		go func() {
			// Triggering ReadAllPasses on a dummy image will prime the pool
			_, _ = ocr.ReadAllPasses(image.NewGray(image.Rect(0, 0, 1, 1)))
		}()
	}

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

	// Initialise optional features.
	initLearningMode()

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
