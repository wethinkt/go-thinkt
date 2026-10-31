package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// DiskBuffer provides local disk buffering for trace payloads when the
// collector is unreachable. Payloads are stored as individual JSON files
// in the buffer directory and drained in chronological order.
type DiskBuffer struct {
	dir        string
	quarantine string // where permanently rejected payloads go; "" deletes them
	maxBytes   int64
	seq        atomic.Uint64 // orders writes within one nanosecond
}

// NewDiskBuffer creates a new DiskBuffer at the given directory with a size limit.
func NewDiskBuffer(dir string, maxSizeMB int) (*DiskBuffer, error) {
	if err := os.MkdirAll(dir, config.DirPerms); err != nil {
		return nil, fmt.Errorf("create buffer dir: %w", err)
	}

	return &DiskBuffer{
		dir:      dir,
		maxBytes: int64(maxSizeMB) * 1024 * 1024,
	}, nil
}

// Write stores a payload to the buffer directory as a JSON file.
func (b *DiskBuffer) Write(payload tracewire.Payload) error {
	// Check size limit
	currentSize, err := b.Size()
	if err != nil {
		return fmt.Errorf("check buffer size: %w", err)
	}
	if currentSize >= b.maxBytes {
		return fmt.Errorf("buffer full (%d MB limit reached)", b.maxBytes/(1024*1024))
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// The name sorts chronologically and never contains the session id
	// itself, which can hold a "/" (subagent ids).
	key := sha256.Sum256([]byte(payload.Source + "\x00" + payload.Session.ID))
	filename := fmt.Sprintf("%020d_%06d_%s.json", time.Now().UnixNano(), b.seq.Add(1)%1000000, hex.EncodeToString(key[:])[:12])
	path := filepath.Join(b.dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write buffer file: %w", err)
	}

	tuilog.Log.Debug("Buffered payload", "path", path, "session_id", payload.Session.ID, "entries", len(payload.Entries))
	return nil
}

// Drain reads all buffered payloads in order and ships them using the provided
// function. Successfully shipped payloads are deleted from disk.
// Returns the number of payloads successfully drained.
func (b *DiskBuffer) Drain(ctx context.Context, ship func(tracewire.Payload) error) (int, error) {
	files, err := b.listFiles()
	if err != nil {
		return 0, err
	}

	shipped := 0
	for _, path := range files {
		select {
		case <-ctx.Done():
			return shipped, ctx.Err()
		default:
		}

		data, err := os.ReadFile(path)
		if err != nil {
			tuilog.Log.Warn("Failed to read buffer file", "path", path, "error", err)
			continue
		}

		var payload tracewire.Payload
		if err := json.Unmarshal(data, &payload); err != nil {
			tuilog.Log.Warn("Failed to decode buffer file, removing", "path", path, "error", err)
			os.Remove(path)
			continue
		}

		if err := ship(payload); err != nil {
			var rejected *RejectedError
			if errors.As(err, &rejected) {
				b.setAside(path, payload, err)
				continue
			}
			tuilog.Log.Debug("Drain ship failed, stopping", "path", path, "error", err)
			return shipped, nil // Stop draining on first failure, don't delete
		}

		os.Remove(path)
		shipped++
	}

	return shipped, nil
}

// Size returns the total size of all buffered files in bytes.
func (b *DiskBuffer) Size() (int64, error) {
	files, err := b.listFiles()
	if err != nil {
		return 0, err
	}

	var total int64
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total, nil
}

// Count returns the number of buffered payloads.
func (b *DiskBuffer) Count() (int, error) {
	files, err := b.listFiles()
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

// listFiles returns all JSON files in the buffer directory, sorted by name (chronological).
func (b *DiskBuffer) listFiles() ([]string, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read buffer dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".json" {
			files = append(files, filepath.Join(b.dir, e.Name()))
		}
	}

	sort.Strings(files)
	return files, nil
}

// Discard removes the buffered payloads keep rejects (a source turned off
// since they were written) and returns how many it removed.
func (b *DiskBuffer) Discard(keep func(tracewire.Payload) bool) (int, error) {
	files, err := b.listFiles()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var payload tracewire.Payload
		if json.Unmarshal(data, &payload) != nil || !keep(payload) {
			if os.Remove(path) == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// setAside moves a buffered payload the collector permanently rejected
// into the quarantine directory, so it no longer blocks the payloads
// behind it but is kept for inspection.
func (b *DiskBuffer) setAside(path string, payload tracewire.Payload, reason error) {
	tuilog.Log.Error("Collector rejected a buffered payload; moved to quarantine",
		"source", payload.Source, "session_id", payload.Session.ID, "entries", len(payload.Entries), "error", reason)
	if b.quarantine == "" {
		_ = os.Remove(path)
		return
	}
	if err := os.MkdirAll(b.quarantine, config.DirPerms); err == nil {
		if os.Rename(path, filepath.Join(b.quarantine, filepath.Base(path))) == nil {
			return
		}
	}
	_ = os.Remove(path)
}

// Quarantine writes a payload the collector rejected straight into the
// quarantine directory.
func (b *DiskBuffer) Quarantine(payload tracewire.Payload, reason error) {
	tmp := &DiskBuffer{dir: b.quarantine, maxBytes: 1 << 62}
	if b.quarantine == "" || os.MkdirAll(b.quarantine, config.DirPerms) != nil || tmp.Write(payload) != nil {
		tuilog.Log.Error("Collector rejected a payload; it could not be kept", "session_id", payload.Session.ID, "error", reason)
		return
	}
	tuilog.Log.Error("Collector rejected a payload; kept in quarantine",
		"source", payload.Source, "session_id", payload.Session.ID, "entries", len(payload.Entries), "error", reason)
}
