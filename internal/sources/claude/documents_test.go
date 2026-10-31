package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestSessionDocumentsRebuildsPlans(t *testing.T) {
	t.Setenv("THINKT_HOME", t.TempDir())
	home := filepath.Join(t.TempDir(), "claude")
	plan := filepath.Join(home, "plans", "calm-otter.md")
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte("on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := func(uuid, name, input string) string {
		return fmt.Sprintf(`{"type":"assistant","uuid":%q,"timestamp":"2026-10-01T00:00:00Z","cwd":"/p","message":{"role":"assistant","model":"claude-opus-4","content":[{"type":"tool_use","id":%q,"name":%q,"input":%s}]}}`, uuid, uuid, name, input)
	}
	id := "11111111-2222-4333-8444-555555555555"
	dir := filepath.Join(home, "projects", "-p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTempSession(t, dir, id+".jsonl",
		tool("e1", "Write", fmt.Sprintf(`{"file_path":%q,"content":"first draft"}`, plan))+"\n"+
			tool("e2", "Edit", fmt.Sprintf(`{"file_path":%q,"old_string":"first","new_string":"second"}`, plan))+"\n"+
			tool("e3", "Write", `{"file_path":"/p/code.go","content":"x"}`)+"\n"+
			tool("e4", "ExitPlanMode", `{"plan":"the plan"}`)+"\n")
	docs, err := NewStore(home).SessionDocuments(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("%+v", docs)
	}
	file, stated := docs[0], docs[1]
	if file.Name != "calm-otter.md" || file.Final != "on disk" || len(file.Versions) != 2 || file.Versions[1].Content != "second draft" {
		t.Fatalf("%+v", file)
	}
	if stated.Name != "plan" || stated.Kind != thinkt.DocPlan || stated.Final != "the plan" || stated.Versions[0].EntryUUID != "e4" {
		t.Fatalf("%+v", stated)
	}
}

// A path that climbs out of plans/ is not a document, so its file is never read.
func TestSessionDocumentsRefusesTraversal(t *testing.T) {
	t.Setenv("THINKT_HOME", t.TempDir())
	home := filepath.Join(t.TempDir(), "claude")
	if err := os.MkdirAll(filepath.Join(home, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "secret.md"), []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := "11111111-2222-4333-8444-555555555555"
	dir := filepath.Join(home, "projects", "-p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"assistant","uuid":"e1","timestamp":"2026-10-01T00:00:00Z","cwd":"/p","message":{"role":"assistant","model":"claude-opus-4","content":[{"type":"tool_use","id":"t","name":"Write","input":{"file_path":%q,"content":"x"}}]}}`, filepath.Join(home, "plans")+"/../secret.md")
	writeTempSession(t, dir, id+".jsonl", line+"\n")
	docs, err := NewStore(home).SessionDocuments(context.Background(), id)
	if err != nil || len(docs) != 0 {
		t.Fatalf("%+v %v", docs, err)
	}
}
