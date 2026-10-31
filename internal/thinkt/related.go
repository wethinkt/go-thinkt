package thinkt

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ToolPath names the argument of a tool that holds a path, and the relation
// a call records.
type ToolPath struct{ Arg, Reason string }

// ToolPaths maps a source's path-bearing tools to their ToolPath.
type ToolPaths map[string]ToolPath

// UnquotePath returns a path argument without the JSON quotes some sources
// store around it ("\"/repo\"").
func UnquotePath(s string) string {
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

// RelatedExclude returns the path prefixes a source never records as related
// folders: its own home and the system temp dirs.
func RelatedExclude(home string) []string {
	return []string{home, os.TempDir(), "/tmp", "/private/tmp"}
}

// RelatedTracker gathers the folders outside the project a session touched,
// folded to their repo roots. A nil tracker records nothing.
type RelatedTracker struct {
	base    string // the project as given, for relative paths
	project string // symlinks resolved
	exclude []string
	byPath  map[string]*RelatedPath
	roots   map[string]string // directory → its repo root, "" for none
}

// NewRelatedTracker returns a tracker for a session whose project is
// projectPath. Paths under any exclude prefix are never recorded.
func NewRelatedTracker(projectPath string, exclude []string) *RelatedTracker {
	r := &RelatedTracker{byPath: map[string]*RelatedPath{}, roots: map[string]string{}}
	r.SetProject(projectPath)
	for _, e := range exclude {
		if e != "" {
			r.exclude = append(r.exclude, resolved(filepath.Clean(e)))
		}
	}
	return r
}

// SetProject names the session's project, for a source that learns it while
// reading the session. The first project named stays.
func (r *RelatedTracker) SetProject(projectPath string) {
	if r == nil || r.project != "" || projectPath == "" {
		return
	}
	r.base = filepath.Clean(projectPath)
	r.project = resolved(r.base)
}

// HasProject reports whether the tracker knows its project.
func (r *RelatedTracker) HasProject() bool {
	return r != nil && r.project != ""
}

// TrackRelated returns a tracker for a session of projectPath, or nil with
// no project: "outside the project" means nothing without one.
func TrackRelated(projectPath string, exclude []string) *RelatedTracker {
	if projectPath == "" {
		return nil
	}
	return NewRelatedTracker(projectPath, exclude)
}

// Call records the path a tool call touched, if tools lists the tool.
func (r *RelatedTracker) Call(tools ToolPaths, name string, args map[string]any, base string) {
	if r == nil {
		return
	}
	if tp, ok := tools[name]; ok {
		s, _ := args[tp.Arg].(string)
		r.Touch(s, base, tp.Reason)
	}
}

// Blocks records the tool calls among blocks.
func (r *RelatedTracker) Blocks(tools ToolPaths, blocks []ContentBlock, base string) {
	if r == nil {
		return
	}
	for _, b := range blocks {
		if args, ok := b.ToolInput.(map[string]any); ok && b.ToolName != "" {
			r.Call(tools, b.ToolName, args, base)
		}
	}
}

// Touch records one tool call on path. A relative path joins base, or the
// project with no base; it is dropped when there is neither.
func (r *RelatedTracker) Touch(path, base, reason string) {
	if r == nil {
		return
	}
	path = UnquotePath(path)
	if path == "" {
		return
	}
	if !filepath.IsAbs(path) {
		if base = UnquotePath(base); base == "" {
			base = r.base
		}
		if base == "" {
			return
		}
		path = filepath.Join(base, path)
	}
	path = filepath.Clean(path)
	if r.project != "" && within(path, r.project) {
		return
	}
	if root := r.keep(r.lookupRoot(path)); root != "" {
		rp := r.entry(root)
		rp.Count++
		r.addReason(rp, reason)
	}
}

// Workspace records another workspace folder of the session; it adds a
// reason, not a count. A folder outside any repo is kept as it is.
func (r *RelatedTracker) Workspace(path string) {
	if r == nil {
		return
	}
	path = UnquotePath(path)
	root := repoRoot(path)
	if root == "" {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			root = filepath.Clean(path)
		}
	}
	if root = r.keep(root); root != "" {
		r.addReason(r.entry(root), RelatedWorkspace)
	}
}

// lookupRoot is repoRoot, asked once per directory: sessions make thousands
// of calls into the same few folders.
func (r *RelatedTracker) lookupRoot(path string) string {
	// The path itself may be a repo (a working folder, a directory argument).
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return path
	}
	dir := filepath.Dir(path)
	root, ok := r.roots[dir]
	if !ok {
		root = repoRoot(dir)
		r.roots[dir] = root
	}
	return root
}

// keep returns a repo root when it is outside the project and every exclude
// prefix, "" otherwise.
func (r *RelatedTracker) keep(root string) string {
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

func (r *RelatedTracker) entry(root string) *RelatedPath {
	rp := r.byPath[root]
	if rp == nil {
		rp = &RelatedPath{Path: root}
		r.byPath[root] = rp
	}
	return rp
}

func (r *RelatedTracker) addReason(rp *RelatedPath, reason string) {
	if !slices.Contains(rp.Reasons, reason) {
		rp.Reasons = append(rp.Reasons, reason)
		slices.Sort(rp.Reasons)
	}
}

// Paths returns the related folders, most-touched first, then by path; nil
// when there are none.
func (r *RelatedTracker) Paths() []RelatedPath {
	if r == nil || len(r.byPath) == 0 {
		return nil
	}
	out := make([]RelatedPath, 0, len(r.byPath))
	for _, rp := range r.byPath {
		out = append(out, *rp)
	}
	slices.SortFunc(out, func(a, b RelatedPath) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}
