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

	"github.com/ghost-mcp/internal/learner"
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

// =============================================================================
// CALL LIMIT TRACKING - Prevent infinite loops
// =============================================================================

const (
	MaxConsecutiveFailures  = 3  // After 3 failures, suggest different approach
	MaxSameCoordinateClicks = 5  // After 5 clicks at same spot, stop
	MaxToolCallsPerSession  = 25 // Global limit per session
)

type clickHistoryEntry struct {
	x, y      int
	timestamp time.Time
	button    string
	success   bool
}

type callTracker struct {
	mu                  sync.RWMutex
	consecutiveFailures map[string]int // key: tool:text
	clickHistory        []clickHistoryEntry
	totalCalls          int
	sessionStartTime    time.Time
}

var tracker = &callTracker{
	consecutiveFailures: make(map[string]int),
	clickHistory:        make([]clickHistoryEntry, 0),
	sessionStartTime:    time.Now(),
}

// recordCall records a tool call and returns current stats
func (t *callTracker) recordCall(tool, text string, success bool) CallStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.totalCalls++
	key := fmt.Sprintf("%s:%s", tool, strings.ToLower(text))

	if success {
		t.consecutiveFailures[key] = 0
	} else {
		t.consecutiveFailures[key]++
	}

	return CallStats{
		TotalCalls:          t.totalCalls,
		ConsecutiveFailures: t.consecutiveFailures[key],
		RemainingCalls:      MaxToolCallsPerSession - t.totalCalls,
		ShouldGiveUp:        t.totalCalls >= MaxToolCallsPerSession,
	}
}

// recordClick records a click at coordinates and checks for repetition
func (t *callTracker) recordClick(x, y int, button string, success bool) ClickWarning {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-30 * time.Second) // Look at last 30 seconds

	// Count clicks at same coordinate within time window
	sameCoordCount := 0
	for _, entry := range t.clickHistory {
		if entry.timestamp.After(windowStart) {
			// Same coordinate within 10 pixels
			if abs(entry.x-x) < 10 && abs(entry.y-y) < 10 {
				sameCoordCount++
			}
		}
	}

	// Add to history (keep last 100 entries)
	t.clickHistory = append(t.clickHistory, clickHistoryEntry{
		x: x, y: y, timestamp: now, button: button, success: success,
	})
	if len(t.clickHistory) > 100 {
		t.clickHistory = t.clickHistory[1:]
	}

	warning := ClickWarning{
		ShouldStop: false,
		Reason:     "",
		ClickCount: sameCoordCount + 1,
	}

	if sameCoordCount >= MaxSameCoordinateClicks {
		warning.ShouldStop = true
		warning.Reason = fmt.Sprintf("Clicked same coordinates (%d,%d) %d times in 30 seconds", x, y, sameCoordCount+1)
	}

	return warning
}

// CallStats returns current call statistics
type CallStats struct {
	TotalCalls          int
	ConsecutiveFailures int
	RemainingCalls      int
	ShouldGiveUp        bool
}

// ClickWarning warns about repeated clicks
type ClickWarning struct {
	ShouldStop bool
	Reason     string
	ClickCount int
}

// getTrackerStats returns current tracker stats (for debugging)
func getTrackerStats() CallStats {
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()

	return CallStats{
		TotalCalls:     tracker.totalCalls,
		RemainingCalls: MaxToolCallsPerSession - tracker.totalCalls,
		ShouldGiveUp:   tracker.totalCalls >= MaxToolCallsPerSession,
	}
}

// =============================================================================
// REGION CACHE FOR OPTIMIZATION
// =============================================================================

// RegionCacheEntry stores a cached region for a text label
type RegionCacheEntry struct {
	X        int       `json:"x"`
	Y        int       `json:"y"`
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	LastUsed time.Time `json:"last_used"`
	HitCount int       `json:"hit_count"`
	ScreenW  int       `json:"screen_w"` // Screen width when cached
	ScreenH  int       `json:"screen_h"` // Screen height when cached
}

// RegionCache stores frequently used UI element regions
type RegionCache struct {
	mu      sync.RWMutex
	entries map[string]*RegionCacheEntry
	maxSize int           // Maximum number of entries
	maxAge  time.Duration // Maximum age before eviction
	stats   CacheStats
}

// CacheStats tracks cache performance
type CacheStats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Updates   int64 `json:"updates"`
}

// Global region cache instance
var regionCache = &RegionCache{
	entries: make(map[string]*RegionCacheEntry),
	maxSize: 100,
	maxAge:  24 * time.Hour,
}

// Get retrieves a cached region for the given text label
// Returns the entry and a bool indicating if it was found and is still valid
func (rc *RegionCache) Get(text string, screenW, screenH int) (*RegionCacheEntry, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	normalizedText := normalizeText(text)
	entry, exists := rc.entries[normalizedText]
	if !exists {
		return nil, false
	}

	// Check if entry is still valid (screen resolution hasn't changed)
	if entry.ScreenW != screenW || entry.ScreenH != screenH {
		return nil, false
	}

	// Check if entry has expired
	if time.Since(entry.LastUsed) > rc.maxAge {
		return nil, false
	}

	return entry, true
}

// Put stores or updates a region cache entry
func (rc *RegionCache) Put(text string, x, y, width, height, screenW, screenH int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	normalizedText := normalizeText(text)

	// Check if we need to evict entries
	if len(rc.entries) >= rc.maxSize {
		rc.evictOldest()
	}

	entry, exists := rc.entries[normalizedText]
	if exists {
		// Update existing entry
		entry.X = x
		entry.Y = y
		entry.Width = width
		entry.Height = height
		entry.LastUsed = time.Now()
		entry.HitCount++
		entry.ScreenW = screenW
		entry.ScreenH = screenH
		rc.stats.Updates++
	} else {
		// Create new entry
		rc.entries[normalizedText] = &RegionCacheEntry{
			X:        x,
			Y:        y,
			Width:    width,
			Height:   height,
			LastUsed: time.Now(),
			HitCount: 0,
			ScreenW:  screenW,
			ScreenH:  screenH,
		}
	}
}

// evictOldest removes the least recently used entry
// Must be called with rc.mu.Lock() held
func (rc *RegionCache) evictOldest() {
	if len(rc.entries) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time = time.Now()

	for key, entry := range rc.entries {
		if entry.LastUsed.Before(oldestTime) {
			oldestTime = entry.LastUsed
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(rc.entries, oldestKey)
		rc.stats.Evictions++
	}
}

// RecordHit records a cache hit
func (rc *RegionCache) RecordHit() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.stats.Hits++
}

