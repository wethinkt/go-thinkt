// Package tuilog provides file-based logging for TUI applications.
// It is a separate package to avoid import cycles with the tui package.
package tuilog

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger provides file-based logging for the TUI since stdout/stderr
// are not available during terminal UI operation.
type Logger struct {
	mu      sync.Mutex
	file    *os.File
	enabled bool
}

var (
	// Log is the global logger instance for the TUI
	Log     = &Logger{}
	logOnce sync.Once
)

// Init initializes the global logger to write to the specified file.
// If path is empty, logging is disabled.
func Init(path string) error {
	if path == "" {
		Log.enabled = false
		return nil
	}

	var initErr error
	logOnce.Do(func() {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			initErr = err
			return
		}
		Log.file = f
		Log.enabled = true
		Log.Info("Logger initialized", "path", path)
	})
	return initErr
}

// Close closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Enabled returns whether logging is active.
func (l *Logger) Enabled() bool {
	return l.enabled
}

// Writer returns the underlying io.Writer for use with other logging libraries.
func (l *Logger) Writer() io.Writer {
	if !l.enabled || l.file == nil {
		return io.Discard
	}
	return l.file
}

func (l *Logger) log(level string, msg string, keyvals ...any) {
	if !l.enabled || l.file == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("15:04:05.000")
	line := fmt.Sprintf("%s [%s] %s", timestamp, level, msg)

	// Append key-value pairs
	for i := 0; i < len(keyvals)-1; i += 2 {
		key := keyvals[i]
		val := keyvals[i+1]
		line += fmt.Sprintf(" %v=%v", key, val)
	}

	fmt.Fprintln(l.file, line)
	l.file.Sync() // Ensure log is written immediately
}

// Debug logs a debug message with optional key-value pairs.
func (l *Logger) Debug(msg string, keyvals ...any) {
	l.log("DEBUG", msg, keyvals...)
}

// Info logs an info message with optional key-value pairs.
func (l *Logger) Info(msg string, keyvals ...any) {
	l.log("INFO", msg, keyvals...)
}

// Warn logs a warning message with optional key-value pairs.
func (l *Logger) Warn(msg string, keyvals ...any) {
	l.log("WARN", msg, keyvals...)
}

// Error logs an error message with optional key-value pairs.
func (l *Logger) Error(msg string, keyvals ...any) {
	l.log("ERROR", msg, keyvals...)
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, args ...any) {
	l.log("DEBUG", fmt.Sprintf(format, args...))
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...any) {
	l.log("INFO", fmt.Sprintf(format, args...))
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, args ...any) {
	l.log("WARN", fmt.Sprintf(format, args...))
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...any) {
	l.log("ERROR", fmt.Sprintf(format, args...))
}

// Timed logs the duration of an operation. Usage:
//
//	defer tuilog.Log.Timed("operation name")()
func (l *Logger) Timed(operation string) func() {
	if !l.enabled {
		return func() {}
	}
	start := time.Now()
	l.Debug(operation, "status", "started")
	return func() {
		l.Debug(operation, "status", "completed", "duration", time.Since(start))
	}
}
