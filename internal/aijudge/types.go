// Package aijudge provides AI-powered evaluation of Ghost MCP's GUI element
// identification accuracy. It uses the Gemini Vision API as an independent
// "judge" to verify how well the OCR + heuristic pipeline detects and classifies
// on-screen GUI elements.
package aijudge

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// Core types
// =============================================================================

// Rect is an axis-aligned bounding box in pixel coordinates.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Center returns the center point of the rectangle.
func (r Rect) Center() (cx, cy int) {
	return r.X + r.Width/2, r.Y + r.Height/2
}

// Area returns the area of the rectangle.
func (r Rect) Area() int {
	if r.Width <= 0 || r.Height <= 0 {
		return 0
	}
	return r.Width * r.Height
}

// JudgedElement is an element identified by the AI judge (Gemini).
type JudgedElement struct {
	Text string `json:"text"`
	Type string `json:"type"` // button, label, input, heading, link, checkbox, radio, dropdown, toggle, slider, icon, text
	Rect Rect   `json:"rect"`
}

// GhostElement is a Ghost MCP element adapted for comparison.
type GhostElement struct {
	ID         int     `json:"id"`
	Text       string  `json:"text"`
	Type       string  `json:"type"`
	Rect       Rect    `json:"rect"`
	Confidence float64 `json:"confidence"`
	OcrPass    string  `json:"ocr_pass"`
}

// ElementMatch pairs a Ghost element with its matching Judge element.
type ElementMatch struct {
	Ghost     GhostElement  `json:"ghost"`
	Judge     JudgedElement `json:"judge"`
	TextScore float64       `json:"text_score"` // 0..1, how well text matches
	IoU       float64       `json:"iou"`        // intersection over union of bounding boxes
	TypeMatch bool          `json:"type_match"` // whether element type agrees
}

// TypeMismatch records a case where both pipelines found the same element
// but classified it differently.
type TypeMismatch struct {
	Text      string `json:"text"`
	GhostType string `json:"ghost_type"`
	JudgeType string `json:"judge_type"`
}

// =============================================================================
// Accuracy report
// =============================================================================

// AccuracyReport is the structured output of comparing Ghost MCP's element
// detection against the AI judge's independent analysis.
type AccuracyReport struct {
	Timestamp      time.Time       `json:"timestamp"`
	FixtureName    string          `json:"fixture_name"`
	GhostCount     int             `json:"ghost_count"`   // elements Ghost MCP found
	JudgeCount     int             `json:"judge_count"`   // elements Gemini found
	MatchedCount   int             `json:"matched_count"` // elements both found
	Precision      float64         `json:"precision"`     // matched / ghost_count
	Recall         float64         `json:"recall"`        // matched / judge_count
	F1             float64         `json:"f1"`            // harmonic mean of P & R
	TypeAccuracy   float64         `json:"type_accuracy"` // % of matched elems with correct type
	Matches        []ElementMatch  `json:"matches"`
	MissedByGhost  []JudgedElement `json:"missed_by_ghost"` // judge found, ghost missed
	FalsePositives []GhostElement  `json:"false_positives"` // ghost found, judge didn't see
	TypeMismatches []TypeMismatch  `json:"type_mismatches"`
}

// String returns a human-readable markdown report.
func (r *AccuracyReport) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# AI Judge Accuracy Report: %s\n\n", r.FixtureName))
	b.WriteString(fmt.Sprintf("**Timestamp:** %s\n\n", r.Timestamp.Format(time.RFC3339)))

	b.WriteString("## Summary\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	b.WriteString(fmt.Sprintf("| Ghost MCP elements | %d |\n", r.GhostCount))
	b.WriteString(fmt.Sprintf("| AI Judge elements | %d |\n", r.JudgeCount))
	b.WriteString(fmt.Sprintf("| Matched | %d |\n", r.MatchedCount))
	b.WriteString(fmt.Sprintf("| Precision | %.1f%% |\n", r.Precision*100))
	b.WriteString(fmt.Sprintf("| Recall | %.1f%% |\n", r.Recall*100))
	b.WriteString(fmt.Sprintf("| F1 Score | %.1f%% |\n", r.F1*100))
	b.WriteString(fmt.Sprintf("| Type Accuracy | %.1f%% |\n", r.TypeAccuracy*100))

	if len(r.MissedByGhost) > 0 {
		b.WriteString("\n## Missed by Ghost MCP\n\n")
		b.WriteString("| Text | Type | Location |\n")
		b.WriteString("|------|------|----------|\n")
		for _, e := range r.MissedByGhost {
			b.WriteString(fmt.Sprintf("| %s | %s | (%d,%d) %dx%d |\n",
				e.Text, e.Type, e.Rect.X, e.Rect.Y, e.Rect.Width, e.Rect.Height))
		}
	}

	if len(r.FalsePositives) > 0 {
		b.WriteString("\n## False Positives (Ghost only)\n\n")
		b.WriteString("| Text | Type | Confidence |\n")
		b.WriteString("|------|------|------------|\n")
		for _, e := range r.FalsePositives {
			b.WriteString(fmt.Sprintf("| %s | %s | %.0f |\n",
				e.Text, e.Type, e.Confidence))
		}
	}

	if len(r.TypeMismatches) > 0 {
		b.WriteString("\n## Type Mismatches\n\n")
		b.WriteString("| Text | Ghost Type | Judge Type |\n")
		b.WriteString("|------|-----------|------------|\n")
		for _, m := range r.TypeMismatches {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
				m.Text, m.GhostType, m.JudgeType))
		}
	}

	return b.String()
}

