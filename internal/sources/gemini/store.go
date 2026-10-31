package gemini

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

// Store implements thinkt.Store for Gemini CLI sessions.
type Store struct {
	baseDir  string
	cacheDir string // directory for persistent metadata cache
	cache    thinkt.StoreCache
	mcOnce   sync.Once
	mc       *thinkt.MetadataCache // lazily loaded

	headerMu  sync.Mutex
	headerIDs map[string]string // session ID → the file's sessionId (resume, links)
}

// NewStore creates a new Gemini store.
func NewStore(baseDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".gemini")
	}
	cacheDir := ""
	if dir, err := config.Dir(); err == nil {
		cacheDir = filepath.Join(dir, "cache")
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir, headerIDs: map[string]string{}}
}

// NewStoreWithCacheDir creates a Gemini store with an explicit cache directory.
// This is primarily useful for testing.
func NewStoreWithCacheDir(baseDir, cacheDir string) *Store {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".gemini")
	}
	return &Store{baseDir: baseDir, cacheDir: cacheDir, headerIDs: map[string]string{}}
}

// metadataCache returns the lazily-loaded persistent metadata cache.
func (s *Store) metadataCache() *thinkt.MetadataCache {
	s.mcOnce.Do(func() {
		if s.cacheDir == "" {
			s.mc = &thinkt.MetadataCache{
				Version:  1,
				Source:   thinkt.SourceGemini,
				Sessions: make(map[string]thinkt.CachedSession),
			}
			return
		}
		s.mc, _ = thinkt.LoadMetadataCache(thinkt.SourceGemini, s.cacheDir)
	})
	return s.mc
}

// SetCacheTTL sets the cache time-to-live for this store.
func (s *Store) SetCacheTTL(d time.Duration) {
	s.cache.SetName("gemini")
	s.cache.SetTTL(d)
}

// Source returns the store type.
func (s *Store) Source() thinkt.Source {
	return thinkt.SourceGemini
}

// Workspace returns information about this store's workspace.
func (s *Store) Workspace() thinkt.Workspace {
	hostname, _ := os.Hostname()
	workspaceID := hostname

	// Try to read installation_id
	installIDPath := filepath.Join(s.baseDir, "installation_id")
	if data, err := os.ReadFile(installIDPath); err == nil {
		workspaceID = strings.TrimSpace(string(data))
	}

	return thinkt.Workspace{
		ID:       workspaceID,
		Name:     hostname,
		Hostname: hostname,
		Source:   thinkt.SourceGemini,
		BasePath: s.baseDir,
	}
}

// ListProjects returns one project per projectHash: a leftover hash folder
// and its slug folder are the same project.
func (s *Store) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return s.cache.LoadProjects(func() ([]thinkt.Project, error) {
		groups, order, err := s.projectGroups()
		if err != nil {
			return nil, err
		}
		ws := s.Workspace()
		var projects []thinkt.Project
		for _, hash := range order {
			folders := groups[hash]
			files := s.scanProject(hash, folders)
			var count, subs int
			var lastMod time.Time
			for _, f := range files {
				if f.meta.ParentSessionID != "" {
					subs++
					continue
				}
				count++
				if f.meta.ModifiedAt.After(lastMod) {
					lastMod = f.meta.ModifiedAt
				}
			}
			if count == 0 {
				continue
			}
			p := thinkt.Project{
				ID:             hash,
				Name:           projectName(folders),
				Path:           folders[0].Path,
				DisplayPath:    folders[0].Path,
				SessionCount:   count,
				SubagentCount:  subs,
				LastModified:   lastMod,
				Source:         thinkt.SourceGemini,
				WorkspaceID:    ws.ID,
				SourceBasePath: ws.BasePath,
			}
			if p.Path == "" {
				p.DisplayPath = "gemini://" + hash[:min(len(hash), 8)]
			} else if _, err := os.Stat(p.Path); err == nil {
				p.PathExists = true
			}
			projects = append(projects, p)
		}
		return projects, nil
	})
}

