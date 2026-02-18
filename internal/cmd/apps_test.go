package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/config"
)

// testEnv holds the dynamic IDs discovered from the platform's DefaultApps.
type testEnv struct {
	home           string
	apps           []config.AppConfig
	disabledID     string // an app we disabled for testing enable
	enabledID      string // an app that is enabled (for testing disable)
	terminalID     string // an enabled app with ExecRun
	nonTerminalID  string // an enabled app without ExecRun
}

// setupTestHome creates a temp dir, sets HOME, writes a config using platform
// defaults with one app disabled, and returns the test environment.
func setupTestHome(t *testing.T) testEnv {
	t.Helper()

	defaults := config.DefaultApps()
	if len(defaults) < 2 {
		t.Skip("need at least 2 default apps on this platform")
	}

	// Find a terminal app (has ExecRun) and a non-terminal app
	var terminalID, nonTerminalID string
	for _, app := range defaults {
		if len(app.ExecRun) > 0 && terminalID == "" {
			terminalID = app.ID
		}
		if len(app.ExecRun) == 0 && nonTerminalID == "" {
			nonTerminalID = app.ID
		}
	}
	if terminalID == "" || nonTerminalID == "" {
		t.Skip("need both terminal and non-terminal apps on this platform")
	}

	// Copy defaults and disable the last app
	apps := make([]config.AppConfig, len(defaults))
	copy(apps, defaults)
	disabledID := apps[len(apps)-1].ID
	apps[len(apps)-1].Enabled = false

	// Pick an enabled app (first one)
	enabledID := apps[0].ID

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := config.Config{
		Theme:       "dark",
		AllowedApps: apps,
	}
	writeTestConfig(t, tmpDir, cfg)

	return testEnv{
		home:          tmpDir,
		apps:          apps,
		disabledID:    disabledID,
		enabledID:     enabledID,
		terminalID:    terminalID,
		nonTerminalID: nonTerminalID,
	}
}

func writeTestConfig(t *testing.T, home string, cfg config.Config) {
	t.Helper()
	configDir := filepath.Join(home, ".thinkt")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe, returns the output.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fnErr := fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return buf.String(), fnErr
}

// withJSON sets the global outputJSON flag for the duration of the test.
func withJSON(t *testing.T) {
	t.Helper()
	old := outputJSON
	outputJSON = true
	t.Cleanup(func() { outputJSON = old })
}

// --- List tests ---

func TestAppsList_Table(t *testing.T) {
	env := setupTestHome(t)

	out, err := captureStdout(t, func() error {
		return runAppsList(appsListCmd, nil)
	})
	if err != nil {
		t.Fatalf("runAppsList: %v", err)
	}

	// Check header
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") || !strings.Contains(out, "TERMINAL") {
		t.Errorf("expected table headers, got:\n%s", out)
	}
	// Check that our known apps appear
	if !strings.Contains(out, env.terminalID) {
		t.Errorf("expected %s in output", env.terminalID)
	}
	if !strings.Contains(out, env.nonTerminalID) {
		t.Errorf("expected %s in output", env.nonTerminalID)
	}
}

func TestAppsList_JSON(t *testing.T) {
	env := setupTestHome(t)
	withJSON(t)

	out, err := captureStdout(t, func() error {
		return runAppsList(appsListCmd, nil)
	})
	if err != nil {
		t.Fatalf("runAppsList: %v", err)
	}

	var apps []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Enabled  bool   `json:"enabled"`
		Terminal bool   `json:"terminal"`
	}
	if err := json.Unmarshal([]byte(out), &apps); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if len(apps) != len(env.apps) {
		t.Fatalf("expected %d apps, got %d", len(env.apps), len(apps))
	}

	// Check terminal field for known apps
	for _, app := range apps {
		if app.ID == env.terminalID && !app.Terminal {
			t.Errorf("app %q should have terminal=true", app.ID)
		}
		if app.ID == env.nonTerminalID && app.Terminal {
			t.Errorf("app %q should have terminal=false", app.ID)
		}
	}

	// Check disabled app
	for _, app := range apps {
		if app.ID == env.disabledID && app.Enabled {
			t.Errorf("app %q should be disabled", app.ID)
		}
	}
}

