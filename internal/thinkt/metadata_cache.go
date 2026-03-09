package thinkt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MetadataCache holds cached session metadata for a single source.
// It is stored as a JSON file at {cacheDir}/sessions-{source}.json.
// All exported methods are safe for concurrent use.
type MetadataCache struct {
	mu        sync.RWMutex
	Version   int                      `json:"version"`
	Source    Source                   `json:"source"`
	UpdatedAt time.Time                `json:"updated_at"`
	Sessions  map[string]CachedSession `json:"sessions"`
	dir       string                   // cache directory path (not serialized)
}

// CachedSession holds the expensive-to-compute metadata for a single session file.
// Entries are keyed by the session's full file path and considered fresh only when
// both ModifiedAt and FileSize match the file on disk.
type CachedSession struct {
	FirstPrompt string    `json:"first_prompt,omitempty"`
	Model       string    `json:"model,omitempty"`
	EntryCount  int       `json:"entry_count,omitempty"`
	GitBranch   string    `json:"git_branch,omitempty"`
	ProjectPath string    `json:"project_path,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitzero"`
	ModifiedAt  time.Time `json:"modified_at"`
	FileSize    int64     `json:"file_size"`
}

// metadataCacheJSON is the on-disk representation without the mutex.
type metadataCacheJSON struct {
	Version   int                      `json:"version"`
	Source    Source                   `json:"source"`
	UpdatedAt time.Time                `json:"updated_at"`
	Sessions  map[string]CachedSession `json:"sessions"`
}

// cacheFileName returns the filename for a source's metadata cache.
func cacheFileName(source Source) string {
	return fmt.Sprintf("sessions-%s.json", source)
}

// LoadMetadataCache reads the cache file for the given source from cacheDir.
// On missing or corrupt files it returns an empty cache without error,
// so callers never need to handle first-run specially.
func LoadMetadataCache(source Source, cacheDir string) (*MetadataCache, error) {
	mc := &MetadataCache{
		Version:  1,
		Source:   source,
		Sessions: make(map[string]CachedSession),
		dir:      cacheDir,
	}

	path := filepath.Join(cacheDir, cacheFileName(source))
	data, err := os.ReadFile(path)
	if err != nil {
		// Missing file is expected on first run.
		return mc, nil
	}

	var disk metadataCacheJSON
	if err := json.Unmarshal(data, &disk); err != nil {
		// Corrupt file: return empty cache, don't propagate error.
		return mc, nil
	}

	mc.Version = disk.Version
	mc.Source = source // defensive against edited files
	mc.UpdatedAt = disk.UpdatedAt
	if disk.Sessions != nil {
		mc.Sessions = disk.Sessions
	}

	return mc, nil
}

// Lookup returns the cached entry for fullPath only if both modTime and fileSize
// match the stored values. A mismatch means the session file changed and the
// cache entry is stale.
func (mc *MetadataCache) Lookup(fullPath string, modTime time.Time, fileSize int64) (CachedSession, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	entry, ok := mc.Sessions[fullPath]
	if !ok {
		return CachedSession{}, false
	}
	if !entry.ModifiedAt.Equal(modTime) || entry.FileSize != fileSize {
		return CachedSession{}, false
	}
	return entry, true
}

// Set upserts a cache entry for the given session path.
func (mc *MetadataCache) Set(fullPath string, entry CachedSession) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.Sessions[fullPath] = entry
}

// Save persists the cache to disk. It re-reads the current file first and merges
// in-memory updates on top (disk entries are the base, our entries override).
// This allows concurrent processes to each contribute entries without losing
// the other's work. The write is atomic (temp file + rename).
func (mc *MetadataCache) Save() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Create cache directory if needed.
	if err := os.MkdirAll(mc.dir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	path := filepath.Join(mc.dir, cacheFileName(mc.Source))

	// Re-read current disk state as the merge base.
	base := make(map[string]CachedSession)
	if data, err := os.ReadFile(path); err == nil {
		var disk metadataCacheJSON
		if json.Unmarshal(data, &disk) == nil && disk.Sessions != nil {
			base = disk.Sessions
		}
	}

	// Merge: start from disk, overlay our entries.
	merged := make(map[string]CachedSession, len(base)+len(mc.Sessions))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range mc.Sessions {
		merged[k] = v
	}

	out := metadataCacheJSON{
		Version:   1,
		Source:    mc.Source,
		UpdatedAt: time.Now(),
		Sessions:  merged,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	// Atomic write: temp file in same directory, then rename.
	tmp, err := os.CreateTemp(mc.dir, ".sessions-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}

	// Update in-memory state to reflect what was written.
	mc.Sessions = merged
	return nil
}

// MergeInto copies cached fields into meta if the cache entry is fresh
// (mtime + size match). Only fills fields that are currently empty/zero in meta.
// Returns true if a fresh cache entry was found and fields were potentially merged.
func (mc *MetadataCache) MergeInto(meta *SessionMeta) bool {
	mc.mu.RLock()
	entry, ok := mc.Sessions[meta.FullPath]
	mc.mu.RUnlock()
	if !ok {
		return false
	}
	if !entry.ModifiedAt.Equal(meta.ModifiedAt) || entry.FileSize != meta.FileSize {
		return false
	}

	if meta.FirstPrompt == "" {
		meta.FirstPrompt = entry.FirstPrompt
	}
	if meta.Model == "" {
		meta.Model = entry.Model
	}
	if meta.EntryCount == 0 {
		meta.EntryCount = entry.EntryCount
	}
	if meta.GitBranch == "" {
		meta.GitBranch = entry.GitBranch
	}
	if meta.ProjectPath == "" || meta.ProjectPath == "unknown" {
		meta.ProjectPath = entry.ProjectPath
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = entry.CreatedAt
	}

	return true
}
