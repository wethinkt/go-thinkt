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
-- Keyed by (session_id, uuid) since entry UUIDs are only unique within a session
-- (e.g. Kimi uses short per-session IDs like L1, L2).
CREATE TABLE IF NOT EXISTS entries (
    session_id    VARCHAR NOT NULL,
    uuid          VARCHAR NOT NULL,
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
    line_number   INTEGER,

    PRIMARY KEY (session_id, uuid)
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_entries_session ON entries(session_id);
CREATE INDEX IF NOT EXISTS idx_entries_ts ON entries(timestamp);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
