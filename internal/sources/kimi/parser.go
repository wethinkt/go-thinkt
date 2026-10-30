package kimi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// approval is a permission.record_approval_result, kept by tool call id
// until the call's result is queued.
type approval struct {
	decision, scope, selectedLabel, feedback string
}

// openStep is the loop step under construction: the assistant entry's
// blocks plus every entry produced while the step was open, queued after
// it in arrival order.
type openStep struct {
	uuid, turnID string
	time         int64
	blocks       []thinkt.ContentBlock
	buffered     []thinkt.Entry
	closed       bool
	finish       string
	usage        *thinkt.TokenUsage
	messageID    string
	errMsg       string
}

// Parser reads a Kimi Code wire.jsonl and yields thinkt entries in file
// order. Loop steps are assembled into one assistant entry each; tool
// results, approvals, spawns, and the batched agent.message.appended twins
// follow the rules in the design's section C.
type Parser struct {
	scanner   *bufio.Scanner
	sessionID string
	lineNo    int
	conv      converter
	turn      turnState

	step      *openStep
	approvals map[string]approval
	spawns    map[string]string // parent tool call id → child session id

	seenCalls    map[string]bool // tool call ids the loop emitted
	seenResults  map[string]bool // tool call ids whose result the loop emitted
	seenMessages map[string]bool
	seenUserIDs  map[string]bool
	turnTexts    map[string]bool

	out     []thinkt.Entry
	unknown thinkt.UnknownTally[string]
	done    bool
}

// NewParser creates a parser for one wire file. sessionID names the session
// the file belongs to (the session directory name, or "<session>/<agent>").
func NewParser(r io.Reader, sessionID string) *Parser {
	p := &Parser{
		unknown:      thinkt.UnknownTally[string]{Source: thinkt.SourceKimi},
		scanner:      thinkt.NewScannerWithMaxCapacityCustom(r, 64*1024, thinkt.MaxScannerCapacity),
		sessionID:    sessionID,
		approvals:    map[string]approval{},
		spawns:       map[string]string{},
		seenCalls:    map[string]bool{},
		seenResults:  map[string]bool{},
		seenMessages: map[string]bool{},
		seenUserIDs:  map[string]bool{},
		turnTexts:    map[string]bool{},
	}
	p.conv = converter{sessionID: sessionID, turn: &p.turn}
	return p
}

// UnknownTypes returns the record keys the parser did not recognise and how
// often each appeared.
func (p *Parser) UnknownTypes() map[string]int {
	return p.unknown.Snapshot()
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
			p.flush()
			continue
		}
		p.lineNo++
		line := bytes.TrimSpace(p.scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		p.process(line)
	}
}

// emit queues entries after the open step, or straight to the output.
func (p *Parser) emit(entries []thinkt.Entry) {
	if len(entries) == 0 {
		return
	}
	if p.step != nil {
		p.step.buffered = append(p.step.buffered, entries...)
		return
	}
	p.out = append(p.out, entries...)
}

func (p *Parser) process(line []byte) {
	head, class := ClassifyLine(line)
	p.conv.lineNo = p.lineNo
	p.turn.apply(head)
	if class == thinkt.ClassUnknown {
		p.unknown.Note(head.Key, "line", p.lineNo)
	}
	// A step closed by an error or interruption waits one line for the
	// turn.step.interrupted that carries its message.
	if p.step != nil && p.step.closed {
		switch head.Type {
		case "turn.step.interrupted":
			// handled below
		case "turn.step.retrying":
			// The retry record carries the readable error of the step that
			// just failed; keep holding in case an interruption follows.
			p.takeRetryError(head)
			return
		default:
			p.flush()
		}
	}

	switch head.Key {
	case "permission.record_approval_result":
		p.recordApproval(head)
	case "subagent.spawned":
		p.recordSpawn(head)
		p.emit(p.conv.convertEvent(head, class, line))
	case "loop/step.begin":
		p.flush()
		p.step = &openStep{uuid: head.StepUUID, turnID: head.TurnID, time: head.Time}
	case "loop/content.part":
		p.appendPart(head)
	case "loop/tool.call":
		p.appendCall(head)
	case "loop/tool.result":
		if head.ToolCallID != "" {
			p.seenResults[head.ToolCallID] = true
		}
		p.emit([]thinkt.Entry{p.resultEntry(head)})
	case "loop/step.end":
		p.closeStep(head)
	case "turn.step.interrupted":
		p.interruptStep(head)
	case "agent.turn.started":
		p.turnTexts = map[string]bool{}
	case "appended/assistant":
		if p.seenMessages[head.ID] || p.anyCallSeen(head) {
			return
		}
		p.emit(p.conv.convertLegacyAssistant(head))
	case "appended/tool":
		if p.seenResults[head.ToolCallID] {
			return
		}
		p.emit(p.conv.convertLegacyTool(head))
	case "appended/user":
		// Typed prompts arrive here before their context twin; steered
		// in-turn messages arrive here after it, carrying the same id.
		text := strings.TrimSpace(head.LeadText)
		if head.ID != "" && p.seenUserIDs[head.ID] {
			return
		}
		if head.Source == "notify" && p.turnTexts[text] {
			return
		}
		p.emitUser(head)
	case "ctx/user":
		if head.ID != "" && p.seenUserIDs[head.ID] {
			return
		}
		p.emitUser(head)
	case "turn.steer":
		// A steer repeats the in-turn context message written just before it.
		if p.turnTexts[strings.TrimSpace(head.LeadText)] {
			return
		}
		p.emitUser(head)
	case "ctx/assistant":
		p.emit(p.conv.convertLegacyAssistant(head))
	case "ctx/tool":
		p.emit(p.conv.convertLegacyTool(head))
	default:
		if class.IsEntry() {
			p.emit(p.conv.convertEvent(head, class, line))
		}
	}
}

