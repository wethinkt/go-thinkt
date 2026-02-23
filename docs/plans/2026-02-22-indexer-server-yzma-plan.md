# Indexer Server + Yzma Embeddings Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the CLI-based indexer with a long-running server that owns the DB and embedding model, and swap Apple's NLContextualEmbedding for Qwen3-Embedding via yzma.

**Architecture:** `thinkt-indexer server` is a persistent process that holds a DuckDB write connection and a loaded yzma model. CLI commands (`sync`, `search`, etc.) connect to it via a Unix socket using a JSON-over-newline RPC protocol, falling back to inline execution when no server is running.

**Tech Stack:** Go, yzma (llama.cpp bindings), DuckDB, fsnotify, Unix domain sockets

---

## Batch 1: Yzma Embedding Backend

### Task 1: Add yzma dependency and create in-process embedder

**Files:**
- Modify: `go.mod` (add yzma dependency)
- Create: `internal/indexer/embedding/yzma.go`
- Create: `internal/indexer/embedding/yzma_test.go`

**Step 1: Add yzma dependency**

```bash
go get github.com/hybridgroup/yzma@v1.9.0
```

**Step 2: Create the yzma embedder**

Create `internal/indexer/embedding/yzma.go`. This replaces the subprocess-based `Client`. The `Embedder` loads the yzma model once and exposes `Embed()` for batch embedding.

```go
package embedding

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/hybridgroup/yzma/pkg/download"
	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	DefaultModelName = "Qwen3-Embedding-0.6B-Q8_0.gguf"
	DefaultModelURL  = "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf"
	DefaultModelDir  = "models" // relative to ~/.thinkt/
	ModelID          = "qwen3-embedding-0.6b"
)

// Embedder wraps a yzma model for in-process text embedding.
type Embedder struct {
	mu    sync.Mutex
	model uintptr // llama.Model
	ctx   uintptr // llama.Context
	vocab uintptr // llama.Vocab
	dim   int
}

// NewEmbedder loads the yzma model from the given path.
// If modelPath is empty, uses DefaultModelPath().
func NewEmbedder(modelPath string) (*Embedder, error) {
	if modelPath == "" {
		var err error
		modelPath, err = DefaultModelPath()
		if err != nil {
			return nil, err
		}
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model not found at %s (run sync to auto-download)", modelPath)
	}

	// Load yzma runtime
	libPath, err := ensureRuntime()
	if err != nil {
		return nil, fmt.Errorf("failed to load yzma runtime: %w", err)
	}

	if err := llama.Load(libPath); err != nil {
		return nil, fmt.Errorf("llama.Load: %w", err)
	}

	llama.LogSet(llama.LogSilent())
	llama.Init()

	model, err := llama.ModelLoadFromFile(modelPath, llama.ModelDefaultParams())
	if err != nil {
		return nil, fmt.Errorf("failed to load model: %w", err)
	}

	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = 2048
	ctxParams.NBatch = 1024
	ctxParams.PoolingType = llama.PoolingTypeLast
	ctxParams.Embeddings = 1

	ctx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		llama.ModelFree(model)
		return nil, fmt.Errorf("failed to init context: %w", err)
	}

	dim := llama.ModelNEmbd(model)
	if dim <= 0 {
		dim = llama.ModelNEmbdOut(model)
	}

	return &Embedder{
		model: model,
		ctx:   ctx,
		vocab: llama.ModelGetVocab(model),
		dim:   dim,
	}, nil
}

// Dim returns the embedding dimension.
func (e *Embedder) Dim() int {
	return e.dim
}

// ModelID returns the model identifier for storage.
func (e *Embedder) ModelID() string {
	return ModelID
}

// Embed produces embedding vectors for the given texts.
// Thread-safe (holds a mutex during inference).
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	results := make([][]float32, 0, len(texts))
	for _, text := range texts {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		vec, err := e.embedOne(text)
		if err != nil {
			return results, fmt.Errorf("embed text: %w", err)
		}
		normalizeL2(vec)
		results = append(results, vec)
	}
	return results, nil
}

func (e *Embedder) embedOne(text string) ([]float32, error) {
	tokens := llama.Tokenize(e.vocab, text, true, true)
	if len(tokens) == 0 {
		return make([]float32, e.dim), nil
	}

	batch := llama.BatchGetOne(tokens)
	if _, err := llama.Decode(e.ctx, batch); err != nil {
		return nil, err
	}
	llama.Synchronize(e.ctx)

	vec, err := llama.GetEmbeddingsSeq(e.ctx, 0, e.dim)
	if err != nil {
		return nil, err
	}

	// Clear context memory for next call
	mem := llama.GetMemory(e.ctx)
	llama.MemoryClear(mem, false)

	return vec, nil
}

// Close unloads the model and frees resources.
func (e *Embedder) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	llama.Free(e.ctx)
	llama.ModelFree(e.model)
}

func normalizeL2(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	norm := float32(math.Sqrt(sum))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
}

// DefaultModelPath returns ~/.thinkt/models/Qwen3-Embedding-0.6B-Q8_0.gguf.
func DefaultModelPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".thinkt", DefaultModelDir, DefaultModelName), nil
}

// EnsureModel downloads the model if it doesn't exist.
// onProgress is called with download progress (can be nil).
func EnsureModel(onProgress func(downloaded, total int64)) error {
	modelPath, err := DefaultModelPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(modelPath); err == nil {
		return nil // already exists
	}

	dir := filepath.Dir(modelPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Use yzma's download helper
	return download.GetModelWithProgress(DefaultModelURL, dir, onProgress)
}

// ensureRuntime installs the llama.cpp runtime if needed and returns its path.
func ensureRuntime() (string, error) {
	// Check YZMA_LIB env var first
	if libPath := os.Getenv("YZMA_LIB"); libPath != "" {
		return libPath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	libPath := filepath.Join(home, ".yzma", "lib")

	// Check if runtime already exists
	// (yzma auto-installs on first Load if needed, but we check explicitly)
	return libPath, nil
}
```

