package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/export"
	"github.com/wethinkt/go-thinkt/internal/prompt"
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

	exportTmplFile   string
	exportTmplFormat string
	exportTmplOutput string
)

var exportCmd = &cobra.Command{
	Use:   "export [session]",
	Short: "Export a session as Markdown, HTML, or JSON",
	Long: `Export a session as Markdown (default), self-contained HTML, or JSON.

Without arguments, exports the most recent session for the current project.
With a session argument, exports that specific session (ID, path, or suffix).

The --view flag previews the export: pipes Markdown through glow, or opens
HTML in the default browser.

Examples:
  thinkt export                          # Latest session as Markdown to stdout
  thinkt export --html -o session.html   # Export as HTML to file
  thinkt export --json                   # Export as JSON
  thinkt export --view                   # Preview Markdown in terminal via glow
  thinkt export --html --view            # Export HTML and open in browser
  thinkt export abc123                   # Export specific session`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	// Handle format shorthands
	if html, _ := cmd.Flags().GetBool("html"); html {
		exportFormat = "html"
	}
	if j, _ := cmd.Flags().GetBool("json"); j {
		exportFormat = "json"
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
		Title: buildExportTitle(meta),
	}

	if exportView {
		switch exportFormat {
		case "html":
			return exportViewHTML(entries, opts)
		case "json":
			return fmt.Errorf("--view is not supported with JSON format")
		default:
			return exportWithGlow(entries, opts)
		}
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

	switch exportFormat {
	case "html":
		return export.ExportHTML(w, entries, opts)
	case "json":
		return export.ExportJSON(w, entries, opts)
	default:
		return export.ExportMarkdown(w, entries, opts)
	}
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

func exportViewHTML(entries []thinkt.Entry, opts export.Options) error {
	f, err := os.CreateTemp("", "thinkt-export-*.html")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if err := export.ExportHTML(f, entries, opts); err != nil {
		f.Close()
		return err
	}
	f.Close()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", f.Name())
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", f.Name())
	default:
		cmd = exec.Command("xdg-open", f.Name())
	}
	return cmd.Start()
}

var exportTemplateCmd = &cobra.Command{
	Use:   "template [session]",
	Short: "Export user prompts using a Go template",
	Long: `Extract user prompts from a session and render them using a Go template.

Outputs Markdown by default using the built-in template, or use --template
to provide a custom Go template file. Use --json for structured JSON output,
or --format plain for raw text.

Examples:
  thinkt export template                        # Prompts as Markdown
  thinkt export template --json                 # Prompts as JSON
  thinkt export template --format plain         # Raw prompt text
  thinkt export template --template my.tmpl     # Custom template`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExportTemplate,
}

func runExportTemplate(cmd *cobra.Command, args []string) error {
	// Handle --json shorthand
	if j, _ := cmd.Flags().GetBool("json"); j {
		exportTmplFormat = "json"
	}

	format, err := prompt.ParseFormat(exportTmplFormat)
	if err != nil {
		return err
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

	// Extract user prompts from entries
	entries := ls.Entries()
	var prompts []prompt.Prompt
	for _, entry := range entries {
		if entry.Role != thinkt.RoleUser {
			continue
		}
		text := entry.Text
		if text == "" {
			for _, block := range entry.ContentBlocks {
				if block.Type == "text" && block.Text != "" {
					text = block.Text
					break
				}
			}
		}
		if text == "" {
			continue
		}
		prompts = append(prompts, prompt.Prompt{
			Text:      text,
			Timestamp: entry.Timestamp.Format("2006-01-02T15:04:05Z"),
			UUID:      entry.UUID,
		})
	}

	// Open output
	w := os.Stdout
	if exportTmplOutput != "" && exportTmplOutput != "-" {
		f, err := os.Create(exportTmplOutput)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		w = f
	}

	// Build formatter options
	var opts []prompt.FormatterOption
	if exportTmplFile != "" && format == prompt.FormatMarkdown {
		tmpl, err := prompt.LoadTemplateFile(exportTmplFile)
		if err != nil {
			return fmt.Errorf("load template: %w", err)
		}
		opts = append(opts, prompt.WithTemplate(tmpl))
	}

	formatter := prompt.NewFormatter(w, format, opts...)
	return formatter.Write(prompts)
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
