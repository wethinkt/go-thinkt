-- Schema for thinkt DuckDB indexer

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
    id            VARCHAR PRIMARY KEY, -- Hash of path
    path          VARCHAR UNIQUE,
    name          VARCHAR,
    source        VARCHAR, -- 'claude', 'kimi', etc.
    workspace_id  VARCHAR
);

-- Session metadata
CREATE TABLE IF NOT EXISTS sessions (
    id            VARCHAR PRIMARY KEY, -- UUID
    project_id    VARCHAR REFERENCES projects(id),
    path          VARCHAR,
    model         VARCHAR,
    first_prompt  TEXT,
    created_at    TIMESTAMP,
    updated_at    TIMESTAMP
);

-- Conversation entries
CREATE TABLE IF NOT EXISTS entries (
    uuid          VARCHAR PRIMARY KEY,
    session_id    VARCHAR REFERENCES sessions(id),
    timestamp     TIMESTAMP,
    role          VARCHAR,
    body          JSON 
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_entries_session ON entries(session_id);
CREATE INDEX IF NOT EXISTS idx_entries_ts ON entries(timestamp);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);
