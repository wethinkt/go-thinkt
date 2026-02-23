package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// StreamLocal opens a JSONL session file, seeks to the end, and streams
// new entries as they are appended. The returned channel is closed when
// ctx is cancelled or the file is deleted.
func StreamLocal(ctx context.Context, sessionPath string) (<-chan StreamEntry, error) {
	f, err := os.Open(sessionPath)
	if err != nil {
		return nil, err
	}

	// Seek to end — we only want new entries
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

	ch := make(chan StreamEntry, 64)
	go streamLocalLoop(ctx, f, watcher, ch)
	return ch, nil
}

func streamLocalLoop(ctx context.Context, f *os.File, watcher *fsnotify.Watcher, ch chan<- StreamEntry) {
	defer close(ch)
	defer f.Close()
	defer watcher.Close()

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
		json.Unmarshal(t, &entryType)
	}

	switch entryType {
	case "human":
		entry.Role = "user"
		entry.Text = extractText(raw)
	case "assistant":
		entry.Role = "assistant"
		entry.Text = extractText(raw)
		entry.Model = extractString(raw, "model")
	default:
		// Try generic role field
		var role string
		if r, ok := raw["role"]; ok {
			json.Unmarshal(r, &role)
		}
		if role == "" {
			return StreamEntry{}, false
		}
		entry.Role = role
		entry.Text = extractText(raw)
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

func extractText(raw map[string]json.RawMessage) string {
	// Try message.content[].text pattern (Claude format)
	if msg, ok := raw["message"]; ok {
		var message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
				Name string `json:"name"`
			} `json:"content"`
			Model string `json:"model"`
		}
		if json.Unmarshal(msg, &message) == nil {
			for _, c := range message.Content {
				if c.Type == "text" && c.Text != "" {
					return c.Text
				}
				if c.Type == "tool_use" && c.Name != "" {
					return "[tool_use: " + c.Name + "]"
				}
			}
		}
	}

	// Try direct text field
	if t, ok := raw["text"]; ok {
		var text string
		json.Unmarshal(t, &text)
		return text
	}

	return ""
}

func extractString(raw map[string]json.RawMessage, key string) string {
	if v, ok := raw[key]; ok {
		var s string
		json.Unmarshal(v, &s)
		return s
	}
	// Try nested in message
	if msg, ok := raw["message"]; ok {
		var m map[string]json.RawMessage
		if json.Unmarshal(msg, &m) == nil {
			if v, ok := m[key]; ok {
				var s string
				json.Unmarshal(v, &s)
				return s
			}
		}
	}
	return ""
}
