// audit_test.go - Tests for the tamper-evident audit logging system
package main

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

// newTestLogger creates an AuditLogger writing to a temporary directory.
func newTestLogger(t *testing.T) *AuditLogger {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(AuditEnvVar, dir)
	al, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	t.Cleanup(al.Close)
	return al
}

// readEntries reads all AuditEntry records from the first .jsonl file found
// in the logger's directory. It closes the logger first to flush the file.
func readEntries(t *testing.T, al *AuditLogger) []AuditEntry {
	t.Helper()
	al.Close()

	pattern := filepath.Join(al.LogDir(), "ghost-mcp-audit-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Fatalf("no audit log file found in %s", al.LogDir())
	}

	f, err := os.Open(matches[0])
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer f.Close()

	var entries []AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e AuditEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal entry: %v\nline: %s", err, line)
		}
		entries = append(entries, e)
	}
	return entries
}

// firstLogFile returns the path of the first audit log file in the logger's directory.
func firstLogFile(t *testing.T, al *AuditLogger) string {
	t.Helper()
	al.Close()
	pattern := filepath.Join(al.LogDir(), "ghost-mcp-audit-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Fatalf("no audit log file found in %s", al.LogDir())
	}
	return matches[0]
}

// =============================================================================
// NEWAUDITLOGGER TESTS
// =============================================================================

// TestNewAuditLogger_CreatesDirectory tests that NewAuditLogger creates
// the log directory when it does not already exist.
func TestNewAuditLogger_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "audit")
	t.Setenv(AuditEnvVar, dir)

	al, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer al.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Expected directory to be created, but it does not exist")
	}
}

// TestNewAuditLogger_CreatesLogFile tests that a log file is created on startup.
func TestNewAuditLogger_CreatesLogFile(t *testing.T) {
	al := newTestLogger(t)

	pattern := filepath.Join(al.LogDir(), "ghost-mcp-audit-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Error("Expected audit log file to be created")
	}
}

// TestNewAuditLogger_FileNameContainsDate tests that the log file name
// includes today's UTC date.
func TestNewAuditLogger_FileNameContainsDate(t *testing.T) {
	al := newTestLogger(t)

	today := time.Now().UTC().Format("2006-01-02")
	expected := fmt.Sprintf("ghost-mcp-audit-%s.jsonl", today)

	pattern := filepath.Join(al.LogDir(), "ghost-mcp-audit-*.jsonl")
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		t.Fatal("No audit log file created")
	}
	if filepath.Base(matches[0]) != expected {
		t.Errorf("Expected file name %q, got %q", expected, filepath.Base(matches[0]))
	}
}

// TestNewAuditLogger_DisabledOnBadDirectory tests that an invalid directory
// returns a disabled logger, not nil.
func TestNewAuditLogger_DisabledOnBadDirectory(t *testing.T) {
	// Use a path that cannot be created (file exists at parent path)
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(AuditEnvVar, filepath.Join(blocker, "subdir"))

	al, err := NewAuditLogger()
	if err == nil {
		t.Error("Expected error for bad directory")
	}
	if al == nil {
		t.Fatal("Expected non-nil disabled logger, got nil")
	}
	// Writes to a disabled logger must not panic
	al.Log(EventServerStart, "", "", nil)
}

// =============================================================================
// LOG ENTRY TESTS
// =============================================================================

// TestLog_WritesEntry tests that Log writes a correctly structured entry.
func TestLog_WritesEntry(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", map[string]interface{}{
		"version": "1.0.0",
	})

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

// TestLog_SequenceIncrements tests that sequence numbers are monotonically increasing.
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

// TestLog_ClientIDIncluded tests that the client ID appears in entries.
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

// TestLog_ErrorFieldPopulated tests that error messages appear in entries.
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

// TestLog_ToolNameIncluded tests that the tool name appears in entries.
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

// TestHashChain_GenesisHash tests that the first entry's prev_hash is the genesis value.
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

// TestHashChain_ChainedCorrectly tests that each entry's prev_hash matches
// the hash of the preceding entry.
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

// TestHashChain_HashDiffersPerEntry tests that distinct entries produce distinct hashes.
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

// TestVerifyLogFile_ValidFile tests that an intact log file passes verification.
func TestVerifyLogFile_ValidFile(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", nil)
	al.Log(EventToolCall, "click", "", nil)
	al.Log(EventToolSuccess, "click", "", nil)

	path := firstLogFile(t, al)
	if err := VerifyLogFile(path); err != nil {
		t.Errorf("Expected valid file to pass, got error: %v", err)
	}
}

// TestVerifyLogFile_EmptyFile tests that an empty file is valid.
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

