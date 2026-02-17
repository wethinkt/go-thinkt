// Package sources aggregates all source factories.
package sources

import (
	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/codex"
	"github.com/wethinkt/go-thinkt/internal/sources/copilot"
	"github.com/wethinkt/go-thinkt/internal/sources/gemini"
	"github.com/wethinkt/go-thinkt/internal/sources/kimi"
	"github.com/wethinkt/go-thinkt/internal/sources/qwen"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// AllFactories returns factories for all supported sources.
// Add new sources here when adding support for a new AI coding tool.
func AllFactories() []thinkt.StoreFactory {
	return []thinkt.StoreFactory{
		claude.Factory(),
		codex.Factory(),
		copilot.Factory(),
		gemini.Factory(),
		kimi.Factory(),
		qwen.Factory(),
	}
}
