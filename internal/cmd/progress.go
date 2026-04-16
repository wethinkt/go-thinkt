package cmd

import (
	"fmt"
	"os"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"golang.org/x/term"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

const (
	progressBarMinWidth     = 10
	progressBarDefaultWidth = 30
	// spacing: " %s  %s  %s  %s     %s" → 2+2+2+5 = 11 chars of padding + 1 leading space
	progressLinePadding = 12
)

// SyncProgress handles themed progress display for sync-style commands.
type SyncProgress struct {
	startTime time.Time
	isTTY     bool
	bar       progress.Model
	bar2      progress.Model
	rendered  bool

	phaseStyle   lipgloss.Style
	countStyle   lipgloss.Style
	detailStyle  lipgloss.Style
	elapsedStyle lipgloss.Style
}

// NewSyncProgress creates a themed sync progress reporter.
func NewSyncProgress() *SyncProgress {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	t := theme.Current()

	newBar := func() progress.Model {
		b := progress.New(
			progress.WithColors(lipgloss.Color(t.GetAccent())),
			progress.WithoutPercentage(),
			progress.WithWidth(progressBarDefaultWidth),
		)
		b.EmptyColor = lipgloss.Color(t.TextMuted.Fg)
		return b
	}

	return &SyncProgress{
		startTime: time.Now(),
		isTTY:     isTTY,
		bar:       newBar(),
		bar2:      newBar(),
		phaseStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.GetAccent())).
			Bold(true),
		countStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)),
		detailStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg)),
		elapsedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg)),
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
func (sp *SyncProgress) IsTTY() bool { return sp.isTTY }

func (sp *SyncProgress) getWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func (sp *SyncProgress) renderLine(phase, count, detail string, pct float64) {
	elapsed := formatElapsed(time.Since(sp.startTime))

	fixedWidth := len(phase) + len(count) + len(detail) + len(elapsed) + progressLinePadding
	barWidth := sp.getWidth() - fixedWidth
	switch {
	case barWidth < progressBarMinWidth:
		barWidth = progressBarMinWidth
	case barWidth > progressBarDefaultWidth:
		barWidth = progressBarDefaultWidth
	}

	sp.bar.SetWidth(barWidth)
	bar := sp.bar.ViewAs(pct)

	line := fmt.Sprintf(" %s  %s  %s  %s     %s",
		sp.phaseStyle.Render(phase),
		bar,
		sp.countStyle.Render(count),
		sp.detailStyle.Render(detail),
		sp.elapsedStyle.Render(elapsed),
	)
	sp.Print(line)
}

// renderDualLine renders two bars side-by-side for project/session progress.
func (sp *SyncProgress) renderDualLine(phase, count1 string, pct1 float64, count2 string, pct2 float64, detail string) {
	elapsed := formatElapsed(time.Since(sp.startTime))

	const dualPadding = 14 // " " + "  "*3 + "     " = 1+6+5 = 12 ... plus count separators totals 14
	fixedWidth := len(phase) + len(count1) + len(count2) + len(detail) + len(elapsed) + dualPadding

	totalBarSpace := sp.getWidth() - fixedWidth
	if totalBarSpace < progressBarMinWidth*2 {
		totalBarSpace = progressBarMinWidth * 2
	}
	if totalBarSpace > progressBarDefaultWidth*2 {
		totalBarSpace = progressBarDefaultWidth * 2
	}

	bar1Width := totalBarSpace / 2
	bar2Width := totalBarSpace - bar1Width
	sp.bar.SetWidth(bar1Width)
	sp.bar2.SetWidth(bar2Width)

	line := fmt.Sprintf(" %s  %s %s  %s %s  %s     %s",
		sp.phaseStyle.Render(phase),
		sp.bar.ViewAs(pct1),
		sp.countStyle.Render(count1),
		sp.bar2.ViewAs(pct2),
		sp.countStyle.Render(count2),
		sp.detailStyle.Render(detail),
		sp.elapsedStyle.Render(elapsed),
	)
	sp.Print(line)
}

// RenderIndexing renders an indexing progress line. When there are multiple projects,
// two progress bars are shown: overall project progress and sessions within the current project.
func (sp *SyncProgress) RenderIndexing(pIdx, pTotal, sIdx, sTotal int, message string) {
	if !sp.isTTY {
		sp.Print(fmt.Sprintf("Projects [%d/%d] | Sessions [%d/%d] %s", pIdx, pTotal, sIdx, sTotal, message))
		return
	}

	if pTotal <= 1 {
		var pct float64
		if sTotal > 0 {
			pct = float64(sIdx) / float64(sTotal)
		}
		sp.renderLine(
			thinktI18n.T("cmd.sync.progress.indexing", "Indexing"),
			fmt.Sprintf("%d/%d sessions", sIdx, sTotal),
			message, pct,
		)
		return
	}

	var projectPct, sessionPct float64
	if pTotal > 0 {
		projectPct = float64(pIdx-1) / float64(pTotal)
		if sTotal > 0 {
			projectPct = (float64(pIdx-1) + float64(sIdx)/float64(sTotal)) / float64(pTotal)
			sessionPct = float64(sIdx) / float64(sTotal)
		}
	}

	sp.renderDualLine(
		thinktI18n.T("cmd.sync.progress.indexing", "Indexing"),
		fmt.Sprintf("%d/%d", pIdx, pTotal), projectPct,
		fmt.Sprintf("%d/%d", sIdx, sTotal), sessionPct,
		message,
	)
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

// Finish prints a trailing newline if any progress was rendered in TTY mode.
func (sp *SyncProgress) Finish() {
	if sp.isTTY && sp.rendered {
		fmt.Println()
	}
}

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
