package relay

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// appendAfterRead reads a session in full, then appends a line to its
// file, like an agent writing while the startup reconcile runs.
type appendAfterRead struct {
	thinkt.Store
	path, line string
	once       sync.Once
}

func (a *appendAfterRead) OpenSession(ctx context.Context, id string) (thinkt.SessionReader, error) {
	r, err := a.Store.OpenSession(ctx, id)
	if err != nil {
		return nil, err
	}
	defer r.Close() //nolint:errcheck
	m := &memReader{meta: r.Metadata()}
	for {
		e, err := r.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		m.entries = append(m.entries, e)
	}
	a.once.Do(func() {
		f, err := os.OpenFile(a.path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString(a.line + "\n")
			_ = f.Close()
		}
	})
	return m, nil
}

type memReader struct {
	meta    thinkt.SessionMeta
	entries []*thinkt.Entry
}

func (m *memReader) ReadNext() (*thinkt.Entry, error) {
	if len(m.entries) == 0 {
		return nil, io.EOF
	}
	e := m.entries[0]
	m.entries = m.entries[1:]
	return e, nil
}
func (m *memReader) Metadata() thinkt.SessionMeta { return m.meta }
func (m *memReader) Close() error                 { return nil }

// A line written while the startup reconcile runs still ships: the watcher
// is up before the reconcile reads anything.
func TestStartShipsWritesDuringReconcile(t *testing.T) {
	h := layoutHomes(t)
	qwen, _ := sources.RegistryFor(nil).Get(thinkt.SourceQwen)
	path := filepath.Join(h.qwen, "projects", "abc123", "chats", "s1.jsonl")
	reg := thinkt.NewRegistry()
	reg.Register(&appendAfterRead{Store: qwen, path: path,
		line: `{"uuid":"q3","sessionId":"s1","type":"user","timestamp":"2026-03-15T10:00:02Z","cwd":"/tmp/proj","message":{"role":"user","parts":[{"text":"late"}]}}`})
	rec := &recorder{}
	r := newTestRelay(t, h, rec, reg, BackfillAll)
	r.cfg.WatchDirs = []WatchDir{{Path: h.qwen, Source: "qwen", Config: qwen.WatchConfig()}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx) }()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if rec.sessions()["qwen/s1"]["q3"] {
			cancel()
			<-done
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf("line written during reconcile never shipped: %v", rec.sessions())
}
