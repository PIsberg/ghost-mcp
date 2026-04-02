package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ghost-mcp/internal/learner"
	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/ocr"
	"github.com/ghost-mcp/internal/validate"
	"github.com/go-vgo/robotgo"
	"github.com/mark3labs/mcp-go/mcp"
)

// =============================================================================
// GLOBAL LEARNER STATE
// =============================================================================

// globalLearner is the singleton learner for this server process.
// It is initialised in initLearningMode and used by OCR tool handlers.
var globalLearner = learner.New()

// initLearningMode enables learning mode when GHOST_MCP_LEARNING=1.
func initLearningMode() {
	if os.Getenv("GHOST_MCP_LEARNING") == "1" {
		globalLearner.Enable()
		logging.Info("Learning mode enabled (GHOST_MCP_LEARNING=1)")
	}
}

// =============================================================================
// SCREEN DISCOVERY - builds the internal view
// =============================================================================

// learnCfg holds parameters for a learning scan.
type learnCfg struct {
	// Optional scan region; zero values mean full screen.
	RegionX, RegionY, RegionW, RegionH int
	// ScrollAmount is wheel-click ticks scrolled per page (default 5).
	ScrollAmount int
	// MaxPages caps the number of scroll pages scanned (default 10).
	MaxPages int
	// ScrollDirection is "down" or "up" (default "down").
	ScrollDirection string
}

// learnScreen performs a full GUI discovery scan and returns the combined view.
//
// Algorithm:
//  1. Capture the initial viewport and run multi-pass OCR.
//  2. Scroll down scroll_amount ticks, wait for animation, repeat up to max_pages.
//  3. Stop early when consecutive pages share identical text (end of scrollable area).
//  4. Scroll back to the top to restore the original viewport position.
//  5. Return all discovered elements with their page indices.
func learnScreen(cfg learnCfg) (*learner.View, error) {
	screenW, screenH := uiGetScreenSize()

	// Apply defaults and bounds.
	if cfg.RegionW <= 0 || cfg.RegionH <= 0 {
		cfg.RegionX, cfg.RegionY = 0, 0
		cfg.RegionW, cfg.RegionH = screenW, screenH
	}
	if cfg.MaxPages <= 0 {
		cfg.MaxPages = 10
	}
	if cfg.ScrollAmount <= 0 {
		cfg.ScrollAmount = 5
	}
	if cfg.ScrollDirection == "" {
		cfg.ScrollDirection = "down"
	}
	if err := validate.ScreenRegion(cfg.RegionX, cfg.RegionY, cfg.RegionW, cfg.RegionH, screenW, screenH); err != nil {
		return nil, fmt.Errorf("invalid scan region: %w", err)
	}

	var allElements []learner.Element
	prevPageText := ""
	scrollsDone := 0

	for page := 0; page < cfg.MaxPages; page++ {
		img, err := uiCaptureImage(cfg.RegionX, cfg.RegionY, cfg.RegionW, cfg.RegionH)
		if err != nil {
			return nil, fmt.Errorf("page %d: capture failed: %w", page, err)
		}

		// Run two OCR passes in parallel: normal (grayscale+contrast) and inverted
		// (catches white text on dark/coloured backgrounds).
		normalResult, _ := uiReadImage(img, ocr.Options{})
		invertedResult, _ := uiReadImage(img, ocr.Options{Inverted: true})

		// Merge words from both passes, keeping higher-confidence duplicates.
		pageElements := mergeOCRResults(page, cfg.RegionX, cfg.RegionY, normalResult, invertedResult)
		allElements = append(allElements, pageElements...)

		// Extract page text for repeat-detection.
		currentText := extractText(normalResult)

		logging.Info("learn_screen: page %d — %d elements (text len=%d)", page, len(pageElements), len(currentText))

		// Stop when the page content hasn't changed (end of scrollable area).
		if page > 0 && textSimilarity(prevPageText, currentText) > 0.85 {
			logging.Info("learn_screen: page %d content matches previous page, stopping early", page)
			break
		}
		prevPageText = currentText

		// Scroll to the next page (skip on the last iteration).
		if page < cfg.MaxPages-1 {
			uiScrollDir(cfg.ScrollAmount, cfg.ScrollDirection)
			scrollsDone++
			time.Sleep(300 * time.Millisecond)

			if err := uiCheckFailsafe(); err != nil {
				// Scroll back before returning the error.
				scrollBack(scrollsDone, cfg.ScrollAmount)
				return nil, err
			}
		}
	}

	// Always restore the original scroll position.
	scrollBack(scrollsDone, cfg.ScrollAmount)

	view := &learner.View{
		Elements:         learner.DeduplicateElements(allElements),
		PageCount:        scrollsDone + 1,
		ScrollAmountUsed: cfg.ScrollAmount,
		CapturedAt:       time.Now(),
		ScreenW:          screenW,
		ScreenH:          screenH,
	}
	logging.Info("learn_screen: complete — %d unique elements across %d pages", len(view.Elements), view.PageCount)
	return view, nil
}

