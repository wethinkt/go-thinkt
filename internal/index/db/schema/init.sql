-- Schema for thinkt SQLite index (Privacy-first Light Index)

-- Tracks which files we've indexed and how far we've read
CREATE TABLE IF NOT EXISTS sync_state (
    file_path     TEXT PRIMARY KEY,
    last_mod_time INTEGER,
    file_size     INTEGER,
    lines_read    INTEGER,
    last_synced   TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Projects mapping
CREATE TABLE IF NOT EXISTS projects (
    id            TEXT PRIMARY KEY,
    path          TEXT,
    name          TEXT,
    source        TEXT,
    workspace_id  TEXT,
    UNIQUE(path, source)
);

-- Migration tracking
CREATE TABLE IF NOT EXISTS migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Session metadata
CREATE TABLE IF NOT EXISTS sessions (
    id            TEXT PRIMARY KEY,
    project_id    TEXT REFERENCES projects(id),
    path          TEXT,
    model         TEXT,
    first_prompt  TEXT,
    summary       TEXT,
    entry_count   INTEGER,
    created_at    TEXT,
    updated_at    TEXT
);

-- Conversation entries (Metadata only, no private content)
-- Keyed by (session_id, uuid) since entry UUIDs are only unique within a session.
CREATE TABLE IF NOT EXISTS entries (
    session_id    TEXT NOT NULL,
    uuid          TEXT NOT NULL,
    timestamp     TEXT,
    role          TEXT,

    -- Extracted Metrics (NO message text stored)
    input_tokens  INTEGER,
    output_tokens INTEGER,
    tool_name     TEXT,
    is_error      INTEGER,
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
