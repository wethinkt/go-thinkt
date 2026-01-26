// thinkt is an interactive TUI explorer for Claude Code sessions.
package main

import (
	"fmt"
	"os"
	"runtime/pprof"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tuilog"
)

var (
	baseDir     string
	profilePath string
	logPath     string
)

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
	rootCmd.Flags().StringVar(&profilePath, "profile", "", "write CPU profile to file (use with go tool pprof)")
	rootCmd.Flags().StringVar(&logPath, "log", "", "write debug log to file (e.g., --log thinkt.log)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	// Initialize logger if requested
	if logPath != "" {
		if err := tuilog.Init(logPath); err != nil {
			return fmt.Errorf("init logger: %w", err)
		}
		defer tuilog.Log.Close()
	}

	// Start CPU profiling if requested
	if profilePath != "" {
		f, err := os.Create(profilePath)
		if err != nil {
			return fmt.Errorf("create profile file: %w", err)
		}
		defer f.Close()

		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("start CPU profile: %w", err)
		}
		defer pprof.StopCPUProfile()
	}

	tuilog.Log.Info("Starting TUI", "baseDir", baseDir)

	model := tui.NewModel(baseDir)
	p := tea.NewProgram(model)
	_, err := p.Run()

	tuilog.Log.Info("TUI exited", "error", err)
	return err
}
