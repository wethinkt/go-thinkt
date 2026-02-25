package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/indexer"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

var noWatch bool

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the indexer server with RPC socket, file watcher, and embedding",
	RunE:  runServer,
}

func init() {
	serverCmd.Flags().BoolVar(&noWatch, "no-watch", false, "disable file watching")
	rootCmd.AddCommand(serverCmd)
}

// indexerServer implements rpc.Handler and holds all server state.
type indexerServer struct {
	db        *db.DB
	embDB     *db.DB // separate embeddings database
	registry  *thinkt.StoreRegistry
	embedder  *embedding.Embedder
	watcher   *indexer.Watcher // file watcher (nil if disabled)
	watching  bool
	startedAt time.Time

	// shutdownCtx is cancelled on SIGTERM/SIGINT to abort in-flight work.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// Sync coordination: one sync runs at a time, but multiple clients
	// can subscribe to its progress stream.
	indexMu     sync.Mutex
	indexSubs   []rpcSubscriber
	indexSubsMu sync.Mutex
	indexDone   chan struct{}
	indexResult *rpc.Response
	indexErr    error

	// Embed sync coordination: independent from index sync.
	embedMu     sync.Mutex
	embedSubs   []rpcSubscriber
	embedSubsMu sync.Mutex
	embedDone   chan struct{}
	embedResult *rpc.Response
	embedErr    error

	// Config reload coordination
	reloadMu sync.Mutex

	// Status tracking
	stateMu   sync.RWMutex
	syncing   bool
	embedding bool
	syncProg  *rpc.ProgressInfo
	embedProg *rpc.ProgressInfo
}

type rpcSubscriber struct {
	id int
	fn func(rpc.Progress)
}

var nextSubID int

// broadcastToSubs sends a progress event to a subscriber list (sync or embed).
func broadcastToSubs(mu *sync.Mutex, subs *[]rpcSubscriber, p rpc.Progress) {
	mu.Lock()
	snapshot := make([]rpcSubscriber, len(*subs))
	copy(snapshot, *subs)
	mu.Unlock()

	for _, sub := range snapshot {
		sub.fn(p)
	}
}

// addSubscriber adds a progress listener to a subscriber list and returns a removal function.
func addSubscriber(mu *sync.Mutex, subs *[]rpcSubscriber, fn func(rpc.Progress)) func() {
	mu.Lock()
	nextSubID++
	id := nextSubID
	*subs = append(*subs, rpcSubscriber{id: id, fn: fn})
	mu.Unlock()

	return func() {
		mu.Lock()
		defer mu.Unlock()
		for i, sub := range *subs {
			if sub.id == id {
				*subs = append((*subs)[:i], (*subs)[i+1:]...)
				break
			}
		}
	}
}

func (s *indexerServer) HandleIndexSync(ctx context.Context, params rpc.SyncParams, send func(rpc.Progress)) (*rpc.Response, error) {
	if !s.indexMu.TryLock() {
		// Sync already in progress — subscribe to its progress stream and wait.
		remove := addSubscriber(&s.indexSubsMu, &s.indexSubs, send)
		defer remove()
		<-s.indexDone
		return s.indexResult, s.indexErr
	}
	defer s.indexMu.Unlock()

	// Set up done channel for subscribers to wait on.
	s.indexDone = make(chan struct{})

	// The initiator is also a subscriber.
	remove := addSubscriber(&s.indexSubsMu, &s.indexSubs, send)

	// Ensure subscribers see the result and get cleaned up.
	var indexResp *rpc.Response
	var indexErr error
	defer func() {
		s.indexResult = indexResp
		s.indexErr = indexErr
		remove()
		close(s.indexDone)
	}()

	// Cancel if either the request context or the server shutdown context is done.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
		case <-s.shutdownCtx.Done():
			cancel()
		}
	}()

	s.stateMu.Lock()
	s.syncing = true
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		s.syncing = false
		s.syncProg = nil
		s.stateMu.Unlock()
	}()

	ingester := indexer.NewIngester(s.db, s.embDB, s.registry, s.embedder)

	// Wire up progress reporting
	ingester.OnProgress = func(pIdx, pTotal, sIdx, sTotal int, message string) {
		s.stateMu.Lock()
		s.syncProg = &rpc.ProgressInfo{
			Done: sIdx, Total: sTotal,
			Project: pIdx, ProjectTotal: pTotal, ProjectName: message,
			Message: fmt.Sprintf("Project %d/%d %s", pIdx, pTotal, message),
		}
		s.stateMu.Unlock()

		data, _ := json.Marshal(map[string]any{
			"project": pIdx, "project_total": pTotal,
			"session": sIdx, "session_total": sTotal,
			"message": message,
		})
		broadcastToSubs(&s.indexSubsMu, &s.indexSubs, rpc.Progress{Data: data})
	}

	projects, err := s.registry.ListAllProjects(ctx)
	if err != nil {
		indexErr = fmt.Errorf("list projects: %w", err)
		return nil, indexErr
	}

	if params.Force {
		tuilog.Log.Info("indexer: force sync requested, clearing sync state")
		if _, err := s.db.ExecContext(ctx, "DELETE FROM sync_state"); err != nil {
			tuilog.Log.Warn("indexer: failed to clear sync state", "error", err)
		}
	}

	totalProjects := len(projects)
	for idx, p := range projects {
		if ctx.Err() != nil {
			tuilog.Log.Warn("indexer: sync cancelled")
			break
		}
		if err := ingester.IngestProject(ctx, p, idx+1, totalProjects); err != nil {
			tuilog.Log.Error("indexer: failed to index project", "project", p.Name, "error", err)
		}
	}

	result, _ := json.Marshal(map[string]any{
		"projects": totalProjects,
	})
	indexResp = &rpc.Response{OK: true, Data: result}
	return indexResp, nil
}

