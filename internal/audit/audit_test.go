// audit_test.go — Tests for the tamper-evident audit logging system.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// HELPERS
// =============================================================================

func newTestLogger(t *testing.T) *Logger {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvVar, dir)
	al, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(al.Close)
	return al
}

func readEntries(t *testing.T, al *Logger) []Entry {
	t.Helper()
	al.Close()

	pattern := filepath.Join(al.Dir(), "ghost-mcp-audit-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Fatalf("no audit log file found in %s", al.Dir())
	}

	f, err := os.Open(matches[0])
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal entry: %v\nline: %s", err, line)
		}
		entries = append(entries, e)
	}
	return entries
}

func firstLogFile(t *testing.T, al *Logger) string {
	t.Helper()
	al.Close()
	pattern := filepath.Join(al.Dir(), "ghost-mcp-audit-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Fatalf("no audit log file found in %s", al.Dir())
	}
	return matches[0]
}

// =============================================================================
// NEW LOGGER TESTS
// =============================================================================

func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "audit")
	t.Setenv(EnvVar, dir)

	al, err := New()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer al.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Expected directory to be created, but it does not exist")
	}
}

func TestNew_CreatesLogFile(t *testing.T) {
	al := newTestLogger(t)

	pattern := filepath.Join(al.Dir(), "ghost-mcp-audit-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Error("Expected audit log file to be created")
	}
}

func TestNew_FileNameContainsDate(t *testing.T) {
	al := newTestLogger(t)

	today := time.Now().UTC().Format("2006-01-02")
	expected := fmt.Sprintf("ghost-mcp-audit-%s.jsonl", today)

	pattern := filepath.Join(al.Dir(), "ghost-mcp-audit-*.jsonl")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		t.Fatal("No audit log file created")
	}
	if filepath.Base(matches[0]) != expected {
		t.Errorf("Expected file name %q, got %q", expected, filepath.Base(matches[0]))
	}
}

func TestNew_DisabledOnBadDirectory(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, filepath.Join(blocker, "subdir"))

	al, err := New()
	if err == nil {
		t.Error("Expected error for bad directory")
	}
	if al == nil {
		t.Fatal("Expected non-nil disabled logger, got nil")
	}
	al.Log(EventServerStart, "", "", nil) // must not panic
}

// =============================================================================
// LOG ENTRY TESTS
// =============================================================================

func TestLog_WritesEntry(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", map[string]interface{}{"version": "1.0.0"})

	entries := readEntries(t, al)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}
	e := entries[0]

	if e.Sequence != 1 {
		t.Errorf("Expected seq 1, got %d", e.Sequence)
	}
	if e.Event != EventServerStart {
		t.Errorf("Expected event %q, got %q", EventServerStart, e.Event)
	}
	if e.Timestamp == "" {
		t.Error("Expected non-empty timestamp")
	}
	if e.Hash == "" {
		t.Error("Expected non-empty hash")
	}
	if e.Params["version"] != "1.0.0" {
		t.Errorf("Expected param version=1.0.0, got %v", e.Params["version"])
	}
}

func TestLog_SequenceIncrements(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", nil)
	al.Log(EventToolCall, "click", "", nil)
	al.Log(EventToolSuccess, "click", "", nil)

	entries := readEntries(t, al)
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}
	for i, e := range entries {
		if e.Sequence != int64(i+1) {
			t.Errorf("Entry %d: expected seq %d, got %d", i, i+1, e.Sequence)
		}
	}
}

func TestLog_ClientIDIncluded(t *testing.T) {
	al := newTestLogger(t)
	al.SetClientID("claude-desktop")
	al.Log(EventToolCall, "click", "", nil)

	entries := readEntries(t, al)
	if len(entries) == 0 {
		t.Fatal("No entries written")
	}
	if entries[0].ClientID != "claude-desktop" {
		t.Errorf("Expected clientID 'claude-desktop', got %q", entries[0].ClientID)
	}
}

func TestLog_ErrorFieldPopulated(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventAuthFailure, "", "invalid token", nil)

	entries := readEntries(t, al)
	if len(entries) == 0 {
		t.Fatal("No entries written")
	}
	if entries[0].Error != "invalid token" {
		t.Errorf("Expected error 'invalid token', got %q", entries[0].Error)
	}
}

func TestLog_ToolNameIncluded(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventToolCall, "take_screenshot", "", nil)

	entries := readEntries(t, al)
	if len(entries) == 0 {
		t.Fatal("No entries written")
	}
	if entries[0].Tool != "take_screenshot" {
		t.Errorf("Expected tool 'take_screenshot', got %q", entries[0].Tool)
	}
}

// =============================================================================
// HASH CHAIN TESTS
// =============================================================================

func TestHashChain_GenesisHash(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", nil)

	entries := readEntries(t, al)
	if len(entries) == 0 {
		t.Fatal("No entries written")
	}
	genesis := strings.Repeat("0", 64)
	if entries[0].PrevHash != genesis {
		t.Errorf("Expected genesis prev_hash, got %q", entries[0].PrevHash)
	}
}

func TestHashChain_ChainedCorrectly(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", nil)
	al.Log(EventToolCall, "click", "", nil)
	al.Log(EventToolSuccess, "click", "", nil)

	entries := readEntries(t, al)
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].PrevHash != entries[i-1].Hash {
			t.Errorf("Entry %d: prev_hash %q does not match entry %d hash %q",
				i, entries[i].PrevHash, i-1, entries[i-1].Hash)
		}
	}
}

