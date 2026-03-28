// validate_test.go — Tests for input validation and sanitisation.
package validate

import (
	"strings"
	"testing"
)

// Screen dimensions used throughout these tests to avoid CGo calls.
const (
	testScreenW = 1920
	testScreenH = 1080
)

// =============================================================================
// Coords
// =============================================================================

func TestCoords_Valid(t *testing.T) {
	cases := []struct{ x, y int }{
		{0, 0},
		{100, 200},
		{testScreenW - 1, testScreenH - 1},
		{testScreenW / 2, testScreenH / 2},
	}
	for _, c := range cases {
		if err := Coords(c.x, c.y, testScreenW, testScreenH); err != nil {
			t.Errorf("Coords(%d, %d): unexpected error: %v", c.x, c.y, err)
		}
	}
}

func TestCoords_NegativeX(t *testing.T) {
	if err := Coords(-1, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative x")
	}
}

func TestCoords_NegativeY(t *testing.T) {
	if err := Coords(100, -1, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative y")
	}
}

func TestCoords_XEqualsWidth(t *testing.T) {
	if err := Coords(testScreenW, 0, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when x equals screen width")
	}
}

func TestCoords_YEqualsHeight(t *testing.T) {
	if err := Coords(0, testScreenH, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when y equals screen height")
	}
}

func TestCoords_XExceedsWidth(t *testing.T) {
	if err := Coords(testScreenW+100, 0, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when x exceeds screen width")
	}
}

func TestCoords_YExceedsHeight(t *testing.T) {
	if err := Coords(0, testScreenH+100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when y exceeds screen height")
	}
}

func TestCoords_BothNegative(t *testing.T) {
	if err := Coords(-10, -20, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for both negative coordinates")
	}
}

// =============================================================================
// ScreenRegion
// =============================================================================

func TestScreenRegion_Valid(t *testing.T) {
	cases := []struct{ x, y, w, h int }{
		{0, 0, testScreenW, testScreenH},         // full screen
		{0, 0, 1, 1},                             // minimum region
		{100, 100, 400, 300},                     // interior region
		{0, 0, testScreenW - 1, testScreenH - 1}, // just inside
		{testScreenW - 1, testScreenH - 1, 1, 1}, // bottom-right 1×1
	}
	for _, c := range cases {
		if err := ScreenRegion(c.x, c.y, c.w, c.h, testScreenW, testScreenH); err != nil {
			t.Errorf("ScreenRegion(%d,%d,%d,%d): unexpected error: %v", c.x, c.y, c.w, c.h, err)
		}
	}
}

func TestScreenRegion_NegativeX(t *testing.T) {
	if err := ScreenRegion(-1, 0, 100, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative x")
	}
}

func TestScreenRegion_NegativeY(t *testing.T) {
	if err := ScreenRegion(0, -1, 100, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative y")
	}
}

func TestScreenRegion_ZeroWidth(t *testing.T) {
	if err := ScreenRegion(0, 0, 0, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for zero width")
	}
}

func TestScreenRegion_NegativeWidth(t *testing.T) {
	if err := ScreenRegion(0, 0, -1, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative width")
	}
}

func TestScreenRegion_ZeroHeight(t *testing.T) {
	if err := ScreenRegion(0, 0, 100, 0, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for zero height")
	}
}

func TestScreenRegion_NegativeHeight(t *testing.T) {
	if err := ScreenRegion(0, 0, 100, -1, testScreenW, testScreenH); err == nil {
		t.Error("Expected error for negative height")
	}
}

func TestScreenRegion_OverflowRight(t *testing.T) {
	if err := ScreenRegion(1800, 0, 200, 100, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when region overflows right edge")
	}
}

func TestScreenRegion_OverflowBottom(t *testing.T) {
	if err := ScreenRegion(0, 1000, 100, 200, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when region overflows bottom edge")
	}
}

func TestScreenRegion_OverflowBoth(t *testing.T) {
	if err := ScreenRegion(1900, 1000, 100, 200, testScreenW, testScreenH); err == nil {
		t.Error("Expected error when region overflows both edges")
	}
}

func TestScreenRegion_ExactlyFull(t *testing.T) {
	if err := ScreenRegion(0, 0, testScreenW, testScreenH, testScreenW, testScreenH); err != nil {
		t.Errorf("Full-screen region should be valid: %v", err)
	}
}

