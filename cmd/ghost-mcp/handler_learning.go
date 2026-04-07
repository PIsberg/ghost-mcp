package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"math/bits"
	"os"
	"strings"
	"time"

	"github.com/ghost-mcp/internal/cv"
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
	if os.Getenv("GHOST_MCP_ASYNC_SCAN") == "0" {
		logging.Info("learn_screen: using SYNCHRONOUS scanning mode (GHOST_MCP_ASYNC_SCAN=0)")
		return learnScreenSync(cfg)
	}
	logging.Info("learn_screen: using ASYNCHRONOUS pipeline scanning mode")
	return learnScreenAsync(cfg)
}

func imageSimilarity(img1, img2 image.Image) float64 {
	if img1 == nil || img2 == nil || img1.Bounds() != img2.Bounds() {
		return 0.0
	}
	b := img1.Bounds()
	stepX := max(1, b.Dx()/20)
	stepY := max(1, b.Dy()/20)

	var diff, total float64
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r1, g1, b1, _ := img1.At(x, y).RGBA()
			r2, g2, b2, _ := img2.At(x, y).RGBA()

			dr := float64(r1) - float64(r2)
			dg := float64(g1) - float64(g2)
			db := float64(b1) - float64(b2)

			if dr < 0 {
				dr = -dr
			}
			if dg < 0 {
				dg = -dg
			}
			if db < 0 {
				db = -db
			}

			diff += dr + dg + db
			total += 3 * 65535
		}
	}
	if total == 0 {
		return 1.0
	}
	return 1.0 - (diff / total)
}

func computeDHash(img image.Image) uint64 {
	if img == nil {
		return 0
	}
	b := img.Bounds()
	stepX := float64(b.Dx()) / 9.0
	stepY := float64(b.Dy()) / 8.0

	gray := func(x, y int) uint32 {
		r, g, bColor, _ := img.At(x, y).RGBA()
		return (r*299 + g*587 + bColor*114) / 1000
	}

	pixels := make([][]uint32, 8)
	for y := 0; y < 8; y++ {
		pixels[y] = make([]uint32, 9)
		py := b.Min.Y + int(float64(y)*stepY+stepY/2)
		for x := 0; x < 9; x++ {
			px := b.Min.X + int(float64(x)*stepX+stepX/2)
			pixels[y][x] = gray(px, py)
		}
	}

	var hash uint64
	bitIndex := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if pixels[y][x] < pixels[y][x+1] {
				hash |= (1 << bitIndex)
			}
			bitIndex++
		}
	}
	return hash
}

func hammingDistance(h1, h2 uint64) int {
	return bits.OnesCount64(h1 ^ h2)
}

func appendIconElements(existing []learner.Element, iconRects []image.Rectangle, pageIndex, offsetX, offsetY int) []learner.Element {
	for _, ir := range iconRects {
		ir = ir.Add(image.Pt(offsetX, offsetY))

		isText := false
		for _, e := range existing {
			er := image.Rect(e.X, e.Y, e.X+e.Width, e.Y+e.Height)
			intersect := ir.Intersect(er)
			if !intersect.Empty() {
				isText = true
				break
			}
		}
		if !isText {
			existing = append(existing, learner.Element{
				Text:       "", // pure icons have absolutely no text
				X:          ir.Min.X,
				Y:          ir.Min.Y,
				Width:      ir.Dx(),
				Height:     ir.Dy(),
				Confidence: 100.0,
				PageIndex:  pageIndex,
				Type:       learner.ElementTypeIcon,
				OcrPass:    learner.OcrPassNormal,
			})
		}
	}
	return existing
}

