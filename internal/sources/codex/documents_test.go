package codex

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestSessionDocumentsCollectsPlans(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "sessions", "2026", "10", "01", "rollout-2026-10-01T00-00-00-s.jsonl")
	args := `{"explanation":"why","plan":[{"step":"read","status":"completed"},{"step":"write","status":"in_progress"},{"step":"test","status":"pending"}]}`
	writeLines(t, path,
		`{"timestamp":"2026-10-01T00:00:00Z","type":"session_meta","payload":{"id":"s","cwd":"/p"}}`,
		fmt.Sprintf(`{"timestamp":"2026-10-01T00:00:01Z","type":"response_item","payload":{"type":"function_call","name":"update_plan","call_id":"u","arguments":%q}}`, args),
		`{"timestamp":"2026-10-01T00:00:02Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Plan","id":"pl","text":"the plan"}}}`)
	docs, err := NewStore(base).SessionDocuments(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Name != "plan" || docs[0].Kind != thinkt.DocPlan || len(docs[0].Versions) != 2 {
		t.Fatalf("%+v", docs)
	}
	want := "why\n\n- [x] read\n- [~] write\n- [ ] test\n"
	if got := docs[0].Versions[0]; got.Content != want || got.Tool != "update_plan" {
		t.Fatalf("%q", got.Content)
	}
	if docs[0].Final != "the plan" {
		t.Fatalf("%q", docs[0].Final)
	}
}
