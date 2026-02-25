package cmd

import (
	"fmt"
	"os"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

const (
	barMinWidth     = 10
	barDefaultWidth = 30
	// spacing: " %s  %s  %s  %s     %s" → 2+2+2+5 = 11 chars of padding + 1 leading space
	linePadding = 12
)

// SyncProgress handles themed progress display for sync operations.
type SyncProgress struct {
	startTime time.Time
	isTTY     bool
	bar       progress.Model
	rendered  bool // true once any progress line has been printed

	// lipgloss styles (from theme)
	phaseStyle   lipgloss.Style
	countStyle   lipgloss.Style
	detailStyle  lipgloss.Style
	elapsedStyle lipgloss.Style
}

// NewSyncProgress creates a new themed sync progress reporter.
func NewSyncProgress() *SyncProgress {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	t := theme.Current()

	bar := progress.New(
		progress.WithColors(lipgloss.Color(t.GetAccent())),
		progress.WithoutPercentage(),
		progress.WithWidth(30),
	)
	bar.EmptyColor = lipgloss.Color(t.TextMuted.Fg)

	return &SyncProgress{
		startTime: time.Now(),
		isTTY:     isTTY,
		bar:       bar,
		phaseStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.GetAccent())).
			Bold(true),
		countStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		detailStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		elapsedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.TextMuted.Fg)),
	}
}

// ShouldShowProgress returns true if progress should be displayed.
func (sp *SyncProgress) ShouldShowProgress(quiet, verbose bool) bool {
	if quiet {
		return false
	}
	if verbose {
		return true
	}
	return sp.isTTY
}

// IsTTY returns whether stdout is a TTY.
func (sp *SyncProgress) IsTTY() bool {
	return sp.isTTY
}

// getWidth returns the current terminal width.
func (sp *SyncProgress) getWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

// renderLine assembles a progress line, dynamically sizing the bar to fit the terminal.
func (sp *SyncProgress) renderLine(phase, count, detail string, pct float64) {
	elapsed := formatElapsed(time.Since(sp.startTime))

	// Calculate how much space the non-bar elements need (use plain text lengths for width math)
	fixedWidth := len(phase) + len(count) + len(detail) + len(elapsed) + linePadding
	barWidth := sp.getWidth() - fixedWidth
	if barWidth < barMinWidth {
		barWidth = barMinWidth
	} else if barWidth > barDefaultWidth {
		barWidth = barDefaultWidth
	}

	sp.bar.SetWidth(barWidth)
	bar := sp.bar.ViewAs(pct)

	// Now render with styles
	line := fmt.Sprintf(" %s  %s  %s  %s     %s",
		sp.phaseStyle.Render(phase),
		bar,
		sp.countStyle.Render(count),
		sp.detailStyle.Render(detail),
		sp.elapsedStyle.Render(elapsed),
	)
	sp.Print(line)
}

// RenderDownload renders a model download progress line.
func (sp *SyncProgress) RenderDownload(modelID string, pct float64) {
	if !sp.isTTY {
		sp.Print(fmt.Sprintf("Downloading %s: %.0f%%", modelID, pct*100))
		return
	}

	sp.renderLine("Download", modelID, fmt.Sprintf("%.0f%%", pct*100), pct)
}

// RenderIndexing renders an indexing progress line.
func (sp *SyncProgress) RenderIndexing(pIdx, pTotal, sIdx, sTotal int, message string) {
	if !sp.isTTY {
		sp.Print(fmt.Sprintf("Projects [%d/%d] | Sessions [%d/%d] %s", pIdx, pTotal, sIdx, sTotal, message))
		return
	}

	var pct float64
	if pTotal > 0 && sTotal > 0 {
		pct = (float64(pIdx-1) + float64(sIdx)/float64(sTotal)) / float64(pTotal)
	}

	count := fmt.Sprintf("%d/%d sessions", sIdx, sTotal)
	if pTotal > 1 {
		count = fmt.Sprintf("%d/%d projects  %d/%d sessions", pIdx, pTotal, sIdx, sTotal)
	}

	sp.renderLine("Indexing", count, message, pct)
}

// RenderEmbedding renders an embedding progress line.
func (sp *SyncProgress) RenderEmbedding(done, total int, detail string) {
	if !sp.isTTY {
		sp.Print(fmt.Sprintf("[%d/%d] %s", done, total, detail))
		return
	}

	var pct float64
	if total > 0 {
		pct = float64(done) / float64(total)
	}

	sp.renderLine("Embedding", fmt.Sprintf("%d/%d sessions", done, total), detail, pct)
}

// Print outputs a progress line, using carriage return + clear on TTY.
func (sp *SyncProgress) Print(line string) {
	sp.rendered = true
	if sp.isTTY {
		fmt.Printf("\r\x1b[K%s", line)
	} else {
		fmt.Println(line)
	}
}

// Finish prints a final newline if in TTY mode and progress was rendered.
func (sp *SyncProgress) Finish() {
	if sp.isTTY && sp.rendered {
		fmt.Println()
	}
}

// formatElapsed formats a duration as elapsed time (e.g., "1m 23s", "45s", "1.2s").
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
