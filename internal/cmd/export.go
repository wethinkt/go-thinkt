package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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

	// Build target flags from CLI flags and positional args
	flags := target.Flags{
		Project: exportProject,
		Sources: exportSources,
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

	// Content filtering
	filter := target.DefaultFilter()
	filterFlagsSet := cmd.Flags().Changed("no-thinking") ||
		cmd.Flags().Changed("no-tools") ||
		cmd.Flags().Changed("no-media") ||
		cmd.Flags().Changed("system")

	if filterFlagsSet {
		filter.IncludeThinking = !exportNoThink
		filter.IncludeToolUse = !exportNoTools
		filter.IncludeToolResults = !exportNoTools
		filter.IncludeMedia = !exportNoMedia
		filter.IncludeSystem = exportSystem
	} else if target.IsTTY() {
		filter, err = target.PickContentFilter(filter)
		if err != nil {
			return err
		}
	}

	entries := target.FilterEntries(result.Entries, filter)

	// Format picker
	if !cmd.Flags().Changed("format") &&
		!cmd.Flags().Changed("html") &&
		!cmd.Flags().Changed("json") &&
		!cmd.Flags().Changed("md") {
		if target.IsTTY() {
			exportFormat, err = tui.PickFormat()
			if err != nil {
				return err
			}
		}
	}

	opts := export.Options{
		Title: buildExportTitle(result.Meta),
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

	// Output destination picker
	if !cmd.Flags().Changed("output") && target.IsTTY() {
		ext := "." + exportFormat
		suggested := sanitizeFilename(opts.Title) + ext
		choice, err := tui.PickOutput(suggested)
		if err != nil {
			return err
		}
		if choice.Mode == "file" {
			exportOutput = choice.Path
		}
		// stdout: exportOutput stays ""
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

func sanitizeFilename(s string) string {
	// Replace characters unsafe for filenames
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "", "?", "",
		"\"", "", "<", "", ">", "", "|", "", "\n", " ",
	)
	name := replacer.Replace(s)
	if len(name) > 60 {
		name = name[:60]
	}
	return strings.TrimSpace(name)
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
		Project: exportProject,
		Sources: exportSources,
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
