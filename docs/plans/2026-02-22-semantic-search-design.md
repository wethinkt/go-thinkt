# Semantic Search with Apple Intelligence On-Device Embeddings

## Overview

Add meaning-based search to thinkt using Apple's NLContextualEmbedding framework for on-device text embeddings. Users can search sessions with natural language queries (e.g., "that conversation about auth timeouts") instead of exact keyword matching.

Semantic search is additive — existing text search remains unchanged.

## Architecture

```
┌─────────────┐      stdin/stdout       ┌─────────────────────┐
│  Go Indexer  │ ───── JSON lines ─────▶ │  thinkt-embed-apple │
│  (thinkt)    │ ◀──── JSON lines ────── │  (Swift CLI)        │
└──────┬───────┘                         └─────────────────────┘
       │                                   NLContextualEmbedding
       │ store/query                       512-dim FLOAT vectors
       ▼
┌──────────────┐
│   DuckDB     │
│  embeddings  │
│  + HNSW idx  │
└──────────────┘
```

## Components

### 1. Swift CLI: `thinkt-embed-apple`

A minimal Swift binary that wraps Apple's NLContextualEmbedding framework.

**Source:** `tools/thinkt-embed-apple/`

**Interface:**
- Reads JSON lines from stdin: `{"id": "entry-uuid", "text": "..."}`
- Writes JSON lines to stdout: `{"id": "entry-uuid", "embedding": [0.1, 0.2, ...], "dim": 512}`
- Errors go to stderr
- Loads the model once on startup, processes all input lines, exits

**Implementation:**
- Uses `NLContextualEmbedding(script: .latin)` to find the model
- Calls `embedding.load()` once at startup (~546ms cold, ~45ms warm)
- Per-item embedding via `embeddingResult(for:language:)` (~8ms each)
- Averages token vectors to produce a single sentence embedding per input
- Outputs FLOAT (32-bit) precision — sufficient for similarity search
- Handles `requestAssets()` on first run if model not yet downloaded

**Naming convention:** `thinkt-embed-{platform}`. Future backends: `thinkt-embed-ollama`, `thinkt-embed-onnx`, etc.

### 2. Indexer Integration

The Go indexer (`internal/indexer/`) calls `thinkt-embed-apple` during session ingestion.

**During `IngestSession()`:**
1. Read entries and extract all text content (user prompts, assistant text, tool results, thinking)
2. Skip entries with < 10 characters of meaningful text
3. Chunk entries longer than ~2000 characters with ~200 character overlap
4. Pipe chunks as JSON lines to `thinkt-embed-apple` subprocess
5. Read back embedding vectors
6. Store embeddings in DuckDB

**Change detection:** Only embed new or changed entries. Use `text_hash` (SHA-256 of chunk text) to skip re-embedding unchanged content. Piggybacks on existing `sync_state` file modification tracking.

**Watch mode:** When the file watcher detects a session change, re-index and embed any new entries.

**Graceful degradation:** If `thinkt-embed-apple` is not in PATH (e.g., on Linux), the indexer skips embedding silently. Semantic search returns a clear error: "semantic search unavailable: thinkt-embed-apple not found".

### 3. Storage (DuckDB)

**New table:**

```sql
CREATE TABLE IF NOT EXISTS embeddings (
    id          VARCHAR PRIMARY KEY,    -- "{entry_uuid}_{chunk_index}"
    session_id  VARCHAR NOT NULL,
    entry_uuid  VARCHAR NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    model       VARCHAR NOT NULL,       -- e.g. "apple-nlcontextual-v1"
    dim         INTEGER NOT NULL,       -- 512 for Apple's model
    embedding   FLOAT[512] NOT NULL,
    text_hash   VARCHAR NOT NULL,       -- SHA-256, detect changes without re-embedding
    created_at  TIMESTAMP DEFAULT current_timestamp,
    UNIQUE(entry_uuid, chunk_index, model)
);

-- HNSW index for fast cosine similarity search
CREATE INDEX idx_embeddings_vec ON embeddings USING HNSW (embedding)
    WITH (metric = 'cosine');
```

**Design choices:**
- `FLOAT[512]` — fixed-size array required by DuckDB VSS extension for HNSW indexing
- `model` column — partitions embeddings by backend, prevents comparing incompatible vectors
- `dim` column — denormalized for validation, enables future multi-dimension support
- `text_hash` — avoids re-embedding unchanged content
- No raw text stored — privacy-first, consistent with existing design
- For future models with different dimensions: add dimension-specific tables (e.g., `embeddings_768`)

**DuckDB VSS extension:**
- Provides HNSW (Hierarchical Navigable Small Worlds) indexing
- Supports `array_cosine_distance()` for similarity queries
- Still experimental — WAL recovery not implemented
- Deletions are lazy; periodic `PRAGMA hnsw_compact_index` needed

### 4. Semantic Search Query Flow

1. User submits a natural language query
2. thinkt pipes query text to `thinkt-embed-apple`, gets 512-dim vector back
3. DuckDB query using HNSW-accelerated cosine distance:

```sql
SELECT e.session_id, e.entry_uuid, e.chunk_index,
       array_cosine_distance(e.embedding, ?::FLOAT[512]) AS distance
FROM embeddings e
WHERE e.model = ?
ORDER BY distance ASC
LIMIT ?;
```

4. Results ranked by similarity (lower distance = more similar)
5. Optional filters: project, source
6. Default minimum similarity threshold ~0.75 (configurable)

**Integration points:**
- New `semantic_search` MCP tool alongside existing `search_sessions`
- New CLI flag: `thinkt search --semantic "query"` or dedicated subcommand
- Existing text search unchanged

## Benchmarks (measured on-device)

| Operation | Latency |
|-----------|---------|
| Cold model load | 546ms |
| Warm model reload | 45ms |
| Per-embedding | ~8ms |
| Batch (100 items) | 789ms (7.9ms/item) |
| Vector dimension | 512 |

A session with 100 entries embeds in under 1 second (plus 546ms startup).

## Error Handling & Edge Cases

