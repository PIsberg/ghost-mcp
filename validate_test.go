// validate_test.go — Tests for input validation and sanitisation.
package main

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// Screen dimensions used throughout these tests to avoid CGo calls.
const (
	testScreenW = 1920
	testScreenH = 1080
)

// =============================================================================
// ValidateCoords
// =============================================================================

func TestValidateCoords_Valid(t *testing.T) {
	cases := []struct{ x, y int }{
		{0, 0},
		{100, 200},
		{testScreenW - 1, testScreenH - 1},
		{testScreenW / 2, testScreenH / 2},
	}
	for _, c := range cases {
		if err := ValidateCoords(c.x, c.y, testScreenW, testScreenH); err != nil {
			t.Errorf("ValidateCoords(%d, %d): unexpected error: %v", c.x, c.y, err)
		}
	}
}

func TestValidateCoords_NegativeX(t *testing.T) {
	if err := ValidateCoords(-1, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative x")
	}
}

func TestValidateCoords_NegativeY(t *testing.T) {
	if err := ValidateCoords(100, -1, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative y")
	}
}

func TestValidateCoords_XEqualsWidth(t *testing.T) {
	// x == screenW is out-of-bounds (0-indexed)
	if err := ValidateCoords(testScreenW, 0, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when x equals screen width")
	}
}

func TestValidateCoords_YEqualsHeight(t *testing.T) {
	if err := ValidateCoords(0, testScreenH, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when y equals screen height")
	}
}

func TestValidateCoords_XExceedsWidth(t *testing.T) {
	if err := ValidateCoords(testScreenW+100, 0, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when x exceeds screen width")
	}
}

func TestValidateCoords_YExceedsHeight(t *testing.T) {
	if err := ValidateCoords(0, testScreenH+100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when y exceeds screen height")
	}
}

func TestValidateCoords_BothNegative(t *testing.T) {
	if err := ValidateCoords(-10, -20, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for both negative coordinates")
	}
}

// =============================================================================
// ValidateScreenRegion
// =============================================================================

func TestValidateScreenRegion_Valid(t *testing.T) {
	cases := []struct{ x, y, w, h int }{
		{0, 0, testScreenW, testScreenH},             // full screen
		{0, 0, 1, 1},                                 // minimum region
		{100, 100, 400, 300},                         // interior region
		{0, 0, testScreenW - 1, testScreenH - 1},     // just inside
		{testScreenW - 1, testScreenH - 1, 1, 1},     // bottom-right 1×1
	}
	for _, c := range cases {
		if err := ValidateScreenRegion(c.x, c.y, c.w, c.h, testScreenW, testScreenH); err != nil {
			t.Errorf("ValidateScreenRegion(%d,%d,%d,%d): unexpected error: %v", c.x, c.y, c.w, c.h, err)
		}
	}
}

func TestValidateScreenRegion_NegativeX(t *testing.T) {
	if err := ValidateScreenRegion(-1, 0, 100, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative x")
	}
}

func TestValidateScreenRegion_NegativeY(t *testing.T) {
	if err := ValidateScreenRegion(0, -1, 100, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative y")
	}
}

func TestValidateScreenRegion_ZeroWidth(t *testing.T) {
	if err := ValidateScreenRegion(0, 0, 0, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for zero width")
	}
}

func TestValidateScreenRegion_NegativeWidth(t *testing.T) {
	if err := ValidateScreenRegion(0, 0, -1, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative width")
	}
}

func TestValidateScreenRegion_ZeroHeight(t *testing.T) {
	if err := ValidateScreenRegion(0, 0, 100, 0, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for zero height")
	}
}

func TestValidateScreenRegion_NegativeHeight(t *testing.T) {
	if err := ValidateScreenRegion(0, 0, 100, -1, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative height")
	}
}

func TestValidateScreenRegion_OverflowRight(t *testing.T) {
	// x=1800, w=200 → right edge at 2000 > 1920
	if err := ValidateScreenRegion(1800, 0, 200, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when region overflows right edge")
	}
}

