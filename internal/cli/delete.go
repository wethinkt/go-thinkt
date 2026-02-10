package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

// DeleteOptions configures project deletion behavior.
type DeleteOptions struct {
	Force  bool      // Skip confirmation prompt
	Stdout io.Writer // For writing output (defaults to os.Stdout)
}

// ProjectDeleter handles project deletion with confirmation.
type ProjectDeleter struct {
	registry *thinkt.StoreRegistry
	opts     DeleteOptions
}

// NewProjectDeleter creates a new project deleter.
func NewProjectDeleter(registry *thinkt.StoreRegistry, opts DeleteOptions) *ProjectDeleter {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &ProjectDeleter{registry: registry, opts: opts}
}

// Delete removes session files for a project after confirmation.
// projectPath can be project ID, full project path, or a path suffix.
// Returns an error if the project is not found or deletion fails.
func (d *ProjectDeleter) Delete(projectPath string) error {
	project, err := d.findProject(projectPath)
	if err != nil {
		return err
	}

	store, ok := d.registry.Get(project.Source)
	if !ok {
		return fmt.Errorf("source not available: %s", project.Source)
	}

	sessions, err := store.ListSessions(context.Background(), project.ID)
	if err != nil {
		return fmt.Errorf("list project sessions: %w", err)
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found in %s", project.Path)
	}

	// Show info and confirm
	if !d.opts.Force {
		// Display project info
		fmt.Fprintf(d.opts.Stdout, "Project: %s\n", project.Path)
		fmt.Fprintf(d.opts.Stdout, "Source: %s\n", project.Source)
		fmt.Fprintf(d.opts.Stdout, "Sessions: %d\n", len(sessions))
		if !project.LastModified.IsZero() {
			fmt.Fprintf(d.opts.Stdout, "Last modified: %s\n", project.LastModified.Format("2006-01-02 15:04"))
		}
		fmt.Fprintln(d.opts.Stdout)

		result, err := tui.Confirm(tui.ConfirmOptions{
			Prompt:      "Permanently delete all session data for this project?",
			Affirmative: "Delete",
			Negative:    "Cancel",
			Default:     false, // Default to Cancel
		})

		if err != nil || result != tui.ConfirmYes {
			fmt.Fprintf(d.opts.Stdout, "Cancelled.\n")
			return nil
		}
	}

	deleted := 0
	for _, session := range sessions {
		if session.FullPath == "" {
			continue
		}
		if err := os.Remove(session.FullPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("delete session %s: %w", session.FullPath, err)
		}
		deleted++
	}

	fmt.Fprintf(d.opts.Stdout, "Deleted %s (%d sessions)\n", project.Path, deleted)
	return nil
}

// findProject finds a project by its original path.
func (d *ProjectDeleter) findProject(targetPath string) (*thinkt.Project, error) {
	project, err := ResolveProject(d.registry, targetPath)
	if err != nil {
		return nil, fmt.Errorf("%w\n\nUse 'thinkt projects list' to see available projects", err)
	}
	return project, nil
}
