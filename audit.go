// audit.go - Tamper-evident audit logging for Ghost MCP
//
// Every tool invocation, authentication failure, and server lifecycle event is
// written as a JSON Lines record to a dedicated log file.
//
// # Tamper Evidence
//
// Each entry carries:
//   - A SHA-256 hash of its own content (self-hash)
//   - A reference to the hash of the previous entry (prev_hash)
//
// Together these form a hash chain: if any record is modified, deleted, or
// reordered the chain breaks and VerifyLogFile will report it.
//
// # File Location
//
// Set GHOST_MCP_AUDIT_LOG to a directory path to control where logs are written.
// Default: <UserConfigDir>/ghost-mcp/audit/
//
// Files are named ghost-mcp-audit-YYYY-MM-DD.jsonl (UTC date, rotated daily).
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AuditEnvVar is the environment variable that sets the audit log directory.
const AuditEnvVar = "GHOST_MCP_AUDIT_LOG"

// maxParamValueLen is the maximum rune count logged for a single parameter value.
// Longer values are truncated to prevent log bloat (e.g., long type_text inputs).
const maxParamValueLen = 500

// ErrAuthFailed is the sentinel error for authentication failures.
// It is wrapped into the error returned by makeTokenValidator so that
// callers can use errors.Is to distinguish auth failures from other errors.
var ErrAuthFailed = errors.New("authentication required")

// Audit event type constants used in the "event" field of every log entry.
const (
	EventServerStart  = "SERVER_START"
	EventServerStop   = "SERVER_STOP"
	EventClientConn   = "CLIENT_CONNECTED"
	EventToolCall     = "TOOL_CALL"
	EventToolSuccess  = "TOOL_SUCCESS"
	EventToolFailure  = "TOOL_FAILURE"
	EventAuthFailure  = "AUTH_FAILURE"
	EventScreenshot   = "SCREENSHOT_REQUESTED"
	EventRequestError = "REQUEST_ERROR"
)

// AuditEntry is one record in the audit log.
type AuditEntry struct {
	Sequence  int64                  `json:"seq"`
	Timestamp string                 `json:"timestamp"`
	Event     string                 `json:"event"`
	Tool      string                 `json:"tool,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
	ClientID  string                 `json:"client_id,omitempty"`
	Error     string                 `json:"error,omitempty"`
	PrevHash  string                 `json:"prev_hash"`
	Hash      string                 `json:"hash"`
}

// AuditLogger writes tamper-evident audit records. Safe for concurrent use.
// A disabled logger silently drops all writes; it is never nil.
type AuditLogger struct {
	dir      string
	mu       sync.Mutex
	file     *os.File
	date     string // current file's UTC date (YYYY-MM-DD)
	seq      int64
	lastHash string // hash of the most recently written entry
	clientID string // set after MCP initialize handshake
	disabled bool   // true when startup failed; all writes become no-ops
}

// NewAuditLogger creates an AuditLogger. On error it returns a disabled logger
// so callers never need nil-checks; writes simply become no-ops.
func NewAuditLogger() (*AuditLogger, error) {
	dir := os.Getenv(AuditEnvVar)
	if dir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			configDir = os.TempDir()
		}
		dir = filepath.Join(configDir, "ghost-mcp", "audit")
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return &AuditLogger{disabled: true},
			fmt.Errorf("failed to create audit log directory %q: %w", dir, err)
	}

	al := &AuditLogger{
		dir:      dir,
		lastHash: strings.Repeat("0", 64), // genesis hash — no previous entry
	}
	if err := al.openFile(); err != nil {
		return &AuditLogger{disabled: true}, err
	}

	logInfo("Audit logging enabled: %s", dir)
	return al, nil
}

// openFile opens (or creates) today's audit log file in append mode.
// Caller must hold al.mu.
func (al *AuditLogger) openFile() error {
	date := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(al.dir, fmt.Sprintf("ghost-mcp-audit-%s.jsonl", date))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open audit log %q: %w", path, err)
	}

	if al.file != nil {
		al.file.Close()
	}
	al.file = f
	al.date = date
	return nil
}

// SetClientID records the MCP client name so it appears in future log entries.
func (al *AuditLogger) SetClientID(id string) {
	al.mu.Lock()
	al.clientID = id
	al.mu.Unlock()
}

// GetClientID returns the current client identity string.
func (al *AuditLogger) GetClientID() string {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.clientID
}

// LogDir returns the directory where audit logs are written.
func (al *AuditLogger) LogDir() string {
	return al.dir
}

// Log writes one audit entry. It is a no-op when the logger is disabled.
func (al *AuditLogger) Log(event, tool, errMsg string, params map[string]interface{}) {
	if al == nil || al.disabled {
		return
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	// Rotate to a new file when the UTC date changes.
	today := time.Now().UTC().Format("2006-01-02")
	if today != al.date {
		if err := al.openFile(); err != nil {
			logError("Audit log rotation failed: %v", err)
			return
		}
	}

	al.seq++
	entry := AuditEntry{
		Sequence:  al.seq,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Event:     event,
		Tool:      tool,
		Params:    sanitizeParams(params),
		ClientID:  al.clientID,
		Error:     errMsg,
		PrevHash:  al.lastHash,
	}
	entry.Hash = computeEntryHash(entry)
	al.lastHash = entry.Hash

	data, err := json.Marshal(entry)
	if err != nil {
		logError("Audit: failed to marshal entry: %v", err)
		return
	}
	if _, err := fmt.Fprintf(al.file, "%s\n", data); err != nil {
		logError("Audit: failed to write entry: %v", err)
		return
	}
	// Sync to disk after every entry so the log survives crashes.
	al.file.Sync() //nolint:errcheck

	logDebug("Audit[%d] event=%s tool=%q client=%q", al.seq, event, tool, al.clientID)
}

// Close flushes and closes the underlying log file.
func (al *AuditLogger) Close() {
	if al == nil || al.disabled {
		return
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.file != nil {
		al.file.Sync() //nolint:errcheck
		al.file.Close()
		al.file = nil
	}
}

// =============================================================================
// HASH CHAIN
// =============================================================================

// hashableEntry mirrors AuditEntry without the Hash field so we can produce
// a deterministic hash of the entry's content.
type hashableEntry struct {
	Sequence  int64                  `json:"seq"`
	Timestamp string                 `json:"timestamp"`
	Event     string                 `json:"event"`
	Tool      string                 `json:"tool,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
	ClientID  string                 `json:"client_id,omitempty"`
	Error     string                 `json:"error,omitempty"`
	PrevHash  string                 `json:"prev_hash"`
}

