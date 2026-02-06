package thinkt

// StoreCache provides project and session caching for Store implementations.
// Stores embed this struct and use its methods to avoid repeated filesystem
// scans during a single process lifetime. All fields are lazily populated
// on first access. Call Reset to force a full rescan.
type StoreCache struct {
	projectsCached bool
	projects       []Project
	projectsErr    error

	// sessions is keyed by projectID, populated on demand per project.
	sessions map[string]*sessionsCacheEntry
}

type sessionsCacheEntry struct {
	sessions []SessionMeta
	err      error
}

// GetProjects returns the cached project list and whether it was cached.
func (c *StoreCache) GetProjects() ([]Project, error, bool) {
	return c.projects, c.projectsErr, c.projectsCached
}

// SetProjects stores the project list in the cache.
func (c *StoreCache) SetProjects(projects []Project, err error) {
	c.projectsCached = true
	c.projects = projects
	c.projectsErr = err
}

// GetSessions returns the cached session list for a project and whether it was cached.
func (c *StoreCache) GetSessions(projectID string) ([]SessionMeta, error, bool) {
	if c.sessions == nil {
		return nil, nil, false
	}
	entry, ok := c.sessions[projectID]
	if !ok {
		return nil, nil, false
	}
	return entry.sessions, entry.err, true
}

// SetSessions stores the session list for a project in the cache.
func (c *StoreCache) SetSessions(projectID string, sessions []SessionMeta, err error) {
	if c.sessions == nil {
		c.sessions = make(map[string]*sessionsCacheEntry)
	}
	c.sessions[projectID] = &sessionsCacheEntry{sessions: sessions, err: err}
}

// Reset clears all cached data, forcing the next calls to rescan.
func (c *StoreCache) Reset() {
	c.projectsCached = false
	c.projects = nil
	c.projectsErr = nil
	c.sessions = nil
}
