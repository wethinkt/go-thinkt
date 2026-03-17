package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/export"
	"github.com/wethinkt/go-thinkt/internal/prompt"
	"github.com/wethinkt/go-thinkt/internal/target"
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

	exportProject string
	exportSession string
	exportSources []string

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

	flags := target.Flags{
		Project:       exportProject,
		Sources:       exportSources,
		HeaderContext: "export",
	}
	if len(args) > 0 {
		flags.Session = args[0]
	} else if exportSession != "" {
		flags.Session = exportSession
	}

	// Non-TTY: use the original sequential flow
	if !target.IsTTY() {
		return runExportNonInteractive(cmd, registry, flags)
	}

	// TTY: run the unified export wizard
	return runExportWizard(cmd, registry, flags)
}

func runExportWizard(cmd *cobra.Command, registry *thinkt.StoreRegistry, flags target.Flags) error {
	config := exportWizardConfig{
		ViewMode: exportView,
	}

	// Pre-resolve filter from flags
	filterFlagsSet := cmd.Flags().Changed("no-thinking") ||
		cmd.Flags().Changed("no-tools") ||
		cmd.Flags().Changed("no-media") ||
		cmd.Flags().Changed("system")
	if filterFlagsSet {
		f := target.ContentFilter{
			IncludeThinking:    !exportNoThink,
			IncludeToolUse:     !exportNoTools,
			IncludeToolResults: !exportNoTools,
			IncludeMedia:       !exportNoMedia,
			IncludeSystem:      exportSystem,
		}
		config.Filter = &f
	}

	// Pre-resolve format from flags
	if cmd.Flags().Changed("format") || cmd.Flags().Changed("html") ||
		cmd.Flags().Changed("json") || cmd.Flags().Changed("md") {
		config.Format = exportFormat
	}

	// Pre-resolve project/session if possible
	res, err := target.ResolveProjectNonInteractive(registry, flags)
	if err != nil {
		return err
	}

	if res.Resolved {
		config.ProjectID = res.ProjectID
		config.ProjectName = res.ProjectName

		// Try to resolve session too
		if flags.Session != "" && filepath.IsAbs(flags.Session) {
			result, err := target.LoadSession(flags.Session, res.ProjectName, registry)
			if err != nil {
				return err
			}
			config.Session = &result.Meta
		} else {
			sessions, err := target.GetSessionsForProject(registry, res.ProjectID, flags.Sources)
			if err != nil {
				return err
			}
			if flags.Session != "" {
				if s := target.ResolveSessionByID(sessions, flags.Session); s != nil {
					config.Session = s
				} else {
					return fmt.Errorf("session not found: %s", flags.Session)
				}
			} else {
				config.Sessions = sessions
			}
		}
	}

	termW, termH := tui.TermSize()
	wizard := newExportWizard(registry, flags, config, termW, termH)
	p := tea.NewProgram(wizard, tea.WithWindowSize(termW, termH))
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	wiz := finalModel.(exportWizardModel)
	if wiz.err != nil {
		return wiz.err
	}
	if wiz.result.Cancelled {
		return fmt.Errorf("cancelled")
	}

	// Load the session entries
	result, err := target.LoadSession(wiz.result.Session.FullPath, wiz.result.ProjectName, registry)
	if err != nil {
		return err
	}

	entries := target.FilterEntries(result.Entries, wiz.result.Filter)
	opts := export.Options{
		Title: buildExportTitle(result.Meta),
	}

	if exportView {
		switch wiz.result.Format {
		case "html":
			if err := exportViewHTML(entries, opts); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "exported %s to browser\n", formatDisplayName(wiz.result.Format))
			return nil
		case "json":
			return fmt.Errorf("--view is not supported with JSON format")
		default:
			if err := exportWithGlow(entries, opts); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "exported %s to terminal\n", formatDisplayName(wiz.result.Format))
			return nil
		}
	}

	// Write output
	outputPath := ""
	if wiz.result.Output != nil && wiz.result.Output.Mode == "file" {
		outputPath = wiz.result.Output.Path
	}
	if cmd.Flags().Changed("output") {
		outputPath = exportOutput
	}

	w := os.Stdout
	if outputPath != "" && outputPath != "-" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		w = f
	}

	var exportErr error
	switch wiz.result.Format {
	case "html":
		exportErr = export.ExportHTML(w, entries, opts)
	case "json":
		exportErr = export.ExportJSON(w, entries, opts)
	default:
		exportErr = export.ExportMarkdown(w, entries, opts)
	}
	if exportErr != nil {
		return exportErr
	}

	dest := "stdout"
	if outputPath != "" && outputPath != "-" {
		dest = outputPath
	}
	fmt.Fprintf(os.Stderr, "exported %s to %s\n", formatDisplayName(wiz.result.Format), dest)
	return nil
}

func runExportNonInteractive(cmd *cobra.Command, registry *thinkt.StoreRegistry, flags target.Flags) error {
	result, err := target.ResolveSession(registry, flags)
	if err != nil {
		return err
	}

	filter := target.DefaultFilter()
	if cmd.Flags().Changed("no-thinking") || cmd.Flags().Changed("no-tools") ||
		cmd.Flags().Changed("no-media") || cmd.Flags().Changed("system") {
		filter.IncludeThinking = !exportNoThink
		filter.IncludeToolUse = !exportNoTools
		filter.IncludeToolResults = !exportNoTools
		filter.IncludeMedia = !exportNoMedia
		filter.IncludeSystem = exportSystem
	}

	entries := target.FilterEntries(result.Entries, filter)
	opts := export.Options{Title: buildExportTitle(result.Meta)}

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

func formatDisplayName(format string) string {
	switch format {
	case "html":
		return "HTML"
	case "json":
		return "JSON"
	default:
		return "Markdown"
	}
}

func filterSummary(f target.ContentFilter) string {
	var included []string
	if f.IncludeThinking {
		included = append(included, "thinking")
	}
	if f.IncludeToolUse {
		included = append(included, "tools")
	}
	if f.IncludeMedia {
		included = append(included, "media")
	}
	if f.IncludeSystem {
		included = append(included, "system")
	}
	if len(included) == 0 {
		return "text only"
	}
	return strings.Join(included, ", ")
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

	// Build target flags
	flags := target.Flags{
		Project:       exportProject,
		Sources:       exportSources,
		HeaderContext: "export",
	}
	if len(args) > 0 {
		flags.Session = args[0]
	} else if exportSession != "" {
		flags.Session = exportSession
	}

	result, err := target.ResolveSession(registry, flags)
	if err != nil {
		return err
	}

	// Extract user prompts from entries
	var prompts []prompt.Prompt
	for _, entry := range result.Entries {
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
	var fmtOpts []prompt.FormatterOption
	if exportTmplFile != "" && format == prompt.FormatMarkdown {
		tmpl, err := prompt.LoadTemplateFile(exportTmplFile)
		if err != nil {
			return fmt.Errorf("load template: %w", err)
		}
		fmtOpts = append(fmtOpts, prompt.WithTemplate(tmpl))
	}

	formatter := prompt.NewFormatter(w, format, fmtOpts...)
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
