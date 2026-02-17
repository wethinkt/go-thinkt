package config

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestDefaultApps(t *testing.T) {
	apps := DefaultApps()

	// DefaultApps only returns apps that are available on this system
	// so we can't assert specific apps are present, but we can verify
	// that all returned apps are enabled and well-formed.
	for _, app := range apps {
		if !app.Enabled {
			t.Errorf("app %q should be enabled (only available apps should be returned)", app.ID)
		}
		if app.ID == "" {
			t.Error("app has empty ID")
		}
		if app.Name == "" {
			t.Error("app has empty Name")
		}
		if len(app.Exec) == 0 {
			t.Errorf("app %q has empty Exec", app.ID)
		}
	}

	// On macOS, Finder should always be available
	if runtime.GOOS == "darwin" {
		var found bool
		for _, app := range apps {
			if app.ID == "finder" {
				found = true
				break
			}
		}
		if !found {
			t.Error("finder app should always be available on macOS")
		}
	}
}

func TestFilterAvailable(t *testing.T) {
	input := []AppConfig{
		{ID: "available", Name: "Available", Exec: []string{"echo"}, Enabled: true},
		{ID: "unavailable", Name: "Unavailable", Exec: []string{"echo"}, Enabled: false},
		{ID: "also-available", Name: "Also Available", Exec: []string{"echo"}, Enabled: true},
	}

	result := filterAvailable(input)

	if len(result) != 2 {
		t.Fatalf("expected 2 available apps, got %d", len(result))
	}
	if result[0].ID != "available" || result[1].ID != "also-available" {
		t.Errorf("unexpected app IDs: %v", result)
	}
	for _, app := range result {
		if !app.Enabled {
			t.Errorf("filtered app %q should be enabled", app.ID)
		}
	}
}

func TestFilterAvailableEmpty(t *testing.T) {
	input := []AppConfig{
		{ID: "unavailable", Enabled: false},
	}

	result := filterAvailable(input)
	if len(result) != 0 {
		t.Errorf("expected 0 available apps, got %d", len(result))
	}
}