// RecordMiss records a cache miss
func (rc *RegionCache) RecordMiss() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.stats.Misses++
}

// GetStats returns current cache statistics
func (rc *RegionCache) GetStats() CacheStats {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.stats
}

// Clear clears all cache entries
func (rc *RegionCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.entries = make(map[string]*RegionCacheEntry)
}

// normalizeText normalizes text for cache key lookup
// Converts to lowercase and trims whitespace for consistent matching
func normalizeText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
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

	// Check global call limit FIRST
	stats := tracker.recordCall("find_and_click", searchText, true) // Will update success later
	if stats.ShouldGiveUp {
		return mcp.NewToolResultError(fmt.Sprintf(`{"error":"MAXIMUM TOOL CALLS REACHED (%d). Stop and try a completely different approach.","total_calls":%d,"suggestion":"Use find_elements to see what's actually on screen, or try a different search term."}`, MaxToolCallsPerSession, stats.TotalCalls)), nil
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

	// Auto-scroll feature: scroll down searching for text if not found on first screen
	scrollDirection, _ := getStringParam(request, "scroll_direction")
	maxScrolls := 8
	if n, err := getIntParam(request, "max_scrolls"); err == nil && n > 0 {
		maxScrolls = n
	}
	scrollAmount := 5
	if n, err := getIntParam(request, "scroll_amount"); err == nil && n > 0 {
		scrollAmount = n
	}

	// Multi-page search: navigate between pages while searching
	nextPageKeys, _ := getStringParam(request, "next_page_keys") // e.g., "Page_Down" or "Arrow_Down"
	maxPages := 1
	if n, err := getIntParam(request, "max_pages"); err == nil && n > 0 {
		maxPages = n
	}

	screenW, screenH := robotgo.GetScreenSize()

	// Check region cache first (only if user didn't specify custom region)
	userSpecifiedRegion := false
	regionX := 0
	regionY := 0
	regionW := screenW
	regionH := screenH

	if v, err := getIntParam(request, "x"); err == nil {
		regionX = v
		userSpecifiedRegion = true
	}
	if v, err := getIntParam(request, "y"); err == nil {
		regionY = v
		userSpecifiedRegion = true
	}
	if v, err := getIntParam(request, "width"); err == nil {
		regionW = v
		userSpecifiedRegion = true
	}
	if v, err := getIntParam(request, "height"); err == nil {
		regionH = v
		userSpecifiedRegion = true
	}

	// Try cache lookup only if no custom region specified
	cachedRegion := ""
	if !userSpecifiedRegion {
		normalizedText := normalizeText(searchText)
		if entry, found := regionCache.Get(normalizedText, screenW, screenH); found {
			// Use cached region with small padding
			padding := 20
			regionX = max(0, entry.X-padding)
			regionY = max(0, entry.Y-padding)
			regionW = min(screenW-entry.X, entry.Width+2*padding)
			regionH = min(screenH-entry.Y, entry.Height+2*padding)
			cachedRegion = fmt.Sprintf("cached (%d,%d) %dx%d", entry.X, entry.Y, entry.Width, entry.Height)
			regionCache.RecordHit()
			logging.Info("REGION CACHE HIT: %q -> %s (hits=%d)", normalizedText, cachedRegion, regionCache.GetStats().Hits)
		} else {
			regionCache.RecordMiss()
			logging.Info("REGION CACHE MISS: %q (misses=%d)", normalizedText, regionCache.GetStats().Misses)
		}
	}

	if err := validate.ScreenRegion(regionX, regionY, regionW, regionH, screenW, screenH); err != nil {
		logging.Error("Screen region validation failed: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("invalid screen region: %v", err)), nil
	}

	// Scroll-and-search mode (same page)
	if scrollDirection != "" && (scrollDirection == "down" || scrollDirection == "up") {
		logging.Info("find_and_click: scroll-and-search mode, direction=%s, max_scrolls=%d", scrollDirection, maxScrolls)
		return findAndClickWithScroll(ctx, request, searchText, button, nth, scrollDirection, maxScrolls, scrollAmount, screenW, screenH)
	}

	// Multi-page search mode (different pages/tabs)
	if nextPageKeys != "" || maxPages > 1 {
		logging.Info("find_and_click: multi-page search mode, max_pages=%d, next_keys=%s", maxPages, nextPageKeys)
		return findAndClickMultiPage(ctx, request, searchText, button, nth, nextPageKeys, maxPages, screenW, screenH)
	}

	// Learning-mode fast path: if a learned view exists and no custom region was
	// specified, use the view to narrow the scan to the element's known location.
	// When the element was found on a non-zero scroll page, navigate there first.
	if !userSpecifiedRegion && cachedRegion == "" {
		autoLearnIfNeeded()
		
		// Check if learned view is stale (>60 seconds old) - auto-clear it
		if globalLearner.IsEnabled() && globalLearner.HasView() {
			view := globalLearner.GetView()
			if view != nil && time.Since(view.CapturedAt) > 60*time.Second {
				logging.Info("find_and_click: learned view is stale (%v old), auto-clearing", time.Since(view.CapturedAt))
				globalLearner.ClearView()
				autoLearnIfNeeded()
			}
		}
		
		if lx, ly, lw, lh, scrollsNeeded, ok := learnerRegionHint(searchText, screenW, screenH); ok {
			if scrollsNeeded > 0 {
				logging.Info("find_and_click: learned view — element on scroll page, scrolling down %d ticks then using region (%d,%d) %dx%d",
					scrollsNeeded, lx, ly, lw, lh)
				uiScrollDir(scrollsNeeded, "down")
				time.Sleep(300 * time.Millisecond)
				if err := uiCheckFailsafe(); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
			}
			regionX, regionY, regionW, regionH = lx, ly, lw, lh
			logging.Info("find_and_click: learned view region (%d,%d) %dx%d for %q (scroll_page=%d)",
				regionX, regionY, regionW, regionH, searchText, scrollsNeeded)
		}
	}

	if cachedRegion != "" {
		logging.Info("find_and_click: using %s, scanning region (%d,%d) %dx%d for text %q", cachedRegion, regionX, regionY, regionW, regionH, searchText)
	} else {
		logging.Info("find_and_click: OCR region (%d,%d) %dx%d for text %q", regionX, regionY, regionW, regionH, searchText)
	}

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

		// Get candidates to show what WAS found (helps AI decide next action)
		candidates := getMatchCandidates(searchText, img, grayscale)

		// Record failure and get updated stats
		failStats := tracker.recordCall("find_and_click", searchText, false)

		// Check if we've failed too many times
		var giveUpMessage string
		if failStats.ConsecutiveFailures >= MaxConsecutiveFailures {
			giveUpMessage = fmt.Sprintf(` GIVE UP RECOMMENDATION: Failed %d times with "%s". STOP trying this text. Try: (1) find_elements to see actual text, (2) shorter search term, (3) scroll_direction if off-screen, or (4) completely different approach.`, failStats.ConsecutiveFailures, searchText)
		}

		// Check if we're running out of calls
		remainingWarning := ""
		if failStats.RemainingCalls <= 5 {
			remainingWarning = fmt.Sprintf(` WARNING: Only %d tool calls remaining before forced stop.`, failStats.RemainingCalls)
		}

		// Try text variations before giving up (punctuation removal, word splits)
		if variationResult, found := tryTextVariations(ctx, request, searchText, nth, regionX, regionY, regionW, regionH, screenW, screenH); found {
			return variationResult, nil
		}

		// Check if there are partial matches that might be off-screen
		response := buildFindTextFailureMessage(img, searchText, nth, regionX, regionY, regionW, regionH, grayscale)

		// Add candidates and scroll suggestion
		result := fmt.Sprintf(`{"error":%q,"candidates":%s,"suggestion":%s,"consecutive_failures":%d,"remaining_calls":%d}`,
			response+giveUpMessage+remainingWarning,
			candidates,
			getScrollSuggestion(searchText, candidates),
			failStats.ConsecutiveFailures,
			failStats.RemainingCalls,
		)
		return mcp.NewToolResultError(result), nil
	}

	// Calculate center of the merged button bounds
	cx := regionX + (minX+maxX)/2
	cy := regionY + (minY+maxY)/2

	if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("found text but center out of bounds: %v", err)), nil
	}

	// Update cache with found region (only if not from cache and not user-specified)
	if cachedRegion == "" && !userSpecifiedRegion {
		normalizedText := normalizeText(searchText)
		regionCache.Put(normalizedText, regionX+minX, regionY+minY, maxX-minX, maxY-minY, screenW, screenH)
		logging.Info("REGION CACHE UPDATE: %q -> (%d,%d) %dx%d", normalizedText, regionX+minX, regionY+minY, maxX-minX, maxY-minY)
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

	// Record successful click and check for repetition warning
	clickWarning := tracker.recordClick(finalX, finalY, button, true)

	// Record success for this search term
	tracker.recordCall("find_and_click", searchText, true)

	// Build response (minimal for speed - candidates only on failure)
	response := fmt.Sprintf(
		`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"requested_x":%d,"requested_y":%d,"actual_x":%d,"actual_y":%d,"button":%q,"occurrence":%d}`,
		searchText, minX, minY, maxX-minX, maxY-minY, cx, cy, finalX, finalY, button, nth,
	)

	// Add click warning if clicking same spot too many times
	if clickWarning.ShouldStop {
		response = fmt.Sprintf(`%s,"warning":{"should_stop":true,"reason":%q,"click_count":%d,"message":"You've clicked this spot %d times. Verify this is correct before continuing."}`,
			response[:len(response)-1], clickWarning.Reason, clickWarning.ClickCount, clickWarning.ClickCount)
	}

	return mcp.NewToolResultText(response + "}"), nil
}

