package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/export"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tui"
)

var (
	exportFormat  string
	exportOutput  string
	exportView    bool
	exportNoThink bool
	exportNoTools bool
	exportNoMedia bool
	exportSystem  bool
)

var exportCmd = &cobra.Command{
	Use:   "export [session]",
	Short: "Export a session as Markdown or HTML",
	Long: `Export a session as Markdown (default) or self-contained HTML.

Without arguments, exports the most recent session for the current project.
With a session argument, exports that specific session (ID, path, or suffix).

The --view flag pipes Markdown output through glow for terminal preview.

Examples:
  thinkt export                          # Latest session as Markdown to stdout
  thinkt export --html -o session.html   # Export as HTML to file
  thinkt export --view                   # Preview in terminal via glow
  thinkt export abc123                   # Export specific session
  thinkt export /path/to/session.jsonl   # Export by path`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	// Handle --html shorthand
	if html, _ := cmd.Flags().GetBool("html"); html {
		exportFormat = "html"
	}

	registry := CreateSourceRegistry()

	sessionPath, err := resolveExportSession(registry, args)
	if err != nil {
		return err
	}

	ls, err := tui.OpenLazySessionWithRegistry(sessionPath, registry)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer ls.Close()

	if err := ls.LoadAll(); err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	meta := ls.Metadata()
	entries := ls.Entries()

	opts := export.Options{
		Title:              buildExportTitle(meta),
		IncludeThinking:    !exportNoThink,
		IncludeToolUse:     !exportNoTools,
		IncludeToolResults: !exportNoTools,
		IncludeMedia:       !exportNoMedia,
		IncludeSystem:      exportSystem,
	}

	isHTML := exportFormat == "html"

	if exportView && isHTML {
		return fmt.Errorf("--view only works with Markdown format (remove --html)")
	}

	if exportView {
		return exportWithGlow(entries, opts)
	}

	w := os.Stdout
	if exportOutput != "" && exportOutput != "-" {
		f, err := os.Create(exportOutput)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		w = f
	}

	if isHTML {
		return export.ExportHTML(w, entries, opts)
	}
	return export.ExportMarkdown(w, entries, opts)
}

func resolveExportSession(registry *thinkt.StoreRegistry, args []string) (string, error) {
	ctx := context.Background()

	// Explicit absolute path
	if len(args) > 0 && filepath.IsAbs(args[0]) {
		return args[0], nil
	}

	// Auto-detect project from cwd
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	project := registry.FindProjectForPath(ctx, cwd)
	if project == nil {
		if len(args) > 0 {
			// Maybe it's a relative path to a session file
			absPath, err := filepath.Abs(args[0])
			if err == nil {
				if _, statErr := os.Stat(absPath); statErr == nil {
					return absPath, nil
				}
			}
		}
		return "", fmt.Errorf("no project found for current directory\n\nRun from inside a project directory, or specify a session path")
	}

	sessions, err := GetSessionsForProject(registry, project.ID, nil)
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions found in project %s", project.Name)
	}

	// Sort by ModifiedAt descending
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
	})

	// If session ID/suffix specified, match it
	if len(args) > 0 {
		for _, s := range sessions {
			if s.ID == args[0] ||
				strings.HasSuffix(s.FullPath, args[0]) ||
				strings.HasSuffix(s.FullPath, args[0]+".jsonl") {
				return s.FullPath, nil
			}
		}
		return "", fmt.Errorf("session not found: %s", args[0])
	}

	// Default: most recent session
	return sessions[0].FullPath, nil
}

func buildExportTitle(meta thinkt.SessionMeta) string {
	if meta.FirstPrompt != "" {
		title := meta.FirstPrompt
		if len(title) > 80 {
			title = title[:77] + "..."
		}
		return title
	}
	if meta.ProjectPath != "" {
		return filepath.Base(meta.ProjectPath) + " session"
	}
	return "Session Export"
}

func exportWithGlow(entries []thinkt.Entry, opts export.Options) error {
	glowPath, err := exec.LookPath("glow")
	if err != nil {
		return fmt.Errorf("glow not found — install with: brew install glow")
	}

	glowCmd := exec.Command(glowPath, "-p", "-")
	glowCmd.Stdout = os.Stdout
	glowCmd.Stderr = os.Stderr

	stdin, err := glowCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("glow stdin: %w", err)
	}

	if err := glowCmd.Start(); err != nil {
		return fmt.Errorf("start glow: %w", err)
	}

	exportErr := export.ExportMarkdown(stdin, entries, opts)
	stdin.Close()

	if waitErr := glowCmd.Wait(); waitErr != nil {
		return fmt.Errorf("glow: %w", waitErr)
	}
	return exportErr
}
