package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Parser reads Claude Code JSONL trace files.
type Parser struct {
	scanner *bufio.Scanner
	lineNum int
	errors  []error
}

// NewParser creates a parser from an io.Reader.
func NewParser(r io.Reader) *Parser {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for long lines (some tool results can be large)
	const maxCapacity = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxCapacity)

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
		entry.UserMessage = &msg

	case EntryTypeAssistant:
		var msg AssistantMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			return fmt.Errorf("parse assistant message: %w", err)
		}
		entry.AssistantMessage = &msg
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
		if session.Model == "" && e.AssistantMessage != nil && e.AssistantMessage.Model != "" {
			session.Model = e.AssistantMessage.Model
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
