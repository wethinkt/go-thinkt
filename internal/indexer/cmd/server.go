package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/cmd"
	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/indexer"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/indexer/summarize"
	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

var noWatch bool

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the indexer server with RPC socket, file watcher, and embedding",
	Args:  cobra.NoArgs,
	RunE:  runServer,
}

func init() {
	serverCmd.Flags().BoolVar(&noWatch, "no-watch", false, "disable file watching")
	rootCmd.AddCommand(serverCmd)
}

// indexerServer implements rpc.Handler and holds all server state.
type indexerServer struct {
	db         *db.DB
	embDB      *db.DB // separate embeddings database (per-model; re-opened on model change)
	embDBModel string // model ID that embDB was opened for
	registry   *thinkt.StoreRegistry
	embedder   *embedding.Embedder
	watcher    *indexer.Watcher // file watcher (nil if disabled)
	watching   bool
	startedAt  time.Time

	// shutdownCtx is cancelled on SIGTERM/SIGINT to abort in-flight work.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// Sync coordination: one operation per gate, with progress fan-out.
	indexGate indexer.SyncGate
	embedGate indexer.SyncGate
	sumGate   indexer.SyncGate
	sumDB     *db.DB // separate summaries database (per-model)

	// embedCancelMu guards embedCancelFn, which is set while a sync is in progress.
	// Separate from embedGate to avoid deadlock with reloadMu.
	embedCancelMu sync.Mutex
	embedCancelFn context.CancelFunc

	// GPU mutex: serializes embed and summarize sync so only one uses
	// the GPU (yzma/llama.cpp) at a time.
	gpuMu sync.Mutex

	// Config reload coordination
	reloadMu sync.Mutex

	// Status tracking
	stateMu   sync.RWMutex
	syncing   bool
	embedding bool
	syncProg  *rpc.ProgressInfo
	embedProg *rpc.ProgressInfo
}

func (s *indexerServer) HandleIndexSync(ctx context.Context, params rpc.SyncParams, send func(rpc.Progress)) (*rpc.Response, error) {
	return s.indexGate.Run(send, func(broadcast func(rpc.Progress)) (*rpc.Response, error) {
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

		ingester := indexer.NewIngester(s.db, s.embDB, nil, s.registry, s.embedder, nil)

		ingester.OnProgress = func(pIdx, pTotal, sIdx, sTotal int, message string) {
			s.stateMu.Lock()
			s.syncProg = &rpc.ProgressInfo{
				Done: sIdx, Total: sTotal,
				Project: pIdx, ProjectTotal: pTotal, ProjectName: message,
				Message: fmt.Sprintf("Project %d/%d %s", pIdx, pTotal, message),
			}
			s.stateMu.Unlock()

			broadcast(rpc.ProgressFrom(rpc.SyncProgressData{
				Project: pIdx, ProjectTotal: pTotal,
				Session: sIdx, SessionTotal: sTotal,
				Message: message,
			}))
		}

		projects, err := s.registry.ListAllProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
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

		return rpc.OKResponse(rpc.SyncData{Projects: totalProjects})
	})
}

