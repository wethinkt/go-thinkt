// Command gen-themes downloads iTerm2 color schemes and converts them to thinkt theme JSON.
//
// Usage:
//
//	go run cmd/gen-themes/main.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/tui/theme"
)

// scheme defines an iTerm2 color scheme to import.
type scheme struct {
	itermName string // filename in the iTerm2-Color-Schemes repo (without .itermcolors)
	thinktName string // kebab-case name for the thinkt theme
}

var curated = []scheme{
	{"Dracula", "dracula"},
	{"Nord", "nord"},
	{"Gruvbox Dark", "gruvbox-dark"},
	{"Gruvbox Light", "gruvbox-light"},
	{"Catppuccin Mocha", "catppuccin-mocha"},
	{"Catppuccin Latte", "catppuccin-latte"},
	{"Solarized Dark Patched", "solarized-dark"},
	{"iTerm2 Solarized Light", "solarized-light"},
	{"Rose Pine", "rose-pine"},
	{"TokyoNight", "tokyo-night"},
	{"Atom One Dark", "one-dark"},
	{"Monokai Soda", "monokai"},
}

const baseURL = "https://raw.githubusercontent.com/mbadolato/iTerm2-Color-Schemes/master/schemes/"

func main() {
	outDir := filepath.Join("internal", "tui", "theme", "themes")

	for _, s := range curated {
		fmt.Printf("%-20s ", s.thinktName)

		url := baseURL + strings.ReplaceAll(s.itermName, " ", "%20") + ".itermcolors"

		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("FETCH ERROR: %v\n", err)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			fmt.Printf("HTTP %d\n", resp.StatusCode)
			continue
		}

		t, err := theme.ImportIterm(resp.Body, s.thinktName)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("PARSE ERROR: %v\n", err)
			continue
		}

		data, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			fmt.Printf("JSON ERROR: %v\n", err)
			continue
		}

		path := filepath.Join(outDir, s.thinktName+".json")
		if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
			fmt.Printf("WRITE ERROR: %v\n", err)
			continue
		}

		fmt.Printf("OK\n")
	}
}