- **Binary not found:** Embedding is optional. Indexer skips it, semantic search returns descriptive error.
- **Model assets not downloaded:** Swift CLI handles `requestAssets()` on first run, reports progress to stderr.
- **Empty/short entries:** Skip entries with < 10 chars of meaningful text.
- **Long entries:** Chunk at ~2000 chars with ~200 char overlap. Each chunk is a separate embedding row.
- **HNSW index corruption:** Embeddings table is rebuildable — `DROP INDEX` + `CREATE INDEX`. Raw embedding rows are the source of truth.
- **Platform gating:** `model` column ensures Apple embeddings on Mac aren't queried on platforms without a compatible model.
- **DuckDB concurrency:** Same constraints as existing indexer — lazy pool with copy-on-read fallback.

## Scope

**In scope (MVP):**
- `thinkt-embed-apple` Swift CLI
- Indexer embedding pipeline (sync + watch)
- DuckDB embeddings table with HNSW index
- Semantic search MCP tool
- Semantic search CLI

**Out of scope (MVP):**
- Session summarization
- Hybrid text + semantic search ranking
- Web UI integration

## Future: Cross-Platform Backend via kronk

[kronk](https://github.com/ardanlabs/kronk) is a Go SDK for local LLM inference via llama.cpp with hardware acceleration (Metal on macOS, CUDA on Linux). It supports embedding generation from Go directly using open-source GGUF models (e.g., `nomic-embed-text`, `bge-small`).

**Plan:** Use kronk as the cross-platform embedding backend (`thinkt-embed-kronk` or built directly into the Go indexer) for Linux and Windows. The `model` column in the embeddings table already supports multiple backends — Apple embeddings and kronk embeddings coexist without conflict.

| | thinkt-embed-apple | kronk |
|---|---|---|
| Platform | macOS only | macOS, Linux, Windows |
| Dependencies | None (ships with OS) | llama.cpp binding + GGUF model download |
| Integration | Swift CLI via stdin/stdout | Pure Go, in-process |
| Model | Apple NLContextualEmbedding (512-dim) | Any GGUF embedding model |

---

# Semantic Search Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add semantic search to thinkt using Apple's NLContextualEmbedding for on-device embeddings, with a Swift CLI helper and DuckDB VSS storage.

**Architecture:** A Swift CLI (`thinkt-embed-apple`) generates 512-dim embeddings via stdin/stdout JSON lines. The Go indexer pipes entry text through it during ingestion and stores vectors in a DuckDB `embeddings` table with HNSW indexing. A new `semantic-search` CLI command and MCP tool query the index.

**Tech Stack:** Swift (NLContextualEmbedding), Go (indexer/search/MCP), DuckDB (VSS extension, HNSW), Cobra (CLI)

---

### Task 1: Swift CLI — `thinkt-embed-apple`

**Files:**
- Create: `tools/thinkt-embed-apple/main.swift`
- Create: `tools/thinkt-embed-apple/Package.swift` (optional, swiftc is fine)

**Step 1: Write the Swift CLI**

```swift
import Foundation
import NaturalLanguage

struct EmbedRequest: Decodable {
    let id: String
    let text: String
}

struct EmbedResponse: Encodable {
    let id: String
    let embedding: [Float]
    let dim: Int
}

// Find model
guard let model = NLContextualEmbedding(script: .latin) ??
                  NLContextualEmbedding(language: .english) else {
    fputs("ERROR: No contextual embedding model found\n", stderr)
    exit(1)
}

// Download assets if needed
if !model.hasAvailableAssets {
    fputs("Downloading embedding model assets...\n", stderr)
    let sem = DispatchSemaphore(value: 0)
    model.requestAssets { _, error in
        if let error = error {
            fputs("ERROR: Failed to download assets: \(error)\n", stderr)
            exit(1)
        }
        sem.signal()
    }
    sem.wait()
}

// Load model once
do {
    try model.load()
} catch {
    fputs("ERROR: Failed to load model: \(error)\n", stderr)
    exit(1)
}

let encoder = JSONEncoder()
let decoder = JSONDecoder()

// Process stdin line by line
while let line = readLine() {
    guard !line.isEmpty else { continue }

    guard let data = line.data(using: .utf8),
          let req = try? decoder.decode(EmbedRequest.self, from: data) else {
        fputs("WARN: Invalid JSON input, skipping\n", stderr)
        continue
    }

    guard let result = try? model.embeddingResult(for: req.text, language: .english) else {
        fputs("WARN: Failed to embed id=\(req.id), skipping\n", stderr)
        continue
    }

    // Average token vectors into a single sentence embedding
    var vector = [Float](repeating: 0, count: model.dimension)
    var tokenCount = 0
    result.enumerateTokenVectors(in: req.text.startIndex..<req.text.endIndex) { tokenVector, _ in
        for (j, v) in tokenVector.enumerated() {
            vector[j] += Float(v)
        }
        tokenCount += 1
        return true
    }
    if tokenCount > 0 {
        for j in 0..<vector.count {
            vector[j] /= Float(tokenCount)
        }
    }

    let resp = EmbedResponse(id: req.id, embedding: vector, dim: model.dimension)
    if let jsonData = try? encoder.encode(resp),
       let jsonStr = String(data: jsonData, encoding: .utf8) {
        print(jsonStr)
        fflush(stdout)
    }
}

model.unload()
```

**Step 2: Compile and test manually**

```bash
cd tools/thinkt-embed-apple
swiftc -O main.swift -o thinkt-embed-apple
echo '{"id":"test1","text":"debugging the auth timeout"}' | ./thinkt-embed-apple
```

Expected: JSON line with `id`, `embedding` (array of 512 floats), `dim: 512`.

**Step 3: Add build task to Taskfile.yml**

Modify: `Taskfile.yml` — add after the `build:indexer` task (~line 66):

```yaml
  build:embed-apple:
    desc: Build the thinkt-embed-apple Swift CLI (macOS only)
    platforms: [darwin]
    sources:
      - ./tools/thinkt-embed-apple/*.swift
    generates:
      - "{{.BIN_DIR}}/thinkt-embed-apple"
    cmds:
      - swiftc -O tools/thinkt-embed-apple/main.swift -o {{.BIN_DIR}}/thinkt-embed-apple
```

Update the `build` task deps to include `build:embed-apple`.

**Step 4: Verify build task works**

```bash
task build:embed-apple
echo '{"id":"t1","text":"hello world"}' | ./bin/thinkt-embed-apple
```

Expected: JSON output with 512-dim embedding.

**Step 5: Commit**

```bash
git add tools/thinkt-embed-apple/main.swift Taskfile.yml
git commit -m "feat: add thinkt-embed-apple Swift CLI for on-device embeddings"
```

---

### Task 2: DuckDB Schema — Embeddings Table

**Files:**
- Modify: `internal/indexer/db/schema/init.sql` (append after line 56)
- Modify: `internal/indexer/db/db.go:43-67` (load VSS extension before schema)

**Step 1: Write the failing test**

Create: `internal/indexer/db/embeddings_test.go`

```go
package db_test

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/wethinkt/go-thinkt/internal/indexer/db"
)

func TestEmbeddingsTableExists(t *testing.T) {
    path := filepath.Join(t.TempDir(), "test.db")
    d, err := db.Open(path)
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    // Verify embeddings table exists
    var count int
    err = d.QueryRow("SELECT count(*) FROM information_schema.tables WHERE table_name = 'embeddings'").Scan(&count)
    if err != nil {
        t.Fatal(err)
    }
    if count != 1 {
        t.Fatalf("expected embeddings table to exist, got count=%d", count)
    }
}

func TestInsertAndQueryEmbedding(t *testing.T) {
    path := filepath.Join(t.TempDir(), "test.db")
    d, err := db.Open(path)
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    // Insert a test embedding (512 floats)
    embedding := make([]float32, 512)
    for i := range embedding {
        embedding[i] = float32(i) / 512.0
    }

    _, err = d.Exec(`
        INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
        VALUES (?, ?, ?, ?, ?, ?, ?::FLOAT[512], ?)`,
        "entry1_0", "sess1", "entry1", 0, "apple-nlcontextual-v1", 512, embedding, "abc123",
    )
    if err != nil {
        t.Fatal(err)
    }

    // Query it back
    var id, sessionID string
    err = d.QueryRow("SELECT id, session_id FROM embeddings WHERE id = ?", "entry1_0").Scan(&id, &sessionID)
    if err != nil {
        t.Fatal(err)
    }
    if id != "entry1_0" || sessionID != "sess1" {
        t.Fatalf("unexpected values: id=%s session_id=%s", id, sessionID)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/evan/wethinkt/go-thinkt
CGO_ENABLED=1 go test ./internal/indexer/db/ -run TestEmbeddings -v
```

Expected: FAIL — table `embeddings` does not exist.

**Step 3: Add embeddings schema to init.sql**

Append to `internal/indexer/db/schema/init.sql` after line 56:

```sql

-- Embeddings for semantic search (requires VSS extension)
CREATE TABLE IF NOT EXISTS embeddings (
    id          VARCHAR PRIMARY KEY,
    session_id  VARCHAR NOT NULL,
    entry_uuid  VARCHAR NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    model       VARCHAR NOT NULL,
    dim         INTEGER NOT NULL,
    embedding   FLOAT[512] NOT NULL,
    text_hash   VARCHAR NOT NULL,
    created_at  TIMESTAMP DEFAULT current_timestamp,
    UNIQUE(entry_uuid, chunk_index, model)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_session ON embeddings(session_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_entry ON embeddings(entry_uuid);
```

**Note:** The HNSW index requires the VSS extension and `SET hnsw_enable_experimental_persistence = true`. This should be loaded in `db.Open()` before running the schema. If VSS is not available (e.g., DuckDB build without it), skip the HNSW index gracefully — the embeddings table still works with brute-force `array_cosine_distance()`.

**Step 4: Load VSS extension in db.Open()**

Modify `internal/indexer/db/db.go` in the `Open()` function, after the security hardening line (~line 64). Add VSS loading attempt:

```go
    // Try to load VSS extension for HNSW indexing (optional — not all builds include it)
    if _, err := db.Exec("INSTALL vss; LOAD vss;"); err != nil {
        // VSS not available — semantic search will use brute-force cosine similarity
        // This is fine for small-to-medium datasets
    } else {
        // Enable experimental persistence for HNSW indexes
        db.Exec("SET hnsw_enable_experimental_persistence = true")
    }
```

**Step 5: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test ./internal/indexer/db/ -run TestEmbeddings -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add internal/indexer/db/schema/init.sql internal/indexer/db/db.go internal/indexer/db/embeddings_test.go
git commit -m "feat: add embeddings table schema with optional VSS extension"
```

---

### Task 3: Go Embedding Client — Text Extraction & Chunking

**Files:**
- Create: `internal/indexer/embedding/chunker.go`
- Create: `internal/indexer/embedding/chunker_test.go`
- Create: `internal/indexer/embedding/extract.go`
- Create: `internal/indexer/embedding/extract_test.go`

**Step 1: Write the chunker test**

```go
package embedding_test

import (
    "testing"

    "github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func TestChunkText_Short(t *testing.T) {
    chunks := embedding.ChunkText("hello world", 2000, 200)
    if len(chunks) != 1 {
        t.Fatalf("expected 1 chunk, got %d", len(chunks))
    }
    if chunks[0] != "hello world" {
        t.Fatalf("unexpected chunk: %q", chunks[0])
    }
}

func TestChunkText_Long(t *testing.T) {
    // Create a 5000-char string
    text := ""
    for i := 0; i < 250; i++ {
        text += "twenty char string. "
    }
    chunks := embedding.ChunkText(text, 2000, 200)
    if len(chunks) < 3 {
        t.Fatalf("expected at least 3 chunks for 5000 chars, got %d", len(chunks))
    }
    // Verify overlap: end of chunk[0] should appear at start of chunk[1]
    overlap := chunks[0][len(chunks[0])-200:]
    if chunks[1][:200] != overlap {
        t.Fatal("expected 200-char overlap between chunks")
    }
}

func TestChunkText_Empty(t *testing.T) {
    chunks := embedding.ChunkText("", 2000, 200)
    if len(chunks) != 0 {
        t.Fatalf("expected 0 chunks for empty text, got %d", len(chunks))
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/indexer/embedding/ -run TestChunkText -v
```

Expected: FAIL — package doesn't exist yet.

**Step 3: Implement ChunkText**

Create `internal/indexer/embedding/chunker.go`:

```go
package embedding

// ChunkText splits text into chunks of at most maxChars characters
// with overlap characters of overlap between consecutive chunks.
// Returns nil for empty text.
func ChunkText(text string, maxChars, overlap int) []string {
    if len(text) == 0 {
        return nil
    }
    if len(text) <= maxChars {
        return []string{text}
    }

    var chunks []string
    step := maxChars - overlap
    for start := 0; start < len(text); start += step {
        end := start + maxChars
        if end > len(text) {
            end = len(text)
        }
        chunks = append(chunks, text[start:end])
        if end == len(text) {
            break
        }
    }
    return chunks
}
```

**Step 4: Run chunker tests**

```bash
go test ./internal/indexer/embedding/ -run TestChunkText -v
```

Expected: PASS

**Step 5: Write the text extraction test**

Create `internal/indexer/embedding/extract_test.go`:

```go
package embedding_test

import (
    "testing"

    "github.com/wethinkt/go-thinkt/internal/indexer/embedding"
    "github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestExtractText_UserEntry(t *testing.T) {
    entry := thinkt.Entry{
        Role: thinkt.RoleUser,
        Text: "How do I fix auth timeouts?",
    }
    text := embedding.ExtractText(entry)
    if text != "How do I fix auth timeouts?" {
        t.Fatalf("unexpected: %q", text)
    }
}

func TestExtractText_AssistantWithBlocks(t *testing.T) {
    entry := thinkt.Entry{
        Role: thinkt.RoleAssistant,
        ContentBlocks: []thinkt.ContentBlock{
            {Type: "thinking", Thinking: "Let me analyze..."},
            {Type: "text", Text: "Here is the fix."},
            {Type: "tool_use", ToolName: "Read", ToolInput: "file.go"},
        },
    }
    text := embedding.ExtractText(entry)
    if text != "Let me analyze...\nHere is the fix." {
        t.Fatalf("unexpected: %q", text)
    }
}

func TestExtractText_ToolResult(t *testing.T) {
    entry := thinkt.Entry{
        Role: thinkt.RoleTool,
        ContentBlocks: []thinkt.ContentBlock{
            {Type: "tool_result", ToolResult: "func main() { fmt.Println(\"hello\") }"},
        },
    }
    text := embedding.ExtractText(entry)
    if text != "func main() { fmt.Println(\"hello\") }" {
        t.Fatalf("unexpected: %q", text)
    }
}

func TestExtractText_SkipsShort(t *testing.T) {
    entry := thinkt.Entry{Role: thinkt.RoleUser, Text: "ok"}
    text := embedding.ExtractText(entry)
    if text != "" {
        t.Fatalf("expected empty for short text, got: %q", text)
    }
}

func TestExtractText_SkipsCheckpoints(t *testing.T) {
    entry := thinkt.Entry{Role: thinkt.RoleCheckpoint, Text: "checkpoint data..."}
    text := embedding.ExtractText(entry)
    if text != "" {
        t.Fatalf("expected empty for checkpoint, got: %q", text)
    }
}
```

**Step 6: Implement ExtractText**

Create `internal/indexer/embedding/extract.go`:

```go
package embedding

import (
    "strings"

    "github.com/wethinkt/go-thinkt/internal/thinkt"
)

const MinTextLength = 10

// ExtractText extracts embeddable text from an entry.
// Returns empty string for entries that should be skipped
// (checkpoints, progress, too short).
func ExtractText(entry thinkt.Entry) string {
    // Skip non-content roles
    switch entry.Role {
    case thinkt.RoleCheckpoint, thinkt.RoleProgress, thinkt.RoleSystem:
        return ""
    }

    // If entry has content blocks, extract text from them
    if len(entry.ContentBlocks) > 0 {
        var parts []string
        for _, b := range entry.ContentBlocks {
            switch b.Type {
            case "text":
                if b.Text != "" {
                    parts = append(parts, b.Text)
                }
            case "thinking":
                if b.Thinking != "" {
                    parts = append(parts, b.Thinking)
                }
            case "tool_result":
                if b.ToolResult != "" {
                    parts = append(parts, b.ToolResult)
                }
            // Skip tool_use (just function names/args, not meaningful text)
            // Skip media blocks
            }
        }
        text := strings.Join(parts, "\n")
        if len(text) < MinTextLength {
            return ""
        }
        return text
    }

    // Fall back to plain text
    if len(entry.Text) < MinTextLength {
        return ""
    }
    return entry.Text
}
```

**Step 7: Run all embedding tests**

```bash
go test ./internal/indexer/embedding/ -v
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/indexer/embedding/
git commit -m "feat: add text extraction and chunking for embedding pipeline"
```

---

### Task 4: Go Embedding Client — Subprocess Bridge

**Files:**
- Create: `internal/indexer/embedding/client.go`
- Create: `internal/indexer/embedding/client_test.go`

**Step 1: Write the client test**

Create `internal/indexer/embedding/client_test.go`:

```go
package embedding_test

import (
    "context"
    "testing"

    "github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func TestClient_EmbedBatch_Integration(t *testing.T) {
    // Skip if thinkt-embed-apple not available
    client, err := embedding.NewClient()
    if err != nil {
        t.Skipf("thinkt-embed-apple not available: %v", err)
    }

    items := []embedding.EmbedRequest{
        {ID: "a", Text: "debugging the authentication timeout"},
        {ID: "b", Text: "refactoring the database pool"},
    }

    results, err := client.EmbedBatch(context.Background(), items)
    if err != nil {
        t.Fatal(err)
    }
    if len(results) != 2 {
        t.Fatalf("expected 2 results, got %d", len(results))
    }
    for _, r := range results {
        if r.Dim != 512 {
            t.Fatalf("expected dim=512, got %d", r.Dim)
        }
        if len(r.Embedding) != 512 {
            t.Fatalf("expected 512 floats, got %d", len(r.Embedding))
        }
    }
}

func TestClient_NotFound(t *testing.T) {
    _, err := embedding.NewClientWithBinary("nonexistent-binary-xyz")
    if err == nil {
        t.Fatal("expected error for missing binary")
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/indexer/embedding/ -run TestClient -v
```

Expected: FAIL — `NewClient` not defined.

**Step 3: Implement the client**

Create `internal/indexer/embedding/client.go`:

```go
package embedding

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os/exec"
)

const DefaultBinary = "thinkt-embed-apple"

type EmbedRequest struct {
    ID   string `json:"id"`
    Text string `json:"text"`
}

type EmbedResponse struct {
    ID        string    `json:"id"`
    Embedding []float32 `json:"embedding"`
    Dim       int       `json:"dim"`
}

type Client struct {
    binary string
}

// NewClient creates a client using the default binary name.
// Returns an error if the binary is not found in PATH.
func NewClient() (*Client, error) {
    return NewClientWithBinary(DefaultBinary)
}

// NewClientWithBinary creates a client with a specific binary path.
func NewClientWithBinary(binary string) (*Client, error) {
    path, err := exec.LookPath(binary)
    if err != nil {
        return nil, fmt.Errorf("embedding binary not found: %s: %w", binary, err)
    }
    return &Client{binary: path}, nil
}

// EmbedBatch sends a batch of text items to the embedding binary and returns
// the embedding vectors. Spawns one subprocess for the entire batch.
func (c *Client) EmbedBatch(ctx context.Context, items []EmbedRequest) ([]EmbedResponse, error) {
    if len(items) == 0 {
        return nil, nil
    }

    cmd := exec.CommandContext(ctx, c.binary)
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, fmt.Errorf("stdin pipe: %w", err)
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("stdout pipe: %w", err)
    }
    cmd.Stderr = nil // inherit stderr for warnings

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start %s: %w", c.binary, err)
    }

    // Write all requests to stdin
    enc := json.NewEncoder(stdin)
    for _, item := range items {
        if err := enc.Encode(item); err != nil {
            stdin.Close()
            cmd.Wait()
            return nil, fmt.Errorf("write request: %w", err)
        }
    }
    stdin.Close()

    // Read all responses from stdout
    var results []EmbedResponse
    scanner := bufio.NewScanner(stdout)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line
    for scanner.Scan() {
        var resp EmbedResponse
        if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
            continue // skip malformed lines
        }
        results = append(results, resp)
    }

    if err := cmd.Wait(); err != nil {
        return results, fmt.Errorf("%s exited with error: %w", c.binary, err)
    }
    return results, nil
}

// Available returns true if the embedding binary is found in PATH.
func Available() bool {
    _, err := exec.LookPath(DefaultBinary)
    return err == nil
}
```

**Step 4: Run client tests**

```bash
CGO_ENABLED=1 go test ./internal/indexer/embedding/ -run TestClient -v
```

Expected: PASS (TestClient_EmbedBatch_Integration may skip if binary not in PATH; TestClient_NotFound should pass).

**Step 5: Commit**

```bash
git add internal/indexer/embedding/client.go internal/indexer/embedding/client_test.go
git commit -m "feat: add Go embedding client for thinkt-embed-apple subprocess"
```

---

### Task 5: Indexer Embedding Pipeline

**Files:**
- Create: `internal/indexer/embedding/embedder.go`
- Create: `internal/indexer/embedding/embedder_test.go`
- Modify: `internal/indexer/ingester.go:72-123` (add embedding step after entry ingestion)

**Step 1: Write the embedder test**

Create `internal/indexer/embedding/embedder_test.go`:

```go
package embedding_test

import (
    "crypto/sha256"
    "fmt"
    "testing"

    "github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func TestTextHash(t *testing.T) {
    hash := embedding.TextHash("hello world")
    expected := fmt.Sprintf("%x", sha256.Sum256([]byte("hello world")))
    if hash != expected {
        t.Fatalf("expected %s, got %s", expected, hash)
    }
}

func TestPrepareEntries(t *testing.T) {
    // Test that PrepareEntries generates correct EmbedRequests with chunking
    entries := []embedding.EntryText{
        {UUID: "e1", SessionID: "s1", Text: "short text for embedding"},
    }
    requests, mapping := embedding.PrepareEntries(entries, 2000, 200)
    if len(requests) != 1 {
        t.Fatalf("expected 1 request, got %d", len(requests))
    }
    if requests[0].ID != "e1_0" {
        t.Fatalf("expected id=e1_0, got %s", requests[0].ID)
    }
    if len(mapping) != 1 {
        t.Fatalf("expected 1 mapping entry, got %d", len(mapping))
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/indexer/embedding/ -run TestTextHash -v
go test ./internal/indexer/embedding/ -run TestPrepareEntries -v
```

Expected: FAIL — functions not defined.

**Step 3: Implement the embedder**

Create `internal/indexer/embedding/embedder.go`:

```go
package embedding

import (
    "crypto/sha256"
    "fmt"
)

// EntryText holds the extracted text for an entry, ready for embedding.
type EntryText struct {
    UUID      string
    SessionID string
    Text      string
}

// ChunkMapping tracks which chunk maps back to which entry.
type ChunkMapping struct {
    EntryUUID  string
    SessionID  string
    ChunkIndex int
    TextHash   string
}

// TextHash returns the SHA-256 hex digest of the given text.
func TextHash(text string) string {
    h := sha256.Sum256([]byte(text))
    return fmt.Sprintf("%x", h)
}

// PrepareEntries takes extracted entry texts, chunks them, and returns
// EmbedRequests suitable for the Client, plus a mapping from request ID
// back to entry/chunk metadata.
func PrepareEntries(entries []EntryText, maxChars, overlap int) ([]EmbedRequest, []ChunkMapping) {
    var requests []EmbedRequest
    var mapping []ChunkMapping

    for _, e := range entries {
        chunks := ChunkText(e.Text, maxChars, overlap)
        for i, chunk := range chunks {
            id := fmt.Sprintf("%s_%d", e.UUID, i)
            requests = append(requests, EmbedRequest{
                ID:   id,
                Text: chunk,
            })
            mapping = append(mapping, ChunkMapping{
                EntryUUID:  e.UUID,
                SessionID:  e.SessionID,
                ChunkIndex: i,
                TextHash:   TextHash(chunk),
            })
        }
    }

    return requests, mapping
}
```

**Step 4: Run tests**

```bash
go test ./internal/indexer/embedding/ -v
```

Expected: All PASS.

**Step 5: Integrate into ingester**

Modify `internal/indexer/ingester.go`. Add a new method and call it at the end of `IngestSession()` (after line 122, before the return):

Add to the `Ingester` struct a new field:

```go
type Ingester struct {
    db           *db.DB
    registry     *thinkt.StoreRegistry
    embedClient  *embedding.Client // nil if embedding unavailable
    OnProgress   func(pIdx, pTotal, sIdx, sTotal int, message string)
}
```

In `NewIngester()`, try to create the embedding client:

```go
func NewIngester(database *db.DB, registry *thinkt.StoreRegistry) *Ingester {
    var ec *embedding.Client
    if embedding.Available() {
        ec, _ = embedding.NewClient()
    }
    return &Ingester{
        db:          database,
        registry:    registry,
        embedClient: ec,
    }
}
```

Add a new method `embedSession()` and call it from `IngestSession()` after `updateSyncState()`:

```go
func (i *Ingester) embedSession(ctx context.Context, sessionID string, entries []thinkt.Entry) error {
    if i.embedClient == nil {
        return nil
    }

    // Extract text from entries
    var entryTexts []embedding.EntryText
    for _, e := range entries {
        text := embedding.ExtractText(e)
        if text == "" {
            continue
        }
        entryTexts = append(entryTexts, embedding.EntryText{
            UUID:      e.UUID,
            SessionID: sessionID,
            Text:      text,
        })
    }
    if len(entryTexts) == 0 {
        return nil
    }

    // Prepare chunks and embed
    requests, mapping := embedding.PrepareEntries(entryTexts, 2000, 200)
    responses, err := i.embedClient.EmbedBatch(ctx, requests)
    if err != nil {
        return fmt.Errorf("embedding failed: %w", err)
    }

    // Build response lookup
    respMap := make(map[string]embedding.EmbedResponse)
    for _, r := range responses {
        respMap[r.ID] = r
    }

    // Store embeddings
    for idx, m := range mapping {
        id := requests[idx].ID
        resp, ok := respMap[id]
        if !ok {
            continue
        }
        _, err := i.db.ExecContext(ctx, `
            INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
            VALUES (?, ?, ?, ?, ?, ?, ?::FLOAT[512], ?)
            ON CONFLICT (id) DO UPDATE SET
                embedding = excluded.embedding,
                text_hash = excluded.text_hash`,
            id, m.SessionID, m.EntryUUID, m.ChunkIndex,
            "apple-nlcontextual-v1", resp.Dim, resp.Embedding, m.TextHash,
        )
        if err != nil {
            return fmt.Errorf("store embedding %s: %w", id, err)
        }
    }

    return nil
}
```

This requires collecting entries during `IngestSession()`. Modify the entry loop (lines 99-114) to collect entries into a slice, then call `embedSession()` after `updateSyncState()`.

**Step 6: Run existing tests to verify nothing broke**

```bash
CGO_ENABLED=1 go test ./internal/indexer/... -v
```

Expected: PASS (embedding is skipped if binary not available).

**Step 7: Commit**

```bash
git add internal/indexer/embedding/embedder.go internal/indexer/embedding/embedder_test.go internal/indexer/ingester.go
git commit -m "feat: integrate embedding pipeline into indexer ingestion"
```

---

### Task 6: Semantic Search Service

**Files:**
- Create: `internal/indexer/search/semantic.go`
- Create: `internal/indexer/search/semantic_test.go`

**Step 1: Write the failing test**

Create `internal/indexer/search/semantic_test.go`:

```go
package search_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/wethinkt/go-thinkt/internal/indexer/db"
    "github.com/wethinkt/go-thinkt/internal/indexer/search"
)

func TestSemanticSearch_NoResults(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "test.db")
    d, err := db.Open(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    svc := search.NewService(d)
    results, err := svc.SemanticSearch(search.SemanticSearchOptions{
        QueryEmbedding: make([]float32, 512),
        Model:          "apple-nlcontextual-v1",
        Limit:          10,
    })
    if err != nil {
        t.Fatal(err)
    }
    if len(results) != 0 {
        t.Fatalf("expected 0 results, got %d", len(results))
    }
}

func TestSemanticSearch_FindsSimilar(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "test.db")
    d, err := db.Open(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    // Insert two embeddings: one similar to query, one not
    similar := make([]float32, 512)
    similar[0] = 1.0 // pointing in one direction

    different := make([]float32, 512)
    different[1] = 1.0 // pointing in orthogonal direction

    for _, tc := range []struct {
        id, sessID, entryUUID string
        emb                   []float32
    }{
        {"e1_0", "s1", "e1", similar},
        {"e2_0", "s2", "e2", different},
    } {
        _, err := d.Exec(`
            INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
            VALUES (?, ?, ?, 0, 'apple-nlcontextual-v1', 512, ?::FLOAT[512], 'hash')`,
            tc.id, tc.sessID, tc.entryUUID, tc.emb)
        if err != nil {
            t.Fatal(err)
        }
    }

    // Query with vector similar to "similar"
    query := make([]float32, 512)
    query[0] = 1.0

    svc := search.NewService(d)
    results, err := svc.SemanticSearch(search.SemanticSearchOptions{
        QueryEmbedding: query,
        Model:          "apple-nlcontextual-v1",
        Limit:          10,
    })
    if err != nil {
        t.Fatal(err)
    }
    if len(results) < 1 {
        t.Fatal("expected at least 1 result")
    }
    // First result should be the similar one
    if results[0].SessionID != "s1" {
        t.Fatalf("expected s1 first, got %s", results[0].SessionID)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/indexer/search/ -run TestSemantic -v
```

Expected: FAIL — `SemanticSearch` not defined.

**Step 3: Implement semantic search**

Create `internal/indexer/search/semantic.go`:

```go
package search

import (
    "fmt"
    "strings"
)

// SemanticSearchOptions contains options for semantic search.
type SemanticSearchOptions struct {
    QueryEmbedding []float32
    Model          string
    FilterProject  string
    FilterSource   string
    Limit          int
    MaxDistance     float64 // 0 means no threshold
}

// SemanticResult represents a single semantic search hit.
type SemanticResult struct {
    SessionID  string  `json:"session_id"`
    EntryUUID  string  `json:"entry_uuid"`
    ChunkIndex int     `json:"chunk_index"`
    Distance   float64 `json:"distance"`
    // Enriched by join
    ProjectName string `json:"project_name,omitempty"`
    Source      string `json:"source,omitempty"`
    Path        string `json:"path,omitempty"`
}

// SemanticSearch queries the embeddings table for vectors similar to the query.
func (s *Service) SemanticSearch(opts SemanticSearchOptions) ([]SemanticResult, error) {
    if opts.Limit <= 0 {
        opts.Limit = 20
    }

    // Build query — join with sessions and projects for metadata
    q := `
        SELECT e.session_id, e.entry_uuid, e.chunk_index,
               array_cosine_distance(e.embedding, ?::FLOAT[512]) AS distance,
               COALESCE(p.name, '') AS project_name,
               COALESCE(p.source, '') AS source,
               COALESCE(s.path, '') AS path
        FROM embeddings e
        LEFT JOIN sessions s ON e.session_id = s.id
        LEFT JOIN projects p ON s.project_id = p.id
        WHERE e.model = ?`

    args := []interface{}{opts.QueryEmbedding, opts.Model}

    if opts.FilterProject != "" {
        q += " AND p.name LIKE ?"
        args = append(args, "%"+opts.FilterProject+"%")
    }
    if opts.FilterSource != "" {
        q += " AND p.source = ?"
        args = append(args, opts.FilterSource)
    }
    if opts.MaxDistance > 0 {
        q += fmt.Sprintf(" AND array_cosine_distance(e.embedding, ?::FLOAT[512]) < %f", opts.MaxDistance)
        args = append(args, opts.QueryEmbedding)
    }

    q += " ORDER BY distance ASC LIMIT ?"
    args = append(args, opts.Limit)

    rows, err := s.db.Query(q, args...)
    if err != nil {
        return nil, fmt.Errorf("semantic search query: %w", err)
    }
    defer rows.Close()

    var results []SemanticResult
    for rows.Next() {
        var r SemanticResult
        if err := rows.Scan(&r.SessionID, &r.EntryUUID, &r.ChunkIndex,
            &r.Distance, &r.ProjectName, &r.Source, &r.Path); err != nil {
            return nil, fmt.Errorf("scan result: %w", err)
        }
        results = append(results, r)
    }

    return results, nil
}
```

**Step 4: Run tests**

```bash
CGO_ENABLED=1 go test ./internal/indexer/search/ -run TestSemantic -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/indexer/search/semantic.go internal/indexer/search/semantic_test.go
git commit -m "feat: add semantic search service with cosine distance queries"
```

---

### Task 7: Semantic Search CLI Command

**Files:**
- Create: `internal/indexer/cmd/semantic_search.go`

**Step 1: Implement the CLI command**

Create `internal/indexer/cmd/semantic_search.go`:

```go
package cmd

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "github.com/spf13/cobra"

    "github.com/wethinkt/go-thinkt/internal/indexer/embedding"
    "github.com/wethinkt/go-thinkt/internal/indexer/search"
)

var (
    semFilterProject string
    semFilterSource  string
    semLimit         int
    semMaxDistance    float64
    semJSON          bool
)

var semanticSearchCmd = &cobra.Command{
    Use:   "semantic-search <query>",
    Short: "Semantic search across indexed sessions using on-device embeddings",
    Long: `Search for sessions by meaning using Apple's on-device NLContextualEmbedding.

Requires thinkt-embed-apple to be installed and in PATH.
The query is embedded and compared against stored session embeddings
using cosine similarity.`,
    Args: cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        queryText := args[0]

        // Get embedding for query
        client, err := embedding.NewClient()
        if err != nil {
            return fmt.Errorf("semantic search unavailable: %w", err)
        }

        responses, err := client.EmbedBatch(context.Background(), []embedding.EmbedRequest{
            {ID: "query", Text: queryText},
        })
        if err != nil {
            return fmt.Errorf("failed to embed query: %w", err)
        }
        if len(responses) == 0 {
            return fmt.Errorf("embedding returned no results for query")
        }

        // Search
        db, err := getReadOnlyDB()
        if err != nil {
            return err
        }
        defer db.Close()

        svc := search.NewService(db)
        results, err := svc.SemanticSearch(search.SemanticSearchOptions{
            QueryEmbedding: responses[0].Embedding,
            Model:          "apple-nlcontextual-v1",
            FilterProject:  semFilterProject,
            FilterSource:   semFilterSource,
            Limit:          semLimit,
            MaxDistance:     semMaxDistance,
        })
        if err != nil {
            return err
        }

        if semJSON {
            return json.NewEncoder(os.Stdout).Encode(results)
        }

        if len(results) == 0 {
            fmt.Println("No semantic matches found.")
            return nil
        }

        for _, r := range results {
            fmt.Printf("%.4f  %s  %s  %s\n",
                r.Distance, r.SessionID, r.ProjectName, search.ShortenPath(r.Path))
        }
        return nil
    },
}

func init() {
    semanticSearchCmd.Flags().StringVarP(&semFilterProject, "project", "p", "", "Filter by project name")
    semanticSearchCmd.Flags().StringVarP(&semFilterSource, "source", "s", "", "Filter by source")
    semanticSearchCmd.Flags().IntVarP(&semLimit, "limit", "n", 20, "Max results (default 20)")
    semanticSearchCmd.Flags().Float64Var(&semMaxDistance, "max-distance", 0, "Max cosine distance (0 = no threshold)")
    semanticSearchCmd.Flags().BoolVar(&semJSON, "json", false, "Output as JSON")

    rootCmd.AddCommand(semanticSearchCmd)
}
```

**Step 2: Build and test manually**

```bash
task build:indexer
# First index some sessions:
./bin/thinkt-indexer sync
# Then search:
./bin/thinkt-indexer semantic-search "debugging auth timeouts"
```

Expected: Either results or "No semantic matches found." (depending on whether embeddings have been indexed).

**Step 3: Commit**

```bash
git add internal/indexer/cmd/semantic_search.go
git commit -m "feat: add semantic-search CLI command to thinkt-indexer"
```

---

### Task 8: Semantic Search MCP Tool

**Files:**
- Modify: `internal/server/mcp.go:127-147` (register new tool)
- Modify: `internal/server/mcp.go` (add handler and input type)

**Step 1: Add the MCP tool registration**

In `registerTools()` in `internal/server/mcp.go`, add after the `search_sessions` registration (~line 137):

```go
        // semantic_search
        if ms.isToolAllowed("semantic_search") {
            mcp.AddTool(ms.server, &mcp.Tool{
                Name:        "semantic_search",
                Description: "Search for sessions by meaning using on-device embeddings. Requires thinkt-embed-apple. Returns sessions ranked by semantic similarity to the query.",
            }, ms.handleSemanticSearch)
        }
```

**Step 2: Add the input type and handler**

Add near the other input types:

```go
type semanticSearchInput struct {
    Query   string `json:"query" jsonschema:"required,description=Natural language search query"`
    Project string `json:"project,omitempty" jsonschema:"description=Filter by project name"`
    Source  string `json:"source,omitempty" jsonschema:"description=Filter by source"`
    Limit   int    `json:"limit,omitempty" jsonschema:"description=Max results (default 20)"`
}
```

Add the handler (follows same pattern as `handleSearchSessions` — delegates to `thinkt-indexer`):

```go
func (ms *MCPServer) handleSemanticSearch(ctx context.Context, req *mcp.CallToolRequest, input semanticSearchInput) (*mcp.CallToolResult, any, error) {
    path := findIndexerBinary()
    if path == "" {
        return nil, nil, fmt.Errorf("indexer not found")
    }
    args := []string{"semantic-search", "--json", input.Query}
    if input.Project != "" {
        args = append(args, "--project", input.Project)
    }
    if source := strings.TrimSpace(strings.ToLower(input.Source)); source != "" {
        args = append(args, "--source", source)
    }
    if input.Limit > 0 {
        args = append(args, "--limit", fmt.Sprintf("%d", input.Limit))
    }

    cmd := exec.Command(path, args...)
    out, err := cmd.Output()
    if err != nil {
        return nil, nil, fmt.Errorf("semantic search failed: %w", err)
    }
    var res any
    if err := json.Unmarshal(out, &res); err != nil {
        return nil, nil, fmt.Errorf("indexer returned invalid JSON: %w", err)
    }
    return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, res, nil
}
```

**Step 3: Run existing MCP tests**

```bash
CGO_ENABLED=1 go test ./internal/server/ -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add internal/server/mcp.go
git commit -m "feat: add semantic_search MCP tool"
```

---

### Task 9: End-to-End Integration Test

**Files:**
- Create: `internal/indexer/embedding/integration_test.go`

**Step 1: Write end-to-end test**

This test exercises the full pipeline: extract text → chunk → embed → store → search.

```go
package embedding_test

import (
    "context"
    "path/filepath"
    "testing"

    "github.com/wethinkt/go-thinkt/internal/indexer/db"
    "github.com/wethinkt/go-thinkt/internal/indexer/embedding"
    "github.com/wethinkt/go-thinkt/internal/indexer/search"
    "github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestEndToEnd_EmbedAndSearch(t *testing.T) {
    // Skip if binary not available
    client, err := embedding.NewClient()
    if err != nil {
        t.Skipf("thinkt-embed-apple not available: %v", err)
    }

    // Setup DB
    dbPath := filepath.Join(t.TempDir(), "test.db")
    d, err := db.Open(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    defer d.Close()

    // Create test entries
    entries := []thinkt.Entry{
        {UUID: "e1", Role: thinkt.RoleUser, Text: "How do I fix the authentication timeout in the login flow?"},
        {UUID: "e2", Role: thinkt.RoleAssistant, Text: "The timeout is caused by a slow database query in the auth middleware."},
        {UUID: "e3", Role: thinkt.RoleUser, Text: "Can you help me set up a CI/CD pipeline with GitHub Actions?"},
    }

    // Extract and prepare
    var entryTexts []embedding.EntryText
    for _, e := range entries {
        text := embedding.ExtractText(e)
        if text == "" {
            continue
        }
        entryTexts = append(entryTexts, embedding.EntryText{
            UUID: e.UUID, SessionID: "s1", Text: text,
        })
    }

    requests, mapping := embedding.PrepareEntries(entryTexts, 2000, 200)

    // Embed
    responses, err := client.EmbedBatch(context.Background(), requests)
    if err != nil {
        t.Fatal(err)
    }

    // Store
    respMap := make(map[string]embedding.EmbedResponse)
    for _, r := range responses {
        respMap[r.ID] = r
    }

    // Need a session row for the join
    d.Exec("INSERT INTO projects (id, path, name, source) VALUES ('p1', '/test', 'test-project', 'claude')")
    d.Exec("INSERT INTO sessions (id, project_id, path, entry_count) VALUES ('s1', 'p1', '/test/s1.jsonl', 3)")

    for idx, m := range mapping {
        id := requests[idx].ID
        resp := respMap[id]
        _, err := d.Exec(`
            INSERT INTO embeddings (id, session_id, entry_uuid, chunk_index, model, dim, embedding, text_hash)
            VALUES (?, ?, ?, ?, 'apple-nlcontextual-v1', 512, ?::FLOAT[512], ?)`,
            id, m.SessionID, m.EntryUUID, m.ChunkIndex, resp.Embedding, m.TextHash)
        if err != nil {
            t.Fatal(err)
        }
    }

    // Search for "auth timeout" — should find e1 and e2, not e3
    queryResp, err := client.EmbedBatch(context.Background(), []embedding.EmbedRequest{
        {ID: "q", Text: "authentication timeout problem"},
    })
    if err != nil {
        t.Fatal(err)
    }

    svc := search.NewService(d)
    results, err := svc.SemanticSearch(search.SemanticSearchOptions{
        QueryEmbedding: queryResp[0].Embedding,
        Model:          "apple-nlcontextual-v1",
        Limit:          10,
    })
    if err != nil {
        t.Fatal(err)
    }

    if len(results) == 0 {
        t.Fatal("expected results")
    }

    // The auth-related entries should rank higher than CI/CD
    t.Logf("Results:")
    for _, r := range results {
        t.Logf("  distance=%.4f entry=%s", r.Distance, r.EntryUUID)
    }

    // e1 or e2 should be the top result
    top := results[0].EntryUUID
    if top != "e1" && top != "e2" {
        t.Fatalf("expected e1 or e2 as top result, got %s", top)
    }
}
```

**Step 2: Run the integration test**

```bash
CGO_ENABLED=1 go test ./internal/indexer/embedding/ -run TestEndToEnd -v -timeout 30s
```

Expected: PASS (or SKIP if binary not available).

**Step 3: Commit**

```bash
git add internal/indexer/embedding/integration_test.go
git commit -m "test: add end-to-end integration test for semantic search pipeline"
```

---

### Task 10: Clean Up Benchmark Tool

**Files:**
- Remove: `tools/embed-benchmark/` (served its purpose, benchmark code now lives in tests)

**Step 1: Remove benchmark directory**

```bash
rm -rf tools/embed-benchmark
```

**Step 2: Commit**

```bash
git add -A tools/embed-benchmark
git commit -m "chore: remove embed benchmark tool (replaced by integration tests)"
```
