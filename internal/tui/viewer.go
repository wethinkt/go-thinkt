package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func termSizeOpts() []tea.ProgramOption {
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
	return opts
}

// RunViewer runs a single session viewer TUI.
func RunViewer(sessionPath string) error {
	model := NewMultiViewerModel([]string{sessionPath})
	p := tea.NewProgram(model, termSizeOpts()...)
	_, err := p.Run()
	return err
}

// RunMultiViewer runs a multi-session viewer TUI.
func RunMultiViewer(sessionPaths []string) error {
	model := NewMultiViewerModel(sessionPaths)
	p := tea.NewProgram(model, termSizeOpts()...)
	_, err := p.Run()
	return err
}

// RunSessionBrowser runs a session picker with back-navigable viewer.
// Selecting a session opens the viewer; ESC returns to the picker via PopPageMsg.
func RunSessionBrowser(sessions []thinkt.SessionMeta) error {
	shell := NewShellWithSessions(sessions)
	p := tea.NewProgram(shell, termSizeOpts()...)
	_, err := p.Run()
	return err
}
