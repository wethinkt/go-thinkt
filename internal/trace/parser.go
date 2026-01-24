package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

		return &entry, nil
	}

	if err := p.scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return nil, nil // EOF
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

		// Only process user entries
		if entry.Type != "user" {
			continue
		}

		// Parse the message
		var msg UserMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			p.errors = append(p.errors, fmt.Errorf("line %d: parse user message: %w", p.lineNum, err))
			continue
		}

		// Skip tool results (content is array with tool_result type)
		text := ParseUserContent(msg.Content)
		if text == "" {
			continue
		}

		// Skip if this looks like a tool result (heuristic: starts with tool use ID reference)
		// Tool results have content blocks with type "tool_result"
		var blocks []ContentBlock
		if json.Unmarshal(msg.Content, &blocks) == nil && len(blocks) > 0 {
			isToolResult := true
			for _, b := range blocks {
				if b.Type != "tool_result" {
					isToolResult = false
					break
				}
			}
			if isToolResult {
				continue
			}
		}

		return &Prompt{
			Text:      text,
			Timestamp: entry.Timestamp,
			UUID:      entry.UUID,
		}, nil
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
