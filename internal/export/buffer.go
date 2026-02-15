package export

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// DiskBuffer provides local disk buffering for trace payloads when the
// collector is unreachable. Payloads are stored as individual JSON files
// in the buffer directory and drained in chronological order.
type DiskBuffer struct {
	dir       string
	maxBytes  int64
}

// NewDiskBuffer creates a new DiskBuffer at the given directory with a size limit.
func NewDiskBuffer(dir string, maxSizeMB int) (*DiskBuffer, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create buffer dir: %w", err)
	}

	return &DiskBuffer{
		dir:      dir,
		maxBytes: int64(maxSizeMB) * 1024 * 1024,
	}, nil
}

// Write stores a payload to the buffer directory as a JSON file.
func (b *DiskBuffer) Write(payload TracePayload) error {
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

	// Use timestamp-based filename for ordering
	filename := fmt.Sprintf("%d_%s.json", time.Now().UnixNano(), payload.SessionID)
	path := filepath.Join(b.dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write buffer file: %w", err)
	}

	tuilog.Log.Debug("Buffered payload", "path", path, "entries", len(payload.Entries))
	return nil
}

// Drain reads all buffered payloads in order and ships them using the provided
// function. Successfully shipped payloads are deleted from disk.
// Returns the number of payloads successfully drained.
func (b *DiskBuffer) Drain(ctx context.Context, ship func(TracePayload) error) (int, error) {
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

		var payload TracePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			tuilog.Log.Warn("Failed to decode buffer file, removing", "path", path, "error", err)
			os.Remove(path)
			continue
		}

		if err := ship(payload); err != nil {
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
