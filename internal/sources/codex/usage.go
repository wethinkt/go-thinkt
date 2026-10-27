package codex

import (
	"encoding/json"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// usageRecord is the payload of a token_usage_record line.
type usageRecord struct {
	TurnID string `json:"turn_id"`
	Usage  *struct {
		InputTokens           int `json:"input_tokens"`
		CachedInputTokens     int `json:"cached_input_tokens"`
		CacheWriteInputTokens int `json:"cache_write_input_tokens"`
		OutputTokens          int `json:"output_tokens"`
		ReasoningOutputTokens int `json:"reasoning_output_tokens"`
		TotalTokens           int `json:"total_tokens"`
	} `json:"usage"`
}

// decodeUsage maps a token_usage_record payload onto TokenUsage; nil when
// the payload has no usage object.
func decodeUsage(payload json.RawMessage) *thinkt.TokenUsage {
	var r usageRecord
	if json.Unmarshal(payload, &r) != nil || r.Usage == nil {
		return nil
	}
	return &thinkt.TokenUsage{
		InputTokens:              r.Usage.InputTokens,
		OutputTokens:             r.Usage.OutputTokens,
		CacheReadInputTokens:     r.Usage.CachedInputTokens,
		CacheCreationInputTokens: r.Usage.CacheWriteInputTokens,
		ThinkingTokens:           r.Usage.ReasoningOutputTokens,
	}
}
