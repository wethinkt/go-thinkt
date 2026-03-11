package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

const maxRows = 200

type row struct {
	At        string
	Event     string
	String    string
	Keystroke string
	Code      string
	Text      string
	Mod       string
	Shifted   string
	Base      string
	Repeat    string
}

type model struct {
	width                  int
	height                 int
	rows                   []row
	supportsDisambiguation bool
	supportsEventTypes     bool
	flags                  int
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyboardEnhancementsMsg:
		m.flags = msg.Flags
		m.supportsDisambiguation = msg.SupportsKeyDisambiguation()
		m.supportsEventTypes = msg.SupportsEventTypes()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		m.appendRow("press", msg.String(), msg.Keystroke(), msg.Key())
	case tea.KeyReleaseMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		m.appendRow("release", msg.String(), msg.Keystroke(), msg.Key())
	}
	return m, nil
}

func (m *model) appendRow(kind, str, stroke string, k tea.Key) {
	m.rows = append(m.rows, row{
		At:        time.Now().Format("15:04:05.000"),
		Event:     kind,
		String:    str,
		Keystroke: stroke,
		Code:      fmt.Sprintf("%d", k.Code),
		Text:      quoteCell(k.Text),
		Mod:       fmt.Sprintf("%d", k.Mod),
		Shifted:   fmt.Sprintf("%d", k.ShiftedCode),
		Base:      fmt.Sprintf("%d", k.BaseCode),
		Repeat:    fmt.Sprintf("%t", k.IsRepeat),
	})
	if len(m.rows) > maxRows {
		m.rows = m.rows[len(m.rows)-maxRows:]
	}
}

func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString("Bubble Tea v2 key debug\n")
	b.WriteString("Press keys to inspect events. Press q or ctrl+c to quit.\n")
	b.WriteString(fmt.Sprintf(
		"Keyboard enhancements: flags=%d disambiguation=%t event_types=%t\n\n",
		m.flags,
		m.supportsDisambiguation,
		m.supportsEventTypes,
	))

	headers := []string{"time", "event", "string", "keystroke", "code", "text", "mod", "shifted", "base", "repeat"}
	widths := []int{12, 8, 18, 18, 8, 10, 6, 8, 6, 8}
	writeCells(&b, headers, widths)
	writeSeparator(&b, widths)

	rows := m.rows
	if maxVisible := m.maxVisibleRows(); len(rows) > maxVisible {
		rows = rows[len(rows)-maxVisible:]
	}
	for _, r := range rows {
		writeCells(&b, []string{
			r.At,
			r.Event,
			r.String,
			r.Keystroke,
			r.Code,
			r.Text,
			r.Mod,
			r.Shifted,
			r.Base,
			r.Repeat,
		}, widths)
	}

	var v tea.View
	v.SetContent(b.String())
	v.AltScreen = true
	v.KeyboardEnhancements.ReportEventTypes = true
	return v
}

func (m model) maxVisibleRows() int {
	if m.height <= 0 {
		return 30
	}
	rows := m.height - 6
	if rows < 1 {
		return 1
	}
	return rows
}

func writeCells(b *strings.Builder, cells []string, widths []int) {
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(padOrTrim(cell, widths[i]))
	}
	b.WriteByte('\n')
}

func writeSeparator(b *strings.Builder, widths []int) {
	for i, w := range widths {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Repeat("-", w))
	}
	b.WriteByte('\n')
}

func padOrTrim(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) > width {
		if width <= 1 {
			return string(r[:width])
		}
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-len(r))
}

func quoteCell(s string) string {
	return fmt.Sprintf("%q", s)
}

func main() {
	var opts []tea.ProgramOption
	for _, fd := range []int{int(os.Stdout.Fd()), int(os.Stdin.Fd()), int(os.Stderr.Fd())} {
		if term.IsTerminal(fd) {
			w, h, err := term.GetSize(fd)
			if err == nil && w > 0 && h > 0 {
				opts = append(opts, tea.WithWindowSize(w, h))
				break
			}
		}
	}

	p := tea.NewProgram(model{}, opts...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "keydebug: %v\n", err)
		os.Exit(1)
	}
}
