// Package learner provides the learning mode feature for ghost-mcp.
//
// Learning mode performs a full GUI reconnaissance before acting: it takes
// screenshots and runs OCR across multiple scroll positions to build a
// complete internal view of the current interface. Subsequent tool calls can
// look up elements in this cached view instead of re-scanning the full screen.
//
// Thread safety: all exported methods on Learner are safe for concurrent use.
package learner

import (
	"sort"
	"strings"
	"sync"
	"time"
)

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
}

// View is the combined internal representation of the GUI built by scanning
// screenshots and OCR results across multiple scroll positions.
type View struct {
	Elements          []Element `json:"elements"`
	PageCount         int       `json:"page_count"`
	ScrollAmountUsed  int       `json:"scroll_amount_used"`  // wheel clicks per scroll step
	CapturedAt        time.Time `json:"captured_at"`
	ScreenW           int       `json:"screen_w"`
	ScreenH           int       `json:"screen_h"`
}

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
// Internal matching helpers (no locks, no exported state)
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

// DeduplicateElements removes elements with identical text and overlapping
// bounding boxes, keeping the higher-confidence copy. Elements on different
// pages are kept even if they share text.
func DeduplicateElements(elements []Element) []Element {
	if len(elements) == 0 {
		return elements
	}

	// Sort so highest-confidence comes first within each (text, page) group.
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
			// Same page, same text: check bounding box overlap.
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
