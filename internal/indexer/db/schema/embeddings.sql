-- Schema for thinkt embeddings database (separate from main index)
-- Embeddings are a derived cache with a different lifecycle: model changes
-- invalidate them entirely, and they grow significantly larger than the index.

CREATE TABLE IF NOT EXISTS embeddings (
    id          VARCHAR PRIMARY KEY,    -- "{source}:{session_id}:{entry_uuid}_{chunk_index}"
    session_id  VARCHAR NOT NULL,
    entry_uuid  VARCHAR NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    model       VARCHAR NOT NULL,       -- e.g. "qwen3-embedding-0.6b"
    dim         INTEGER NOT NULL,       -- 1024 for Qwen3-Embedding
    embedding   FLOAT[1024] NOT NULL,
    text_hash   VARCHAR NOT NULL,       -- SHA-256, detect changes without re-embedding
    created_at  TIMESTAMP DEFAULT current_timestamp,
    UNIQUE(session_id, entry_uuid, chunk_index, model)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_session ON embeddings(session_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_entry ON embeddings(session_id, entry_uuid);
