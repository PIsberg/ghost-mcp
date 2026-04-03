package validate

import "testing"

// BenchmarkCoords_Valid measures the hot-path: valid coordinates accepted
// on every tool call that takes (x, y) arguments.
func BenchmarkCoords_Valid(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Coords(960, 540, 1920, 1080)
	}
}

// BenchmarkCoords_Invalid measures the rejection path.
func BenchmarkCoords_Invalid(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Coords(-1, 540, 1920, 1080)
	}
}

// BenchmarkScreenRegion_Valid measures region validation on the happy path.
func BenchmarkScreenRegion_Valid(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ScreenRegion(100, 100, 800, 600, 1920, 1080)
	}
}

// BenchmarkText_Short measures validation of a short string (common case).
func BenchmarkText_Short(b *testing.B) {
	s := "Hello"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Text(s)
	}
}

// BenchmarkText_Long measures validation of a long string near the limit.
func BenchmarkText_Long(b *testing.B) {
	// Build a string with 5000 ASCII characters (half the limit).
	s := make([]byte, 5000)
	for i := range s {
		s[i] = 'a'
	}
	str := string(s)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Text(str)
	}
}

// BenchmarkKey_Valid measures allowlist lookup for a common key.
func BenchmarkKey_Valid(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Key("enter")
	}
}

// BenchmarkKey_Invalid measures rejection of an unknown key name.
func BenchmarkKey_Invalid(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Key("notakey")
	}
}
