package gemini

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// A JSONL session is append-only: a message is written again as it grows,
// and later records can edit ($patch), truncate ($rewindTo) or replace
// ($set.messages) what came before. replay rebuilds the message list the
// way the CLI's loader (createJsonlRecordAccumulator in gemini-cli
// chatRecordingService.ts) does, so thinkt shows the transcript the CLI
// would resume.

// jsonlToolCall is one tool call recorded on a gemini message, with its
// result inline.
type jsonlToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Args      map[string]any  `json:"args"`
	Result    json.RawMessage `json:"result"`
	Status    string          `json:"status"`
	AgentID   string          `json:"agentId"`
	Timestamp time.Time       `json:"timestamp"`
}

// lineMessage is one message as the CLI keeps it.
type lineMessage struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"` // user, gemini, info, warning, error
	Timestamp      time.Time       `json:"timestamp"`
	Content        json.RawMessage `json:"content"` // part list or string
	DisplayContent json.RawMessage `json:"displayContent"`
	ToolCalls      []jsonlToolCall `json:"toolCalls"`
	Thoughts       []Thought       `json:"thoughts"`
	Tokens         *Tokens         `json:"tokens"`
	Model          string          `json:"model"`

	Line       int   `json:"-"` // 1-based line of the copy that set it last
	patchLines []int // $patch lines that edited it
}

// replayHeader is the session header: the first line of a file, and any
// later line carrying sessionId and projectHash.
type replayHeader struct {
	SessionID   string    `json:"sessionId"`
	ProjectHash string    `json:"projectHash"`
	StartTime   time.Time `json:"startTime"`
	LastUpdated time.Time `json:"lastUpdated"`
	Kind        string    `json:"kind"`
	Directories []string  `json:"directories"`
}

// replayMeta holds the merged $set metadata fields thinkt reads.
type replayMeta struct {
	LastUpdated time.Time
	Summary     string // only when $set.summary is a string
}

// replayed is a rebuilt session.
type replayed struct {
	Header   replayHeader
	Meta     replayMeta
	Messages []lineMessage // final order
	// Live holds the 1-based line numbers whose content survives: the
	// final copy of each remaining message, the $patch lines that edited
	// one, and the lines of a live $set.messages checkpoint. The original
	// line of a patched message stays live too, which overstates a little.
	Live    map[int]bool
	Unknown thinkt.UnknownTally[string]
}

// orderedMessages is an insertion-ordered map: setting an existing id
// keeps its position, as a JavaScript Map does.
type orderedMessages struct {
	order []string
	byID  map[string]*lineMessage
}

func (o *orderedMessages) set(m *lineMessage) {
	if o.byID == nil {
		o.byID = map[string]*lineMessage{}
	}
	if _, ok := o.byID[m.ID]; !ok {
		o.order = append(o.order, m.ID)
	}
	o.byID[m.ID] = m
}

func (o *orderedMessages) remove(id string) {
	if _, ok := o.byID[id]; !ok {
		return
	}
	delete(o.byID, id)
	for i, v := range o.order {
		if v == id {
			o.order = append(o.order[:i], o.order[i+1:]...)
			return
		}
	}
}

func (o *orderedMessages) clear() {
	o.order, o.byID = nil, map[string]*lineMessage{}
}

// rewindTo deletes id and everything after it, or everything when id is
// not present.
func (o *orderedMessages) rewindTo(id string) {
	for i, v := range o.order {
		if v == id {
			for _, gone := range o.order[i:] {
				delete(o.byID, gone)
			}
			o.order = o.order[:i]
			return
		}
	}
	o.clear()
}

// reorder keeps the ids not listed first, in their order, then the listed
// ids that exist, in the listed order.
func (o *orderedMessages) reorder(ids []string) {
	listed := map[string]bool{}
	var tail []string
	for _, id := range ids {
		if _, ok := o.byID[id]; ok && !listed[id] {
			listed[id] = true
			tail = append(tail, id)
		}
	}
	var head []string
	for _, id := range o.order {
		if !listed[id] {
			head = append(head, id)
		}
	}
	o.order = append(head, tail...)
}

type toolCallPatch struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	hasRes bool
}

func (t *toolCallPatch) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	_ = json.Unmarshal(raw["id"], &t.ID)
	t.Result, t.hasRes = raw["result"]
	return nil
}