// scrollBack undoes n down-scrolls by scrolling up the same amount.
func scrollBack(scrollsDone, scrollAmount int) {
	if scrollsDone <= 0 {
		return
	}
	logging.Debug("learn_screen: scrolling back up %d steps", scrollsDone)
	for i := 0; i < scrollsDone; i++ {
		uiScrollDir(scrollAmount, "up")
		time.Sleep(100 * time.Millisecond)
	}
}

// mergeOCRResults combines words from normalResult and invertedResult,
// offsets them by (offsetX, offsetY) to convert region-relative to screen
// coordinates, and tags each element with pageIndex.
func mergeOCRResults(pageIndex, offsetX, offsetY int, normalResult, invertedResult *ocr.Result) []learner.Element {
	type wordKey struct{ text string; x, y int }
	seen := make(map[wordKey]bool)
	var elements []learner.Element

	addWords := func(result *ocr.Result) {
		if result == nil {
			return
		}
		for _, w := range result.Words {
			if w.Confidence < ocr.MinConfidence {
				continue
			}
			text := strings.TrimSpace(w.Text)
			if text == "" {
				continue
			}
			key := wordKey{
				text: strings.ToLower(text),
				// Quantize to 10-pixel grid to suppress near-duplicate positions.
				x: (w.X / 10) * 10,
				y: (w.Y / 10) * 10,
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			elements = append(elements, learner.Element{
				Text:       text,
				X:          offsetX + w.X,
				Y:          offsetY + w.Y,
				Width:      w.Width,
				Height:     w.Height,
				Confidence: w.Confidence,
				PageIndex:  pageIndex,
			})
		}
	}

	addWords(normalResult)
	addWords(invertedResult)
	return elements
}

// extractText returns the full text from an OCR result, or empty string.
func extractText(r *ocr.Result) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Text)
}