// projectGroups groups the tmp/ folders by projectHash. Within a group the
// slug folder comes first; groups keep the order of their first folder.
func (s *Store) projectGroups() (map[string][]projectFolder, []string, error) {
	folders, err := scanProjectFolders(s.baseDir)
	if err != nil {
		return nil, nil, err
	}
	groups := map[string][]projectFolder{}
	var order []string
	for _, f := range folders {
		if _, ok := groups[f.Hash]; !ok {
			order = append(order, f.Hash)
		}
		groups[f.Hash] = append(groups[f.Hash], f)
	}
	for h, fs := range groups {
		sort.SliceStable(fs, func(i, j int) bool { return fs[i].IsSlug && !fs[j].IsSlug })
		path := ""
		for _, f := range fs {
			if f.Path != "" {
				path = f.Path
				break
			}
		}
		for i := range fs {
			fs[i].Path = path
		}
		groups[h] = fs
	}
	return groups, order, nil
}

// projectName is the base name of the project path, else the slug folder,
// else the first 8 characters of the hash.
func projectName(folders []projectFolder) string {
	if p := folders[0].Path; p != "" && p != "/" {
		return filepath.Base(p)
	}
	for _, f := range folders {
		if f.IsSlug {
			return f.Dir
		}
	}
	return folders[0].Hash[:min(len(folders[0].Hash), 8)]
}

// sessionFile is one listed session file of a project.
type sessionFile struct {
	meta     thinkt.SessionMeta
	headerID string
	jsonl    bool
	slug     bool
	key      string // path relative to chats/, the dedupe key across folders
}

// scanProject lists every session file of one project, main and subagent:
// one copy per file name across the project's folders, and no .json file a
// .jsonl conversion supersedes.
func (s *Store) scanProject(hash string, folders []projectFolder) []sessionFile {
	ws := s.Workspace()
	mc := s.metadataCache()
	var files []sessionFile
	for _, folder := range folders {
		chats := filepath.Join(s.baseDir, "tmp", folder.Dir, "chats")
		entries, err := os.ReadDir(chats)
		if err != nil {
			continue
		}
		for _, e := range entries {
			full := filepath.Join(chats, e.Name())
			switch {
			case e.IsDir():
				continue // subagent folders: below, once parents are known
			case strings.HasSuffix(e.Name(), ".jsonl"):
				if f, ok := s.jsonlFile(mc, full, strings.TrimSuffix(e.Name(), ".jsonl"), ""); ok {
					f.slug, f.key = folder.IsSlug, e.Name()
					files = append(files, f)
				}
			case strings.HasSuffix(e.Name(), ".json"):
				if f, ok := s.jsonFile(mc, full, ws.ID); ok {
					f.slug, f.key = folder.IsSlug, e.Name()
					files = append(files, f)
				}
			}
		}
	}

	// Subagents live in chats/<parent header sessionId>/.
	parentStem := map[string]string{}
	for _, f := range files {
		if f.jsonl {
			parentStem[f.headerID] = f.meta.ID
		}
	}
	for _, folder := range folders {
		chats := filepath.Join(s.baseDir, "tmp", folder.Dir, "chats")
		dirs, _ := os.ReadDir(chats)
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			parent := parentStem[d.Name()]
			if parent == "" {
				parent = d.Name()
			}
			subs, _ := filepath.Glob(filepath.Join(chats, d.Name(), "*.jsonl"))
			for _, full := range subs {
				name := filepath.Base(full)
				if f, ok := s.jsonlFile(mc, full, strings.TrimSuffix(name, ".jsonl"), parent); ok {
					f.slug, f.key = folder.IsSlug, d.Name()+"/"+name
					files = append(files, f)
				}
			}
		}
	}
	_ = mc.Save()

	files = dedupeFolders(files)
	files = s.dropSuperseded(files)

	path := ""
	if len(folders) > 0 {
		path = folders[0].Path
	}
	children := map[string]int{}
	for _, f := range files {
		if f.meta.ParentSessionID != "" {
			children[f.meta.ParentSessionID]++
		}
	}
	s.headerMu.Lock()
	for i := range files {
		files[i].meta.ProjectPath = path
		files[i].meta.WorkspaceID = ws.ID
		if n := children[files[i].meta.ID]; n > 0 {
			files[i].meta.SubagentCount = n
		}
		s.headerIDs[files[i].meta.ID] = files[i].headerID
	}
	s.headerMu.Unlock()
	return files
}