// =============================================================================
// Comparison engine
// =============================================================================

// CompareConfig controls comparison thresholds.
type CompareConfig struct {
	// MinTextScore is the minimum text similarity (0..1) to consider a match.
	// Default: 0.5
	MinTextScore float64

	// MinIoU is the minimum bounding box IoU to consider a spatial match.
	// Set to 0 to disable spatial matching (text-only).
	// Default: 0.0 (text-only matching by default since Gemini bbox estimates are rough)
	MinIoU float64
}

// DefaultCompareConfig returns sensible defaults.
func DefaultCompareConfig() CompareConfig {
	return CompareConfig{
		MinTextScore: 0.5,
		MinIoU:       0.0, // text-only by default; Gemini's bbox estimates are approximate
	}
}

// CompareResults matches Ghost MCP elements against AI judge elements
// and produces an accuracy report.
func CompareResults(fixtureName string, ghostElems []GhostElement, judgeElems []JudgedElement, cfg CompareConfig) *AccuracyReport {
	report := &AccuracyReport{
		Timestamp:   time.Now(),
		FixtureName: fixtureName,
		GhostCount:  len(ghostElems),
		JudgeCount:  len(judgeElems),
	}

	if len(ghostElems) == 0 && len(judgeElems) == 0 {
		report.Precision = 1.0
		report.Recall = 1.0
		report.F1 = 1.0
		report.TypeAccuracy = 1.0
		return report
	}

	// Build a similarity matrix: ghost[i] × judge[j] → score
	type candidate struct {
		ghostIdx  int
		judgeIdx  int
		textScore float64
		iou       float64
		typeMatch bool
	}

	var candidates []candidate
	for i, g := range ghostElems {
		for j, jj := range judgeElems {
			ts := textSimilarity(g.Text, jj.Text)
			iou := computeIoU(g.Rect, jj.Rect)

			if ts >= cfg.MinTextScore && iou >= cfg.MinIoU {
				candidates = append(candidates, candidate{
					ghostIdx:  i,
					judgeIdx:  j,
					textScore: ts,
					iou:       iou,
					typeMatch: normalizeType(g.Type) == normalizeType(jj.Type),
				})
			}
		}
	}

	// Greedy matching: sort by text score desc, then prefer type-matching
	// candidates (so when two judge elements share the same text, the one whose
	// type also matches wins), then IoU desc.
	sort.Slice(candidates, func(a, b int) bool {
		if candidates[a].textScore != candidates[b].textScore {
			return candidates[a].textScore > candidates[b].textScore
		}
		if candidates[a].typeMatch != candidates[b].typeMatch {
			return candidates[a].typeMatch
		}
		return candidates[a].iou > candidates[b].iou
	})

	matchedGhost := make(map[int]bool)
	matchedJudge := make(map[int]bool)
	typeCorrect := 0

	for _, c := range candidates {
		if matchedGhost[c.ghostIdx] || matchedJudge[c.judgeIdx] {
			continue
		}
		matchedGhost[c.ghostIdx] = true
		matchedJudge[c.judgeIdx] = true

		g := ghostElems[c.ghostIdx]
		j := judgeElems[c.judgeIdx]
		typeMatch := normalizeType(g.Type) == normalizeType(j.Type)

		report.Matches = append(report.Matches, ElementMatch{
			Ghost:     g,
			Judge:     j,
			TextScore: c.textScore,
			IoU:       c.iou,
			TypeMatch: typeMatch,
		})

		if typeMatch {
			typeCorrect++
		} else {
			report.TypeMismatches = append(report.TypeMismatches, TypeMismatch{
				Text:      g.Text,
				GhostType: g.Type,
				JudgeType: j.Type,
			})
		}
	}

	report.MatchedCount = len(report.Matches)

	// Missed by Ghost (judge found, ghost didn't)
	for j, elem := range judgeElems {
		if !matchedJudge[j] {
			report.MissedByGhost = append(report.MissedByGhost, elem)
		}
	}

	// False positives (ghost found, judge didn't)
	for i, elem := range ghostElems {
		if !matchedGhost[i] {
			report.FalsePositives = append(report.FalsePositives, elem)
		}
	}

	// Compute metrics
	if report.GhostCount > 0 {
		report.Precision = float64(report.MatchedCount) / float64(report.GhostCount)
	}
	if report.JudgeCount > 0 {
		report.Recall = float64(report.MatchedCount) / float64(report.JudgeCount)
	}
	if report.Precision+report.Recall > 0 {
		report.F1 = 2 * report.Precision * report.Recall / (report.Precision + report.Recall)
	}
	if report.MatchedCount > 0 {
		report.TypeAccuracy = float64(typeCorrect) / float64(report.MatchedCount)
	}

	return report
}

