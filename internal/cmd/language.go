package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/tui"
	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

var languageCmd = &cobra.Command{
	Use:   "language",
	Short: "Get or set the display language",
	Long: `Get or set the display language.

Running without a subcommand shows the current language.

Examples:
  thinkt language              # show current language
  thinkt language get --json   # JSON output
  thinkt language list         # list available languages
  thinkt language set zh-Hans  # set directly
  thinkt language set          # interactive picker`,
	RunE: runLanguageGet,
}

var languageGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show current display language",
	Args:  cobra.NoArgs,
	RunE:  runLanguageGet,
}

var languageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available languages",
	Args:  cobra.NoArgs,
	RunE:  runLanguageList,
}

var languageSetCmd = &cobra.Command{
	Use:   "set [lang]",
	Short: "Set the display language",
	Long: `Set the display language. Use a BCP 47 tag (e.g., en, zh-Hans).

Without an argument, launches an interactive picker (requires a terminal).

Examples:
  thinkt language set zh-Hans  # set to Chinese (Simplified)
  thinkt language set          # interactive picker`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLanguageSet,
}

func runLanguageGet(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	activeTag := thinktI18n.ResolveLocale(cfg.Language)
	terminalLang := thinktI18n.ResolveLocale("") // env-only resolution

	// Look up display names for the active language.
	langs := thinktI18n.AvailableLanguages(activeTag)
	var activeName, activeEnglish string
	for _, l := range langs {
		if l.Tag == activeTag {
			activeName = l.Name
			activeEnglish = l.EnglishName
			break
		}
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"current":      activeTag,
			"name":         activeName,
			"english_name": activeEnglish,
			"terminal":     terminalLang,
		})
	}

	t := theme.Current()
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true)
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))

	currentLabel := thinktI18n.T("cmd.language.current", "Current:")
	terminalLabel := thinktI18n.T("cmd.language.terminal", "Terminal:")

	if !isTTY() {
		display := activeTag
		if activeName != "" {
			display += " (" + activeName + ")"
		}
		fmt.Printf("%s  %s\n", currentLabel, display)
		fmt.Printf("%s %s\n", terminalLabel, terminalLang)
		return nil
	}

	display := tagStyle.Render(activeTag)
	if activeName != "" {
		display += " " + valueStyle.Render(activeName)
		if activeEnglish != "" && activeEnglish != activeName {
			display += " " + labelStyle.Render("("+activeEnglish+")")
		}
	}
	fmt.Printf("%s  %s\n", labelStyle.Render(currentLabel), display)
	fmt.Printf("%s %s\n", labelStyle.Render(terminalLabel), tagStyle.Render(terminalLang))
	return nil
}

func runLanguageList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	activeTag := thinktI18n.ResolveLocale(cfg.Language)
	langs := thinktI18n.AvailableLanguages(activeTag)

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(langs)
	}

	// Compute column widths using display width (handles wide chars).
	const gap = 2
	hActive := thinktI18n.T("cmd.language.header.active", "ACTIVE")
	hTag := thinktI18n.T("cmd.language.header.tag", "TAG")
	hName := thinktI18n.T("cmd.language.header.name", "NAME")
	hEnglish := thinktI18n.T("cmd.language.header.englishName", "ENGLISH NAME")
	colActive := lipgloss.Width(hActive)
	colTag := lipgloss.Width(hTag)
	colName := lipgloss.Width(hName)
	for _, l := range langs {
		if w := lipgloss.Width(l.Tag); w > colTag {
			colTag = w
		}
		if w := lipgloss.Width(l.Name); w > colName {
			colName = w
		}
	}

	pad := func(s string, width int) string {
		sw := lipgloss.Width(s)
		if sw >= width {
			return s
		}
		return s + strings.Repeat(" ", width-sw)
	}

	if !isTTY() {
		fmt.Println(pad(hActive, colActive+gap) + pad(hTag, colTag+gap) + pad(hName, colName+gap) + hEnglish)
		for _, l := range langs {
			active := ""
			if l.Active {
				active = "*"
			}
			fmt.Println(pad(active, colActive+gap) + pad(l.Tag, colTag+gap) + pad(l.Name, colName+gap) + l.EnglishName)
		}
		return nil
	}

	t := theme.Current()
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true)
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))

	// Pad first, then apply style — ANSI escapes don't affect column alignment this way.
	fmt.Println(headerStyle.Render(pad(hActive, colActive+gap) + pad(hTag, colTag+gap) + pad(hName, colName+gap) + hEnglish))
	for _, l := range langs {
		active := pad("", colActive+gap)
		if l.Active {
			active = activeStyle.Render(pad("*", colActive+gap))
		}

		row := active +
			tagStyle.Render(pad(l.Tag, colTag+gap)) +
			nameStyle.Render(pad(l.Name, colName+gap)) +
			mutedStyle.Render(l.EnglishName)
		fmt.Println(row)
	}
	return nil
}

func runLanguageSet(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var tag string
	if len(args) == 1 {
		tag = args[0]
	} else {
		// Interactive picker
		if !isTTY() {
			return fmt.Errorf("interactive language picker requires a terminal; use 'thinkt language set <lang>'")
		}
		activeTag := thinktI18n.ResolveLocale(cfg.Language)
		selected, err := tui.RunLanguagePicker(activeTag)
		if err != nil {
			return err
		}
		if selected == "" {
			return nil // cancelled
		}
		tag = selected
	}

	// Validate the tag is available
	activeTag := thinktI18n.ResolveLocale(cfg.Language)
	langs := thinktI18n.AvailableLanguages(activeTag)
	found := false
	for _, l := range langs {
		if l.Tag == tag {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("language %q not available; use 'thinkt language list' to see options", tag)
	}

	cfg.Language = tag
	if err := config.Save(cfg); err != nil {
		return err
	}

	if isTTY() {
		t := theme.Current()
		accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent())).Bold(true)
		fmt.Println(thinktI18n.Tf("cmd.language.setSuccess", "Language set to: %s", accentStyle.Render(tag)))
	} else {
		fmt.Println(thinktI18n.Tf("cmd.language.setSuccess", "Language set to: %s", tag))
	}
	return nil
}
