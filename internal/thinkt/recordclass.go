package thinkt

// RecordClass says what one line of a source's session file is for. Each
// source's classifier decides it from the line's record type and a few
// cheap fields; formats drift, so a type the classifier has never seen is
// surfaced rather than dropped.
type RecordClass int

const (
	ClassTurn         RecordClass = iota // user, assistant and tool pairs
	ClassEvent                           // system events the reader may want to see
	ClassContext                         // harness-injected context sent to the model
	ClassCheckpoint                      // file snapshots and summaries
	ClassSessionState                    // titles, settings and other session state; not an entry
	ClassBookkeeping                     // dropped: request dumps, mirrors, counters
	ClassUnknown                         // type never seen; emitted so format drift is visible
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
