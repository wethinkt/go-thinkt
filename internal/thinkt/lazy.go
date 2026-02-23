package thinkt

import (
	"io"
	"sync"
)

// GenericLazySession wraps any SessionReader to provide lazy loading capabilities.
// It loads entries incrementally and tracks loading progress.
type GenericLazySession struct {
	mu sync.Mutex

	// Metadata
	Meta SessionMeta

	// Loaded entries
	entries []Entry

	// Reader state
	reader      SessionReader
	fullyLoaded bool
}

// NewLazySession creates a new lazy session wrapper around a SessionReader.
// It preloads the first few entries to populate metadata.
func NewLazySession(reader SessionReader) (*GenericLazySession, error) {
	ls := &GenericLazySession{
		Meta:    reader.Metadata(),
		reader:  reader,
		entries: make([]Entry, 0, 64),
	}

	// Preload first few entries (up to 8KB of content)
	if err := ls.loadUntilBytes(8 * 1024); err != nil && err != io.EOF {
		reader.Close()
		return nil, err
	}

	return ls, nil
}

// loadUntilBytes loads entries until we've read approximately maxBytes of content.
func (ls *GenericLazySession) loadUntilBytes(maxBytes int) error {
	contentBytes := 0
	for contentBytes < maxBytes && !ls.fullyLoaded {
		entry, err := ls.reader.ReadNext()
		if err == io.EOF {
			ls.fullyLoaded = true
			return io.EOF
		}
		if err != nil {
			return err
		}
		if entry != nil {
			ls.entries = append(ls.entries, *entry)
			contentBytes += estimateEntrySize(entry)
		}
	}
	return nil
}

// estimateEntrySize estimates the displayable content size of an entry.
func estimateEntrySize(e *Entry) int {
	size := len(e.Text)
	for _, cb := range e.ContentBlocks {
		size += len(cb.Text)
		size += len(cb.Thinking)
		size += len(cb.ToolResult)
	}
	return size
}

// Entries returns all currently loaded entries.
func (ls *GenericLazySession) Entries() []Entry {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.entries
}

// EntryCount returns the number of loaded entries.
func (ls *GenericLazySession) EntryCount() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.entries)
}

// HasMore returns true if there's more content to load.
func (ls *GenericLazySession) HasMore() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return !ls.fullyLoaded
}

// IsFullyLoaded returns true if the entire session has been read.
func (ls *GenericLazySession) IsFullyLoaded() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.fullyLoaded
}

// LoadMore loads additional entries up to maxContentBytes of displayable content.
// Returns the number of new entries loaded and any error.
func (ls *GenericLazySession) LoadMore(maxContentBytes int) (int, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.fullyLoaded {
		return 0, nil
	}

	startCount := len(ls.entries)
	contentBytes := 0

	if ls.reader == nil {
		return 0, nil
	}

	for contentBytes < maxContentBytes && !ls.fullyLoaded {
		entry, err := ls.reader.ReadNext()
		if err == io.EOF {
			ls.fullyLoaded = true
			break
		}
		if err != nil {
			return len(ls.entries) - startCount, err
		}
		if entry != nil {
			ls.entries = append(ls.entries, *entry)
			contentBytes += estimateEntrySize(entry)
		}
	}

	return len(ls.entries) - startCount, nil
}

// LoadAll loads all remaining entries from the session.
// Use with caution on large sessions.
func (ls *GenericLazySession) LoadAll() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	for !ls.fullyLoaded {
		entry, err := ls.reader.ReadNext()
		if err == io.EOF {
			ls.fullyLoaded = true
			break
		}
		if err != nil {
			return err
		}
		if entry != nil {
			ls.entries = append(ls.entries, *entry)
		}
	}

	return nil
}

// Progress returns the percentage of content loaded (0.0 to 1.0).
// For files, this is based on entry count vs the metadata entry count.
// Returns 1.0 if fully loaded.
func (ls *GenericLazySession) Progress() float64 {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.fullyLoaded {
		return 1.0
	}

	if ls.Meta.EntryCount > 0 {
		return float64(len(ls.entries)) / float64(ls.Meta.EntryCount)
	}

	return 0.0
}

// Metadata returns the session metadata.
func (ls *GenericLazySession) Metadata() SessionMeta {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.Meta
}

// Close closes the underlying reader.
func (ls *GenericLazySession) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.reader != nil {
		err := ls.reader.Close()
		ls.reader = nil
		return err
	}
	return nil
}

// ReadNext implements SessionReader but is not supported for lazy sessions.
// Use LoadMore or LoadAll to read entries incrementally.
func (ls *GenericLazySession) ReadNext() (*Entry, error) {
	return nil, io.ErrClosedPipe
}

// ensure GenericLazySession implements LazySession
var _ LazySession = (*GenericLazySession)(nil)
