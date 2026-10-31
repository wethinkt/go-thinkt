package relay

import (
	"testing"
)

func TestNormalizeCollectorURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"http://localhost:4318", "http://localhost:4318/v1/traces"},
		{"http://localhost:4318/", "http://localhost:4318/v1/traces"},
		{"http://localhost:4318/v1/traces", "http://localhost:4318/v1/traces"},
		{"http://localhost:4318/v1/traces/", "http://localhost:4318/v1/traces"},
		{"http://localhost:4318/v1", "http://localhost:4318/v1/traces"},
		{"https://collect.example.com", "https://collect.example.com/v1/traces"},
		{"https://collect.example.com/v1/traces", "https://collect.example.com/v1/traces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeCollectorURL(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeCollectorURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRelayConfig_DefaultsNormalizesURL(t *testing.T) {
	cfg := RelayConfig{CollectorURL: "http://localhost:4318"}
	cfg.Defaults()
	want := "http://localhost:4318/v1/traces"
	if cfg.CollectorURL != want {
		t.Errorf("Defaults() CollectorURL = %q, want %q", cfg.CollectorURL, want)
	}
}

func TestValidateCollectorURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "localhost http", input: "http://localhost:4318", want: "http://localhost:4318/v1/traces"},
		{name: "remote http", input: "http://collect.example.com", want: "http://collect.example.com/v1/traces"},
		{name: "remote https", input: "https://collect.example.com", want: "https://collect.example.com/v1/traces"},
		{name: "query rejected", input: "https://collect.example.com?q=1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateCollectorURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateCollectorURL(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateCollectorURL(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ValidateCollectorURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
