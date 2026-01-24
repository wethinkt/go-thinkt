// thinkt-claude-prompts extracts user prompts from Claude Code trace files.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/prompt"
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/trace"
	"github.com/spf13/cobra"
)

var (
	inputFile  string
	outputFile string
	appendMode bool
	formatType string
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "thinkt-claude-prompts",
	Short: "Extract prompts from Claude Code traces",
	Long: `Extracts user prompts from Claude Code JSONL trace files
and generates a PROMPTS.md file with ISO 8601 timestamps.

Example:
  thinkt-claude-prompts extract -i ~/.claude/projects/abc123/session.jsonl
  thinkt-claude-prompts extract --latest
  thinkt-claude-prompts extract -i trace.jsonl -o prompts.md -f json`,
}

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract prompts from a trace file",
	RunE:  runExtract,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available Claude Code trace files",
	RunE:  runList,
}

func main() {
	rootCmd.AddCommand(extractCmd)
	rootCmd.AddCommand(listCmd)

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Extract flags
	extractCmd.Flags().StringVarP(&inputFile, "input", "i", "", "input JSONL file (use - for stdin)")
	extractCmd.Flags().StringVarP(&outputFile, "output", "o", "PROMPTS.md", "output file (use - for stdout)")
	extractCmd.Flags().BoolVarP(&appendMode, "append", "a", false, "append to existing file")
	extractCmd.Flags().StringVarP(&formatType, "format", "f", "markdown", "output format (markdown|json|plain)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runExtract(cmd *cobra.Command, args []string) error {
	// Validate input
	if inputFile == "" {
		// Try to find latest trace
		latest, err := findLatestTrace()
		if err != nil {
			return fmt.Errorf("no input file specified and could not find latest trace: %w", err)
		}
		inputFile = latest
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

	// Parse and extract
	parser := trace.NewParser(reader)
	extractor := prompt.NewExtractor(parser)

	prompts, err := extractor.Extract()
	if err != nil {
		return fmt.Errorf("extract prompts: %w", err)
	}

	// Report parse errors
	if verbose {
		for _, e := range extractor.Errors() {
			fmt.Fprintf(os.Stderr, "warning: %v\n", e)
		}
	}

	if verbose {
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

	// Format and write
	formatter := prompt.NewFormatter(writer, format)
	if err := formatter.Write(prompts); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	traces, err := findAllTraces()
	if err != nil {
		return err
	}

	if len(traces) == 0 {
		fmt.Println("No Claude Code traces found in ~/.claude/projects/")
		return nil
	}

	for _, t := range traces {
		fmt.Println(t)
	}
	return nil
}

// findLatestTrace finds the most recently modified trace file.
func findLatestTrace() (string, error) {
	traces, err := findAllTraces()
	if err != nil {
		return "", err
	}
	if len(traces) == 0 {
		return "", fmt.Errorf("no traces found in ~/.claude/projects/")
	}
	return traces[0], nil // Already sorted by mod time, newest first
}

// findAllTraces finds all Claude Code trace files.
func findAllTraces() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var traces []string
	err = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			traces = append(traces, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort by modification time, newest first
	sort.Slice(traces, func(i, j int) bool {
		iInfo, _ := os.Stat(traces[i])
		jInfo, _ := os.Stat(traces[j])
		if iInfo == nil || jInfo == nil {
			return false
		}
		return iInfo.ModTime().After(jInfo.ModTime())
	})

	return traces, nil
}
