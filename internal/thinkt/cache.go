package thinkt

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// StoreCache provides project and session caching for Store implementations.
// Stores embed this struct and use its methods to avoid repeated filesystem
// scans during a single process lifetime. All fields are lazily populated
// on first access. Call Clear to force a full rescan.
//
// When TTL is set (via SetTTL), cached data expires and is transparently
// refetched on the next access. With TTL=0 (default), data is cached forever.
//
// Contract:
//   - All interactions go through methods; internal state is never exposed.
//   - Getter methods return defensive copies to prevent accidental mutation.
//   - Setter methods copy input slices before storing.
//   - Invalidation can be done per-project, per-sessions entry, or globally.
//   - Error results are not cached to avoid sticky transient failures.
type StoreCache struct {
	mu   sync.RWMutex
	name string // identifies this cache in log messages
	ttl  time.Duration

	flights singleflight.Group

	projectsCached   bool
	projectsCachedAt time.Time
	projects         []Project
	projectsErr      error

	// sessions is keyed by projectID, populated on demand per project.
	sessions map[string]*sessionsCacheEntry
}

type sessionsCacheEntry struct {
	cachedAt time.Time
	sessions []SessionMeta
	err      error
}

// SetTTL configures the cache time-to-live. Cached data older than d
// is treated as a miss. Zero means cache forever (default).
func (c *StoreCache) SetTTL(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = d
}

// SetName sets a label for this cache (used in log messages).
func (c *StoreCache) SetName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.name = name
}

// LoadProjects returns cached projects if available, otherwise executes loader.
// Concurrent cache misses are deduplicated so only one loader runs at a time.
func (c *StoreCache) LoadProjects(loader func() ([]Project, error)) ([]Project, error) {
	if cached, err, ok := c.GetProjects(); ok {
		return cached, err
	}

	_, err, _ := c.flights.Do("projects", func() (any, error) {
		// Another goroutine may have filled cache while this call was waiting.
		if cached, err, ok := c.GetProjects(); ok {
			return cached, err
		}
		projects, loadErr := loader()
		c.SetProjects(projects, loadErr)
		return nil, loadErr
	})
	if err != nil {
		return nil, err
	}

	cached, cachedErr, ok := c.GetProjects()
	if ok {
		return cached, cachedErr
	}
	return nil, nil
}

// GetProjects returns the cached project list and whether it was cached.
// Returns not-cached if the TTL has expired.
func (c *StoreCache) GetProjects() ([]Project, error, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.projectsCached {
		return nil, nil, false
	}
	if c.ttl > 0 && time.Since(c.projectsCachedAt) > c.ttl {
		return nil, nil, false
	}
	return copyProjects(c.projects), c.projectsErr, true
}

// SetProjects stores the project list in the cache.
func (c *StoreCache) SetProjects(projects []Project, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		// Avoid caching transient scanner/read errors (error poisoning).
		c.projectsCached = false
		c.projectsCachedAt = time.Time{}
		c.projects = nil
		c.projectsErr = nil
		return
	}
	c.projectsCached = true
	c.projectsCachedAt = time.Now()
	c.projects = copyProjects(projects)
	c.projectsErr = err
}

// LoadSessions returns cached sessions if available, otherwise executes loader.
// Concurrent cache misses for the same projectID are deduplicated.
func (c *StoreCache) LoadSessions(projectID string, loader func() ([]SessionMeta, error)) ([]SessionMeta, error) {
	if cached, err, ok := c.GetSessions(projectID); ok {
		return cached, err
	}

	key := "sessions:" + projectID
	_, err, _ := c.flights.Do(key, func() (any, error) {
		// Another goroutine may have filled cache while this call was waiting.
		if cached, err, ok := c.GetSessions(projectID); ok {
			return cached, err
		}
		sessions, loadErr := loader()
		c.SetSessions(projectID, sessions, loadErr)
		return nil, loadErr
	})
	if err != nil {
		return nil, err
	}

	cached, cachedErr, ok := c.GetSessions(projectID)
	if ok {
		return cached, cachedErr
	}
	return nil, nil
}

// GetSessions returns the cached session list for a project and whether it was cached.
// Returns not-cached if the TTL has expired.
func (c *StoreCache) GetSessions(projectID string) ([]SessionMeta, error, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.sessions == nil {
		return nil, nil, false
	}
	entry, ok := c.sessions[projectID]
	if !ok {
		return nil, nil, false
	}
	if c.ttl > 0 && time.Since(entry.cachedAt) > c.ttl {
		return nil, nil, false
	}
	return copySessionMetas(entry.sessions), entry.err, true
}

// SetSessions stores the session list for a project in the cache.
func (c *StoreCache) SetSessions(projectID string, sessions []SessionMeta, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		// Avoid caching transient scanner/read errors (error poisoning).
		if c.sessions == nil {
			return
		}
		delete(c.sessions, projectID)
		if len(c.sessions) == 0 {
			c.sessions = nil
		}
		return
	}
	if c.sessions == nil {
		c.sessions = make(map[string]*sessionsCacheEntry)
	}
	c.sessions[projectID] = &sessionsCacheEntry{
		cachedAt: time.Now(),
		sessions: copySessionMetas(sessions),
		err:      err,
	}
}

// InvalidateProjects clears only cached project list data.
func (c *StoreCache) InvalidateProjects() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectsCached = false
	c.projectsCachedAt = time.Time{}
	c.projects = nil
	c.projectsErr = nil
}

// InvalidateSessions clears the cached session list for a project.
func (c *StoreCache) InvalidateSessions(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessions == nil {
		return
	}
	delete(c.sessions, projectID)
	if len(c.sessions) == 0 {
		c.sessions = nil
	}
}

// Clear clears all cached data, forcing the next calls to rescan.
func (c *StoreCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectsCached = false
	c.projectsCachedAt = time.Time{}
	c.projects = nil
	c.projectsErr = nil
	c.sessions = nil
}

// Reset clears all cached data.
// Deprecated: use Clear.
func (c *StoreCache) Reset() {
	c.Clear()
}

func copyProjects(in []Project) []Project {
	if in == nil {
		return nil
	}
	out := make([]Project, len(in))
	copy(out, in)
	return out
}

func copySessionMetas(in []SessionMeta) []SessionMeta {
	if in == nil {
		return nil
	}
	out := make([]SessionMeta, len(in))
	copy(out, in)
	return out
}
