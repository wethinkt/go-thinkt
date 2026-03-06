# Apps Setup Step — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an Apps configuration step to the setup wizard between Source Approval and Indexer, with app enable/disable checklist and terminal auto-detection.

**Architecture:** Two new wizard steps (`stepApps`, `stepTerminal`) inserted into the existing BubbleTea setup flow. `DetectTerminal()` reads `TERM_PROGRAM` and `TERM` env vars to pre-select the default terminal. Result struct carries app preferences through to `SaveResult`.

**Tech Stack:** Go, BubbleTea v2, lipgloss v2, existing `config.DefaultApps()` discovery

---

### Task 1: Add `DetectTerminal()` to config package

**Files:**
- Modify: `internal/config/apps.go`
- Create: `internal/config/apps_detect_test.go`

**Step 1: Write the failing test**

```go
package config

import (
	"testing"
)

func TestDetectTerminal(t *testing.T) {
	// Build a list of apps that includes ghostty and iterm
	apps := []AppConfig{
		{ID: "terminal", Name: "Terminal", ExecRun: []string{"osascript"}, Enabled: true},
		{ID: "ghostty", Name: "Ghostty", ExecRun: []string{"open"}, Enabled: true},
		{ID: "iterm", Name: "iTerm", ExecRun: []string{"osascript"}, Enabled: true},
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
		{"TERM xterm-ghostty", "", "xterm-ghostty", "ghostty"},
		{"TERM xterm-256color no match", "", "xterm-256color", ""},
		{"both set prefers TERM_PROGRAM", "Ghostty", "xterm-kitty", "ghostty"},
		{"no env vars", "", "", ""},
		{"unknown TERM_PROGRAM", "SomeUnknown", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectTerminalFrom(apps, tt.termProgram, tt.term)
			if got != tt.want {
				t.Errorf("DetectTerminalFrom(%q, %q) = %q, want %q", tt.termProgram, tt.term, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestDetectTerminal -v`
Expected: FAIL — `DetectTerminalFrom` undefined

**Step 3: Write the implementation**

Add to `internal/config/apps.go`:

```go
// DetectTerminal checks TERM_PROGRAM and TERM environment variables
// to identify the current terminal app from the provided app list.
// Returns the matching app ID, or empty string if no match.
func DetectTerminal(apps []AppConfig) string {
	return DetectTerminalFrom(apps, os.Getenv("TERM_PROGRAM"), os.Getenv("TERM"))
}

// DetectTerminalFrom identifies the current terminal from explicit env values.
// Exported for testing without modifying environment.
func DetectTerminalFrom(apps []AppConfig, termProgram, term string) string {
	// Build lookup of terminal apps (those with ExecRun)
	type termApp struct {
		id   string
		name string
	}
	var terminals []termApp
	for _, app := range apps {
		if len(app.ExecRun) > 0 && app.Enabled {
			terminals = append(terminals, termApp{id: app.id, name: app.Name})
		}
	}

	// Known TERM_PROGRAM mappings
	termProgramMap := map[string]string{
		"Apple_Terminal": "terminal",
		"iTerm.app":     "iterm",
	}

	if termProgram != "" {
		// Check explicit mapping first
		if id, ok := termProgramMap[termProgram]; ok {
			for _, t := range terminals {
				if t.id == id {
					return id
				}
			}
		}
		// Check case-insensitive match against app ID and name
		lower := strings.ToLower(termProgram)
		for _, t := range terminals {
			if strings.ToLower(t.id) == lower || strings.ToLower(t.name) == lower {
				return t.id
			}
		}
	}

	// Parse TERM for xterm-{name} pattern
	if term != "" && strings.HasPrefix(term, "xterm-") {
		suffix := strings.ToLower(strings.TrimPrefix(term, "xterm-"))
		// Skip generic suffixes
		if suffix != "256color" && suffix != "color" {
			for _, t := range terminals {
				if strings.ToLower(t.id) == suffix || strings.ToLower(t.name) == suffix {
					return t.id
				}
			}
		}
	}

	return ""
}
```

