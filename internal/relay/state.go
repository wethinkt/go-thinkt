package relay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// sessionState is what the relay last shipped for one session.
type sessionState struct {
	Hashes      map[string]string `json:"hashes"`       // uuid → tracewire.EntryHash
	SessionHash string            `json:"session_hash"` // hash of the last shipped session snapshot
	ModTime     time.Time         `json:"mod_time"`     // file mtime when last synced
	Size        int64             `json:"size"`         // file size when last synced
	Generation  int64             `json:"generation"`   // generation of the last sync
	Path        string            `json:"path"`         // the session file
}

// relayState is the relay's per-session record, persisted across restarts
// so a restart ships only what changed.
type relayState struct {
	Sessions map[string]*sessionState `json:"sessions"`
}

// stateKey names a session in the state.
func stateKey(source thinkt.Source, sessionID string) string {
	return string(source) + "\x00" + sessionID
}

// loadState reads the state file; a missing file is an empty state.
func loadState(path string) (*relayState, error) {
	st := &relayState{Sessions: map[string]*sessionState{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read relay state: %w", err)
	}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("decode relay state: %w", err)
	}
	if st.Sessions == nil {
		st.Sessions = map[string]*sessionState{}
	}
	return st, nil
}

// save writes the state atomically (temp file, then rename).
func (s *relayState) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), config.DirPerms); err != nil {
		return fmt.Errorf("create relay state dir: %w", err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode relay state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.json")
	if err != nil {
		return fmt.Errorf("write relay state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write relay state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write relay state: %w", err)
	}
	return os.Rename(tmp.Name(), path)
}