func TestHashChain_HashDiffersPerEntry(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventToolCall, "click", "", nil)
	al.Log(EventToolCall, "type_text", "", nil)

	entries := readEntries(t, al)
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}
	if entries[0].Hash == entries[1].Hash {
		t.Error("Expected different hashes for different entries")
	}
}

// =============================================================================
// VERIFY LOG FILE TESTS
// =============================================================================

func TestVerifyLogFile_ValidFile(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", nil)
	al.Log(EventToolCall, "click", "", nil)
	al.Log(EventToolSuccess, "click", "", nil)

	if err := VerifyLogFile(firstLogFile(t, al)); err != nil {
		t.Errorf("Expected valid file to pass, got error: %v", err)
	}
}

func TestVerifyLogFile_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "audit-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := VerifyLogFile(f.Name()); err != nil {
		t.Errorf("Empty file should be valid, got: %v", err)
	}
}

func TestVerifyLogFile_TamperedHash(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventToolCall, "click", "", nil)
	path := firstLogFile(t, al)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"TOOL_CALL"`, `"TAMPERED"`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0600); err != nil {
		t.Fatal(err)
	}

	if err := VerifyLogFile(path); err == nil {
		t.Error("Expected verification to fail after content tampering")
	}
}

func TestVerifyLogFile_TamperedChain(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", nil)
	al.Log(EventToolCall, "click", "", nil)
	al.Log(EventToolSuccess, "click", "", nil)
	path := firstLogFile(t, al)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatal("Expected at least 3 lines")
	}
	kept := append(lines[:1], lines[2:]...)
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := VerifyLogFile(path); err == nil {
		t.Error("Expected verification to fail after deleting an entry")
	}
}

func TestVerifyLogFile_MissingFile(t *testing.T) {
	if err := VerifyLogFile("/does/not/exist.jsonl"); err == nil {
		t.Error("Expected error for missing file")
	}
}

// =============================================================================
// SANITIZE PARAMS TESTS
// =============================================================================

func TestSanitizeParams_NilInput(t *testing.T) {
	if sanitizeParams(nil) != nil {
		t.Error("Expected nil for nil input")
	}
}

func TestSanitizeParams_EmptyInput(t *testing.T) {
	if sanitizeParams(map[string]interface{}{}) != nil {
		t.Error("Expected nil for empty input")
	}
}

func TestSanitizeParams_ShortStringsPreserved(t *testing.T) {
	out := sanitizeParams(map[string]interface{}{"key": "hello"})
	if out["key"] != "hello" {
		t.Errorf("Expected 'hello', got %v", out["key"])
	}
}

func TestSanitizeParams_LongStringTruncated(t *testing.T) {
	long := strings.Repeat("a", maxParamValueLen+100)
	out := sanitizeParams(map[string]interface{}{"text": long})
	s, ok := out["text"].(string)
	if !ok {
		t.Fatal("Expected string output")
	}
	if len([]rune(s)) >= len([]rune(long)) {
		t.Error("Expected truncated string")
	}
	if !strings.Contains(s, "truncated") {
		t.Error("Expected truncation marker in output")
	}
}

func TestSanitizeParams_NonStringPreserved(t *testing.T) {
	out := sanitizeParams(map[string]interface{}{"x": float64(42)})
	if out["x"] != float64(42) {
		t.Errorf("Expected float64(42), got %v", out["x"])
	}
}

// =============================================================================
// COMPUTE ENTRY HASH TESTS
// =============================================================================

func TestComputeEntryHash_Deterministic(t *testing.T) {
	entry := Entry{
		Sequence:  1,
		Timestamp: "2024-01-01T00:00:00Z",
		Event:     EventToolCall,
		Tool:      "click",
		PrevHash:  strings.Repeat("0", 64),
	}
	if computeEntryHash(entry) != computeEntryHash(entry) {
		t.Error("Expected deterministic hash")
	}
}

func TestComputeEntryHash_ChangesWithContent(t *testing.T) {
	base := Entry{
		Sequence:  1,
		Timestamp: "2024-01-01T00:00:00Z",
		Event:     EventToolCall,
		Tool:      "click",
		PrevHash:  strings.Repeat("0", 64),
	}
	modified := base
	modified.Tool = "move_mouse"
	if computeEntryHash(base) == computeEntryHash(modified) {
		t.Error("Expected different hashes for different content")
	}
}

func TestComputeEntryHash_HashFieldIgnored(t *testing.T) {
	entry := Entry{
		Sequence:  1,
		Timestamp: "2024-01-01T00:00:00Z",
		Event:     EventServerStart,
		PrevHash:  strings.Repeat("0", 64),
		Hash:      "some-old-hash",
	}
	h1 := computeEntryHash(entry)
	entry.Hash = "different-hash"
	if h1 != computeEntryHash(entry) {
		t.Error("Hash field should not influence the computed hash")
	}
}

// =============================================================================
// CLIENT ID TESTS
// =============================================================================

func TestSetGetClientID(t *testing.T) {
	al := newTestLogger(t)
	al.SetClientID("my-client")
	if al.GetClientID() != "my-client" {
		t.Errorf("Expected 'my-client', got %q", al.GetClientID())
	}
}

// =============================================================================
// DISABLED LOGGER TESTS
// =============================================================================

func TestDisabledLogger_DoesNotPanic(t *testing.T) {
	al := &Logger{disabled: true}
	al.Log(EventServerStart, "", "", nil)
	al.SetClientID("x")
	_ = al.GetClientID()
	al.Close()
}
