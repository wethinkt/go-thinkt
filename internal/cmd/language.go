package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

var languageCmd = &cobra.Command{
	Use:   "language [lang]",
	Short: "Get or set the display language",
	Long: `Get or set the display language. Use a BCP 47 tag (e.g., en, zh-Hans, ja).

Examples:
  thinkt language          # show current language
  thinkt language zh-Hans  # set to Chinese (Simplified)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			lang := thinktI18n.ResolveLocale(cfg.Language)
			fmt.Printf("Current language: %s\n", lang)
			return nil
		}

		cfg.Language = args[0]
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Language set to: %s\n", args[0])
		return nil
	},
}