Note: `app.id` should be `app.ID` — use the struct field name. Also add `"strings"` to the import if not already present.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestDetectTerminal -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/apps.go internal/config/apps_detect_test.go
git commit -m "feat(config): add DetectTerminal for env-based terminal detection"
```

---

### Task 2: Add `Apps` and `Terminal` to setup Result and SaveResult

**Files:**
- Modify: `internal/tui/setup/setup.go` (Result struct)
- Modify: `internal/tui/setup/defaults.go` (SaveResult, RunDefaults)

**Step 1: Update Result struct**

In `internal/tui/setup/setup.go`, add to Result:

```go
type Result struct {
	Language   string
	HomeDir    string
	Sources    map[string]bool
	Apps       map[string]bool // app ID -> enabled
	Terminal   string          // default terminal app ID
	Indexer    bool
	Embeddings bool
	Completed  bool
}
```

**Step 2: Update SaveResult to write apps and terminal**

In `internal/tui/setup/defaults.go`, update `SaveResult`:

```go
func SaveResult(result Result) error {
	cfg := config.Default()
	cfg.Language = result.Language
	cfg.Indexer.Watch = result.Indexer
	cfg.Embedding.Enabled = result.Embeddings
	cfg.Terminal = result.Terminal

	now := time.Now()
	cfg.DiscoveredAt = &now

	cfg.Sources = make(map[string]config.SourceConfig, len(result.Sources))
	for name, enabled := range result.Sources {
		cfg.Sources[name] = config.SourceConfig{Enabled: enabled}
	}

	// Apply app enabled/disabled preferences from setup
	if result.Apps != nil {
		for i := range cfg.AllowedApps {
			if enabled, ok := result.Apps[cfg.AllowedApps[i].ID]; ok {
				cfg.AllowedApps[i].Enabled = enabled
			}
		}
	}

	return config.Save(cfg)
}
```

**Step 3: Update RunDefaults to auto-detect terminal**

In `internal/tui/setup/defaults.go`, update `RunDefaults`:

```go
func RunDefaults(factories []thinkt.StoreFactory) (Result, error) {
	lang := thinktI18n.ResolveLocale("")

	homeDir, err := config.Dir()
	if err != nil {
		return Result{}, err
	}

	d := thinkt.NewDiscovery(factories...)
	detailed, err := d.DiscoverDetailed(context.Background(), nil)
	if err != nil {
		return Result{}, err
	}

	sources := make(map[string]bool, len(detailed))
	for _, info := range detailed {
		sources[string(info.Source)] = true
	}

	// Auto-detect terminal from environment
	apps := config.DefaultApps()
	terminal := config.DetectTerminal(apps)
	if terminal == "" {
		terminal = "terminal"
	}

	result := Result{
		Language:   lang,
		HomeDir:    homeDir,
		Sources:    sources,
		Terminal:   terminal,
		Indexer:    true,
		Embeddings: false,
		Completed:  true,
	}

	if err := SaveResult(result); err != nil {
		return Result{}, err
	}

	return result, nil
}
```

**Step 4: Verify build**

Run: `go build ./...`
Expected: compiles without errors

**Step 5: Commit**

```bash
git add internal/tui/setup/setup.go internal/tui/setup/defaults.go
git commit -m "feat(setup): add Apps and Terminal fields to Result and SaveResult"
```

---

### Task 3: Add step constants and model fields

**Files:**
- Modify: `internal/tui/setup/setup.go`

**Step 1: Update step constants**

Replace the step iota block:

```go
const (
	stepWelcome step = iota
	stepHome
	stepSourceConsent
	stepSourceApproval
	stepApps
	stepTerminal
	stepIndexer
	stepEmbeddings
	stepSuggestions
	stepDone
)
```

**Step 2: Add model fields**

Add to Model struct:

```go
// App selection
apps         []config.AppConfig // discovered apps for checklist
appCursor    int                // cursor position in app list
termApps     []config.AppConfig // terminal apps (have ExecRun)
detectedTerm string             // auto-detected terminal app ID
termCursor   int                // cursor for terminal picker
termPicking  bool               // true when showing terminal picker (after declining detected)
```

Add import for `"github.com/wethinkt/go-thinkt/internal/config"` to setup.go.

**Step 3: Update step routing in Update()**

Add cases in the switch:

```go
case stepApps:
	return m.updateApps(msg)
case stepTerminal:
	return m.updateTerminal(msg)
```

**Step 4: Update View()**

Add cases:

```go
case stepApps:
	content = m.viewApps()
case stepTerminal:
	content = m.viewTerminal()
