package i18n

import (
	"testing"
)

func TestChineseLocale(t *testing.T) {
	Init("zh-Hans")

	tests := []struct {
		id     string
		def    string
		wantZh string
	}{
		{"common.loading", "Loading...", "加载中..."},
		{"tui.shell.selectProject", "Select project...", "选择项目..."},
		{"tui.filter.user", "User", "用户"},
		{"tui.filter.assistant", "Assistant", "助手"},
		{"tui.search.title", "Search Sessions", "搜索会话"},
		{"tui.help.scrollUp", "scroll up", "向上滚动"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := T(tt.id, tt.def)
			if got != tt.wantZh {
				t.Errorf("T(%q) = %q, want %q", tt.id, got, tt.wantZh)
			}
		})
	}
}

func TestEnglishDoesNotReturnChinese(t *testing.T) {
	Init("en")

	got := T("common.loading", "Loading...")
	if got != "Loading..." {
		t.Errorf("English T(common.loading) = %q, want %q", got, "Loading...")
	}
}

func TestLocaleSwitch(t *testing.T) {
	// Start with English
	Init("en")
	en := T("tui.filter.user", "User")
	if en != "User" {
		t.Errorf("English filter.user = %q, want %q", en, "User")
	}

	// Switch to Chinese
	Init("zh-Hans")
	zh := T("tui.filter.user", "User")
	if zh != "用户" {
		t.Errorf("Chinese filter.user = %q, want %q", zh, "用户")
	}

	// Switch back to English
	Init("en")
	en2 := T("tui.filter.user", "User")
	if en2 != "User" {
		t.Errorf("English filter.user after switch = %q, want %q", en2, "User")
	}
}

func TestUntranslatedKeyFallsBack(t *testing.T) {
	Init("zh-Hans")

	// Use a key that definitely isn't in zh-Hans.toml
	got := T("some.untranslated.key", "English fallback")
	if got != "English fallback" {
		t.Errorf("untranslated key = %q, want %q", got, "English fallback")
	}
}
