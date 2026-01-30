package tui

import (
	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
	"os"
)

// RunViewer runs a single session viewer TUI.
func RunViewer(sessionPath string) error {
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

	model := NewMultiViewerModel([]string{sessionPath})
	p := tea.NewProgram(model, opts...)
	_, err := p.Run()
	return err
}

// RunMultiViewer runs a multi-session viewer TUI.
func RunMultiViewer(sessionPaths []string) error {
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

	model := NewMultiViewerModel(sessionPaths)
	p := tea.NewProgram(model, opts...)
	_, err := p.Run()
	return err
}