func TestAppsList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// validateApps returns defaults when given empty list, so we need to
	// test with the actual Load behavior. An empty AllowedApps will be
	// replaced by defaults via validateApps. Instead, test the function
	// directly with a config that has no apps.
	// We can't prevent validateApps from running on Load(), but we can
	// call runAppsList with a config that truly has no apps by writing
	// a config where all apps are present but the list is explicitly
	// tested through the text output path.
	//
	// Skip this: validateApps always returns defaults for empty input.
	t.Skip("validateApps replaces empty AllowedApps with defaults")
}

// --- Enable / Disable tests ---

func TestSetAppEnabled_Enable(t *testing.T) {
	env := setupTestHome(t)

	out, err := captureStdout(t, func() error {
		return setAppEnabled(env.disabledID, true)
	})
	if err != nil {
		t.Fatalf("setAppEnabled: %v", err)
	}
	if !strings.Contains(out, "enabled") {
		t.Errorf("expected 'enabled' in output, got: %s", out)
	}

	// Verify saved config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, app := range cfg.AllowedApps {
		if app.ID == env.disabledID && !app.Enabled {
			t.Errorf("%s should be enabled after setAppEnabled(true)", env.disabledID)
		}
	}
}

func TestSetAppEnabled_Disable(t *testing.T) {
	env := setupTestHome(t)

	out, err := captureStdout(t, func() error {
		return setAppEnabled(env.enabledID, false)
	})
	if err != nil {
		t.Fatalf("setAppEnabled: %v", err)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected 'disabled' in output, got: %s", out)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, app := range cfg.AllowedApps {
		if app.ID == env.enabledID && app.Enabled {
			t.Errorf("%s should be disabled", env.enabledID)
		}
	}
}

func TestSetAppEnabled_UnknownApp(t *testing.T) {
	setupTestHome(t)

	_, err := captureStdout(t, func() error {
		return setAppEnabled("nonexistent", true)
	})
	if err == nil {
		t.Fatal("expected error for unknown app")
	}
	if !strings.Contains(err.Error(), "unknown app") {
		t.Errorf("expected 'unknown app' error, got: %v", err)
	}
}

func TestSetAppEnabled_JSON_Success(t *testing.T) {
	env := setupTestHome(t)
	withJSON(t)

	out, err := captureStdout(t, func() error {
		return setAppEnabled(env.disabledID, true)
	})
	if err != nil {
		t.Fatalf("setAppEnabled: %v", err)
	}

	var result map[string]bool
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if !result["enabled"] {
		t.Error("expected enabled=true in JSON")
	}
}

