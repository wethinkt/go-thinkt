package cmd

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive TUI explorer",
	Long: `Browse Claude Code conversation sessions in a three-column
terminal interface. Navigate projects, sessions, and conversation
content with keyboard controls.

Column 1: Project directories
Column 2: Sessions with timestamps
Column 3: Conversation content with colored blocks

Press T to open thinking-tracer for the selected session.`,
	RunE: runTUI,
}

func runTUI(cmd *cobra.Command, args []string) error {
	tuilog.Log.Info("Starting TUI")

	// Get initial terminal size - try stdout, stdin, stderr in order
	var opts []tea.ProgramOption
	for _, fd := range []int{int(os.Stdout.Fd()), int(os.Stdin.Fd()), int(os.Stderr.Fd())} {
		if term.IsTerminal(fd) {
			w, h, err := term.GetSize(fd)
			if err == nil && w > 0 && h > 0 {
				tuilog.Log.Info("Terminal size", "fd", fd, "width", w, "height", h)
				opts = append(opts, tea.WithWindowSize(w, h))
				break
			}
		}
	}

	// Use new Shell with NavStack for multi-source support
	shell := tui.NewShell(tui.InitialPageAuto)
	p := tea.NewProgram(shell, opts...)
	_, err := p.Run()

	tuilog.Log.Info("TUI exited", "error", err)
	return err
}