func (s *indexerServer) HandleEmbedSync(ctx context.Context, params rpc.EmbedSyncParams, send func(rpc.Progress)) (*rpc.Response, error) {
	if !s.embedMu.TryLock() {
		// Embed sync already in progress — subscribe and wait.
		remove := addSubscriber(&s.embedSubsMu, &s.embedSubs, send)
		defer remove()
		<-s.embedDone
		return s.embedResult, s.embedErr
	}
	defer s.embedMu.Unlock()

	s.embedDone = make(chan struct{})
	remove := addSubscriber(&s.embedSubsMu, &s.embedSubs, send)

	var embedResp *rpc.Response
	var embedErr error
	defer func() {
		s.embedResult = embedResp
		s.embedErr = embedErr
		remove()
		close(s.embedDone)
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
		case <-s.shutdownCtx.Done():
			cancel()
		}
	}()

	// Lazy-init embedder if not already loaded
	if s.embedder == nil {
		cfg, err := config.Load()
		if err != nil || !cfg.Embedding.Enabled {
			embedResp = &rpc.Response{OK: false, Error: "embedding not enabled"}
			return embedResp, nil
		}
		modelID := cfg.Embedding.Model
		tuilog.Log.Info("indexer: lazy-loading embedding model", "model", modelID)

		if err := embedding.EnsureModel(modelID, func(downloaded, total int64) {
			if total > 0 {
				pct := float64(downloaded) / float64(total) * 100
				data, _ := json.Marshal(map[string]any{
					"model_download": true,
					"downloaded":     downloaded,
					"total":          total,
					"percent":        pct,
				})
				broadcastToSubs(&s.embedSubsMu, &s.embedSubs, rpc.Progress{Data: data})
			}
		}); err != nil {
			embedErr = fmt.Errorf("ensure embedding model: %w", err)
			return nil, embedErr
		}

		e, err := embedding.NewEmbedder(modelID, "")
		if err != nil {
			embedErr = fmt.Errorf("create embedder: %w", err)
			return nil, embedErr
		}

		if s.embDB == nil {
			d, err := getEmbeddingsDB(e.Dim())
			if err != nil {
				e.Close()
				embedErr = fmt.Errorf("open embeddings database: %w", err)
				return nil, embedErr
			}
			s.embDB = d
		}

		s.embedder = e
		if s.watcher != nil {
			s.watcher.SetEmbedder(e)
		}
		tuilog.Log.Info("indexer: embedder loaded", "model", e.EmbedModelID(), "dim", e.Dim())
	}

	s.stateMu.Lock()
	s.embedding = true
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		s.embedding = false
		s.embedProg = nil
		s.stateMu.Unlock()
	}()

	ingester := indexer.NewIngester(s.db, s.embDB, s.registry, s.embedder)
	ingester.Verbose = verbose

	// Migrate embeddings if model changed
	if err := ingester.MigrateEmbeddings(ctx); err != nil {
		tuilog.Log.Warn("indexer: migration check failed", "error", err)
	}

	if params.Force {
		tuilog.Log.Info("indexer: force embed sync requested, clearing embed state")
		if s.embDB != nil {
			if _, err := s.embDB.ExecContext(ctx, "DELETE FROM embeddings"); err != nil {
				tuilog.Log.Warn("indexer: failed to clear embeddings", "error", err)
			}
		}
	}

	ingester.OnEmbedProgress = func(done, total, chunks, entries int, sessionID, sessionPath string, elapsed time.Duration) {
		s.stateMu.Lock()
		s.embedProg = &rpc.ProgressInfo{Done: done, Total: total, SessionID: sessionID, Entries: entries}
		s.stateMu.Unlock()

		data, _ := json.Marshal(map[string]any{
			"done": done, "total": total,
			"chunks": chunks, "entries": entries,
			"session_id":   sessionID,
			"session_path": sessionPath,
			"elapsed_ms":   elapsed.Milliseconds(),
		})
		broadcastToSubs(&s.embedSubsMu, &s.embedSubs, rpc.Progress{Data: data})
	}
	ingester.OnEmbedChunkProgress = func(chunksDone, chunksTotal, tokensDone int, sessionID string) {
		s.stateMu.Lock()
		if s.embedProg != nil {
			s.embedProg.ChunksDone = chunksDone
			s.embedProg.ChunksTotal = chunksTotal
		}
		s.stateMu.Unlock()

		data, _ := json.Marshal(map[string]any{
			"chunks_done":  chunksDone,
			"chunks_total": chunksTotal,
			"tokens_done":  tokensDone,
			"session_id":   sessionID,
		})
		broadcastToSubs(&s.embedSubsMu, &s.embedSubs, rpc.Progress{Data: data})
	}

	if err := ingester.EmbedAllSessions(ctx); err != nil {
		tuilog.Log.Error("indexer: embedding pass failed", "error", err)
	}

	embedResp = &rpc.Response{OK: true, Data: json.RawMessage(`{"ok":true}`)}
	return embedResp, nil
}

