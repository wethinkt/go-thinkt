package config

import (
	"os"
	"os/exec"
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
//   - Canonicalization to an absolute, symlink-resolved path
//   - Location within allowed directories (verified)
//
// The path is passed directly to exec.Command, not through a shell, but
// proper path validation is essential to prevent opening unintended locations.
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

// Launch executes the application with the specified validated path.
// It returns an error if the command fails to start.
// The command is started in the background; caller does not wait for it to exit.
func (a AppConfig) Launch(validatedPath string) error {
	cmdName, args := a.BuildCommand(validatedPath)
	if cmdName == "" {
		return os.ErrInvalid
	}

	cmd := exec.Command(cmdName, args...)
	if err := cmd.Start(); err != nil {
		return err
	}

	// Don't wait for the command to finish - it's opening an external app
	go cmd.Wait()
	return nil
}

// filterAvailable returns only apps that were probed as available (Enabled == true).
func filterAvailable(apps []AppConfig) []AppConfig {
	var available []AppConfig
	for _, app := range apps {
		if app.Enabled {
			available = append(available, app)
		}
	}
	return available
}


// checkCommandExists checks if a command is available in PATH.
func checkCommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
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
