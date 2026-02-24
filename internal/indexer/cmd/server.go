package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	syncMu      sync.Mutex
	syncSubs    []syncSubscriber // active progress subscribers
	syncSubsMu  sync.Mutex
	syncDone    chan struct{} // closed when current sync finishes
	syncResult  *rpc.Response
	syncErr     error

	// Config reload coordination
	reloadMu sync.Mutex

	// Status tracking
	stateMu   sync.RWMutex
	state     string          // "idle", "syncing", "embedding"
	syncProg  *rpc.ProgressInfo
	embedProg *rpc.ProgressInfo
}

func (s *indexerServer) setState(state string) {
	s.stateMu.Lock()
	s.state = state
	s.stateMu.Unlock()
}

type syncSubscriber struct {
	id int
	fn func(rpc.Progress)
}

// broadcastProgress sends a progress event to all subscribed clients.
func (s *indexerServer) broadcastProgress(p rpc.Progress) {
	s.syncSubsMu.Lock()
	subs := make([]syncSubscriber, len(s.syncSubs))
	copy(subs, s.syncSubs)
	s.syncSubsMu.Unlock()

	for _, sub := range subs {
		sub.fn(p)
	}
}

var nextSubID int

// addSyncSubscriber adds a progress listener and returns a removal function.
func (s *indexerServer) addSyncSubscriber(fn func(rpc.Progress)) func() {
	s.syncSubsMu.Lock()
	nextSubID++
	id := nextSubID
	s.syncSubs = append(s.syncSubs, syncSubscriber{id: id, fn: fn})
	s.syncSubsMu.Unlock()

	return func() {
		s.syncSubsMu.Lock()
		defer s.syncSubsMu.Unlock()
		for i, sub := range s.syncSubs {
			if sub.id == id {
				s.syncSubs = append(s.syncSubs[:i], s.syncSubs[i+1:]...)
				break
			}
		}
	}
}

