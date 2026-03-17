package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// HeaderBarHeight is the number of lines the header bar occupies.
const HeaderBarHeight = 1

// RenderHeaderBar renders a full-width header bar with "🧠thinkt <context>"
// on the left and an optional detail string on the right
// (e.g. "🧠thinkt export > myproject  (149 sessions)").
func RenderHeaderBar(context, detail string, width int) string {
	if context == "" || width <= 0 {
		return ""
	}

	t := theme.Current()

	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(t.GetAccent())).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true)

	left := " 🧠thinkt " + context

	if detail == "" {
		pad := width - lipgloss.Width(left)
		if pad < 0 {
			pad = 0
		}
		return barStyle.Render(left + strings.Repeat(" ", pad))
	}

	right := detail + " "
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return barStyle.Render(left + strings.Repeat(" ", gap) + right)
}
