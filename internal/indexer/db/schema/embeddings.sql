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

COMMENT ON TABLE embeddings IS 'Vector store for conversation snippets and reasoning blocks.';
COMMENT ON COLUMN embeddings.id IS 'Globally unique identifier for the embedding chunk.';
COMMENT ON COLUMN embeddings.session_id IS 'Unique identifier for the parent session.';
COMMENT ON COLUMN embeddings.entry_uuid IS 'Unique identifier for the specific message entry.';
COMMENT ON COLUMN embeddings.chunk_index IS 'Position of the chunk if the source text was split.';
COMMENT ON COLUMN embeddings.tier IS 'Source of the text: "conversation" (messages) or "reasoning" (internal thinking).';
COMMENT ON COLUMN embeddings.model IS 'The embedding model name used to generate this vector.';
COMMENT ON COLUMN embeddings.dim IS 'The number of dimensions in the embedding vector.';
COMMENT ON COLUMN embeddings.embedding IS 'The fixed-size float array representing the vector embedding.';
COMMENT ON COLUMN embeddings.text_hash IS 'Hash of the source text to detect changes and avoid redundant re-embedding.';

-- Migration tracking
CREATE TABLE IF NOT EXISTS migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE migrations IS 'Tracks schema versioning for database upgrades.';
COMMENT ON COLUMN migrations.version IS 'The migration version number.';
COMMENT ON COLUMN migrations.applied_at IS 'Timestamp when the migration was applied.';

CREATE INDEX IF NOT EXISTS idx_embeddings_session ON embeddings(session_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_entry ON embeddings(session_id, entry_uuid);
CREATE INDEX IF NOT EXISTS idx_embeddings_tier ON embeddings(tier);
