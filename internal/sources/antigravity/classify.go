package antigravity

import (
	"encoding/json"
	"time"
)

// RecordClass says what a transcript step is for, decided from its type.
type RecordClass int

const (
	ClassUserInput     RecordClass = iota // USER_INPUT
	ClassModelTurn                        // PLANNER_RESPONSE
	ClassToolResult                       // the nine result step types
	ClassSystemMessage                    // SYSTEM_MESSAGE: task notifications, subagent messages
	ClassHarness                          // EPHEMERAL_MESSAGE, CONVERSATION_HISTORY
	ClassCheckpoint                       // CHECKPOINT: compaction summary
	ClassAPIError                         // ERROR_MESSAGE
	ClassUnknown                          // type never seen; emitted as a secondary entry so drift is visible
	ClassInvalid                          // not JSON (a partial last line)
)

func (c RecordClass) String() string {
	switch c {
	case ClassUserInput:
		return "user-input"
	case ClassModelTurn:
		return "model-turn"
	case ClassToolResult:
		return "tool-result"
	case ClassSystemMessage:
		return "system-message"
	case ClassHarness:
		return "harness"
	case ClassCheckpoint:
		return "checkpoint"
	case ClassAPIError:
		return "api-error"
	case ClassUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

// resultTypes are the step types that carry the result of one tool call.
var resultTypes = map[string]bool{
	"GENERIC":          true,
	"VIEW_FILE":        true,
	"RUN_COMMAND":      true,
	"CODE_ACTION":      true,
	"GREP_SEARCH":      true,
	"LIST_DIRECTORY":   true,
	"SEARCH_WEB":       true,
	"READ_URL_CONTENT": true,
	"ASK_QUESTION":     true,
}

// mediaRef is one attachment on a step (an image saved under the brain dir).
type mediaRef struct {
	MimeType string `json:"mime_type"`
	URI      string `json:"uri"`
}

// stepHead is the view of one transcript line that classification and
// conversion share. Raw keeps the line for fields only model turns need.
type stepHead struct {
	Index     int        `json:"step_index"`
	Source    string     `json:"source"`
	Type      string     `json:"type"`
	Status    string     `json:"status"`
	Error     string     `json:"error"`
	CreatedAt time.Time  `json:"created_at"`
	Content   string     `json:"content"`
	Media     []mediaRef `json:"media"`
	Raw       []byte     `json:"-"`
}

// ClassifyLine decodes one transcript line and says what it is.
func ClassifyLine(line []byte) (stepHead, RecordClass) {
	var h stepHead
	if err := json.Unmarshal(line, &h); err != nil {
		return stepHead{}, ClassInvalid
	}
	h.Raw = line
	switch {
	case h.Type == "USER_INPUT":
		return h, ClassUserInput
	case h.Type == "PLANNER_RESPONSE":
		return h, ClassModelTurn
	case resultTypes[h.Type]:
		return h, ClassToolResult
	case h.Type == "SYSTEM_MESSAGE":
		return h, ClassSystemMessage
	case h.Type == "EPHEMERAL_MESSAGE" || h.Type == "CONVERSATION_HISTORY":
		return h, ClassHarness
	case h.Type == "CHECKPOINT":
		return h, ClassCheckpoint
	case h.Type == "ERROR_MESSAGE":
		return h, ClassAPIError
	default:
		return h, ClassUnknown
	}
}

// NewSearchFilter returns a per-file predicate that keeps the lines a person
// reads: prompts, model turns, tool results, and a subagent's task prompt.
// Harness steps, other system messages, checkpoints and errors stay out of
// the search index.
//
// The filter sees one file and not the parent's id, so a task prompt is the
// first system message from another conversation before any user input; a
// main conversation always opens with user input.
func NewSearchFilter() func(line []byte) bool {
	seenOpening := false
	return func(line []byte) bool {
		h, class := ClassifyLine(line)
		switch class {
		case ClassUserInput:
			seenOpening = true
			return true
		case ClassModelTurn, ClassToolResult:
			return true
		case ClassSystemMessage:
			if seenOpening || !isConversationSender(messageSender(h.Content)) {
				return false
			}
			seenOpening = true
			return true
		default:
			return false
		}
	}
}
