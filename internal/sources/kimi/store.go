package kimi

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const allSessionsCacheKey = "\x00kimi_all_sessions"

// Store implements thinkt.Store for Kimi Code (~/.kimi-code).
type Store struct {
	baseDir  string
	cacheDir string
	cache    thinkt.StoreCache
	// relatedExclude holds path prefixes never recorded as related folders.
	relatedExclude []string
	mcOnce         sync.Once
	mc             *thinkt.MetadataCache
}

// NewStore creates a Kimi Code store rooted at baseDir (default ~/.kimi-code).
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		baseDir, _ = defaultDir()
	}
	cacheDir := ""
	if dir, err := config.Dir(); err == nil {
		cacheDir = filepath.Join(dir, "cache")
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir, relatedExclude: thinkt.RelatedExclude(baseDir)}
}

// NewStoreWithCacheDir creates a Kimi Code store with an explicit metadata
// cache directory. Primarily useful for testing.
func NewStoreWithCacheDir(baseDir, cacheDir string) *Store {
	if baseDir == "" {
		baseDir, _ = defaultDir()
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir, relatedExclude: thinkt.RelatedExclude(baseDir)}
}

// tracker returns the related-folders tracker for a session of project, nil
// when the project is the "unknown" placeholder.
func (s *Store) tracker(project string) *thinkt.RelatedTracker {
	if project == "unknown" {
		return nil
	}
	return thinkt.TrackRelated(project, s.relatedExclude)
}

func (s *Store) metadataCache() *thinkt.MetadataCache {
	s.mcOnce.Do(func() {
		if s.cacheDir == "" {
			s.mc = &thinkt.MetadataCache{Version: 1, Source: thinkt.SourceKimi, Sessions: make(map[string]thinkt.CachedSession)}
			return
		}
		s.mc, _ = thinkt.LoadMetadataCache(thinkt.SourceKimi, s.cacheDir)
	})
	return s.mc
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("kimi")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source { return thinkt.SourceKimi }

// Workspace returns the workspace this store reads, identified by Kimi's
// device_id when present.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()
	workspaceID := hostname
	if data, err := os.ReadFile(filepath.Join(s.baseDir, "device_id")); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			workspaceID = id
		}
	}
	return thinkt.Workspace{
		ID:       workspaceID,
		Name:     hostname,
		Hostname: hostname,
		Source:   thinkt.SourceKimi,
		BasePath: s.baseDir,
	}
}

// ResetCache clears all cached data.
func (s *Store) ResetCache() { s.cache.Clear() }

// ListProjects groups sessions by work-dir path. Subagent sessions count in
// SubagentCount, not SessionCount.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.cache.LoadProjects(func() ([]thinkt.Project, error) {
		sessions, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}
		ws := s.Workspace()
		byPath := map[string]*thinkt.Project{}
		for _, sess := range sessions {
			path := sess.ProjectPath
			if path == "" {
				path = "unknown"
			}
			p := byPath[path]
			if p == nil {
				name := filepath.Base(path)
				if path == "unknown" || name == "" || name == "." || name == "/" {
					name = "unknown"
				}
				exists := false
				if path != "unknown" {
					if info, err := os.Stat(path); err == nil && info.IsDir() {
						exists = true
					}
				}
				p = &thinkt.Project{
					ID:             path,
					Name:           name,
					Path:           path,
					DisplayPath:    path,
					Source:         thinkt.SourceKimi,
					WorkspaceID:    ws.ID,
					SourceBasePath: ws.BasePath,
					PathExists:     exists,
				}
				byPath[path] = p
			}
			if sess.ParentSessionID == "" {
				p.SessionCount++
			} else {
				p.SubagentCount++
			}
			if sess.ModifiedAt.After(p.LastModified) {
				p.LastModified = sess.ModifiedAt
			}
		}
		projects := make([]thinkt.Project, 0, len(byPath))
		for _, p := range byPath {
			projects = append(projects, *p)
		}
		sort.Slice(projects, func(i, j int) bool { return projects[i].LastModified.After(projects[j].LastModified) })
		return projects, nil
	})
}

