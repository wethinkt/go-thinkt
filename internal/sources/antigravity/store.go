package antigravity

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
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

const allSessionsCacheKey = "\x00antigravity_all_sessions"

// noWorkspaceID is the project of conversations with no folder at all. It
// is non-empty because the HTTP API rejects an empty project id.
const noWorkspaceID = "no-workspace"

// noWorkspaceName is that project's name and path.
const noWorkspaceName = "(no workspace)"

// Store implements thinkt.Store for Antigravity CLI (~/.gemini/antigravity-cli).
type Store struct {
	baseDir  string
	cacheDir string
	cache    thinkt.StoreCache
	mcOnce   sync.Once
	mc       *thinkt.MetadataCache

	// relatedExclude holds path prefixes never recorded as related folders.
	relatedExclude []string

	mu     sync.Mutex
	extras map[string][]string // conversation id → workspace folders after the first
}

// NewStore creates an Antigravity store rooted at baseDir (default
// ~/.gemini/antigravity-cli).
func NewStore(baseDir string) *Store {
	cacheDir := ""
	if dir, err := config.Dir(); err == nil {
		cacheDir = filepath.Join(dir, "cache")
	}
	return NewStoreWithCacheDir(baseDir, cacheDir)
}

// NewStoreWithCacheDir creates an Antigravity store with an explicit
// metadata cache directory. Primarily useful for testing.
func NewStoreWithCacheDir(baseDir, cacheDir string) *Store {
	if baseDir == "" {
		baseDir, _ = DefaultDir()
	}
	return &Store{
		baseDir:        baseDir,
		cacheDir:       cacheDir,
		relatedExclude: []string{baseDir, os.TempDir(), "/tmp", "/private/tmp"},
	}
}

func (s *Store) metadataCache() *thinkt.MetadataCache {
	s.mcOnce.Do(func() {
		if s.cacheDir == "" {
			s.mc = &thinkt.MetadataCache{Version: 1, Source: thinkt.SourceAntigravity, Sessions: make(map[string]thinkt.CachedSession)}
			return
		}
		s.mc, _ = thinkt.LoadMetadataCache(thinkt.SourceAntigravity, s.cacheDir)
	})
	return s.mc
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("antigravity")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source { return thinkt.SourceAntigravity }

// Workspace returns the workspace this store reads, identified by
// Antigravity's installation_id when present.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()
	workspaceID := hostname
	if data, err := os.ReadFile(filepath.Join(s.baseDir, "installation_id")); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			workspaceID = id
		}
	}
	return thinkt.Workspace{
		ID:       workspaceID,
		Name:     hostname,
		Hostname: hostname,
		Source:   thinkt.SourceAntigravity,
		BasePath: s.baseDir,
	}
}

// ResetCache clears all cached data.
func (s *Store) ResetCache() { s.cache.Clear() }

// projectKey returns the id of the project a session belongs to.
func projectKey(meta thinkt.SessionMeta) string {
	if meta.ProjectPath == "" {
		return noWorkspaceID
	}
	return meta.ProjectPath
}

