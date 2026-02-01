// Package config provides application configuration management for thinkt.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds the thinkt configuration.
type Config struct {
	Theme       string      `json:"theme"`                  // Name of the active theme
	AllowedApps []AppConfig `json:"allowed_apps,omitempty"` // Apps allowed for open-in
}

// Dir returns the path to the .thinkt directory.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".thinkt"), nil
}

// Path returns the path to the main config file.
func Path() (string, error) {
	configDir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

// Load loads the configuration from ~/.thinkt/config.json.
func Load() (Config, error) {
	configPath, err := Path()
	if err != nil {
		return Default(), err
	}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return Default(), nil
	} else if err != nil {
		return Default(), err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Default(), err
	}

	if config.Theme == "" {
		config.Theme = "dark"
	}

	// Initialize default apps if not set
	if config.AllowedApps == nil {
		config.AllowedApps = DefaultApps()
	}

	return config, nil
}

// Default returns a default configuration with all defaults set.
func Default() Config {
	return Config{
		Theme:       "dark",
		AllowedApps: DefaultApps(),
	}
}

// Save saves the configuration to ~/.thinkt/config.json.
func Save(config Config) error {
	configPath, err := Path()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