func TestConfigGetApp(t *testing.T) {
	cfg := Config{
		AllowedApps: []AppConfig{
			{ID: "test", Name: "Test", Exec: []string{"echo"}, Enabled: true},
			{ID: "disabled", Name: "Disabled", Exec: []string{"echo"}, Enabled: false},
		},
	}

	// Should find enabled app
	app := cfg.GetApp("test")
	if app == nil {
		t.Fatal("GetApp should return enabled app")
	}
	if app.ID != "test" {
		t.Errorf("got app ID %q, want %q", app.ID, "test")
	}

	// Should not find disabled app
	app = cfg.GetApp("disabled")
	if app != nil {
		t.Error("GetApp should not return disabled app")
	}

	// Should not find non-existent app
	app = cfg.GetApp("nonexistent")
	if app != nil {
		t.Error("GetApp should not return non-existent app")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := Default()

	if cfg.Theme != "dark" {
		t.Errorf("default theme should be 'dark', got %q", cfg.Theme)
	}

	if cfg.AllowedApps == nil {
		t.Error("AllowedApps should not be nil")
	}
}

func TestAppConfigJSON(t *testing.T) {
	app := AppConfig{
		ID:      "vscode",
		Name:    "VS Code",
		Exec:    []string{"code", "--new-window", "{}"},
		Enabled: true,
	}

	data, err := json.Marshal(app)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded AppConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != app.ID || decoded.Name != app.Name {
		t.Error("JSON round-trip failed")
	}

	if len(decoded.Exec) != 3 || decoded.Exec[0] != "code" || decoded.Exec[2] != "{}" {
		t.Errorf("Exec not preserved in JSON round-trip: %v", decoded.Exec)
	}
}

func TestBuildCommand(t *testing.T) {
	tests := []struct {
		name     string
		exec     []string
		path     string
		wantCmd  string
		wantArgs []string
	}{
		{
			name:     "placeholder at end",
			exec:     []string{"open", "{}"},
			path:     "/foo/bar",
			wantCmd:  "open",
			wantArgs: []string{"/foo/bar"},
		},
		{
			name:     "placeholder in middle",
			exec:     []string{"code", "--goto", "{}", "--reuse-window"},
			path:     "/foo/bar",
			wantCmd:  "code",
			wantArgs: []string{"--goto", "/foo/bar", "--reuse-window"},
		},
		{
			name:     "no placeholder appends path",
			exec:     []string{"open", "-a", "Terminal"},
			path:     "/foo/bar",
			wantCmd:  "open",
			wantArgs: []string{"-a", "Terminal", "/foo/bar"},
		},
		{
			name:     "empty exec",
			exec:     []string{},
			path:     "/foo/bar",
			wantCmd:  "",
			wantArgs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := AppConfig{Exec: tt.exec}
			cmd, args := app.BuildCommand(tt.path)

			if cmd != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tt.wantCmd)
			}

			if len(args) != len(tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
				return
			}

			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestAppInfo(t *testing.T) {
	app := AppConfig{
		ID:      "test",
		Name:    "Test App",
		Exec:    []string{"secret", "command"},
		Enabled: true,
	}

	info := app.Info()

	if info.ID != app.ID || info.Name != app.Name || info.Enabled != app.Enabled {
		t.Error("Info() should copy ID, Name, Enabled")
	}

	// Verify Exec is not exposed via JSON
	data, _ := json.Marshal(info)
	if string(data) != `{"id":"test","name":"Test App","enabled":true}` {
		t.Errorf("unexpected JSON: %s", data)
	}
}

func TestGetEnabledApps(t *testing.T) {
	cfg := Config{
		AllowedApps: []AppConfig{
			{ID: "a", Enabled: true},
			{ID: "b", Enabled: false},
			{ID: "c", Enabled: true},
		},
	}

	enabled := cfg.GetEnabledApps()
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled apps, got %d", len(enabled))
	}

	ids := make(map[string]bool)
	for _, app := range enabled {
		ids[app.ID] = true
	}

	if !ids["a"] || !ids["c"] {
		t.Error("expected apps 'a' and 'c' to be enabled")
	}
	if ids["b"] {
		t.Error("app 'b' should not be in enabled list")
	}
}

func TestValidateApps_RejectsTamperedExec(t *testing.T) {
	// Simulate a tampered config with a malicious Exec command
	tampered := []AppConfig{
		{ID: "finder", Name: "Finder", Exec: []string{"rm", "-rf", "{}"}, Enabled: true},
	}

	result := validateApps(tampered)

	// Find the finder app in the result
	for _, app := range result {
		if app.ID == "finder" {
			// Exec should be from the trusted list, not the tampered one
			if len(app.Exec) > 0 && app.Exec[0] == "rm" {
				t.Error("validateApps should replace tampered Exec with trusted Exec")
			}
			// User's Enabled preference should be preserved
			if !app.Enabled {
				t.Error("validateApps should preserve user's Enabled preference")
			}
			return
		}
	}
}

func TestValidateApps_RejectsUnknownApps(t *testing.T) {
	// Unknown apps should be dropped entirely
	loaded := []AppConfig{
		{ID: "malicious-app", Name: "Evil", Exec: []string{"evil"}, Enabled: true},
	}

	result := validateApps(loaded)

	for _, app := range result {
		if app.ID == "malicious-app" {
			t.Error("validateApps should reject apps not in the trusted list")
		}
	}
}

func TestValidateApps_PreservesUserEnabled(t *testing.T) {
	// User disables a trusted app - that preference should be preserved
	defaults := DefaultApps()
	if len(defaults) == 0 {
		t.Skip("no default apps on this platform")
	}

	// Disable the first default app
	loaded := []AppConfig{
		{ID: defaults[0].ID, Enabled: false},
	}

	result := validateApps(loaded)

	for _, app := range result {
		if app.ID == defaults[0].ID {
			if app.Enabled {
				t.Errorf("validateApps should preserve user's disabled preference for %q", app.ID)
			}
			return
		}
	}
}

func TestValidateApps_EmptyReturnsDefaults(t *testing.T) {
	result := validateApps(nil)
	if len(result) == 0 {
		t.Error("validateApps with nil input should return defaults")
	}
}
