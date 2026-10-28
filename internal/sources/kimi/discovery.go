package kimi

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Discoverer implements thinkt.StoreFactory for Kimi Code.
type Discoverer struct{}

// NewDiscoverer creates a new Kimi Code discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// Source returns the Kimi source type.
func (d *Discoverer) Source() thinkt.Source {
	return thinkt.SourceKimi
}

// Create creates a Kimi store when the base directory exists.
func (d *Discoverer) Create() (thinkt.Store, error) {
	basePath := d.basePath()
	if basePath == "" {
		return nil, nil
	}
	return NewStore(basePath), nil
}

// IsAvailable reports whether Kimi Code has a sessions directory.
func (d *Discoverer) IsAvailable() (bool, error) {
	basePath := d.basePath()
	if basePath == "" {
		return false, nil
	}
	info, err := os.Stat(filepath.Join(basePath, "sessions"))
	if err != nil {
		return false, nil
	}
	return info.IsDir(), nil
}

// basePath returns the Kimi Code base directory when it exists:
// THINKT_KIMI_HOME, else ~/.kimi-code.
func (d *Discoverer) basePath() string {
	if kimiHome := os.Getenv("THINKT_KIMI_HOME"); kimiHome != "" {
		return kimiHome
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".kimi-code")
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	return dir
}

// DefaultDir returns the Kimi Code base directory: THINKT_KIMI_HOME if set,
// otherwise ~/.kimi-code.
func DefaultDir() (string, error) {
	if kimiHome := os.Getenv("THINKT_KIMI_HOME"); kimiHome != "" {
		return kimiHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kimi-code"), nil
}

// splitWirePath returns the base directory, session directory name, and
// agent id of a path of the form
// <base>/sessions/<wd>/<session>/agents/<agent>/wire.jsonl, or ok=false.
func splitWirePath(path string) (base, session, agent string, ok bool) {
	clean := filepath.Clean(path)
	if filepath.Base(clean) != "wire.jsonl" {
		return "", "", "", false
	}
	agentDir := filepath.Dir(clean)
	agentsDir := filepath.Dir(agentDir)
	if filepath.Base(agentsDir) != "agents" {
		return "", "", "", false
	}
	sessionDir := filepath.Dir(agentsDir)
	wdDir := filepath.Dir(sessionDir)
	sessionsDir := filepath.Dir(wdDir)
	if filepath.Base(sessionsDir) != "sessions" {
		return "", "", "", false
	}
	return filepath.Dir(sessionsDir), filepath.Base(sessionDir), filepath.Base(agentDir), true
}

// IsSessionPath reports whether path is a Kimi Code wire file:
// .../sessions/<wd>/<session>/agents/<agent>/wire.jsonl.
func IsSessionPath(path string) bool {
	_, _, _, ok := splitWirePath(path)
	return ok
}

// BaseDirForPath returns the Kimi Code base directory a wire path belongs
// to (the parent of its sessions/ component), or "" when the path is not a
// wire file.
func BaseDirForPath(path string) string {
	base, _, _, ok := splitWirePath(path)
	if !ok {
		return ""
	}
	return base
}

// SessionIDForPath returns the session id a wire path denotes: the session
// directory name for the main wire, "<session>/<agent>" for a subagent
// wire, "" for anything else.
func SessionIDForPath(path string) string {
	_, session, agent, ok := splitWirePath(path)
	if !ok {
		return ""
	}
	if agent == "main" {
		return session
	}
	return session + "/" + agent
}

// rootSessionID returns the session directory name of an id that may name
// a subagent ("<session>/<agent>").
func rootSessionID(id string) string {
	if i := strings.IndexByte(id, '/'); i >= 0 {
		return id[:i]
	}
	return id
}

// Factory returns a thinkt.StoreFactory for Kimi Code.
func Factory() thinkt.StoreFactory {
	return NewDiscoverer()
}
