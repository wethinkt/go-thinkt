package gemini

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

const devProj = "/Users/dev/proj"

func hashOf(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

// put writes content to base/rel, creating directories.
func put(t *testing.T, base, rel, content string) string {
	t.Helper()
	p := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func header(hash string) string {
	return `{"sessionId":"s","projectHash":"` + hash + `","startTime":"2026-05-17T11:00:00Z","lastUpdated":"2026-05-17T11:00:00Z","kind":"main"}` + "\n"
}

func folderByDir(fs []projectFolder, dir string) *projectFolder {
	for i := range fs {
		if fs[i].Dir == dir {
			return &fs[i]
		}
	}
	return nil
}

func TestProjectFoldersHashFromHeader(t *testing.T) {
	base := t.TempDir()
	put(t, base, "tmp/proj/chats/session-a.jsonl", header("abc"))
	put(t, base, "tmp/proj/.project_root", devProj+"\n")
	fs, err := scanProjectFolders(base)
	if err != nil {
		t.Fatal(err)
	}
	f := folderByDir(fs, "proj")
	if f == nil || f.Hash != "abc" || f.Path != devProj || !f.IsSlug {
		t.Fatalf("%+v", fs)
	}
}

func TestProjectFoldersPathFromProjectsJSON(t *testing.T) {
	base := t.TempDir()
	h := hashOf(devProj)
	put(t, base, "projects.json", `{"projects":{"`+devProj+`":"proj"}}`)
	put(t, base, "tmp/"+h+"/chats/session-a.json", `{"sessionId":"s","projectHash":"`+h+`","messages":[]}`)
	fs, err := scanProjectFolders(base)
	if err != nil {
		t.Fatal(err)
	}
	f := folderByDir(fs, h)
	if f == nil || f.Hash != h || f.Path != devProj || f.IsSlug {
		t.Fatalf("%+v", fs)
	}
}

func TestProjectFoldersUnknownPath(t *testing.T) {
	base := t.TempDir()
	h := hashOf("/somewhere/else")
	put(t, base, "tmp/"+h+"/chats/session-a.jsonl", header(h))
	fs, _ := scanProjectFolders(base)
	if f := folderByDir(fs, h); f == nil || f.Path != "" || f.Hash != h {
		t.Fatalf("%+v", fs)
	}
}

func TestProjectFoldersHashFromProjectRoot(t *testing.T) {
	base := t.TempDir()
	put(t, base, "tmp/proj/chats/.keep", "")
	put(t, base, "tmp/proj/.project_root", devProj)
	fs, _ := scanProjectFolders(base)
	if f := folderByDir(fs, "proj"); f == nil || f.Hash != hashOf(devProj) {
		t.Fatalf("%+v", fs)
	}
}

func TestProjectFoldersSkipNoChats(t *testing.T) {
	base := t.TempDir()
	put(t, base, "tmp/bin/.project_root", devProj)
	fs, _ := scanProjectFolders(base)
	if len(fs) != 0 {
		t.Fatalf("%+v", fs)
	}
}
