package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"strings"
	"time"

	"github.com/ghost-mcp/internal/learner"
	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/ocr"
	"github.com/ghost-mcp/internal/validate"
	"github.com/ghost-mcp/internal/visual"
	"github.com/go-vgo/robotgo"
	"github.com/mark3labs/mcp-go/mcp"
)

// =============================================================================
// GLOBAL LEARNER STATE
// =============================================================================

// globalLearner is the singleton learner for this server process.
var globalLearner = learner.New()

// initLearningMode enables learning mode by default.
// Set GHOST_MCP_LEARNING=0 to opt out.
func initLearningMode() {
	if os.Getenv("GHOST_MCP_LEARNING") == "0" {
		logging.Info("Learning mode disabled (GHOST_MCP_LEARNING=0)")
		return
	}
	globalLearner.Enable()
	logging.Info("Learning mode enabled (default; set GHOST_MCP_LEARNING=0 to disable)")
}

// =============================================================================
// SCREEN DISCOVERY
// =============================================================================

// learnCfg holds parameters for a learning scan.
type learnCfg struct {
	RegionX, RegionY, RegionW, RegionH int
	ScrollAmount                       int
	MaxPages                           int
	ScrollDirection                    string
}

// learnScreen performs a full GUI discovery scan and returns the combined view.
//
// Each scroll page produces three layers of understanding:
//  1. Four OCR passes (normal, inverted, bright-text, colour) merged and deduplicated.
//  2. Element type inference on every word found.
//  3. A JPEG screenshot stored in the view for visual retrieval later.
//
// After scanning, the viewport is restored to its original position.
func learnScreen(cfg learnCfg) (*learner.View, error) {
	screenW, screenH := uiGetScreenSize()

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
	var pages []learner.PageSnapshot
	prevPageText := ""
	scrollsDone := 0

	for page := 0; page < cfg.MaxPages; page++ {
		img, err := uiCaptureImage(cfg.RegionX, cfg.RegionY, cfg.RegionW, cfg.RegionH)
		if err != nil {
			return nil, fmt.Errorf("page %d: capture failed: %w", page, err)
		}

		// ── OCR: four passes ─────────────────────────────────────────────────
		// 1. Normal — grayscale + contrast stretch. Best for most UI text.
		// 2. Inverted — catches white text on dark/coloured backgrounds.
		// 3. BrightText — isolates near-white pixels (white text on any background).
		// 4. Color — full-colour pass for coloured-background buttons.
		//
		// PrepareParallelImageSet runs all four preprocessing variants concurrently,
		// then each is decoded by a separate pooled Tesseract client. This bypasses
		// ReadImage's single-slot cache which, when given the same image object,
		// returns the cached normal result for every subsequent opts variant — so
		// inverted, bright-text, and colour passes would silently produce duplicate
		// normal-pass output instead of their own preprocessed results.
		prepared, prepErr := ocr.PrepareParallelImageSet(img, true)
		if prepErr != nil {
			return nil, fmt.Errorf("page %d: prepare OCR passes: %w", page, prepErr)
		}
		normalResult, _ := ocr.ReadPreparedBytes(prepared.Normal, ocr.ScaleFactor, ocr.Options{})
		invertedResult, _ := ocr.ReadPreparedBytes(prepared.Inverted, ocr.ScaleFactor, ocr.Options{})
		brightResult, _ := ocr.ReadPreparedBytes(prepared.BrightText, ocr.ScaleFactor, ocr.Options{})
		darkResult, _ := ocr.ReadPreparedBytes(prepared.DarkText, ocr.ScaleFactor, ocr.Options{})
		colorResult, _ := ocr.ReadPreparedBytes(prepared.Color, ocr.ScaleFactor, ocr.Options{})

		pageElements := mergeOCRPasses(page, cfg.RegionX, cfg.RegionY,
			normalResult, invertedResult, brightResult, darkResult, colorResult)
		allElements = append(allElements, pageElements...)

		// ── Screenshot storage ───────────────────────────────────────────────
		jpegBytes := encodeJPEG(img)
		snap := learner.PageSnapshot{
			Index:                 page,
			CumulativeScrollTicks: scrollsDone * cfg.ScrollAmount,
			Width:                 cfg.RegionW,
			Height:                cfg.RegionH,
			ElementCount:          len(pageElements),
			JPEG:                  jpegBytes,
		}
		pages = append(pages, snap)

		// ── Repeat detection ─────────────────────────────────────────────────
		currentText := extractText(normalResult)
		logging.Info("learn_screen: page %d — %d elements, screenshot %d bytes",
			page, len(pageElements), len(jpegBytes))

		if page > 0 && textSimilarity(prevPageText, currentText) > 0.85 {
			logging.Info("learn_screen: page %d content matches previous — reached end of scrollable area", page)
			break
		}
		prevPageText = currentText

		if page < cfg.MaxPages-1 {
			uiScrollDir(cfg.ScrollAmount, cfg.ScrollDirection)
			scrollsDone++
			time.Sleep(300 * time.Millisecond)

			if err := uiCheckFailsafe(); err != nil {
				scrollBack(scrollsDone, cfg.ScrollAmount)
				return nil, err
			}
		}
	}

	scrollBack(scrollsDone, cfg.ScrollAmount)

	// ── Post-processing ──────────────────────────────────────────────────────
	// Deduplicate, infer element types, then associate labels with inputs.
	deduped := learner.DeduplicateElements(allElements)
	typed := inferTypes(deduped)
	associated := learner.AssociateLabels(typed)

	view := &learner.View{
		Elements:         associated,
		Pages:            pages,
		PageCount:        scrollsDone + 1,
		ScrollAmountUsed: cfg.ScrollAmount,
		CapturedAt:       time.Now(),
		ScreenW:          screenW,
		ScreenH:          screenH,
	}
	logging.Info("learn_screen: complete — %d elements across %d pages (%d screenshots)",
		len(view.Elements), view.PageCount, len(view.Pages))
	return view, nil
}

