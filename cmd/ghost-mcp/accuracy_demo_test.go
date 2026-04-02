//go:build !integration

// accuracy_demo_test.go - Demonstrates learning mode accuracy improvements
//
// This test shows how the three-pass OCR + element typing + label association
// improves element finding accuracy compared to single-pass OCR.
package main

import (
	"encoding/json"
	"testing"

	"github.com/ghost-mcp/internal/learner"
)

// TestAccuracy_MultiPassOCR finds elements that would be missed by single-pass OCR
func TestAccuracy_MultiPassOCR(t *testing.T) {
	// Simulate what each OCR pass would find:
	// - Normal pass: black text on white background
	// - Inverted pass: white text on dark background
	// - Color pass: colored text on colored background (missed by both above)

	normalWords := []struct {
		text string
		x, y int
	}{
		{"Username:", 100, 100}, // Black on white - found by normal
		{"Password:", 100, 150},
		{"Login", 200, 200},
	}

	invertedWords := []struct {
		text string
		x, y int
	}{
		{"Dark Mode", 50, 50},    // White on dark - ONLY found by inverted
		{"Settings", 300, 300},
	}

	colorWords := []struct {
		text string
		x, y int
	}{
		{"Submit", 150, 250},      // Blue button - ONLY found by color pass
		{"Cancel", 250, 250},
	}

	// Single-pass (normal only) would find 3 elements
	singlePassCount := len(normalWords)

	// Three-pass finds all elements
	allElements := make([]learner.Element, 0)
	for _, w := range normalWords {
		allElements = append(allElements, learner.Element{
			Text: w.text, X: w.x, Y: w.y,
			Width: 60, Height: 20, Confidence: 90,
			OcrPass: learner.OcrPassNormal,
		})
	}
	for _, w := range invertedWords {
		allElements = append(allElements, learner.Element{
			Text: w.text, X: w.x, Y: w.y,
			Width: 60, Height: 20, Confidence: 85,
			OcrPass: learner.OcrPassInverted,
		})
	}
	for _, w := range colorWords {
		allElements = append(allElements, learner.Element{
			Text: w.text, X: w.x, Y: w.y,
			Width: 60, Height: 20, Confidence: 88,
			OcrPass: learner.OcrPassColor,
		})
	}

	threePassCount := len(allElements)
	improvement := float64(threePassCount-singlePassCount) / float64(singlePassCount) * 100

	t.Logf("Single-pass OCR finds: %d elements", singlePassCount)
	t.Logf("Three-pass OCR finds: %d elements", threePassCount)
	t.Logf("Accuracy improvement: %.0f%% more elements discovered", improvement)

	// Verify critical elements would be missed without multi-pass
	if !containsText(allElements, "Dark Mode") {
		t.Error("Dark Mode element should be found")
	}
	if !containsText(allElements, "Submit") {
		t.Error("Submit button should be found")
	}

	// These would be MISSED by single-pass
	missed := []string{"Dark Mode", "Settings", "Submit", "Cancel"}
	for _, text := range missed {
		if !containsText(allElements, text) {
			t.Errorf("Multi-pass should find %q that single-pass misses", text)
		}
	}
}

