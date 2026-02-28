// Package i18n provides internationalization support for thinkt.
//
// Usage:
//
//	i18n.Init("en")                                              // at startup
//	i18n.T("tui.loading", "Loading...")                          // simple string
//	i18n.Tf("tui.sessions.title", "%d sessions", count)          // with fmt args
//	i18n.Tn("tui.sessions", "{{.Count}} session", "{{.Count}} sessions", n) // plural
package i18n

import (
	"embed"
	"fmt"
	"os"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var localeFS embed.FS

var (
	bundle    *i18n.Bundle
	localizer *i18n.Localizer
	mu        sync.RWMutex
)

// Init initializes the i18n system with the given language tag.
// Falls back to English if the language is not available.
// Safe to call multiple times (e.g., after config reload).
func Init(lang string) {
	mu.Lock()
	defer mu.Unlock()

	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	// Load all available locale files from embedded FS.
	entries, _ := localeFS.ReadDir("locales")
	for _, e := range entries {
		_, _ = bundle.LoadMessageFileFS(localeFS, "locales/"+e.Name())
	}

	localizer = i18n.NewLocalizer(bundle, lang, "en")
}

// T returns the localized string for the given message ID.
// The defaultMsg is used as the English fallback and is what
// goi18n extract picks up from source code.
func T(id string, defaultMsg string) string {
	mu.RLock()
	l := localizer
	mu.RUnlock()

	if l == nil {
		return defaultMsg
	}

	s, err := l.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    id,
			Other: defaultMsg,
		},
	})
	if err != nil {
		return defaultMsg
	}
	return s
}

// Tf returns the localized string with fmt.Sprintf-style formatting.
// Use for strings with %d, %s, etc. placeholders.
func Tf(id string, defaultMsg string, args ...any) string {
	return fmt.Sprintf(T(id, defaultMsg), args...)
}

// Tn returns the localized string with pluralization.
// one/other use go template syntax with {{.Count}}.
func Tn(id string, one string, other string, count int) string {
	mu.RLock()
	l := localizer
	mu.RUnlock()

	if l == nil {
		if count == 1 {
			return fmt.Sprintf("%d %s", count, one)
		}
		return fmt.Sprintf("%d %s", count, other)
	}

	s, err := l.Localize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    id,
			One:   one,
			Other: other,
		},
		PluralCount:  count,
		TemplateData: map[string]int{"Count": count},
	})
	if err != nil {
		return fmt.Sprintf(other, count)
	}
	return s
}

// ResolveLocale determines the active locale from env/config.
// Priority: THINKT_LANG > configLang > LANG/LC_ALL > "en"
func ResolveLocale(configLang string) string {
	if v := os.Getenv("THINKT_LANG"); v != "" {
		return v
	}
	if configLang != "" {
		return configLang
	}
	if v := os.Getenv("LC_ALL"); v != "" {
		return normalizeLocale(v)
	}
	if v := os.Getenv("LANG"); v != "" {
		return normalizeLocale(v)
	}
	return "en"
}

// normalizeLocale converts POSIX locale format to BCP 47.
// e.g., "zh_CN.UTF-8" -> "zh-CN", "en_US" -> "en-US"
func normalizeLocale(posix string) string {
	// Strip encoding suffix (.UTF-8, .utf8, etc.)
	for i, c := range posix {
		if c == '.' {
			posix = posix[:i]
			break
		}
	}
	// Replace underscore with hyphen
	result := make([]byte, len(posix))
	for i := range posix {
		if posix[i] == '_' {
			result[i] = '-'
		} else {
			result[i] = posix[i]
		}
	}
	return string(result)
}
