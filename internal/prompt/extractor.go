// Package prompt provides extraction and formatting of user prompts from traces.
package prompt

import (
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// Prompt is an alias for claude.Prompt for API convenience.
type Prompt = claude.Prompt

// Extractor extracts user prompts from a Claude parser.
type Extractor struct {
	parser *claude.Parser
}

// NewExtractor creates a new prompt extractor.
func NewExtractor(parser *claude.Parser) *Extractor {
	return &Extractor{parser: parser}
}

// Extract reads all user prompts from the trace.
func (e *Extractor) Extract() ([]Prompt, error) {
	return e.parser.ReadAllPrompts()
}

// Errors returns any parse errors from the underlying parser.
func (e *Extractor) Errors() []error {
	return e.parser.Errors()
}
