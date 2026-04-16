package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/index/llm"
)

var llmCmd = &cobra.Command{
	Use:   "llm",
	Short: "Manage local LLM operations (embedding, summarization, models)",
}

var llmStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current LLM model state and availability",
	Args:  cobra.NoArgs,
	RunE:  runLLMStatus,
}

func init() {
	llmCmd.AddCommand(llmStatusCmd)
	// LLM command — hidden from CLI surface; keep wiring intact for re-enable.
	// rootCmd.AddCommand(llmCmd)
}

func runLLMStatus(cmd *cobra.Command, args []string) error {
	models := llm.ListModels()
	for _, m := range models {
		path, err := llm.ModelPath(m.ID)
		if err != nil {
			continue
		}
		status := "not downloaded"
		if llm.IsModelDownloaded(path) {
			status = "ready"
		}
		fmt.Fprintf(os.Stdout, "%-30s %-12s %s\n", m.ID, m.Kind, status)
	}
	return nil
}
