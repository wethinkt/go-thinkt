package share

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/urlutil"
)

const defaultEndpoint = "https://share.wethinkt.com"

// Endpoint returns the validated share API URL from THINKT_SHARE_URL, or the default.
func Endpoint() (string, error) {
	raw := defaultEndpoint
	if v := os.Getenv("THINKT_SHARE_URL"); v != "" {
		raw = v
	}
	u, err := urlutil.ValidateEndpointURL(raw)
	if err != nil {
		return "", fmt.Errorf("invalid share endpoint: %w", err)
	}
	return u, nil
}

type Credentials struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Endpoint string `json:"endpoint"`
	Provider string `json:"provider,omitempty"` // "github" or "google"
}

func DefaultCredentialsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "thinkt", "auth.json")
}

func SaveCredentials(path string, creds *Credentials) error {
	if _, err := urlutil.ValidateEndpointURL(creds.Endpoint); err != nil {
		return fmt.Errorf("invalid credentials endpoint: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), config.DirPerms); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

func LoadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if _, err := urlutil.ValidateEndpointURL(creds.Endpoint); err != nil {
		return nil, fmt.Errorf("invalid credentials endpoint: %w", err)
	}
	return &creds, nil
}
