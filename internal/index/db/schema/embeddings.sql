-- Schema for thinkt embeddings database (sqlite-vec)
-- This file is a template: {DIM} is replaced at runtime with the embedding dimension.

CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec0(
    embedding_id INTEGER PRIMARY KEY,
    embedding float[{DIM}] distance_metric=cosine,
    session_id TEXT,
    tier TEXT,
    model TEXT
);

CREATE TABLE IF NOT EXISTS embedding_meta (
    embedding_id INTEGER PRIMARY KEY,
    entry_uuid   TEXT NOT NULL,
    chunk_index  INTEGER NOT NULL DEFAULT 0,
    text_hash    TEXT NOT NULL,
    created_at   TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
