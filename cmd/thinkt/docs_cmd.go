package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var (
	docsOutputDir        string
	docsEnableAutoGenTag bool
	docsHugo             bool
)

var docsCmd = &cobra.Command{
	Use:    "docs",
	Short:  "Generate documentation for thinkt",
	Hidden: true,
	Long: `Generate documentation for all thinkt commands.

Subcommands:
  markdown  Generate plain markdown (default)
  man       Generate man pages

The auto-generation tag (timestamp footer) is disabled by default for stable,
reproducible files. Use --enableAutoGenTag for publishing.

Examples:
  thinkt docs                       # Generate markdown docs in ./docs/
  thinkt docs markdown -o ./wiki    # Generate markdown in custom directory
  thinkt docs markdown --hugo -o docs/command  # Generate Hugo-compatible docs
  thinkt docs man -o /usr/share/man/man1`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to markdown subcommand
		return runDocsMarkdown(cmd, args)
	},
}

var docsMarkdownCmd = &cobra.Command{
	Use:   "markdown",
	Short: "Generate markdown documentation",
	Long: `Generate markdown documentation for all thinkt commands.

By default, generates plain markdown suitable for GitHub wikis and basic
documentation sites. Use --hugo to generate markdown with YAML front matter
for Hugo static site generator.`,
	RunE: runDocsMarkdown,
}

var docsManCmd = &cobra.Command{
	Use:   "man",
	Short: "Generate man pages",
	Long: `Generate man pages for all thinkt commands.

Man pages are generated in roff format suitable for installation
in /usr/share/man/man1 or /usr/local/share/man/man1.`,
	RunE: runDocsMan,
}

func runDocsMarkdown(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(docsOutputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	rootCmd.DisableAutoGenTag = !docsEnableAutoGenTag

	if docsHugo {
		// Generate Hugo-compatible markdown with front matter
		prepender := func(filename string) string {
			name := filepath.Base(filename)
			name = strings.TrimSuffix(name, filepath.Ext(name))
			title := strings.ReplaceAll(name, "_", " ")
			return fmt.Sprintf(`---
title: "%s"
---

`, title)
		}
		linkHandler := func(name string) string {
			return name
		}

		if err := doc.GenMarkdownTreeCustom(rootCmd, docsOutputDir, prepender, linkHandler); err != nil {
			return fmt.Errorf("generate markdown: %w", err)
		}
		fmt.Print("(Hugo front matter enabled)\n")
	} else {
		// Generate plain markdown
		if err := doc.GenMarkdownTree(rootCmd, docsOutputDir); err != nil {
			return fmt.Errorf("generate markdown: %w", err)
		}
	}

	count := countFiles(docsOutputDir, ".md")
	fmt.Printf("Generated %d markdown files in %s\n", count, docsOutputDir)
	return nil
}

func runDocsMan(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(docsOutputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	rootCmd.DisableAutoGenTag = !docsEnableAutoGenTag

	header := &doc.GenManHeader{
		Title:   "THINKT",
		Section: "1",
	}
	if err := doc.GenManTree(rootCmd, header, docsOutputDir); err != nil {
		return fmt.Errorf("generate man pages: %w", err)
	}

	count := countFiles(docsOutputDir, ".1")
	fmt.Printf("Generated %d man pages in %s\n", count, docsOutputDir)
	return nil
}

func countFiles(dir, ext string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var count int
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ext {
			count++
		}
	}
	return count
}
