// Package validate provides input validation for Ghost MCP tool parameters.
//
// Every exported Validate* function returns a descriptive error on failure and
// nil on success. Handlers call these after parameter extraction and before any
// robotgo call, ensuring invalid input never reaches the OS automation layer.
package validate

import (
	"fmt"
	"unicode/utf8"
)

// =============================================================================
// LIMITS
// =============================================================================

const (
	// MaxTextLength is the maximum number of Unicode code points accepted by
	// the type_text tool. Prevents accidental or malicious exhaustion of input
	// buffers in target applications.
	MaxTextLength = 10_000

	// MaxKeyNameLength is a pre-allowlist guard: key names longer than this are
	// rejected immediately without a map lookup.
	MaxKeyNameLength = 32
)

// =============================================================================
// COORDINATE VALIDATION
// =============================================================================

// Coords checks that (x, y) fall within the screen rectangle
// [0, screenW) × [0, screenH).
func Coords(x, y, screenW, screenH int) error {
	if x < 0 {
		return fmt.Errorf("x coordinate %d is negative", x)
	}
	if y < 0 {
		return fmt.Errorf("y coordinate %d is negative", y)
	}
	if x >= screenW {
		return fmt.Errorf("x coordinate %d exceeds screen width %d", x, screenW)
	}
	if y >= screenH {
		return fmt.Errorf("y coordinate %d exceeds screen height %d", y, screenH)
	}
	return nil
}

// ScreenRegion checks that the rectangle (x, y, w, h) lies entirely within a
// screen of dimensions screenW × screenH, and that w and h are positive.
func ScreenRegion(x, y, w, h, screenW, screenH int) error {
	if x < 0 {
		return fmt.Errorf("screenshot x %d is negative", x)
	}
	if y < 0 {
		return fmt.Errorf("screenshot y %d is negative", y)
	}
	if w <= 0 {
		return fmt.Errorf("screenshot width %d must be positive", w)
	}
	if h <= 0 {
		return fmt.Errorf("screenshot height %d must be positive", h)
	}
	if x+w > screenW {
		return fmt.Errorf("screenshot region (x=%d, width=%d) extends beyond screen width %d", x, w, screenW)
	}
	if y+h > screenH {
		return fmt.Errorf("screenshot region (y=%d, height=%d) extends beyond screen height %d", y, h, screenH)
	}
	return nil
}

// =============================================================================
// TEXT VALIDATION
// =============================================================================

// Text checks that s is non-empty and within MaxTextLength Unicode code points.
// The rune count is used (not byte length) so multi-byte characters are counted
// naturally.
func Text(s string) error {
	if s == "" {
		return fmt.Errorf("text must not be empty")
	}
	n := utf8.RuneCountInString(s)
	if n > MaxTextLength {
		return fmt.Errorf("text length %d exceeds maximum %d characters", n, MaxTextLength)
	}
	return nil
}

// =============================================================================
// KEY VALIDATION
// =============================================================================

// Key checks that key is a member of the robotgo key allowlist.
// This prevents arbitrary strings from reaching the OS key-tap API.
func Key(key string) error {
	if len(key) > MaxKeyNameLength {
		return fmt.Errorf("key name too long (max %d characters)", MaxKeyNameLength)
	}
	if !allowedKeys[key] {
		return fmt.Errorf("unknown key %q — see documentation for the list of supported keys", key)
	}
	return nil
}

// allowedKeys is the set of key names accepted by Key.
// Sourced from the robotgo key constants; extended with common aliases.
var allowedKeys = map[string]bool{
	// Lowercase letters
	"a": true, "b": true, "c": true, "d": true, "e": true, "f": true,
	"g": true, "h": true, "i": true, "j": true, "k": true, "l": true,
	"m": true, "n": true, "o": true, "p": true, "q": true, "r": true,
	"s": true, "t": true, "u": true, "v": true, "w": true, "x": true,
	"y": true, "z": true,

	// Digit keys (top row)
	"0": true, "1": true, "2": true, "3": true, "4": true,
	"5": true, "6": true, "7": true, "8": true, "9": true,

	// Function keys
	"f1": true, "f2": true, "f3": true, "f4": true,
	"f5": true, "f6": true, "f7": true, "f8": true,
	"f9": true, "f10": true, "f11": true, "f12": true,

	// Navigation & editing
	"enter": true, "return": true,
	"tab": true,
	"esc": true, "escape": true,
	"backspace": true,
	"delete":    true, "del": true,
	"space": true,
	"home":  true, "end": true,
	"pageup": true, "pagedown": true,
	"insert": true,

	// Arrow keys
	"up": true, "down": true, "left": true, "right": true,

	// Modifier keys
	"shift": true, "ctrl": true, "alt": true,
	"cmd": true, "command": true, // macOS
	"super": true, "windows": true, "win": true, // Linux / Windows
	"option": true, // macOS alias for alt

	// Lock / system keys
	"capslock": true, "numlock": true, "scrolllock": true,
	"printscreen": true, "print": true, "pause": true,

	// Numpad
	"num0": true, "num1": true, "num2": true, "num3": true, "num4": true,
	"num5": true, "num6": true, "num7": true, "num8": true, "num9": true,
	"num_lock":    true,
	"num_decimal": true, "num_period": true,
	"num_plus": true, "num_minus": true, "num_multiply": true, "num_divide": true,
	"num_enter": true,

	// Punctuation (robotgo names)
	"minus": true, "equal": true,
	"leftbracket": true, "rightbracket": true,
	"backslash": true,
	"semicolon": true, "quote": true,
	"grave": true, "tilde": true,
	"comma": true, "dot": true, "period": true, "slash": true,
}