func (s *indexerServer) HandleEmbedSync(ctx context.Context, params rpc.EmbedSyncParams, send func(rpc.Progress)) (*rpc.Response, error) {
	return s.embedGate.Run(send, func(broadcast func(rpc.Progress)) (*rpc.Response, error) {
		ctx, cancel := context.WithCancel(ctx)
		s.embedCancelMu.Lock()
		s.embedCancelFn = cancel
		s.embedCancelMu.Unlock()
		defer func() {
			cancel()
			s.embedCancelMu.Lock()
			s.embedCancelFn = nil
			s.embedCancelMu.Unlock()
		}()
		go func() {
			select {
			case <-ctx.Done():
			case <-s.shutdownCtx.Done():
				cancel()
			}
		}()

		// Serialize GPU access: only one of embed/summarize runs at a time.
		s.gpuMu.Lock()
		defer s.gpuMu.Unlock()

		// Lazy-init embedder if not already loaded
		if s.embedder == nil {
			cfg, err := config.Load()
			if err != nil || !cfg.Embedding.Enabled {
				return &rpc.Response{OK: false, Error: "embedding not enabled"}, nil
			}
			modelID := cfg.Embedding.Model
			tuilog.Log.Info("indexer: lazy-loading embedding model", "model", modelID)

			if err := embedding.EnsureModel(modelID, func(downloaded, total int64) {
				if total > 0 {
					broadcast(rpc.ProgressFrom(rpc.ModelDownloadProgressData{
						ModelDownload: true,
						Downloaded:    downloaded,
						Total:         total,
						Percent:       float64(downloaded) / float64(total) * 100,
					}))
				}
			}); err != nil {
				return nil, fmt.Errorf("ensure embedding model: %w", err)
			}

			e, err := embedding.NewEmbedder(modelID, "")
			if err != nil {
				return nil, fmt.Errorf("create embedder: %w", err)
			}

			if s.embDB == nil || s.embDBModel != e.EmbedModelID() {
				if s.embDB != nil {
					s.embDB.Close()
				}
				d, err := getEmbeddingsDB(e.EmbedModelID(), e.Dim())
				if err != nil {
					e.Close()
					return nil, fmt.Errorf("open embeddings database: %w", err)
				}
				s.embDB = d
				s.embDBModel = e.EmbedModelID()
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

		ingester := indexer.NewIngester(s.db, s.embDB, nil, s.registry, s.embedder, nil)
		ingester.Verbose = verbose

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

			broadcast(rpc.ProgressFrom(rpc.EmbedProgressData{
				Done: done, Total: total,
				Chunks: chunks, Entries: entries,
				SessionID:   sessionID,
				SessionPath: sessionPath,
				ElapsedMs:   elapsed.Milliseconds(),
			}))
		}
		ingester.OnEmbedChunkProgress = func(chunksDone, chunksTotal, tokensDone int, sessionID string) {
			s.stateMu.Lock()
			if s.embedProg != nil {
				s.embedProg.ChunksDone = chunksDone
				s.embedProg.ChunksTotal = chunksTotal
			}
			s.stateMu.Unlock()

			broadcast(rpc.ProgressFrom(rpc.EmbedChunkProgressData{
				ChunksDone:  chunksDone,
				ChunksTotal: chunksTotal,
				TokensDone:  tokensDone,
				SessionID:   sessionID,
			}))
		}

		if err := ingester.EmbedAllSessions(ctx); err != nil {
			tuilog.Log.Error("indexer: embedding pass failed", "error", err)
		}

		return &rpc.Response{OK: true}, nil
	})
}

func (s *indexerServer) HandleSummarizeSync(ctx context.Context, params rpc.SummarizeSyncParams, send func(rpc.Progress)) (*rpc.Response, error) {
	return s.sumGate.Run(send, func(broadcast func(rpc.Progress)) (*rpc.Response, error) {
		cfg, err := config.Load()
		if err != nil || !cfg.Summarization.Enabled {
			return &rpc.Response{OK: false, Error: "summarization not enabled"}, nil
		}

		// Serialize GPU access: only one of embed/summarize runs at a time.
		s.gpuMu.Lock()
		defer s.gpuMu.Unlock()

		modelID := cfg.Summarization.Model

		// Download model if needed.
		if err := summarize.EnsureModel(modelID, func(downloaded, total int64) {
			if total > 0 {
				broadcast(rpc.ProgressFrom(rpc.ModelDownloadProgressData{
					ModelDownload: true,
					Downloaded:    downloaded,
					Total:         total,
					Percent:       float64(downloaded) / float64(total) * 100,
				}))
			}
		}); err != nil {
			return nil, fmt.Errorf("ensure summarization model: %w", err)
		}

		summarizer, err := summarize.NewSummarizer(modelID, "")
		if err != nil {
			return nil, fmt.Errorf("create summarizer: %w", err)
		}
		defer summarizer.Close()

		// Lazy-init summaries DB.
		if s.sumDB == nil {
			d, err := getSummariesDB(modelID)
			if err != nil {
				return nil, fmt.Errorf("open summaries database: %w", err)
			}
			s.sumDB = d
		}

		ingester := indexer.NewIngester(s.db, nil, s.sumDB, s.registry, nil, summarizer)
		ingester.Verbose = verbose

		ingester.OnSummarizeProgress = func(done, total int, sessionID string, elapsed time.Duration) {
			broadcast(rpc.ProgressFrom(rpc.SummarizeProgressData{
				Done:      done,
				Total:     total,
				SessionID: sessionID,
				ElapsedMs: elapsed.Milliseconds(),
			}))
		}

		if err := ingester.SummarizeAllSessions(ctx); err != nil {
			tuilog.Log.Error("indexer: summarization pass failed", "error", err)
		}

		return &rpc.Response{OK: true}, nil
	})
}

func (s *indexerServer) HandleSearch(ctx context.Context, params rpc.SearchParams) (*rpc.Response, error) {
	svc := search.NewService(s.db, nil)

	opts := search.SearchOptions{
		Query:           params.Query,
		FilterProject:   params.Project,
		FilterSource:    strings.TrimSpace(strings.ToLower(params.Source)),
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

	return rpc.OKResponse(rpc.SearchData{
		Results:      results,
		TotalMatches: totalMatches,
	})
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
		FilterSource:   strings.TrimSpace(strings.ToLower(params.Source)),
		FilterTier:     params.Tier,
		Limit:          limit,
		MaxDistance:    maxDist,
		Diversity:      params.Diversity,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	return rpc.OKResponse(rpc.SemanticSearchData{Results: results})
}

func (s *indexerServer) HandleStats(ctx context.Context) (*rpc.Response, error) {
	var stats rpc.StatsData

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

	rows, err := s.db.QueryContext(ctx, "SELECT tool_name, count(*) AS cnt FROM entries WHERE tool_name != '' GROUP BY tool_name ORDER BY cnt DESC LIMIT 25")
	if err == nil {
		for rows.Next() {
			var tc rpc.ToolCount
			if err := rows.Scan(&tc.Name, &tc.Count); err == nil {
				stats.TopTools = append(stats.TopTools, tc)
			}
		}
		rows.Close()
	}

	return rpc.OKResponse(stats)
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

	return rpc.OKResponse(status)
}

func (s *indexerServer) HandleMetrics(ctx context.Context) (*rpc.Response, error) {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, fmt.Errorf("gather metrics: %w", err)
	}

	var buf bytes.Buffer
	for _, mf := range mfs {
		if _, err := expfmt.MetricFamilyToText(&buf, mf); err != nil {
			return nil, fmt.Errorf("encode metrics: %w", err)
		}
	}

	return rpc.OKResponse(rpc.MetricsData{Text: buf.String()})
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

		resp, _ := rpc.OKResponse(rpc.ConfigReloadData{EmbeddingEnabled: true})
		return resp, nil
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

		// Close cached embDB so it re-opens with new model path if re-enabled
		if s.embDB != nil {
			s.embDB.Close()
			s.embDB = nil
		}

		resp, _ := rpc.OKResponse(rpc.ConfigReloadData{EmbeddingEnabled: false})
		return resp, nil
	}

	// Both enabled — check if model changed
	if wantEnabled && wasEnabled && s.embedder != nil && s.embedder.EmbedModelID() != cfg.Embedding.Model {
		tuilog.Log.Info("indexer: config reload detected model change",
			"old", s.embedder.EmbedModelID(), "new", cfg.Embedding.Model)

		// Cancel any in-flight embed sync so it doesn't waste time on the old model.
		s.embedCancelMu.Lock()
		fn := s.embedCancelFn
		s.embedCancelMu.Unlock()
		if fn != nil {
			fn()
		}

		// Clear the cached embedder and DB; HandleEmbedSync will re-init with the new model.
		old := s.embedder
		s.embedder = nil
		old.Close()
		if s.embDB != nil {
			s.embDB.Close()
			s.embDB = nil
			s.embDBModel = ""
		}
		if s.watcher != nil {
			s.watcher.SetEmbedder(nil)
		}

		newModel := cfg.Embedding.Model
		go func() {
			// Wait for the cancelled sync to fully exit before attempting a
			// new sync — otherwise we'd subscribe to the dying run.
			s.embedGate.WaitIdle()
			shutdown := s.shutdownCtx.Err() != nil
			if shutdown {
				return
			}
			tuilog.Log.Info("indexer: starting post-model-change embed sync", "model", newModel)
			resp, err := s.HandleEmbedSync(s.shutdownCtx, rpc.EmbedSyncParams{}, func(rpc.Progress) {})
			if err != nil {
				tuilog.Log.Error("indexer: post-model-change embed sync error", "error", err)
			} else if resp != nil && !resp.OK {
				tuilog.Log.Error("indexer: post-model-change embed sync failed", "error", resp.Error)
			} else {
				tuilog.Log.Info("indexer: post-model-change embed sync complete")
			}
		}()

		resp, _ := rpc.OKResponse(rpc.ConfigReloadData{EmbeddingEnabled: true, ModelChanged: true})
		return resp, nil
	}

	// No change
	resp, _ := rpc.OKResponse(rpc.ConfigReloadData{EmbeddingEnabled: wantEnabled})
	return resp, nil
}

