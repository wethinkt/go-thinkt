// thinkt is an interactive TUI explorer for Claude Code sessions.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
)

var baseDir string

var rootCmd = &cobra.Command{
	Use:   "thinkt",
	Short: "Interactive TUI explorer for Claude Code sessions",
	Long: `Browse Claude Code conversation sessions in a three-column
terminal interface. Navigate projects, sessions, and conversation
content with keyboard controls.

Column 1: Project directories
Column 2: Sessions with timestamps
Column 3: Conversation content with colored blocks

Press T to open thinking-tracer for the selected session.`,
	RunE: runTUI,
}

func main() {
	rootCmd.Flags().StringVarP(&baseDir, "dir", "d", "", "base directory (default ~/.claude)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	model := tui.NewModel(baseDir)
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}
