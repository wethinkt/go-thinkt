package claude

import (
	"encoding/json"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// RecordClass says what a transcript line is for, decided from its type alone.
// Claude Code declares the on-disk format internal, so unknown types are
// expected over time and are surfaced rather than dropped.
type RecordClass int

const (
	ClassTurn         RecordClass = iota // user, assistant
	ClassEvent                           // system, queue-operation, progress
	ClassContext                         // allow-listed attachment injected into the model turn
	ClassCheckpoint                      // file-history-snapshot, file-history-delta, summary
	ClassSessionState                    // ai-title, last-prompt, pr-link, relocated, worktree-state, cost-state
	ClassBookkeeping                     // dropped: request dumps, mode latches, noisy attachments
	ClassUnknown                         // type never seen; emitted as context so format drift is visible
)

func (c RecordClass) String() string {
	switch c {
	case ClassTurn:
		return "turn"
	case ClassEvent:
		return "event"
	case ClassContext:
		return "context"
	case ClassCheckpoint:
		return "checkpoint"
	case ClassSessionState:
		return "session-state"
	case ClassBookkeeping:
		return "bookkeeping"
	case ClassUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

// IsEntry reports whether records of this class become thinkt entries.
func (c RecordClass) IsEntry() bool {
	switch c {
	case ClassTurn, ClassEvent, ClassContext, ClassCheckpoint, ClassUnknown:
		return true
	default:
		return false
	}
}

// ContextAttachmentTypes lists the attachment types a reader may want to see.
// Every other attachment type is harness bookkeeping and is dropped.
var ContextAttachmentTypes = map[string]bool{
	"hook_success":            true,
	"hook_additional_context": true,
	"queued_command":          true,
	"task_status":             true,
	"instructions":            true,
	"file":                    true,
	"invoked_skills":          true,
	"skill_listing":           true,
	"diagnostics":             true,
	"edited_text_file":        true,
	"compact_file_reference":  true,
	"read_truncation_notice":  true,
}

// ClassifyType is the pure type table. attachmentType matters only for
// EntryTypeAttachment.
func ClassifyType(t EntryType, attachmentType string) RecordClass {
	switch t {
	case EntryTypeUser, EntryTypeAssistant:
		return ClassTurn
	case EntryTypeSystem, EntryTypeQueueOperation, EntryTypeProgress:
		// classifyHead demotes a queue-operation with no content to bookkeeping.
		return ClassEvent
	case EntryTypeAttachment:
		if ContextAttachmentTypes[attachmentType] {
			return ClassContext
		}
		return ClassBookkeeping
	case EntryTypeFileHistorySnapshot, EntryTypeFileHistoryDelta, EntryTypeSummary:
		return ClassCheckpoint
	case EntryTypeAITitle, EntryTypeLastPrompt, EntryTypePRLink, EntryTypeRelocated, EntryTypeWorktreeState, EntryTypeCostState:
		return ClassSessionState
	case EntryTypeAPIRequest, EntryTypeAPIRequestBlob, EntryTypeAPIRequestShape, EntryTypeMode, EntryTypePermissionMode, EntryTypeAtisLatch, "":
		return ClassBookkeeping
	default:
		return ClassUnknown
	}
}

// decodeRecord turns a raw line into an Entry when its class is emitted as an
// entry. It returns (nil, class, nil) for lines that are skipped by design and
// an error only for malformed lines of an entry class. User and assistant
// messages are pre-parsed; unknown types keep their raw line.
func decodeRecord(line []byte) (*Entry, RecordClass, error) {
	_, _, class, err := classifyHead(line)
	if err != nil {
		return nil, class, err
	}
	if !class.IsEntry() {
		return nil, class, nil
	}
	var entry Entry
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil, class, err
	}
	if class == ClassUnknown {
		entry.Raw = append([]byte(nil), line...)
	}
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
	return &entry, class, nil
}

// IsSearchable reports whether a full-text hit inside this raw line is
// conversation content: the line must be an entry class and, once converted,
// not secondary. Bookkeeping, session state, reminders, hook output, notices,
// and command echoes all return false. It fully decodes the line, so call it
// only after a cheap match.
func IsSearchable(line []byte) bool {
	entry, class, err := decodeRecord(line)
	if err != nil || entry == nil || !class.IsEntry() {
		return false
	}
	return !convertEntry(entry, thinkt.SourceClaude, "").IsSecondary()
}

// ClassifyLine decodes only the type and attachment.type fields of a raw
// line. A line that is not a JSON object classifies as bookkeeping.
func ClassifyLine(line []byte) (EntryType, string, RecordClass) {
	typ, att, class, _ := classifyHead(line)
	return typ, att, class
}

// classifyHead is ClassifyLine with the decode error, so the Parser can keep
// reporting malformed lines while the classification still drops them.
func classifyHead(line []byte) (EntryType, string, RecordClass, error) {
	var head struct {
		Type       EntryType `json:"type"`
		Attachment struct {
			Type string `json:"type"`
		} `json:"attachment"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return "", "", ClassBookkeeping, err
	}
	class := ClassifyType(head.Type, head.Attachment.Type)
	// A queue-operation without content (dequeue, remove, popAll) is only a
	// timestamp; it says nothing a reader can act on.
	if head.Type == EntryTypeQueueOperation && !hasContent(head.Content) {
		class = ClassBookkeeping
	}
	return head.Type, head.Attachment.Type, class, nil
}

// hasContent reports whether a raw content field holds a non-empty value.
func hasContent(raw json.RawMessage) bool {
	trimmed := string(raw)
	return trimmed != "" && trimmed != `""` && trimmed != "null" && trimmed != "[]"
}
