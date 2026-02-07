package cmd

import (
	"strings"
	"testing"
	"time"
)

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{500 * time.Millisecond, "0.5s"},
		{1 * time.Second, "1s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m 30s"},
		{3661 * time.Second, "1h 1m"},
		{7384 * time.Second, "2h 3m"},
	}

	for _, tt := range tests {
		got := formatElapsed(tt.duration)
		if got != tt.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}

func TestProgressReporter_FormatProgress(t *testing.T) {
	pr := &ProgressReporter{
		startTime: time.Now().Add(-65 * time.Second), // 1m 5s ago
		isTTY:     true,
	}

	message := "Projects [3/10] | Sessions [15/42] Indexing foo"
	formatted := pr.FormatProgress(message)

	// Check that message is included
	if !strings.Contains(formatted, "Projects [3/10]") {
		t.Errorf("Formatted message should contain original message, got: %q", formatted)
	}

	// Check that elapsed time is included
	if !strings.Contains(formatted, "1m") {
		t.Errorf("Formatted message should contain elapsed time, got: %q", formatted)
	}

	// Check formatting (should have | separator)
	if !strings.Contains(formatted, " | ") {
		t.Errorf("Formatted message should have ' | ' separator, got: %q", formatted)
	}
}

func TestProgressReporter_FormatProgress_NonTTY(t *testing.T) {
	pr := &ProgressReporter{
		startTime: time.Now(),
		isTTY:     false,
	}

	message := "Test message"
	formatted := pr.FormatProgress(message)

	// In non-TTY mode, should return message unchanged
	if formatted != message {
		t.Errorf("Non-TTY FormatProgress() = %q, want %q", formatted, message)
	}
}

func TestProgressReporter_ShouldShowProgress(t *testing.T) {
	tests := []struct {
		name    string
		isTTY   bool
		quiet   bool
		verbose bool
		want    bool
	}{
		{"TTY, default flags", true, false, false, true},
		{"Non-TTY, default flags", false, false, false, false},
		{"TTY, quiet=true", true, true, false, false},
		{"Non-TTY, verbose=true", false, false, true, true},
		{"TTY, verbose=true", true, false, true, true},
		{"Quiet overrides verbose", true, true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := &ProgressReporter{isTTY: tt.isTTY}
			got := pr.ShouldShowProgress(tt.quiet, tt.verbose)
			if got != tt.want {
				t.Errorf("ShouldShowProgress(quiet=%v, verbose=%v) with isTTY=%v = %v, want %v",
					tt.quiet, tt.verbose, tt.isTTY, got, tt.want)
			}
		})
	}
}

func TestProgressReporter_LongMessage(t *testing.T) {
	pr := &ProgressReporter{
		startTime: time.Now(),
		isTTY:     true,
	}

	// Create a message that's definitely longer than most terminal widths
	longMessage := strings.Repeat("This is a very long message that exceeds the terminal width ", 5)
	formatted := pr.FormatProgress(longMessage)

	// The formatted message should be shorter than the original (got truncated)
	if len(formatted) >= len(longMessage) {
		t.Errorf("Long message should be truncated, but formatted length %d >= original length %d",
			len(formatted), len(longMessage))
	}

	// Should contain ellipsis for truncation
	if !strings.Contains(formatted, "...") {
		t.Errorf("Long message should be truncated with '...', got: %q", formatted)
	}

	// Should still include elapsed time
	if !strings.Contains(formatted, " | ") {
		t.Errorf("Formatted message should have ' | ' separator for elapsed time, got: %q", formatted)
	}
}
