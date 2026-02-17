package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/wethinkt/go-thinkt/internal/jsonl"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Parser reads Claude Code JSONL trace files.
type Parser struct {
	scanner *bufio.Scanner
	lineNum int
	errors  []error
}

// NewParser creates a parser from an io.Reader.
func NewParser(r io.Reader) *Parser {
	scanner := thinkt.NewScannerWithMaxCapacityCustom(r, 128*1024, thinkt.MaxLineCapacity)

	return &Parser{
		scanner: scanner,
		lineNum: 0,
	}
}

// NewParserFromFile creates a parser from a file path.
func NewParserFromFile(path string) (*Parser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	return NewParser(f), nil
}

// NextEntry reads the next entry from the trace.
// Returns nil, nil when EOF is reached.
func (p *Parser) NextEntry() (*Entry, error) {
	for p.scanner.Scan() {
		p.lineNum++
		line := p.scanner.Bytes()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			p.errors = append(p.errors, fmt.Errorf("line %d: %w", p.lineNum, err))
			continue
		}

		// Parse the message based on entry type
		if err := p.parseMessage(&entry); err != nil {
			p.errors = append(p.errors, fmt.Errorf("line %d: %w", p.lineNum, err))
		}

		return &entry, nil
	}

	if err := p.scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return nil, nil // EOF
}

// parseMessage parses the message field based on entry type.
func (p *Parser) parseMessage(entry *Entry) error {
	if len(entry.Message) == 0 {
		return nil
	}

	switch entry.Type {
	case EntryTypeUser:
		var msg UserMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			return fmt.Errorf("parse user message: %w", err)
		}
		entry.SetUserMessage(&msg)

	case EntryTypeAssistant:
		var msg AssistantMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			return fmt.Errorf("parse assistant message: %w", err)
		}
		entry.SetAssistantMessage(&msg)
	}

	return nil
}

// ReadAllEntries reads all entries from the trace.
func (p *Parser) ReadAllEntries() ([]Entry, error) {
	var entries []Entry
	for {
		entry, err := p.NextEntry()
		if err != nil {
			return entries, err
		}
		if entry == nil {
			break
		}
		entries = append(entries, *entry)
	}
	return entries, nil
}

// ReadSession reads all entries and constructs a Session.
func (p *Parser) ReadSession(path string) (*Session, error) {
	entries, err := p.ReadAllEntries()
	if err != nil {
		return nil, err
	}

	session := &Session{
		Path:    path,
		Entries: entries,
	}

	// Extract metadata from entries
	for _, e := range entries {
		if session.ID == "" && e.SessionID != "" {
			session.ID = e.SessionID
		}
		if session.Branch == "" && e.GitBranch != "" {
			session.Branch = e.GitBranch
		}
		if session.Version == "" && e.Version != "" {
			session.Version = e.Version
		}
		if session.CWD == "" && e.CWD != "" {
			session.CWD = e.CWD
		}
		if session.Model == "" {
			if msg := e.GetAssistantMessage(); msg != nil && msg.Model != "" {
				session.Model = msg.Model
			}
		}

		// Parse timestamps
		if e.Timestamp != "" {
			t, err := time.Parse(time.RFC3339, e.Timestamp)
			if err == nil {
				if session.StartTime.IsZero() || t.Before(session.StartTime) {
					session.StartTime = t
				}
				if t.After(session.EndTime) {
					session.EndTime = t
				}
			}
		}
	}

	return session, nil
}

// NextPrompt reads entries until it finds a user prompt.
// Returns nil, nil when EOF is reached.
func (p *Parser) NextPrompt() (*Prompt, error) {
	for {
		entry, err := p.NextEntry()
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, nil // EOF
		}

		text := entry.GetPromptText()
		if text != "" {
			return &Prompt{
				Text:      text,
				Timestamp: entry.Timestamp,
				UUID:      entry.UUID,
			}, nil
		}
	}
}

// ReadAllPrompts reads all user prompts from the trace.
func (p *Parser) ReadAllPrompts() ([]Prompt, error) {
	var prompts []Prompt
	for {
		prompt, err := p.NextPrompt()
		if err != nil {
			return prompts, err
		}
		if prompt == nil {
			break
		}
		prompts = append(prompts, *prompt)
	}
	return prompts, nil
}

// Errors returns all parse errors encountered.
func (p *Parser) Errors() []error {
	return p.errors
}

// LineNum returns the current line number.
func (p *Parser) LineNum() int {
	return p.lineNum
}

// LoadSession loads a session from a trace file path.
func LoadSession(path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	parser := NewParser(f)
	return parser.ReadSession(path)
}

// LoadSessionPreview loads a limited number of entries from a session.
// This is useful for large files where only a preview is needed.
// maxEntries of 0 means no limit.
func LoadSessionPreview(path string, maxEntries int) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	parser := NewParser(f)
	return parser.ReadSessionPreview(path, maxEntries)
}

