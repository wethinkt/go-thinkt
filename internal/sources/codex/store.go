package codex

import (
	"bytes"
	"context"
	"encoding/json"
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

const allSessionsCacheKey = "\x00codex_all_sessions"

// Store implements thinkt.Store for Codex CLI sessions.
type Store struct {
	baseDir  string
	cacheDir string // directory for persistent metadata cache
	cache    thinkt.StoreCache
	// relatedExclude holds path prefixes never recorded as related folders.
	relatedExclude []string
	mcOnce         sync.Once
	mc             *thinkt.MetadataCache // lazily loaded
}

// NewStore creates a new Codex store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".codex")
	}
	cacheDir := ""
	if dir, err := config.Dir(); err == nil {
		cacheDir = filepath.Join(dir, "cache")
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir, relatedExclude: thinkt.RelatedExclude(baseDir)}
}

// NewStoreWithCacheDir creates a Codex store with an explicit cache directory.
// This is primarily useful for testing.
func NewStoreWithCacheDir(baseDir, cacheDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".codex")
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir, relatedExclude: thinkt.RelatedExclude(baseDir)}
}

// tracker returns the related-folders tracker for a session of project.
func (s *Store) tracker(project string) *thinkt.RelatedTracker {
	return thinkt.TrackRelated(project, s.relatedExclude)
}

// metadataCache returns the lazily-loaded persistent metadata cache.
func (s *Store) metadataCache() *thinkt.MetadataCache {
	s.mcOnce.Do(func() {
		if s.cacheDir == "" {
			s.mc = &thinkt.MetadataCache{
				Version:  1,
				Source:   thinkt.SourceCodex,
				Sessions: make(map[string]thinkt.CachedSession),
			}
			return
		}
		s.mc, _ = thinkt.LoadMetadataCache(thinkt.SourceCodex, s.cacheDir)
	})
	return s.mc
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("codex")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceCodex
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()
	return thinkt.Workspace{
		ID:       "codex-cli-" + hostname,
		Name:     "Codex CLI",
		Hostname: hostname,
		Source:   thinkt.SourceCodex,
		BasePath: s.baseDir,
	}
}

// ResetCache clears all cached data.
func (s *Store) ResetCache() {
	s.cache.Clear()
}

// ListProjects returns all Codex projects inferred from session metadata.
// Subagent rollouts count toward SubagentCount, not SessionCount.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.cache.LoadProjects(func() ([]thinkt.Project, error) {
		sessions, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}

		ws := s.Workspace()
		projectsMap := make(map[string]*thinkt.Project)
		for _, sess := range sessions {
			projectPath := sess.ProjectPath
			if projectPath == "" {
				projectPath = "unknown"
			}

			p := projectsMap[projectPath]
			if p == nil {
				name := filepath.Base(projectPath)
				if projectPath == "unknown" || name == "" || name == "." || name == "/" {
					name = "unknown"
				}

				pathExists := false
				if projectPath != "unknown" {
					if info, err := os.Stat(projectPath); err == nil && info.IsDir() {
						pathExists = true
					}
				}

				p = &thinkt.Project{
					ID:             projectPath,
					Name:           name,
					Path:           projectPath,
					DisplayPath:    projectPath,
					Source:         thinkt.SourceCodex,
					WorkspaceID:    ws.ID,
					SourceBasePath: ws.BasePath,
					PathExists:     pathExists,
				}
				projectsMap[projectPath] = p
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

		projects := make([]thinkt.Project, 0, len(projectsMap))
		for _, p := range projectsMap {
			projects = append(projects, *p)
		}
		sort.Slice(projects, func(i, j int) bool {
			return projects[i].LastModified.After(projects[j].LastModified)
		})

		return projects, nil
	})
}

// GetProject returns a specific project by ID/path.
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

