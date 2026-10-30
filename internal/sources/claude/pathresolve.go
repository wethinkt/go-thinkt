package claude

import (
	"context"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// ResolveSessionPath implements thinkt.SessionPathResolver for a .jsonl
// file under the Claude home. A transcript directly in a project folder
// carries that folder's DirPath as its project id.
func (d *Discoverer) ResolveSessionPath(path string) (thinkt.SessionPath, bool) {
	if !IsSessionPath(path) {
		return thinkt.SessionPath{}, false
	}
	home := d.basePath()
	sp := thinkt.SessionPath{Source: thinkt.SourceClaude, BaseDir: home, SessionID: thinkt.HomeSpelling(path, home)}
	projectDir := filepath.Dir(path)
	if filepath.Base(filepath.Dir(projectDir)) == "projects" {
		sp.ProjectID = filepath.Join(home, "projects", filepath.Base(projectDir))
	}
	return sp, true
}

// OpenSessionPath implements thinkt.SessionPathResolver.
func (d *Discoverer) OpenSessionPath(_ context.Context, _ thinkt.SessionPath, path string) (thinkt.LazySession, error) {
	return OpenLazySession(path)
}
