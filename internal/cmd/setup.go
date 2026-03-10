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
	"github.com/wethinkt/go-thinkt/internal/tui/setup"
)

var setupOK bool
var errSetupIncomplete = errors.New("setup did not complete")
var runSetupDefaultsFn = setup.RunDefaults
var runSetupInteractiveFn = runSetupInteractive

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Scan for AI session sources and configure thinkt",
	Args:  cobra.NoArgs,
	RunE:  runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	factories := sources.AllFactories()

	if setupOK || outputJSON {
		result, err := runSetupDefaultsFn(factories)
		if err != nil {
			return fmt.Errorf("setup defaults: %w", err)
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

	return runSetupInteractiveFn(factories)
}

func runSetupInteractive(factories []thinkt.StoreFactory) error {
	model := setup.New(factories)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("setup requires a terminal; run 'thinkt setup --ok' to configure with defaults: %w", err)
	}

	result := finalModel.(setup.Model).GetResult()
	if !result.Completed {
		fmt.Fprintln(os.Stderr, "Setup cancelled. Run 'thinkt setup' to try again.")
		return nil
	}

	if err := setup.SaveResult(result); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	startIndexerIfEnabled(result)
	return nil
}

func startIndexerIfEnabled(result setup.Result) {
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
	if err := os.MkdirAll(filepath.Dir(logFile), config.DirPerms); err != nil {
		return
	}

	c := exec.Command(binPath, "sync", "--quiet", "--log", logFile)
	_ = config.StartBackground(c)
}

// needsSetup returns true when no config file exists on disk.
func needsSetup() bool {
	_, err := config.Load()
	return errors.Is(err, config.ErrNoConfig)
}

// skipSetup returns true for commands that don't need config and should
// not trigger the first-run setup wizard. Checks the command and all
// its parents so that subcommands like "completion bash" are also skipped.
func skipSetup(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "setup",
			"help", "completion",
			"docs", "markdown", "man", // doc generation
			cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return true
		}
	}
	return false
}