// ListSessions returns a project's sessions. Subagent rollouts (those with a
// ParentSessionID) are hidden unless WithSubagents is passed. WithEnrichSync
// scans uncached sessions before returning; WithEnrich scans them in the
// background and reports through the callback.
func (s *Store) ListSessions(ctx context.Context, projectID string, opts ...thinkt.ListSessionsOption) ([]thinkt.SessionMeta, error) {
	cfg := thinkt.ResolveListOptions(opts)

	all, err := s.cache.LoadSessions(projectID, func() ([]thinkt.SessionMeta, error) {
		every, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}

		filtered := make([]thinkt.SessionMeta, 0, len(every))
		for _, sess := range every {
			if sess.ProjectPath == projectID {
				filtered = append(filtered, sess)
			}
		}
		return filtered, nil
	})
	if err != nil {
		return nil, err
	}

	mc := s.metadataCache()
	for i := range all {
		mc.MergeInto(&all[i])
	}

	keep := func(m thinkt.SessionMeta) bool {
		return cfg.IncludeSubagents || m.ParentSessionID == ""
	}

	if cfg.EnrichSync {
		changed := false
		for i := range all {
			if all[i].FullPath != "" && keep(all[i]) && s.enrichInto(mc, &all[i]) {
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

// startEnrichment scans the kept sessions whose metadata is stale in a
// goroutine, updating the caches and calling cb with the visible list after
// each session.
func (s *Store) startEnrichment(ctx context.Context, projectID string, all []thinkt.SessionMeta, keep func(thinkt.SessionMeta) bool, mc *thinkt.MetadataCache, cb func(string, []thinkt.SessionMeta)) {
	var staleIdxs []int
	for i, sess := range all {
		if sess.FullPath == "" || !keep(sess) {
			continue
		}
		// A cached fast scan has no EntryCount; only a full scan sets it.
		if sess.EntryCount == 0 {
			staleIdxs = append(staleIdxs, i)
		}
	}
	if len(staleIdxs) == 0 {
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
		for _, idx := range staleIdxs {
			if ctx.Err() != nil {
				return
			}
			sess := &enriched[idx]
			scan := scanSessionFile(sess.FullPath, s.tracker(sess.ProjectPath))
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

// enrichInto fills meta from the cache, or from a file scan that it records
// in the cache without saving. It reports whether the cache changed, so a
// caller enriching many sessions can save once.
func (s *Store) enrichInto(mc *thinkt.MetadataCache, meta *thinkt.SessionMeta) bool {
	// The listing caches its fast head scan, which never sets EntryCount;
	// a hit with EntryCount still zero has not been scanned in full.
	if mc.MergeInto(meta) && meta.EntryCount > 0 {
		return false
	}
	scan := scanSessionFile(meta.FullPath, s.tracker(meta.ProjectPath))
	scan.applyTo(meta)
	mc.Set(meta.FullPath, scan.cached(*meta))
	return true
}

// GetSessionMeta returns session metadata by id or absolute path. Subagent
// ids resolve too, since the lookup covers every rollout.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	ws := s.Workspace()

	var meta *thinkt.SessionMeta
	if filepath.IsAbs(sessionID) {
		if err := thinkt.ValidateSessionPath(sessionID, s.baseDir); err != nil {
			return nil, nil
		}
		if _, err := os.Stat(sessionID); os.IsNotExist(err) {
			return nil, nil
		}
		m, err := s.readSessionMetaFast(sessionID, ws.ID)
		if err != nil || m == nil {
			return nil, err
		}
		if t := loadSessionIndex(s.baseDir)[m.ID]; t != "" {
			m.Title = t
		}
		meta = m
	} else {
		sessions, err := s.loadAllSessions()
		if err != nil {
			return nil, err
		}
		for i := range sessions {
			if sessions[i].ID == sessionID {
				m := sessions[i]
				meta = &m
				break
			}
		}
		if meta == nil {
			return nil, nil
		}
	}

	mc := s.metadataCache()
	if s.enrichInto(mc, meta) {
		_ = mc.Save()
	}
	return meta, nil
}

// LoadSession loads a complete session.
func (s *Store) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	parser := NewParser(f, meta.ID)
	entries := make([]thinkt.Entry, 0, meta.EntryCount)
	for {
		entry, err := parser.NextEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entry.WorkspaceID = meta.WorkspaceID
		if entry.Source == "" {
			entry.Source = thinkt.SourceCodex
		}
		entries = append(entries, *entry)
	}

	return &thinkt.Session{
		Meta:    *meta,
		Entries: entries,
	}, nil
}

// OpenSession returns a streaming reader for a session.
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

	return &sessionReader{
		parser: NewParser(f, meta.ID),
		file:   f,
		meta:   *meta,
	}, nil
}

// scanSessions walks every rollout under sessions/, serving listing
// metadata from the persistent cache when the file is unchanged. After the
// walk it merges titles from session_index.jsonl and counts each parent's
// subagents.
func (s *Store) scanSessions() ([]thinkt.SessionMeta, error) {
	root := filepath.Join(s.baseDir, "sessions")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	ws := s.Workspace()
	mc := s.metadataCache()
	sessions := make([]thinkt.SessionMeta, 0, 128)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if cached, ok := mc.Lookup(path, info.ModTime(), info.Size()); ok {
			// The cache has no field for the rollout's own id. Every
			// Codex rollout name ends with it; a file named otherwise is
			// read for its session_meta line.
			sessionID := rolloutIDFromPath(path)
			if sessionID == "" {
				sessionID = cachedSessionID(path)
			}
			if sessionID == "" {
				sessionID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			}
			projectPath := cached.ProjectPath
			if projectPath == "" {
				projectPath = "unknown"
			}
			createdAt := cached.CreatedAt
			if createdAt.IsZero() {
				createdAt = info.ModTime()
			}
			meta := thinkt.SessionMeta{
				ID:              sessionID,
				ProjectPath:     projectPath,
				FullPath:        path,
				FirstPrompt:     cached.FirstPrompt,
				Model:           cached.Model,
				EntryCount:      cached.EntryCount,
				GitBranch:       cached.GitBranch,
				Title:           cached.Title,
				LastPrompt:      cached.LastPrompt,
				ParentSessionID: cached.ParentSessionID,
				Cost:            cached.Cost,
				FileSize:        info.Size(),
				CreatedAt:       createdAt,
				ModifiedAt:      info.ModTime(),
				Source:          thinkt.SourceCodex,
				WorkspaceID:     ws.ID,
				ChunkCount:      1,
			}
			sessions = append(sessions, meta)
			return nil
		}

		meta, err := s.readSessionMetaFast(path, ws.ID)
		if err != nil || meta == nil {
			return nil
		}
		mc.Set(path, thinkt.CachedSession{
			FirstPrompt:     meta.FirstPrompt,
			Model:           meta.Model,
			EntryCount:      meta.EntryCount,
			GitBranch:       meta.GitBranch,
			ProjectPath:     meta.ProjectPath,
			CreatedAt:       meta.CreatedAt,
			ModifiedAt:      info.ModTime(),
			FileSize:        info.Size(),
			Title:           meta.Title,
			ParentSessionID: meta.ParentSessionID,
		})
		sessions = append(sessions, *meta)
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = mc.Save()

	titles := loadSessionIndex(s.baseDir)
	children := make(map[string]int)
	for i := range sessions {
		if t := titles[sessions[i].ID]; t != "" {
			sessions[i].Title = t
		}
		if sessions[i].ParentSessionID != "" {
			children[sessions[i].ParentSessionID]++
		}
	}
	for i := range sessions {
		sessions[i].SubagentCount = children[sessions[i].ID]
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModifiedAt.After(sessions[j].ModifiedAt)
	})
	return sessions, nil
}

func (s *Store) loadAllSessions() ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions(allSessionsCacheKey, func() ([]thinkt.SessionMeta, error) {
		return s.scanSessions()
	})
}

// readSessionMetaFast builds listing metadata from the head of a rollout:
// the whole session_meta record plus the first prompt and model from the
// first 200 lines.
func (s *Store) readSessionMetaFast(path, workspaceID string) (*thinkt.SessionMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	meta := &thinkt.SessionMeta{
		ID:          strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		FullPath:    path,
		FileSize:    info.Size(),
		ModifiedAt:  info.ModTime(),
		Source:      thinkt.SourceCodex,
		WorkspaceID: workspaceID,
		ChunkCount:  1,
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	const maxLines = 200
	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxScannerCapacity)
	lineCount := 0
	providerModel := "" // session_meta.model_provider, used only when no turn names a model
	for scanner.Scan() {
		lineCount++
		if lineCount > maxLines {
			break
		}

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		h, _ := ClassifyLine(line)
		if h.Type == "" {
			continue
		}
		if meta.CreatedAt.IsZero() {
			meta.CreatedAt = parseTimestamp(h.Timestamp)
		}

		switch h.Key() {
		case "session_meta":
			var r sessionMetaRecord
			if err := json.Unmarshal(h.Payload, &r); err != nil {
				continue
			}
			if r.ID != "" {
				meta.ID = r.ID
			}
			meta.ParentSessionID = r.parentThreadID()
			if meta.Title == "" {
				meta.Title = r.subagentTitle()
			}
			if r.CWD != "" && meta.ProjectPath == "" {
				meta.ProjectPath = r.CWD
			}
			if thinkt.IsRealModel(r.Model) {
				meta.Model = r.Model
			} else if r.ModelProvider != "" {
				providerModel = r.ModelProvider
			}
			if r.Git.Branch != "" {
				meta.GitBranch = r.Git.Branch
			}
			if meta.CreatedAt.IsZero() {
				meta.CreatedAt = parseTimestamp(r.Timestamp)
			}

		case "turn_context":
			var tc struct {
				CWD   string `json:"cwd"`
				Model string `json:"model"`
			}
			if err := json.Unmarshal(h.Payload, &tc); err != nil {
				continue
			}
			if meta.ProjectPath == "" {
				meta.ProjectPath = tc.CWD
			}
			if !thinkt.IsRealModel(meta.Model) && thinkt.IsRealModel(tc.Model) {
				meta.Model = tc.Model
			}

		case "event_msg/user_message":
			if meta.FirstPrompt == "" {
				var p struct {
					Message string `json:"message"`
				}
				if json.Unmarshal(h.Payload, &p) == nil {
					meta.FirstPrompt = strings.TrimSpace(p.Message)
				}
			}

		case "response_item/message":
			if meta.FirstPrompt != "" || h.Role != "user" {
				continue
			}
			if class, kind := userMessageClass(h); class == thinkt.ClassTurn && kind == thinkt.KindNone {
				meta.FirstPrompt = userPromptText(h.Payload)
			}
		}

		if meta.ProjectPath != "" && meta.FirstPrompt != "" && thinkt.IsRealModel(meta.Model) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.ModifiedAt
	}
	if meta.ProjectPath == "" {
		meta.ProjectPath = "unknown"
	}
	if meta.Model == "" {
		meta.Model = providerModel
	}
	return meta, nil
}

// cachedSessionID reads the session_meta id from the head of a rollout for a
// cache hit, which stores no id of its own.
func cachedSessionID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxScannerCapacity)
	for i := 0; scanner.Scan() && i < 5; i++ {
		h, _ := ClassifyLine(bytes.TrimSpace(scanner.Bytes()))
		if h.Key() != "session_meta" {
			continue
		}
		var r sessionMetaRecord
		if json.Unmarshal(h.Payload, &r) == nil {
			return r.ID
		}
	}
	return ""
}

type sessionReader struct {
	parser *Parser
	file   io.Closer
	meta   thinkt.SessionMeta
}

func (r *sessionReader) ReadNext() (*thinkt.Entry, error) {
	entry, err := r.parser.NextEntry()
	if err != nil {
		return nil, err
	}
	entry.WorkspaceID = r.meta.WorkspaceID
	if entry.Source == "" {
		entry.Source = thinkt.SourceCodex
	}
	return entry, nil
}

func (r *sessionReader) Metadata() thinkt.SessionMeta {
	return r.meta
}

func (r *sessionReader) Close() error {
	return r.file.Close()
}

// WatchConfig returns the watch configuration for Codex session files.
func (s *Store) WatchConfig() thinkt.WatchConfig {
	return thinkt.WatchConfig{
		IncludeDirs: []string{"sessions"},
		MaxDepth:    5,
	}
}