func (s *indexerServer) HandleSync(ctx context.Context, params rpc.SyncParams, send func(rpc.Progress)) (*rpc.Response, error) {
	if !s.syncMu.TryLock() {
		// Sync already in progress — subscribe to its progress stream and wait.
		remove := s.addSyncSubscriber(send)
		defer remove()
		<-s.syncDone
		return s.syncResult, s.syncErr
	}
	defer s.syncMu.Unlock()

	// Set up done channel for subscribers to wait on.
	s.syncDone = make(chan struct{})

	// The initiator is also a subscriber.
	remove := s.addSyncSubscriber(send)

	// Ensure subscribers see the result and get cleaned up.
	var syncResp *rpc.Response
	var syncErr error
	defer func() {
		s.syncResult = syncResp
		s.syncErr = syncErr
		remove()
		close(s.syncDone)
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

	s.setState("syncing")
	defer s.setState("idle")

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
		s.broadcastProgress(rpc.Progress{Data: data})
	}

	projects, err := s.registry.ListAllProjects(ctx)
	if err != nil {
		syncErr = fmt.Errorf("list projects: %w", err)
		return nil, syncErr
	}

	if params.Force {
		log.Printf("Force sync: clearing sync state")
		if _, err := s.db.ExecContext(ctx, "DELETE FROM sync_state"); err != nil {
			log.Printf("Warning: failed to clear sync state: %v", err)
		}
	}

	totalProjects := len(projects)
	for idx, p := range projects {
		if ctx.Err() != nil {
			log.Printf("Sync cancelled")
			break
		}
		if err := ingester.IngestProject(ctx, p, idx+1, totalProjects); err != nil {
			log.Printf("Error indexing project %s: %v", p.Name, err)
		}
	}

	// Second pass: embeddings
	if ingester.HasEmbedder() {
		s.setState("embedding")
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
			s.broadcastProgress(rpc.Progress{Data: data})
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
			s.broadcastProgress(rpc.Progress{Data: data})
		}
		if err := ingester.EmbedAllSessions(ctx); err != nil {
			log.Printf("Embedding error: %v", err)
		}
	}

	// Clear progress
	s.stateMu.Lock()
	s.syncProg = nil
	s.embedProg = nil
	s.stateMu.Unlock()

	result, _ := json.Marshal(map[string]any{
		"projects": totalProjects,
	})
	syncResp = &rpc.Response{OK: true, Data: result}
	return syncResp, nil
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
	status := rpc.StatusData{
		State:         s.state,
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
		// Enable embedding
		log.Printf("Config reload: enabling embedding")

		if err := embedding.EnsureModel(func(downloaded, total int64) {
			if total > 0 {
				pct := float64(downloaded) / float64(total) * 100
				log.Printf("Downloading model: %.1f%%", pct)
			}
		}); err != nil {
			return nil, fmt.Errorf("failed to ensure embedding model: %w", err)
		}

		e, err := embedding.NewEmbedder("")
		if err != nil {
			return nil, fmt.Errorf("failed to create embedder: %w", err)
		}

		// Open embeddings DB if not already open
		if s.embDB == nil {
			d, err := getEmbeddingsDB()
			if err != nil {
				e.Close()
				return nil, fmt.Errorf("failed to open embeddings database: %w", err)
			}
			s.embDB = d
		}

		s.embedder = e
		if s.watcher != nil {
			s.watcher.SetEmbedder(e)
		}
		log.Printf("Embedder loaded: %s (dim=%d)", e.EmbedModelID(), e.Dim())

		// Trigger a background sync to embed existing sessions
		go func() {
			log.Printf("Starting embedding sync after enable...")
			resp, err := s.HandleSync(s.shutdownCtx, rpc.SyncParams{}, func(rpc.Progress) {})
			if err != nil {
				log.Printf("Post-enable sync error: %v", err)
			} else if resp != nil && !resp.OK {
				log.Printf("Post-enable sync failed: %s", resp.Error)
			} else {
				log.Printf("Post-enable sync complete")
			}
		}()

		data, _ := json.Marshal(map[string]any{"embedding_enabled": true})
		return &rpc.Response{OK: true, Data: data}, nil
	}

	if !wantEnabled && wasEnabled {
		// Disable embedding
		log.Printf("Config reload: disabling embedding")

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
		log.Printf("Warning: failed to load config, using defaults: %v", err)
		cfg = config.Default()
	}

	// 1. Open DBs
	database, err := getDB()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	var embDatabase *db.DB

	// 2-3. Create embedder (if enabled)
	var embedder *embedding.Embedder
	if cfg.Embedding.Enabled {
		log.Printf("Ensuring embedding model is available...")
		var lastLog time.Time
		if err := embedding.EnsureModel(func(downloaded, total int64) {
			if total > 0 && time.Since(lastLog) >= time.Second {
				lastLog = time.Now()
				pct := float64(downloaded) / float64(total) * 100
				log.Printf("Downloading model: %.1f%% (%d / %d bytes)", pct, downloaded, total)
			}
		}); err != nil {
			return fmt.Errorf("failed to ensure embedding model: %w", err)
		}

		e, err := embedding.NewEmbedder("")
		if err != nil {
			return fmt.Errorf("failed to create embedder: %w", err)
		}
		embedder = e
		log.Printf("Embedder loaded: %s (dim=%d)", embedder.EmbedModelID(), embedder.Dim())

		d, err := getEmbeddingsDB()
		if err != nil {
			embedder.Close()
			return fmt.Errorf("failed to open embeddings database: %w", err)
		}
		embDatabase = d
	} else {
		log.Printf("Embedding disabled by config")
	}

	// 4. Create registry and server struct
	registry := cmd.CreateSourceRegistryFiltered(cfg.Indexer.Sources)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	srv := &indexerServer{
		db:             database,
		embDB:          embDatabase,
		registry:       registry,
		embedder:       embedder,
		startedAt:      time.Now(),
		state:          "idle",
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}

	// 5. Migrate embeddings (drop old model embeddings)
	ctx := context.Background()
	ingester := indexer.NewIngester(database, embDatabase, registry, embedder)
	if err := ingester.MigrateEmbeddings(ctx); err != nil {
		log.Printf("Warning: migration check failed: %v", err)
	}

	// 6. Start RPC server
	socketPath := rpc.DefaultSocketPath()
	rpcServer := rpc.NewServer(socketPath, srv)
	if err := rpcServer.Start(); err != nil {
		return fmt.Errorf("failed to start RPC server: %w", err)
	}
	defer rpcServer.Stop()
	log.Printf("RPC server listening on %s", socketPath)

	// 7. Register instance
	inst := config.Instance{
		Type:      config.InstanceIndexerServer,
		PID:       os.Getpid(),
		LogPath:   logPath,
		StartedAt: time.Now(),
	}
	if err := config.RegisterInstance(inst); err != nil {
		log.Printf("Warning: failed to register instance: %v", err)
	}
	defer func() {
		_ = config.UnregisterInstance(os.Getpid())
	}()

	// 8. Start watcher (unless --no-watch or config disables it)
	var watcher *indexer.Watcher
	watchEnabled := cfg.Indexer.Watch && !noWatch
	if watchEnabled {
		w, err := indexer.NewWatcher(dbPath, embDBPath, registry, embedder, cfg.Indexer.DebounceDuration())
		if err != nil {
			log.Printf("Warning: failed to create watcher: %v", err)
		} else {
			watchCtx, watchCancel := context.WithCancel(context.Background())
			defer watchCancel()
			if err := w.Start(watchCtx); err != nil {
				log.Printf("Warning: failed to start watcher: %v", err)
			} else {
				watcher = w
				srv.watcher = w
				srv.watching = true
				log.Printf("File watcher started")
			}
		}
	}

	// 9. Run initial sync in background
	go func() {
		log.Printf("Starting initial sync...")
		resp, err := srv.HandleSync(shutdownCtx, rpc.SyncParams{}, func(rpc.Progress) {})
		if err != nil {
			log.Printf("Initial sync error: %v", err)
		} else if resp != nil && !resp.OK {
			log.Printf("Initial sync failed: %s", resp.Error)
		} else {
			log.Printf("Initial sync complete")
		}
	}()

	if !quiet {
		fmt.Fprintf(os.Stderr, "Indexer server running (PID: %d). Press Ctrl+C to stop.\n", os.Getpid())
	}

	// 10. Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan
	log.Printf("Received signal %v, shutting down...", sig)
	shutdownCancel() // Cancel in-flight sync/embed operations

	// 11. Shutdown
	if watcher != nil {
		log.Printf("Stopping file watcher...")
		if err := watcher.Stop(); err != nil {
			log.Printf("Warning: watcher stop error: %v", err)
		}
	}

	log.Printf("Stopping RPC server...")
	// rpcServer.Stop() called by defer

	// Close embedder and embeddings DB (lifecycle managed by server, not defer)
	if srv.embedder != nil {
		srv.embedder.Close()
	}
	if srv.embDB != nil {
		srv.embDB.Close()
	}

	log.Printf("Server stopped")
	return nil
}
