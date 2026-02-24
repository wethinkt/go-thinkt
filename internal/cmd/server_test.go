package cmd

import "testing"

func TestResolveCORSOrigin(t *testing.T) {
	prevFlag := serveCORSOrigin
	defer func() {
		serveCORSOrigin = prevFlag
	}()

	t.Run("flag takes precedence", func(t *testing.T) {
		t.Setenv("THINKT_CORS_ORIGIN", "https://env.example")
		serveCORSOrigin = "https://flag.example"
		if got := resolveCORSOrigin(true); got != "https://flag.example" {
			t.Fatalf("resolveCORSOrigin() = %q, want %q", got, "https://flag.example")
		}
	})

	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("THINKT_CORS_ORIGIN", "https://env.example")
		serveCORSOrigin = ""
		if got := resolveCORSOrigin(true); got != "https://env.example" {
			t.Fatalf("resolveCORSOrigin() = %q, want %q", got, "https://env.example")
		}
	})

	t.Run("auth enabled default disables cors", func(t *testing.T) {
		t.Setenv("THINKT_CORS_ORIGIN", "")
		serveCORSOrigin = ""
		if got := resolveCORSOrigin(true); got != "" {
			t.Fatalf("resolveCORSOrigin() = %q, want empty string", got)
		}
	})

	t.Run("auth disabled default allows wildcard", func(t *testing.T) {
		t.Setenv("THINKT_CORS_ORIGIN", "")
		serveCORSOrigin = ""
		if got := resolveCORSOrigin(false); got != "*" {
			t.Fatalf("resolveCORSOrigin() = %q, want %q", got, "*")
		}
	})
}
