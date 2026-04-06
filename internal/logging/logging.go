// Package logging provides structured logging for Ghost MCP.
//
// All output goes to stderr (for immediate feedback) and optionally to a file
// (for "storytelling" and persistence). stdout is reserved for MCP JSON-RPC.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var (
	defaultLogger *slog.Logger
	logLevel      = new(slog.LevelVar)
)

func init() {
	// Default to stderr-only INFO logging until Init() is called.
	logLevel.Set(slog.LevelInfo)
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
}

// Init initializes the logging system.
// It sets up a multi-writer to both stderr and the specified log file.
func Init(logFilePath string, level string) error {
	// Parse log level
	var l slog.Level
	switch level {
	case "DEBUG":
		l = slog.LevelDebug
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	logLevel.Set(l)

	var writers []io.Writer
	writers = append(writers, os.Stderr)

	if logFilePath != "" {
		// Ensure directory exists
		dir := filepath.Dir(logFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}

		f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		writers = append(writers, f)
		fmt.Fprintf(os.Stderr, "Ghost MCP is narrating its journey to: %s\n", logFilePath)
	}

	multiWriter := io.MultiWriter(writers...)
	defaultLogger = slog.New(slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level: logLevel,
	}))

	return nil
}

// Info writes an informational message.
func Info(format string, args ...interface{}) {
	defaultLogger.Info(fmt.Sprintf(format, args...))
}

// Error writes an error message.
func Error(format string, args ...interface{}) {
	defaultLogger.Error(fmt.Sprintf(format, args...))
}

// Debug writes a debug message.
func Debug(format string, args ...interface{}) {
	defaultLogger.Debug(fmt.Sprintf(format, args...))
}

// With wraps the current logger with additional attributes.
func With(args ...interface{}) *slog.Logger {
	return defaultLogger.With(args...)
}

// GetLogger returns the underlying slog logger.
func GetLogger() *slog.Logger {
	return defaultLogger
}

// Context adds a context to logging calls (for advanced slog features).
func Context(ctx context.Context) *slog.Logger {
	return defaultLogger // simplistic for now, but allows future expansion
}
