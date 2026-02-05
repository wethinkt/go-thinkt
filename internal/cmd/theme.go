package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/cli"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// Theme command
var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Display and manage TUI theme settings",
	Long: `Display the current TUI theme with styled samples.

The theme controls colors for conversation blocks, labels, borders,
and other UI elements. Themes are stored in ~/.thinkt/themes/.

Built-in themes: dark, light
User themes can be added to ~/.thinkt/themes/

Examples:
  thinkt theme               # Show current theme with samples
  thinkt theme --json        # Output theme as JSON
  thinkt theme list          # List all available themes
  thinkt theme set light     # Switch to light theme
  thinkt theme builder       # Interactive theme builder`,
	RunE: runTheme,
}

var themeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available themes",
	Long:  `List all built-in and user themes. The active theme is marked with *.`,
	RunE:  runThemeList,
}

var themeSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Set the active theme",
	Long: `Set the active theme by name.

Available built-in themes: dark, light
User themes from ~/.thinkt/themes/ are also available.

Examples:
  thinkt theme set dark
  thinkt theme set light
  thinkt theme set my-custom-theme`,
	Args: cobra.ExactArgs(1),
	RunE: runThemeSet,
}

var themeBuilderCmd = &cobra.Command{
	Use:   "builder [name]",
	Short: "Launch interactive theme builder",
	Long: `Launch an interactive TUI for building and editing themes.

The theme builder shows a live preview of conversation styles and
allows editing colors for all theme elements interactively.

If no name is provided, edits a copy of the current theme.
If the theme doesn't exist, creates a new one based on the default.

Examples:
  thinkt theme builder             # Edit current theme
  thinkt theme builder my-theme    # Edit or create my-theme
  thinkt theme builder dark        # Edit the dark theme`,
	Args: cobra.MaximumNArgs(1),
	RunE: runThemeBuilder,
}

// runTheme displays the current theme.
func runTheme(cmd *cobra.Command, args []string) error {
	t, err := theme.Load()
	if err != nil {
		// Fall back to defaults on error
		t = theme.DefaultTheme()
	}

	display := cli.NewThemeDisplay(os.Stdout, t)

	if outputJSON {
		return display.ShowJSON()
	}

	return display.Show()
}

// runThemeList lists all available themes.
func runThemeList(cmd *cobra.Command, args []string) error {
	return cli.ListThemes(os.Stdout)
}

// runThemeSet sets the active theme.
func runThemeSet(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := theme.SetActive(name); err != nil {
		return fmt.Errorf("failed to set theme: %w", err)
	}

	fmt.Printf("Theme set to: %s\n", name)
	return nil
}

func runThemeBuilder(cmd *cobra.Command, args []string) error {
	name := theme.ActiveName()
	if len(args) > 0 {
		name = args[0]
	}

	return tui.RunThemeBuilder(name)
}
