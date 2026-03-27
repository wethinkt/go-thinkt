package embedding

import (
	"crypto/sha256"
	"fmt"
)

// EmbedRequest holds an ID and text for embedding a chunk.
type EmbedRequest struct {
	ID   string
	Text string
}

// EntryText holds the extracted text for an entry, ready for embedding.
type EntryText struct {
	UUID      string
	SessionID string
	Source    string // used to scope embedding IDs across sources
	Text      string
	Tier      Tier
}

// ChunkMapping tracks which chunk maps back to which entry.
type ChunkMapping struct {
	EntryUUID  string
	SessionID  string
	ChunkIndex int
	TextHash   string
	Tier       Tier
}

// TextHash returns the SHA-256 hex digest of the given text.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

// PrepareEntries takes extracted entry texts, chunks them, and returns
// EmbedRequests suitable for the Client, plus a mapping from request ID
// back to entry/chunk metadata.
func PrepareEntries(entries []EntryText, maxChars, overlap int) ([]EmbedRequest, []ChunkMapping) {
	var requests []EmbedRequest
	var mapping []ChunkMapping

	for _, e := range entries {
		chunks := ChunkText(e.Text, maxChars, overlap)
		for i, chunk := range chunks {
			id := fmt.Sprintf("%s:%s:%s_%s_%d", e.Source, e.SessionID, e.UUID, e.Tier, i)
			requests = append(requests, EmbedRequest{
				ID:   id,
				Text: chunk,
			})
			mapping = append(mapping, ChunkMapping{
				EntryUUID:  e.UUID,
				SessionID:  e.SessionID,
				ChunkIndex: i,
				TextHash:   TextHash(chunk),
				Tier:       e.Tier,
			})
		}
	}

	return requests, mapping
}
