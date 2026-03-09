package agents

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestFetchRemoteAgents_SendsBearerToken(t *testing.T) {
	const token = "secret-token"

	hub := NewHub(HubConfig{
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.Header.Get("Authorization"); got != "Bearer "+token {
					t.Fatalf("Authorization = %q, want %q", got, "Bearer "+token)
				}
				if got := req.URL.String(); got != "http://collector.example/v1/agents" {
					t.Fatalf("URL = %q, want %q", got, "http://collector.example/v1/agents")
				}
				body := `[{"instance_id":"agent-1","platform":"codex","hostname":"host","status":"active"}]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})

	agents := hub.fetchRemoteAgents(context.Background(), CollectorEndpoint{
		URL:   "http://collector.example",
		Token: token,
	})

	if len(agents) != 1 {
		t.Fatalf("len(agents) = %d, want 1", len(agents))
	}
	if got := agents[0].CollectorToken; got != token {
		t.Fatalf("CollectorToken = %q, want %q", got, token)
	}
}

func TestStream_RemoteUsesCollectorToken(t *testing.T) {
	orig := streamRemoteFn
	t.Cleanup(func() {
		streamRemoteFn = orig
	})

	called := false
	streamRemoteFn = func(ctx context.Context, wsURL string, token string) (<-chan StreamEntry, error) {
		called = true
		if wsURL != "http://collector.example/v1/sessions/session-123/ws" {
			t.Fatalf("wsURL = %q", wsURL)
		}
		if token != "secret-token" {
			t.Fatalf("token = %q", token)
		}
		ch := make(chan StreamEntry)
		close(ch)
		return ch, nil
	}

	hub := NewHub(HubConfig{})
	hub.mu.Lock()
	hub.agents = append(hub.agents, UnifiedAgent{
		ID:             "agent-1",
		SessionID:      "session-123",
		CollectorURL:   "http://collector.example",
		CollectorToken: "secret-token",
		MachineID:      "remote-machine",
	})
	hub.mu.Unlock()

	if _, err := hub.Stream(context.Background(), "agent-1", 0); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if !called {
		t.Fatal("expected remote stream to be invoked")
	}
}

func TestFetchRemoteAgents_UnauthorizedReturnsNoAgents(t *testing.T) {
	hub := NewHub(HubConfig{
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	})

	agents := hub.fetchRemoteAgents(context.Background(), CollectorEndpoint{
		URL:   "http://collector.example",
		Token: "bad-token",
	})
	if len(agents) != 0 {
		t.Fatalf("len(agents) = %d, want 0", len(agents))
	}
}
