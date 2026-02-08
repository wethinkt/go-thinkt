package config

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestDefaultApps(t *testing.T) {
	apps := DefaultApps()

	if len(apps) == 0 {
		t.Fatal("DefaultApps returned empty slice")
	}

	// Each platform should include its file manager and editor apps
	var expectedID string
	switch runtime.GOOS {
	case "darwin":
		expectedID = "finder"
	case "linux":
		expectedID = "files"
	case "windows":
		expectedID = "explorer"
	default:
		t.Skipf("unsupported GOOS %q", runtime.GOOS)
	}

	var found bool
	for _, app := range apps {
		if app.ID == expectedID {
			found = true
			if len(app.Exec) == 0 {
				t.Errorf("%s app has empty Exec", expectedID)
			}
			break
		}
	}
	if !found {
		t.Errorf("%s app not found in default apps", expectedID)
	}

	// All platforms should include editor apps (vscode, cursor, zed)
	editorIDs := map[string]bool{"vscode": false, "cursor": false, "zed": false}
	for _, app := range apps {
		if _, ok := editorIDs[app.ID]; ok {
			editorIDs[app.ID] = true
		}
	}
	for id, found := range editorIDs {
		if !found {
			t.Errorf("editor app %q not found in default apps", id)
		}
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