func (p *Parser) emitUser(head recordHead) {
	entries := p.conv.convertUser(head)
	if len(entries) == 0 {
		return
	}
	if head.ID != "" {
		p.seenUserIDs[head.ID] = true
	}
	p.turnTexts[strings.TrimSpace(head.LeadText)] = true
	p.emit(entries)
}

// anyCallSeen reports whether an appended assistant record repeats a tool
// call the loop already emitted.
func (p *Parser) anyCallSeen(head recordHead) bool {
	m, ok := p.conv.legacyMessage(head)
	if !ok {
		return false
	}
	for _, tc := range m.ToolCalls {
		if tc.ID != "" && p.seenCalls[tc.ID] {
			return true
		}
	}
	return false
}

func (p *Parser) recordApproval(head recordHead) {
	var r struct {
		Result struct {
			Decision      string `json:"decision"`
			Scope         string `json:"scope"`
			SelectedLabel string `json:"selectedLabel"`
			Feedback      string `json:"feedback"`
		} `json:"result"`
	}
	if json.Unmarshal(head.Payload, &r) != nil || head.ToolCallID == "" {
		return
	}
	p.approvals[head.ToolCallID] = approval{r.Result.Decision, r.Result.Scope, r.Result.SelectedLabel, r.Result.Feedback}
}

func (p *Parser) recordSpawn(head recordHead) {
	var r struct {
		SubagentID       string `json:"subagentId"`
		ParentToolCallID string `json:"parentToolCallId"`
	}
	if json.Unmarshal(head.Payload, &r) != nil || r.SubagentID == "" || r.ParentToolCallID == "" {
		return
	}
	p.spawns[r.ParentToolCallID] = childSessionID(p.sessionID, r.SubagentID)
}

// ensureStep opens an implicit step for loop content that arrives with no
// step.begin (never seen on disk, but a truncated file could do it).
func (p *Parser) ensureStep(head recordHead) {
	if p.step == nil {
		uuid := head.StepUUID
		if uuid == "" {
			uuid = composeUUID(p.sessionID, p.lineNo, "loop/step")
		}
		p.step = &openStep{uuid: uuid, turnID: head.TurnID, time: head.Time}
	}
}

func (p *Parser) appendPart(head recordHead) {
	var r struct {
		Event struct {
			Part struct {
				Type  string `json:"type"`
				Text  string `json:"text"`
				Think string `json:"think"`
			} `json:"part"`
		} `json:"event"`
	}
	if json.Unmarshal(head.Payload, &r) != nil {
		return
	}
	var block thinkt.ContentBlock
	switch r.Event.Part.Type {
	case "think":
		if r.Event.Part.Think == "" {
			return
		}
		block = thinkt.ContentBlock{Type: "thinking", Thinking: r.Event.Part.Think}
	case "text":
		if r.Event.Part.Text == "" {
			return
		}
		block = thinkt.ContentBlock{Type: "text", Text: r.Event.Part.Text}
	default:
		return
	}
	p.ensureStep(head)
	blocks := p.step.blocks
	if n := len(blocks); n > 0 && blocks[n-1].Type == block.Type {
		blocks[n-1].Thinking += block.Thinking
		blocks[n-1].Text += block.Text
		return
	}
	p.step.blocks = append(blocks, block)
}

func (p *Parser) appendCall(head recordHead) {
	var r struct {
		Event struct {
			Name string `json:"name"`
			Args any    `json:"args"`
		} `json:"event"`
	}
	if json.Unmarshal(head.Payload, &r) != nil || head.ToolCallID == "" {
		return
	}
	p.ensureStep(head)
	p.step.blocks = append(p.step.blocks, thinkt.ContentBlock{Type: "tool_use", ToolUseID: head.ToolCallID, ToolName: r.Event.Name, ToolInput: r.Event.Args})
	p.seenCalls[head.ToolCallID] = true
}

