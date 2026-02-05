package config

import (
	"os"
	"os/exec"
	"path/filepath"
)

// AppConfig defines a launchable application for the open-in feature.
type AppConfig struct {
	ID      string   `json:"id"`             // Short identifier (e.g., "finder", "vscode")
	Name    string   `json:"name"`           // Display name (e.g., "Finder", "VS Code")
	Exec    []string `json:"exec,omitempty"` // Command and args; {} is replaced with path
	Enabled bool     `json:"enabled"`        // Whether this app is enabled
}

// AppInfo is the public API representation of an app (excludes Exec).
type AppInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// Info returns the public API representation of this app.
func (a AppConfig) Info() AppInfo {
	return AppInfo{
		ID:      a.ID,
		Name:    a.Name,
		Enabled: a.Enabled,
	}
}

// BuildCommand returns the command and args with {} replaced by path.
// If no {} placeholder exists, path is appended as the last argument.
//
// SECURITY NOTE: The path parameter must be validated before calling this function.
// It should be an absolute path that has been checked for:
//   - Shell metacharacters (rejected)
//   - Path traversal attempts (rejected)
//   - Symlink resolution (verified)
//   - Location within allowed directories (verified)
// The path is passed directly to exec.Command, not through a shell, but
// proper validation is essential to prevent command injection.
func (a AppConfig) BuildCommand(path string) (string, []string) {
	if len(a.Exec) == 0 {
		return "", nil
	}

	cmd := a.Exec[0]
	args := make([]string, 0, len(a.Exec))

	hasPlaceholder := false
	for _, arg := range a.Exec[1:] {
		if arg == "{}" {
			args = append(args, path)
			hasPlaceholder = true
		} else {
			args = append(args, arg)
		}
	}

	if !hasPlaceholder {
		args = append(args, path)
	}

	return cmd, args
}

// DefaultApps returns the default app configurations.
// On macOS, it checks which apps are available.
func DefaultApps() []AppConfig {
	return []AppConfig{
		{
			ID:      "finder",
			Name:    "Finder",
			Exec:    []string{"open", "{}"},
			Enabled: true,
		},
		{
			ID:      "terminal",
			Name:    "Terminal",
			Exec:    []string{"open", "-a", "Terminal", "{}"},
			Enabled: true,
		},
		{
			ID:      "iterm",
			Name:    "iTerm",
			Exec:    []string{"open", "-a", "iTerm", "{}"},
			Enabled: checkAppExists("iTerm"),
		},
		{
			ID:      "vscode",
			Name:    "VS Code",
			Exec:    []string{"code", "{}"},
			Enabled: checkCommandExists("code"),
		},
		{
			ID:      "cursor",
			Name:    "Cursor",
			Exec:    []string{"cursor", "{}"},
			Enabled: checkCommandExists("cursor"),
		},
		{
			ID:      "zed",
			Name:    "Zed",
			Exec:    []string{"zed", "{}"},
			Enabled: checkCommandExists("zed"),
		},
	}
}

// checkCommandExists checks if a command is available in PATH.
func checkCommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// checkAppExists checks if a macOS app exists in /Applications.
func checkAppExists(name string) bool {
	paths := []string{
		"/Applications/" + name + ".app",
		filepath.Join(os.Getenv("HOME"), "Applications", name+".app"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// GetApp returns an app config by ID, or nil if not found or disabled.
func (c Config) GetApp(id string) *AppConfig {
	for i := range c.AllowedApps {
		if c.AllowedApps[i].ID == id && c.AllowedApps[i].Enabled {
			return &c.AllowedApps[i]
		}
	}
	return nil
}

// GetEnabledApps returns all enabled apps as public API info.
func (c Config) GetEnabledApps() []AppInfo {
	var enabled []AppInfo
	for _, app := range c.AllowedApps {
		if app.Enabled {
			enabled = append(enabled, app.Info())
		}
	}
	return enabled
}