// jsonlFile reads one .jsonl session's metadata through the cache.
func (s *Store) jsonlFile(mc *thinkt.MetadataCache, full, stem, parent string) (sessionFile, bool) {
	info, err := os.Stat(full)
	if err != nil {
		return sessionFile{}, false
	}
	if cached, ok := mc.Lookup(full, info.ModTime(), info.Size()); ok {
		headerID := jsonlHeaderID(full)
		if headerID == "" {
			return sessionFile{}, false
		}
		id := stem
		if parent != "" {
			id = parent + "/" + stem
		}
		meta := thinkt.SessionMeta{
			ID: id, ParentSessionID: cached.ParentSessionID, FullPath: full, FileSize: info.Size(),
			CreatedAt: cached.CreatedAt, ModifiedAt: info.ModTime(), Title: cached.Title,
			FirstPrompt: cached.FirstPrompt, Model: cached.Model, EntryCount: cached.EntryCount,
			Cost: cached.Cost, Source: thinkt.SourceGemini, ChunkCount: 1,
		}
		return sessionFile{meta: meta, headerID: headerID, jsonl: true}, true
	}
	scan, err := scanJSONL(full, stem, parent)
	if err != nil || scan == nil {
		if err != nil {
			tuilog.Log.Debug("gemini: skip session file", "path", full, "error", err)
		}
		return sessionFile{}, false
	}
	m := scan.Meta
	// The metadata cache is keyed by file mtime; lastUpdated is written on
	// every append, so the later of the two is the same on a hit and a miss.
	if info.ModTime().After(m.ModifiedAt) {
		m.ModifiedAt = info.ModTime()
	}
	mc.Set(full, thinkt.CachedSession{
		FirstPrompt: m.FirstPrompt, Model: m.Model, EntryCount: m.EntryCount, Title: m.Title,
		ParentSessionID: m.ParentSessionID, Cost: m.Cost, CreatedAt: m.CreatedAt,
		ModifiedAt: info.ModTime(), FileSize: info.Size(),
	})
	return sessionFile{meta: m, headerID: scan.HeaderID, jsonl: true}, true
}

// jsonlHeaderID reads the sessionId from a .jsonl file's first line.
func jsonlHeaderID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	line, _ := bufio.NewReaderSize(f, 4096).ReadBytes('\n')
	var head replayHeader
	if json.Unmarshal(line, &head) != nil || head.ProjectHash == "" {
		return ""
	}
	return head.SessionID
}

// jsonFile reads one .json session's metadata through the cache.
func (s *Store) jsonFile(mc *thinkt.MetadataCache, full, wsID string) (sessionFile, bool) {
	info, err := os.Stat(full)
	if err != nil {
		return sessionFile{}, false
	}
	stem := strings.TrimSuffix(filepath.Base(full), ".json")
	if cached, ok := mc.Lookup(full, info.ModTime(), info.Size()); ok {
		meta := thinkt.SessionMeta{
			ID: stem, FullPath: full, FirstPrompt: cached.FirstPrompt, Model: cached.Model,
			EntryCount: cached.EntryCount, GitBranch: cached.GitBranch, FileSize: info.Size(),
			CreatedAt: cached.CreatedAt, ModifiedAt: info.ModTime(),
			Source: thinkt.SourceGemini, ChunkCount: 1,
		}
		return sessionFile{meta: meta}, true
	}
	meta, headerID, err := s.readSessionMeta(full, info.Size(), info.ModTime(), wsID)
	if err != nil {
		tuilog.Log.Error("failed to read gemini session meta", "path", full, "error", err)
		return sessionFile{}, false
	}
	mc.Set(full, thinkt.CachedSession{
		FirstPrompt: meta.FirstPrompt, Model: meta.Model, EntryCount: meta.EntryCount,
		GitBranch: meta.GitBranch, CreatedAt: meta.CreatedAt, ModifiedAt: info.ModTime(),
		FileSize: info.Size(),
	})
	return sessionFile{meta: *meta, headerID: headerID}, true
}

