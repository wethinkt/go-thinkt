package claude

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"
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
	state     sessionScan // session-state records seen so far (title, last prompt, ...)

	// Loaded entries
	entries []Entry

	// Reader state
	reader       *jsonl.Reader
	bytesRead    int64
	fullyLoaded  bool
	contentBytes int // estimated displayable content loaded

	// Background line count for progress estimation
	totalEntries atomic.Int64 // 0 = unknown/scanning, >0 = known

	unknown unknownTally // record types the classifier has never seen

	legacySummary string // pre-2.1 summary record; title fallback
}

// title is the ai-title, or the legacy summary when no title was written.
func (ls *LazySession) title() string {
	if ls.state.Title != "" {
		return ls.state.Title
	}
	return ls.legacySummary
}

// UnknownTypes returns how many records of each never-seen type were read
// so far, or nil when every record had a known type.
func (ls *LazySession) UnknownTypes() map[EntryType]int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.unknown.snapshot()
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

	// Start a background count of emitted entries for progress estimation.
	// Raw lines would overstate the total, since bookkeeping lines are dropped.
	if !ls.fullyLoaded {
		go func() {
			if n := countConversationRecords(path); n > 0 {
				ls.totalEntries.Store(int64(n))
			}
		}()
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
	line, err := ls.reader.ReadLine()
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(line) == 0 && err == io.EOF {
		return nil, io.EOF
	}
	ls.bytesRead = ls.reader.Position()
	if len(line) == 0 {
		return nil, nil
	}

	// Session-state records never become entries but still carry metadata.
	entry, class, decodeErr := decodeRecord(line)
	if class == ClassSessionState {
		ls.onSessionState(line)
	}
	if decodeErr != nil || entry == nil {
		// Malformed, bookkeeping, or session-state line: nothing to append.
		return nil, nil
	}
	if class == ClassUnknown {
		ls.unknown.note(entry.Type, "path", ls.Path, "offset", ls.bytesRead)
	}
	if entry.Type == EntryTypeSummary && strings.TrimSpace(entry.Summary) != "" {
		ls.legacySummary = entry.Summary
	}
	return entry, nil
}

// onSessionState folds a session-state line (title, last prompt, PR links,
// worktree, cost) into the metadata the readers skip as entries.
func (ls *LazySession) onSessionState(line []byte) {
	typ, _, _ := ClassifyLine(line)
	ls.state.applyState(typ, line)
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
	if !thinkt.IsRealModel(ls.Model) {
		if msg := entry.GetAssistantMessage(); msg != nil && thinkt.IsRealModel(msg.Model) {
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
// Uses background line count when available, falls back to byte-based progress.
func (ls *LazySession) Progress() float64 {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.fullyLoaded {
		return 1.0
	}
	if total := ls.totalEntries.Load(); total > 0 {
		return float64(len(ls.entries)) / float64(total)
	}
	// Fallback to byte-based progress
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

	if ls.fullyLoaded || ls.reader == nil {
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
		Title:      ls.title(),
		Summary:    ls.title(),
		LastPrompt: ls.state.LastPrompt,
		PRLinks:    ls.state.PRLinks,
		Worktree:   ls.state.Worktree,
		Cost:       ls.state.Cost,
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
