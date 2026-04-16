package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/index"
	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/server"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// startBackgroundSync runs a best-effort idempotent sync in a goroutine and, if the
// thinkt server isn't already running locally, starts an index watcher for the
// duration of the returned context. The returned function stops the watcher and
// closes the index DB; safe to call if nothing was started.
func startBackgroundSync(ctx context.Context, cfg config.Config) func() {
	stop := func() {}
	if !cfg.Indexer.Watch {
		return stop
	}

	dbPath, err := indexdb.DefaultPath()
	if err != nil {
		tuilog.Log.Warn("bgsync: resolve db path failed", "error", err)
		return stop
	}

	database, err := indexdb.Open(dbPath)
	if err != nil {
		tuilog.Log.Warn("bgsync: open index db failed", "error", err)
		return stop
	}

	registry := CreateSourceRegistry()

	// Quick-scan in a goroutine. Idempotent thanks to sync_state.
	go func() {
		ingester := index.NewIngester(database, registry)
		projects, err := registry.ListAllProjects(ctx)
		if err != nil {
			tuilog.Log.Warn("bgsync: list projects failed", "error", err)
			return
		}
		total := len(projects)
		for i, p := range projects {
			if ctx.Err() != nil {
				return
			}
			if err := ingester.IngestProject(ctx, p, i+1, total); err != nil {
				tuilog.Log.Warn("bgsync: ingest project failed", "project", p.Name, "error", err)
			}
		}
		tuilog.Log.Info("bgsync: quick-scan complete", "projects", total)
	}()

	// If thinkt server is already running locally, let it own the watcher.
	if serverRunning(fmt.Sprintf("127.0.0.1:%d", server.DefaultPortServer)) {
		tuilog.Log.Info("bgsync: thinkt server detected, skipping TUI watcher")
		return func() { _ = database.Close() }
	}

	watcher, err := index.NewWatcher(database, registry, cfg.Indexer.DebounceDuration())
	if err != nil {
		tuilog.Log.Warn("bgsync: watcher creation failed", "error", err)
		return func() { _ = database.Close() }
	}
	watcher.SetRescanInterval(cfg.Indexer.RescanIntervalDuration())
	if err := watcher.Start(ctx); err != nil {
		tuilog.Log.Warn("bgsync: watcher start failed", "error", err)
		return func() { _ = database.Close() }
	}

	return func() {
		_ = watcher.Stop()
		_ = database.Close()
	}
}

// serverRunning returns true if something is accepting HTTP on the given address.
func serverRunning(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	// Confirm it speaks HTTP (avoid false positives from squatters).
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/info")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}