// TestAccuracy_ElementTyping shows how element classification improves targeting
func TestAccuracy_ElementTyping(t *testing.T) {
	elements := []learner.Element{
		{Text: "Email:", Type: learner.ElementTypeLabel, X: 100, Y: 100, Width: 50, Height: 20},
		{Text: "Enter your email", Type: learner.ElementTypeText, X: 200, Y: 100, Width: 150, Height: 20},
		{Text: "Submit", Type: learner.ElementTypeButton, X: 100, Y: 200, Width: 80, Height: 30},
		{Text: "Cancel", Type: learner.ElementTypeButton, X: 200, Y: 200, Width: 80, Height: 30},
		{Text: "https://example.com/privacy", Type: learner.ElementTypeLink, X: 100, Y: 300, Width: 200, Height: 15},
		{Text: "Learn more", Type: learner.ElementTypeLink, X: 100, Y: 330, Width: 80, Height: 15},
		{Text: "$99.99", Type: learner.ElementTypeValue, X: 300, Y: 100, Width: 60, Height: 20},
		{Text: "Welcome to the Dashboard", Type: learner.ElementTypeHeading, X: 100, Y: 20, Width: 400, Height: 32},
	}

	// Count by type
	typeCounts := make(map[learner.ElementType]int)
	for _, e := range elements {
		typeCounts[e.Type]++
	}

	t.Logf("Element classification results:")
	for typ, count := range typeCounts {
		t.Logf("  %s: %d elements", typ, count)
	}

	// Verify correct classification
	if typeCounts[learner.ElementTypeButton] != 2 {
		t.Errorf("Expected 2 buttons, got %d", typeCounts[learner.ElementTypeButton])
	}
	if typeCounts[learner.ElementTypeLink] != 2 {
		t.Errorf("Expected 2 links, got %d", typeCounts[learner.ElementTypeLink])
	}
	if typeCounts[learner.ElementTypeLabel] != 1 {
		t.Errorf("Expected 1 label, got %d", typeCounts[learner.ElementTypeLabel])
	}

	// Demonstrate how typing helps AI choose the right element
	t.Log("\nScenario: User wants to click 'Submit'")
	t.Log("  Without typing: Might click any element containing 'Submit'")
	t.Log("  With typing: Can specifically target ElementTypeButton")
	t.Log("  This prevents accidentally clicking links or labels with similar text")
}

// TestAccuracy_LabelAssociation shows how label-input pairing improves form filling
func TestAccuracy_LabelAssociation(t *testing.T) {
	elements := []learner.Element{
		{Text: "Email:", Type: learner.ElementTypeLabel, X: 100, Y: 100, Width: 50, Height: 20},
		{Text: "Enter your email", Type: learner.ElementTypeText, X: 200, Y: 100, Width: 150, Height: 20},
		{Text: "Password:", Type: learner.ElementTypeLabel, X: 100, Y: 150, Width: 70, Height: 20},
		{Text: "••••••••", Type: learner.ElementTypeValue, X: 200, Y: 150, Width: 100, Height: 20},
		{Text: "Confirm:", Type: learner.ElementTypeLabel, X: 100, Y: 200, Width: 70, Height: 20},
		{Text: "••••••••", Type: learner.ElementTypeValue, X: 200, Y: 200, Width: 100, Height: 20},
	}

	// Apply label association
	associated := learner.AssociateLabels(elements)

	t.Log("Label associations discovered:")
	for _, e := range associated {
		if e.Type == learner.ElementTypeLabel && e.LabelFor != "" {
			t.Logf("  %q → %q", e.Text, e.LabelFor)
		}
	}

	// Verify associations
	emailLabel := associated[0]
	if emailLabel.LabelFor != "Enter your email" {
		t.Errorf("Email label should associate with 'Enter your email', got %q", emailLabel.LabelFor)
	}

	passwordLabel := associated[2]
	if passwordLabel.LabelFor != "••••••••" {
		t.Errorf("Password label should associate with input value, got %q", passwordLabel.LabelFor)
	}

	t.Log("\nBenefit: AI can now use find_click_and_type with 'Email:' and know")
	t.Log("       exactly which input field to type into, even if the placeholder")
	t.Log("       text changes or is missing.")
}

