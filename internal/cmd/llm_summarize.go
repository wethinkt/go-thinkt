package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/index/llm"
	"github.com/wethinkt/go-thinkt/internal/index/summarize"
)

var llmSummarizeForce bool

var llmSummarizeCmd = &cobra.Command{
	Use:   "summarize",
	Short: "Run summarization pass on all indexed sessions",
	Args:  cobra.NoArgs,
	RunE:  runLLMSummarize,
}

func init() {
	llmSummarizeCmd.Flags().BoolVar(&llmSummarizeForce, "force", false, "re-summarize all sessions")
	llmCmd.AddCommand(llmSummarizeCmd)
}

func runLLMSummarize(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	modelID := cfg.Summarization.Model
	if modelID == "" {
		modelID = summarize.DefaultModelID
	}

	fmt.Fprintf(os.Stderr, "Ensuring summarization model %s...\n", modelID)
	if err := llm.EnsureModel(modelID, func(downloaded, total int64) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			fmt.Fprintf(os.Stderr, "\rDownloading: %.1f%%", pct)
		}
	}); err != nil {
		return fmt.Errorf("ensure model: %w", err)
	}
	fmt.Fprintln(os.Stderr, " done")

	// TODO: Wire actual summarization loop when ingester is adapted to SQLite storage.
	fmt.Fprintln(os.Stderr, "Summarization pass not yet wired to SQLite storage.")
	return nil
}
