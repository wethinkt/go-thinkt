-- Schema for thinkt embeddings database (separate from main index)
-- Embeddings are a derived cache with a different lifecycle: model changes
-- invalidate them entirely, and they grow significantly larger than the index.
-- NOTE: {DIM} is replaced at runtime with the actual embedding dimension.

CREATE TABLE IF NOT EXISTS embeddings (
    id          VARCHAR PRIMARY KEY,    -- "{source}:{session_id}:{entry_uuid}_{tier}_{chunk_index}"
    session_id  VARCHAR NOT NULL,
    entry_uuid  VARCHAR NOT NULL,
    chunk_index INTEGER NOT NULL DEFAULT 0,
    tier        VARCHAR NOT NULL DEFAULT 'conversation',  -- "conversation" or "reasoning"
    model       VARCHAR NOT NULL,       -- e.g. "nomic-embed-text-v1.5"
    dim         INTEGER NOT NULL,       -- embedding dimension (e.g. 768, 1024)
    embedding   FLOAT[{DIM}] NOT NULL,
    text_hash   VARCHAR NOT NULL,       -- SHA-256, detect changes without re-embedding
    created_at  TIMESTAMP DEFAULT current_timestamp,
    UNIQUE(session_id, entry_uuid, chunk_index, tier, model)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_session ON embeddings(session_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_entry ON embeddings(session_id, entry_uuid);
CREATE INDEX IF NOT EXISTS idx_embeddings_tier ON embeddings(tier);
