package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/cli"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// Theme command
var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Browse and manage TUI themes",
	Long: `Browse and manage TUI themes.

Running without a subcommand launches the interactive theme browser.

The theme controls colors for conversation blocks, labels, borders,
and other UI elements. Themes are stored in ~/.thinkt/themes/.

Examples:
  thinkt theme               # Browse themes interactively
  thinkt theme show          # Show current theme with samples
  thinkt theme show --json   # Output theme as JSON
  thinkt theme list          # List all available themes
  thinkt theme set dracula   # Switch to a theme
  thinkt theme builder       # Interactive theme builder
  thinkt theme import f.itermcolors  # Import iTerm2 color scheme`,
	RunE: runThemeBrowse,
}

var themeShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Display a theme with styled samples",
	Long: `Display a theme with styled samples and a conversation preview.

If no name is provided, shows the active theme.

Examples:
  thinkt theme show            # Show active theme
  thinkt theme show dracula    # Show the dracula theme
  thinkt theme show --json     # Output active theme as JSON`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runThemeShow,
	SilenceUsage: true,
}

var themeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available themes",
	Long:  `List all built-in and user themes. The active theme is marked with *.`,
	Args:  cobra.NoArgs,
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

// runThemeShow displays a theme by name, or the active theme if no name given.
func runThemeShow(cmd *cobra.Command, args []string) error {
	var t theme.Theme
	var err error

	if len(args) > 0 {
		t, err = theme.LoadByName(args[0])
		if err != nil {
			return fmt.Errorf("theme %q not found", args[0])
		}
	} else {
		t, err = theme.Load()
		if err != nil {
			t = theme.DefaultTheme()
		}
	}

	display := cli.NewThemeDisplay(os.Stdout, t)

	if outputJSON {
		return display.ShowJSON()
	}

	return display.Show()
}

// runThemeList lists all available themes.
func runThemeList(cmd *cobra.Command, args []string) error {
	if outputJSON {
		return cli.ListThemesJSON(os.Stdout)
	}
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

var themeBrowseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Browse and preview themes interactively",
	Long: `Browse available themes with a live preview.

Navigate themes with arrow keys or j/k, see a live preview of each theme,
and activate one with Enter.

Key bindings:
  ↑/↓ or j/k   Navigate themes
  enter         Activate the highlighted theme
  e             Edit the highlighted theme in the builder
  n             Create a new theme in the builder
  q/esc         Quit without changing`,
	Args: cobra.NoArgs,
	RunE: runThemeBrowse,
}

func runThemeBrowse(cmd *cobra.Command, args []string) error {
	if !isTTY() {
		return fmt.Errorf("interactive theme browser requires a terminal; use 'thinkt theme list' or 'thinkt theme show'")
	}
	return tui.RunThemeBrowser()
}

var themeImportName string

var themeImportCmd = &cobra.Command{
	Use:   "import <file.itermcolors>",
	Short: "Import an iTerm2 color scheme as a theme",
	Long: `Import an iTerm2 .itermcolors file and convert it to a thinkt theme.

The imported theme is saved to ~/.thinkt/themes/ and can be activated
with 'thinkt theme set'.

Examples:
  thinkt theme import ~/Downloads/Dracula.itermcolors
  thinkt theme import scheme.itermcolors --name my-theme`,
	Args: cobra.ExactArgs(1),
	RunE: runThemeImport,
}

func runThemeImport(cmd *cobra.Command, args []string) error {
	path := args[0]

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	name := themeImportName
	if name == "" {
		// Derive name from filename
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
		name = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	}

	t, err := theme.ImportIterm(f, name)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	if err := theme.Save(name, t); err != nil {
		return fmt.Errorf("save theme: %w", err)
	}

	fmt.Printf("Theme %q imported successfully.\n", name)
	fmt.Printf("Activate it with: thinkt theme set %s\n", name)
	return nil
}

func runThemeBuilder(cmd *cobra.Command, args []string) error {
	if !isTTY() {
		return fmt.Errorf("interactive theme builder requires a terminal")
	}
	name := theme.ActiveName()
	if len(args) > 0 {
		name = args[0]
	}

	return tui.RunThemeBuilder(name)
}
