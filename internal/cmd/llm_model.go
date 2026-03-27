package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/index/llm"
)

var llmModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage local LLM models",
}

var llmModelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available and downloaded models",
	Args:  cobra.NoArgs,
	RunE:  runLLMModelList,
}

var llmModelPullCmd = &cobra.Command{
	Use:   "pull <model-id>",
	Short: "Download a model",
	Args:  cobra.ExactArgs(1),
	RunE:  runLLMModelPull,
}

var llmModelCancelCmd = &cobra.Command{
	Use:   "cancel",
	Short: "Cancel an in-progress model download",
	Args:  cobra.NoArgs,
	RunE:  runLLMModelCancel,
}

func init() {
	llmModelCmd.AddCommand(llmModelListCmd)
	llmModelCmd.AddCommand(llmModelPullCmd)
	llmModelCmd.AddCommand(llmModelCancelCmd)
	llmCmd.AddCommand(llmModelCmd)
}

func runLLMModelList(cmd *cobra.Command, args []string) error {
	models := llm.ListModels()
	fmt.Fprintf(os.Stdout, "%-30s %-12s %-6s %s\n", "MODEL", "KIND", "DIM", "STATUS")
	fmt.Fprintf(os.Stdout, "%-30s %-12s %-6s %s\n", "-----", "----", "---", "------")
	for _, m := range models {
		path, err := llm.ModelPath(m.ID)
		if err != nil {
			continue
		}
		status := "not downloaded"
		if llm.IsModelDownloaded(path) {
			status = "ready"
		}
		dimStr := "-"
		if m.Dim > 0 {
			dimStr = fmt.Sprintf("%d", m.Dim)
		}
		fmt.Fprintf(os.Stdout, "%-30s %-12s %-6s %s\n", m.ID, m.Kind, dimStr, status)
	}
	return nil
}

func runLLMModelPull(cmd *cobra.Command, args []string) error {
	modelID := args[0]
	if _, err := llm.LookupModel(modelID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Downloading model %s...\n", modelID)
	if err := llm.EnsureModel(modelID, func(downloaded, total int64) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			fmt.Fprintf(os.Stderr, "\rDownloading: %.1f%% (%d / %d bytes)", pct, downloaded, total)
		}
	}); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\nModel %s downloaded successfully.\n", modelID)
	return nil
}

func runLLMModelCancel(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(os.Stderr, "Not yet implemented — kill the process to cancel a download.")
	return nil
}
