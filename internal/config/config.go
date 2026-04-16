// Package config provides application configuration management for thinkt.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	envHome                   = "THINKT_HOME"
	defaultDirName            = ".thinkt"
	defaultConfigFile         = "config.json"
	defaultTheme              = "dark"
	defaultEmbedModel         = "nomic-embed-text-v1.5"
	defaultSummarizationModel = "qwen2.5-3b-instruct"
	defaultDebounce           = "2s"
	defaultRescanInterval     = "60s"
	defaultFilePerms          = 0600
)

// DirPerms is the permission mode for directories under ~/.thinkt/.
const DirPerms = 0700

// ErrNoConfig is returned by Load when no config file exists on disk.
var ErrNoConfig = errors.New("config file does not exist")

// SourceConfig holds per-source settings.
type SourceConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
}

// Config holds the thinkt configuration.
type Config struct {
	Sources       map[string]SourceConfig `json:"sources,omitempty"`       // Per-source enabled/disabled settings
	DiscoveredAt  *time.Time              `json:"discovered_at,omitempty"` // When discover wizard last ran
	Theme         string                  `json:"theme"`                   // Name of the active theme
	Language      string                  `json:"language,omitempty"`      // BCP 47 language tag (e.g., "en", "zh-Hans", "ja")
	Terminal      string                  `json:"terminal,omitempty"`      // App ID for default terminal (e.g., "ghostty", "kitty")
	AllowedApps   []AppConfig             `json:"allowed_apps,omitempty"`  // Apps allowed for open-in
	Embedding     EmbeddingConfig         `json:"embedding"`               // Embedding settings
	Summarization SummarizationConfig     `json:"summarization"`           // Summarization settings
	Indexer       IndexerConfig           `json:"indexer"`                 // Indexer settings
	Share         ShareConfig             `json:"share"`                   // Sharing platform settings
}

// EmbeddingConfig holds embedding-related settings.
type EmbeddingConfig struct {
	Enabled bool   `json:"enabled"` // Enable GPU embedding
	Model   string `json:"model"`   // Embedding model ID
}

// SummarizationConfig holds summarization-related settings.
type SummarizationConfig struct {
	Enabled bool   `json:"enabled"` // Enable local summarization
	Model   string `json:"model"`   // Summarization model ID
}

// ShareConfig holds sharing platform settings.
type ShareConfig struct {
	Enabled bool   `json:"enabled"` // Enable sharing features
	URL     string `json:"url,omitempty"` // Share endpoint URL override
}

// IndexerConfig holds indexer-related settings.
type IndexerConfig struct {
	Sources        []string `json:"sources"`         // Source filter (empty = all)
	Watch          bool     `json:"watch"`           // Enable file watching
	Debounce       string   `json:"debounce"`        // Debounce duration (e.g. "2s")
	RescanInterval string   `json:"rescan_interval"` // Periodic project rescan (e.g. "60s"; "0" disables)
}

// DebounceDuration returns the parsed debounce duration (default: 2s).
func (c IndexerConfig) DebounceDuration() time.Duration {
	if c.Debounce != "" {
		if d, err := time.ParseDuration(c.Debounce); err == nil {
			return d
		}
	}
	return 2 * time.Second
}

// RescanIntervalDuration returns the parsed rescan interval. A value ≤0 disables rescanning.
// Empty or unparseable defaults to 60s.
func (c IndexerConfig) RescanIntervalDuration() time.Duration {
	if c.RescanInterval == "" {
		return 60 * time.Second
	}
	d, err := time.ParseDuration(c.RescanInterval)
	if err != nil {
		return 60 * time.Second
	}
	return d
}

// EnabledSources returns the sorted names of all enabled sources.
func (c Config) EnabledSources() []string {
	if c.Sources == nil {
		return nil
	}
	var enabled []string
	for name, sc := range c.Sources {
		if sc.Enabled {
			enabled = append(enabled, name)
		}
	}
	sort.Strings(enabled)
	return enabled
}

// Dir returns the path to the .thinkt directory.
// It checks the THINKT_HOME environment variable first, then defaults to ~/.thinkt.
func Dir() (string, error) {
	if v := os.Getenv(envHome); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultDirName), nil
}

// Path returns the path to the main config file.
func Path() (string, error) {
	configDir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, defaultConfigFile), nil
}

// Load loads the configuration from ~/.thinkt/config.json.
func Load() (Config, error) {
	configPath, err := Path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return Default(), ErrNoConfig
	} else if err != nil {
		return Config{}, err
	}

	// Start from defaults so missing keys get correct values
	// (e.g. existing configs without embedding/indexer sections
	// won't get false/zero which would disable features).
	config := Default()
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}

	if config.Theme == "" {
		config.Theme = defaultTheme
	}

	// Validate apps against the trusted list.
	// Only apps with IDs matching the hardcoded defaults are kept.
	// The Exec command is always taken from the trusted list (never from disk).
	// The user's Enabled preference from the config file is preserved.
	config.AllowedApps = validateApps(config.AllowedApps)

	return config, nil
}

// Default returns a default configuration with all defaults set.
func Default() Config {
	return Config{
		Theme:       defaultTheme,
		AllowedApps: DefaultApps(),
		Embedding: EmbeddingConfig{
			Enabled: false,
			Model:   defaultEmbedModel,
		},
		Summarization: SummarizationConfig{
			Enabled: false,
			Model:   defaultSummarizationModel,
		},
		Indexer: IndexerConfig{
			Sources:        []string{},
			Watch:          true,
			Debounce:       defaultDebounce,
			RescanInterval: defaultRescanInterval,
		},
		Share: ShareConfig{
			Enabled: true,
		},
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
	if err := os.MkdirAll(dir, DirPerms); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, defaultFilePerms)
}
