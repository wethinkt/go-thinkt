// Package prompt provides extraction and formatting of user prompts from traces.
package prompt

import (
	"github.com/Brain-STM-org/thinking-tracer-tools/internal/trace"
)

// Prompt is an alias for trace.Prompt for API convenience.
type Prompt = trace.Prompt

// Extractor extracts user prompts from a trace parser.
type Extractor struct {
	parser *trace.Parser
}

// NewExtractor creates a new prompt extractor.
func NewExtractor(parser *trace.Parser) *Extractor {
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
