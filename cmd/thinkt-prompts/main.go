// thinkt-prompts extracts user prompts from LLM agent trace files.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/prompt"
	"github.com/spf13/cobra"
)

// Supported trace types
const (
	TraceTypeClaude = "claude"
)

var supportedTypes = []string{TraceTypeClaude}

var (
	inputFile    string
	outputFile   string
	appendMode   bool
	formatType   string
	templateFile string
	traceType    string
	baseDir      string
	verbose      bool
)

var rootCmd = &cobra.Command{
	Use:   "thinkt-prompts",
	Short: "Extract prompts from LLM agent traces",
	Long: `Extracts user prompts from LLM agent trace files
and generates a PROMPTS.md file with ISO 8601 timestamps.

Supported trace types:
  claude    Claude Code JSONL traces (~/.claude/projects/)

Example:
  thinkt-prompts extract -t claude -i session.jsonl
  thinkt-prompts extract -t claude --latest
  thinkt-prompts list -t claude
  thinkt-prompts templates`,
}

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract prompts from a trace file",
	RunE:  runExtract,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available trace files",
	RunE:  runList,
}

var infoCmd = &cobra.Command{
	Use:   "info [file]",
	Short: "Show session information",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runInfo,
}

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List available templates and show template variables",
	RunE:  runTemplates,
}

func main() {
	rootCmd.AddCommand(extractCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(templatesCmd)

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&traceType, "type", "t", TraceTypeClaude, "trace type (claude)")
	rootCmd.PersistentFlags().StringVarP(&baseDir, "dir", "d", "", "base directory for trace files (default ~/.claude)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Extract flags
	extractCmd.Flags().StringVarP(&inputFile, "input", "i", "", "input trace file (use - for stdin)")
	extractCmd.Flags().StringVarP(&outputFile, "output", "o", "PROMPTS.md", "output file (use - for stdout)")
	extractCmd.Flags().BoolVarP(&appendMode, "append", "a", false, "append to existing file")
	extractCmd.Flags().StringVarP(&formatType, "format", "f", "markdown", "output format (markdown|json|plain)")
	extractCmd.Flags().StringVar(&templateFile, "template", "", "custom template file (for markdown format)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func validateTraceType() error {
	for _, t := range supportedTypes {
		if traceType == t {
			return nil
		}
	}
	return fmt.Errorf("unsupported trace type: %s (supported: %v)", traceType, supportedTypes)
}

func runExtract(cmd *cobra.Command, args []string) error {
	if err := validateTraceType(); err != nil {
		return err
	}

	// Validate input
	if inputFile == "" {
		switch traceType {
		case TraceTypeClaude:
			latest, err := claude.FindLatestSession(baseDir)
			if err != nil {
				return fmt.Errorf("could not find latest trace: %w", err)
			}
			if latest == "" {
				dir := baseDir
			if dir == "" {
				dir = "~/.claude"
			}
			return fmt.Errorf("no traces found in %s/projects/", dir)
			}
			inputFile = latest
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Using latest trace: %s\n", inputFile)
		}
	}

	// Parse format
	format, err := prompt.ParseFormat(formatType)
	if err != nil {
		return err
	}

	// Open input
	var reader io.Reader
	if inputFile == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(inputFile)
		if err != nil {
			return fmt.Errorf("open input: %w", err)
		}
		defer f.Close()
		reader = f
	}

	// Parse and extract based on trace type
	var prompts []prompt.Prompt
	var parseErrors []error

	switch traceType {
	case TraceTypeClaude:
		parser := claude.NewParser(reader)
		extractor := prompt.NewExtractor(parser)
		prompts, err = extractor.Extract()
		parseErrors = extractor.Errors()
	}

	if err != nil {
		return fmt.Errorf("extract prompts: %w", err)
	}

	// Report parse errors
	if verbose {
		for _, e := range parseErrors {
			fmt.Fprintf(os.Stderr, "warning: %v\n", e)
		}
		fmt.Fprintf(os.Stderr, "Extracted %d prompts\n", len(prompts))
	}

	// Open output
	var writer io.Writer
	if outputFile == "-" {
		writer = os.Stdout
	} else {
		flags := os.O_CREATE | os.O_WRONLY
		if appendMode {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		f, err := os.OpenFile(outputFile, flags, 0644)
		if err != nil {
			return fmt.Errorf("open output: %w", err)
		}
		defer f.Close()
		writer = f
	}

	// Build formatter options
	var opts []prompt.FormatterOption

	// Load custom template if specified
	if templateFile != "" && format == prompt.FormatMarkdown {
		tmpl, err := prompt.LoadTemplateFile(templateFile)
		if err != nil {
			return fmt.Errorf("load template: %w", err)
		}
		opts = append(opts, prompt.WithTemplate(tmpl))
		if verbose {
			fmt.Fprintf(os.Stderr, "Using template: %s\n", templateFile)
		}
	}

	// Format and write
	formatter := prompt.NewFormatter(writer, format, opts...)
	if err := formatter.Write(prompts); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	if err := validateTraceType(); err != nil {
		return err
	}

	var sessions []string
	var err error

	switch traceType {
	case TraceTypeClaude:
		sessions, err = claude.FindSessions(baseDir)
	}

	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Printf("No %s traces found\n", traceType)
		return nil
	}

	for _, s := range sessions {
		fmt.Println(s)
	}
	return nil
}

func runInfo(cmd *cobra.Command, args []string) error {
	if err := validateTraceType(); err != nil {
		return err
	}

	var path string
	if len(args) > 0 {
		path = args[0]
	} else {
		switch traceType {
		case TraceTypeClaude:
			latest, err := claude.FindLatestSession(baseDir)
			if err != nil {
				return err
			}
			if latest == "" {
				return fmt.Errorf("no %s traces found", traceType)
			}
			path = latest
		}
	}

	switch traceType {
	case TraceTypeClaude:
		return showClaudeInfo(path)
	}

	return nil
}

func showClaudeInfo(path string) error {
	session, err := claude.LoadSession(path)
	if err != nil {
		return err
	}

	fmt.Printf("Session: %s\n", session.ID)
	fmt.Printf("Path:    %s\n", session.Path)
	fmt.Printf("Model:   %s\n", session.Model)
	fmt.Printf("Version: %s\n", session.Version)
	fmt.Printf("Branch:  %s\n", session.Branch)
	fmt.Printf("CWD:     %s\n", session.CWD)
	fmt.Printf("Start:   %s\n", session.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("End:     %s\n", session.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration: %s\n", session.Duration().Round(1e9))
	fmt.Printf("Turns:   %d\n", session.TurnCount())
	fmt.Printf("Entries: %d\n", len(session.Entries))

	return nil
}

func runTemplates(cmd *cobra.Command, args []string) error {
	fmt.Println("Available Templates")
	fmt.Println("===================")
	fmt.Println()

	templates, err := prompt.ListEmbeddedTemplates()
	if err != nil {
		return err
	}

	fmt.Println("Embedded templates:")
	for _, t := range templates {
		fmt.Printf("  - %s\n", t)
	}

	fmt.Println()
	fmt.Println(prompt.DefaultTemplateHelp)

	return nil
}
