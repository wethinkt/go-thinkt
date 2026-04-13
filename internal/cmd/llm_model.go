package cmd

import (
	"fmt"
	"os"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/index/llm"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
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

	t := theme.Current()
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.GetAccent()))
	primaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg))
	secondaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))

	type row struct {
		id, kind, dim, status string
		ready                 bool
	}

	var rows []row
	for _, m := range models {
		path, err := llm.ModelPath(m.ID)
		if err != nil {
			continue
		}
		ready := llm.IsModelDownloaded(path)
		status := "not downloaded"
		if ready {
			status = "ready"
		}
		dimStr := "-"
		if m.Dim > 0 {
			dimStr = fmt.Sprintf("%d", m.Dim)
		}
		rows = append(rows, row{m.ID, string(m.Kind), dimStr, status, ready})
	}

	const gap = 2
	colModel := 5  // "MODEL"
	colKind := 4   // "KIND"
	colDim := 3    // "DIM"
	for _, r := range rows {
		if len(r.id) > colModel {
			colModel = len(r.id)
		}
		if len(r.kind) > colKind {
			colKind = len(r.kind)
		}
		if len(r.dim) > colDim {
			colDim = len(r.dim)
		}
	}
	colModel += gap
	colKind += gap
	colDim += gap

	col := func(s lipgloss.Style, w int) lipgloss.Style { return s.Width(w) }

	fmt.Fprintf(os.Stdout, "%s%s%s%s\n",
		col(headerStyle, colModel).Render("MODEL"),
		col(headerStyle, colKind).Render("KIND"),
		col(headerStyle, colDim).Render("DIM"),
		headerStyle.Render("STATUS"))

	for _, r := range rows {
		statusStyle := mutedStyle
		if r.ready {
			statusStyle = accentStyle
		}
		fmt.Fprintf(os.Stdout, "%s%s%s%s\n",
			col(primaryStyle, colModel).Render(r.id),
			col(secondaryStyle, colKind).Render(r.kind),
			col(mutedStyle, colDim).Render(r.dim),
			statusStyle.Render(r.status))
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
