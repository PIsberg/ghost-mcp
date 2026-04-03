package audit

import (
	"os"
	"testing"
)

// BenchmarkLog measures the cost of writing one audit entry including the
// SHA-256 hash computation and JSONL serialization.
func BenchmarkLog(b *testing.B) {
	dir := b.TempDir()
	b.Setenv(EnvVar, dir)

	al, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer al.Close()

	params := map[string]interface{}{
		"text": "Hello",
		"x":    100,
		"y":    200,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		al.Log(EventToolCall, "find_and_click", "", params)
	}
}

// BenchmarkLog_NoParams measures the minimal log path (no parameters).
func BenchmarkLog_NoParams(b *testing.B) {
	dir := b.TempDir()
	b.Setenv(EnvVar, dir)

	al, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer al.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		al.Log(EventToolSuccess, "click", "", nil)
	}
}

// BenchmarkComputeEntryHash measures the SHA-256 hash chain computation
// that runs on every written entry.
func BenchmarkComputeEntryHash(b *testing.B) {
	entry := Entry{
		Sequence:  42,
		Timestamp: "2026-04-03T12:00:00Z",
		Event:     EventToolCall,
		Tool:      "find_and_click",
		Params: map[string]interface{}{
			"text": "Submit",
			"x":    960,
			"y":    540,
		},
		ClientID: "claude",
		PrevHash: "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeEntryHash(entry)
	}
}

// BenchmarkSanitizeParams measures parameter sanitization for typical tool calls.
func BenchmarkSanitizeParams(b *testing.B) {
	params := map[string]interface{}{
		"text":   "Click the Submit button",
		"x":      960,
		"y":      540,
		"region": "full",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sanitizeParams(params)
	}
}

// BenchmarkSanitizeParams_LongValue measures truncation of a long parameter value.
func BenchmarkSanitizeParams_LongValue(b *testing.B) {
	long := make([]byte, maxParamValueLen*2)
	for i := range long {
		long[i] = 'x'
	}
	params := map[string]interface{}{
		"text": string(long),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sanitizeParams(params)
	}
}

// BenchmarkVerifyLogFile measures hash-chain verification of a small log file.
func BenchmarkVerifyLogFile(b *testing.B) {
	dir := b.TempDir()
	b.Setenv(EnvVar, dir)

	al, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	// Write 20 entries so there's something to verify.
	for i := 0; i < 20; i++ {
		al.Log(EventToolCall, "find_and_click", "", map[string]interface{}{"text": "OK"})
	}
	al.Close()

	// Find the log file just written.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		b.Fatal("no audit log file created")
	}
	logPath := dir + "/" + entries[0].Name()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := VerifyLogFile(logPath); err != nil {
			b.Fatalf("VerifyLogFile: %v", err)
		}
	}
}