// textSimilarity returns a rough [0,1] similarity score between two strings.
// It counts shared trigrams (3-character substrings) relative to their union.
func textSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) < 3 || len(b) < 3 {
		return 0.0
	}
	setA := trigrams(a)
	setB := trigrams(b)

	intersection := 0
	for k := range setA {
		if setB[k] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// trigrams returns a set of all 3-character substrings in s.
func trigrams(s string) map[string]bool {
	result := make(map[string]bool)
	runes := []rune(s)
	for i := 0; i+3 <= len(runes); i++ {
		result[string(runes[i:i+3])] = true
	}
	return result
}

// =============================================================================
// MCP TOOL HANDLERS
// =============================================================================

// handleLearnScreen runs a full GUI discovery scan and stores the result.
func handleLearnScreen(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling learn_screen request")

	cfg := learnCfg{}
	if v, err := getIntParam(request, "x"); err == nil {
		cfg.RegionX = v
	}
	if v, err := getIntParam(request, "y"); err == nil {
		cfg.RegionY = v
	}
	if v, err := getIntParam(request, "width"); err == nil {
		cfg.RegionW = v
	}
	if v, err := getIntParam(request, "height"); err == nil {
		cfg.RegionH = v
	}
	if v, err := getIntParam(request, "max_pages"); err == nil && v > 0 {
		cfg.MaxPages = v
	}
	if v, err := getIntParam(request, "scroll_amount"); err == nil && v > 0 {
		cfg.ScrollAmount = v
	}
	if v, err := getStringParam(request, "scroll_direction"); err == nil {
		switch v {
		case "up", "down":
			cfg.ScrollDirection = v
		default:
			return mcp.NewToolResultError(fmt.Sprintf("invalid scroll_direction %q, must be 'up' or 'down'", v)), nil
		}
	}

	startTime := time.Now()
	view, err := learnScreen(cfg)
	if err != nil {
		logging.Error("learn_screen failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("learn_screen failed: %v", err)), nil
	}
	globalLearner.SetView(view)

	elapsed := time.Since(startTime)
	logging.Info("learn_screen: stored view with %d elements in %v", len(view.Elements), elapsed)

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"elements_found":%d,"pages_scanned":%d,"screen_w":%d,"screen_h":%d,"elapsed_ms":%d}`,
		len(view.Elements), view.PageCount, view.ScreenW, view.ScreenH, elapsed.Milliseconds(),
	)), nil
}

// handleGetLearnedView returns the current learned view as JSON.
func handleGetLearnedView(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling get_learned_view request")

	view := globalLearner.GetView()
	if view == nil {
		return mcp.NewToolResultText(`{"learned":false,"message":"No view has been learned yet. Call learn_screen first."}`), nil
	}

	type elementJSON struct {
		Text       string  `json:"text"`
		X          int     `json:"x"`
		Y          int     `json:"y"`
		Width      int     `json:"width"`
		Height     int     `json:"height"`
		Confidence float64 `json:"confidence"`
		PageIndex  int     `json:"page_index"`
	}
	elems := make([]elementJSON, len(view.Elements))
	for i, e := range view.Elements {
		elems[i] = elementJSON{
			Text: e.Text, X: e.X, Y: e.Y,
			Width: e.Width, Height: e.Height,
			Confidence: e.Confidence, PageIndex: e.PageIndex,
		}
	}

	data, err := json.Marshal(struct {
		Learned    bool          `json:"learned"`
		PageCount  int           `json:"page_count"`
		ScreenW    int           `json:"screen_w"`
		ScreenH    int           `json:"screen_h"`
		CapturedAt string        `json:"captured_at"`
		Elements   []elementJSON `json:"elements"`
	}{
		Learned:    true,
		PageCount:  view.PageCount,
		ScreenW:    view.ScreenW,
		ScreenH:    view.ScreenH,
		CapturedAt: view.CapturedAt.Format(time.RFC3339),
		Elements:   elems,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to serialise view: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handleClearLearnedView discards the current learned view.
func handleClearLearnedView(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling clear_learned_view request")
	globalLearner.ClearView()
	logging.Info("Learned view cleared")
	return mcp.NewToolResultText(`{"success":true,"message":"Learned view cleared. Call learn_screen to rebuild."}`), nil
}

// handleSetLearningMode enables or disables learning mode at runtime.
func handleSetLearningMode(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling set_learning_mode request")

	enabled := getBoolParam(request, "enabled", false)
	if enabled {
		globalLearner.Enable()
		logging.Info("Learning mode enabled via set_learning_mode tool")
		return mcp.NewToolResultText(`{"success":true,"learning_mode":true}`), nil
	}
	globalLearner.Disable()
	logging.Info("Learning mode disabled via set_learning_mode tool")
	return mcp.NewToolResultText(`{"success":true,"learning_mode":false}`), nil
}

// =============================================================================
// LEARNING MODE INTEGRATION - helpers used by OCR handlers
// =============================================================================

// learnerRegionHint checks the learned view for a matching element and returns
// a region hint that covers the element with padding.  Returns (0,0,0,0,false)
// when no hint is available.
//
// If the element is on a non-zero page the caller must scroll to that page
// before using the coordinates.  The required scroll count is returned.
func learnerRegionHint(searchText string, screenW, screenH int) (x, y, w, h, scrollsNeeded int, found bool) {
	if !globalLearner.IsEnabled() || !globalLearner.HasView() {
		return 0, 0, 0, 0, 0, false
	}
	elem := globalLearner.FindElement(searchText)
	if elem == nil {
		return 0, 0, 0, 0, 0, false
	}

	const padding = 50
	rx := max(0, elem.X-padding)
	ry := max(0, elem.Y-padding)
	rw := min(screenW-rx, elem.Width+2*padding)
	rh := min(screenH-ry, elem.Height+2*padding)

	view := globalLearner.GetView()
	scrolls := 0
	if view != nil {
		scrolls = elem.PageIndex * view.ScrollAmountUsed
	}

	return rx, ry, rw, rh, scrolls, true
}

// autoLearnIfNeeded triggers a learning scan the first time a tool is called
// while learning mode is enabled but no view exists yet.
func autoLearnIfNeeded() {
	if !globalLearner.IsEnabled() || globalLearner.HasView() {
		return
	}
	logging.Info("learning mode: auto-learning screen before first tool call")
	view, err := learnScreen(learnCfg{})
	if err != nil {
		logging.Error("auto-learn failed: %v", err)
		return
	}
	globalLearner.SetView(view)

	// Restore focus after scroll-back by moving mouse to centre.
	sw, sh := uiGetScreenSize()
	robotgo.Move(sw/2, sh/2)
}
