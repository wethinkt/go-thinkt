package cli

import (
	"fmt"
	"io"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// FormatTasks prints one line per task, naming the prompt that started it.
func FormatTasks(w io.Writer, tasks []thinkt.BackgroundTask) {
	if len(tasks) == 0 {
		fmt.Fprintln(w, "No background tasks.")
		return
	}
	for _, t := range tasks {
		fmt.Fprintf(w, "%-24s %-8s %-10s %s\n", t.ID, t.Kind, t.Status, t.Description)
		if t.PromptID != "" || t.TurnID != "" {
			fmt.Fprintf(w, "  started in prompt %s, turn %s\n", t.PromptID, t.TurnID)
		}
	}
}
