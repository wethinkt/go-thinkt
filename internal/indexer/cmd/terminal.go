package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// ProgressReporter handles progress display with TTY detection and elapsed time
type ProgressReporter struct {
	startTime time.Time
	isTTY     bool
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter() *ProgressReporter {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	return &ProgressReporter{
		startTime: time.Now(),
		isTTY:     isTTY,
	}
}

// getWidth returns the current terminal width, checking on each call to handle resizes
func (pr *ProgressReporter) getWidth() int {
	if !pr.isTTY {
		return 80 // default for non-TTY
	}

	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}

	return 80 // fallback default
}

// ShouldShowProgress returns true if progress should be displayed
func (pr *ProgressReporter) ShouldShowProgress(quiet, verbose bool) bool {
	// If quiet is set, never show (takes precedence over verbose)
	if quiet {
		return false
	}
	// If verbose is set, always show
	if verbose {
		return true
	}
	// Otherwise, show only if TTY
	return pr.isTTY
}

// IsTTY returns whether stdout is a TTY
func (pr *ProgressReporter) IsTTY() bool {
	return pr.isTTY
}

// FormatProgress formats a progress line with elapsed time on the right if in TTY
func (pr *ProgressReporter) FormatProgress(message string) string {
	if !pr.isTTY {
		// Not a TTY, just return the message without formatting
		return message
	}

	// Get current terminal width (handles resizes)
	width := pr.getWidth()

	elapsed := time.Since(pr.startTime)
	elapsedStr := formatElapsed(elapsed)

	// Calculate available space for the message
	// Account for the elapsed time string and some padding
	availableWidth := width - len(elapsedStr) - 3 // 3 for " | "

	if availableWidth < 20 {
		// Terminal too narrow, just show message
		return message
	}

	// Truncate message if needed
	displayMsg := message
	if len(message) > availableWidth {
		displayMsg = message[:availableWidth-3] + "..."
	}

	// Pad message to push elapsed time to the right
	padding := width - len(displayMsg) - len(elapsedStr) - 3
	if padding < 0 {
		padding = 0
	}

	return fmt.Sprintf("%s%s | %s", displayMsg, strings.Repeat(" ", padding), elapsedStr)
}

// Print prints a progress line with carriage return and clear to end of line
func (pr *ProgressReporter) Print(message string) {
	if pr.isTTY {
		// \r moves cursor to start of line
		// \x1b[K clears from cursor to end of line
		fmt.Printf("\r\x1b[K%s", pr.FormatProgress(message))
	} else {
		// Not a TTY, print with newline
		fmt.Println(message)
	}
}

// Finish prints a final newline if in TTY mode
func (pr *ProgressReporter) Finish() {
	if pr.isTTY {
		fmt.Println()
	}
}

// formatElapsed formats a duration as elapsed time (e.g., "1m 23s", "45s", "1.2s")
func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	seconds := int(d.Seconds())

	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}

	minutes := seconds / 60
	seconds = seconds % 60

	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}

	hours := minutes / 60
	minutes = minutes % 60

	return fmt.Sprintf("%dh %dm", hours, minutes)
}
