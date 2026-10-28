package kimi

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestResumeCommandUsesParentIDAndProjectPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable needs a POSIX shell")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	store := NewStore(t.TempDir())
	for _, id := range []string{"session_p1", "session_p1/agent-0"} {
		info, err := store.ResumeCommand(thinkt.SessionMeta{ID: id, ProjectPath: "/proj"})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if info.Command != filepath.Join(bin, "kimi") || info.Dir != "/proj" || len(info.Args) != 3 || info.Args[0] != "kimi" || info.Args[1] != "--session" || info.Args[2] != "session_p1" {
			t.Fatalf("%s → %+v", id, info)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := store.ResumeCommand(thinkt.SessionMeta{ID: "session_p1", ProjectPath: "/proj"}); err == nil {
		t.Fatal("missing kimi binary must error")
	}
}