Note: The exact yzma API (function signatures, types) may need adjustment based on the actual v1.9.0 API. Refer to `/Users/evan/wethinkt/yzma_play/examples/smollm2/main.go` for the working patterns. The model/context handles may be typed differently than `uintptr` — use whatever types yzma exports.

**Step 3: Write test**

Create `internal/indexer/embedding/yzma_test.go`:

```go
package embedding_test

import (
	"context"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func TestEmbedder_Embed(t *testing.T) {
	modelPath, err := embedding.DefaultModelPath()
	if err != nil {
		t.Fatal(err)
	}
	embedder, err := embedding.NewEmbedder(modelPath)
	if err != nil {
		t.Skipf("yzma model not available: %v", err)
	}
	defer embedder.Close()

	vecs, err := embedder.Embed(context.Background(), []string{
		"authentication timeout in login flow",
		"CI/CD pipeline with GitHub Actions",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != embedder.Dim() {
		t.Fatalf("expected dim %d, got %d", embedder.Dim(), len(vecs[0]))
	}

	// Vectors should be L2-normalized (magnitude ≈ 1.0)
	var mag float64
	for _, v := range vecs[0] {
		mag += float64(v) * float64(v)
	}
	if mag < 0.99 || mag > 1.01 {
		t.Fatalf("expected L2-normalized vector, magnitude = %f", mag)
	}

	t.Logf("Embedding dim: %d", embedder.Dim())
}
```

**Step 4: Run test**

```bash
CGO_ENABLED=1 go test ./internal/indexer/embedding/ -run TestEmbedder -v -timeout 60s
```

Expected: PASS (or SKIP if model not downloaded yet)

**Step 5: Commit**

```bash
git add go.mod go.sum internal/indexer/embedding/yzma.go internal/indexer/embedding/yzma_test.go
git commit -m "feat: add yzma-based in-process embedder for Qwen3-Embedding"
```

---

### Task 2: Update schema from FLOAT[512] to FLOAT[1024]

**Files:**
- Modify: `internal/indexer/db/schema/init.sql:66` (change FLOAT[512] to FLOAT[1024])
- Modify: `internal/indexer/ingester.go:415` (change FLOAT[512] cast)
- Modify: `internal/indexer/search/semantic.go:43,69` (change FLOAT[512] cast in queries)

**Step 1: Update init.sql**

In `internal/indexer/db/schema/init.sql`, line 66, change:
```sql
embedding   FLOAT[512] NOT NULL,
```
to:
```sql
embedding   FLOAT[1024] NOT NULL,
```