// TestVerifyLogFile_TamperedHash tests that modifying an entry's content
// is detected by VerifyLogFile.
func TestVerifyLogFile_TamperedHash(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventToolCall, "click", "", nil)
	path := firstLogFile(t, al)

	// Read the file, modify one entry's event field, write it back.
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

// TestVerifyLogFile_TamperedChain tests that deleting an entry is detected.
func TestVerifyLogFile_TamperedChain(t *testing.T) {
	al := newTestLogger(t)
	al.Log(EventServerStart, "", "", nil)
	al.Log(EventToolCall, "click", "", nil)
	al.Log(EventToolSuccess, "click", "", nil)
	path := firstLogFile(t, al)

	// Remove the second line to break the chain.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatal("Expected at least 3 lines")
	}
	// Keep first and third, drop second
	kept := append(lines[:1], lines[2:]...)
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := VerifyLogFile(path); err == nil {
		t.Error("Expected verification to fail after deleting an entry")
	}
}

// TestVerifyLogFile_MissingFile tests that a missing file returns an error.
func TestVerifyLogFile_MissingFile(t *testing.T) {
	if err := VerifyLogFile("/does/not/exist.jsonl"); err == nil {
		t.Error("Expected error for missing file")
	}
}

// =============================================================================
// PARAMETER SANITIZATION TESTS
// =============================================================================

// TestSanitizeParams_NilInput tests that nil input returns nil.
func TestSanitizeParams_NilInput(t *testing.T) {
	if sanitizeParams(nil) != nil {
		t.Error("Expected nil for nil input")
	}
}

// TestSanitizeParams_EmptyInput tests that empty input returns nil.
func TestSanitizeParams_EmptyInput(t *testing.T) {
	if sanitizeParams(map[string]interface{}{}) != nil {
		t.Error("Expected nil for empty input")
	}
}

// TestSanitizeParams_ShortStringsPreserved tests that short values are unchanged.
func TestSanitizeParams_ShortStringsPreserved(t *testing.T) {
	input := map[string]interface{}{"key": "hello"}
	out := sanitizeParams(input)
	if out["key"] != "hello" {
		t.Errorf("Expected 'hello', got %v", out["key"])
	}
}

// TestSanitizeParams_LongStringTruncated tests that strings over the limit are truncated.
func TestSanitizeParams_LongStringTruncated(t *testing.T) {
	long := strings.Repeat("a", maxParamValueLen+100)
	input := map[string]interface{}{"text": long}
	out := sanitizeParams(input)
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

// TestSanitizeParams_NonStringPreserved tests that non-string values are unchanged.
func TestSanitizeParams_NonStringPreserved(t *testing.T) {
	input := map[string]interface{}{"x": float64(42)}
	out := sanitizeParams(input)
	if out["x"] != float64(42) {
		t.Errorf("Expected float64(42), got %v", out["x"])
	}
}

// =============================================================================
// COMPUTE ENTRY HASH TESTS
// =============================================================================

// TestComputeEntryHash_Deterministic tests that the same entry always produces
// the same hash.
func TestComputeEntryHash_Deterministic(t *testing.T) {
	entry := AuditEntry{
		Sequence:  1,
		Timestamp: "2024-01-01T00:00:00Z",
		Event:     EventToolCall,
		Tool:      "click",
		PrevHash:  strings.Repeat("0", 64),
	}
	h1 := computeEntryHash(entry)
	h2 := computeEntryHash(entry)
	if h1 != h2 {
		t.Error("Expected deterministic hash")
	}
}

// TestComputeEntryHash_ChangesWithContent tests that different content produces
// different hashes.
func TestComputeEntryHash_ChangesWithContent(t *testing.T) {
	base := AuditEntry{
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

// TestComputeEntryHash_HashFieldIgnored tests that changing the Hash field
// does not affect the computed hash (the hash covers content, not itself).
func TestComputeEntryHash_HashFieldIgnored(t *testing.T) {
	entry := AuditEntry{
		Sequence:  1,
		Timestamp: "2024-01-01T00:00:00Z",
		Event:     EventServerStart,
		PrevHash:  strings.Repeat("0", 64),
		Hash:      "some-old-hash",
	}
	h1 := computeEntryHash(entry)
	entry.Hash = "different-hash"
	h2 := computeEntryHash(entry)
	if h1 != h2 {
		t.Error("Hash field should not influence the computed hash")
	}
}

// =============================================================================
// CLIENT ID TESTS
// =============================================================================

// TestSetGetClientID tests that SetClientID and GetClientID round-trip correctly.
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

// TestDisabledLogger_DoesNotPanic tests that all methods on a disabled logger
// are safe to call without panicking.
func TestDisabledLogger_DoesNotPanic(t *testing.T) {
	al := &AuditLogger{disabled: true}
	al.Log(EventServerStart, "", "", nil)
	al.SetClientID("x")
	_ = al.GetClientID()
	al.Close()
}
