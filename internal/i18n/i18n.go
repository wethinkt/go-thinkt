// Package i18n provides internationalization support for thinkt.
//
// Usage:
//
//	i18n.Init("en")                                              // at startup
//	i18n.T("common.loading", "Loading...")                         // simple string
//	i18n.Tf("tui.filter.user", "User: %s", name)                 // with fmt args
//	i18n.Tn("tui.viewer.sessionsCount", "{{.Count}} session", "{{.Count}} sessions", n) // plural
package i18n

import (
	"embed"
	"fmt"
	"os"
	"sort"
	"strings"
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

// LangInfo describes an available language.
type LangInfo struct {
	Tag         string `json:"tag"`
	Name        string `json:"name"`
	EnglishName string `json:"english_name"`
	Active      bool   `json:"active"`
	Coverage    int    `json:"coverage"`
}

// knownLanguages maps BCP 47 tags to [native name, English name].
var knownLanguages = map[string][2]string{
	"en":      {"English", "English"},
	"zh-Hans": {"简体中文", "Chinese (Simplified)"},
}

// AvailableLanguages returns info about all languages with embedded locale files.
// English is always included even without a locale file.
func AvailableLanguages(activeTag string) []LangInfo {
	entries, _ := localeFS.ReadDir("locales")

	seen := make(map[string]bool)
	var langs []LangInfo

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		tag := strings.TrimSuffix(name, ".toml")
		seen[tag] = true

		coverage := countMessageIDs(name)
		names := knownLanguages[tag]
		if names[0] == "" {
			names = [2]string{tag, tag}
		}

		langs = append(langs, LangInfo{
			Tag:         tag,
			Name:        names[0],
			EnglishName: names[1],
			Active:      tag == activeTag,
			Coverage:    coverage,
		})
	}

	// Ensure English is always listed
	if !seen["en"] {
		names := knownLanguages["en"]
		langs = append(langs, LangInfo{
			Tag:         "en",
			Name:        names[0],
			EnglishName: names[1],
			Active:      "en" == activeTag,
			Coverage:    0,
		})
	}

	sort.Slice(langs, func(i, j int) bool {
		return langs[i].Tag < langs[j].Tag
	})
	return langs
}

// countMessageIDs counts [section] headers in a TOML locale file as a rough coverage metric.
func countMessageIDs(filename string) int {
	data, err := localeFS.ReadFile("locales/" + filename)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[[") {
			count++
		}
	}
	return count
}

// previewKeys are the message IDs and defaults used for the language picker preview.
var previewKeys = [][2]string{
	{"tui.filter.user", "User"},
	{"tui.filter.assistant", "Assistant"},
	{"tui.filter.thinking", "Thinking"},
	{"tui.filter.tools", "Tools"},
	{"tui.search.title", "Search sessions"},
	{"common.loading", "Loading..."},
	{"common.time.justNow", "just now"},
	{"common.time.oneMinAgo", "1 min ago"},
	{"common.time.oneHourAgo", "1 hour ago"},
	{"tui.help.scrollUp", "scroll up"},
	{"tui.help.scrollDown", "scroll down"},
}

// PreviewStrings creates a temporary localizer for the given tag and returns
// sample key→translated string pairs for the TUI preview.
func PreviewStrings(tag string) map[string]string {
	mu.RLock()
	b := bundle
	mu.RUnlock()

	if b == nil {
		// Bundle not initialized; return defaults.
		result := make(map[string]string, len(previewKeys))
		for _, kv := range previewKeys {
			result[kv[0]] = kv[1]
		}
		return result
	}

	loc := i18n.NewLocalizer(b, tag)
	result := make(map[string]string, len(previewKeys))
	for _, kv := range previewKeys {
		s, err := loc.Localize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{ID: kv[0], Other: kv[1]},
		})
		if err != nil {
			s = kv[1]
		}
		result[kv[0]] = s
	}
	return result
}

// PreviewKeys returns the ordered list of preview key IDs and their labels.
func PreviewKeys() [][2]string {
	return previewKeys
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