func (p *Parser) resultEntry(head recordHead) thinkt.Entry {
	var r struct {
		Event struct {
			Result struct {
				Output     any    `json:"output"`
				DurationMs *int64 `json:"durationMs"`
				Note       string `json:"note"`
				IsError    bool   `json:"isError"`
				Truncated  bool   `json:"truncated"`
			} `json:"result"`
		} `json:"event"`
	}
	_ = json.Unmarshal(head.Payload, &r)
	block := toolResultBlock(head.ToolCallID, r.Event.Result.Output, r.Event.Result.Note, r.Event.Result.IsError)
	e := p.conv.newEntry(thinkt.RoleUser, head, head.ToolCallID+":result")
	if r.Event.Result.DurationMs != nil {
		e.Metadata["duration_ms"] = *r.Event.Result.DurationMs
	}
	if r.Event.Result.Truncated {
		e.Metadata["truncated"] = true
	}
	if a, ok := p.approvals[head.ToolCallID]; ok {
		e.Permission = &thinkt.PermissionDecision{Decision: a.decision, Source: "user"}
		if a.scope != "" {
			e.Metadata["scope"] = a.scope
		}
		if a.selectedLabel != "" {
			e.Metadata["selected_label"] = a.selectedLabel
		}
		if a.feedback != "" {
			e.Metadata["feedback"] = a.feedback
		}
	}
	if child := p.spawns[head.ToolCallID]; child != "" {
		e.ChildSessionID = child
		block.ChildSessionID = child
	}
	e.ContentBlocks = []thinkt.ContentBlock{block}
	return e
}

func (p *Parser) closeStep(head recordHead) {
	if p.step == nil {
		return
	}
	var r struct {
		Event struct {
			FinishReason string          `json:"finishReason"`
			Usage        json.RawMessage `json:"usage"`
			MessageID    string          `json:"messageId"`
		} `json:"event"`
	}
	_ = json.Unmarshal(head.Payload, &r)
	p.step.closed = true
	p.step.finish = r.Event.FinishReason
	p.step.usage = decodeUsage(r.Event.Usage)
	p.step.messageID = r.Event.MessageID
	if r.Event.MessageID != "" {
		p.seenMessages[r.Event.MessageID] = true
	}
	if r.Event.FinishReason != "error" && r.Event.FinishReason != "interrupted" {
		p.flush()
	}
}

// takeRetryError records "<errorName>: <errorMessage>" from a
// turn.step.retrying record on the held failed step.
func (p *Parser) takeRetryError(head recordHead) {
	var r struct {
		ErrorName    string `json:"errorName"`
		ErrorMessage string `json:"errorMessage"`
	}
	if json.Unmarshal(head.Payload, &r) != nil || p.step.errMsg != "" {
		return
	}
	switch {
	case r.ErrorName != "" && r.ErrorMessage != "":
		p.step.errMsg = r.ErrorName + ": " + r.ErrorMessage
	case r.ErrorMessage != "":
		p.step.errMsg = r.ErrorMessage
	default:
		p.step.errMsg = r.ErrorName
	}
}

func (p *Parser) interruptStep(head recordHead) {
	if p.step == nil {
		return
	}
	var r struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(head.Payload, &r)
	p.step.errMsg = r.Message
	if p.step.errMsg == "" {
		p.step.errMsg = r.Reason
	}
	if !p.step.closed {
		p.step.closed = true
		p.step.finish = "interrupted"
	}
	p.flush()
}

// flush queues the open step's assistant entry followed by everything that
// happened while it was open, then forgets the step.
func (p *Parser) flush() {
	s := p.step
	if s == nil {
		return
	}
	p.step = nil
	head := recordHead{Type: "context.append_loop_event", Key: "loop/step", Time: s.time, TurnID: s.turnID}
	e := p.conv.newEntry(thinkt.RoleAssistant, head, s.uuid)
	failed := s.finish == "error" || s.finish == "interrupted"
	if failed {
		e.Error = s.errMsg
		if e.Error == "" {
			e.Error = s.finish
		}
	}
	e.StopReason = s.finish
	if s.messageID != "" {
		e.Metadata["message_id"] = s.messageID
	}
	if len(s.blocks) == 0 {
		if !failed {
			p.out = append(p.out, s.buffered...)
			return
		}
		e.Kind = thinkt.KindAPIError
		e.Visibility = thinkt.VisibilitySecondary
		e.Text = e.Error
		p.out = append(p.out, e)
		p.out = append(p.out, s.buffered...)
		return
	}
	for i := range s.blocks {
		if s.blocks[i].Type == "tool_use" {
			if child := p.spawns[s.blocks[i].ToolUseID]; child != "" {
				s.blocks[i].ChildSessionID = child
			}
		}
	}
	e.ContentBlocks = s.blocks
	e.Text = joinTextBlocks(s.blocks)
	e.Usage = s.usage
	p.out = append(p.out, e)
	p.out = append(p.out, s.buffered...)
}