```

**Step 5: Update stepIndicator()**

Change total from 6 to 8:

```go
func (m Model) stepIndicator() string {
	if m.step == stepWelcome || m.step == stepSuggestions || m.step == stepDone {
		return ""
	}
	total := 8
	current := int(m.step)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(m.muted))
	return style.Render(fmt.Sprintf("[%d/%d]", current, total))
}
```

**Step 6: Update renderStickyContext()**

Add apps context after sources block, before indexer block:

```go
if len(m.apps) > 0 && m.step > stepApps {
	enabled := 0
	for _, app := range m.apps {
		if app.Enabled {
			enabled++
		}
	}
	lines = append(lines, fmt.Sprintf("  %s %s",
		labelStyle.Render("Apps:"),
		valueStyle.Render(fmt.Sprintf("%d enabled", enabled)),
	))
}

if m.result.Terminal != "" && m.step > stepTerminal {
	lines = append(lines, fmt.Sprintf("  %s %s",
		labelStyle.Render("Terminal:"),
		valueStyle.Render(m.result.Terminal),
	))
}
```

**Step 7: Update source approval → next step transition**

In `steps_sources.go`, the source approval `updateSourceApproval` transitions to `stepIndexer` on Enter. Change both occurrences:

In `selectConsentChoice` case 2 (disable all): change `m.step = stepIndexer` to `m.step = stepApps`
In `updateSourceApproval` summary Enter handler: change `m.step = stepIndexer` to `m.step = stepApps`

Both should also initialize the apps list:

```go
m.apps = config.DefaultApps()
m.appCursor = 0
m.step = stepApps
```

**Step 8: Verify build**

Run: `go build ./...`
Expected: will fail because `updateApps`, `viewApps`, `updateTerminal`, `viewTerminal` don't exist yet — that's expected, we'll add them in Task 4.

**Step 9: Commit**

```bash
git add internal/tui/setup/setup.go internal/tui/setup/steps_sources.go
git commit -m "feat(setup): add stepApps and stepTerminal constants and model wiring"
```

---

### Task 4: Implement the Apps checklist step

**Files:**
- Create: `internal/tui/setup/steps_apps.go`

**Step 1: Create the apps step file**

```go
package setup

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/wethinkt/go-thinkt/internal/config"
)

// --- stepApps: multi-select checklist of discovered apps ---

func (m Model) updateApps(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.appCursor > 0 {
				m.appCursor--
			}
			return m, nil
		case "down", "j":
			if m.appCursor < len(m.apps)-1 {
				m.appCursor++
			}
			return m, nil
		case "tab":
			m.appCursor = (m.appCursor + 1) % len(m.apps)
			return m, nil
		case " ":
			if m.appCursor < len(m.apps) {
				m.apps[m.appCursor].Enabled = !m.apps[m.appCursor].Enabled
			}
			return m, nil
		case "enter":
			// Save app preferences to result
			m.result.Apps = make(map[string]bool, len(m.apps))
			for _, app := range m.apps {
				m.result.Apps[app.ID] = app.Enabled
			}
			// Prepare terminal step
			m.termApps = nil
			for _, app := range m.apps {
				if len(app.ExecRun) > 0 && app.Enabled {
					m.termApps = append(m.termApps, app)
				}
			}
			if len(m.termApps) == 0 {
				// No terminal apps enabled — skip terminal step
				m.confirm = true
				m.step = stepIndexer
				return m, nil
			}
			m.detectedTerm = config.DetectTerminal(m.apps)
			if m.detectedTerm != "" {
				// Detected a terminal — show confirmation
				m.confirm = true
				m.step = stepTerminal
			} else {
				// No detection — show picker
				m.termPicking = true
				m.termCursor = 0
				m.step = stepTerminal
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewApps() string {
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))
	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))
	accentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accent))

	var b strings.Builder
	b.WriteString(m.renderStepHeader("Apps"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s\n\n",
		bodyStyle.Render("Select which apps thinkt can open files and run commands in.")))

	for i, app := range m.apps {
		pointer := "  "
		if i == m.appCursor {
			pointer = "▸ "
		}

		check := "[ ]"
		if app.Enabled {
			check = "[x]"
		}

		nameStyle := bodyStyle
		checkStyle := mutedStyle
		if i == m.appCursor {
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.primary))
			checkStyle = accentStyle
		}

		tag := ""
		if len(app.ExecRun) > 0 {
			tag = mutedStyle.Render(" (terminal)")
		}

		b.WriteString(fmt.Sprintf("  %s%s %s%s\n",
			pointer,
			checkStyle.Render(check),
			nameStyle.Render(app.Name),
			tag,
		))
	}

	b.WriteString(fmt.Sprintf("\n  %s\n",
		mutedStyle.Render(m.withEscQ("↑/↓: navigate · Space: toggle · Enter: continue · esc: exit"))))

	b.WriteString(fmt.Sprintf("\n\n  %s\n", m.renderCLIHint("thinkt apps enable/disable")))

	return b.String()
}