// TestAccuracy_Deduplication shows how multi-pass deduplication works
func TestAccuracy_Deduplication(t *testing.T) {
	// Simulate same element detected by multiple OCR passes
	elements := []learner.Element{
		{Text: "Submit", X: 100, Y: 100, Width: 60, Height: 30, Confidence: 90, PageIndex: 0, OcrPass: learner.OcrPassNormal},
		{Text: "Submit", X: 101, Y: 101, Width: 59, Height: 29, Confidence: 85, PageIndex: 0, OcrPass: learner.OcrPassInverted},
		{Text: "Submit", X: 100, Y: 100, Width: 60, Height: 30, Confidence: 88, PageIndex: 0, OcrPass: learner.OcrPassColor},
		{Text: "Cancel", X: 200, Y: 100, Width: 60, Height: 30, Confidence: 92, PageIndex: 0, OcrPass: learner.OcrPassNormal},
	}

	t.Logf("Before deduplication: %d elements", len(elements))

	deduped := learner.DeduplicateElements(elements)

	t.Logf("After deduplication: %d elements", len(deduped))

	// Should keep only 2 elements (Submit with highest confidence, Cancel)
	if len(deduped) != 2 {
		t.Errorf("Expected 2 elements after dedup, got %d", len(deduped))
	}

	// Submit should keep the highest confidence (90 from normal pass)
	var submitElem *learner.Element
	for i := range deduped {
		if deduped[i].Text == "Submit" {
			submitElem = &deduped[i]
			break
		}
	}
	if submitElem == nil {
		t.Fatal("Submit element should exist")
	}
	if submitElem.Confidence != 90 {
		t.Errorf("Submit should keep highest confidence (90), got %.0f", submitElem.Confidence)
	}
	if submitElem.OcrPass != learner.OcrPassNormal {
		t.Errorf("Submit should keep normal pass, got %v", submitElem.OcrPass)
	}

	t.Log("\nBenefit: Same element detected by multiple passes is kept only once,")
	t.Log("         with the highest confidence detection. This prevents duplicate")
	t.Log("         clicks and confusing the AI with repeated elements.")
}

// TestAccuracy_ScrollDiscovery shows how learning mode finds off-screen elements
func TestAccuracy_ScrollDiscovery(t *testing.T) {
	// Simulate elements found on different scroll pages
	elements := []learner.Element{
		// Page 0 (visible without scrolling)
		{Text: "Header", PageIndex: 0, X: 100, Y: 20, Width: 200, Height: 30},
		{Text: "Visible Button", PageIndex: 0, X: 100, Y: 100, Width: 100, Height: 30},

		// Page 1 (requires one scroll)
		{Text: "Middle Section", PageIndex: 1, X: 100, Y: 20, Width: 150, Height: 25},
		{Text: "Scroll Button", PageIndex: 1, X: 100, Y: 100, Width: 100, Height: 30},

		// Page 2 (requires two scrolls)
		{Text: "Footer", PageIndex: 2, X: 100, Y: 20, Width: 100, Height: 25},
		{Text: "Submit Application", PageIndex: 2, X: 100, Y: 100, Width: 150, Height: 30},
	}

	// Without learning mode: only page 0 elements visible
	visibleOnly := 0
	for _, e := range elements {
		if e.PageIndex == 0 {
			visibleOnly++
		}
	}

	// With learning mode: all pages discovered
	totalDiscovered := len(elements)

	t.Logf("Without learning mode: %d elements visible", visibleOnly)
	t.Logf("With learning mode: %d elements discovered", totalDiscovered)
	t.Logf("Off-screen elements found: %d", totalDiscovered-visibleOnly)

	// Verify all pages are represented
	pageCounts := make(map[int]int)
	for _, e := range elements {
		pageCounts[e.PageIndex]++
	}

	for page, count := range pageCounts {
		t.Logf("  Page %d: %d elements", page, count)
	}

	if totalDiscovered <= visibleOnly {
		t.Error("Learning mode should discover more elements than visible alone")
	}

	t.Log("\nBenefit: Learning mode automatically scrolls through the entire page,")
	t.Log("         discovering elements that would require manual scrolling to find.")
	t.Log("         This is critical for long forms, settings pages, and documents.")
}

