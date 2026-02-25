package embedding_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
)

func TestTextHash(t *testing.T) {
	hash := embedding.TextHash("hello world")
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("hello world")))
	if hash != expected {
		t.Fatalf("expected %s, got %s", expected, hash)
	}
}

func TestPrepareEntries(t *testing.T) {
	entries := []embedding.EntryText{
		{UUID: "e1", SessionID: "s1", Source: "claude", Text: "short text for embedding", Tier: embedding.TierConversation},
	}
	requests, mapping := embedding.PrepareEntries(entries, 2000, 200)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	if requests[0].ID != "claude:s1:e1_conversation_0" {
		t.Fatalf("expected id=claude:s1:e1_conversation_0, got %s", requests[0].ID)
	}
	if len(mapping) != 1 {
		t.Fatalf("expected 1 mapping entry, got %d", len(mapping))
	}
	if mapping[0].Tier != embedding.TierConversation {
		t.Fatalf("expected conversation tier, got %s", mapping[0].Tier)
	}
}
