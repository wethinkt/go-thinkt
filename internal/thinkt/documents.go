package thinkt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Agent document kinds.
const (
	DocPlan        = "plan"
	DocTask        = "task"
	DocWalkthrough = "walkthrough"
	DocOther       = "other"
)

// AgentDocument is a document an agent keeps alongside a conversation, with
// the versions the session wrote. Path is empty for a plan that exists only
// in the conversation. Final is the file on disk, or the last version.
type AgentDocument struct {
	Name      string            `json:"name"`
	Path      string            `json:"path,omitempty"`
	Kind      string            `json:"kind"`
	Summary   string            `json:"summary,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitzero"`
	Final     string            `json:"final,omitempty"`
	Versions  []DocumentVersion `json:"versions,omitempty"`
}

// DocumentVersion is one state of a document. EntryUUID names the entry that
// produced it. Partial means the history was incomplete, so Content is the
// text known so far rather than the document as it stood.
type DocumentVersion struct {
	Number    int       `json:"number"`
	EntryUUID string    `json:"entry_uuid,omitempty"`
	Timestamp time.Time `json:"timestamp,omitzero"`
	Tool      string    `json:"tool,omitempty"`
	Content   string    `json:"content"`
	Partial   bool      `json:"partial,omitempty"`
}

// DocumentStore is implemented by stores whose sessions keep documents.
type DocumentStore interface {
	SessionDocuments(ctx context.Context, sessionID string) ([]AgentDocument, error)
}

// DocTool names the arguments of a tool that writes documents. Content is a
// full write; Old and New an edit; All the flag replacing every match. With
// Chunks, the edits are a list of objects holding Old and New.
type DocTool struct {
	Path, Content, Old, New, All, Chunks string
}

// DocumentKind guesses a document's kind from its file name.
func DocumentKind(name string) string {
	switch stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name))); {
	case strings.Contains(stem, "plan"), strings.Contains(stem, "roadmap"):
		return DocPlan
	case strings.Contains(stem, "task"):
		return DocTask
	case strings.Contains(stem, "walkthrough"):
		return DocWalkthrough
	}
	return DocOther
}

// DocumentTracker rebuilds a session's documents from its entries.
type DocumentTracker struct {
	tools map[string]DocTool
	isDoc func(path string) bool
	docs  map[string]*AgentDocument // by Path, or by Name with no path
}

// NewDocumentTracker returns a tracker over tools, for the paths isDoc accepts.
func NewDocumentTracker(tools map[string]DocTool, isDoc func(path string) bool) *DocumentTracker {
	return &DocumentTracker{tools: tools, isDoc: isDoc, docs: map[string]*AgentDocument{}}
}

// Entry feeds one entry's tool calls to the tracker.
func (d *DocumentTracker) Entry(e *Entry) {
	for _, b := range e.ContentBlocks {
		dt, ok := d.tools[b.ToolName]
		args, isMap := b.ToolInput.(map[string]any)
		if !ok || !isMap {
			continue
		}
		s, _ := args[dt.Path].(string)
		path := UnquotePath(s)
		if path == "" || d.isDoc == nil || !d.isDoc(path) {
			continue
		}
		name := filepath.Base(path)
		doc := d.doc(path, name, path, DocumentKind(name))
		content, partial := apply(dt, args, doc)
		add(doc, e, b.ToolName, content, partial)
	}
}

// Note adds a version of a document stated in the conversation.
func (d *DocumentTracker) Note(name, kind string, e *Entry, tool, text string) {
	add(d.doc(name, name, "", kind), e, tool, text, false)
}

// Documents returns the documents by name, each with Final and UpdatedAt set.
func (d *DocumentTracker) Documents() []AgentDocument {
	out := make([]AgentDocument, 0, len(d.docs))
	for _, doc := range d.docs {
		last := doc.Versions[len(doc.Versions)-1]
		doc.Final, doc.UpdatedAt = last.Content, last.Timestamp
		if doc.Path != "" {
			if data, err := os.ReadFile(doc.Path); err == nil {
				doc.Final = string(data)
			}
		}
		out = append(out, *doc)
	}
	slices.SortFunc(out, func(a, b AgentDocument) int { return strings.Compare(a.Name+a.Path, b.Name+b.Path) })
	return out
}

func (d *DocumentTracker) doc(key, name, path, kind string) *AgentDocument {
	doc := d.docs[key]
	if doc == nil {
		doc = &AgentDocument{Name: name, Path: path, Kind: kind}
		d.docs[key] = doc
	}
	return doc
}

func add(doc *AgentDocument, e *Entry, tool, content string, partial bool) {
	doc.Versions = append(doc.Versions, DocumentVersion{
		Number: len(doc.Versions) + 1, EntryUUID: e.UUID, Timestamp: e.Timestamp,
		Tool: tool, Content: content, Partial: partial,
	})
}

// apply returns the document's text after a call, and whether part of the
// call could not be applied.
func apply(dt DocTool, args map[string]any, doc *AgentDocument) (string, bool) {
	if dt.Content != "" {
		if s, ok := args[dt.Content].(string); ok {
			return unquoteText(s), false
		}
	}
	if len(doc.Versions) == 0 {
		return "", true
	}
	text := doc.Versions[len(doc.Versions)-1].Content
	all, _ := args[dt.All].(bool)
	partial := false
	for _, c := range edits(dt, args) {
		oldText, _ := c[dt.Old].(string)
		newText, _ := c[dt.New].(string)
		oldText, newText = unquoteText(oldText), unquoteText(newText)
		switch {
		case oldText == "" || !strings.Contains(text, oldText):
			partial = true
		case all:
			text = strings.ReplaceAll(text, oldText, newText)
		default:
			text = strings.Replace(text, oldText, newText, 1)
		}
	}
	return text, partial
}

// unquoteText returns a text argument without the JSON quoting some sources
// store around it.
func unquoteText(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var u string
		if json.Unmarshal([]byte(s), &u) == nil {
			return u
		}
	}
	return s
}

// edits returns a call's edits: its chunks (a list, or JSON text of one), or
// the call itself.
func edits(dt DocTool, args map[string]any) []map[string]any {
	if dt.Chunks == "" {
		return []map[string]any{args}
	}
	raw := args[dt.Chunks]
	if s, ok := raw.(string); ok {
		_ = json.Unmarshal([]byte(s), &raw)
	}
	list, _ := raw.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// VisitEntries calls visit for every entry of a session.
func VisitEntries(ctx context.Context, store Store, sessionID string, visit func(*Entry)) error {
	r, err := store.OpenSession(ctx, sessionID)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for {
		e, err := r.ReadNext()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		visit(e)
	}
}
