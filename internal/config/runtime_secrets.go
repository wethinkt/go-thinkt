package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	runtimeDirName    = "runtime"
	runtimeFilePerms  = 0o600
	browserLaunchTTL  = 30 * time.Second
	browserLaunchSize = 16
)

var (
	// ErrRuntimeSecretNotFound indicates that the requested runtime secret does
	// not exist.
	ErrRuntimeSecretNotFound = errors.New("runtime secret not found")
	// ErrRuntimeSecretExpired indicates that the requested runtime secret has
	// expired and can no longer be used.
	ErrRuntimeSecretExpired = errors.New("runtime secret expired")
)

// BrowserLaunchPayload holds the one-time browser bootstrap data needed to
// restore auth and deep-link state without putting secrets in argv.
type BrowserLaunchPayload struct {
	Path      string     `json:"path,omitempty"`
	Fragment  url.Values `json:"fragment,omitempty"`
	Token     string     `json:"token,omitempty"`
	ExpiresAt time.Time  `json:"expires_at"`
}

func runtimeDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, runtimeDirName), nil
}

func ensureRuntimeDir() (string, error) {
	dir, err := runtimeDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, DirPerms); err != nil {
		return "", err
	}
	if err := securePathPermissions(dir, DirPerms); err != nil {
		return "", err
	}
	return dir, nil
}

func instanceTokenPath(t InstanceType, pid int) (string, error) {
	if t == "" {
		return "", fmt.Errorf("instance type is required")
	}
	if pid <= 0 {
		return "", fmt.Errorf("pid must be positive")
	}
	dir, err := ensureRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d.token", t, pid)), nil
}

func browserLaunchPath(id string) (string, error) {
	if !isRuntimeID(id) {
		return "", fmt.Errorf("invalid browser launch id")
	}
	dir, err := ensureRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("launch-%s.json", id)), nil
}

func isRuntimeID(id string) bool {
	if len(id) == 0 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func randomRuntimeID(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate runtime id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// WriteInstanceToken stores a runtime bearer token for the given instance in a
// private file outside the public instance registry.
func WriteInstanceToken(t InstanceType, pid int, token string) error {
	path, err := instanceTokenPath(t, pid)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(token), runtimeFilePerms); err != nil {
		return err
	}
	return securePathPermissions(path, runtimeFilePerms)
}

// ReadInstanceToken loads a runtime bearer token for the given instance.
func ReadInstanceToken(t InstanceType, pid int) (string, error) {
	path, err := instanceTokenPath(t, pid)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", ErrRuntimeSecretNotFound
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// RemoveInstanceToken deletes any runtime bearer token associated with the
// given instance.
func RemoveInstanceToken(t InstanceType, pid int) error {
	path, err := instanceTokenPath(t, pid)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CreateBrowserLaunch stores a short-lived, single-use browser bootstrap
// payload and returns the opaque launch ticket.
func CreateBrowserLaunch(payload BrowserLaunchPayload) (string, error) {
	if payload.ExpiresAt.IsZero() {
		payload.ExpiresAt = time.Now().Add(browserLaunchTTL)
	}
	id, err := randomRuntimeID(browserLaunchSize)
	if err != nil {
		return "", err
	}
	path, err := browserLaunchPath(id)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, runtimeFilePerms); err != nil {
		return "", err
	}
	if err := securePathPermissions(path, runtimeFilePerms); err != nil {
		return "", err
	}
	return id, nil
}

// ConsumeBrowserLaunch atomically reads and removes a browser bootstrap payload.
func ConsumeBrowserLaunch(id string) (BrowserLaunchPayload, error) {
	path, err := browserLaunchPath(id)
	if err != nil {
		return BrowserLaunchPayload{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return BrowserLaunchPayload{}, ErrRuntimeSecretNotFound
	}
	if err != nil {
		return BrowserLaunchPayload{}, err
	}
	_ = os.Remove(path)

	var payload BrowserLaunchPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return BrowserLaunchPayload{}, err
	}
	if !payload.ExpiresAt.IsZero() && time.Now().After(payload.ExpiresAt) {
		return BrowserLaunchPayload{}, ErrRuntimeSecretExpired
	}
	return payload, nil
}
