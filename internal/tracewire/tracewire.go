// Package tracewire defines the payload a relay posts to a collector's
// POST /v1/traces: one session's full entries (thinkt.Entry, as the REST
// API serialises them), the uuids it no longer has, and a snapshot of the
// session's metadata.
package tracewire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Version is the payload version this package reads and writes.
const Version = 2

// Session is a snapshot of one session's metadata. Every payload carries
// the whole snapshot. Generation identifies the relay's sync of the session
// that produced the payload; every chunk of one sync shares it.
type Session struct {
	ID              string              `json:"id"`
	ProjectPath     string              `json:"project_path,omitempty"`
	ParentSessionID string              `json:"parent_session_id,omitempty"`
	Title           string              `json:"title,omitempty"`
	Model           string              `json:"model,omitempty"`
	CreatedAt       time.Time           `json:"created_at,omitzero"`
	ModifiedAt      time.Time           `json:"modified_at,omitzero"`
	EntryCount      int                 `json:"entry_count,omitempty"`
	Cost            *thinkt.SessionCost `json:"cost,omitempty"`
	Generation      int64               `json:"generation"`
}

// Payload is the POST /v1/traces body.
type Payload struct {
	Version    int            `json:"version"`
	InstanceID string         `json:"instance_id"`
	MachineID  string         `json:"machine_id"`
	Source     string         `json:"source"`
	Session    Session        `json:"session"`
	Entries    []thinkt.Entry `json:"entries,omitempty"`
	Removed    []string       `json:"removed,omitempty"`
}

// SessionFromMeta copies a session's metadata into a snapshot. Generation
// is left zero for the relay to set.
func SessionFromMeta(m thinkt.SessionMeta) Session {
	return Session{
		ID:              m.ID,
		ProjectPath:     m.ProjectPath,
		ParentSessionID: m.ParentSessionID,
		Title:           m.Title,
		Model:           m.Model,
		CreatedAt:       m.CreatedAt,
		ModifiedAt:      m.ModifiedAt,
		EntryCount:      m.EntryCount,
		Cost:            m.Cost,
	}
}

// EntryHash is the hex SHA-256 of an entry's JSON encoding. encoding/json
// writes struct fields in order and map keys sorted, so equal entries hash
// equally.
func EntryHash(e thinkt.Entry) string {
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