// =============================================================================
// Text
// =============================================================================

func TestText_Valid(t *testing.T) {
	cases := []string{
		"a",
		"Hello, World!",
		strings.Repeat("x", MaxTextLength),
		"Unicode: 日本語テスト",
		"Emoji: 🎉🚀",
	}
	for _, s := range cases {
		if err := Text(s); err != nil {
			t.Errorf("Text: unexpected error for valid text: %v", err)
		}
	}
}

func TestText_Empty(t *testing.T) {
	if err := Text(""); err == nil {
		t.Error("Expected error for empty text")
	}
}

func TestText_ExactlyAtLimit(t *testing.T) {
	if err := Text(strings.Repeat("a", MaxTextLength)); err != nil {
		t.Errorf("Text at exact limit should be valid: %v", err)
	}
}

func TestText_OneOverLimit(t *testing.T) {
	if err := Text(strings.Repeat("a", MaxTextLength+1)); err == nil {
		t.Error("Expected error for text one character over limit")
	}
}

func TestText_WayOverLimit(t *testing.T) {
	if err := Text(strings.Repeat("z", MaxTextLength*2)); err == nil {
		t.Error("Expected error for text far over limit")
	}
}

func TestText_MultibyteAtLimit(t *testing.T) {
	// Each '日' is 3 bytes but 1 rune — the limit is in runes, not bytes.
	if err := Text(strings.Repeat("日", MaxTextLength)); err != nil {
		t.Errorf("Multibyte text at rune limit should be valid: %v", err)
	}
}

func TestText_MultibyteOverLimit(t *testing.T) {
	if err := Text(strings.Repeat("日", MaxTextLength+1)); err == nil {
		t.Error("Expected error for multibyte text over rune limit")
	}
}

// =============================================================================
// Key
// =============================================================================

func TestKey_AllAllowedKeys(t *testing.T) {
	for key := range allowedKeys {
		if err := Key(key); err != nil {
			t.Errorf("Key(%q): unexpected error for known key: %v", key, err)
		}
	}
}

func TestKey_CommonKeys(t *testing.T) {
	common := []string{"enter", "tab", "esc", "space", "backspace", "ctrl", "shift", "alt"}
	for _, k := range common {
		if err := Key(k); err != nil {
			t.Errorf("Key(%q): unexpected error: %v", k, err)
		}
	}
}

func TestKey_Letters(t *testing.T) {
	for _, c := range "abcdefghijklmnopqrstuvwxyz" {
		key := string(c)
		if err := Key(key); err != nil {
			t.Errorf("Key(%q): unexpected error: %v", key, err)
		}
	}
}

func TestKey_Digits(t *testing.T) {
	for _, c := range "0123456789" {
		key := string(c)
		if err := Key(key); err != nil {
			t.Errorf("Key(%q): unexpected error: %v", key, err)
		}
	}
}

func TestKey_FunctionKeys(t *testing.T) {
	for _, key := range []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12"} {
		if err := Key(key); err != nil {
			t.Errorf("Key(%q): unexpected error: %v", key, err)
		}
	}
}

func TestKey_UnknownKey(t *testing.T) {
	unknowns := []string{
		"unknown_key",
		"ENTER",         // case-sensitive: uppercase not allowed
		"ctrl+c",        // combinations not supported via this tool
		"",              // empty
		"javascript",    // potential injection string
		"; rm -rf /",    // shell injection attempt
		"../etc/passwd", // path traversal attempt
	}
	for _, k := range unknowns {
		if err := Key(k); err == nil {
			t.Errorf("Key(%q): expected error for invalid key", k)
		}
	}
}

func TestKey_TooLong(t *testing.T) {
	if err := Key(strings.Repeat("a", MaxKeyNameLength+1)); err == nil {
		t.Error("Expected error for key name exceeding max length")
	}
}

func TestKey_ExactlyMaxLength(t *testing.T) {
	// At the length limit but not in the allowlist — should fail with
	// "unknown key", not "too long".
	key := strings.Repeat("x", MaxKeyNameLength)
	err := Key(key)
	if err == nil {
		t.Error("Expected error (unknown key)")
	}
	if strings.Contains(err.Error(), "too long") {
		t.Errorf("Expected 'unknown key' error, got: %v", err)
	}
}