// inferTypes applies InferElementType to every element in the slice.
func inferTypes(elements []learner.Element) []learner.Element {
	out := make([]learner.Element, len(elements))
	copy(out, elements)
	for i := range out {
		out[i].Type = learner.InferElementType(out[i].Text, out[i].Width, out[i].Height)
	}
	return out
}

// encodeJPEG compresses img to JPEG at quality 60 and returns the bytes.
// Returns nil if encoding fails (non-fatal — screenshot is optional).
func encodeJPEG(img image.Image) []byte {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 60}); err != nil {
		logging.Debug("encodeJPEG: encode failed: %v", err)
		return nil
	}
	return buf.Bytes()
}

// =============================================================================
// HELPERS
// =============================================================================

// scrollBack undoes n down-scrolls by scrolling up the same amount.
func scrollBack(scrollsDone, scrollAmount int) {
	if scrollsDone <= 0 {
		return
	}
	logging.Debug("learn_screen: restoring scroll position (%d steps up)", scrollsDone)
	for i := 0; i < scrollsDone; i++ {
		uiScrollDir(scrollAmount, "up")
		time.Sleep(100 * time.Millisecond)
	}
}

// mergeOCRPasses combines words from up to five OCR passes into a single
// deduplicated element list. The pass that first discovers each word is
// recorded in OcrPass so the AI knows which rendering caught the element.
// Expected argument order: normal, inverted, bright-text, dark-text, color.
func mergeOCRPasses(pageIndex, offsetX, offsetY int, results ...*ocr.Result) []learner.Element {
	passTags := []learner.OcrPass{
		learner.OcrPassNormal,
		learner.OcrPassInverted,
		learner.OcrPassBrightText,
		learner.OcrPassDarkText,
		learner.OcrPassColor,
	}
	passes := make([]struct {
		result *ocr.Result
		pass   learner.OcrPass
	}, len(results))
	for i, r := range results {
		tag := learner.OcrPassNormal
		if i < len(passTags) {
			tag = passTags[i]
		}
		passes[i] = struct {
			result *ocr.Result
			pass   learner.OcrPass
		}{r, tag}
	}

	type wordKey struct {
		text string
		x, y int
	}
	seen := make(map[wordKey]bool)
	var elements []learner.Element

	for _, p := range passes {
		if p.result == nil {
			continue
		}
		for _, w := range p.result.Words {
			if w.Confidence < ocr.MinConfidence {
				continue
			}
			text := strings.TrimSpace(w.Text)
			if text == "" {
				continue
			}
			key := wordKey{
				text: strings.ToLower(text),
				x:    (w.X / 10) * 10,
				y:    (w.Y / 10) * 10,
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
				OcrPass:    p.pass,
			})
		}
	}
	return elements
}

