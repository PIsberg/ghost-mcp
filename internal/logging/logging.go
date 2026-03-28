// Package logging provides stderr logging helpers for Ghost MCP.
//
// All output goes to stderr — stdout is reserved for the MCP JSON-RPC protocol
// and must never be written to directly.
package logging

import (
	"fmt"
	"os"
)

// Info writes an informational message to stderr.
func Info(format string, args ...interface{}) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf("[INFO] "+format, args...))
}

// Error writes an error message to stderr.
func Error(format string, args ...interface{}) {
	fmt.Fprintln(os.Stderr, fmt.Sprintf("[ERROR] "+format, args...))
}

// Debug writes a debug message to stderr, only when GHOST_MCP_DEBUG=1.
func Debug(format string, args ...interface{}) {
	if os.Getenv("GHOST_MCP_DEBUG") == "1" {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("[DEBUG] "+format, args...))
	}
}