// ListProjects groups conversations by folder. Subagent conversations count
// in SubagentCount, not SessionCount.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.cache.LoadProjects(func() ([]thinkt.Project, error) {
		sessions, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}
		ws := s.Workspace()
		byID := map[string]*thinkt.Project{}
		for _, sess := range sessions {
			id := projectKey(sess)
			p := byID[id]
			if p == nil {
				p = &thinkt.Project{
					ID:             id,
					Name:           filepath.Base(sess.ProjectPath),
					Path:           sess.ProjectPath,
					DisplayPath:    sess.ProjectPath,
					Source:         thinkt.SourceAntigravity,
					WorkspaceID:    ws.ID,
					SourceBasePath: ws.BasePath,
				}
				if sess.ProjectPath == "" {
					// Not absolute, so it names no real folder; CLI and TUI views
					// show an empty path as "~".
					p.Name, p.Path, p.DisplayPath = noWorkspaceName, noWorkspaceName, noWorkspaceName
				} else if info, err := os.Stat(sess.ProjectPath); err == nil && info.IsDir() {
					p.PathExists = true
				}
				byID[id] = p
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
		projects := make([]thinkt.Project, 0, len(byID))
		for _, p := range byID {
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
		if projects[i].ID == id || (id != "" && projects[i].Path == id) {
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
			if projectKey(sess) == projectID {
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
			scan := s.scan(*sess)
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

// scan runs the parser over a session's transcript with its project, extra
// workspace folders and exclude prefixes.
func (s *Store) scan(meta thinkt.SessionMeta) sessionScan {
	s.mu.Lock()
	extra := s.extras[meta.ID]
	s.mu.Unlock()
	return scanSessionFile(meta.FullPath, meta.ID, meta.ParentSessionID, meta.ProjectPath, extra, s.relatedExclude)
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
	scan := s.scan(*meta)
	scan.applyTo(meta)
	mc.Set(meta.FullPath, scan.cached(*meta))
	return true
}

// GetSessionMeta resolves a conversation id or an absolute transcript path
// under the base directory, and enriches it.
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

// OpenSession returns a streaming reader for a conversation id or
// transcript path.
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
	return &sessionReader{parser: NewParser(f, meta.ID, meta.ParentSessionID), file: f, meta: *meta}, nil
}

// scanSessions lists the brain dirs that have a transcript, joined to the
// summaries db, and resolves each conversation's folder. Listing metadata
// is served from the persistent cache when the transcript is unchanged.
func (s *Store) scanSessions() ([]thinkt.SessionMeta, error) {
	brain := filepath.Join(s.baseDir, "brain")
	dirs, err := os.ReadDir(brain)
	if err != nil {
		return nil, nil
	}
	rows, err := loadSummaries(s.baseDir)
	if err != nil && !os.IsNotExist(err) {
		tuilog.Log.Debug("antigravity: summaries db unreadable", "error", err)
	}
	last := loadLastConversations(s.baseDir)
	ws := s.Workspace()
	mc := s.metadataCache()

	byID := map[string]*thinkt.SessionMeta{}
	var order []string
	extras := map[string][]string{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		id := d.Name()
		path := sessionFile(s.baseDir, id)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		row := rows[id]
		meta := &thinkt.SessionMeta{
			ID:              id,
			FullPath:        path,
			FileSize:        info.Size(),
			ModifiedAt:      row.Modified,
			Source:          thinkt.SourceAntigravity,
			WorkspaceID:     ws.ID,
			Title:           row.Title,
			ParentSessionID: row.ParentID,
		}
		// Background task results are appended after the summary row is
		// written, so the later of the two times is the last change.
		if info.ModTime().After(meta.ModifiedAt) {
			meta.ModifiedAt = info.ModTime().UTC()
		}
		if meta.Title == "" {
			meta.Title = row.Preview
		}
		if len(row.Workspaces) > 1 {
			extras[id] = row.Workspaces[1:]
		}
		if cached, ok := mc.Lookup(path, meta.ModifiedAt, meta.FileSize); ok && cached.ProjectPath != "" {
			meta.ProjectPath = cached.ProjectPath
		} else if len(row.Workspaces) > 0 {
			meta.ProjectPath = row.Workspaces[0]
		} else if row.ParentID == "" {
			cwd := commandCwd(path, id)
			meta.ProjectPath = resolveWorkspace(row, cwd, last, id)
		}
		byID[id] = meta
		order = append(order, id)
	}
	// A subagent's folder is its parent's when its own row names none.
	for _, id := range order {
		meta := byID[id]
		parent := byID[meta.ParentSessionID]
		if meta.ParentSessionID == "" || parent == nil {
			continue
		}
		parent.SubagentCount++
		if meta.ProjectPath == "" {
			meta.ProjectPath = parent.ProjectPath
		}
	}

	sessions := make([]thinkt.SessionMeta, 0, len(order))
	changed := false
	for _, id := range order {
		meta := *byID[id]
		if _, ok := mc.Lookup(meta.FullPath, meta.ModifiedAt, meta.FileSize); !ok {
			mc.Set(meta.FullPath, thinkt.CachedSession{
				ProjectPath:     meta.ProjectPath,
				ModifiedAt:      meta.ModifiedAt,
				FileSize:        meta.FileSize,
				Title:           meta.Title,
				SubagentCount:   meta.SubagentCount,
				ParentSessionID: meta.ParentSessionID,
			})
			changed = true
		}
		sessions = append(sessions, meta)
	}
	if changed {
		_ = mc.Save()
	}
	s.mu.Lock()
	s.extras = extras
	s.mu.Unlock()
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt) })
	return sessions, nil
}

func (s *Store) loadAllSessions() ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions(allSessionsCacheKey, s.scanSessions)
}

// sessionReader streams a transcript's entries, stamping the workspace.
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
	return e, nil
}

func (r *sessionReader) Metadata() thinkt.SessionMeta { return r.meta }

func (r *sessionReader) Close() error { return r.file.Close() }

// WatchConfig returns the watch configuration for Antigravity transcripts:
// brain/<id>/.system_generated/logs is depth 4; split copies under
// logs/chunks are not watched.
func (s *Store) WatchConfig() thinkt.WatchConfig {
	return thinkt.WatchConfig{
		IncludeDirs: []string{"brain"},
		HiddenDirs:  []string{".system_generated"},
		ExcludeDirs: []string{"chunks"},
		MaxDepth:    4,
	}
}
