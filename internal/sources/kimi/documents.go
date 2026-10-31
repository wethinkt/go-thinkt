package kimi

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var _ thinkt.DocumentStore = (*Store)(nil)

// SessionDocuments returns an agent's plans: the final file under plans/ and
// its snapshots under plan/<name>/, each linked to its plan.revision entry.
func (s *Store) SessionDocuments(ctx context.Context, sessionID string) ([]thinkt.AgentDocument, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil || meta == nil {
		return nil, err
	}
	dir := filepath.Dir(meta.FullPath)
	paths, _ := filepath.Glob(filepath.Join(dir, "plans", "*.md"))
	if len(paths) == 0 {
		return nil, nil
	}
	revisions := map[string]*thinkt.Entry{} // by key, relative to the agent dir
	err = thinkt.VisitEntries(ctx, s, sessionID, func(e *thinkt.Entry) {
		if e.Subtype == "plan" && e.Kind == thinkt.KindAttachment {
			revisions[e.Text] = e
		}
	})
	if err != nil {
		return nil, err
	}
	var docs []thinkt.AgentDocument
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		d := thinkt.AgentDocument{Name: name, Path: path, Kind: thinkt.DocPlan, Final: string(data)}
		if info, err := os.Stat(path); err == nil {
			d.UpdatedAt = info.ModTime()
		}
		d.Versions = planVersions(dir, strings.TrimSuffix(name, ".md"), revisions)
		docs = append(docs, d)
	}
	return docs, nil
}

// planVersions reads a plan's snapshots in version order.
func planVersions(dir, key string, revisions map[string]*thinkt.Entry) []thinkt.DocumentVersion {
	files, _ := filepath.Glob(filepath.Join(dir, "plan", key, "v*.md"))
	var out []thinkt.DocumentVersion
	for _, f := range files {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "v"), ".md"))
		if err != nil {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		v := thinkt.DocumentVersion{Number: n, Tool: "plan.revision", Content: string(data)}
		if e := revisions["plan/"+key+"/"+filepath.Base(f)]; e != nil {
			v.EntryUUID, v.Timestamp = e.UUID, e.Timestamp
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b thinkt.DocumentVersion) int { return a.Number - b.Number })
	return out
}