// jsonHeaderID reads a .json session's sessionId without decoding its
// messages.
func jsonHeaderID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	return jsonTopString(f, "sessionId")
}

// jsonIDs reads a .json session's sessionId and the id of its last message.

func jsonIDs(path string) (sessionID, lastMessageID string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	var doc struct {
		SessionID string `json:"sessionId"`
		Messages  []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if json.NewDecoder(f).Decode(&doc) != nil {
		return "", ""
	}
	if n := len(doc.Messages); n > 0 {
		lastMessageID = doc.Messages[n-1].ID
	}
	return doc.SessionID, lastMessageID
}

// dedupeFolders keeps one copy of each file name: the slug folder's, unless
// the copies differ and the other one is newer.
func dedupeFolders(files []sessionFile) []sessionFile {
	best := map[string]int{}
	var out []sessionFile
	for _, f := range files {
		i, seen := best[f.key]
		if !seen {
			best[f.key] = len(out)
			out = append(out, f)
			continue
		}
		cur := out[i]
		same := cur.meta.FileSize == f.meta.FileSize && cur.meta.ModifiedAt.Equal(f.meta.ModifiedAt)
		switch {
		case same && f.slug && !cur.slug:
			out[i] = f
		case !same && f.meta.ModifiedAt.After(cur.meta.ModifiedAt):
			out[i] = f
		}
	}
	return out
}

// dropSuperseded removes a .json file whose conversion is a .jsonl file of
// the same project: the same sessionId, with the .json's last message in
// the .jsonl. Another .json under that sessionId is a different branch and
// stays.
func (s *Store) dropSuperseded(files []sessionFile) []sessionFile {
	byHeader := map[string][]int{}
	for i, f := range files {
		if f.jsonl && f.meta.ParentSessionID == "" && f.headerID != "" {
			byHeader[f.headerID] = append(byHeader[f.headerID], i)
		}
	}
	if len(byHeader) == 0 {
		return files
	}
	for i := range files {
		if !files[i].jsonl && files[i].headerID == "" {
			files[i].headerID = jsonHeaderID(files[i].meta.FullPath)
		}
	}
	msgIDs := map[int]map[string]bool{}
	out := files[:0]
	for _, f := range files {
		if f.jsonl || byHeader[f.headerID] == nil {
			out = append(out, f)
			continue
		}
		_, last := jsonIDs(f.meta.FullPath)
		superseded := false
		for _, i := range byHeader[f.headerID] {
			if msgIDs[i] == nil {
				msgIDs[i] = jsonlMessageIDs(files[i].meta.FullPath)
			}
			if last != "" && msgIDs[i][last] {
				superseded = true
				break
			}
		}
		if !superseded {
			out = append(out, f)
		}
	}
	return out
}

// jsonlMessageIDs returns the ids in a .jsonl session's final message list.
func jsonlMessageIDs(path string) map[string]bool {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	r, err := replay(f)
	if err != nil || r == nil {
		return nil
	}
	ids := make(map[string]bool, len(r.Messages))
	for _, m := range r.Messages {
		ids[m.ID] = true
	}
	return ids
}

// allSessions lists one project's sessions, subagents included, through
// the store cache.
func (s *Store) allSessions(hash string) ([]thinkt.SessionMeta, error) {
	return s.cache.LoadSessions("all\x00"+hash, func() ([]thinkt.SessionMeta, error) {
		groups, _, err := s.projectGroups()
		if err != nil {
			return nil, err
		}
		folders, ok := groups[hash]
		if !ok {
			return nil, nil
		}
		var out []thinkt.SessionMeta
		for _, f := range s.scanProject(hash, folders) {
			out = append(out, f.meta)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ModifiedAt.After(out[j].ModifiedAt) })
		return out, nil
	})
}