// extractText returns trimmed full text from an OCR result, or empty string.
func extractText(r *ocr.Result) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Text)
}

// textSimilarity returns a [0,1] Jaccard similarity over 3-character trigrams.
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

	// Build per-type summary for the response.
	typeCounts := map[string]int{}
	for _, e := range view.Elements {
		typeCounts[string(e.Type)]++
	}
	typeJSON, _ := json.Marshal(typeCounts)

	logging.Info("learn_screen: stored view with %d elements in %v", len(view.Elements), elapsed)

	// Build response using proper JSON marshaling to avoid injection issues
	response := map[string]interface{}{
		"success":            true,
		"elements_found":     len(view.Elements),
		"pages_scanned":      view.PageCount,
		"screenshots_stored": len(view.Pages),
		"element_types":      json.RawMessage(typeJSON), // Embed JSON directly
		"screen_w":           view.ScreenW,
		"screen_h":           view.ScreenH,
		"elapsed_ms":         elapsed.Milliseconds(),
	}

	responseJSON, err := json.Marshal(response)
	if err != nil {
		logging.Error("learn_screen: failed to marshal response: %v", err)
		return mcp.NewToolResultError("failed to build response"), nil
	}

	return mcp.NewToolResultText(string(responseJSON)), nil
}

func handleGetLearnedView(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling get_learned_view request")

	view := globalLearner.GetView()
	if view == nil {
		return mcp.NewToolResultText(`{"learned":false,"message":"No view has been learned yet. Call learn_screen first."}`), nil
	}

	type elementJSON struct {
		ID         int     `json:"id"`
		Text       string  `json:"text"`
		X          int     `json:"x"`
		Y          int     `json:"y"`
		Width      int     `json:"width"`
		Height     int     `json:"height"`
		Confidence float64 `json:"confidence"`
		PageIndex  int     `json:"page_index"`
		Type       string  `json:"type"`
		OcrPass    string  `json:"ocr_pass"`
		LabelFor   string  `json:"label_for,omitempty"`
	}
	elems := make([]elementJSON, len(view.Elements))
	for i, e := range view.Elements {
		elems[i] = elementJSON{
			ID: e.ID, Text: e.Text, X: e.X, Y: e.Y,
			Width: e.Width, Height: e.Height,
			Confidence: e.Confidence, PageIndex: e.PageIndex,
			Type: string(e.Type), OcrPass: string(e.OcrPass),
			LabelFor: e.LabelFor,
		}
	}

	// Page summary (no JPEG bytes — use get_page_screenshot for images).
	type pageJSON struct {
		Index                 int  `json:"index"`
		CumulativeScrollTicks int  `json:"cumulative_scroll_ticks"`
		Width                 int  `json:"width"`
		Height                int  `json:"height"`
		ElementCount          int  `json:"element_count"`
		HasScreenshot         bool `json:"has_screenshot"`
	}
	pgList := make([]pageJSON, len(view.Pages))
	for i, p := range view.Pages {
		pgList[i] = pageJSON{
			Index: p.Index, CumulativeScrollTicks: p.CumulativeScrollTicks,
			Width: p.Width, Height: p.Height, ElementCount: p.ElementCount,
			HasScreenshot: len(p.JPEG) > 0,
		}
	}

	data, err := json.Marshal(struct {
		Learned    bool          `json:"learned"`
		PageCount  int           `json:"page_count"`
		ScreenW    int           `json:"screen_w"`
		ScreenH    int           `json:"screen_h"`
		CapturedAt string        `json:"captured_at"`
		Pages      []pageJSON    `json:"pages"`
		Elements   []elementJSON `json:"elements"`
	}{
		Learned:    true,
		PageCount:  view.PageCount,
		ScreenW:    view.ScreenW,
		ScreenH:    view.ScreenH,
		CapturedAt: view.CapturedAt.Format(time.RFC3339),
		Pages:      pgList,
		Elements:   elems,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to serialise view: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func handleClearLearnedView(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling clear_learned_view request")
	globalLearner.ClearView()
	logging.Info("Learned view cleared")
	return mcp.NewToolResultText(`{"success":true,"message":"Learned view cleared. Call learn_screen to rebuild."}`), nil
}

func handleGetAnnotatedView(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling get_annotated_view request")

	view := globalLearner.GetView()
	if view == nil {
		return mcp.NewToolResultText(`{"learned":false,"message":"No view has been learned yet. Call learn_screen or find_elements first."}`), nil
	}

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
		return mcp.NewToolResultError(fmt.Sprintf("invalid region: %v", err)), nil
	}

	// Capture the current viewport
	img, err := robotgo.CaptureImg(x, y, width, height)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("capture failed: %v", err)), nil
	}

	// Annotate with visual anchors
	annotated := visual.AnnotateImage(img, view.Elements, x, y)

	// Encode to JPEG
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, annotated, &jpeg.Options{Quality: 85}); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode failed: %v", err)), nil
	}

	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	return mcp.NewToolResultImage(
		fmt.Sprintf(`{"success":true,"element_count":%d,"format":"jpeg","size_bytes":%d}`, len(view.Elements), buf.Len()),
		b64,
		"image/jpeg",
	), nil
}

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

