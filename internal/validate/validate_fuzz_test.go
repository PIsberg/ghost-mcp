package validate

import (
	"testing"
)

// FuzzText tests the Text validation function with random strings.
func FuzzText(f *testing.F) {
	seeds := []string{"", "short", "very long string with special characters!@#$%^&*()", "😀😁😂🤣", "invalid \xff\xfe utf8"}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		err := Text(s)
		// We expect error if s is empty or exceeds MaxTextLength.
		// Otherwise, it should pass.
		// We can't easily check for MaxTextLength here without repeating logic, 
		// but we can ensure it doesn't panic.
		_ = err
	})
}

// FuzzKey tests the Key validation function.
func FuzzKey(f *testing.F) {
	seeds := []string{"a", "enter", "shift", "unknown", "toolongkeynameover32characterslong"}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, key string) {
		err := Key(key)
		// Ensure no panics.
		_ = err
	})
}

// FuzzCoords tests the Coords validation function.
func FuzzCoords(f *testing.F) {
	f.Add(100, 100, 1920, 1080)
	f.Add(-1, 0, 1920, 1080)
	f.Add(2000, 2000, 1920, 1080)
	f.Fuzz(func(t *testing.T, x, y, screenW, screenH int) {
		err := Coords(x, y, screenW, screenH)
		// Ensure no panics.
		_ = err
	})
}
