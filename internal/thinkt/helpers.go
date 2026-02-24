// Package thinkt provides shared helper functions for the thinkt package.
package thinkt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Default truncation length for display strings.
const DefaultTruncateLength = 50

// Default buffer sizes for scanners.
const (
	DefaultBufferSize  = 64 * 1024        // 64KB initial buffer
	MaxLineCapacity    = 10 * 1024 * 1024 // 10MB max line capacity
	MaxScannerCapacity = 16 * 1024 * 1024 // 16MB max scanner capacity
)

// TruncateString truncates a string to max length, adding "..." if truncated.
// If s is shorter than or equal to max, it returns s unchanged.
// If max is 0 or negative, returns empty string.
func TruncateString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// ValidateSessionPath validates that a session ID is within the expected base directory.
// This prevents directory traversal attacks when loading sessions.
// Returns an error if the session ID is not within the base directory.
func ValidateSessionPath(sessionID, baseDir string) error {
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("invalid session path: empty path")
	}
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("invalid base path: empty path")
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("invalid base path: %w", err)
	}
	absSession, err := filepath.Abs(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session path: %w", err)
	}

	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return fmt.Errorf("invalid base path: %w", err)
	}

	realSession, err := filepath.EvalSymlinks(absSession)
	if err != nil {
		// For non-existent leaf paths, resolve the parent directory so symlink
		// traversal is still checked correctly.
		if os.IsNotExist(err) {
			parentReal, parentErr := filepath.EvalSymlinks(filepath.Dir(absSession))
			if parentErr != nil {
				return fmt.Errorf("invalid session path: %w", err)
			}
			realSession = filepath.Join(parentReal, filepath.Base(absSession))
		} else {
			return fmt.Errorf("invalid session path: %w", err)
		}
	}

	if !IsPathWithinAny(realSession, []string{realBase}) {
		return fmt.Errorf("invalid session path: %s is not within %s", sessionID, baseDir)
	}
	return nil
}

// NewScannerWithMaxCapacity creates a bufio.Scanner with optimized buffer settings
// for reading large JSONL files. Uses a 64KB initial buffer and 10MB max line capacity.
func NewScannerWithMaxCapacity(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, DefaultBufferSize)
	scanner.Buffer(buf, MaxLineCapacity)
	return scanner
}

// NewScannerWithMaxCapacityCustom creates a bufio.Scanner with custom buffer settings.
// Use this when you need different capacity limits than the defaults.
func NewScannerWithMaxCapacityCustom(r io.Reader, initialBufSize, maxCapacity int) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, initialBufSize)
	scanner.Buffer(buf, maxCapacity)
	return scanner
}