type messagePatch struct {
	ID         *string         `json:"id"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []toolCallPatch `json:"toolCalls"`
	Updates    []messagePatch  `json:"updates"`
	RemoveIDs  []string        `json:"removeIds"`
	OrderIDs   []string        `json:"orderIds"`
	hasContent bool
}

func (p *messagePatch) UnmarshalJSON(b []byte) error {
	type plain messagePatch
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*p = messagePatch(v)
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) == nil {
		_, p.hasContent = raw["content"]
	}
	return nil
}

// applyPatch edits one existing message in place; a patch for an id that
// is not present does nothing.
func (o *orderedMessages) applyPatch(p messagePatch, line int) {
	if p.ID == nil {
		return
	}
	m, ok := o.byID[*p.ID]
	if !ok {
		return
	}
	edited := false
	if p.hasContent {
		m.Content = p.Content
		edited = true
	}
	if m.Type == "gemini" && len(m.ToolCalls) > 0 {
		for _, tp := range p.ToolCalls {
			for i := range m.ToolCalls {
				if m.ToolCalls[i].ID == tp.ID && tp.hasRes {
					m.ToolCalls[i].Result = tp.Result
					edited = true
					break
				}
			}
		}
	}
	if edited {
		m.patchLines = append(m.patchLines, line)
	}
}

func (o *orderedMessages) addMessages(raw json.RawMessage, line int) {
	var msgs []json.RawMessage
	if json.Unmarshal(raw, &msgs) != nil {
		return
	}
	for _, r := range msgs {
		if m := decodeMessage(r, line); m != nil {
			o.set(m)
		}
	}
}

// decodeMessage decodes a message record: a string id and no $patch.
func decodeMessage(raw json.RawMessage, line int) *lineMessage {
	var m lineMessage
	if json.Unmarshal(raw, &m) != nil || m.ID == "" {
		return nil
	}
	m.Line = line
	return &m
}

// replay rebuilds a JSONL session. It returns nil, nil when the file has no
// header with sessionId and projectHash, as the CLI rejects such a file.
// Unparsable lines (a partial last line while the file is written) are
// skipped.
func replay(r io.Reader) (*replayed, error) {
	out := &replayed{Unknown: thinkt.UnknownTally[string]{Source: thinkt.SourceGemini}}
	var msgs orderedMessages
	msgs.clear()

	scanner := thinkt.NewScannerWithMaxCapacity(r)
	line := 0
	for scanner.Scan() {
		line++
		b := bytes.TrimSpace(scanner.Bytes())
		if len(b) == 0 {
			continue
		}
		var rec map[string]json.RawMessage
		if json.Unmarshal(b, &rec) != nil {
			continue
		}
		var s string
		switch {
		case json.Unmarshal(rec["$rewindTo"], &s) == nil && rec["$rewindTo"] != nil:
			msgs.rewindTo(s)
		case isObject(rec["$patch"]):
			var p messagePatch
			if json.Unmarshal(rec["$patch"], &p) != nil {
				continue
			}
			msgs.applyPatch(p, line)
			for _, u := range p.Updates {
				msgs.applyPatch(u, line)
			}
			for _, id := range p.RemoveIDs {
				msgs.remove(id)
			}
			if p.OrderIDs != nil {
				msgs.reorder(p.OrderIDs)
			}
		case isString(rec["id"]):
			if m := decodeMessage(b, line); m != nil {
				msgs.set(m)
			}
		case isObject(rec["$set"]):
			var set map[string]json.RawMessage
			_ = json.Unmarshal(rec["$set"], &set)
			if raw, ok := set["messages"]; ok && isArray(raw) {
				msgs.clear()
				msgs.addMessages(raw, line)
			}
			var t time.Time
			if json.Unmarshal(set["lastUpdated"], &t) == nil && !t.IsZero() {
				out.Meta.LastUpdated = t
			}
			if raw, ok := set["summary"]; ok {
				var sum string
				if json.Unmarshal(raw, &sum) == nil {
					out.Meta.Summary = sum
				}
			}
		case isString(rec["sessionId"]) && isString(rec["projectHash"]):
			mergeHeader(&out.Header, b)
			var sum string
			if json.Unmarshal(rec["summary"], &sum) == nil && rec["summary"] != nil {
				out.Meta.Summary = sum
			}
			if raw, ok := rec["messages"]; ok && isArray(raw) {
				msgs.addMessages(raw, line)
			}
		default:
			out.Unknown.Note(firstKey(b), "line", line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if out.Header.SessionID == "" || out.Header.ProjectHash == "" {
		return nil, nil
	}

	out.Live = map[int]bool{}
	for _, id := range msgs.order {
		m := msgs.byID[id]
		out.Live[m.Line] = true
		for _, l := range m.patchLines {
			out.Live[l] = true
		}
		out.Messages = append(out.Messages, *m)
	}
	if out.Meta.LastUpdated.IsZero() {
		out.Meta.LastUpdated = out.Header.LastUpdated
	}
	return out, nil
}

// mergeHeader overlays the header fields a line sets onto h.
func mergeHeader(h *replayHeader, b []byte) {
	var next replayHeader
	if json.Unmarshal(b, &next) != nil {
		return
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(b, &raw)
	h.SessionID, h.ProjectHash = next.SessionID, next.ProjectHash
	if _, ok := raw["startTime"]; ok {
		h.StartTime = next.StartTime
	}
	if _, ok := raw["lastUpdated"]; ok {
		h.LastUpdated = next.LastUpdated
	}
	if _, ok := raw["kind"]; ok {
		h.Kind = next.Kind
	}
	if _, ok := raw["directories"]; ok {
		h.Directories = next.Directories
	}
}

func isString(raw json.RawMessage) bool { return len(raw) > 0 && raw[0] == '"' }
func isObject(raw json.RawMessage) bool { return len(raw) > 0 && raw[0] == '{' }
func isArray(raw json.RawMessage) bool  { return len(raw) > 0 && raw[0] == '[' }

// firstKey returns the first key of a JSON object, or "" when there is none.
func firstKey(b []byte) string {
	dec := json.NewDecoder(bufio.NewReader(bytes.NewReader(b)))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return ""
	}
	if tok, err := dec.Token(); err == nil {
		if k, ok := tok.(string); ok {
			return k
		}
	}
	return ""
}
