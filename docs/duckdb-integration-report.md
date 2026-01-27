# DuckDB Integration Report for thinkt

## Overview

DuckDB is an embedded analytical database that excels at:
- In-process analytics (no server required)
- Direct JSON/JSONL file querying
- Columnar storage for fast aggregations
- SQL interface for complex queries

The thinking-tracer webapp already uses DuckDB for search and word frequency analysis. Embedding DuckDB in `thinkt` CLI would bring similar analytical capabilities to the command line.

## Go Integration

DuckDB has official Go bindings via `github.com/marcboeker/go-duckdb`:

```go
import "database/sql"
import _ "github.com/marcboeker/go-duckdb"

db, _ := sql.Open("duckdb", "")  // In-memory
// or
db, _ := sql.Open("duckdb", "sessions.db")  // Persistent
```

## Potential Use Cases

### 1. Full-Text Search Across Sessions

```sql
-- Search all sessions for a term
SELECT
    session_path,
    json_extract_string(entry, '$.message[0].content') as content,
    json_extract_string(entry, '$.type') as type
FROM read_json_auto('~/.claude/projects/*/*.jsonl')
WHERE content ILIKE '%authentication%'
LIMIT 20;
```

**CLI**: `thinkt search "authentication"` or `thinkt sessions search -p ./myproject "error handling"`

### 2. Token/Cost Analysis

```sql
-- Token usage by session
SELECT
    regexp_extract(filename, '([^/]+)\.jsonl$', 1) as session_id,
    SUM(json_extract(entry, '$.usage.input_tokens')::int) as input_tokens,
    SUM(json_extract(entry, '$.usage.output_tokens')::int) as output_tokens,
    SUM(json_extract(entry, '$.usage.cache_read_input_tokens')::int) as cache_tokens
FROM read_json_auto('~/.claude/projects/-Users-evan-myproject/*.jsonl')
GROUP BY session_id
ORDER BY input_tokens + output_tokens DESC;
```

**CLI**: `thinkt stats tokens -p ./myproject`

### 3. Tool Usage Analytics

```sql
-- Most used tools across all sessions
SELECT
    json_extract_string(tool, '$.name') as tool_name,
    COUNT(*) as usage_count
FROM read_json_auto('~/.claude/projects/*/*.jsonl'),
     LATERAL unnest(json_extract(entry, '$.message')::json[]) as t(tool)
WHERE json_extract_string(entry, '$.type') = 'assistant'
  AND json_extract_string(tool, '$.type') = 'tool_use'
GROUP BY tool_name
ORDER BY usage_count DESC
LIMIT 20;
```

**CLI**: `thinkt stats tools` or `thinkt stats tools -p ./myproject`

### 4. Word Frequency / Topic Analysis

```sql
-- Top words in user prompts (excluding common words)
WITH words AS (
    SELECT unnest(string_split(
        lower(regexp_replace(
            json_extract_string(entry, '$.message[0].content'),
            '[^a-zA-Z ]', ' ', 'g'
        )), ' '
    )) as word
    FROM read_json_auto('~/.claude/projects/*/*.jsonl')
    WHERE json_extract_string(entry, '$.type') = 'user'
)
SELECT word, COUNT(*) as freq
FROM words
WHERE length(word) > 3
  AND word NOT IN ('that', 'this', 'with', 'from', 'have', 'will', 'been', 'were', 'they', 'their', 'what', 'about', 'would', 'there', 'could', 'other', 'into', 'more', 'some', 'very', 'just', 'also', 'only', 'over', 'such', 'than', 'then', 'these', 'most')
GROUP BY word
ORDER BY freq DESC
LIMIT 50;
```

**CLI**: `thinkt stats words` or `thinkt stats words -p ./myproject --top 100`

### 5. Session Timeline / Activity Analysis

```sql
-- Sessions by day with message counts
SELECT
    DATE_TRUNC('day', to_timestamp(json_extract(entry, '$.timestamp')::bigint / 1000)) as day,
    COUNT(DISTINCT regexp_extract(filename, '([^/]+)\.jsonl$', 1)) as sessions,
    COUNT(*) as messages
FROM read_json_auto('~/.claude/projects/*/*.jsonl')
GROUP BY day
ORDER BY day DESC
LIMIT 30;
```

**CLI**: `thinkt stats activity --days 30`

### 6. Error/Failure Analysis

```sql
-- Find tool errors and failures
SELECT
    json_extract_string(entry, '$.tool_name') as tool,
    json_extract_string(entry, '$.content') as error_content,
    filename
FROM read_json_auto('~/.claude/projects/*/*.jsonl')
WHERE json_extract_string(entry, '$.type') = 'tool_result'
  AND json_extract_string(entry, '$.is_error') = 'true'
LIMIT 20;
```

**CLI**: `thinkt stats errors`

### 7. Model Usage Comparison

```sql
-- Which models are used most
SELECT
    json_extract_string(entry, '$.model') as model,
    COUNT(*) as responses,
    AVG(json_extract(entry, '$.usage.output_tokens')::int) as avg_output_tokens
FROM read_json_auto('~/.claude/projects/*/*.jsonl')
WHERE json_extract_string(entry, '$.type') = 'assistant'
  AND json_extract_string(entry, '$.model') IS NOT NULL
GROUP BY model
ORDER BY responses DESC;
```

**CLI**: `thinkt stats models`

### 8. Thinking Block Analysis

