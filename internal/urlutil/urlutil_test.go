package urlutil

import "testing"

func TestValidateEndpointURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "https remote", input: "https://share.wethinkt.com"},
		{name: "https path", input: "https://example.com/api/v1"},
		{name: "http localhost", input: "http://localhost:8784"},
		{name: "http loopback ipv4", input: "http://127.0.0.1:8784/v1/traces"},
		{name: "http loopback ipv6", input: "http://[::1]:8784/v1/traces"},
		{name: "empty", input: "", wantErr: true},
		{name: "relative", input: "/api", wantErr: true},
		{name: "missing host", input: "https:///api", wantErr: true},
		{name: "http remote", input: "http://example.com", wantErr: true},
		{name: "bad scheme", input: "javascript:alert(1)", wantErr: true},
		{name: "userinfo", input: "https://user:pass@example.com", wantErr: true},
		{name: "query", input: "https://example.com/path?q=1", wantErr: true},
		{name: "fragment", input: "https://example.com/path#frag", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidateEndpointURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateEndpointURL(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateEndpointURL(%q) unexpected error: %v", tt.input, err)
			}
			if got == "" {
				t.Fatalf("ValidateEndpointURL(%q) returned empty string", tt.input)
			}
		})
	}
}

func TestValidateEndpointURLAllowHTTP(t *testing.T) {
	t.Parallel()

	got, err := ValidateEndpointURLAllowHTTP("http://collect.example.com/v1/traces")
	if err != nil {
		t.Fatalf("ValidateEndpointURLAllowHTTP unexpected error: %v", err)
	}
	if got != "http://collect.example.com/v1/traces" {
		t.Fatalf("ValidateEndpointURLAllowHTTP = %q, want %q", got, "http://collect.example.com/v1/traces")
	}
}
