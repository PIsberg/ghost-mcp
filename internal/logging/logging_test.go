// logging_test.go — Tests for stderr logging helpers.
package logging

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr redirects os.Stderr to a pipe for the duration of f,
// then returns everything written to it.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })

	f()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestInfo(t *testing.T) {
	got := captureStderr(t, func() {
		Info("hello %s", "world")
	})
	if !strings.Contains(got, "[INFO]") {
		t.Errorf("expected [INFO] prefix, got: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("expected message body, got: %q", got)
	}
}

func TestError(t *testing.T) {
	got := captureStderr(t, func() {
		Error("something went %s", "wrong")
	})
	if !strings.Contains(got, "[ERROR]") {
		t.Errorf("expected [ERROR] prefix, got: %q", got)
	}
	if !strings.Contains(got, "something went wrong") {
		t.Errorf("expected message body, got: %q", got)
	}
}

func TestDebug_Disabled(t *testing.T) {
	t.Setenv("GHOST_MCP_DEBUG", "")
	got := captureStderr(t, func() {
		Debug("should not appear")
	})
	if got != "" {
		t.Errorf("expected no output when debug disabled, got: %q", got)
	}
}

func TestDebug_Enabled(t *testing.T) {
	t.Setenv("GHOST_MCP_DEBUG", "1")
	got := captureStderr(t, func() {
		Debug("debug message %d", 42)
	})
	if !strings.Contains(got, "[DEBUG]") {
		t.Errorf("expected [DEBUG] prefix, got: %q", got)
	}
	if !strings.Contains(got, "debug message 42") {
		t.Errorf("expected message body, got: %q", got)
	}
}
