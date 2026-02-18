package claude

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/wethinkt/go-thinkt/internal/jsonl"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// LazySession provides lazy-loading access to a session file.
// It preloads metadata from the first few entries but defers loading
// the full content until requested.
type LazySession struct {
	mu sync.Mutex

	// Metadata (populated on open)
	Path      string
	ID        string
	Branch    string
	Model     string
	Version   string
	CWD       string
	StartTime time.Time
	FileSize  int64

	// Loaded entries
	entries []Entry

	// Reader state
	reader       *jsonl.Reader
	bytesRead    int64
	fullyLoaded  bool
	contentBytes int // estimated displayable content loaded
}

// OpenLazySession opens a session file and preloads metadata.
// It reads the first few entries to extract session metadata but
// does not load the full content. Call LoadMore() to load additional entries.
func OpenLazySession(path string) (*LazySession, error) {
	reader, err := jsonl.NewReader(path)
	if err != nil {
		return nil, err
	}

	ls := &LazySession{
		Path:     path,
		FileSize: reader.FileSize(),
		reader:   reader,
		entries:  make([]Entry, 0, 64),
	}

	// Preload first few entries to get metadata (read up to 8KB)
	if err := ls.loadUntilBytes(8 * 1024); err != nil && err != io.EOF {
		reader.Close()
		return nil, err
	}

	return ls, nil
}

// loadUntilBytes loads entries until we've read approximately maxBytes of content.
// Must be called with lock held.
func (ls *LazySession) loadUntilBytes(maxBytes int) error {
	for ls.contentBytes < maxBytes && !ls.fullyLoaded {
		entry, err := ls.readNextEntry()
		if err == io.EOF {
			ls.fullyLoaded = true
			return io.EOF
		}
		if err != nil {
			return err
		}
		if entry != nil {
			ls.entries = append(ls.entries, *entry)
			ls.contentBytes += estimateEntryContentSize(entry)
			ls.extractMetadata(entry)
		}
	}
	return nil
}

// readNextEntry reads and parses the next entry from the file.
func (ls *LazySession) readNextEntry() (*Entry, error) {
	var entry Entry
	err := ls.reader.ReadJSON(&entry)
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		// Skip malformed lines
		return nil, nil
	}

	ls.bytesRead = ls.reader.Position()

	// Parse the message based on entry type
	// Note: Message is now lazily parsed via GetUserMessage/GetAssistantMessage
	// We pre-parse here for metadata extraction, then set via setters
	if len(entry.Message) > 0 {
		switch entry.Type {
		case EntryTypeUser:
			var msg UserMessage
			if err := json.Unmarshal(entry.Message, &msg); err == nil {
				entry.SetUserMessage(&msg)
			}
		case EntryTypeAssistant:
			var msg AssistantMessage
			if err := json.Unmarshal(entry.Message, &msg); err == nil {
				entry.SetAssistantMessage(&msg)
			}
		}
	}

	return &entry, nil
}

// extractMetadata extracts session metadata from an entry.
func (ls *LazySession) extractMetadata(entry *Entry) {
	if ls.ID == "" && entry.SessionID != "" {
		ls.ID = entry.SessionID
	}
	if ls.Branch == "" && entry.GitBranch != "" {
		ls.Branch = entry.GitBranch
	}
	if ls.Version == "" && entry.Version != "" {
		ls.Version = entry.Version
	}
	if ls.CWD == "" && entry.CWD != "" {
		ls.CWD = entry.CWD
	}
	if ls.Model == "" {
		if msg := entry.GetAssistantMessage(); msg != nil && msg.Model != "" {
			ls.Model = msg.Model
		}
	}

	if entry.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
			if ls.StartTime.IsZero() || t.Before(ls.StartTime) {
				ls.StartTime = t
			}
		}
	}
}

// ClaudeEntries returns all currently loaded entries as claude.Entry.
func (ls *LazySession) ClaudeEntries() []Entry {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.entries
}

// Entries returns all currently loaded entries as thinkt.Entry.
// This implements the thinkt.LazySession interface.
func (ls *LazySession) Entries() []thinkt.Entry {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	result := make([]thinkt.Entry, len(ls.entries))
	for i, e := range ls.entries {
		result[i] = e.ToThinktEntry()
	}
	return result
}

