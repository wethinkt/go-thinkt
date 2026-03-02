package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui/discover"
)

var discoverOK bool

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Scan for AI session sources and configure thinkt",
	RunE:  runDiscover,
}

func runDiscover(cmd *cobra.Command, args []string) error {
	factories := sources.AllFactories()

	if discoverOK || outputJSON {
		result, err := discover.RunDefaults(factories)
		if err != nil {
			return fmt.Errorf("discover defaults: %w", err)
		}
		if outputJSON {
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
		}
		startIndexerIfEnabled(result)
		return nil
	}

	return runDiscoverInteractive(factories)
}

func runDiscoverInteractive(factories []thinkt.StoreFactory) error {
	model := discover.New(factories)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("discover TUI: %w", err)
	}

	result := finalModel.(discover.Model).GetResult()
	if !result.Completed {
		fmt.Fprintln(os.Stderr, "Setup cancelled. Run 'thinkt discover' to try again.")
		return nil
	}

	if err := discover.SaveResult(result); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	startIndexerIfEnabled(result)
	return nil
}

func startIndexerIfEnabled(result discover.Result) {
	if !result.Indexer {
		return
	}

	binPath := config.FindIndexerBinary()
	if binPath == "" {
		return
	}

	confDir, err := config.Dir()
	if err != nil {
		return
	}

	logFile := filepath.Join(confDir, "logs", "indexer.log")
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return
	}

	c := exec.Command(binPath, "sync", "--quiet", "--log", logFile)
	_ = config.StartBackground(c)
}

// needsDiscover returns true when no config file exists on disk.
func needsDiscover() bool {
	_, err := config.Load()
	return errors.Is(err, config.ErrNoConfig)
}
