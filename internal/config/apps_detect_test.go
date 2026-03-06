package config

import (
	"testing"
)

func TestDetectTerminalFrom(t *testing.T) {
	apps := []AppConfig{
		{ID: "terminal", Name: "Terminal", ExecRun: []string{"osascript"}, Enabled: true},
		{ID: "ghostty", Name: "Ghostty", ExecRun: []string{"open"}, Enabled: true},
		{ID: "iterm", Name: "iTerm", ExecRun: []string{"osascript"}, Enabled: true},
		{ID: "kitty", Name: "Kitty", ExecRun: []string{"open"}, Enabled: true},
		{ID: "wezterm", Name: "WezTerm", ExecRun: []string{"open"}, Enabled: true},
		{ID: "vscode", Name: "VS Code", Enabled: true}, // no ExecRun — not a terminal
	}

	tests := []struct {
		name        string
		termProgram string
		term        string
		want        string
	}{
		{"TERM_PROGRAM ghostty", "Ghostty", "", "ghostty"},
		{"TERM_PROGRAM iTerm", "iTerm.app", "", "iterm"},
		{"TERM_PROGRAM Apple_Terminal", "Apple_Terminal", "", "terminal"},
		{"TERM_PROGRAM WezTerm", "WezTerm", "", "wezterm"},
		{"TERM xterm-ghostty", "", "xterm-ghostty", "ghostty"},
		{"TERM xterm-kitty", "", "xterm-kitty", "kitty"},
		{"TERM xterm-256color no match", "", "xterm-256color", ""},
		{"both set prefers TERM_PROGRAM", "Ghostty", "xterm-kitty", "ghostty"},
		{"no env vars", "", "", ""},
		{"unknown TERM_PROGRAM", "SomeUnknown", "", ""},
		{"disabled app not matched", "Ghostty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testApps := apps
			if tt.name == "disabled app not matched" {
				// Copy apps but disable ghostty
				testApps = make([]AppConfig, len(apps))
				copy(testApps, apps)
				for i := range testApps {
					if testApps[i].ID == "ghostty" {
						testApps[i].Enabled = false
					}
				}
			}
			got := DetectTerminalFrom(testApps, tt.termProgram, tt.term)
			if got != tt.want {
				t.Errorf("DetectTerminalFrom(%q, %q) = %q, want %q", tt.termProgram, tt.term, got, tt.want)
			}
		})
	}
}
