package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// HeaderBarHeight is the number of lines the header bar occupies.
const HeaderBarHeight = 1

// RenderHeaderBar renders a full-width header bar with "🧠thinkt <context>"
// on the left, an optional info string (muted, left-aligned after context),
// and an optional detail string on the right.
func RenderHeaderBar(context, info, detail string, width int) string {
	if context == "" || width <= 0 {
		return ""
	}

	t := theme.Current()
	bg := lipgloss.Color(t.GetAccent())

	brandStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#d0d0d0"))

	detailStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(lipgloss.Color("#d0d0d0"))

	padStyle := lipgloss.NewStyle().
		Background(bg)

	left := brandStyle.Render(" 🧠thinkt " + context)
	if info != "" {
		left += infoStyle.Render("  " + info)
	}

	var right string
	if detail != "" {
		right = detailStyle.Render(detail + " ")
	}

	totalWidth := lipgloss.Width(left) + lipgloss.Width(right)
	pad := width - totalWidth
	if pad < 0 {
		pad = 0
	}

	return left + padStyle.Render(strings.Repeat(" ", pad)) + right
}
