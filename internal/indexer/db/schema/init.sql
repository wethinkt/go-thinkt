-- Schema for thinkt DuckDB indexer (Privacy-first Light Index)

-- Tracks which files we've indexed and how far we've read
CREATE TABLE IF NOT EXISTS sync_state (
    file_path     VARCHAR PRIMARY KEY,
    last_mod_time TIMESTAMP,
    file_size     BIGINT,
    lines_read    BIGINT,
    last_synced   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE sync_state IS 'Tracks indexing progress and file metadata to avoid redundant scanning.';
COMMENT ON COLUMN sync_state.file_path IS 'Absolute path to the session data file.';
COMMENT ON COLUMN sync_state.last_mod_time IS 'Last modification time of the file when last indexed.';
COMMENT ON COLUMN sync_state.file_size IS 'Size of the file in bytes at last sync.';
COMMENT ON COLUMN sync_state.lines_read IS 'Number of JSONL lines successfully processed from this file.';
COMMENT ON COLUMN sync_state.last_synced IS 'Timestamp of the most recent indexing operation.';

-- Projects mapping
CREATE TABLE IF NOT EXISTS projects (
    id            VARCHAR PRIMARY KEY,
    path          VARCHAR,
    name          VARCHAR,
    source        VARCHAR,
    workspace_id  VARCHAR,
    UNIQUE(path, source)
);

COMMENT ON TABLE projects IS 'Groups sessions by logical project boundaries (e.g. git repository).';
COMMENT ON COLUMN projects.id IS 'Stable identifier for the project, often derived from path.';
COMMENT ON COLUMN projects.path IS 'Local filesystem path to the project root.';
COMMENT ON COLUMN projects.name IS 'Human-readable project name (e.g. repository name).';
COMMENT ON COLUMN projects.source IS 'Trace source provider (e.g. claude, kimi, gemini).';
COMMENT ON COLUMN projects.workspace_id IS 'Source-specific workspace or organization identifier.';

-- Migration tracking
CREATE TABLE IF NOT EXISTS migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE migrations IS 'Tracks schema versioning for database upgrades.';
COMMENT ON COLUMN migrations.version IS 'The migration version number.';
COMMENT ON COLUMN migrations.applied_at IS 'Timestamp when the migration was applied.';

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

COMMENT ON TABLE sessions IS 'High-level metadata for an individual AI conversation session.';
COMMENT ON COLUMN sessions.id IS 'Unique session identifier (often a UUID).';
COMMENT ON COLUMN sessions.project_id IS 'Foreign key to the projects table.';
COMMENT ON COLUMN sessions.path IS 'Absolute path to the session JSONL file.';
COMMENT ON COLUMN sessions.model IS 'The primary AI model used during this session.';
COMMENT ON COLUMN sessions.first_prompt IS 'Snippet of the first user message for quick preview.';
COMMENT ON COLUMN sessions.entry_count IS 'Total number of messages/turns in the conversation.';

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

COMMENT ON TABLE entries IS 'Metadata and metrics for individual conversation turns. Does NOT contain private message text.';
COMMENT ON COLUMN entries.session_id IS 'Unique identifier for the parent session.';
COMMENT ON COLUMN entries.uuid IS 'Per-session unique identifier for the entry.';
COMMENT ON COLUMN entries.role IS 'The speaker role (e.g. user, assistant, system, tool).';
COMMENT ON COLUMN entries.input_tokens IS 'Number of prompt tokens consumed for this turn.';
COMMENT ON COLUMN entries.output_tokens IS 'Number of response tokens generated for this turn.';
COMMENT ON COLUMN entries.tool_name IS 'Name of the tool called, if role is assistant/tool.';
COMMENT ON COLUMN entries.is_error IS 'Flag indicating if the tool or response resulted in an error.';
COMMENT ON COLUMN entries.thinking_len IS 'Character count of internal reasoning/thinking blocks.';
COMMENT ON COLUMN entries.line_number IS '1-based line number in the source JSONL file.';

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_entries_session ON entries(session_id);
CREATE INDEX IF NOT EXISTS idx_entries_ts ON entries(timestamp);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);

-- Commit to WAL as COMMENTS have replay issues
CHECKPOINT;