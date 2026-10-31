package antigravity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// docTools names the tools that write brain documents.
var docTools = map[string]thinkt.DocTool{
	"write_to_file":              {Path: "TargetFile", Content: "CodeContent", Quoted: true},
	"write_file":                 {Path: "Path", Content: "Content", Quoted: true},
	"replace_file_content":       {Path: "TargetFile", Old: "TargetContent", New: "ReplacementContent", All: "AllowMultiple", Quoted: true},
	"edit_file":                  {Path: "Path", Old: "Target", New: "Replacement", Quoted: true},
	"multi_replace_file_content": {Path: "TargetFile", Chunks: "ReplacementChunks", Old: "TargetContent", New: "ReplacementContent", All: "AllowMultiple", Quoted: true},
}

// artifactKinds maps a metadata artifactType to a document kind.
var artifactKinds = map[string]string{
	"ARTIFACT_TYPE_IMPLEMENTATION_PLAN": thinkt.DocPlan,
	"ARTIFACT_TYPE_TASK":                thinkt.DocTask,
	"ARTIFACT_TYPE_WALKTHROUGH":         thinkt.DocWalkthrough,
}

var _ thinkt.DocumentStore = (*Store)(nil)

// SessionDocuments returns the documents in a conversation's brain folder,
// with the versions rebuilt from its tool calls. A document no call wrote
// has its final text and no versions.
func (s *Store) SessionDocuments(ctx context.Context, sessionID string) ([]thinkt.AgentDocument, error) {
	meta, err := s.GetSessionMeta(ctx, sessionID)
	if err != nil || meta == nil {
		return nil, err
	}
	brain := filepath.Dir(filepath.Dir(filepath.Dir(meta.FullPath)))
	tracker := thinkt.NewDocumentTracker(docTools, func(path string) bool {
		return filepath.Dir(path) == brain && strings.HasSuffix(path, ".md")
	})
	if err := thinkt.VisitEntries(ctx, s, sessionID, tracker.Entry); err != nil {
		return nil, err
	}
	docs := tracker.Documents()
	paths, _ := filepath.Glob(filepath.Join(brain, "*.md"))
	for _, path := range paths {
		if !slices.ContainsFunc(docs, func(d thinkt.AgentDocument) bool { return d.Path == path }) {
			text, ok := thinkt.ReadDocumentFile(path)
			if !ok {
				continue
			}
			name := filepath.Base(path)
			docs = append(docs, thinkt.AgentDocument{Name: name, Path: path, Kind: thinkt.DocumentKind(name), Final: text})
		}
	}
	for i := range docs {
		applyArtifactMeta(&docs[i])
	}
	slices.SortFunc(docs, func(a, b thinkt.AgentDocument) int { return strings.Compare(a.Name, b.Name) })
	return docs, nil
}

// applyArtifactMeta fills a document from its .metadata.json, if it has one.
func applyArtifactMeta(d *thinkt.AgentDocument) {
	data, err := os.ReadFile(d.Path + ".metadata.json")
	if err != nil {
		return
	}
	var m struct {
		ArtifactType string    `json:"artifactType"`
		Summary      string    `json:"summary"`
		UpdatedAt    time.Time `json:"updatedAt"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	if kind, ok := artifactKinds[m.ArtifactType]; ok {
		d.Kind = kind
	}
	d.Summary = m.Summary
	if !m.UpdatedAt.IsZero() {
		d.UpdatedAt = m.UpdatedAt
	}
}