func runServer(cmdObj *cobra.Command, args []string) error {
	// 0. Load config
	cfg, err := config.Load()
	if err != nil {
		tuilog.Log.Warn("indexer: failed to load config, using defaults", "error", err)
		cfg = config.Default()
	}

	// 1. Open DBs
	tuilog.Log.Info("indexer: using database", "path", dbPath)
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
		w, err := indexer.NewWatcher(dbPath, embDBDir, registry, nil, cfg.Indexer.DebounceDuration())
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

		// If summarization is enabled, start summarize sync after embed sync
		if cfg.Summarization.Enabled {
			tuilog.Log.Info("indexer: starting initial summarize sync")
			resp, err := srv.HandleSummarizeSync(shutdownCtx, rpc.SummarizeSyncParams{}, func(rpc.Progress) {})
			if err != nil {
				tuilog.Log.Error("indexer: initial summarize sync error", "error", err)
			} else if resp != nil && !resp.OK {
				tuilog.Log.Error("indexer: initial summarize sync failed", "error", resp.Error)
			} else {
				tuilog.Log.Info("indexer: initial summarize sync complete")
			}
		}
	}()

	if !quiet {
		fmt.Fprint(os.Stderr, thinktI18n.Tf("indexer.server.running", "Indexer server running (PID: %d). Press Ctrl+C to stop.\n", os.Getpid()))
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
	if srv.sumDB != nil {
		srv.sumDB.Close()
	}

	tuilog.Log.Info("indexer: server stopped")
	return nil
}