// ListSessions returns a project's sessions; subagent sessions only with
// thinkt.WithSubagents(). Metadata is complete from the listing; a
// WithEnrich callback fires once with the list.
func (s *Store) ListSessions(ctx context.Context, projectID string, opts ...thinkt.ListSessionsOption) ([]thinkt.SessionMeta, error) {
	cfg := thinkt.ResolveListOptions(opts)
	all, err := s.allSessions(projectID)
	if err != nil {
		return nil, err
	}
	var sessions []thinkt.SessionMeta
	for _, m := range all {
		if m.ParentSessionID != "" && !cfg.IncludeSubagents {
			continue
		}
		sessions = append(sessions, m)
	}
	if cfg.EnrichCallback != nil && len(sessions) > 0 {
		cfg.EnrichCallback(projectID, sessions)
	}
	return sessions, nil
}

// readSessionMeta reads a .json session's metadata, its ID being the file
// stem, and returns the file's sessionId.
func (s *Store) readSessionMeta(path string, size int64, modTime time.Time, wsID string) (*thinkt.SessionMeta, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	var sess Session
	if err := json.NewDecoder(f).Decode(&sess); err != nil {
		return nil, "", err
	}

	meta := &thinkt.SessionMeta{
		ID:          strings.TrimSuffix(filepath.Base(path), ".json"),
		FullPath:    path,
		EntryCount:  len(sess.Messages),
		FileSize:    size,
		CreatedAt:   sess.StartTime,
		ModifiedAt:  sess.LastUpdated,
		Source:      thinkt.SourceGemini,
		WorkspaceID: wsID,
		ChunkCount:  1,
	}
	if meta.ModifiedAt.IsZero() {
		meta.ModifiedAt = modTime
	}
	for _, msg := range sess.Messages {
		if msg.Type == "user" && meta.FirstPrompt == "" {
			meta.FirstPrompt = msg.Content
		}
		if msg.Type == "gemini" && thinkt.IsRealModel(msg.Model) {
			meta.Model = msg.Model
		}
	}
	return meta, sess.SessionID, nil
}

// GetSessionMeta returns a session's metadata by ID or by path. A path that
// is a superseded .json file or a losing copy in a second folder is not a
// session.
func (s *Store) GetSessionMeta(ctx context.Context, sessionID string) (*thinkt.SessionMeta, error) {
	if filepath.IsAbs(sessionID) {
		if err := thinkt.ValidateSessionPath(sessionID, s.baseDir); err != nil {
			return nil, nil
		}
		hash := fileProjectHash(sessionID)
		if hash == "" {
			return nil, nil
		}
		all, err := s.allSessions(hash)
		if err != nil {
			return nil, err
		}
		for _, m := range all {
			if samePath(m.FullPath, sessionID) {
				return &m, nil
			}
		}
		return nil, nil
	}
	projects, _ := s.ListProjects(ctx)
	for _, p := range projects {
		all, _ := s.allSessions(p.ID)
		for _, m := range all {
			if m.ID == sessionID {
				return &m, nil
			}
		}
	}
	return nil, nil
}

// samePath reports whether two paths name the same file.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// LoadSession loads a complete session.
func (s *Store) LoadSession(ctx context.Context, sessionID string) (*thinkt.Session, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil || meta == nil {
		return nil, err
	}

	f, err := os.Open(meta.FullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if strings.HasSuffix(meta.FullPath, ".jsonl") {
		r, err := replay(f)
		if err != nil {
			return nil, err
		}
		entries := convert(r, meta.ID)
		for i := range entries {
			entries[i].WorkspaceID = meta.WorkspaceID
		}
		return &thinkt.Session{Meta: *meta, Entries: entries}, nil
	}

	var geminiSess Session
	if err := json.NewDecoder(f).Decode(&geminiSess); err != nil {
		return nil, err
	}

	return convertSession(&geminiSess, meta), nil
}

// OpenSession returns a streaming reader.
func (s *Store) OpenSession(ctx context.Context, sessionID string) (thinkt.SessionReader, error) {
	sess, err := s.LoadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, os.ErrNotExist
	}

	return &memorySessionReader{
		session: sess,
		idx:     0,
	}, nil
}