func TestValidateScreenRegion_OverflowBottom(t *testing.T) {
	// y=1000, h=200 → bottom edge at 1200 > 1080
	if err := ValidateScreenRegion(0, 1000, 100, 200, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when region overflows bottom edge")
	}
}

func TestValidateScreenRegion_OverflowBoth(t *testing.T) {
	if err := ValidateScreenRegion(1900, 1000, 100, 200, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when region overflows both edges")
	}
}

func TestValidateScreenRegion_ExactlyFull(t *testing.T) {
	// x+w == screenW and y+h == screenH is valid (non-strict)
	if err := ValidateScreenRegion(0, 0, testScreenW, testScreenH, testScreenW, testScreenH); err != nil {
		t.Errorf("Full-screen region should be valid: %v", err)
	}
}

// =============================================================================
// ValidateText
// =============================================================================

func TestValidateText_Valid(t *testing.T) {
	cases := []string{
		"a",
		"Hello, World!",
		strings.Repeat("x", MaxTextLength),
		"Unicode: 日本語テスト",
		"Emoji: 🎉🚀",
	}
	for _, s := range cases {
		if err := ValidateText(s); err != nil {
			t.Errorf("ValidateText: unexpected error for valid text: %v", err)
		}
	}
}

func TestValidateText_Empty(t *testing.T) {
	if err := ValidateText(""); err == nil {
		t.Error("Expected error for empty text")
	}
}

func TestValidateText_ExactlyAtLimit(t *testing.T) {
	text := strings.Repeat("a", MaxTextLength)
	if err := ValidateText(text); err != nil {
		t.Errorf("Text at exact limit should be valid: %v", err)
	}
}

func TestValidateText_OneOverLimit(t *testing.T) {
	text := strings.Repeat("a", MaxTextLength+1)
	if err := ValidateText(text); err == nil {
		t.Error("Expected error for text one character over limit")
	}
}

func TestValidateText_WayOverLimit(t *testing.T) {
	text := strings.Repeat("z", MaxTextLength*2)
	if err := ValidateText(text); err == nil {
		t.Error("Expected error for text far over limit")
	}
}

func TestValidateText_MultibyteAtLimit(t *testing.T) {
	// Each '日' is 3 bytes but 1 rune — the limit is in runes, not bytes.
	text := strings.Repeat("日", MaxTextLength)
	if err := ValidateText(text); err != nil {
		t.Errorf("Multibyte text at rune limit should be valid: %v", err)
	}
}

func TestValidateText_MultibyteOverLimit(t *testing.T) {
	text := strings.Repeat("日", MaxTextLength+1)
	if err := ValidateText(text); err == nil {
		t.Error("Expected error for multibyte text over rune limit")
	}
}

// =============================================================================
// ValidateKey
// =============================================================================

func TestValidateKey_AllAllowedKeys(t *testing.T) {
	for key := range allowedKeys {
		if err := ValidateKey(key); err != nil {
			t.Errorf("ValidateKey(%q): unexpected error for known key: %v", key, err)
		}
	}
}

func TestValidateKey_CommonKeys(t *testing.T) {
	common := []string{"enter", "tab", "esc", "space", "backspace", "ctrl", "shift", "alt"}
	for _, k := range common {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q): unexpected error: %v", k, err)
		}
	}
}

func TestValidateKey_Letters(t *testing.T) {
	for _, c := range "abcdefghijklmnopqrstuvwxyz" {
		key := string(c)
		if err := ValidateKey(key); err != nil {
			t.Errorf("ValidateKey(%q): unexpected error: %v", key, err)
		}
	}
}

func TestValidateKey_Digits(t *testing.T) {
	for _, c := range "0123456789" {
		key := string(c)
		if err := ValidateKey(key); err != nil {
			t.Errorf("ValidateKey(%q): unexpected error: %v", key, err)
		}
	}
}

func TestValidateKey_FunctionKeys(t *testing.T) {
	for _, key := range []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12"} {
		if err := ValidateKey(key); err != nil {
			t.Errorf("ValidateKey(%q): unexpected error: %v", key, err)
		}
	}
}