// handleGetPageScreenshot returns the stored JPEG screenshot for one scroll page
// as a base64-encoded image. This lets the AI visually analyze the page layout,
// icons, colours, and non-text elements that OCR cannot capture.
func handleGetPageScreenshot(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling get_page_screenshot request")

	pageIndex, err := getIntParam(request, "page_index")
	if err != nil {
		return mcp.NewToolResultError("page_index is required"), nil
	}
	if pageIndex < 0 {
		return mcp.NewToolResultError("page_index must be 0 or greater"), nil
	}

	if !globalLearner.HasView() {
		return mcp.NewToolResultError("no learned view — call learn_screen first"), nil
	}

	jpegBytes := globalLearner.GetPageScreenshot(pageIndex)
	if jpegBytes == nil {
		return mcp.NewToolResultError(fmt.Sprintf("no screenshot stored for page %d — re-run learn_screen", pageIndex)), nil
	}

	b64 := base64.StdEncoding.EncodeToString(jpegBytes)
	logging.Info("get_page_screenshot: returning page %d (%d bytes JPEG)", pageIndex, len(jpegBytes))

	return mcp.NewToolResultImage(
		fmt.Sprintf(`{"success":true,"page_index":%d,"format":"jpeg","size_bytes":%d}`, pageIndex, len(jpegBytes)),
		b64,
		"image/jpeg",
	), nil
}

// =============================================================================
// LEARNING MODE INTEGRATION — helpers used by OCR handlers
// =============================================================================

// learnerRegionHint checks the learned view for a matching element and returns
// a padded region hint plus the number of scroll ticks needed to reach it.
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
// while learning mode is on but no view exists yet.
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
	sw, sh := uiGetScreenSize()
	robotgo.Move(sw/2, sh/2)
}

// autoLearnWithPages performs a multi-page learning scan and returns the
// combined view. Unlike autoLearnIfNeeded, it respects the caller's
// scan_pages request and always runs a fresh scan (it does not reuse a
// stale view). The caller is responsible for storing the view via
// globalLearner.SetView if desired.
func autoLearnWithPages(scanPages int) (*learner.View, error) {
	if scanPages < 2 {
		return nil, fmt.Errorf("scan_pages must be >= 2, got %d", scanPages)
	}
	logging.Info("autoLearnWithPages: scanning %d pages", scanPages)
	view, err := learnScreen(learnCfg{
		MaxPages: scanPages,
	})
	if err != nil {
		return nil, fmt.Errorf("autoLearnWithPages: learnScreen failed: %w", err)
	}
	sw, sh := uiGetScreenSize()
	robotgo.Move(sw/2, sh/2)
	return view, nil
}
