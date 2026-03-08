-- Schema for thinkt summaries database (separate from main index)
-- Summaries are model-dependent: switching models invalidates them.
-- One DB file per model at ~/.thinkt/summaries/<model-id>.duckdb

CREATE TABLE IF NOT EXISTS summaries (
    session_id  VARCHAR NOT NULL,
    entry_uuid  VARCHAR NOT NULL,  -- "__session__" for session-level summaries
    summary     TEXT NOT NULL,
    category    VARCHAR,           -- idea|discovery|concern|decision|pattern|rejected (NULL for session-level)
    entities    VARCHAR,           -- JSON array of key entities
    relevance   FLOAT,            -- 0.0-1.0 (NULL for session-level)
    model       VARCHAR NOT NULL,
    created_at  TIMESTAMP DEFAULT current_timestamp,
    PRIMARY KEY (session_id, entry_uuid)
);

CREATE TABLE IF NOT EXISTS migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_summaries_session ON summaries(session_id);
CREATE INDEX IF NOT EXISTS idx_summaries_category ON summaries(category);