func (s *indexerServer) HandleSearch(ctx context.Context, params rpc.SearchParams) (*rpc.Response, error) {
	svc := search.NewService(s.db, nil)

	opts := search.SearchOptions{
		Query:           params.Query,
		FilterProject:   params.Project,
		FilterSource:    params.Source,
		Limit:           params.Limit,
		LimitPerSession: params.LimitPerSession,
		CaseSensitive:   params.CaseSensitive,
		UseRegex:        params.Regex,
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.LimitPerSession <= 0 {
		opts.LimitPerSession = 2
	}

	results, totalMatches, err := svc.Search(opts)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	data, err := json.Marshal(map[string]any{
		"results":       results,
		"total_matches": totalMatches,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal results: %w", err)
	}
	return &rpc.Response{OK: true, Data: data}, nil
}

func (s *indexerServer) HandleSemanticSearch(ctx context.Context, params rpc.SemanticSearchParams) (*rpc.Response, error) {
	if s.embedder == nil {
		return &rpc.Response{OK: false, Error: "embedding model not available"}, nil
	}

	// Embed the query text
	result, err := s.embedder.Embed(ctx, []string{params.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(result.Vectors) == 0 {
		return nil, fmt.Errorf("embedding produced no vectors")
	}

	svc := search.NewService(s.db, s.embDB)

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	maxDist := params.MaxDistance // 0 means no threshold

	results, err := svc.SemanticSearch(search.SemanticSearchOptions{
		QueryEmbedding: result.Vectors[0],
		Model:          s.embedder.EmbedModelID(),
		Dim:            s.embedder.Dim(),
		FilterProject:  params.Project,
		FilterSource:   params.Source,
		Limit:          limit,
		MaxDistance:    maxDist,
		Diversity:      params.Diversity,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	data, err := json.Marshal(map[string]any{
		"results": results,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal results: %w", err)
	}
	return &rpc.Response{OK: true, Data: data}, nil
}

func (s *indexerServer) HandleStats(ctx context.Context) (*rpc.Response, error) {
	var stats struct {
		TotalProjects   int            `json:"total_projects"`
		TotalSessions   int            `json:"total_sessions"`
		TotalEntries    int            `json:"total_entries"`
		TotalTokens     int            `json:"total_tokens"`
		TotalEmbeddings int            `json:"total_embeddings"`
		EmbedModel      string         `json:"embed_model"`
		ToolUsage       map[string]int `json:"tool_usage"`
	}

	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM projects").Scan(&stats.TotalProjects); err != nil {
		return nil, fmt.Errorf("count projects: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM sessions").Scan(&stats.TotalSessions); err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM entries").Scan(&stats.TotalEntries); err != nil {
		return nil, fmt.Errorf("count entries: %w", err)
	}
	_ = s.db.QueryRowContext(ctx, "SELECT COALESCE(sum(input_tokens + output_tokens), 0) FROM entries").Scan(&stats.TotalTokens)
	if s.embDB != nil {
		_ = s.embDB.QueryRowContext(ctx, "SELECT count(*) FROM embeddings").Scan(&stats.TotalEmbeddings)
	}

	if s.embedder != nil {
		stats.EmbedModel = s.embedder.EmbedModelID()
	}

	rows, err := s.db.QueryContext(ctx, "SELECT tool_name, count(*) FROM entries WHERE tool_name != '' GROUP BY tool_name ORDER BY count(*) DESC")
	if err == nil {
		stats.ToolUsage = make(map[string]int)
		for rows.Next() {
			var name string
			var count int
			if err := rows.Scan(&name, &count); err == nil {
				stats.ToolUsage[name] = count
			}
		}
		rows.Close()
	}

	data, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("marshal stats: %w", err)
	}
	return &rpc.Response{OK: true, Data: data}, nil
}

func (s *indexerServer) HandleStatus(ctx context.Context) (*rpc.Response, error) {
	s.stateMu.RLock()
	syncing := s.syncing
	emb := s.embedding
	state := "idle"
	if syncing && emb {
		state = "syncing+embedding"
	} else if syncing {
		state = "syncing"
	} else if emb {
		state = "embedding"
	}
	status := rpc.StatusData{
		Syncing:       syncing,
		Embedding:     emb,
		State:         state,
		SyncProgress:  s.syncProg,
		EmbedProgress: s.embedProg,
		Watching:      s.watching,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
	}
	s.stateMu.RUnlock()

	if s.embedder != nil {
		status.Model = s.embedder.EmbedModelID()
		status.ModelDim = s.embedder.Dim()
	}

	data, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("marshal status: %w", err)
	}
	return &rpc.Response{OK: true, Data: data}, nil
}

func (s *indexerServer) HandleConfigReload(ctx context.Context) (*rpc.Response, error) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	wasEnabled := s.embedder != nil
	wantEnabled := cfg.Embedding.Enabled

	if wantEnabled && !wasEnabled {
		// Enable embedding — kick off embed sync in background (lazy model download)
		tuilog.Log.Info("indexer: config reload enabling embedding")

		go func() {
			tuilog.Log.Info("indexer: starting post-enable embed sync")
			resp, err := s.HandleEmbedSync(s.shutdownCtx, rpc.EmbedSyncParams{}, func(rpc.Progress) {})
			if err != nil {
				tuilog.Log.Error("indexer: post-enable embed sync error", "error", err)
			} else if resp != nil && !resp.OK {
				tuilog.Log.Error("indexer: post-enable embed sync failed", "error", resp.Error)
			} else {
				tuilog.Log.Info("indexer: post-enable embed sync complete")
			}
		}()

		data, _ := json.Marshal(map[string]any{"embedding_enabled": true})
		return &rpc.Response{OK: true, Data: data}, nil
	}

	if !wantEnabled && wasEnabled {
		// Disable embedding
		tuilog.Log.Info("indexer: config reload disabling embedding")

		old := s.embedder
		s.embedder = nil
		if s.watcher != nil {
			s.watcher.SetEmbedder(nil)
		}
		old.Close()

		data, _ := json.Marshal(map[string]any{"embedding_enabled": false})
		return &rpc.Response{OK: true, Data: data}, nil
	}

	// No change
	data, _ := json.Marshal(map[string]any{"embedding_enabled": wantEnabled})
	return &rpc.Response{OK: true, Data: data}, nil
}

func runServer(cmdObj *cobra.Command, args []string) error {
	// 0. Load config
	cfg, err := config.Load()
	if err != nil {
		tuilog.Log.Warn("indexer: failed to load config, using defaults", "error", err)
		cfg = config.Default()
	}

	// 1. Open DBs
	database, err := getDB()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// 2. Create registry and server struct (no model download at startup)
	registry := cmd.CreateSourceRegistryFiltered(cfg.Indexer.Sources)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	srv := &indexerServer{
		db:             database,
		registry:       registry,
		startedAt:      time.Now(),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}

	// 6. Start RPC server
	socketPath := rpc.DefaultSocketPath()
	rpcServer := rpc.NewServer(socketPath, srv)
	if err := rpcServer.Start(); err != nil {
		return fmt.Errorf("failed to start RPC server: %w", err)
	}
	defer rpcServer.Stop()
	tuilog.Log.Info("indexer: RPC server listening", "socket", socketPath)

	// 7. Register instance
	inst := config.Instance{
		Type:      config.InstanceIndexerServer,
		PID:       os.Getpid(),
		LogPath:   logPath,
		StartedAt: time.Now(),
	}
	if err := config.RegisterInstance(inst); err != nil {
		tuilog.Log.Warn("indexer: failed to register instance", "error", err)
	}
	defer func() {
		_ = config.UnregisterInstance(os.Getpid())
	}()

	// 5. Start watcher (unless --no-watch or config disables it)
	// Embedder is nil at startup — watcher embeds inline when embedder is set later.
	var watcher *indexer.Watcher
	watchEnabled := cfg.Indexer.Watch && !noWatch
	if watchEnabled {
		w, err := indexer.NewWatcher(dbPath, embDBPath, registry, nil, cfg.Indexer.DebounceDuration())
		if err != nil {
			tuilog.Log.Warn("indexer: failed to create watcher", "error", err)
		} else {
			watchCtx, watchCancel := context.WithCancel(context.Background())
			defer watchCancel()
			if err := w.Start(watchCtx); err != nil {
				tuilog.Log.Warn("indexer: failed to start watcher", "error", err)
			} else {
				watcher = w
				srv.watcher = w
				srv.watching = true
				tuilog.Log.Info("indexer: file watcher started")
			}
		}
	}

	// 6. Run initial index sync in background, then kick off embed sync
	go func() {
		tuilog.Log.Info("indexer: starting initial index sync")
		resp, err := srv.HandleIndexSync(shutdownCtx, rpc.SyncParams{}, func(rpc.Progress) {})
		if err != nil {
			tuilog.Log.Error("indexer: initial index sync error", "error", err)
		} else if resp != nil && !resp.OK {
			tuilog.Log.Error("indexer: initial index sync failed", "error", resp.Error)
		} else {
			tuilog.Log.Info("indexer: initial index sync complete")
		}

		// If embedding is enabled, start embed sync in background (lazy model download)
		if cfg.Embedding.Enabled {
			tuilog.Log.Info("indexer: starting initial embed sync")
			resp, err := srv.HandleEmbedSync(shutdownCtx, rpc.EmbedSyncParams{}, func(rpc.Progress) {})
			if err != nil {
				tuilog.Log.Error("indexer: initial embed sync error", "error", err)
			} else if resp != nil && !resp.OK {
				tuilog.Log.Error("indexer: initial embed sync failed", "error", resp.Error)
			} else {
				tuilog.Log.Info("indexer: initial embed sync complete")
			}
		}
	}()

	if !quiet {
		fmt.Fprintf(os.Stderr, "Indexer server running (PID: %d). Press Ctrl+C to stop.\n", os.Getpid())
	}

	// 10. Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan
	tuilog.Log.Info("indexer: received signal, shutting down", "signal", sig)
	shutdownCancel() // Cancel in-flight sync/embed operations

	// 11. Shutdown
	if watcher != nil {
		tuilog.Log.Info("indexer: stopping file watcher")
		if err := watcher.Stop(); err != nil {
			tuilog.Log.Warn("indexer: watcher stop error", "error", err)
		}
	}

	tuilog.Log.Info("indexer: stopping RPC server")
	// rpcServer.Stop() called by defer

	// Close embedder and embeddings DB (lifecycle managed by server, not defer)
	if srv.embedder != nil {
		srv.embedder.Close()
	}
	if srv.embDB != nil {
		srv.embDB.Close()
	}

	tuilog.Log.Info("indexer: server stopped")
	return nil
}
