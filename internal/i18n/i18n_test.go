package i18n

import (
	"testing"
)

func TestT_ReturnsDefaultMessage(t *testing.T) {
	Init("en")
	got := T("common.loading", "Loading...")
	if got != "Loading..." {
		t.Errorf("T() = %q, want %q", got, "Loading...")
	}
}

func TestTn_Pluralization(t *testing.T) {
	Init("en")

	one := Tn("test.sessions", "{{.Count}} session", "{{.Count}} sessions", 1)
	if one != "1 session" {
		t.Errorf("Tn(1) = %q, want %q", one, "1 session")
	}

	many := Tn("test.sessions", "{{.Count}} session", "{{.Count}} sessions", 5)
	if many != "5 sessions" {
		t.Errorf("Tn(5) = %q, want %q", many, "5 sessions")
	}
}

func TestInit_FallbackToEnglish(t *testing.T) {
	Init("xx-nonexistent")
	got := T("common.loading", "Loading...")
	if got != "Loading..." {
		t.Errorf("expected English fallback, got %q", got)
	}
}
