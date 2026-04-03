// Package learner provides the learning mode feature for ghost-mcp.
//
// Learning mode performs a full GUI reconnaissance before acting: it takes
// screenshots and runs OCR across multiple scroll positions to build a
// complete internal view of the current interface. The view combines three
// layers of understanding:
//
//  1. OCR text — every readable word from three OCR passes (normal, inverted,
//     color) merged and deduplicated, each with coordinates and confidence.
//  2. Element typing — heuristic classification of each element into button,
//     label, heading, link, value, or text based on size and content.
//  3. Visual screenshots — a compressed JPEG of each scroll page, retrievable
//     later so an AI model can reason about visual layout and non-text content.
//
// Thread safety: all exported methods on Learner are safe for concurrent use.
package learner

import (
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// =============================================================================
// Element types
// =============================================================================

// ElementType classifies a UI element inferred from its size and text content.
type ElementType string

const (
	ElementTypeUnknown  ElementType = "unknown"
	ElementTypeButton   ElementType = "button"   // clickable action element
	ElementTypeLabel    ElementType = "label"    // field label (usually ends with ":")
	ElementTypeInput    ElementType = "input"    // text input field (placeholder text)
	ElementTypeCheckbox ElementType = "checkbox" // checkbox (☐ ☑ ✓ [ ] [x])
	ElementTypeRadio    ElementType = "radio"    // radio button (○ ● ◉)
	ElementTypeDropdown ElementType = "dropdown" // dropdown/select (▼ Select...)
	ElementTypeToggle   ElementType = "toggle"   // toggle switch (ON/OFF)
	ElementTypeSlider   ElementType = "slider"   // slider/range control (◄ ► ───●───)
	ElementTypeHeading  ElementType = "heading"  // section/page heading
	ElementTypeLink     ElementType = "link"     // hyperlink or navigation text
	ElementTypeValue    ElementType = "value"    // numeric or status value
	ElementTypeText     ElementType = "text"     // general body text
)

// OcrPass identifies which OCR preprocessing pass detected an element.
type OcrPass string

const (
	OcrPassNormal     OcrPass = "normal"      // grayscale + contrast stretch
	OcrPassInverted   OcrPass = "inverted"    // brightness inversion (white-on-dark)
	OcrPassBrightText OcrPass = "bright_text" // isolates near-white pixels
	OcrPassColor      OcrPass = "color"       // full colour (coloured-background buttons)
)

// =============================================================================
// Core data types
// =============================================================================

// Element represents a UI text element discovered during screen learning.
type Element struct {
	Text       string  `json:"text"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Confidence float64 `json:"confidence"`
	// PageIndex is the scroll-page on which this element was captured.
	// 0 = top of the screen (no scrolling), 1 = after one scroll step, etc.
	PageIndex int `json:"page_index"`
	// Type is the inferred element classification.
	Type ElementType `json:"type"`
	// OcrPass is which preprocessing pass first detected this element.
	OcrPass OcrPass `json:"ocr_pass"`
	// LabelFor is the text of the nearest input element this label describes,
	// populated by AssociateLabels. Empty when no association was found.
	LabelFor string `json:"label_for,omitempty"`
}

// PageSnapshot stores a compressed screenshot of one scroll page alongside
// summary information. JPEG bytes are held in memory but not serialised to
// JSON (tagged json:"-") to keep get_learned_view responses compact.
// Use get_page_screenshot to retrieve the visual data for a specific page.
type PageSnapshot struct {
	Index                 int `json:"index"`
	CumulativeScrollTicks int `json:"cumulative_scroll_ticks"`
	Width                 int `json:"width"`
	Height                int `json:"height"`
	ElementCount          int `json:"element_count"`
	// JPEG holds the compressed screenshot. Not included in JSON responses.
	JPEG []byte `json:"-"`
}

// View is the combined internal representation of the GUI built by scanning
// screenshots and OCR results across multiple scroll positions.
type View struct {
	Elements         []Element      `json:"elements"`
	Pages            []PageSnapshot `json:"pages"`
	PageCount        int            `json:"page_count"`
	ScrollAmountUsed int            `json:"scroll_amount_used"` // wheel clicks per step
	CapturedAt       time.Time      `json:"captured_at"`
	ScreenW          int            `json:"screen_w"`
	ScreenH          int            `json:"screen_h"`
}

// =============================================================================
// Learner
// =============================================================================

// Learner stores the learned view and provides element lookup.
// It is safe for concurrent use.
type Learner struct {
	mu      sync.RWMutex
	view    *View
	enabled bool
}

// New returns a new Learner with learning mode disabled.
func New() *Learner {
	return &Learner{}
}

// Enable enables learning mode.
func (l *Learner) Enable() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = true
}

// Disable disables learning mode.
func (l *Learner) Disable() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = false
}

// IsEnabled reports whether learning mode is active.
func (l *Learner) IsEnabled() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.enabled
}

// GetView returns the current learned view. Returns nil if not yet learned.
func (l *Learner) GetView() *View {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.view
}

// SetView replaces the current learned view with v.
func (l *Learner) SetView(v *View) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.view = v
}

// ClearView discards the current learned view.
func (l *Learner) ClearView() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.view = nil
}

// HasView reports whether a learned view is currently available.
func (l *Learner) HasView() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.view != nil
}

// GetPageScreenshot returns the stored JPEG bytes for the given page index,
// or nil if the page does not exist or has no screenshot.
func (l *Learner) GetPageScreenshot(pageIndex int) []byte {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.view == nil {
		return nil
	}
	for _, p := range l.view.Pages {
		if p.Index == pageIndex {
			return p.JPEG
		}
	}
	return nil
}

// FindElement returns the best-matching element for the given search text,
// or nil if the view is empty or no match is found.
// Matching is case-insensitive and scored: exact > prefix > suffix > substring.
func (l *Learner) FindElement(text string) *Element {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.view == nil || len(l.view.Elements) == 0 {
		return nil
	}
	return findBestMatch(l.view.Elements, text)
}

// FindAllElements returns all elements that match the given text,
// ordered by descending match score then ascending page index.
func (l *Learner) FindAllElements(text string) []Element {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.view == nil {
		return nil
	}
	return findAllMatches(l.view.Elements, text)
}

// AllElements returns a copy of all elements in the view.
// Returns nil if no view has been learned yet.
func (l *Learner) AllElements() []Element {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.view == nil {
		return nil
	}
	out := make([]Element, len(l.view.Elements))
	copy(out, l.view.Elements)
	return out
}

// =============================================================================
// Element type inference
// =============================================================================

// InferElementType classifies a UI element using its text content and size.
// Heuristics (in priority order):
//  1. Ends with ":" → label
//  2. Known button keyword → button (regardless of size)
//  3. URL-like or known link phrase → link
//  4. Pure numeric / currency / percentage → value
//  5. Large height (>28px), few words → heading
//  6. Short text with button proportions → button
//  7. Everything else → text
func InferElementType(text string, width, height int) ElementType {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ElementTypeUnknown
	}
	lower := strings.ToLower(trimmed)
	words := strings.Fields(trimmed)

	// Label: ends with a colon (English or full-width).
	if strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, ":") {
		return ElementTypeLabel
	}

	// Button: exact match on a common action keyword (highest priority after label).
	if isButtonKeyword(lower) {
		return ElementTypeButton
	}

	// Link: URL or well-known link phrase.
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "www.") {
		return ElementTypeLink
	}
	for _, phrase := range []string{"click here", "learn more", "see all", "read more", "view all", "show more"} {
		if lower == phrase || strings.HasSuffix(lower, " "+phrase) {
			return ElementTypeLink
		}
	}

	// Value: numeric, percentage, currency.
	if isNumericValue(trimmed) {
		return ElementTypeValue
	}

	// Dropdown: dropdown symbols or select/choose patterns (check before input).
	if isDropdownText(lower) {
		return ElementTypeDropdown
	}

	// Input field: common placeholder text patterns.
	if isInputPlaceholder(lower) {
		return ElementTypeInput
	}

	// Checkbox: checkbox symbols or agreement text.
	if isCheckboxText(lower) {
		return ElementTypeCheckbox
	}

	// Radio button: radio symbols or option selection text.
	if isRadioText(lower) {
		return ElementTypeRadio
	}

	// Toggle: ON/OFF or enabled/disabled text (check before button).
	if isToggleText(lower) {
		return ElementTypeToggle
	}

	// Slider: slider symbols or range/volume/brightness text.
	if isSliderText(lower) {
		return ElementTypeSlider
	}

	// Heading: tall text (>28px), few words (max 8), and NOT a button keyword.
	if height > 28 && len(words) <= 8 {
		return ElementTypeHeading
	}

	// Button: short label (max 5 words) with button-proportioned bounding box.
	// Typical button heights: 16-65px, minimum width 40px.
	if len(words) <= 5 && width >= 40 && height >= 16 && height <= 65 {
		return ElementTypeButton
	}

	return ElementTypeText
}

// isInputPlaceholder returns true for common input field placeholder text.
func isInputPlaceholder(s string) bool {
	// Common placeholder patterns (excluding dropdown patterns)
	placeholders := []string{
		"enter your", "type here", "type your", "input your",
		"email...", "password...",
		"username...", "name...", "phone...", "address...",
		"city...", "state...", "zip...", "country...",
		"message...", "comment...", "notes...", "description...",
	}
	for _, ph := range placeholders {
		if strings.Contains(s, ph) {
			return true
		}
	}
	// Also check for common single-word placeholders (excluding dropdown/button words)
	if s == "email" || s == "password" || s == "username" || s == "name" ||
		s == "phone" || s == "address" || s == "city" || s == "message" ||
		s == "comment" {
		return true
	}
	return false
}

// isCheckboxText returns true for checkbox text or symbols.
func isCheckboxText(s string) bool {
	// Check for checkbox symbols
	if strings.ContainsAny(s, "☐☑✓✗✘√[]") {
		return true
	}
	// Check for common checkbox patterns
	patterns := []string{
		"[ ]", "[x]", "[✓]", "[✔]",
		"i agree", "accept", "agree to", "terms",
		"subscribe", "remember me", "keep me",
	}
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// isRadioText returns true for radio button text or symbols.
func isRadioText(s string) bool {
	// Check for radio symbols
	if strings.ContainsAny(s, "○●◉◎") {
		return true
	}
	// Radio buttons often have short option labels
	// This is harder to detect without context, so we check for common patterns
	patterns := []string{
		"option ", "choice ", "select this",
	}
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// isDropdownText returns true for dropdown/select text or symbols.
func isDropdownText(s string) bool {
	// Check for dropdown symbols
	if strings.ContainsAny(s, "▼▾◢⌄⌵") {
		return true
	}
	// Check for common dropdown patterns
	patterns := []string{
		"select...", "choose...", "pick...",
		"select one", "choose one", "pick one",
		"-- select", "-- choose", "-- pick",
		"dropdown", "menu",
	}
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// isToggleText returns true for toggle switch text.
func isToggleText(s string) bool {
	// Common toggle patterns (exact matches only to avoid button conflicts)
	toggles := []string{
		"on", "off",
		"enabled", "disabled",
		"active", "inactive",
	}
	for _, t := range toggles {
		if s == t {
			return true
		}
	}
	return false
}

// isSliderText returns true for slider/range control text or symbols.
func isSliderText(s string) bool {
	// Check for slider symbols (horizontal line with handle)
	if strings.Contains(s, "─●") || strings.Contains(s, "▬●") || strings.Contains(s, "│●") {
		return true
	}
	// Check for common slider patterns
	patterns := []string{
		"volume", "brightness", "contrast",
		"zoom", "speed", "opacity",
		"range", "level", "intensity",
		"min", "max",
	}
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	// Check for percentage (common with sliders)
	if strings.HasSuffix(s, "%") {
		return true
	}
	return false
}

// isButtonKeyword returns true for common UI action words.
func isButtonKeyword(s string) bool {
	keywords := []string{
		"ok", "yes", "no", "cancel", "close", "dismiss", "done",
		"submit", "send", "save", "save as", "save all",
		"delete", "remove", "discard", "clear",
		"back", "next", "previous", "continue", "finish",
		"confirm", "apply", "accept", "reject", "deny",
		"login", "log in", "logout", "log out",
		"sign in", "sign out", "sign up", "register",
		"create", "new", "add", "edit", "update", "duplicate",
		"copy", "cut", "paste", "undo", "redo",
		"open", "browse", "upload", "download", "export", "import",
		"search", "find", "filter", "sort", "refresh", "reload",
		"retry", "try again", "reset", "restore",
		"expand", "collapse", "toggle",
		"more", "less", "show", "hide",
		"select all", "deselect all",
	}
	for _, kw := range keywords {
		if s == kw || s == kw+"..." || s == kw+" »" || s == "« "+kw {
			return true
		}
	}
	return false
}

// isNumericValue returns true when s consists only of digits and common
// numeric punctuation (decimal, thousands separator, sign, currency).
func isNumericValue(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '.', ',', '-', '+', '%', '$', '€', '£', '¥', '₹', '/', '\\':
			continue
		default:
			return false
		}
	}
	return true
}

// =============================================================================
// Label→input association
// =============================================================================

// AssociateLabels pairs label elements with the nearest non-label element
// to their right or immediately below, filling LabelFor with that element's
// text. Returns a new slice; the original is not modified.
//
// This helps AI understand form structure: knowing that "Email:" is the label
// for "Enter your email" lets it use find_click_and_type precisely.
func AssociateLabels(elements []Element) []Element {
	result := make([]Element, len(elements))
	copy(result, elements)

	for i, e := range result {
		if e.Type != ElementTypeLabel {
			continue
		}
		if target := nearestInputText(result, e, i); target != "" {
			result[i].LabelFor = target
		}
	}
	return result
}

// nearestInputText finds the closest non-label element to the right of or
// directly below label, on the same scroll page.
func nearestInputText(elements []Element, label Element, labelIdx int) string {
	const maxHorizGap = 400 // pixels — label to input horizontal distance
	const maxVertGap = 60   // pixels — label to input vertical distance

	labelCY := label.Y + label.Height/2
	bestDist := maxHorizGap + maxVertGap + 1
	bestText := ""

	for i, e := range elements {
		if i == labelIdx || e.Type == ElementTypeLabel || e.PageIndex != label.PageIndex {
			continue
		}
		eCY := e.Y + e.Height/2

		// Candidate to the RIGHT on the same horizontal band.
		horizGap := e.X - (label.X + label.Width)
		if horizGap >= 0 && horizGap <= maxHorizGap && abs(eCY-labelCY) <= label.Height {
			dist := horizGap
			if dist < bestDist {
				bestDist = dist
				bestText = e.Text
			}
			continue
		}

		// Candidate BELOW within a small vertical gap.
		vertGap := e.Y - (label.Y + label.Height)
		if vertGap >= 0 && vertGap <= maxVertGap && abs(e.X-label.X) <= label.Width+50 {
			dist := maxHorizGap + vertGap // weight vertical lower than horizontal
			if dist < bestDist {
				bestDist = dist
				bestText = e.Text
			}
		}
	}
	return bestText
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// =============================================================================
// Deduplication
// =============================================================================

// DeduplicateElements removes elements with identical text and overlapping
// bounding boxes on the same page, keeping the copy with the highest
// confidence. Elements on different pages are always kept separately.
func DeduplicateElements(elements []Element) []Element {
	if len(elements) == 0 {
		return elements
	}

	// Sort so highest-confidence comes first.
	sorted := make([]Element, len(elements))
	copy(sorted, elements)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Confidence > sorted[j].Confidence
	})

	out := make([]Element, 0, len(sorted))
	for _, candidate := range sorted {
		duplicate := false
		for _, kept := range out {
			if kept.PageIndex != candidate.PageIndex {
				continue
			}
			if !strings.EqualFold(kept.Text, candidate.Text) {
				continue
			}
			if rectsOverlap(kept.X, kept.Y, kept.Width, kept.Height,
				candidate.X, candidate.Y, candidate.Width, candidate.Height) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, candidate)
		}
	}
	return out
}

// rectsOverlap reports whether two axis-aligned rectangles overlap.
func rectsOverlap(ax, ay, aw, ah, bx, by, bw, bh int) bool {
	return ax < bx+bw && ax+aw > bx &&
		ay < by+bh && ay+ah > by
}

// =============================================================================
// Matching helpers
// =============================================================================

func findBestMatch(elements []Element, text string) *Element {
	needle := strings.ToLower(strings.TrimSpace(text))
	if needle == "" {
		return nil
	}
	bestScore := 0
	bestIdx := -1
	for i := range elements {
		s := ScoreMatch(elements[i].Text, needle)
		if s > bestScore {
			bestScore = s
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil
	}
	e := elements[bestIdx]
	return &e
}

func findAllMatches(elements []Element, text string) []Element {
	needle := strings.ToLower(strings.TrimSpace(text))
	if needle == "" {
		return nil
	}
	type scored struct {
		elem  Element
		score int
	}
	var matches []scored
	for _, e := range elements {
		s := ScoreMatch(e.Text, needle)
		if s > 0 {
			matches = append(matches, scored{e, s})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].elem.PageIndex < matches[j].elem.PageIndex
	})
	result := make([]Element, len(matches))
	for i, m := range matches {
		result[i] = m.elem
	}
	return result
}

// ScoreMatch scores how well haystack matches needle (case-insensitive).
// needle must already be lowercase. Returns 0 if no match.
//
//	1000 = exact match
//	 500 = haystack starts with needle
//	 400 = haystack ends with needle
//	 100 = haystack contains needle as substring
func ScoreMatch(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	h := strings.ToLower(strings.TrimSpace(haystack))
	switch {
	case h == needle:
		return 1000
	case strings.HasPrefix(h, needle):
		return 500
	case strings.HasSuffix(h, needle):
		return 400
	case strings.Contains(h, needle):
		return 100
	default:
		return 0
	}
}