// getMatchCandidates returns all potential matches with their scores.
// This helps AI understand which text elements were considered and their confidence.
func getMatchCandidates(searchText string, img image.Image, grayscale bool) string {
	ocrResult, err := ocr.ReadImage(img, ocr.Options{Color: !grayscale})
	if err != nil || ocrResult == nil {
		return "[]"
	}

	needle := strings.ToLower(strings.TrimSpace(searchText))
	needleWords := strings.Fields(needle)

	type candidate struct {
		text       string
		score      int     // Our match quality score (1000=exact, 500=prefix, etc.)
		confidence float64 // Tesseract OCR confidence (0-100)
		x, y       int
		width      int
		height     int
	}
	var candidates []candidate

	for _, w := range ocrResult.Words {
		phrase := strings.ToLower(strings.TrimSpace(w.Text))
		score := scoreMatch(phrase, needle, needleWords)
		if score > 0 {
			candidates = append(candidates, candidate{
				text:       w.Text,
				score:      score,
				confidence: w.Confidence,
				x:          w.X,
				y:          w.Y,
				width:      w.Width,
				height:     w.Height,
			})
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Limit to top 5 candidates
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}

	// Build JSON array
	json := "["
	for i, c := range candidates {
		if i > 0 {
			json += ","
		}
		json += fmt.Sprintf(`{"text":%q,"score":%d,"confidence":%.1f,"x":%d,"y":%d,"width":%d,"height":%d}`,
			c.text, c.score, c.confidence, c.x, c.y, c.width, c.height)
	}
	json += "]"

	return json
}

// getScrollSuggestion analyzes candidates and suggests the best next action.
// Helps AI decide whether to scroll, use different search term, or try multi-page.
func getScrollSuggestion(searchText string, candidates string) string {
	if candidates == "[]" {
		return `"no_matches_found"`
	}

	// Parse candidates
	var parsed []struct {
		Text  string `json:"text"`
		Score int    `json:"score"`
	}
	json.Unmarshal([]byte(candidates), &parsed)

	if len(parsed) == 0 {
		return `"no_matches_found"`
	}

	needle := strings.ToLower(strings.TrimSpace(searchText))

	// Check for partial matches that suggest text might be nearby/off-screen
	for _, c := range parsed {
		candidateLower := strings.ToLower(c.Text)

		// Exact word match but low score = might be truncated/off-screen
		if candidateLower == needle && c.Score < 500 {
			return `"scroll_may_help"`
		}

		// Prefix match = text starts with search term
		if strings.HasPrefix(candidateLower, needle) && len(candidateLower) > len(needle) {
			return `"text_continues_off_screen"`
		}

		// Suffix match = text ends with search term, might be cut off at start
		if strings.HasSuffix(candidateLower, needle) && len(candidateLower) > len(needle) {
			return `"text_continues_off_screen"`
		}

		// Contains as substring
		if strings.Contains(candidateLower, needle) && c.Score < 300 {
			return `"scroll_may_help"`
		}
	}

	// Found related text but not exact match
	maxScore := 0
	for _, c := range parsed {
		if c.Score > maxScore {
			maxScore = c.Score
		}
	}

	if maxScore >= 100 {
		return `"try_different_search_term"`
	}

	return `"text_not_visible_try_scroll_or_different_term"`
}

// findAndClickWithScroll scrolls the screen searching for text, then clicks it.
// Used when text may be off-screen and requires scrolling to find.
func findAndClickWithScroll(
	ctx context.Context,
	request mcp.CallToolRequest,
	searchText, button string,
	nth int,
	scrollDirection string,
	maxScrolls, scrollAmount, screenW, screenH int,
) (*mcp.CallToolResult, error) {

	// Scroll and search loop
	for scroll := 0; scroll <= maxScrolls; scroll++ {
		// Capture current screen
		img, captureErr := robotgo.CaptureImg(0, 0, screenW, screenH)
		if captureErr != nil {
			logging.Error("Failed to capture screen during scroll search: %v", captureErr)
			return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
		}

		grayscale := getBoolParam(request, "grayscale", true)

		// Search for text
		minX, minY, maxX, maxY, found, passName := parallelFindText(ctx, img, searchText, nth, grayscale)
		if found {
			// Calculate click position
			cx := minX + (maxX-minX)/2
			cy := minY + (maxY-minY)/2

			if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("found text but center out of bounds: %v", err)), nil
			}

			// Update cache
			normalizedText := normalizeText(searchText)
			regionCache.Put(normalizedText, minX, minY, maxX-minX, maxY-minY, screenW, screenH)
			logging.Info("REGION CACHE UPDATE: %q -> (%d,%d) %dx%d (after %d scrolls)", normalizedText, minX, minY, maxX-minX, maxY-minY, scroll)

			logging.Info("ACTION: Found %q (occurrence %d) via %s pass after %d scrolls at box (%d,%d)-(%d,%d), clicking center (%d,%d) with %s",
				searchText, nth, passName, scroll, minX, minY, maxX, maxY, cx, cy, button)
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
			logging.Info("ACTION COMPLETE: find_and_click %q at (%d, %d) after %d scrolls", searchText, finalX, finalY, scroll)

			return mcp.NewToolResultText(fmt.Sprintf(
				`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"requested_x":%d,"requested_y":%d,"actual_x":%d,"actual_y":%d,"button":%q,"occurrence":%d,"scrolls":%d}`,
				searchText, minX, minY, maxX-minX, maxY-minY, cx, cy, finalX, finalY, button, nth, scroll,
			)), nil
		}

		// Not found - scroll and continue searching
		if scroll < maxScrolls {
			logging.Info("Text %q not found on screen (scroll %d/%d), scrolling %s", searchText, scroll, maxScrolls, scrollDirection)
			robotgo.Move(screenW/2, scrollAmount*20) // Move to scroll area
			uiScrollDir(scrollAmount, scrollDirection)
			time.Sleep(200 * time.Millisecond) // Wait for scroll animation

			if err := checkFailsafe(); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
	}

	// Text not found after all scrolls
	logging.Info("Text %q not found after %d scrolls", searchText, maxScrolls)
	img, _ := robotgo.CaptureImg(0, 0, screenW, screenH)
	return mcp.NewToolResultError(buildFindTextFailureMessage(img, searchText, nth, 0, 0, screenW, screenH, true)), nil
}