// GetProject returns a project by id or path.
func (s *Store) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].ID == id || projects[i].Path == id {
			return &projects[i], nil
		}
	}
	return nil, nil
}

// ListSessions returns a project's sessions. Subagent sessions are hidden
// unless WithSubagents is passed. WithEnrichSync scans unscanned sessions
// before returning; WithEnrich scans them in the background.
func (s *Store) ListSessions(ctx context.Context, projectID string, opts ...thinkt.ListSessionsOption) ([]thinkt.SessionMeta, error) {
	cfg := thinkt.ResolveListOptions(opts)
	all, err := s.cache.LoadSessions(projectID, func() ([]thinkt.SessionMeta, error) {
		every, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}
		out := make([]thinkt.SessionMeta, 0, len(every))
		for _, sess := range every {
			if sess.ProjectPath == projectID {
				out = append(out, sess)
			}
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	mc := s.metadataCache()
	for i := range all {
		mc.MergeInto(&all[i])
	}
	keep := func(m thinkt.SessionMeta) bool { return cfg.IncludeSubagents || m.ParentSessionID == "" }

	if cfg.EnrichSync {
		changed := false
		for i := range all {
			if keep(all[i]) && s.enrichInto(mc, &all[i]) {
				changed = true
			}
		}
		if changed {
			_ = mc.Save()
		}
		s.cache.SetSessions(projectID, all, nil)
	}

	sessions := make([]thinkt.SessionMeta, 0, len(all))
	for _, m := range all {
		if keep(m) {
			sessions = append(sessions, m)
		}
	}
	if cfg.EnrichCallback != nil {
		s.startEnrichment(ctx, projectID, all, keep, mc, cfg.EnrichCallback)
	}
	return sessions, nil
}

func (s *Store) startEnrichment(ctx context.Context, projectID string, all []thinkt.SessionMeta, keep func(thinkt.SessionMeta) bool, mc *thinkt.MetadataCache, cb func(string, []thinkt.SessionMeta)) {
	var stale []int
	for i, sess := range all {
		if sess.FullPath != "" && keep(sess) && sess.EntryCount == 0 {
			stale = append(stale, i)
		}
	}
	if len(stale) == 0 {
		return
	}
	enriched := make([]thinkt.SessionMeta, len(all))
	copy(enriched, all)
	visible := func() []thinkt.SessionMeta {
		out := make([]thinkt.SessionMeta, 0, len(enriched))
		for _, m := range enriched {
			if keep(m) {
				out = append(out, m)
			}
		}
		return out
	}
	go func() {
		for _, idx := range stale {
			if ctx.Err() != nil {
				return
			}
			sess := &enriched[idx]
			scan := scanSessionFile(sess.FullPath, sess.ID, s.tracker(sess.ProjectPath))
			scan.applyTo(sess)
			mc.Set(sess.FullPath, scan.cached(*sess))
			_ = mc.Save()
			result := make([]thinkt.SessionMeta, len(enriched))
			copy(result, enriched)
			s.cache.SetSessions(projectID, result, nil)
			cb(projectID, visible())
		}
	}()
}

// enrichInto fills meta from the cache, or from a parser scan it records in
// the cache without saving. A cache hit with EntryCount zero comes from the
// listing and has not been scanned yet. It reports whether the cache changed.
func (s *Store) enrichInto(mc *thinkt.MetadataCache, meta *thinkt.SessionMeta) bool {
	if meta.FullPath == "" {
		return false
	}
	if mc.MergeInto(meta) && meta.EntryCount > 0 {
		return false
	}
	scan := scanSessionFile(meta.FullPath, meta.ID, s.tracker(meta.ProjectPath))
	scan.applyTo(meta)
	mc.Set(meta.FullPath, scan.cached(*meta))
	return true
}

// GetSessionMeta resolves a session id, a "<session>/<agent>" id, or an
// absolute wire path under the base directory, and enriches it.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	id := sessionID
	if filepath.IsAbs(sessionID) {
		if err := thinkt.ValidateSessionPath(sessionID, s.baseDir); err != nil {
			return nil, nil
		}
		id = sessionIDForPath(sessionID)
		if id == "" {
			return nil, nil
		}
	}
	sessions, err := s.loadAllSessions()
	if err != nil {
		return nil, err
	}
	var meta *thinkt.SessionMeta
	for i := range sessions {
		if sessions[i].ID == id {
			m := sessions[i]
			meta = &m
			break
		}
	}
	if meta == nil {
		return nil, nil
	}
	mc := s.metadataCache()
	if s.enrichInto(mc, meta) {
		_ = mc.Save()
	}
	return meta, nil
}

// LoadSession loads a complete session.
func (s *Store) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	reader, err := s.OpenSession(ctx, sessionID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	meta := reader.Metadata()
	entries := make([]thinkt.Entry, 0, meta.EntryCount)
	for {
		e, err := reader.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, *e)
	}
	return &thinkt.Session{Meta: meta, Entries: entries}, nil
}

