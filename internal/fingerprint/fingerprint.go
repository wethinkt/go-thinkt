// Package fingerprint provides machine fingerprinting capabilities for thinkt.
// It generates a stable, unique identifier for the machine that persists
// across reboots and can be used to correlate workspaces across different
// AI coding assistant sources.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/config"
)

// Info contains detailed information about the fingerprint.
type Info struct {
	// Fingerprint is the machine fingerprint (normalized UUID format).
	Fingerprint string `json:"fingerprint"`

	// Source indicates how the fingerprint was derived.
	Source string `json:"source"`

	// Path is the file path or source location (if applicable).
	Path string `json:"path,omitempty"`

	// Components are the raw identifiers used to generate the fingerprint.
	Components []string `json:"components,omitempty"`
}

// String returns a human-readable representation of the fingerprint info.
func (i Info) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Fingerprint: %s\n", i.Fingerprint)
	fmt.Fprintf(&b, "Source:      %s\n", i.Source)
	if i.Path != "" {
		fmt.Fprintf(&b, "Path:        %s\n", i.Path)
	}
	return b.String()
}

// Get returns the machine fingerprint with detailed information.
// It tries multiple sources in order of preference:
//  1. System identifiers (machine-id, IOPlatformUUID, etc.)
//  2. Cached thinkt machine_id
//  3. Generate and cache a new fingerprint
func Get() (Info, error) {
	// Try platform-specific system identifiers first
	if info := getSystemFingerprint(); info.Fingerprint != "" {
		return info, nil
	}

	// Try cached thinkt machine_id
	if info := getCachedFingerprint(); info.Fingerprint != "" {
		return info, nil
	}

	// Generate and cache a new fingerprint
	return generateAndCacheFingerprint()
}

// GetFingerprint returns just the fingerprint string (convenience method).
func GetFingerprint() (string, error) {
	info, err := Get()
	if err != nil {
		return "", err
	}
	return info.Fingerprint, nil
}

// getCachedFingerprint tries to read a previously generated fingerprint.
func getCachedFingerprint() Info {
	path, err := getMachineIDPath()
	if err != nil {
		return Info{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}
	}

	fp := strings.TrimSpace(string(data))
	if fp == "" {
		return Info{}
	}

	return Info{
		Fingerprint: normalizeFingerprint(fp),
		Source:      "thinkt-generated",
		Path:        path,
	}
}

// generateAndCacheFingerprint creates a new fingerprint and caches it.
func generateAndCacheFingerprint() (Info, error) {
	// Generate from hostname + user + random component
	// This provides some stability while being unique
	fp, err := generateFingerprint()
	if err != nil {
		return Info{}, fmt.Errorf("generating fingerprint: %w", err)
	}

	// Cache it for future use
	path, err := getMachineIDPath()
	if err != nil {
		return Info{}, fmt.Errorf("getting machine ID path: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return Info{}, fmt.Errorf("creating directory: %w", err)
	}

	// Write with restricted permissions
	if err := os.WriteFile(path, []byte(fp+"\n"), 0600); err != nil {
		return Info{}, fmt.Errorf("writing fingerprint: %w", err)
	}

	return Info{
		Fingerprint: fp,
		Source:      "thinkt-generated",
		Path:        path,
	}, nil
}

// generateFingerprint creates a fingerprint from available system info.
func generateFingerprint() (string, error) {
	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}

	// Combine components
	h := sha256.New()
	if hostname != "" {
		h.Write([]byte(hostname))
	}
	if username != "" {
		h.Write([]byte(username))
	}

	// Use first 16 bytes (32 hex chars) for UUID-like format
	hash := h.Sum(nil)
	return formatAsUUID(hex.EncodeToString(hash[:16])), nil
}

// getMachineIDPath returns the path to thinkt's machine_id file.
var getMachineIDPath = func() (string, error) {
	configDir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "machine_id"), nil
}

// normalizeFingerprint ensures consistent format (lowercase, no dashes for hashing).
func normalizeFingerprint(fp string) string {
	// Remove dashes and spaces, lowercase
	fp = strings.ToLower(strings.ReplaceAll(fp, "-", ""))
	fp = strings.ReplaceAll(fp, " ", "")
	fp = strings.TrimSpace(fp)

	// If it's already 32 hex chars, format as UUID
	if len(fp) == 32 {
		return formatAsUUID(fp)
	}

	// Otherwise hash it to get consistent length
	h := sha256.New()
	h.Write([]byte(fp))
	return formatAsUUID(hex.EncodeToString(h.Sum(nil)[:16]))
}

// formatAsUUID formats a 32-char hex string as UUID (8-4-4-4-12).
func formatAsUUID(s string) string {
	if len(s) < 32 {
		// Pad if necessary
		s = s + strings.Repeat("0", 32-len(s))
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return strings.ToLower(s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32])
}