// findAndClickMultiPage searches for text across multiple pages by navigating with keyboard.
// Used when target element may be on a different page, tab, or screen.
func findAndClickMultiPage(
	ctx context.Context,
	request mcp.CallToolRequest,
	searchText, button string,
	nth int,
	nextPageKeys string,
	maxPages int,
	screenW, screenH int,
) (*mcp.CallToolResult, error) {

	selectBest := getBoolParam(request, "select_best", false)

	// Default navigation keys if not specified
	if nextPageKeys == "" {
		nextPageKeys = "Page_Down"
	}
	keys := strings.Split(nextPageKeys, ",")

	// Mode 1: Select best - scan all pages first, collect matches, choose highest score
	if selectBest {
		return findAndClickMultiPageSelectBest(ctx, request, searchText, button, nth, nextPageKeys, maxPages, screenW, screenH, keys)
	}

	// Mode 2: First match - click the first match found (original behavior)
	return findAndClickMultiPageFirstMatch(ctx, request, searchText, button, nth, nextPageKeys, maxPages, screenW, screenH, keys)
}

// findAndClickMultiPageSelectBest scans ALL pages first, collects all matches with scores,
// then navigates to the page with the best match and clicks it.
func findAndClickMultiPageSelectBest(
	ctx context.Context,
	request mcp.CallToolRequest,
	searchText, button string,
	nth int,
	nextPageKeys string,
	maxPages int,
	screenW, screenH int,
	keys []string,
) (*mcp.CallToolResult, error) {

	type pageMatch struct {
		page                   int
		minX, minY, maxX, maxY int
		score                  int
		phrase                 string
	}
	var allMatches []pageMatch

	grayscale := getBoolParam(request, "grayscale", true)

	// Phase 1: Scan all pages and collect matches
	logging.Info("SELECT_BEST mode: scanning all %d pages first", maxPages)

	for page := 0; page < maxPages; page++ {
		// Capture current page
		img, captureErr := robotgo.CaptureImg(0, 0, screenW, screenH)
		if captureErr != nil {
			logging.Error("Failed to capture screen on page %d: %v", page, captureErr)
			return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
		}

		// Get all match candidates with scores
		candidates := getMatchCandidates(searchText, img, grayscale)
		if candidates != "[]" {
			// Parse candidates and add page info
			var parsed []struct {
				Text   string `json:"text"`
				Score  int    `json:"score"`
				X      int    `json:"x"`
				Y      int    `json:"y"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			}
			json.Unmarshal([]byte(candidates), &parsed)

			for _, c := range parsed {
				allMatches = append(allMatches, pageMatch{
					page:   page,
					minX:   c.X,
					minY:   c.Y,
					maxX:   c.X + c.Width,
					maxY:   c.Y + c.Height,
					score:  c.Score,
					phrase: c.Text,
				})
			}
			logging.Info("Page %d: found %d candidates for %q", page, len(parsed), searchText)
		} else {
			logging.Info("Page %d: no matches for %q", page, searchText)
		}

		// Navigate to next page (if not last)
		if page < maxPages-1 {
			for _, key := range keys {
				key = strings.TrimSpace(key)
				if key != "" {
					robotgo.KeyTap(key)
					time.Sleep(300 * time.Millisecond)
				}
			}
			if err := checkFailsafe(); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
	}

	// Check if we found any matches
	if len(allMatches) == 0 {
		logging.Info("No matches found on any of %d pages", maxPages)
		img, _ := robotgo.CaptureImg(0, 0, screenW, screenH)
		return mcp.NewToolResultError(buildFindTextFailureMessage(img, searchText, nth, 0, 0, screenW, screenH, true)), nil
	}

	// Sort by score (highest first)
	sort.Slice(allMatches, func(i, j int) bool {
		return allMatches[i].score > allMatches[j].score
	})

	bestMatch := allMatches[0]
	logging.Info("SELECT_BEST: best match is %q with score %d on page %d", bestMatch.phrase, bestMatch.score, bestMatch.page)

	// Phase 2: Navigate back to the page with best match
	// (Assuming page 0 is the starting page, we need to navigate forward)
	if bestMatch.page > 0 {
		logging.Info("Navigating to page %d for best match", bestMatch.page)
		for p := 0; p < bestMatch.page; p++ {
			for _, key := range keys {
				key = strings.TrimSpace(key)
				if key != "" {
					robotgo.KeyTap(key)
					time.Sleep(300 * time.Millisecond)
				}
			}
		}
		if err := checkFailsafe(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	// Phase 3: Click the best match
	cx := bestMatch.minX + (bestMatch.maxX-bestMatch.minX)/2
	cy := bestMatch.minY + (bestMatch.maxY-bestMatch.minY)/2

	if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("found text but center out of bounds: %v", err)), nil
	}

	// Update cache
	normalizedText := normalizeText(searchText)
	regionCache.Put(normalizedText, bestMatch.minX, bestMatch.minY, bestMatch.maxX-bestMatch.minX, bestMatch.maxY-bestMatch.minY, screenW, screenH)

	logging.Info("ACTION: Clicking best match %q (score %d) on page %d at (%d,%d)", bestMatch.phrase, bestMatch.score, bestMatch.page, cx, cy)
	robotgo.Move(cx, cy)

	if err := checkFailsafe(); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	robotgo.Click(button, false)
	applyClickDelay(request)

	finalX, finalY := robotgo.GetMousePos()
	logging.Info("ACTION COMPLETE: find_and_click %q at (%d, %d) - best of %d candidates across %d pages", searchText, finalX, finalY, len(allMatches), maxPages)

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"requested_x":%d,"requested_y":%d,"actual_x":%d,"actual_y":%d,"button":%q,"occurrence":%d,"page":%d,"score":%d,"total_candidates":%d,"select_best":true}`,
		searchText, bestMatch.minX, bestMatch.minY, bestMatch.maxX-bestMatch.minX, bestMatch.maxY-bestMatch.minY, cx, cy, finalX, finalY, button, nth, bestMatch.page, bestMatch.score, len(allMatches),
	)), nil
}

// findAndClickMultiPageFirstMatch clicks the first match found (original behavior)
func findAndClickMultiPageFirstMatch(
	ctx context.Context,
	request mcp.CallToolRequest,
	searchText, button string,
	nth int,
	nextPageKeys string,
	maxPages int,
	screenW, screenH int,
	keys []string,
) (*mcp.CallToolResult, error) {

	for page := 0; page < maxPages; page++ {
		// Capture and search current page
		img, captureErr := robotgo.CaptureImg(0, 0, screenW, screenH)
		if captureErr != nil {
			logging.Error("Failed to capture screen on page %d: %v", page, captureErr)
			return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
		}

		grayscale := getBoolParam(request, "grayscale", true)
		minX, minY, maxX, maxY, found, passName := parallelFindText(ctx, img, searchText, nth, grayscale)

		if found {
			// Calculate click position
			cx := minX + (maxX-minX)/2
			cy := minY + (maxY-minY)/2

			if err := validate.Coords(cx, cy, screenW, screenH); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("found text but center out of bounds: %v", err)), nil
			}

			// Update cache
			normalizedText := normalizeText(searchText)
			regionCache.Put(normalizedText, minX, minY, maxX-minX, maxY-minY, screenW, screenH)
			logging.Info("REGION CACHE UPDATE: %q -> (%d,%d) %dx%d (found on page %d)", normalizedText, minX, minY, maxX-minX, maxY-minY, page)

			logging.Info("ACTION: Found %q (occurrence %d) via %s pass on page %d at box (%d,%d)-(%d,%d), clicking center (%d,%d) with %s",
				searchText, nth, passName, page, minX, minY, maxX, maxY, cx, cy, button)
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
			logging.Info("ACTION COMPLETE: find_and_click %q at (%d, %d) after checking %d page(s)", searchText, finalX, finalY, page+1)

			return mcp.NewToolResultText(fmt.Sprintf(
				`{"success":true,"found":%q,"box":{"x":%d,"y":%d,"width":%d,"height":%d},"requested_x":%d,"requested_y":%d,"actual_x":%d,"actual_y":%d,"button":%q,"occurrence":%d,"page":%d}`,
				searchText, minX, minY, maxX-minX, maxY-minY, cx, cy, finalX, finalY, button, nth, page,
			)), nil
		}

		// Not found - navigate to next page
		if page < maxPages-1 {
			logging.Info("Text %q not found on page %d/%d, navigating to next page with keys: %v", searchText, page+1, maxPages, keys)

			// Press navigation keys
			for _, key := range keys {
				key = strings.TrimSpace(key)
				if key != "" {
					robotgo.KeyTap(key)
					time.Sleep(300 * time.Millisecond) // Wait for page load
				}
			}

			if err := checkFailsafe(); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
	}

	// Text not found after checking all pages
	logging.Info("Text %q not found after checking %d pages", searchText, maxPages)
	img, _ := robotgo.CaptureImg(0, 0, screenW, screenH)
	return mcp.NewToolResultError(buildFindTextFailureMessage(img, searchText, nth, 0, 0, screenW, screenH, true)), nil
}

