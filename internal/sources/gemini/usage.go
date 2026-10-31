package gemini

import "github.com/wethinkt/go-thinkt/internal/thinkt"

// usageFromTokens maps a Gemini token summary onto TokenUsage. Gemini's
// output excludes thinking (total = input + output + thoughts), while thinkt
// counts ThinkingTokens as part of OutputTokens, so thoughts are added to
// output. input already includes cached. tool and total have no field.
func usageFromTokens(t *Tokens) *thinkt.TokenUsage {
	if t == nil {
		return nil
	}
	return &thinkt.TokenUsage{
		InputTokens:          t.Input,
		CacheReadInputTokens: t.Cached,
		OutputTokens:         t.Output + t.Thoughts,
		ThinkingTokens:       t.Thoughts,
	}
}
