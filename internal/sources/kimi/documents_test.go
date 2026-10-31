package kimi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestSessionDocumentsReadsPlanSnapshots(t *testing.T) {
	base := storeFixture(t)
	main := filepath.Join(base, "sessions", "wd_proj_abc", "session_p1", "agents", "main")
	writeFile(t, filepath.Join(main, "plans", "robin.md"), "final plan")
	writeFile(t, filepath.Join(main, "plan", "robin", "v1.md"), "one")
	writeFile(t, filepath.Join(main, "plan", "robin", "v2.md"), "two")
	writeFile(t, filepath.Join(main, "plan", "robin", "v10.md"), "ten")
	f, err := os.OpenFile(filepath.Join(main, "wire.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"type":"plan.revision","agentId":"main","id":"r2","version":2,"key":"plan/robin/v2.md","time":1760000000000}` + "\n")
	_ = f.Close()

	docs, err := newTestStore(t, base).SessionDocuments(context.Background(), "session_p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("%+v", docs)
	}
	d := docs[0]
	if d.Name != "robin.md" || d.Kind != thinkt.DocPlan || d.Final != "final plan" || len(d.Versions) != 3 {
		t.Fatalf("%+v", d)
	}
	vs := d.Versions
	if vs[0].Number != 1 || vs[1].Number != 2 || vs[2].Number != 10 || vs[2].Content != "ten" {
		t.Fatalf("%+v", vs)
	}
	if vs[1].EntryUUID == "" || vs[1].Timestamp.IsZero() || vs[0].EntryUUID != "" {
		t.Fatalf("revision link: %+v", vs)
	}
}

func TestSessionDocumentsNoneIsEmpty(t *testing.T) {
	docs, err := newTestStore(t, storeFixture(t)).SessionDocuments(context.Background(), "session_p1")
	if err != nil || len(docs) != 0 {
		t.Fatalf("%+v %v", docs, err)
	}
}