**Step 2: Update ingester.go embed storage**

In `internal/indexer/ingester.go`, line 415, change `?::FLOAT[512]` to `?::FLOAT[1024]`.

**Step 3: Update semantic search queries**

In `internal/indexer/search/semantic.go`:
- Line 43: change `?::FLOAT[512]` to `?::FLOAT[1024]`
- Line 69: change `?::FLOAT[512]` to `?::FLOAT[1024]`

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/indexer/... -timeout 60s
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/indexer/db/schema/init.sql internal/indexer/ingester.go internal/indexer/search/semantic.go
git commit -m "feat: update embedding schema from FLOAT[512] to FLOAT[1024] for Qwen3"
```

---

### Task 3: Wire yzma embedder into ingester, replace subprocess client

**Files:**
- Modify: `internal/indexer/ingester.go:18-52` (replace `embedding.Client` with `embedding.Embedder`)
- Modify: `internal/indexer/ingester.go:370-429` (update `embedSession` to use `Embedder.Embed`)
- Modify: `internal/indexer/cmd/sync.go:27` (pass embedder or nil)
- Modify: `internal/indexer/cmd/semantic_search.go:42-55` (use yzma for query embedding)

**Step 1: Update Ingester struct**

In `internal/indexer/ingester.go`, replace the `embedClient *embedding.Client` field (line 21) and `NewIngester` function (lines 30-40):

```go
type Ingester struct {
	db       *db.DB
	registry *thinkt.StoreRegistry
	embedder *embedding.Embedder // nil if embedding unavailable
	OnProgress    func(pIdx, pTotal, sIdx, sTotal int, message string)
	OnEmbedProgress func(done, total, chunks int, sessionID string, elapsed time.Duration)
}

func NewIngester(database *db.DB, registry *thinkt.StoreRegistry, embedder *embedding.Embedder) *Ingester {
	return &Ingester{
		db:       database,
		registry: registry,
		embedder: embedder,
	}
}

func (i *Ingester) HasEmbedder() bool {
	return i.embedder != nil
}