// --- stepTerminal: detect and confirm default terminal ---

func (m Model) updateTerminal(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if m.termPicking {
			// Terminal picker mode
			switch msg.String() {
			case "up", "k":
				if m.termCursor > 0 {
					m.termCursor--
				}
				return m, nil
			case "down", "j":
				if m.termCursor < len(m.termApps)-1 {
					m.termCursor++
				}
				return m, nil
			case "tab":
				m.termCursor = (m.termCursor + 1) % len(m.termApps)
				return m, nil
			case "enter":
				if m.termCursor < len(m.termApps) {
					m.result.Terminal = m.termApps[m.termCursor].ID
				}
				m.confirm = true
				m.step = stepIndexer
				return m, nil
			}
			return m, nil
		}

		// Confirmation mode (detected terminal)
		switch msg.String() {
		case "up", "down", "k", "j", "tab":
			m.confirm = !m.confirm
			return m, nil
		case "Y", "y":
			m.result.Terminal = m.detectedTerm
			m.confirm = true
			m.step = stepIndexer
			return m, nil
		case "N", "n":
			m.termPicking = true
			m.termCursor = 0
			return m, nil
		case "enter":
			if m.confirm {
				m.result.Terminal = m.detectedTerm
				m.confirm = true
				m.step = stepIndexer
			} else {
				m.termPicking = true
				m.termCursor = 0
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) viewTerminal() string {
	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.primary))
	mutedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.muted))
	accentStyle := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(m.accent))

	var b strings.Builder
	b.WriteString(m.renderStepHeader("Default Terminal"))
	b.WriteString("\n")

	if m.termPicking {
		b.WriteString(fmt.Sprintf("  %s\n\n",
			bodyStyle.Render("Select your default terminal app:")))

		for i, app := range m.termApps {
			pointer := "  "
			nameStyle := bodyStyle
			if i == m.termCursor {
				pointer = "▸ "
				nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.primary))
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", pointer, nameStyle.Render(app.Name)))
		}

		b.WriteString(fmt.Sprintf("\n  %s\n",
			mutedStyle.Render(m.withEscQ("↑/↓: navigate · Enter: select · esc: exit"))))
	} else {
		// Find detected terminal name
		termName := m.detectedTerm
		for _, app := range m.termApps {
			if app.ID == m.detectedTerm {
				termName = app.Name
				break
			}
		}

		b.WriteString(fmt.Sprintf("  %s %s%s\n\n",
			bodyStyle.Render("Detected"),
			accentStyle.Render(termName),
			bodyStyle.Render(" as your terminal. Use as default?")))

		b.WriteString(m.renderVerticalConfirm())
	}

	b.WriteString(fmt.Sprintf("\n\n  %s\n", m.renderCLIHint("thinkt apps set-terminal")))

	return b.String()
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: compiles

**Step 3: Run existing tests**

Run: `go test ./internal/tui/setup/ -v`
Expected: PASS (no regressions)

**Step 4: Commit**

```bash
git add internal/tui/setup/steps_apps.go
git commit -m "feat(setup): implement apps checklist and terminal selection steps"
```

---

### Task 5: Integration — wire everything together and test manually

**Files:**
- All previously modified files

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS

**Step 2: Build and run setup interactively**

Run: `go run . setup`

Verify:
1. Welcome → Home → Sources → **Apps checklist appears** → **Terminal detection/confirm** → Indexer → Embeddings → Suggestions
2. Apps shows discovered apps with toggles
3. Terminal detects current terminal correctly
4. Config file has `allowed_apps` and `terminal` fields after completion

**Step 3: Test non-interactive mode**

Run: `go run . setup --ok --json`

Verify: JSON output includes terminal field, config saved correctly.

**Step 4: Commit any fixes**

```bash
git add -A
git commit -m "feat(setup): apps and terminal setup steps complete"
```