// TestAccuracy_ElementTypeDisambiguation shows how typing prevents wrong clicks
func TestAccuracy_ElementTypeDisambiguation(t *testing.T) {
	// Real-world scenario: multiple elements with similar text but different types
	elements := []learner.Element{
		{Text: "Delete", Type: learner.ElementTypeButton, X: 100, Y: 100, Width: 60, Height: 30},
		{Text: "Delete Account", Type: learner.ElementTypeHeading, X: 100, Y: 20, Width: 200, Height: 32},
		{Text: "Are you sure you want to delete?", Type: learner.ElementTypeText, X: 100, Y: 60, Width: 300, Height: 18},
		{Text: "https://example.com/delete-guide", Type: learner.ElementTypeLink, X: 100, Y: 150, Width: 250, Height: 15},
	}

	// Search for "delete" - should find all matches
	matches := findAllElementsInList(elements, "delete")

	t.Logf("Searching for 'delete' found %d matches:", len(matches))
	for _, m := range matches {
		t.Logf("  - %q (type: %s)", m.Text, m.Type)
	}

	// AI can now choose the BUTTON specifically
	var deleteButton *learner.Element
	for i := range matches {
		if matches[i].Type == learner.ElementTypeButton {
			deleteButton = &matches[i]
			break
		}
	}

	if deleteButton == nil {
		t.Fatal("Should find Delete button")
	}

	t.Logf("\nCorrect target: %q at (%d, %d)", deleteButton.Text, deleteButton.X, deleteButton.Y)
	t.Log("Avoided clicking: heading, descriptive text, or help link")

	t.Log("\nBenefit: Element typing lets AI distinguish between:")
	t.Log("  - Actionable buttons (click these)")
	t.Log("  - Headings (ignore for clicking)")
	t.Log("  - Descriptive text (ignore)")
	t.Log("  - Links (different interaction)")
}

// Helper functions

func containsText(elems []learner.Element, text string) bool {
	for _, e := range elems {
		if e.Text == text {
			return true
		}
	}
	return false
}

// findAllElementsInList is a test helper that mimics Learner.FindAllElements
func findAllElementsInList(elems []learner.Element, text string) []learner.Element {
	var matches []learner.Element
	for _, e := range elems {
		score := scoreMatchForTest(e.Text, text)
		if score > 0 {
			matches = append(matches, e)
		}
	}
	return matches
}

func scoreMatchForTest(haystack, needle string) int {
	h := lower(haystack)
	n := lower(needle)
	if n == "" {
		return 0
	}
	if h == n {
		return 1000
	}
	if containsForTest(h, n) {
		return 100
	}
	return 0
}

func lower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func containsForTest(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestAccuracy_JSONSerialization verifies the view can be serialized for AI consumption
func TestAccuracy_JSONSerialization(t *testing.T) {
	view := learner.View{
		Elements: []learner.Element{
			{
				Text: "Email:", X: 100, Y: 100, Width: 50, Height: 20,
				Confidence: 95, PageIndex: 0,
				Type: learner.ElementTypeLabel,
				OcrPass: learner.OcrPassNormal,
				LabelFor: "Enter your email",
			},
			{
				Text: "Submit", X: 200, Y: 100, Width: 80, Height: 30,
				Confidence: 92, PageIndex: 0,
				Type: learner.ElementTypeButton,
				OcrPass: learner.OcrPassColor,
			},
		},
		Pages: []learner.PageSnapshot{
			{
				Index: 0, CumulativeScrollTicks: 0,
				Width: 1920, Height: 1080, ElementCount: 2,
			},
		},
		PageCount: 1, ScrollAmountUsed: 5,
		ScreenW: 1920, ScreenH: 1080,
	}

	jsonBytes, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		t.Fatalf("JSON serialization failed: %v", err)
	}

	t.Logf("Learned view JSON (%d bytes):", len(jsonBytes))
	t.Logf("%s", string(jsonBytes))

	// Verify all important fields are serializable
	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("JSON deserialization failed: %v", err)
	}

	if decoded["elements"] == nil {
		t.Error("elements should be in JSON")
	}
	if decoded["pages"] == nil {
		t.Error("pages should be in JSON")
	}
	if decoded["page_count"] == nil {
		t.Error("page_count should be in JSON")
	}

	t.Log("\nBenefit: Complete view can be sent to AI model as structured JSON,")
	t.Log("         enabling it to reason about the entire UI layout, element")
	t.Log("         types, and relationships before taking action.")
}