// computeEntryHash returns the SHA-256 hex digest of entry content (Hash field excluded).
func computeEntryHash(entry AuditEntry) string {
	h := hashableEntry{
		Sequence:  entry.Sequence,
		Timestamp: entry.Timestamp,
		Event:     entry.Event,
		Tool:      entry.Tool,
		Params:    entry.Params,
		ClientID:  entry.ClientID,
		Error:     entry.Error,
		PrevHash:  entry.PrevHash,
	}
	data, _ := json.Marshal(h)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// VerifyLogFile reads a .jsonl audit log file and validates its hash chain.
// Returns nil when every entry is intact, or an error describing the first
// broken link or corrupted hash.
func VerifyLogFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read log file: %w", err)
	}

	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	prevHash := strings.Repeat("0", 64) // expected genesis prev_hash

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return fmt.Errorf("line %d: invalid JSON: %w", i+1, err)
		}
		if entry.PrevHash != prevHash {
			return fmt.Errorf("line %d (seq %d): broken chain — prev_hash mismatch", i+1, entry.Sequence)
		}
		expected := computeEntryHash(entry)
		if entry.Hash != expected {
			return fmt.Errorf("line %d (seq %d): content hash mismatch — entry may have been tampered with", i+1, entry.Sequence)
		}
		prevHash = entry.Hash
	}
	return nil
}

// =============================================================================
// PARAMETER SANITIZATION
// =============================================================================

// sanitizeParams returns a copy of params safe for logging.
// String values longer than maxParamValueLen are truncated.
func sanitizeParams(params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		if s, ok := v.(string); ok && len([]rune(s)) > maxParamValueLen {
			runes := []rune(s)
			out[k] = fmt.Sprintf("%s...[%d chars truncated]",
				string(runes[:maxParamValueLen]), len(runes)-maxParamValueLen)
		} else {
			out[k] = v
		}
	}
	return out
}

// =============================================================================
// MCP SERVER HOOK REGISTRATION
// =============================================================================

// registerAuditHooks wires audit-log writes into an existing Hooks set.
// It does not replace any hooks already registered (e.g. the token validator).
func registerAuditHooks(hooks *server.Hooks, al *AuditLogger) {
	// Capture client identity from the MCP initialize handshake.
	hooks.AddAfterInitialize(func(
		_ context.Context, _ any,
		req *mcp.InitializeRequest, _ *mcp.InitializeResult,
	) {
		if req == nil {
			return
		}
		name := req.Params.ClientInfo.Name
		if name == "" {
			name = "unknown"
		}
		al.SetClientID(name)
		al.Log(EventClientConn, "", "", map[string]interface{}{
			"client_name":    req.Params.ClientInfo.Name,
			"client_version": req.Params.ClientInfo.Version,
			"protocol":       req.Params.ProtocolVersion,
		})
	})

	// Log every tool invocation with its parameters before execution.
	hooks.AddBeforeCallTool(func(_ context.Context, _ any, req *mcp.CallToolRequest) {
		if req == nil {
			return
		}
		params := make(map[string]interface{}, len(req.GetArguments()))
		for k, v := range req.GetArguments() {
			params[k] = v
		}
		al.Log(EventToolCall, req.Params.Name, "", params)
	})

	// Log tool outcomes after execution.
	hooks.AddAfterCallTool(func(_ context.Context, _ any, req *mcp.CallToolRequest, result any) {
		if req == nil {
			return
		}
		toolResult, _ := result.(*mcp.CallToolResult)
		if toolResult != nil && toolResult.IsError {
			al.Log(EventToolFailure, req.Params.Name, extractToolResultError(toolResult), nil)
			return
		}
		event := EventToolSuccess
		if req.Params.Name == "take_screenshot" {
			event = EventScreenshot
		}
		al.Log(event, req.Params.Name, "", nil)
	})

	// Log MCP-level errors (tool not found, malformed requests, etc.).
	// Note: onRequestInitialization failures (auth) are logged directly in
	// makeTokenValidator because they bypass this hook.
	hooks.AddOnError(func(_ context.Context, _ any, method mcp.MCPMethod, _ any, err error) {
		if err == nil {
			return
		}
		event := EventRequestError
		if errors.Is(err, ErrAuthFailed) {
			event = EventAuthFailure
		}
		al.Log(event, string(method), err.Error(), nil)
	})
}

// extractToolResultError returns the error text from a CallToolResult.
func extractToolResultError(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return "unknown tool error"
	}
	if tc, ok := result.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return "tool error (non-text content)"
}