// findButtonBounds finds the full bounding box of a button by merging adjacent
// words that match searchText. This handles multi-word buttons like "Save Changes"
// by returning the combined bounding box of all matching words on the same line.
// Returns the merged bounds relative to the OCR image, or false if not found.
func findButtonBounds(ocrResult *ocr.Result, searchText string, nth int) (minX, minY, maxX, maxY int, found bool) {
	needle := strings.ToLower(strings.TrimSpace(searchText))
	needleWords := strings.Fields(needle) // Split into words for smarter matching

	type match struct {
		minX, minY, maxX, maxY int
		score                  int
		phrase                 string
	}
	var matches []match

	for i, w := range ocrResult.Words {
		minX, minY = w.X, w.Y
		maxX, maxY = w.X+w.Width, w.Y+w.Height
		avgHeight := w.Height
		avgWidth := w.Width
		verticalThreshold := avgHeight / 3
		maxHGap := avgWidth * 2 // More generous gap for multi-word buttons
		phrase := strings.ToLower(strings.TrimSpace(w.Text))

		// Score this match
		score := scoreMatch(phrase, needle, needleWords)

		// Try to merge adjacent words on the same line
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
			newScore := scoreMatch(phrase, needle, needleWords)
			if newScore > score {
				score = newScore
			}
		}

		// Only consider matches with positive score
		if score > 0 {
			matches = append(matches, match{minX, minY, maxX, maxY, score, phrase})
		}
	}

	// Sort by score (descending), then by area (prefer smaller, more precise matches)
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		areaI := (matches[i].maxX - matches[i].minX) * (matches[i].maxY - matches[i].minY)
		areaJ := (matches[j].maxX - matches[j].minX) * (matches[j].maxY - matches[j].minY)
		return areaI < areaJ
	})

	// Return the nth best match
	if len(matches) >= nth {
		return matches[nth-1].minX, matches[nth-1].minY, matches[nth-1].maxX, matches[nth-1].maxY, true
	}
	return 0, 0, 0, 0, false
}