// =============================================================================
// Helpers
// =============================================================================

// computeIoU calculates intersection-over-union of two rectangles.
func computeIoU(a, b Rect) float64 {
	// Intersection
	x1 := max(a.X, b.X)
	y1 := max(a.Y, b.Y)
	x2 := min(a.X+a.Width, b.X+b.Width)
	y2 := min(a.Y+a.Height, b.Y+b.Height)

	if x2 <= x1 || y2 <= y1 {
		return 0.0
	}

	intersection := float64((x2 - x1) * (y2 - y1))
	union := float64(a.Area()+b.Area()) - intersection
	if union <= 0 {
		return 0.0
	}
	return intersection / union
}

// textSimilarity returns a 0..1 score of how similar two strings are.
// Uses case-insensitive containment with length ratio.
func textSimilarity(a, b string) float64 {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))

	if a == "" || b == "" {
		return 0.0
	}
	if a == b {
		return 1.0
	}

	// Check containment in both directions
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter := len(a)
		longer := len(b)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		return 0.5 + 0.5*float64(shorter)/float64(longer)
	}

	// Trigram similarity as a fallback for OCR errors
	return trigramSimilarity(a, b)
}

// trigramSimilarity computes the Dice coefficient over character trigrams.
func trigramSimilarity(a, b string) float64 {
	triA := trigrams(a)
	triB := trigrams(b)

	if len(triA) == 0 || len(triB) == 0 {
		return 0.0
	}

	common := 0
	for t := range triA {
		if _, ok := triB[t]; ok {
			common++
		}
	}

	return 2.0 * float64(common) / float64(len(triA)+len(triB))
}

// trigrams returns the set of character trigrams in s.
func trigrams(s string) map[string]struct{} {
	runes := []rune(s)
	if len(runes) < 3 {
		return nil
	}
	result := make(map[string]struct{})
	for i := 0; i <= len(runes)-3; i++ {
		result[string(runes[i:i+3])] = struct{}{}
	}
	return result
}

// normalizeType maps element types to a canonical form for comparison.
// Both Ghost MCP and Gemini use similar type names, but there may be
// minor variations.
func normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "button", "btn":
		return "button"
	case "label", "lbl":
		return "label"
	case "input", "text_input", "textinput", "text_field":
		return "input"
	case "heading", "header", "h1", "h2", "h3", "title":
		return "heading"
	case "link", "hyperlink", "anchor":
		return "link"
	case "checkbox", "check", "check_box":
		return "checkbox"
	case "radio", "radio_button", "radiobutton":
		return "radio"
	case "dropdown", "select", "combobox", "combo_box", "drop_down":
		return "dropdown"
	case "toggle", "switch", "toggle_switch":
		return "toggle"
	case "slider", "range", "range_input":
		return "slider"
	case "icon", "image", "img":
		return "icon"
	case "text", "paragraph", "body", "span", "static_text":
		return "text"
	case "value", "number", "numeric":
		return "value"
	default:
		return t
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NormalizeTypeExported is the exported version of normalizeType for use in tests.
func NormalizeTypeExported(t string) string {
	return normalizeType(t)
}
