package cmd

import (
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

func TestSyncProgress_ShouldShowProgress(t *testing.T) {
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
			sp := &SyncProgress{isTTY: tt.isTTY}
			got := sp.ShouldShowProgress(tt.quiet, tt.verbose)
			if got != tt.want {
				t.Errorf("ShouldShowProgress(quiet=%v, verbose=%v) with isTTY=%v = %v, want %v",
					tt.quiet, tt.verbose, tt.isTTY, got, tt.want)
			}
		})
	}
}

func TestProjectLabel(t *testing.T) {
	if got := projectLabel(1, 1); got != "" {
		t.Errorf("projectLabel(1, 1) = %q, want empty", got)
	}
	if got := projectLabel(2, 5); got != "proj 2/5" {
		t.Errorf("projectLabel(2, 5) = %q, want %q", got, "proj 2/5")
	}
}