```sql
-- Sessions with most thinking content
SELECT
    regexp_extract(filename, '([^/]+)\.jsonl$', 1) as session_id,
    COUNT(*) as thinking_blocks,
    SUM(length(json_extract_string(block, '$.thinking'))) as total_thinking_chars
FROM read_json_auto('~/.claude/projects/*/*.jsonl'),
     LATERAL unnest(json_extract(entry, '$.message')::json[]) as t(block)
WHERE json_extract_string(block, '$.type') = 'thinking'
GROUP BY session_id
ORDER BY total_thinking_chars DESC
LIMIT 10;
```

**CLI**: `thinkt stats thinking`

## Proposed CLI Commands

```
thinkt search <query>                    # Full-text search across all sessions
thinkt search -p <project> <query>       # Search within project

thinkt stats tokens [-p <project>]       # Token usage summary
thinkt stats tools [-p <project>]        # Tool usage frequency
thinkt stats words [-p <project>]        # Word frequency analysis
thinkt stats activity [--days N]         # Activity timeline
thinkt stats errors [-p <project>]       # Error analysis
thinkt stats models [-p <project>]       # Model usage breakdown
thinkt stats thinking [-p <project>]     # Thinking block analysis
thinkt stats summary [-p <project>]      # Combined overview

thinkt query "<sql>"                     # Raw SQL for power users
```

## Benefits Over Current Approach

| Feature | Current | With DuckDB |
|---------|---------|-------------|
| Search | Load each file, scan in Go | Single SQL query across all files |
| Aggregations | Manual iteration + maps | Native SQL GROUP BY |
| Filtering | Go code | SQL WHERE clauses |
| Joins | Complex Go code | SQL JOINs |
| Performance | O(n) file reads | Columnar scan, predicate pushdown |
| Flexibility | Requires code changes | SQL queries, easy to modify |
| Memory | Load full files | Streaming, out-of-core processing |

## Implementation Considerations

### Pros
- **Zero infrastructure**: Embedded, no server needed
- **Direct JSONL reading**: `read_json_auto()` handles JSONL natively
- **Fast**: Columnar engine optimized for analytics
- **Familiar**: SQL interface for queries
- **Extensible**: Users can run custom queries with `thinkt query`

### Cons
- **Binary size**: DuckDB adds ~30-50MB to binary size
- **CGO dependency**: Requires CGO for Go bindings
- **Cold start**: First query needs to scan files (can cache to .db file)
- **Complexity**: SQL may be overkill for simple operations

### Hybrid Approach

Keep simple operations (list, delete, copy) as-is. Use DuckDB for:
- Search functionality
- Statistics/analytics
- Complex queries

```go
// Only initialize DuckDB when needed
func (s *StatsCommand) Run() error {
    db, err := sql.Open("duckdb", "")
    if err != nil {
        return err
    }
    defer db.Close()

    // Run analytics query
    rows, err := db.Query(tokenUsageQuery, s.projectPath)
    // ...
}
```

### Caching Strategy

For repeated queries, maintain a cached DuckDB database:

```
~/.cache/thinkt/sessions.db
```

Invalidate based on file modification times:
```sql
-- Track file mtimes
CREATE TABLE file_index (path TEXT, mtime TIMESTAMP, indexed_at TIMESTAMP);
```

## Comparison with thinking-tracer Webapp

The webapp uses DuckDB via WASM for client-side analytics. Key queries there:

1. **Search**: Full-text search across message content
2. **Word frequency**: Top words in conversations
3. **Timeline**: Message activity over time

The CLI would use the same engine but with:
- Native Go bindings (faster than WASM)
- Direct filesystem access (no file upload needed)
- Batch processing capabilities
- Script-friendly output formats (JSON, CSV, plain)

## Recommended Next Steps

1. **Prototype**: Add `thinkt search` command using DuckDB
2. **Benchmark**: Compare performance vs current Go-based scanning
3. **Evaluate binary size**: Test with/without DuckDB
4. **Consider alternatives**:
   - SQLite with JSON1 extension (smaller, no CGO with modernc.org/sqlite)
   - Pure Go search (bleve, tantivy-go)
5. **User research**: What analytics do users actually want?

## Example Implementation Sketch

```go
// internal/analytics/duckdb.go
package analytics

import (
    "database/sql"
    _ "github.com/marcboeker/go-duckdb"
)

type Engine struct {
    db *sql.DB
}

func NewEngine() (*Engine, error) {
    db, err := sql.Open("duckdb", "")
    if err != nil {
        return nil, err
    }
    return &Engine{db: db}, nil
}

func (e *Engine) Search(baseDir, query string, limit int) ([]SearchResult, error) {
    pattern := filepath.Join(baseDir, "projects", "*", "*.jsonl")

    rows, err := e.db.Query(`
        SELECT
            filename,
            json_extract_string(entry, '$.message[0].content') as content,
            json_extract_string(entry, '$.type') as type
        FROM read_json_auto(?)
        WHERE content ILIKE '%' || ? || '%'
        LIMIT ?
    `, pattern, query, limit)
    // ...
}

func (e *Engine) TokenStats(projectDir string) (*TokenStats, error) {
    // ...
}

func (e *Engine) Close() error {
    return e.db.Close()
}
```

## Conclusion

DuckDB integration would significantly enhance `thinkt`'s analytical capabilities, enabling powerful search and statistics features with minimal code. The main trade-offs are binary size and CGO dependency. A hybrid approach—using DuckDB only for analytics while keeping simple operations in pure Go—offers the best balance.

The existing thinking-tracer webapp DuckDB usage validates the approach. Porting those queries to the CLI would provide immediate value for users who want command-line analytics without running the webapp.
