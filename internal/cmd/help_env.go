package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

var envJSON bool

type envVar struct {
	Name    string `json:"name"`
	Default string `json:"default,omitempty"`
	Desc    string `json:"description"`
}

type envGroup struct {
	Title string
	Vars  []envVar
}

var envGroups = []envGroup{
	{"Core", []envVar{
		{"THINKT_HOME", "~/.thinkt", "Base directory for thinkt config and data"},
		{"THINKT_LOG_FILE", "", "Write debug log to this file path"},
		{"THINKT_PROFILE", "", "Write CPU profile to this file path"},
		{"THINKT_LANG", "", "Override display language (e.g. en, ja, zh)"},
	}},
	{"Share", []envVar{
		{"THINKT_SHARE_URL", "https://share.wethinkt.com", "Share API endpoint URL"},
	}},
	{"Server & API", []envVar{
		{"THINKT_API_TOKEN", "", "Bearer token for HTTP API authentication"},
		{"THINKT_CORS_ORIGIN", "", "CORS Access-Control-Allow-Origin header value"},
	}},
	{"MCP", []envVar{
		{"THINKT_MCP_TOKEN", "", "Bearer token for MCP server authentication"},
		{"THINKT_MCP_ALLOW_TOOLS", "", "Comma-separated list of allowed MCP tools"},
		{"THINKT_MCP_DENY_TOOLS", "", "Comma-separated list of denied MCP tools"},
	}},
	{"Collector / Exporter", []envVar{
		{"THINKT_COLLECTOR_URL", "", "Collector endpoint URL for trace export"},
		{"THINKT_API_KEY", "", "Bearer token for collector authentication"},
	}},
	{"Source Directories", []envVar{
		{"THINKT_CLAUDE_HOME", "~/.claude", "Override Claude session directory"},
		{"THINKT_KIMI_HOME", "~/.kimi", "Override Kimi session directory"},
		{"THINKT_GEMINI_HOME", "~/.gemini", "Override Gemini session directory"},
		{"THINKT_CODEX_HOME", "", "Override Codex session directory"},
		{"THINKT_COPILOT_HOME", "", "Override Copilot session directory"},
		{"THINKT_QWEN_HOME", "", "Override Qwen session directory"},
	}},
}

func allEnvVars() []envVar {
	var all []envVar
	for _, g := range envGroups {
		all = append(all, g.Vars...)
	}
	return all
}

type envVarJSON struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

var helpEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Show environment variables used by thinkt",
	Long:  "List all THINKT_* environment variables, their defaults, and current values.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if envJSON {
			var out []envVarJSON
			for _, e := range allEnvVars() {
				out = append(out, envVarJSON{
					Name:        e.Name,
					Value:       os.Getenv(e.Name),
					Default:     e.Default,
					Description: e.Desc,
				})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		themed := isTTY()
		var nameStyle, valueStyle, defaultStyle, descStyle, headerStyle, mutedStyle lipgloss.Style
		if themed {
			t := theme.Current()
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true)
			valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.GetAccent()))
			defaultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
			descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextSecondary.Fg))
			headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextPrimary.Fg)).Bold(true).Underline(true)
			mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.TextMuted.Fg))
		}

		// Find max name width for alignment.
		nameWidth := 0
		for _, g := range envGroups {
			for _, e := range g.Vars {
				if len(e.Name) > nameWidth {
					nameWidth = len(e.Name)
				}
			}
		}

		render := func(s string, style lipgloss.Style) string {
			if themed {
				return style.Render(s)
			}
			return s
		}

		for i, g := range envGroups {
			if i > 0 {
				fmt.Println()
			}
			fmt.Println(render(g.Title, headerStyle))

			for _, e := range g.Vars {
				val := os.Getenv(e.Name)
				name := fmt.Sprintf("  %-*s", nameWidth, e.Name)

				if val != "" {
					fmt.Printf("%s  %s  %s\n",
						render(name, nameStyle),
						render(val, valueStyle),
						render(e.Desc, descStyle))
				} else {
					def := "(unset)"
					if e.Default != "" {
						def = e.Default
					}
					fmt.Printf("%s  %s  %s\n",
						render(name, mutedStyle),
						render(def, defaultStyle),
						render(e.Desc, descStyle))
				}
			}
		}
		return nil
	},
}
