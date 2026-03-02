package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// StreamLocal opens a JSONL session file and streams entries. If backlog > 0,
// the last N entries from the file are sent first before tailing new entries.
// The returned channel is closed when ctx is cancelled or the file is deleted.
func StreamLocal(ctx context.Context, sessionPath string, backlog int) (<-chan StreamEntry, error) {
	f, err := os.Open(sessionPath)
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEntry, 64)

	// Read backlog entries from the file before seeking to end
	var backlogEntries []StreamEntry
	if backlog > 0 {
		backlogEntries = readBacklog(f, backlog)
	}

	// Seek to end for tailing new entries
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		f.Close()
		return nil, err
	}
	if err := watcher.Add(sessionPath); err != nil {
		watcher.Close()
		f.Close()
		return nil, err
	}

	go streamLocalLoop(ctx, f, watcher, ch, backlogEntries)
	return ch, nil
}

// readBacklog reads the entire file and returns the last n parsed entries.
func readBacklog(f *os.File, n int) []StreamEntry {
	reader := bufio.NewReader(f)
	var entries []StreamEntry
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if entry, ok := parseJSONLLine(line); ok {
				entries = append(entries, entry)
			}
		}
		if err != nil {
			break
		}
	}
	if len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries
}

func streamLocalLoop(ctx context.Context, f *os.File, watcher *fsnotify.Watcher, ch chan<- StreamEntry, backlog []StreamEntry) {
	defer close(ch)
	defer f.Close()
	defer watcher.Close()

	// Send backlog entries first
	for _, entry := range backlog {
		select {
		case ch <- entry:
		case <-ctx.Done():
			return
		}
	}

	reader := bufio.NewReader(f)
	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				// Debounce rapid writes
				debounce.Reset(100 * time.Millisecond)
			}
			if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				ch <- StreamEntry{
					Timestamp: time.Now(),
					Role:      "system",
					Text:      "Session file removed — stream ending.",
					Synthetic: true,
				}
				return
			}

		case <-debounce.C:
			// Read all available new lines
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					break
				}
				if entry, ok := parseJSONLLine(line); ok {
					select {
					case ch <- entry:
					case <-ctx.Done():
						return
					}
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			tuilog.Log.Warn("Local stream watcher error", "error", err)
		}
	}
}

// parseJSONLLine attempts to parse a JSONL line from a session file into a StreamEntry.
// Session files vary by source (Claude, Kimi, etc.) but share common patterns.
func parseJSONLLine(line []byte) (StreamEntry, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return StreamEntry{}, false
	}

	entry := StreamEntry{Timestamp: time.Now()}

	// Try to extract "type" field (Claude format)
	var entryType string
	if t, ok := raw["type"]; ok {
		_ = json.Unmarshal(t, &entryType)
	}

	switch entryType {
	case "human", "user":
		entry.Role = "user"
		entry.ContentBlocks = extractContentBlocks(raw)
		if len(entry.ContentBlocks) == 0 {
			entry.Text = extractText(raw)
		}
	case "assistant":
		entry.Role = "assistant"
		entry.ContentBlocks = extractContentBlocks(raw)
		if len(entry.ContentBlocks) == 0 {
			entry.Text = extractText(raw)
		}
		entry.Model = extractString(raw, "model")
	default:
		// Try generic role field
		var role string
		if r, ok := raw["role"]; ok {
			_ = json.Unmarshal(r, &role)
		}
		if role == "" {
			return StreamEntry{}, false
		}
		entry.Role = role
		entry.ContentBlocks = extractContentBlocks(raw)
		if len(entry.ContentBlocks) == 0 {
			entry.Text = extractText(raw)
		}
	}

	// Extract timestamp if present
	if ts, ok := raw["timestamp"]; ok {
		var t time.Time
		if json.Unmarshal(ts, &t) == nil {
			entry.Timestamp = t
		}
	}

	return entry, entry.Role != ""
}

// extractContentBlocks parses message.content into thinkt.ContentBlock slice.
// Content may be a plain string or an array of typed content blocks.
func extractContentBlocks(raw map[string]json.RawMessage) []thinkt.ContentBlock {
	msg, ok := raw["message"]
	if !ok {
		return nil
	}

	// Extract raw content field
	var msgRaw struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(msg, &msgRaw) != nil || len(msgRaw.Content) == 0 {
		return nil
	}

	// Try as plain string first (e.g. user entries: "content": "hello")
	var s string
	if json.Unmarshal(msgRaw.Content, &s) == nil && s != "" {
		return []thinkt.ContentBlock{{Type: "text", Text: s}}
	}

	// Try as array of content blocks
	var contentArr []struct {
		Type     string          `json:"type"`
		Text     string          `json:"text"`
		Thinking string          `json:"thinking"`
		Name     string          `json:"name"`
		ID       string          `json:"id"`
		Input    json.RawMessage `json:"input"`
		Content  json.RawMessage `json:"content"` // tool_result content
		IsError  bool            `json:"is_error"`
	}
	if json.Unmarshal(msgRaw.Content, &contentArr) != nil || len(contentArr) == 0 {
		return nil
	}

	var blocks []thinkt.ContentBlock
	for _, c := range contentArr {
		switch c.Type {
		case "text":
			if c.Text != "" {
				blocks = append(blocks, thinkt.ContentBlock{Type: "text", Text: c.Text})
			}
		case "thinking":
			if c.Thinking != "" {
				blocks = append(blocks, thinkt.ContentBlock{Type: "thinking", Thinking: c.Thinking})
			}
		case "tool_use":
			blocks = append(blocks, thinkt.ContentBlock{
				Type:      "tool_use",
				ToolName:  c.Name,
				ToolUseID: c.ID,
			})
		case "tool_result":
			text := "(result)"
			if c.Content != nil {
				// Try to extract text from tool result content
				var s string
				if json.Unmarshal(c.Content, &s) == nil {
					text = s
				}
			}
			blocks = append(blocks, thinkt.ContentBlock{
				Type:       "tool_result",
				ToolResult: text,
				IsError:    c.IsError,
			})
		}
	}
	return blocks
}

func extractText(raw map[string]json.RawMessage) string {
	if msg, ok := raw["message"]; ok {
		// First extract the raw content field
		var msgRaw struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(msg, &msgRaw) == nil && len(msgRaw.Content) > 0 {
			// Try as plain string (e.g. user entries: "content": "hello")
			var s string
			if json.Unmarshal(msgRaw.Content, &s) == nil && s != "" {
				return s
			}

			// Try as array of content blocks
			var blocks []struct {
				Type string `json:"type"`
				Text string `json:"text"`
				Name string `json:"name"`
			}
			if json.Unmarshal(msgRaw.Content, &blocks) == nil {
				for _, c := range blocks {
					if c.Type == "text" && c.Text != "" {
						return c.Text
					}
					if c.Type == "tool_use" && c.Name != "" {
						return "[tool_use: " + c.Name + "]"
					}
				}
			}
		}
	}

	// Try direct text field
	if t, ok := raw["text"]; ok {
		var text string
		_ = json.Unmarshal(t, &text)
		return text
	}

	return ""
}

func extractString(raw map[string]json.RawMessage, key string) string {
	if v, ok := raw[key]; ok {
		var s string
		_ = json.Unmarshal(v, &s)
		return s
	}
	// Try nested in message
	if msg, ok := raw["message"]; ok {
		var m map[string]json.RawMessage
		if json.Unmarshal(msg, &m) == nil {
			if v, ok := m[key]; ok {
				var s string
				_ = json.Unmarshal(v, &s)
				return s
			}
		}
	}
	return ""
}
