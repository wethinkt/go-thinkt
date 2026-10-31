package gemini

import (
	"encoding/json"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// The classifier sorts JSONL lines into thinkt.RecordClass values:
//   thinkt.ClassTurn         user and gemini messages
//   thinkt.ClassEvent        info, warning and error notices
//   thinkt.ClassSessionState the header line and $set metadata updates
//   thinkt.ClassBookkeeping  $patch, $rewindTo and $set.messages; replay applies them, they are never entries
//   thinkt.ClassUnknown      a line of no known shape

// classifyLine decides a line's class from its keys, in the order the CLI
// loader tests them.
func classifyLine(line []byte) thinkt.RecordClass {
	var rec map[string]json.RawMessage
	if json.Unmarshal(line, &rec) != nil {
		return thinkt.ClassBookkeeping
	}
	switch {
	case isString(rec["$rewindTo"]), isObject(rec["$patch"]):
		return thinkt.ClassBookkeeping
	case isString(rec["id"]):
		var head struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(line, &head)
		switch head.Type {
		case "user", "gemini":
			return thinkt.ClassTurn
		case "info", "warning", "error":
			return thinkt.ClassEvent
		}
		return thinkt.ClassUnknown
	case isObject(rec["$set"]):
		var set map[string]json.RawMessage
		_ = json.Unmarshal(rec["$set"], &set)
		if isArray(set["messages"]) {
			return thinkt.ClassBookkeeping
		}
		return thinkt.ClassSessionState
	case isString(rec["sessionId"]) && isString(rec["projectHash"]):
		return thinkt.ClassSessionState
	}
	return thinkt.ClassUnknown
}