// scoreMatch scores how well a phrase matches the search text.
// Higher scores = better matches. Returns 0 for no match.
func scoreMatch(phrase, needle string, needleWords []string) int {
	// Exact match (case-insensitive) = highest score
	if phrase == needle {
		return 1000
	}

	// Starts with needle (e.g., "Click Me!" matches "Click") = high score
	if strings.HasPrefix(phrase, needle) {
		return 500
	}

	// Ends with needle = medium-high score
	if strings.HasSuffix(phrase, needle) {
		return 400
	}

	// Contains needle as a complete word = medium score
	phraseWords := strings.Fields(phrase)
	for _, pw := range phraseWords {
		if pw == needle {
			return 300
		}
	}

	// Multi-word needle: check if all words appear in order
	if len(needleWords) > 1 {
		phraseLower := " " + phrase + " "
		allWordsFound := true
		for _, nw := range needleWords {
			if !strings.Contains(phraseLower, " "+nw+" ") {
				allWordsFound = false
				break
			}
		}
		if allWordsFound {
			return 200
		}
	}

	// Contains needle as substring (weakest match, may be inside another word)
	if strings.Contains(phrase, needle) {
		// Penalize if needle appears to be inside a larger word
		// e.g., "Click" in "Button Click Tests" gets lower score than standalone "Click"
		needleIndex := strings.Index(phrase, needle)
		before := needleIndex > 0 && !isWordBoundary(phrase[needleIndex-1])
		after := needleIndex+len(needle) < len(phrase) && !isWordBoundary(phrase[needleIndex+len(needle)])
		if before || after {
			return 50 // Inside another word
		}
		return 100 // Substring but with word boundaries
	}

	return 0 // No match
}

// isWordBoundary returns true if the character is a word boundary (space, punctuation, etc.)
func isWordBoundary(c byte) bool {
	return c == ' ' || c == '-' || c == '_' || c == '.' || c == ',' || c == '!' || c == '?' || c == ':' || c == ';'
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
	msg += ` TRY THESE: (a) Search for LABEL text like "Text Input:" or "Email:" (not placeholder). (b) Use find_elements to see visible text. (c) Use scroll_until_text for off-screen content. (d) Try a SHORTER substring - if "CLICK ME!" fails, try "Click" (OCR may split spaced text).`

	// Add learning mode suggestion if not already using it
	if globalLearner.IsEnabled() && globalLearner.HasView() {
		msg += ` ⚡ LEARNING MODE ACTIVE: Using cached view. If screen changed, call clear_learned_view then learn_screen to refresh.`
	} else if globalLearner.IsEnabled() && !globalLearner.HasView() {
		msg += ` ⚡ LEARNING MODE ON BUT NO VIEW: Call learn_screen NOW to capture full UI - this will make future searches 10-25x faster and more accurate!`
	} else {
		msg += ` ⚡ NOT USING LEARNING MODE: Call set_learning_mode(enabled=true), then call learn_screen for much better accuracy!`
	}

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

	// Start with detected labels - make them IMPOSSIBLE TO MISS
	msg := ""
	if len(labels) > 0 {
		msg += fmt.Sprintf("LABELS ON SCREEN: %v USE ONE OF THESE AS YOUR SEARCH TERM! ", labels)
	}

	msg += formatFindTextFailureMessage(searchText, nth, regionX, regionY, regionW, regionH, matches)

	// Suggest using find_elements if this is a repeated failure
	if nth > 1 || strings.Contains(searchText, " ") {
		msg += ` (e) Use find_elements to see ALL visible text on screen.`
	}

	// Suggest text variations for common patterns
	if strings.Contains(searchText, "!") || strings.Contains(searchText, "?") {
		// Try without punctuation
		baseText := strings.TrimRight(searchText, "!?")
		msg += fmt.Sprintf(` (f) Try without punctuation: "%s"`, baseText)
	}
	if len(strings.Fields(searchText)) > 1 {
		// Try shorter substrings
		words := strings.Fields(searchText)
		if len(words) >= 2 {
			msg += fmt.Sprintf(` (g) Try shorter: "%s" or "%s"`, words[0], words[len(words)-1])
		}
	}

	// Auto-refresh hint if learning mode is active but view might be stale
	if globalLearner.IsEnabled() && globalLearner.HasView() {
		view := globalLearner.GetView()
		if view != nil && time.Since(view.CapturedAt) > 30*time.Second {
			msg += ` ⚠️ VIEW STALE (captured >30s ago): Call clear_learned_view + learn_screen NOW to refresh!`
		}
	}

	return msg
}

