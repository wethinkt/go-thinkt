package thinkt

import (
	"sync"
	"time"
)

// StoreCache provides project and session caching for Store implementations.
// Stores embed this struct and use its methods to avoid repeated filesystem
// scans during a single process lifetime. All fields are lazily populated
// on first access. Call Reset to force a full rescan.
//
// When TTL is set (via SetTTL), cached data expires and is transparently
// refetched on the next access. With TTL=0 (default), data is cached forever.
type StoreCache struct {
	mu   sync.RWMutex
	name string // identifies this cache in log messages
	ttl  time.Duration

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
	c.ttl = d
}

// SetName sets a label for this cache (used in log messages).
func (c *StoreCache) SetName(name string) {
	c.name = name
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
	return c.projects, c.projectsErr, true
}

// SetProjects stores the project list in the cache.
func (c *StoreCache) SetProjects(projects []Project, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectsCached = true
	c.projectsCachedAt = time.Now()
	c.projects = projects
	c.projectsErr = err
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
	return entry.sessions, entry.err, true
}

// SetSessions stores the session list for a project in the cache.
func (c *StoreCache) SetSessions(projectID string, sessions []SessionMeta, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessions == nil {
		c.sessions = make(map[string]*sessionsCacheEntry)
	}
	c.sessions[projectID] = &sessionsCacheEntry{
		cachedAt: time.Now(),
		sessions: sessions,
		err:      err,
	}
}

// Reset clears all cached data, forcing the next calls to rescan.
func (c *StoreCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectsCached = false
	c.projects = nil
	c.projectsErr = nil
	c.sessions = nil
}