// EntryCount returns the number of loaded entries.
func (ls *LazySession) EntryCount() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.entries)
}

// BytesRead returns total bytes read from the file.
func (ls *LazySession) BytesRead() int64 {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.bytesRead
}

// Progress returns the percentage of file read (0.0 to 1.0).
func (ls *LazySession) Progress() float64 {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.FileSize == 0 {
		return 1.0
	}
	return float64(ls.bytesRead) / float64(ls.FileSize)
}

// HasMore returns true if there's more content to load.
func (ls *LazySession) HasMore() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return !ls.fullyLoaded
}

// IsFullyLoaded returns true if the entire file has been read.
func (ls *LazySession) IsFullyLoaded() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.fullyLoaded
}

// LoadMore loads additional entries up to maxContentBytes of displayable content.
// Returns the number of new entries loaded and any error.
func (ls *LazySession) LoadMore(maxContentBytes int) (int, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.fullyLoaded {
		return 0, nil
	}

	startCount := len(ls.entries)
	targetBytes := ls.contentBytes + maxContentBytes

	for ls.contentBytes < targetBytes && !ls.fullyLoaded {
		entry, err := ls.readNextEntry()
		if err == io.EOF {
			ls.fullyLoaded = true
			break
		}
		if err != nil {
			return len(ls.entries) - startCount, err
		}
		if entry != nil {
			ls.entries = append(ls.entries, *entry)
			ls.contentBytes += estimateEntryContentSize(entry)
		}
	}

	return len(ls.entries) - startCount, nil
}

// LoadAll loads all remaining entries from the file.
// Use with caution on large files.
func (ls *LazySession) LoadAll() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	for !ls.fullyLoaded {
		entry, err := ls.readNextEntry()
		if err == io.EOF {
			ls.fullyLoaded = true
			break
		}
		if err != nil {
			return err
		}
		if entry != nil {
			ls.entries = append(ls.entries, *entry)
			ls.contentBytes += estimateEntryContentSize(entry)
		}
	}

	return nil
}

// ToSession converts the lazy session to a regular Session struct.
// This includes only the currently loaded entries.
func (ls *LazySession) ToSession() *Session {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Calculate end time from loaded entries
	var endTime time.Time
	for _, e := range ls.entries {
		if e.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
				if t.After(endTime) {
					endTime = t
				}
			}
		}
	}

	return &Session{
		ID:        ls.ID,
		Path:      ls.Path,
		Branch:    ls.Branch,
		Model:     ls.Model,
		Version:   ls.Version,
		CWD:       ls.CWD,
		StartTime: ls.StartTime,
		EndTime:   endTime,
		Entries:   ls.entries,
	}
}

// Close closes the underlying file reader.
func (ls *LazySession) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.reader != nil {
		err := ls.reader.Close()
		ls.reader = nil
		return err
	}
	return nil
}

// Window returns a SessionWindow representing the current state.
func (ls *LazySession) Window() *SessionWindow {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	return &SessionWindow{
		Session:    ls.ToSession(),
		BytesRead:  ls.bytesRead,
		HasMore:    !ls.fullyLoaded,
		TotalSize:  ls.FileSize,
		EntryCount: len(ls.entries),
	}
}

// Metadata returns session metadata as thinkt.SessionMeta.
// This implements the thinkt.LazySession interface.
func (ls *LazySession) Metadata() thinkt.SessionMeta {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	return thinkt.SessionMeta{
		ID:         ls.ID,
		FullPath:   ls.Path,
		GitBranch:  ls.Branch,
		Model:      ls.Model,
		Source:     thinkt.SourceClaude,
		CreatedAt:  ls.StartTime,
		ModifiedAt: ls.StartTime,
		EntryCount: len(ls.entries),
	}
}

// ToThinktEntries converts Claude entries to thinkt.Entry slice.
// This implements the thinkt.LazySession interface.
func (ls *LazySession) ToThinktEntries() []thinkt.Entry {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	result := make([]thinkt.Entry, len(ls.entries))
	for i, e := range ls.entries {
		result[i] = e.ToThinktEntry()
	}
	return result
}

// ensure LazySession implements thinkt.LazySession
var _ thinkt.LazySession = (*LazySession)(nil)