// tryTextVariations attempts to find text with common variations (punctuation, word splits)
// Returns (result, found) - if found is true, result contains the success response
func tryTextVariations(ctx context.Context, request mcp.CallToolRequest, originalText string, nth int, regionX, regionY, regionW, regionH, screenW, screenH int) (*mcp.CallToolResult, bool) {
	variations := []string{}

	// Try without punctuation
	if strings.Contains(originalText, "!") || strings.Contains(originalText, "?") || strings.Contains(originalText, ",") || strings.Contains(originalText, ".") {
		cleanText := strings.TrimRight(originalText, "!?.,")
		if cleanText != originalText && len(cleanText) > 0 {
			variations = append(variations, cleanText)
			logging.Info("tryTextVariations: trying without punctuation: %q", cleanText)
		}
	}

	// Try individual words for multi-word text
	words := strings.Fields(originalText)
	if len(words) > 1 {
		for _, word := range words {
			cleanWord := strings.Trim(word, "!?.,:")
			if len(cleanWord) > 2 && cleanWord != originalText {
				variations = append(variations, cleanWord)
				logging.Info("tryTextVariations: trying word: %q", cleanWord)
			}
		}
	}

	// Get button parameter from request
	button, _ := getStringParam(request, "button")
	if button == "" {
		button = "left"
	}

	// Try each variation
	for _, variation := range variations {
		foundX, foundY, foundW, foundH, found, _ := parallelFindText(ctx, nil, variation, nth, true)
		if found {
			logging.Info("tryTextVariations: FOUND with variation %q at (%d,%d)", variation, foundX, foundY)
			// Update cache with original text but found variation's bounds
			normalizedOriginal := normalizeText(originalText)
			regionCache.Put(normalizedOriginal, foundX, foundY, foundW, foundH, screenW, screenH)
			
			// Click on the found element
			clickX := foundX + foundW/2
			clickY := foundY + foundH/2
			robotgo.Move(clickX, clickY)
			time.Sleep(50 * time.Millisecond)
			if err := uiCheckFailsafe(); err != nil {
				return mcp.NewToolResultError(err.Error()), true
			}
			robotgo.Click(button, false)
			logging.Info("ACTION: Clicked %q (variation of %q) at (%d,%d)", variation, originalText, clickX, clickY)
			return mcp.NewToolResultText(fmt.Sprintf(`{"success":true,"text":%q,"x":%d,"y":%d,"variation":%q}`, originalText, clickX, clickY, variation)), true
		}
	}

	return nil, false
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
	userSpecifiedRegion := false
	if x, err := getIntParam(request, "x"); err == nil {
		regionX = x
		userSpecifiedRegion = true
	}
	if y, err := getIntParam(request, "y"); err == nil {
		regionY = y
		userSpecifiedRegion = true
	}
	if w, err := getIntParam(request, "width"); err == nil {
		regionW = w
		userSpecifiedRegion = true
	}
	if h, err := getIntParam(request, "height"); err == nil {
		regionH = h
		userSpecifiedRegion = true
	}

	// Auto-learn if learning mode is on and no view exists yet.
	if !userSpecifiedRegion {
		autoLearnIfNeeded()
	}
	_ = userSpecifiedRegion // used above, suppress unused warning

	// Capture the region
	img, captureErr := robotgo.CaptureImg(regionX, regionY, regionW, regionH)
	if captureErr != nil {
		logging.Error("Failed to capture screen: %v", captureErr)
		return mcp.NewToolResultError(fmt.Sprintf("failed to capture screen: %v", captureErr)), nil
	}

	saveScreenshotIfKept(img, "ghost-mcp-findelements")

	// Run OCR with grayscale mode for best text detection
	// Grayscale + contrast stretch detects labels better than color mode
	ocrResult, ocrErr := ocr.ReadImage(img, ocr.Options{Color: false})
	if ocrErr != nil {
		logging.Error("OCR failed: %v", ocrErr)
		return mcp.NewToolResultError(fmt.Sprintf("OCR failed: %v", ocrErr)), nil
	}

	// Group words into clickable elements (buttons, links, labels)
	// Filter by confidence and minimum size to avoid noise
	elements := make([]map[string]interface{}, 0)
	for _, w := range ocrResult.Words {
		if w.Confidence < 40 {
			continue // Skip low-confidence detections (lowered from 50 to catch labels)
		}
		if w.Width < 15 || w.Height < 8 {
			continue // Skip tiny text (likely noise) - lowered thresholds for labels
		}

		// Infer element type to help AI identify buttons vs labels vs headings
		elementType := learner.InferElementType(w.Text, w.Width, w.Height)

		elements = append(elements, map[string]interface{}{
			"text":       w.Text,
			"x":          regionX + w.X,
			"y":          regionY + w.Y,
			"width":      w.Width,
			"height":     w.Height,
			"center_x":   regionX + w.X + w.Width/2,
			"center_y":   regionY + w.Y + w.Height/2,
			"confidence": w.Confidence,
			"type":       elementType, // button, label, heading, link, value, text
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

	// Extract labels (words ending with ":") for easy discovery
	labels := findLabelCandidates(ocrResult, 10)
	labelsJSON := "["
	for i, label := range labels {
		if i > 0 {
			labelsJSON += ","
		}
		labelsJSON += fmt.Sprintf(`%q`, label)
	}
	labelsJSON += "]"

	// If learning mode is on, append elements from non-visible scroll pages.
	offPageNote := ""
	offPageElements := "[]"
	if globalLearner.IsEnabled() && globalLearner.HasView() {
		view := globalLearner.GetView()
		var offPage []string
		for _, e := range view.Elements {
			if e.PageIndex > 0 {
				offPage = append(offPage, fmt.Sprintf(
					`{"text":%q,"x":%d,"y":%d,"width":%d,"height":%d,"center_x":%d,"center_y":%d,"confidence":%.1f,"page_index":%d}`,
					e.Text, e.X, e.Y, e.Width, e.Height, e.X+e.Width/2, e.Y+e.Height/2, e.Confidence, e.PageIndex,
				))
			}
		}
		if len(offPage) > 0 {
			offPageElements = "[" + strings.Join(offPage, ",") + "]"
			offPageNote = fmt.Sprintf("(+%d elements from %d scroll page(s) in learned view)", len(offPage), view.PageCount-1)
			logging.Info("find_elements: appending %d off-page elements from learned view", len(offPage))
		}
	}

	// Build JSON response - put labels FIRST so agent sees them immediately
	if offPageNote != "" {
		return mcp.NewToolResultText(fmt.Sprintf(
			`{"success":true,"labels":%s,"label_note":"FIELD LABELS VISIBLE ON SCREEN - search for these exact texts!","element_count":%d,"region":{"x":%d,"y":%d,"width":%d,"height":%d},"elements":%s,"learned_off_page_note":%q,"learned_off_page_elements":%s}`,
			labelsJSON, len(elements), regionX, regionY, regionW, regionH, elementsJSON, offPageNote, offPageElements,
		)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(
		`{"success":true,"labels":%s,"label_note":"FIELD LABELS VISIBLE ON SCREEN - search for these exact texts!","element_count":%d,"region":{"x":%d,"y":%d,"width":%d,"height":%d},"elements":%s}`,
		labelsJSON, len(elements), regionX, regionY, regionW, regionH, elementsJSON,
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
		maxX, maxY = w.X+w.Width, w.Y+w.Height
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
			if next.Y+next.Height > maxY {
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
					labelMaxX, labelMaxY := checkWord.X+checkWord.Width, checkWord.Y+checkWord.Height
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
						if adjWord.Y+adjWord.Height > labelMaxY {
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

	// If not found, try label search BEFORE scrolling (much faster!)
	labelFound := false
	if !found {
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

	// Only scroll if label search also failed
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

// =============================================================================
// REGION CACHE MANAGEMENT TOOLS
// =============================================================================

func handleGetRegionCacheStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling get_region_cache_stats request")

	stats := regionCache.GetStats()
	regionCache.mu.RLock()
	entryCount := len(regionCache.entries)
	regionCache.mu.RUnlock()

	hitRate := float64(0)
	total := stats.Hits + stats.Misses
	if total > 0 {
		hitRate = float64(stats.Hits) / float64(total) * 100
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		`{"entries":%d,"hits":%d,"misses":%d,"hit_rate":%.1f,"evictions":%d,"updates":%d}`,
		entryCount, stats.Hits, stats.Misses, hitRate, stats.Evictions, stats.Updates,
	)), nil
}

func handleClearRegionCache(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling clear_region_cache request")

	stats := regionCache.GetStats()
	regionCache.mu.RLock()
	entryCount := len(regionCache.entries)
	regionCache.mu.RUnlock()
	regionCache.Clear()

	logging.Info("REGION CACHE CLEARED: Had %d entries, %d hits, %d misses", entryCount, stats.Hits, stats.Misses)

	return mcp.NewToolResultText(`{"success":true,"message":"Region cache cleared"}`), nil
}

// =============================================================================
// CLICK WITH VERIFICATION TOOLS
// =============================================================================

// handleClickUntilTextAppears clicks at coordinates and waits for text to appear
func handleClickUntilTextAppears(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	logging.Debug("Handling click_until_text_appears request")

	x, err := getIntParam(request, "x")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid x parameter: %v", err)), nil
	}
	y, err := getIntParam(request, "y")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid y parameter: %v", err)), nil
	}

	waitText, err := getStringParam(request, "wait_for_text")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("wait_for_text is required: %v", err)), nil
	}

	button, _ := getStringParam(request, "button")
	if button == "" {
		button = "left"
	}

	timeoutMS := 5000
	if n, err := getIntParam(request, "timeout_ms"); err == nil && n > 0 {
		timeoutMS = n
	}
	if timeoutMS > 30000 {
		timeoutMS = 30000
	}

	maxClicks := 3
	if n, err := getIntParam(request, "max_clicks"); err == nil && n > 0 {
		maxClicks = n
	}

	screenW, screenH := robotgo.GetScreenSize()
	if err := validate.Coords(x, y, screenW, screenH); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid coordinates: %v", err)), nil
	}

	logging.Info("ACTION: Clicking (%d,%d) waiting for %q (timeout: %dms, max_clicks: %d)", x, y, waitText, timeoutMS, maxClicks)

	// Check for repeated clicks warning
	clickWarning := tracker.recordClick(x, y, button, true)
	if clickWarning.ShouldStop {
		return mcp.NewToolResultError(fmt.Sprintf(`{"error":"%s","suggestion":"You've clicked this spot %d times already. Stop and verify before continuing."}`, clickWarning.Reason, clickWarning.ClickCount)), nil
	}

	// Perform initial click
	robotgo.Move(x, y)
	robotgo.Click(button, false)
	time.Sleep(200 * time.Millisecond)

	// Poll for text appearance
	startTime := time.Now()
	clickCount := 1

	for time.Since(startTime) < time.Duration(timeoutMS)*time.Millisecond {
		// Capture and search for text
		img, err := robotgo.CaptureImg(0, 0, screenW, screenH)
		if err == nil {
			ocrResult, ocrErr := ocr.ReadImage(img, ocr.Options{Color: true})
			if ocrErr == nil && ocrResult != nil {
				// Search for wait text
				needle := strings.ToLower(strings.TrimSpace(waitText))
				for _, w := range ocrResult.Words {
					if strings.Contains(strings.ToLower(w.Text), needle) {
						logging.Info("ACTION COMPLETE: Text %q appeared after %d clicks in %dms", waitText, clickCount, time.Since(startTime).Milliseconds())
						return mcp.NewToolResultText(fmt.Sprintf(
							`{"success":true,"text":%q,"clicks":%d,"waited_ms":%d,"found":true}`,
							waitText, clickCount, time.Since(startTime).Milliseconds(),
						)), nil
					}
				}
			}
		}

		// Text not found - click again if under limit
		if clickCount < maxClicks {
			clickCount++
			robotgo.Move(x, y)
			robotgo.Click(button, false)
			time.Sleep(500 * time.Millisecond)
		} else {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Timeout - text never appeared
	logging.Info("TIMEOUT: Text %q did not appear after %d clicks in %dms", waitText, clickCount, timeoutMS)
	return mcp.NewToolResultError(fmt.Sprintf(
		`{"success":false,"text":%q,"clicks":%d,"waited_ms":%d,"found":false,"error":"Text did not appear after %d clicks. The click may have missed or the expected text is different."}`,
		waitText, clickCount, timeoutMS, clickCount,
	)), nil
}
