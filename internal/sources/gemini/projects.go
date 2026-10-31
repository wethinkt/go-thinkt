package gemini

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Gemini CLI keeps a project's sessions under ~/.gemini/tmp/<folder>/. The
// folder was named by sha256 of the project path and is renamed to a slug
// (from projects.json) when the project is next opened, so one project can
// have a leftover hash folder beside its slug folder. projectHash, written
// in every session file, is the same for both.

// projectFolder is one tmp/<folder> holding chats.
type projectFolder struct {
	Dir    string // folder name under tmp/
	Hash   string // projectHash
	Path   string // real project path, or "" when unknown
	IsSlug bool   // the folder name is not the hash
}

var hexHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// sha256Hex returns the lowercase hex sha256 of s.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// loadProjectsJSON returns projects.json as hash → path.
func loadProjectsJSON(baseDir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(baseDir, "projects.json"))
	if err != nil {
		return nil
	}
	var doc struct {
		Projects map[string]string `json:"projects"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	out := make(map[string]string, len(doc.Projects))
	for path := range doc.Projects {
		out[sha256Hex(path)] = path
	}
	return out
}

// fileProjectHash reads projectHash from a session file: the first line of
// a .jsonl file, or the top-level field of a .json file.
func fileProjectHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var head struct {
		ProjectHash string `json:"projectHash"`
	}
	if strings.HasSuffix(path, ".jsonl") {
		r := bufio.NewReaderSize(f, 4096)
		line, _ := r.ReadBytes('\n')
		if json.Unmarshal(line, &head) == nil {
			return head.ProjectHash
		}
		return ""
	}
	if json.NewDecoder(f).Decode(&head) == nil {
		return head.ProjectHash
	}
	return ""
}

// scanProjectFolders lists the tmp/ folders that hold chats, with each
// folder's projectHash and real path.
func scanProjectFolders(baseDir string) ([]projectFolder, error) {
	tmp := filepath.Join(baseDir, "tmp")
	entries, err := os.ReadDir(tmp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	known := loadProjectsJSON(baseDir)
	var out []projectFolder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(tmp, e.Name())
		chats := filepath.Join(dir, "chats")
		if st, err := os.Stat(chats); err != nil || !st.IsDir() {
			continue
		}
		f := projectFolder{Dir: e.Name(), IsSlug: !hexHash.MatchString(e.Name())}
		if b, err := os.ReadFile(filepath.Join(dir, ".project_root")); err == nil {
			f.Path = strings.TrimSpace(string(b))
		}
		f.Hash = firstSessionHash(chats)
		if f.Hash == "" && !f.IsSlug {
			f.Hash = e.Name()
		}
		if f.Hash == "" && f.Path != "" {
			f.Hash = sha256Hex(f.Path)
		}
		if f.Hash == "" {
			f.Hash = e.Name()
		}
		if f.Path == "" {
			f.Path = known[f.Hash]
		}
		out = append(out, f)
	}
	return out, nil
}

// firstSessionHash returns the projectHash of the first session file in a
// chats folder, in name order.
func firstSessionHash(chats string) string {
	names, _ := filepath.Glob(filepath.Join(chats, "*.json*"))
	sort.Strings(names)
	for _, n := range names {
		if !strings.HasSuffix(n, ".json") && !strings.HasSuffix(n, ".jsonl") {
			continue
		}
		if h := fileProjectHash(n); h != "" {
			return h
		}
	}
	return ""
}