func TestValidateKey_UnknownKey(t *testing.T) {
	unknowns := []string{
		"unknown_key",
		"ENTER",      // case-sensitive: uppercase not allowed
		"ctrl+c",     // combinations not supported via this tool
		"",           // empty
		"javascript", // potential injection string
		"; rm -rf /", // shell injection attempt
		"../etc/passwd", // path traversal attempt
	}
	for _, k := range unknowns {
		if err := ValidateKey(k); err == nil {
			t.Errorf("ValidateKey(%q): expected error for invalid key", k)
		}
	}
}

func TestValidateKey_TooLong(t *testing.T) {
	key := strings.Repeat("a", MaxKeyNameLength+1)
	if err := ValidateKey(key); err == nil {
		t.Error("Expected error for key name exceeding max length")
	}
}

func TestValidateKey_ExactlyMaxLength(t *testing.T) {
	// A key name at the length limit but not in the allowlist — should
	// fail with "unknown key", not "too long".
	key := strings.Repeat("x", MaxKeyNameLength)
	err := ValidateKey(key)
	if err == nil {
		t.Error("Expected error (unknown key)")
	}
	if strings.Contains(err.Error(), "too long") {
		t.Errorf("Expected 'unknown key' error, got: %v", err)
	}
}

// =============================================================================
// getIntParam — whole-number enforcement
// =============================================================================

func TestGetIntParam_WholeFloat(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"n": float64(42)},
		},
	}
	v, err := getIntParam(req, "n")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if v != 42 {
		t.Errorf("Expected 42, got %d", v)
	}
}

func TestGetIntParam_FractionalFloat_Rejected(t *testing.T) {
	cases := []float64{1.5, 0.1, 42.9, -1.1, 100.001}
	for _, f := range cases {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{"n": f},
			},
		}
		if _, err := getIntParam(req, "n"); err == nil {
			t.Errorf("getIntParam with %v: expected error for fractional float, got nil", f)
		}
	}
}

func TestGetIntParam_NegativeWholeFloat(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"n": float64(-5)},
		},
	}
	v, err := getIntParam(req, "n")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if v != -5 {
		t.Errorf("Expected -5, got %d", v)
	}
}

func TestGetIntParam_ZeroFloat(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"n": float64(0)},
		},
	}
	v, err := getIntParam(req, "n")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if v != 0 {
		t.Errorf("Expected 0, got %d", v)
	}
}

// =============================================================================
// Handler-level validation (no CGo — only tests the validation branch)
// =============================================================================

func TestHandleTypeText_EmptyText(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"text": ""},
		},
	}
	// getStringParam succeeds for empty string, but ValidateText should reject it.
	result, err := handleTypeText(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error result for empty text")
	}
}

func TestHandleTypeText_TooLong(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"text": strings.Repeat("a", MaxTextLength+1)},
		},
	}
	result, err := handleTypeText(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error result for oversized text")
	}
}

func TestHandlePressKey_UnknownKey(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"key": "not_a_real_key"},
		},
	}
	result, err := handlePressKey(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error result for unknown key")
	}
}

func TestHandlePressKey_InjectionAttempt(t *testing.T) {
	malicious := []string{"; rm -rf /", "$(whoami)", "../../../etc/passwd", "ctrl+alt+del"}
	for _, key := range malicious {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]interface{}{"key": key},
			},
		}
		result, err := handlePressKey(nil, req)
		if err != nil {
			t.Fatalf("Handler returned unexpected Go error for %q: %v", key, err)
		}
		if !result.IsError {
			t.Errorf("Expected tool error for injection attempt %q", key)
		}
	}
}

func TestHandleMoveMouse_FractionalCoords(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"x": 100.5, "y": float64(200)},
		},
	}
	result, err := handleMoveMouse(nil, req)
	if err != nil {
		t.Fatalf("Handler returned unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected tool error for fractional x coordinate")
	}
}

