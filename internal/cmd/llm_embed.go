package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/index/llm"
)

var llmEmbedForce bool

var llmEmbedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Run embedding pass on all indexed sessions",
	Args:  cobra.NoArgs,
	RunE:  runLLMEmbed,
}

func init() {
	llmEmbedCmd.Flags().BoolVar(&llmEmbedForce, "force", false, "re-embed all sessions (ignore existing embeddings)")
	llmCmd.AddCommand(llmEmbedCmd)
}

func runLLMEmbed(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	modelID := cfg.Embedding.Model
	if modelID == "" {
		modelID = "nomic-embed-text-v1.5"
	}

	fmt.Fprintf(os.Stderr, "Ensuring embedding model %s...\n", modelID)
	if err := llm.EnsureModel(modelID, func(downloaded, total int64) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			fmt.Fprintf(os.Stderr, "\rDownloading: %.1f%%", pct)
		}
	}); err != nil {
		return fmt.Errorf("ensure model: %w", err)
	}
	fmt.Fprintln(os.Stderr, " done")

	// TODO: Wire actual embedding loop when ingester is adapted to SQLite storage.
	// For now, this command ensures the model is downloaded and ready.
	fmt.Fprintln(os.Stderr, "Embedding pass not yet wired to SQLite storage.")
	return nil
}
