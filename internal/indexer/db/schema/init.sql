-- Schema for thinkt DuckDB indexer (Privacy-first Light Index)

-- Tracks which files we've indexed and how far we've read
CREATE TABLE IF NOT EXISTS sync_state (
    file_path     VARCHAR PRIMARY KEY,
    last_mod_time TIMESTAMP,
    file_size     BIGINT,
    lines_read    BIGINT,
    last_synced   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Projects mapping
CREATE TABLE IF NOT EXISTS projects (
    id            VARCHAR PRIMARY KEY,
    path          VARCHAR,
    name          VARCHAR,
    source        VARCHAR,
    workspace_id  VARCHAR,
    UNIQUE(path, source)
);

-- Session metadata
CREATE TABLE IF NOT EXISTS sessions (
    id            VARCHAR PRIMARY KEY,
    project_id    VARCHAR,
    path          VARCHAR,
    model         VARCHAR,
    first_prompt  TEXT,
    entry_count   INTEGER,
    created_at    TIMESTAMP,
    updated_at    TIMESTAMP
);

-- Conversation entries (Metadata only, no private content)
CREATE TABLE IF NOT EXISTS entries (
    uuid          VARCHAR PRIMARY KEY,
    session_id    VARCHAR,
    timestamp     TIMESTAMP,
    role          VARCHAR,
    
    -- Extracted Metrics
    input_tokens  INTEGER,
    output_tokens INTEGER,
    tool_name     VARCHAR,
    is_error      BOOLEAN,
    word_count    INTEGER,
    thinking_len  INTEGER,
    
    -- Reference to source
    line_number   INTEGER
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_entries_session ON entries(session_id);
CREATE INDEX IF NOT EXISTS idx_entries_ts ON entries(timestamp);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);

-- Embeddings for semantic search (requires VSS extension for HNSW indexing)
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

CREATE INDEX IF NOT EXISTS idx_embeddings_session ON embeddings(session_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_entry ON embeddings(entry_uuid);