// OpenSession returns a streaming reader for a session id or wire path.
func (s *Store) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, os.ErrNotExist
	}
	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}
	return &sessionReader{parser: NewParser(f, meta.ID), file: f, meta: *meta}, nil
}

// scanSessions walks sessions/wd_*/<session>/state.json, serving listing
// metadata from the persistent cache when the main wire is unchanged.
func (s *Store) scanSessions() ([]thinkt.SessionMeta, error) {
	root := filepath.Join(s.baseDir, "sessions")
	if _, err := os.Stat(root); err != nil {
		return nil, nil
	}
	states, err := filepath.Glob(filepath.Join(root, "*", "*", "state.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(states)
	ws := s.Workspace()
	index := loadSessionIndex(s.baseDir)
	workspaces := loadWorkspaces(s.baseDir)
	mc := s.metadataCache()
	var sessions []thinkt.SessionMeta
	for _, statePath := range states {
		sessionDir := filepath.Dir(statePath)
		wdName := filepath.Base(filepath.Dir(sessionDir))
		for _, meta := range sessionsFromDir(s.baseDir, sessionDir, wdName, ws.ID, index, workspaces) {
			if _, ok := mc.Lookup(meta.FullPath, meta.ModifiedAt, meta.FileSize); !ok {
				mc.Set(meta.FullPath, thinkt.CachedSession{
					ProjectPath:     meta.ProjectPath,
					CreatedAt:       meta.CreatedAt,
					ModifiedAt:      meta.ModifiedAt,
					FileSize:        meta.FileSize,
					Title:           meta.Title,
					LastPrompt:      meta.LastPrompt,
					SubagentCount:   meta.SubagentCount,
					ParentSessionID: meta.ParentSessionID,
				})
			}
			sessions = append(sessions, meta)
		}
	}
	_ = mc.Save()
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt) })
	return sessions, nil
}

func (s *Store) loadAllSessions() ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions(allSessionsCacheKey, s.scanSessions)
}

// sessionReader streams a wire file's entries, stamping provenance and
// giving timestamp-less (protocol 1.0) entries the session's creation time.
type sessionReader struct {
	parser *Parser
	file   io.Closer
	meta   thinkt.SessionMeta
}

func (r *sessionReader) ReadNext() (*thinkt.Entry, error) {
	e, err := r.parser.NextEntry()
	if err != nil {
		return nil, err
	}
	e.WorkspaceID = r.meta.WorkspaceID
	if e.Timestamp.IsZero() {
		e.Timestamp = r.meta.CreatedAt
	}
	resolveBlobs(e.ContentBlocks, filepath.Dir(r.meta.FullPath))
	return e, nil
}

func (r *sessionReader) Metadata() thinkt.SessionMeta { return r.meta }

func (r *sessionReader) Close() error { return r.file.Close() }

// WatchConfig returns the watch configuration for Kimi Code wire files.
func (s *Store) WatchConfig() thinkt.WatchConfig {
	return thinkt.WatchConfig{IncludeDirs: []string{"sessions"}, MaxDepth: 5}
}