func (i *Ingester) Close() {
	// Embedder lifecycle is owned by the caller (server or CLI), not the ingester
}
```

**Step 2: Update embedSession**

In `internal/indexer/ingester.go`, update `embedSession` (around line 370) to use the yzma embedder:

```go
func (i *Ingester) embedSession(ctx context.Context, sessionID string, entries []thinkt.Entry) (int, error) {
	if i.embedder == nil {
		return 0, nil
	}

	var entryTexts []embedding.EntryText
	for _, e := range entries {
		text := embedding.ExtractText(e)
		if text == "" {
			continue
		}
		entryTexts = append(entryTexts, embedding.EntryText{
			UUID: e.UUID, SessionID: sessionID, Text: text,
		})
	}
	if len(entryTexts) == 0 {
		return 0, nil
	}

	requests, mapping := embedding.PrepareEntries(entryTexts, 2000, 200)

	// Extract just the text strings for yzma
	texts := make([]string, len(requests))
	for idx, r := range requests {
		texts[idx] = r.Text
	}

	vectors, err := i.embedder.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embedding failed: %w", err)
	}

	stored := 0
	for idx, m := range mapping {
		if idx >= len(vectors) {
			break
		}
		id := requests[idx].ID
		_, err := i.db.ExecContext(ctx, `
			INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?::FLOAT[1024], ?)
			ON CONFLICT (id) DO UPDATE SET
				embedding = excluded.embedding,
				text_hash = excluded.text_hash`,
			id, m.SessionID, m.EntryUUID, m.ChunkIndex,
			i.embedder.ModelID(), i.embedder.Dim(), vectors[idx], m.TextHash,
		)
		if err != nil {
			return stored, fmt.Errorf("store embedding %s: %w", id, err)
		}
		stored++
	}

	return stored, nil
}
```

**Step 3: Update all NewIngester call sites**

Search for all `NewIngester(` calls and update to pass an embedder or nil:

- `internal/indexer/cmd/sync.go:27`: Create embedder, pass to NewIngester
- `internal/indexer/watcher.go:257`: The watcher's `handleFileChange` creates an Ingester — pass the watcher's embedder
- `internal/indexer/cmd/semantic_search.go:42-55`: Replace `embedding.NewClient()` with `embedding.NewEmbedder()` for query embedding

For `sync.go`:
```go
var embedder *embedding.Embedder
modelPath, _ := embedding.DefaultModelPath()
if e, err := embedding.NewEmbedder(modelPath); err == nil {
	embedder = e
	defer e.Close()
}
ingester := indexer.NewIngester(database, registry, embedder)
```

For `semantic_search.go`, replace the `embedding.NewClient()` block with:
```go
embedder, err := embedding.NewEmbedder("")
if err != nil {
	return fmt.Errorf("semantic search unavailable: %w", err)
}
defer embedder.Close()

vecs, err := embedder.Embed(context.Background(), []string{queryText})
if err != nil {
	return fmt.Errorf("failed to embed query: %w", err)
}
if len(vecs) == 0 {
	return fmt.Errorf("embedding returned no results for query")
}
// Use vecs[0] as QueryEmbedding
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/indexer/... -timeout 60s
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/indexer/ingester.go internal/indexer/cmd/sync.go internal/indexer/cmd/semantic_search.go internal/indexer/watcher.go
git commit -m "feat: wire yzma embedder into ingester, replace subprocess client"
```

---

### Task 4: Add model migration (drop old embeddings on model change)

**Files:**
- Modify: `internal/indexer/ingester.go` (add `MigrateEmbeddings` method)

**Step 1: Add migration check**

Add to `internal/indexer/ingester.go`:

```go
// MigrateEmbeddings drops embeddings if the stored model doesn't match the current one.
func (i *Ingester) MigrateEmbeddings(ctx context.Context) error {
	if i.embedder == nil {
		return nil
	}

	var count int
	err := i.db.QueryRowContext(ctx, `SELECT count(*) FROM embeddings WHERE model != ?`, i.embedder.ModelID()).Scan(&count)
	if err != nil {
		return nil // table may not exist yet
	}
	if count == 0 {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Dropping %d embeddings from old model (will re-embed with %s)\n", count, i.embedder.ModelID())
	_, err = i.db.ExecContext(ctx, `DELETE FROM embeddings WHERE model != ?`, i.embedder.ModelID())
	return err
}
```

Call this from `sync.go` after creating the ingester, and from the server on startup.

**Step 2: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/indexer/... -timeout 60s
```

**Step 3: Commit**

```bash
git add internal/indexer/ingester.go internal/indexer/cmd/sync.go
git commit -m "feat: auto-migrate embeddings when model changes"
```

---

### Task 5: Remove Apple embedding backend

**Files:**
- Delete: `tools/thinkt-embed-apple/main.swift`
- Delete: `internal/indexer/embedding/client.go`
- Modify: `Taskfile.yml` (remove `build:embed-apple` task and from `build` deps)
- Modify: `internal/indexer/cmd/stats.go:66` (remove `embedding.Available()` reference)
- Modify: `internal/indexer/cmd/sync.go` (remove `embedding` import if unused)
- Update: `internal/indexer/embedding/integration_test.go` (use yzma embedder instead of Client)

**Step 1: Delete files**

```bash
rm -rf tools/thinkt-embed-apple/
rm internal/indexer/embedding/client.go
```

**Step 2: Update Taskfile.yml**

Remove `build:embed-apple` task entirely. Remove it from the `build` task's deps list (line 38).

**Step 3: Update stats.go**

Replace the `embedding.Available()` check with a check for whether the model file exists:
```go
modelPath, _ := embedding.DefaultModelPath()
if _, err := os.Stat(modelPath); err == nil {
	fmt.Printf("Embedder:    %s (available)\n", embedding.ModelID)
} else {
	fmt.Printf("Embedder:    %s (model not downloaded)\n", embedding.ModelID)
}
```

**Step 4: Update integration test**

In `internal/indexer/embedding/integration_test.go`, replace `embedding.NewClient()` with `embedding.NewEmbedder("")` and `client.EmbedBatch()` with `embedder.Embed()`.

**Step 5: Build and test**

```bash
CGO_ENABLED=1 go build ./... && go test ./internal/indexer/... -timeout 60s
```

**Step 6: Commit**

```bash
git add -A
git commit -m "chore: remove Apple embedding backend, fully replaced by yzma"
```

---

## Checkpoint: Review Batch 1

Verify:
1. `go build ./...` succeeds
2. `go test ./internal/indexer/... -timeout 60s` passes
3. No references to `thinkt-embed-apple` or `embedding.Client` remain
4. Embedding dimension is 1024 throughout

---

## Batch 2: RPC Protocol & Socket Server

### Task 6: Create RPC protocol types and socket server

**Files:**
- Create: `internal/indexer/rpc/protocol.go`
- Create: `internal/indexer/rpc/server.go`
- Create: `internal/indexer/rpc/client.go`
- Create: `internal/indexer/rpc/server_test.go`

**Step 1: Define protocol types**

Create `internal/indexer/rpc/protocol.go`:

```go
package rpc

import "encoding/json"

// Request is a JSON-over-newline RPC request.
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is a final RPC response.
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Progress is a streaming progress update.
type Progress struct {
	Progress bool            `json:"progress"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// SyncParams for the sync method.
type SyncParams struct {
	Force bool `json:"force,omitempty"`
}

// SearchParams for the search method.
type SearchParams struct {
	Query           string `json:"query"`
	Project         string `json:"project,omitempty"`
	Source          string `json:"source,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	LimitPerSession int    `json:"limit_per_session,omitempty"`
	CaseSensitive   bool   `json:"case_sensitive,omitempty"`
	Regex           bool   `json:"regex,omitempty"`
}

// SemanticSearchParams for the semantic_search method.
type SemanticSearchParams struct {
	Query       string  `json:"query"`
	Project     string  `json:"project,omitempty"`
	Source      string  `json:"source,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	MaxDistance  float64 `json:"max_distance,omitempty"`
}

// StatusData returned by the status method.
type StatusData struct {
	State         string       `json:"state"` // "idle", "syncing", "embedding"
	SyncProgress  *ProgressInfo `json:"sync_progress,omitempty"`
	EmbedProgress *ProgressInfo `json:"embed_progress,omitempty"`
	Model         string       `json:"model"`
	ModelDim      int          `json:"model_dim"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	Watching      bool         `json:"watching"`
}

type ProgressInfo struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}
```

**Step 2: Create the socket server**

Create `internal/indexer/rpc/server.go`. This listens on a Unix socket and dispatches methods to a handler interface:

```go
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

// Handler processes RPC methods.
type Handler interface {
	HandleSync(ctx context.Context, params SyncParams, send func(Progress)) (json.RawMessage, error)
	HandleSearch(ctx context.Context, params SearchParams) (json.RawMessage, error)
	HandleSemanticSearch(ctx context.Context, params SemanticSearchParams) (json.RawMessage, error)
	HandleStats(ctx context.Context) (json.RawMessage, error)
	HandleStatus(ctx context.Context) (json.RawMessage, error)
}

// Server listens on a Unix socket for RPC requests.
type Server struct {
	socketPath string
	handler    Handler
	listener   net.Listener
	wg         sync.WaitGroup
}

// NewServer creates an RPC server.
func NewServer(socketPath string, handler Handler) *Server {
	return &Server{
		socketPath: socketPath,
		handler:    handler,
	}
}

// Start begins accepting connections.
func (s *Server) Start() error {
	// Remove stale socket
	os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.socketPath, err)
	}
	s.listener = ln

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go s.handleConn(conn)
		}
	}()

	return nil
}

// Stop closes the listener and waits for connections to drain.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
	s.wg.Wait()
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !scanner.Scan() {
		return
	}

	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeJSON(conn, Response{OK: false, Error: "invalid request"})
		return
	}

	ctx := context.Background()
	var data json.RawMessage
	var err error

	switch req.Method {
	case "sync":
		var params SyncParams
		json.Unmarshal(req.Params, &params)
		send := func(p Progress) {
			writeJSON(conn, p)
		}
		data, err = s.handler.HandleSync(ctx, params, send)
	case "search":
		var params SearchParams
		json.Unmarshal(req.Params, &params)
		data, err = s.handler.HandleSearch(ctx, params)
	case "semantic_search":
		var params SemanticSearchParams
		json.Unmarshal(req.Params, &params)
		data, err = s.handler.HandleSemanticSearch(ctx, params)
	case "stats":
		data, err = s.handler.HandleStats(ctx)
	case "status":
		data, err = s.handler.HandleStatus(ctx)
	default:
		err = fmt.Errorf("unknown method: %s", req.Method)
	}

	if err != nil {
		writeJSON(conn, Response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(conn, Response{OK: true, Data: data})
}

func writeJSON(conn net.Conn, v any) {
	data, _ := json.Marshal(v)
	data = append(data, '\n')
	conn.Write(data)
}
```

**Step 3: Create the client**

Create `internal/indexer/rpc/client.go`:

```go
package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// DefaultSocketPath returns ~/.thinkt/indexer.sock.
func DefaultSocketPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.thinkt/indexer.sock"
}

// ServerAvailable returns true if the socket exists and is connectable.
func ServerAvailable() bool {
	path := DefaultSocketPath()
	conn, err := net.Dial("unix", path)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Call sends an RPC request and returns the final response.
// progressFn is called for each progress update (can be nil).
func Call(method string, params any, progressFn func(Progress)) (*Response, error) {
	conn, err := net.Dial("unix", DefaultSocketPath())
	if err != nil {
		return nil, fmt.Errorf("connect to indexer server: %w", err)
	}
	defer conn.Close()

	req := Request{Method: method}
	if params != nil {
		raw, _ := json.Marshal(params)
		req.Params = raw
	}

	data, _ := json.Marshal(req)
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Try as progress first
		var prog Progress
		if json.Unmarshal(line, &prog) == nil && prog.Progress {
			if progressFn != nil {
				progressFn(prog)
			}
			continue
		}

		// Must be final response
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return nil, fmt.Errorf("invalid response: %w", err)
		}
		return &resp, nil
	}

	return nil, fmt.Errorf("connection closed without response")
}
```

**Step 4: Write server test**

Create `internal/indexer/rpc/server_test.go` — test the socket server with a mock handler, verifying request dispatch and response format.

**Step 5: Run tests**

```bash
go test ./internal/indexer/rpc/ -v -timeout 30s
```

**Step 6: Commit**

```bash
git add internal/indexer/rpc/
git commit -m "feat: add Unix socket RPC protocol, server, and client"
```

---

### Task 7: Create `server` command, replace `watch`

**Files:**
- Create: `internal/indexer/cmd/server.go`
- Delete: `internal/indexer/cmd/watch.go`
- Modify: `internal/config/instances.go:24` (change `InstanceIndexerWatch` to `InstanceIndexerServer`)

**Step 1: Create server command**

Create `internal/indexer/cmd/server.go`. This is the main long-running command. It:
1. Opens DuckDB
2. Ensures model is downloaded, loads yzma embedder
3. Starts RPC socket server
4. Registers instance
5. Optionally starts file watcher
6. Runs initial sync
7. Starts background embed worker
8. Waits for signal

The server implements the `rpc.Handler` interface by delegating to the ingester, search service, etc.

**Step 2: Delete watch.go**

```bash
rm internal/indexer/cmd/watch.go
```

**Step 3: Update instance type**

In `internal/config/instances.go`, line 24, change:
```go
InstanceIndexerWatch InstanceType = "indexer-watch"
```
to:
```go
InstanceIndexerServer InstanceType = "indexer-server"
```

Update any references to `InstanceIndexerWatch` in the codebase.

**Step 4: Build and test**

```bash
CGO_ENABLED=1 go build ./cmd/thinkt-indexer && go test ./... -timeout 60s
```

**Step 5: Commit**

```bash
git add -A
git commit -m "feat: add thinkt-indexer server command, replace watch"
```

---

### Task 8: Update CLI commands to try RPC first

**Files:**
- Modify: `internal/indexer/cmd/sync.go` (try RPC, fall back to inline)
- Modify: `internal/indexer/cmd/search.go` (try RPC, fall back to inline)
- Modify: `internal/indexer/cmd/semantic_search.go` (try RPC, fall back to inline)
- Modify: `internal/indexer/cmd/stats.go` (try RPC, fall back to inline)

**Step 1: Add RPC-first pattern to each command**

Each command follows this pattern:

```go
// Try RPC first
if rpc.ServerAvailable() {
	resp, err := rpc.Call("method", params, progressFn)
	if err == nil {
		// Handle response
		return nil
	}
	// Fall through to inline if RPC fails
}

// Inline fallback (existing code)
```

The `sync` command streams progress from the server via the `progressFn` callback.

**Step 2: Build and test**

```bash
CGO_ENABLED=1 go build ./cmd/thinkt-indexer && go test ./internal/indexer/cmd/ -timeout 60s
```

**Step 3: Commit**

```bash
git add internal/indexer/cmd/
git commit -m "feat: CLI commands try RPC server first, fall back to inline"
```

---

## Checkpoint: Review Batch 2

Verify:
1. `thinkt-indexer server` starts, listens on socket, watches files
2. `thinkt-indexer sync` connects to server when running, falls back to inline
3. `thinkt-indexer search` and `thinkt-indexer semantic search` work via RPC
4. `thinkt-indexer stats` works via RPC
5. All tests pass

---

## Batch 3: Integration & Cleanup

### Task 9: Update thinkt serve to launch indexer server

**Files:**
- Modify: `internal/server/indexer_api.go` (launch `thinkt-indexer server` instead of `thinkt-indexer watch`)

**Step 1: Update the launch logic**

Find where thinkt launches the watch process and change it to launch `thinkt-indexer server` instead. Use `config.StartBackground()` to detach it.

**Step 2: Build and test**

```bash
go build ./cmd/thinkt && CGO_ENABLED=1 go test ./internal/server/ -timeout 30s
```

**Step 3: Commit**

```bash
git add internal/server/
git commit -m "feat: thinkt serve launches thinkt-indexer server"
```

---

### Task 10: Update Watcher to use shared embedder

**Files:**
- Modify: `internal/indexer/watcher.go` (accept embedder, pass to ingester)

**Step 1: Update Watcher struct**

The `Watcher` should accept an `*embedding.Embedder` and pass it to each `Ingester` it creates in `handleFileChange`. This way the server's embedder is shared.

```go
type Watcher struct {
	dbPath       string
	registry     *thinkt.StoreRegistry
	embedder     *embedding.Embedder // shared, owned by server
	// ... existing fields
}
```

Update `NewWatcher` to accept the embedder, and `handleFileChange` to pass it to `NewIngester`.

**Step 2: Build and test**

```bash
CGO_ENABLED=1 go build ./cmd/thinkt-indexer && go test ./internal/indexer/... -timeout 60s
```

**Step 3: Commit**

```bash
git add internal/indexer/watcher.go
git commit -m "refactor: watcher shares server's embedder instance"
```

---

### Task 11: Update integration test for yzma

**Files:**
- Modify: `internal/indexer/embedding/integration_test.go`

**Step 1: Update test to use yzma embedder**

Replace all `embedding.NewClient()` / `client.EmbedBatch()` calls with `embedding.NewEmbedder()` / `embedder.Embed()`. Update the model name and dimension assertions.

**Step 2: Run test**

```bash
CGO_ENABLED=1 go test ./internal/indexer/embedding/ -run TestEndToEnd -v -timeout 60s
```

**Step 3: Commit**

```bash
git add internal/indexer/embedding/integration_test.go
git commit -m "test: update integration test for yzma embedder"
```

---

### Task 12: Final cleanup and documentation

**Files:**
- Modify: `Taskfile.yml` (remove `build:embed-apple` references if any remain)
- Verify: no remaining references to `thinkt-embed-apple`, `embedding.Client`, `FLOAT[512]`

**Step 1: Grep for stale references**

```bash
grep -r "thinkt-embed-apple" --include="*.go" --include="*.yml" --include="*.md" .
grep -r "FLOAT\[512\]" --include="*.go" --include="*.sql" .
grep -r "embedding.Client" --include="*.go" .
grep -r "embedding.Available" --include="*.go" .
```

Fix any remaining references.

**Step 2: Run full test suite**

```bash
CGO_ENABLED=1 go test ./... -timeout 120s
```

**Step 3: Commit**

```bash
git add -A
git commit -m "chore: final cleanup of Apple embedding references"
```

---

## Checkpoint: Review Batch 3

Verify:
1. Full build: `go build ./...`
2. Full tests: `CGO_ENABLED=1 go test ./... -timeout 120s`
3. `thinkt-indexer server` starts, syncs, embeds, handles RPC
4. `thinkt serve` launches indexer server automatically
5. No references to old Apple embedding backend
6. Embedding dimension is 1024 everywhere
