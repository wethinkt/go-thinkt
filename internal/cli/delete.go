package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/sources/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/tui"
)

// DeleteOptions configures project deletion behavior.
type DeleteOptions struct {
	Force  bool      // Skip confirmation prompt
	Stdout io.Writer // For writing output (defaults to os.Stdout)
}

// ProjectDeleter handles project deletion with confirmation.
type ProjectDeleter struct {
	baseDir string
	opts    DeleteOptions
}

// NewProjectDeleter creates a new project deleter.
func NewProjectDeleter(baseDir string, opts DeleteOptions) *ProjectDeleter {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &ProjectDeleter{baseDir: baseDir, opts: opts}
}

// Delete removes a project directory after confirmation.
// projectPath can be the full original project path (e.g., /Users/evan/myproject).
// Returns an error if the project is not found or deletion fails.
func (d *ProjectDeleter) Delete(projectPath string) error {
	// Resolve to absolute path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Find the project in our list
	// Note: findProject returns an error if the directory has no sessions
	project, err := d.findProject(absPath)
	if err != nil {
		return err
	}

	// Show info and confirm
	if !d.opts.Force {
		// Display project info
		fmt.Fprintf(d.opts.Stdout, "Project: %s\n", project.FullPath)
		fmt.Fprintf(d.opts.Stdout, "Sessions: %d\n", project.SessionCount)
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

	// Delete the project directory
	if err := os.RemoveAll(project.DirPath); err != nil {
		return fmt.Errorf("delete project directory: %w", err)
	}

	fmt.Fprintf(d.opts.Stdout, "Deleted %s (%d sessions)\n", project.FullPath, project.SessionCount)
	return nil
}

// findProject finds a project by its original path.
func (d *ProjectDeleter) findProject(targetPath string) (*claude.Project, error) {
	projects, err := claude.ListProjects(d.baseDir)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	for _, p := range projects {
		if p.FullPath == targetPath {
			return &p, nil
		}
	}

	// Check if directory exists but has no sessions (more helpful error)
	encodedName := encodePathToDirName(targetPath)
	projectsDir, _ := claude.ProjectsDir(d.baseDir)
	potentialDir := filepath.Join(projectsDir, encodedName)
	if info, err := os.Stat(potentialDir); err == nil && info.IsDir() {
		return nil, fmt.Errorf("no sessions found in %s\n\nThis tool only deletes Claude project directories containing session data", targetPath)
	}

	return nil, fmt.Errorf("project not found: %s\n\nUse 'thinkt projects' to list available projects", targetPath)
}

// encodePathToDirName converts a path to Claude's directory name format.
// e.g., /Users/evan/myproject -> -Users-evan-myproject
func encodePathToDirName(path string) string {
	if path == "" {
		return "-"
	}
	// Replace / with - and ensure leading -
	encoded := strings.ReplaceAll(path, "/", "-")
	if !strings.HasPrefix(encoded, "-") {
		encoded = "-" + encoded
	}
	return encoded
}