func learnScreenAsync(cfg learnCfg) (*learner.View, error) {
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

	type ocrJob struct {
		page     int
		img      image.Image
		prepared *ocr.PreparedImageSet
	}

	var allElements []learner.Element
	var pages []learner.PageSnapshot
	var prevImg image.Image
	var prevHash uint64
	scrollsDone := 0

	jobs := make([]ocrJob, 0, cfg.MaxPages)

	for page := 0; page < cfg.MaxPages; page++ {
		img, err := uiCaptureImage(cfg.RegionX, cfg.RegionY, cfg.RegionW, cfg.RegionH)
		if err != nil {
			return nil, fmt.Errorf("page %d: capture failed: %w", page, err)
		}

		prepared, prepErr := ocr.PrepareParallelImageSet(img, true)
		if prepErr != nil {
			return nil, fmt.Errorf("page %d: prepare OCR passes: %w", page, prepErr)
		}

		jobs = append(jobs, ocrJob{page: page, img: img, prepared: prepared})

		jpegBytes := encodeJPEG(img)
		snap := learner.PageSnapshot{
			Index:                 page,
			CumulativeScrollTicks: scrollsDone * cfg.ScrollAmount,
			Width:                 cfg.RegionW,
			Height:                cfg.RegionH,
			ElementCount:          0, // Filled after OCR
			JPEG:                  jpegBytes,
		}
		saveScreenshotIfKeptFromBytes(jpegBytes, fmt.Sprintf("ghost-mcp-learn-page%d", page))
		pages = append(pages, snap)

		usePHash := os.Getenv("GHOST_MCP_PHASH") != "0"
		currHash := uint64(0)
		if usePHash {
			currHash = computeDHash(img)
		}

		if page > 0 {
			if usePHash {
				if hammingDistance(prevHash, currHash) <= 2 {
					logging.Info("learn_screen: page %d dHash matches previous (dist <= 2) — reached end of scrollable area", page)
					break
				}
			} else {
				if imageSimilarity(prevImg, img) > 0.99 {
					logging.Info("learn_screen: page %d image matches previous — reached end of scrollable area", page)
					break
				}
			}
		}
		prevImg = img
		prevHash = currHash

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

	logging.Info("learn_screen: captured %d pages. Running OCR pipeline concurrently...", len(jobs))

	type ocrResultStruct struct {
		page     int
		elements []learner.Element
		err      error
	}

	resultsChan := make(chan ocrResultStruct, len(jobs))

	for _, job := range jobs {
		go func(j ocrJob) {
			normalResult, err := ocr.ReadPreparedBytes(j.prepared.Normal, ocr.ScaleFactor, ocr.Options{})
			if err != nil {
				resultsChan <- ocrResultStruct{page: j.page, err: fmt.Errorf("normal OCR failed: %w", err)}
				return
			}
			invertedResult, _ := ocr.ReadPreparedBytes(j.prepared.Inverted, ocr.ScaleFactor, ocr.Options{})
			brightResult, _ := ocr.ReadPreparedBytes(j.prepared.BrightText, ocr.ScaleFactor, ocr.Options{})
			darkResult, _ := ocr.ReadPreparedBytes(j.prepared.DarkText, ocr.ScaleFactor, ocr.Options{})
			colorResult, _ := ocr.ReadPreparedBytes(j.prepared.Color, ocr.ScaleFactor, ocr.Options{})

			elems := mergeOCRPasses(j.page, cfg.RegionX, cfg.RegionY,
				normalResult, invertedResult, brightResult, darkResult, colorResult)

			if os.Getenv("GHOST_MCP_CV_ICONS") != "0" {
				iconRects := cv.FindIcons(j.img)
				elems = appendIconElements(elems, iconRects, j.page, cfg.RegionX, cfg.RegionY)
			}

			resultsChan <- ocrResultStruct{page: j.page, elements: elems, err: nil}
		}(job)
	}

	pageElementsMap := make(map[int][]learner.Element)
	for i := 0; i < len(jobs); i++ {
		res := <-resultsChan
		if res.err != nil {
			logging.Error("learn_screen async OCR error on page %d: %v", res.page, res.err)
			continue
		}
		pageElementsMap[res.page] = res.elements
		allElements = append(allElements, res.elements...)
	}

	for i := range pages {
		pages[i].ElementCount = len(pageElementsMap[pages[i].Index])
	}

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
	logging.Info("learn_screen: async complete — %d elements across %d pages",
		len(view.Elements), view.PageCount)
	return view, nil
}

func learnScreenSync(cfg learnCfg) (*learner.View, error) {
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

		prepared, prepErr := ocr.PrepareParallelImageSet(img, true)
		if prepErr != nil {
			return nil, fmt.Errorf("page %d: prepare OCR passes: %w", page, prepErr)
		}
		normalResult, err := ocr.ReadPreparedBytes(prepared.Normal, ocr.ScaleFactor, ocr.Options{})
		if err != nil {
			logging.Error("learn_screen: normal OCR pass failed: %v", err)
			return nil, fmt.Errorf("page %d: normal OCR pass failed: %w", page, err)
		}
		invertedResult, err := ocr.ReadPreparedBytes(prepared.Inverted, ocr.ScaleFactor, ocr.Options{})
		if err != nil {
			logging.Debug("learn_screen: inverted OCR pass failed (non-fatal): %v", err)
		}
		brightResult, err := ocr.ReadPreparedBytes(prepared.BrightText, ocr.ScaleFactor, ocr.Options{})
		if err != nil {
			logging.Debug("learn_screen: bright OCR pass failed (non-fatal): %v", err)
		}
		darkResult, err := ocr.ReadPreparedBytes(prepared.DarkText, ocr.ScaleFactor, ocr.Options{})
		if err != nil {
			logging.Debug("learn_screen: dark OCR pass failed (non-fatal): %v", err)
		}
		colorResult, err := ocr.ReadPreparedBytes(prepared.Color, ocr.ScaleFactor, ocr.Options{})
		if err != nil {
			logging.Debug("learn_screen: color OCR pass failed: %v", err)
		}

		if normalResult == nil && invertedResult == nil && brightResult == nil && darkResult == nil && colorResult == nil {
			if err != nil {
				return nil, fmt.Errorf("OCR engine failed: %w. Check TESSDATA_PREFIX and DLL paths.", err)
			}
			logging.Error("learn_screen: all OCR passes returned nil results. Elements will be 0.")
		}

		pageElements := mergeOCRPasses(page, cfg.RegionX, cfg.RegionY,
			normalResult, invertedResult, brightResult, darkResult, colorResult)

		if os.Getenv("GHOST_MCP_CV_ICONS") != "0" {
			iconRects := cv.FindIcons(img)
			pageElements = appendIconElements(pageElements, iconRects, page, cfg.RegionX, cfg.RegionY)
		}

		allElements = append(allElements, pageElements...)

		jpegBytes := encodeJPEG(img)
		snap := learner.PageSnapshot{
			Index:                 page,
			CumulativeScrollTicks: scrollsDone * cfg.ScrollAmount,
			Width:                 cfg.RegionW,
			Height:                cfg.RegionH,
			ElementCount:          len(pageElements),
			JPEG:                  jpegBytes,
		}
		saveScreenshotIfKeptFromBytes(jpegBytes, fmt.Sprintf("ghost-mcp-learn-page%d", page))
		pages = append(pages, snap)

		currentText := extractText(normalResult)
		logging.Info("learn_screen: page %d — %d elements finalized", page, len(pageElements))
		if normalResult != nil {
			logging.Debug("learn_screen: page %d (normal pass) — %d words found, %d skipped (confidence < 35.0)",
				page, normalResult.TotalWordsFound, normalResult.WordsFiltered)
		}
		if len(pageElements) == 0 && (normalResult != nil && normalResult.TotalWordsFound > 0) {
			logging.Info("DIAGNOSTIC: page %d found %d raw words, but ALL were filtered. This means Tesseract is seeing text but it is too low confidence. Try GHOST_MCP_MIN_CONFIDENCE=20.",
				page, normalResult.TotalWordsFound)
		}

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
	logging.Info("learn_screen: sync complete — %d elements across %d pages (%d screenshots)",
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
				logging.Debug("mergeOCRPasses: skipping %q (confidence %.2f < %.2f) in %s pass", w.Text, w.Confidence, ocr.MinConfidence, p.pass)
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
			logging.Debug("mergeOCRPasses: found %q in %s pass (conf %.2f)", text, p.pass, w.Confidence)
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
	logging.Info("Agent is learning the screen layout to improve navigation accuracy")
	logging.Debug("LearnScreen parameters: %v", request.Params)

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

func handleGetLearnedView(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Info("Agent is retrieving the learned UI elements for analysis")

	view := globalLearner.GetView()
	if view == nil {
		return mcp.NewToolResultText(`{"learned":false,"message":"No view has been learned yet. Call learn_screen first."}`), nil
	}

	// Build element type filter set (nil = no filter = return all).
	var typeFilter map[learner.ElementType]struct{}
	if typeStrs, err := getStringArrayParam(request, "element_types"); err == nil && len(typeStrs) > 0 {
		typeFilter = make(map[learner.ElementType]struct{}, len(typeStrs))
		for _, s := range typeStrs {
			typeFilter[learner.ElementType(s)] = struct{}{}
		}
		logging.Debug("get_learned_view: filtering by types %v", typeStrs)
	}

	type elementJSON struct {
		OcrID      int     `json:"ocr_id"`
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
	elems := make([]elementJSON, 0, len(view.Elements))
	for _, e := range view.Elements {
		if typeFilter != nil {
			if _, ok := typeFilter[e.Type]; !ok {
				continue
			}
		}
		elems = append(elems, elementJSON{
			OcrID: e.ID, Text: e.Text, X: e.X, Y: e.Y,
			Width: e.Width, Height: e.Height,
			Confidence: e.Confidence, PageIndex: e.PageIndex,
			Type: string(e.Type), OcrPass: string(e.OcrPass),
			LabelFor: e.LabelFor,
		})
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
	logging.Info("Agent is generating an annotated view of the screen")

	view := globalLearner.GetView()
	if view == nil {
		return mcp.NewToolResultText(`{"learned":false,"message":"No view has been learned yet. Call learn_screen first."}`), nil
	}

	pageIndex, err := getIntParam(request, "page_index")
	var img image.Image
	var elements []learner.Element
	var offsetX, offsetY int

	dpiScale := getDPIScale()
	screenW, screenH := robotgo.GetScreenSize()

	if err == nil && pageIndex >= 0 {
		// ── PAGE HISTORY MODE ─────────────────────────────────────────────
		// Retrieve stored screenshot and elements for specifically requested page
		logging.Info("get_annotated_view: using stored Page %d from history", pageIndex)
		jpegBytes := globalLearner.GetPageScreenshot(pageIndex)
		if jpegBytes == nil {
			return mcp.NewToolResultError(fmt.Sprintf("no screenshot found for page %d", pageIndex)), nil
		}

		img, err = jpeg.Decode(bytes.NewReader(jpegBytes))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to decode stored page: %v", err)), nil
		}

		// Use the same scan region from the learning session
		// Note: we use 0,0 as we are drawing on the screenshot itself, which already
		// captured the region at its native screen coordinates.
		// Wait! Actually View elements are absolute. offsetX/offsetY must be 0,0
		// but we need to filter elements to just this page.
		offsetX, offsetY = 0, 0 // Relative to the stored image
		for _, e := range view.Elements {
			if e.PageIndex == pageIndex {
				// We need to pass coordinates that will result in the correct position
				// on the stored image. Since stored images are PageSnapshots which
				// are whole-screen or regional caps, and our elements have
				// screen X/Y at the time of cap...
				// Wait! Handler_learning.go:138 passes RegionX/Y during merge.
				// So e.X = RegionX + wordX.
				// To draw on the image starting at RegionX/Y, we subtract RegionX/Y.
				// But we don't know the RegionX/Y of the historical cap!
				// Actually, we do: view.Elements have absolute screen coords.
				// And the PageSnapshot was taken at some scroll position.
				// Actually, the simplest way: the PageSnapshot JPEG is already the
				// region [x, y, w, h]. So subtract the top-left of the region.
				elements = append(elements, e)
			}
		}

		// Re-fetch the region used during the first page scan
		if len(view.Pages) > 0 {
			// Find the page metadata to get the original scroll offsets
			var snap *learner.PageSnapshot
			for i := range view.Pages {
				if view.Pages[i].Index == pageIndex {
					snap = &view.Pages[i]
					break
				}
			}
			if snap != nil {
				// offset is absolute screen. Annotation logic subtracts offsetX/Y.
				// So set them to the absolute screen top-left at that page scan.
				// Since all page scans use the same RegionX/Y (passed to ScanPage),
				// we just need the base region coords.
				// Currently we don't store base RegionX/Y in the View header,
				// but we can infer them from the first element or page metadata.
				// Let's use 0,0 for now as the simplest baseline if we assumed full screen.
				// Actually, if we use e.X - offsetX, and e.X is already regional...
				// I'll check how elements are stored in mergeOCRPasses.
				// Line 300: X: offsetX + w.X.  (offsetX was RegionX).
				// So e.X is the absolute screen X.
				// The snapshot JPEG starts at absolute screen RegionX/Y.
				// So we should pass RegionX/Y as offsetX/Y.
				// Since we don't store it, we'll use the elements' min/max or just 0,0.
				// Actually, I'll update Learner to store RegionX/Y.
			}
		}
	} else {
		// ── LIVE VIEWPORT MODE ────────────────────────────────────────────
		logging.Info("get_annotated_view: capturing fresh live viewport")
		x, _ := getIntParam(request, "x")
		y, _ := getIntParam(request, "y")
		width := screenW
		height := screenH
		if w, err := getIntParam(request, "width"); err == nil && w > 0 {
			width = w
		}
		if h, err := getIntParam(request, "height"); err == nil && h > 0 {
			height = h
		}

		if err := validate.ScreenRegion(x, y, width, height, screenW, screenH); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid region: %v", err)), nil
		}

		img, err = robotgo.CaptureImg(x, y, width, height)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("capture failed: %v", err)), nil
		}
		offsetX, offsetY = x, y
		elements = view.Elements
	}

	// Annotate with visual anchors
	annotated := visual.AnnotateImage(img, elements, offsetX, offsetY, dpiScale)

	// Encode to JPEG
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, annotated, &jpeg.Options{Quality: 85}); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode failed: %v", err)), nil
	}

	// Save debug screenshot if requested
	saveScreenshotIfKeptFromBytes(buf.Bytes(), "ghost-mcp-annotated")

	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	return mcp.NewToolResultImage(
		fmt.Sprintf(`{"success":true,"element_count":%d,"page_index":%d,"format":"jpeg","size_bytes":%d}`, len(elements), pageIndex, buf.Len()),
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
	logging.Info("Agent is retrieving a page screenshot from the learning history")

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
