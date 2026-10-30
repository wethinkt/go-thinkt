package sources

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// sniffJSONL names the source of a .jsonl file no path rule claimed, from
// its first recognisable record: Claude, Codex or Qwen. It returns "" when
// no record is recognised. Claude records carry sessionId too, so a line
// counts as Qwen only when it has a Qwen-shaped field (message.parts,
// toolCallResult, usageMetadata, systemPayload).
func sniffJSONL(path string) (thinkt.Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxScannerCapacity)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}

		if isQwenRecord(obj) {
			return thinkt.SourceQwen, nil
		}

		// Claude JSONL entries include both "type" and "message" on message lines.
		if _, hasType := obj["type"]; hasType {
			if _, hasMessage := obj["message"]; hasMessage {
				return thinkt.SourceClaude, nil
			}
		}

		// Some Claude entries (e.g. snapshots) omit "message"; use UUID+type as a secondary hint.
		if t, ok := obj["type"].(string); ok {
			switch {
			case t == "summary":
				return thinkt.SourceClaude, nil
			case t == "user" || t == "assistant" || t == "system" || t == "tool" || t == "progress":
				if _, hasUUID := obj["uuid"]; hasUUID {
					return thinkt.SourceClaude, nil
				}
			case strings.HasPrefix(t, "file-history-"):
				return thinkt.SourceClaude, nil
			case t == "session_meta":
				return thinkt.SourceCodex, nil
			case t == "event_msg":
				if payload, ok := obj["payload"].(map[string]any); ok {
					switch payloadType, _ := payload["type"].(string); payloadType {
					case "user_message", "agent_message", "agent_reasoning", "turn_context", "turn_aborted", "token_count":
						return thinkt.SourceCodex, nil
					}
				}
			case t == "response_item":
				if payload, ok := obj["payload"].(map[string]any); ok {
					switch payloadType, _ := payload["type"].(string); payloadType {
					case "message", "reasoning", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output":
						return thinkt.SourceCodex, nil
					}
				}
			}
			// Claude session-state and other records the Claude
			// classifier knows (ai-title, last-prompt, mode, ...).
			if _, hasSessionID := obj["sessionId"]; hasSessionID {
				if _, _, class := claude.ClassifyLine([]byte(line)); class != thinkt.ClassUnknown {
					return thinkt.SourceClaude, nil
				}
			}
		}
	}

	return "", scanner.Err()
}

// isQwenRecord reports whether a decoded line has a field only Qwen Code
// writes.
func isQwenRecord(obj map[string]any) bool {
	if _, ok := obj["sessionId"]; !ok {
		return false
	}
	for _, k := range []string{"toolCallResult", "usageMetadata", "systemPayload"} {
		if _, ok := obj[k]; ok {
			return true
		}
	}
	msg, _ := obj["message"].(map[string]any)
	_, hasParts := msg["parts"]
	return hasParts
}
