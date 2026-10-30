package antigravity

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// pathArgs maps each path-bearing tool to the argument holding the path
// and the relation it records. Edit arguments name every file a CODE_ACTION
// result reports, so result text is not parsed.
var pathArgs = map[string]struct{ arg, reason string }{
	"view_file":                  {"AbsolutePath", thinkt.RelatedRead},
	"read_file":                  {"Path", thinkt.RelatedRead},
	"list_dir":                   {"DirectoryPath", thinkt.RelatedRead},
	"grep_search":                {"SearchPath", thinkt.RelatedRead},
	"replace_file_content":       {"TargetFile", thinkt.RelatedEdited},
	"multi_replace_file_content": {"TargetFile", thinkt.RelatedEdited},
	"write_to_file":              {"TargetFile", thinkt.RelatedEdited},
	"edit_file":                  {"Path", thinkt.RelatedEdited},
	"write_file":                 {"Path", thinkt.RelatedEdited},
	"run_command":                {"Cwd", thinkt.RelatedCommand},
}

// unquotePath returns a path argument without the JSON quotes Antigravity
// sometimes stores around it ("\"/repo\"").
func unquotePath(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}

// repoRoot returns the nearest ancestor of path (or path itself) that
// contains .git, or "" when there is none. The path itself need not exist.
func repoRoot(path string) string {
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	dir := filepath.Clean(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// within reports whether path is dir or below it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolved returns path with symlinks resolved, or path when that fails.
func resolved(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// relatedTracker gathers the folders outside the project a conversation
// touched, folded to their repo roots.
type relatedTracker struct {
	project string
	exclude []string
	byPath  map[string]*thinkt.RelatedPath
}

// newRelatedTracker returns a tracker for a conversation whose project is
// projectPath. Paths under any exclude prefix (the Antigravity home, the
// system temp dirs) are never recorded.
func newRelatedTracker(projectPath string, exclude []string) *relatedTracker {
	r := &relatedTracker{byPath: map[string]*thinkt.RelatedPath{}}
	if projectPath != "" {
		r.project = resolved(filepath.Clean(projectPath))
	}
	for _, e := range exclude {
		if e != "" {
			r.exclude = append(r.exclude, resolved(filepath.Clean(e)))
		}
	}
	return r
}

// call records the path a tool call touched, if the tool carries one.
func (r *relatedTracker) call(name string, args map[string]any) {
	if r == nil {
		return
	}
	pa, ok := pathArgs[name]
	if !ok {
		return
	}
	s, _ := args[pa.arg].(string)
	if root := r.root(repoRoot(unquotePath(s))); root != "" {
		rp := r.entry(root)
		rp.Count++
		r.addReason(rp, pa.reason)
	}
}

// workspace records another workspace folder of the conversation; it adds a
// reason, not a count. A folder outside any repo is kept as it is.
func (r *relatedTracker) workspace(path string) {
	if r == nil {
		return
	}
	path = unquotePath(path)
	root := repoRoot(path)
	if root == "" {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			root = filepath.Clean(path)
		}
	}
	if root = r.root(root); root != "" {
		r.addReason(r.entry(root), thinkt.RelatedWorkspace)
	}
}

// root returns a repo root when it is outside the project and every
// exclude prefix, "" otherwise.
func (r *relatedTracker) root(root string) string {
	if root == "" {
		return ""
	}
	real := resolved(root)
	if r.project != "" && (within(real, r.project) || within(r.project, real)) {
		return ""
	}
	for _, e := range r.exclude {
		if within(real, e) {
			return ""
		}
	}
	return root
}

func (r *relatedTracker) entry(root string) *thinkt.RelatedPath {
	rp := r.byPath[root]
	if rp == nil {
		rp = &thinkt.RelatedPath{Path: root}
		r.byPath[root] = rp
	}
	return rp
}

func (r *relatedTracker) addReason(rp *thinkt.RelatedPath, reason string) {
	if !slices.Contains(rp.Reasons, reason) {
		rp.Reasons = append(rp.Reasons, reason)
		slices.Sort(rp.Reasons)
	}
}

// paths returns the related folders, most-touched first, then by path; nil
// when there are none.
func (r *relatedTracker) paths() []thinkt.RelatedPath {
	if r == nil || len(r.byPath) == 0 {
		return nil
	}
	out := make([]thinkt.RelatedPath, 0, len(r.byPath))
	for _, rp := range r.byPath {
		out = append(out, *rp)
	}
	slices.SortFunc(out, func(a, b thinkt.RelatedPath) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}
