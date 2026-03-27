-- Schema for thinkt summaries database
-- LLM-extracted summaries with categories and entities.
-- Prunable: delete file on model change and regenerate.

CREATE TABLE IF NOT EXISTS summaries (
    session_id  TEXT NOT NULL,
    entry_uuid  TEXT NOT NULL,
    summary     TEXT NOT NULL,
    category    TEXT,
    entities    TEXT,
    relevance   REAL,
    model       TEXT NOT NULL,
    created_at  TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (session_id, entry_uuid)
);

CREATE INDEX IF NOT EXISTS idx_summaries_session ON summaries(session_id);
CREATE INDEX IF NOT EXISTS idx_summaries_category ON summaries(category);

CREATE TABLE IF NOT EXISTS migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
