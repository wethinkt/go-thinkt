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

// ViewerResult holds the outcome of a standalone viewer run.
type ViewerResult struct {
	// Back is true if the user pressed esc (back), false if they pressed q/ctrl+c (quit).
	Back bool
}

// RunViewer runs a single session viewer TUI.
func RunViewer(sessionPath string) (ViewerResult, error) {
	return RunViewerWithRegistry(sessionPath, nil)
}

// RunViewerWithRegistry runs a single-session viewer with source-aware session loading.
func RunViewerWithRegistry(sessionPath string, registry *thinkt.StoreRegistry) (ViewerResult, error) {
	model := NewMultiViewerModelWithRegistry([]string{sessionPath}, registry)
	model.standalone = true
	p := tea.NewProgram(model, termSizeOpts()...)
	finalModel, err := p.Run()
	if err != nil {
		return ViewerResult{}, err
	}
	return ViewerResult{Back: finalModel.(MultiViewerModel).BackRequested()}, nil
}

// RunMultiViewer runs a multi-session viewer TUI.
func RunMultiViewer(sessionPaths []string) (ViewerResult, error) {
	return RunMultiViewerWithRegistry(sessionPaths, nil)
}

// RunMultiViewerWithRegistry runs a multi-session viewer with source-aware session loading.
func RunMultiViewerWithRegistry(sessionPaths []string, registry *thinkt.StoreRegistry) (ViewerResult, error) {
	model := NewMultiViewerModelWithRegistry(sessionPaths, registry)
	model.standalone = true
	p := tea.NewProgram(model, termSizeOpts()...)
	finalModel, err := p.Run()
	if err != nil {
		return ViewerResult{}, err
	}
	return ViewerResult{Back: finalModel.(MultiViewerModel).BackRequested()}, nil
}

// RunSessionBrowser runs a session picker with back-navigable viewer.
// Selecting a session opens the viewer; ESC returns to the picker via PopPageMsg.
func RunSessionBrowser(sessions []thinkt.SessionMeta) error {
	return RunSessionBrowserWithRegistry(sessions, nil, "")
}

// RunSessionBrowserWithRegistry runs a session picker with source-aware session loading.
// projectName is shown in the header breadcrumb; pass "" to auto-detect from session metadata.
func RunSessionBrowserWithRegistry(sessions []thinkt.SessionMeta, registry *thinkt.StoreRegistry, projectName string) error {
	shell := NewShellWithSessionsAndRegistry(sessions, registry, projectName)
	p := tea.NewProgram(shell, termSizeOpts()...)
	_, err := p.Run()
	return err
}
