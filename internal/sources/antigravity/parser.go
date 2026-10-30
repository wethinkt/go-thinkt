package antigravity

import (
	"bufio"
	"bytes"
	"io"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Parser streams entries from one Antigravity transcript.
//
// Tool calls carry no ids, so each result pairs with the oldest unanswered
// call of the latest model turn; a new model turn drops the previous one's
// unanswered calls. The latest model-turn entry is held until its calls are
// answered or the next model turn, user input or EOF arrives, so a later
// invoke_subagent result can set ChildSessionID on its tool_use block. The
// entries that arrive meanwhile are held behind it, keeping file order.
type Parser struct {
	scanner *bufio.Scanner
	conv    converter
	unknown unknownTally
	related *relatedTracker // nil unless TrackRelated was called

	held    *thinkt.Entry  // model turn waiting for its calls
	behind  []thinkt.Entry // entries after the held turn
	pending []toolCall     // unanswered calls of the latest model turn
	out     []thinkt.Entry
	done    bool
}

// NewParser returns a parser over one transcript. parentID is the parent
// conversation of a subagent, "" otherwise.
func NewParser(r io.Reader, conversationID, parentID string) *Parser {
	return &Parser{
		scanner: thinkt.NewScannerWithMaxCapacityCustom(r, 64*1024, thinkt.MaxScannerCapacity),
		conv:    converter{conversationID: conversationID, parentID: parentID},
	}
}

// TrackRelated makes the parser gather the folders outside projectPath
// that the conversation's tool calls touched; paths under any exclude prefix
// are not recorded. Call it before the first NextEntry.
func (p *Parser) TrackRelated(projectPath string, exclude []string) {
	p.related = newRelatedTracker(projectPath, exclude)
}

// RelatedPaths returns the folders gathered so far, or nil.
func (p *Parser) RelatedPaths() []thinkt.RelatedPath {
	return p.related.paths()
}

// UnknownTypes returns the step types the parser did not recognise and how
// often each appeared.
func (p *Parser) UnknownTypes() map[string]int {
	return p.unknown.snapshot()
}

// NextEntry returns the next entry, or io.EOF at the end of the stream.
func (p *Parser) NextEntry() (*thinkt.Entry, error) {
	for {
		if len(p.out) > 0 {
			e := p.out[0]
			p.out = p.out[1:]
			return &e, nil
		}
		if p.done {
			return nil, io.EOF
		}
		if !p.scanner.Scan() {
			p.done = true
			if err := p.scanner.Err(); err != nil {
				return nil, err
			}
			p.release()
			continue
		}
		line := bytes.TrimSpace(p.scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		p.process(line)
	}
}

// emit queues an entry behind the held turn, or straight to the output.
func (p *Parser) emit(e thinkt.Entry) {
	if p.held != nil {
		p.behind = append(p.behind, e)
		return
	}
	p.out = append(p.out, e)
}

// release sends the held turn and everything behind it to the output.
func (p *Parser) release() {
	if p.held != nil {
		p.out = append(p.out, *p.held)
		p.held = nil
	}
	p.out = append(p.out, p.behind...)
	p.behind = nil
}

func (p *Parser) process(line []byte) {
	h, class := ClassifyLine(line)
	switch class {
	case ClassInvalid:
		return // a partial last line while the file is written
	case ClassModelTurn:
		p.release()
		p.pending = nil
		e, calls, ok := p.conv.modelTurn(h)
		if !ok {
			return
		}
		p.pending = calls
		for _, c := range calls {
			p.related.call(c.Name, c.Args)
		}
		if len(calls) == 0 {
			p.out = append(p.out, e)
			return
		}
		p.held = &e
	case ClassToolResult:
		p.result(h)
	case ClassUserInput:
		p.release()
		p.pending = nil
		if e, ok := p.conv.convert(h, class); ok {
			p.out = append(p.out, e)
		}
	default:
		if class == ClassUnknown {
			p.unknown.note(h.Type, "step", h.Index)
		}
		if e, ok := p.conv.convert(h, class); ok {
			p.emit(e)
		}
	}
}

// result pairs a result step with the oldest unanswered call.
func (p *Parser) result(h stepHead) {
	var call toolCall
	if len(p.pending) > 0 {
		call, p.pending = p.pending[0], p.pending[1:]
	}
	e := p.conv.resultEntry(h, call.ID)
	if ids, _ := e.Metadata["child_session_ids"].([]string); len(ids) > 0 && p.held != nil {
		for i := range p.held.ContentBlocks {
			if b := &p.held.ContentBlocks[i]; b.Type == "tool_use" && b.ToolUseID == call.ID {
				b.ChildSessionID = ids[0]
			}
		}
	}
	p.emit(e)
	if len(p.pending) == 0 {
		p.release()
	}
}
