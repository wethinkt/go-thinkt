package config

import (
	"os"
	"os/exec"
	"strings"
)

// AppConfig defines a launchable application for the open-in feature.
type AppConfig struct {
	ID      string   `json:"id"`                  // Short identifier (e.g., "finder", "vscode")
	Name    string   `json:"name"`                // Display name (e.g., "Finder", "VS Code")
	Exec    []string `json:"exec,omitempty"`      // Command and args; {} is replaced with path
	ExecRun []string `json:"exec_run,omitempty"`  // Command to run a shell command in this app; {} is replaced inline
	Enabled bool     `json:"enabled"`             // Whether this app is enabled
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
		if strings.Contains(arg, "{}") {
			args = append(args, strings.ReplaceAll(arg, "{}", path))
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
	// Run Wait in background to reap the zombie process, ignore errors
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

// BuildRunCommand returns the command and args for running a shell command in this app.
// Unlike BuildCommand, {} is replaced inline within args (not just as a standalone arg),
// allowing patterns like ["osascript", "-e", "tell app \"Terminal\" to do script \"{}\""].
func (a AppConfig) BuildRunCommand(shellCmd string) (string, []string) {
	if len(a.ExecRun) == 0 {
		return "", nil
	}

	cmd := a.ExecRun[0]
	args := make([]string, 0, len(a.ExecRun)-1)
	for _, arg := range a.ExecRun[1:] {
		args = append(args, strings.ReplaceAll(arg, "{}", shellCmd))
	}

	return cmd, args
}

// LaunchCommand executes a shell command inside this application (e.g., run a command in a terminal).
// It uses the ExecRun pattern rather than Exec.
func (a AppConfig) LaunchCommand(shellCmd string) error {
	cmdName, args := a.BuildRunCommand(shellCmd)
	if cmdName == "" {
		return os.ErrInvalid
	}

	cmd := exec.Command(cmdName, args...)
	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		_ = cmd.Wait()
	}()
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


// validateApps validates loaded apps against the hardcoded trusted list.
// Only apps with IDs matching defaults are kept. The Exec field is always
// taken from the trusted list (never from disk) to prevent command injection
// via config file tampering. The user's Enabled preference is preserved.
// If the loaded list is nil or empty, defaults are returned.
func validateApps(loaded []AppConfig) []AppConfig {
	if len(loaded) == 0 {
		return DefaultApps()
	}

	// Build lookup of trusted apps by ID, using the hardcoded Exec.
	trusted := DefaultApps()
	trustedByID := make(map[string]AppConfig, len(trusted))
	for _, app := range trusted {
		trustedByID[app.ID] = app
	}

	// Build lookup of user's enabled preferences by ID.
	userEnabled := make(map[string]bool, len(loaded))
	for _, app := range loaded {
		userEnabled[app.ID] = app.Enabled
	}

	// Rebuild the list: use trusted Exec, but apply user's Enabled preference.
	var validated []AppConfig
	for _, app := range trusted {
		if enabled, exists := userEnabled[app.ID]; exists {
			app.Enabled = enabled
		}
		validated = append(validated, app)
	}

	return validated
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

// GetTerminalApp returns the configured terminal app, falling back to the "terminal" app ID.
func (c Config) GetTerminalApp() *AppConfig {
	id := c.Terminal
	if id == "" {
		id = "terminal"
	}
	return c.GetApp(id)
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
