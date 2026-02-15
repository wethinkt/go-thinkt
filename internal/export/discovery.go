package export

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// collectorJSON is the schema for .thinkt/collector.json and .well-known/thinkt.json.
type collectorJSON struct {
	CollectorURL string `json:"collector_url"`
}

// DiscoverCollector resolves a collector endpoint using a priority cascade:
//  1. THINKT_COLLECTOR_URL environment variable
//  2. .thinkt/collector.json in the project directory
//  3. Well-known endpoint: https://{domain}/.well-known/thinkt.json (not implemented yet)
//  4. Fallback to local file write (no remote collector)
func DiscoverCollector(projectPath string) (*CollectorEndpoint, error) {
	// 1. Environment variable (highest priority)
	if url := os.Getenv("THINKT_COLLECTOR_URL"); url != "" {
		tuilog.Log.Info("Collector discovered via env", "url", url)
		return &CollectorEndpoint{URL: url, Origin: "env"}, nil
	}

	// 2. Project-level config file
	if projectPath != "" {
		configPath := filepath.Join(projectPath, ".thinkt", "collector.json")
		if endpoint, err := readCollectorConfig(configPath); err == nil {
			tuilog.Log.Info("Collector discovered via project config", "url", endpoint.URL, "path", configPath)
			return endpoint, nil
		}
	}

	// 3. Well-known endpoint discovery
	if endpoint, err := discoverWellKnown(projectPath); err == nil {
		tuilog.Log.Info("Collector discovered via well-known", "url", endpoint.URL)
		return endpoint, nil
	}

	// 4. No remote collector found
	tuilog.Log.Info("No collector discovered, using local buffer only")
	return &CollectorEndpoint{Origin: "local"}, nil
}

// readCollectorConfig reads a collector.json file and returns the endpoint.
func readCollectorConfig(path string) (*CollectorEndpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg collectorJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid collector config %s: %w", path, err)
	}

	if cfg.CollectorURL == "" {
		return nil, fmt.Errorf("collector_url is empty in %s", path)
	}

	return &CollectorEndpoint{URL: cfg.CollectorURL, Origin: "project"}, nil
}

// discoverWellKnown attempts to fetch collector config from a well-known URL.
// This is a placeholder for future domain-based discovery.
func discoverWellKnown(projectPath string) (*CollectorEndpoint, error) {
	// For now, check if there's a domain hint in the project's git remote.
	// This is a simplified implementation - in production you'd parse the git remote
	// and construct the well-known URL from the domain.
	_ = projectPath

	// Example: https://example.com/.well-known/thinkt.json
	// For now, this always fails to fall through to local.
	return nil, fmt.Errorf("well-known discovery not configured")
}

// FetchWellKnown fetches a collector endpoint from a well-known URL.
// The URL should be https://{domain}/.well-known/thinkt.json.
func FetchWellKnown(wellKnownURL string) (*CollectorEndpoint, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(wellKnownURL)
	if err != nil {
		return nil, fmt.Errorf("fetch well-known: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("well-known returned %d", resp.StatusCode)
	}

	var cfg collectorJSON
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode well-known response: %w", err)
	}

	if cfg.CollectorURL == "" {
		return nil, fmt.Errorf("collector_url is empty in well-known response")
	}

	return &CollectorEndpoint{URL: cfg.CollectorURL, Origin: "well-known"}, nil
}
