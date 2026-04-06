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
		Init("", "INFO")
		Info("hello %s", "world")
	})
	if !strings.Contains(got, "level=INFO") {
		t.Errorf("expected level=INFO, got: %q", got)
	}
	if !strings.Contains(got, "msg=\"hello world\"") {
		t.Errorf("expected message body, got: %q", got)
	}
}

func TestError(t *testing.T) {
	got := captureStderr(t, func() {
		Init("", "INFO")
		Error("something went %s", "wrong")
	})
	if !strings.Contains(got, "level=ERROR") {
		t.Errorf("expected level=ERROR, got: %q", got)
	}
	if !strings.Contains(got, "msg=\"something went wrong\"") {
		t.Errorf("expected message body, got: %q", got)
	}
}

func TestDebug_Disabled(t *testing.T) {
	got := captureStderr(t, func() {
		Init("", "INFO")
		Debug("should not appear")
	})
	// Search in the output, excluding the initialization message
	if strings.Contains(got, "level=DEBUG") {
		t.Errorf("expected no level=DEBUG when level is INFO, got: %q", got)
	}
}

func TestDebug_Enabled(t *testing.T) {
	got := captureStderr(t, func() {
		Init("", "DEBUG")
		Debug("debug message %d", 42)
	})
	if !strings.Contains(got, "level=DEBUG") {
		t.Errorf("expected level=DEBUG, got: %q", got)
	}
	if !strings.Contains(got, "msg=\"debug message 42\"") {
		t.Errorf("expected message body, got: %q", got)
	}
}
