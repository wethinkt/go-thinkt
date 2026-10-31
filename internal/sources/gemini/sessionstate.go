package gemini

import (
	"os"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// jsonlScan is what a JSONL session file says about itself.
type jsonlScan struct {
	Meta       thinkt.SessionMeta
	HeaderID   string          // the header's sessionId
	MessageIDs map[string]bool // ids in the final message list (the supersede check)
}

// scanJSONL reads one JSONL session's metadata in one replay. stem is the
// file name without .jsonl; parentStem is the parent session's ID for a
// subagent file, else "". It returns nil, nil for a file with no header.
func scanJSONL(path, stem, parentStem string) (*jsonlScan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	r, err := replay(f)
	if err != nil || r == nil {
		return nil, err
	}

	id := stem
	if parentStem != "" {
		id = parentStem + "/" + stem
	}
	s := &jsonlScan{HeaderID: r.Header.SessionID, MessageIDs: make(map[string]bool, len(r.Messages))}
	m := thinkt.SessionMeta{
		ID:              id,
		ParentSessionID: parentStem,
		FullPath:        path,
		FileSize:        info.Size(),
		CreatedAt:       r.Header.StartTime,
		ModifiedAt:      r.Meta.LastUpdated,
		Title:           sessionTitle(r.Meta.Summary),
		Source:          thinkt.SourceGemini,
		ChunkCount:      1,
	}
	if m.ModifiedAt.IsZero() {
		m.ModifiedAt = info.ModTime()
	}
	var cost thinkt.SessionCost
	for _, msg := range r.Messages {
		s.MessageIDs[msg.ID] = true
		switch msg.Type {
		case "user":
			if text := joinText(parts(msg.Content)); m.FirstPrompt == "" && !isIgnoredUserContent(text) {
				m.FirstPrompt = text
			}
		case "gemini":
			if msg.Model != "" {
				m.Model = msg.Model
			}
			if u := usageFromTokens(msg.Tokens); u != nil {
				cost.InputTokens += u.InputTokens
				cost.OutputTokens += u.OutputTokens
				cost.CachedInputTokens += u.CacheReadInputTokens
			}
		}
	}
	if cost != (thinkt.SessionCost{}) {
		m.Cost = &cost
	}
	m.EntryCount = len(convert(r, id))
	s.Meta = m
	return s, nil
}

// sessionTitle returns a summary when it reads as a title: one line, not a
// JSON document (a subagent's summary is a report of file locations).
func sessionTitle(summary string) string {
	t := strings.TrimSpace(summary)
	if t == "" || strings.ContainsAny(t, "\n\r") || strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		return ""
	}
	return t
}