type memorySessionReader struct {
	session *thinkt.Session
	idx     int
}

func (r *memorySessionReader) ReadNext() (*thinkt.Entry, error) {
	if r.idx >= len(r.session.Entries) {
		return nil, io.EOF
	}
	entry := r.session.Entries[r.idx]
	r.idx++
	return &entry, nil
}

func (r *memorySessionReader) Metadata() thinkt.SessionMeta {
	return r.session.Meta
}

func (r *memorySessionReader) Close() error {
	return nil
}

// ResetCache clears all cached data.
func (s *Store) ResetCache() { s.cache.Clear() }

// GetProject returns a specific project.
func (s *Store) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range projects {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, nil
}

// convertSession converts Gemini specific structure to thinkt.
func convertSession(g *Session, meta *thinkt.SessionMeta) *thinkt.Session {
	var entries []thinkt.Entry

	for _, msg := range g.Messages {
		if msg.Type == "user" {
			entries = append(entries, thinkt.Entry{
				UUID:        msg.ID,
				Role:        thinkt.RoleUser,
				Timestamp:   msg.Timestamp,
				Source:      thinkt.SourceGemini,
				WorkspaceID: meta.WorkspaceID,
				Text:        msg.Content,
			})
		} else if msg.Type == "gemini" {
			asstEntry := thinkt.Entry{
				UUID:        msg.ID,
				Role:        thinkt.RoleAssistant,
				Timestamp:   msg.Timestamp,
				Source:      thinkt.SourceGemini,
				WorkspaceID: meta.WorkspaceID,
				Model:       msg.Model,
			}

			asstEntry.Usage = usageFromTokens(msg.Tokens)

			if msg.Content != "" {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}

			for _, t := range msg.Thoughts {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type:     "thinking",
					Thinking: fmt.Sprintf("[%s] %s", t.Subject, t.Description),
				})
			}

			for _, tc := range msg.ToolCalls {
				asstEntry.ContentBlocks = append(asstEntry.ContentBlocks, thinkt.ContentBlock{
					Type:      "tool_use",
					ToolUseID: tc.ID,
					ToolName:  tc.Name,
					ToolInput: tc.Args,
				})
			}

			entries = append(entries, asstEntry)

			// Separate entries for Tool Results
			for _, tc := range msg.ToolCalls {
				for _, res := range tc.Result {
					output := ""
					if o, ok := res.FunctionResponse.Response["output"]; ok {
						if s, ok := o.(string); ok {
							output = s
						} else {
							b, _ := json.Marshal(o)
							output = string(b)
						}
					}

					// Ensure unique UUID for tool results
					resID := res.FunctionResponse.ID
					if resID == "" {
						resID = fmt.Sprintf("%s-res", tc.ID)
					}

					entries = append(entries, thinkt.Entry{
						UUID:        resID,
						Role:        thinkt.RoleTool,
						Timestamp:   msg.Timestamp,
						Source:      thinkt.SourceGemini,
						WorkspaceID: meta.WorkspaceID,
						ContentBlocks: []thinkt.ContentBlock{
							{
								Type:       "tool_result",
								ToolUseID:  tc.ID,
								ToolResult: output,
							},
						},
					})
				}
			}
		}
	}

	meta.EntryCount = len(entries)
	return &thinkt.Session{
		Meta:    *meta,
		Entries: entries,
	}
}

// WatchConfig returns the watch configuration for Gemini session files.
func (s *Store) WatchConfig() thinkt.WatchConfig {
	return thinkt.WatchConfig{
		IncludeDirs: []string{"tmp"},
		MaxDepth:    4, // tmp/<project>/chats/<parent>/<subagent>.jsonl
	}
}