func TestSetAppEnabled_JSON_Error(t *testing.T) {
	setupTestHome(t)
	withJSON(t)

	out, err := captureStdout(t, func() error {
		return setAppEnabled("nonexistent", true)
	})
	// jsonError returns nil error
	if err != nil {
		t.Fatalf("expected nil error from jsonError, got: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if result["error"] == "" {
		t.Error("expected error field in JSON")
	}
}

// --- Get Terminal tests ---

func TestGetTerminal_Default(t *testing.T) {
	setupTestHome(t)

	out, err := captureStdout(t, func() error {
		return runAppsGetTerminal(appsGetTermCmd, nil)
	})
	if err != nil {
		t.Fatalf("runAppsGetTerminal: %v", err)
	}
	if strings.TrimSpace(out) != "terminal" {
		t.Errorf("expected 'terminal', got %q", strings.TrimSpace(out))
	}
}

func TestGetTerminal_Configured(t *testing.T) {
	env := setupTestHome(t)

	// Rewrite config with terminal set
	writeTestConfig(t, env.home, config.Config{
		Theme:       "dark",
		Terminal:    env.terminalID,
		AllowedApps: env.apps,
	})

	out, err := captureStdout(t, func() error {
		return runAppsGetTerminal(appsGetTermCmd, nil)
	})
	if err != nil {
		t.Fatalf("runAppsGetTerminal: %v", err)
	}
	if strings.TrimSpace(out) != env.terminalID {
		t.Errorf("expected %q, got %q", env.terminalID, strings.TrimSpace(out))
	}
}

func TestGetTerminal_JSON(t *testing.T) {
	setupTestHome(t)
	withJSON(t)

	out, err := captureStdout(t, func() error {
		return runAppsGetTerminal(appsGetTermCmd, nil)
	})
	if err != nil {
		t.Fatalf("runAppsGetTerminal: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["terminal"] != "terminal" {
		t.Errorf("expected terminal='terminal', got %q", result["terminal"])
	}
}

// --- Set Terminal tests ---

func TestSetTerminal_Valid(t *testing.T) {
	env := setupTestHome(t)

	out, err := captureStdout(t, func() error {
		return runAppsSetTerminal(appsSetTermCmd, []string{env.terminalID})
	})
	if err != nil {
		t.Fatalf("runAppsSetTerminal: %v", err)
	}
	if !strings.Contains(out, env.terminalID) {
		t.Errorf("expected %q in output, got: %s", env.terminalID, out)
	}

	// Verify saved
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Terminal != env.terminalID {
		t.Errorf("expected Terminal=%q, got %q", env.terminalID, cfg.Terminal)
	}
}

func TestSetTerminal_NonTerminalApp(t *testing.T) {
	env := setupTestHome(t)

	_, err := captureStdout(t, func() error {
		return runAppsSetTerminal(appsSetTermCmd, []string{env.nonTerminalID})
	})
	if err == nil {
		t.Fatal("expected error for non-terminal app")
	}
	if !strings.Contains(err.Error(), "not a terminal") {
		t.Errorf("expected 'not a terminal' error, got: %v", err)
	}
}

func TestSetTerminal_DisabledApp(t *testing.T) {
	env := setupTestHome(t)

	// The disabledID might or might not be a terminal. We need a disabled
	// terminal app specifically. Disable our known terminal app.
	for i := range env.apps {
		if env.apps[i].ID == env.terminalID {
			env.apps[i].Enabled = false
		}
	}
	writeTestConfig(t, env.home, config.Config{Theme: "dark", AllowedApps: env.apps})

	_, err := captureStdout(t, func() error {
		return runAppsSetTerminal(appsSetTermCmd, []string{env.terminalID})
	})
	if err == nil {
		t.Fatal("expected error for disabled app")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected 'disabled' error, got: %v", err)
	}
}

func TestSetTerminal_UnknownApp(t *testing.T) {
	setupTestHome(t)

	_, err := captureStdout(t, func() error {
		return runAppsSetTerminal(appsSetTermCmd, []string{"nonexistent"})
	})
	if err == nil {
		t.Fatal("expected error for unknown app")
	}
	if !strings.Contains(err.Error(), "unknown app") {
		t.Errorf("expected 'unknown app' error, got: %v", err)
	}
}

// --- terminalApps helper tests ---

func TestTerminalApps(t *testing.T) {
	env := setupTestHome(t)
	cfg := config.Config{AllowedApps: env.apps}

	terminals := terminalApps(cfg)

	// All returned should have ExecRun and be enabled
	for _, app := range terminals {
		if len(app.ExecRun) == 0 {
			t.Errorf("terminalApps returned %q without ExecRun", app.ID)
		}
		if !app.Enabled {
			t.Errorf("terminalApps returned disabled app %q", app.ID)
		}
	}

	// Our known terminal should be in the list
	found := false
	for _, app := range terminals {
		if app.ID == env.terminalID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %q in terminal apps", env.terminalID)
	}
}

func TestTerminalApps_ExcludesDisabled(t *testing.T) {
	apps := []config.AppConfig{
		{ID: "enabled-term", ExecRun: []string{"echo"}, Enabled: true},
		{ID: "disabled-term", ExecRun: []string{"echo"}, Enabled: false},
	}
	cfg := config.Config{AllowedApps: apps}

	terminals := terminalApps(cfg)
	if len(terminals) != 1 {
		t.Fatalf("expected 1 terminal, got %d", len(terminals))
	}
	if terminals[0].ID != "enabled-term" {
		t.Errorf("expected enabled-term, got %s", terminals[0].ID)
	}
}

// --- Enable/Disable via cobra RunE ---

func TestRunAppsEnable_WithArg(t *testing.T) {
	env := setupTestHome(t)

	out, err := captureStdout(t, func() error {
		return runAppsEnable(appsEnableCmd, []string{env.disabledID})
	})
	if err != nil {
		t.Fatalf("runAppsEnable: %v", err)
	}
	if !strings.Contains(out, "enabled") {
		t.Errorf("expected 'enabled' in output, got: %s", out)
	}
}

func TestRunAppsDisable_WithArg(t *testing.T) {
	env := setupTestHome(t)

	out, err := captureStdout(t, func() error {
		return runAppsDisable(appsDisableCmd, []string{env.enabledID})
	})
	if err != nil {
		t.Fatalf("runAppsDisable: %v", err)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected 'disabled' in output, got: %s", out)
	}
}

// --- Round-trip tests ---

func TestEnableDisableRoundTrip(t *testing.T) {
	env := setupTestHome(t)

	// Disable an enabled app
	if _, err := captureStdout(t, func() error {
		return setAppEnabled(env.enabledID, false)
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	// Verify disabled
	cfg, _ := config.Load()
	for _, app := range cfg.AllowedApps {
		if app.ID == env.enabledID && app.Enabled {
			t.Fatalf("%s should be disabled", env.enabledID)
		}
	}

	// Re-enable
	if _, err := captureStdout(t, func() error {
		return setAppEnabled(env.enabledID, true)
	}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// Verify enabled
	cfg, _ = config.Load()
	for _, app := range cfg.AllowedApps {
		if app.ID == env.enabledID && !app.Enabled {
			t.Fatalf("%s should be enabled", env.enabledID)
		}
	}
}

func TestSetTerminalRoundTrip(t *testing.T) {
	env := setupTestHome(t)

	// Find a second terminal app if possible
	var secondTerminal string
	for _, app := range env.apps {
		if len(app.ExecRun) > 0 && app.Enabled && app.ID != env.terminalID {
			secondTerminal = app.ID
			break
		}
	}

	// Set to our terminal
	if _, err := captureStdout(t, func() error {
		return runAppsSetTerminal(appsSetTermCmd, []string{env.terminalID})
	}); err != nil {
		t.Fatalf("set %s: %v", env.terminalID, err)
	}

	// Verify via get
	out, err := captureStdout(t, func() error {
		return runAppsGetTerminal(appsGetTermCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != env.terminalID {
		t.Errorf("expected %q, got %q", env.terminalID, strings.TrimSpace(out))
	}

	// If we have a second terminal, change to it and verify
	if secondTerminal != "" {
		if _, err := captureStdout(t, func() error {
			return runAppsSetTerminal(appsSetTermCmd, []string{secondTerminal})
		}); err != nil {
			t.Fatalf("set %s: %v", secondTerminal, err)
		}

		out, err = captureStdout(t, func() error {
			return runAppsGetTerminal(appsGetTermCmd, nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out) != secondTerminal {
			t.Errorf("expected %q, got %q", secondTerminal, strings.TrimSpace(out))
		}
	}
}