// SessionWindow holds a window of session content with position info.
type SessionWindow struct {
	Session    *Session
	BytesRead  int64 // Total bytes read from file
	HasMore    bool  // True if there's more content in the file
	TotalSize  int64 // Total file size
	EntryCount int   // Number of entries loaded
}

// LoadSessionWindow loads entries from a session file until content limit is reached.
// maxContentBytes limits total raw content size (text, thinking, etc.) as a proxy for screen lines.
// startOffset allows resuming from a previous position (0 for start).
// This is much more efficient than loading by entry count since entries vary wildly in size.
func LoadSessionWindow(path string, startOffset int64, maxContentBytes int) (*SessionWindow, error) {
	reader, err := jsonl.NewReader(path)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer reader.Close()

	totalSize := reader.FileSize()

	// Seek to start offset if specified
	if startOffset > 0 {
		if err := reader.SeekTo(startOffset); err != nil {
			return nil, fmt.Errorf("seek to offset: %w", err)
		}
	}

	var entries []Entry
	var contentBytes int

	for {
		var entry Entry
		err := reader.ReadJSON(&entry)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines, continue reading
			continue
		}

		// Parse the message based on entry type
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

		// Estimate content size for this entry
		entryContentSize := estimateEntryContentSize(&entry)
		contentBytes += entryContentSize
		entries = append(entries, entry)

		// Stop if we've accumulated enough content
		if maxContentBytes > 0 && contentBytes >= maxContentBytes {
			break
		}
	}

	session := &Session{
		Path:    path,
		Entries: entries,
	}

	// Extract metadata from entries
	for _, e := range entries {
		if session.ID == "" && e.SessionID != "" {
			session.ID = e.SessionID
		}
		if session.Branch == "" && e.GitBranch != "" {
			session.Branch = e.GitBranch
		}
		if session.Version == "" && e.Version != "" {
			session.Version = e.Version
		}
		if session.CWD == "" && e.CWD != "" {
			session.CWD = e.CWD
		}
		if session.Model == "" {
			if msg := e.GetAssistantMessage(); msg != nil && msg.Model != "" {
				session.Model = msg.Model
			}
		}

		if e.Timestamp != "" {
			t, err := time.Parse(time.RFC3339, e.Timestamp)
			if err == nil {
				if session.StartTime.IsZero() || t.Before(session.StartTime) {
					session.StartTime = t
				}
				if t.After(session.EndTime) {
					session.EndTime = t
				}
			}
		}
	}

	return &SessionWindow{
		Session:    session,
		BytesRead:  reader.Position(),
		HasMore:    reader.HasMore(),
		TotalSize:  totalSize,
		EntryCount: len(entries),
	}, nil
}

// estimateEntryContentSize returns an estimate of displayable content size in bytes.
func estimateEntryContentSize(entry *Entry) int {
	size := 0

	// User message content
	if msg := entry.GetUserMessage(); msg != nil {
		size += len(msg.Content.Text)
		for _, block := range msg.Content.Blocks {
			size += len(block.Text)
		}
	}

	// Assistant message content
	if msg := entry.GetAssistantMessage(); msg != nil {
		for _, block := range msg.Content {
			size += len(block.Text)
			size += len(block.Thinking)
			// Tool calls are usually short, just count the name
			size += len(block.Name) * 10
		}
	}

	return size
}

// ReadSessionPreview reads up to maxEntries and constructs a Session.
// If maxEntries is 0, reads all entries.
func (p *Parser) ReadSessionPreview(path string, maxEntries int) (*Session, error) {
	var entries []Entry
	count := 0
	for {
		if maxEntries > 0 && count >= maxEntries {
			break
		}
		entry, err := p.NextEntry()
		if err != nil {
			return nil, err
		}
		if entry == nil {
			break
		}
		entries = append(entries, *entry)
		count++
	}

	session := &Session{
		Path:    path,
		Entries: entries,
	}

	// Extract metadata from entries
	for _, e := range entries {
		if session.ID == "" && e.SessionID != "" {
			session.ID = e.SessionID
		}
		if session.Branch == "" && e.GitBranch != "" {
			session.Branch = e.GitBranch
		}
		if session.Version == "" && e.Version != "" {
			session.Version = e.Version
		}
		if session.CWD == "" && e.CWD != "" {
			session.CWD = e.CWD
		}
		if session.Model == "" {
			if msg := e.GetAssistantMessage(); msg != nil && msg.Model != "" {
				session.Model = msg.Model
			}
		}

		// Parse timestamps
		if e.Timestamp != "" {
			t, err := time.Parse(time.RFC3339, e.Timestamp)
			if err == nil {
				if session.StartTime.IsZero() || t.Before(session.StartTime) {
					session.StartTime = t
				}
				if t.After(session.EndTime) {
					session.EndTime = t
				}
			}
		}
	}

	return session, nil
}
