package share

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong auth header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content-type header")
		}

		body, _ := io.ReadAll(r.Body)
		var req UploadRequest
		_ = json.Unmarshal(body, &req)

		if req.Visibility != "public" {
			t.Errorf("visibility = %v, want public", req.Visibility)
		}
		if req.Title != "Test Trace" {
			t.Errorf("title = %v, want Test Trace", req.Title)
		}

		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(UploadResponse{
			ID:         "trace-123",
			Slug:       "abcd1234",
			URL:        "https://share.wethinkt.com/t/abcd1234",
			Visibility: "public",
		})
	}))
	defer server.Close()

	client := &UploadClient{
		Endpoint:   server.URL,
		Token:      "test-token",
		HTTPClient: http.DefaultClient,
	}

	resp, err := client.Upload([]byte(`{"entries":[]}`), "public", "Test Trace")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if resp.Slug != "abcd1234" {
		t.Errorf("slug = %q, want %q", resp.Slug, "abcd1234")
	}
	if resp.URL != "https://share.wethinkt.com/t/abcd1234" {
		t.Errorf("url = %q, want expected", resp.URL)
	}
}

func TestUploadTrace_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_ = json.NewEncoder(w).Encode(UploadResponse{Error: "Trace limit reached (50)"})
	}))
	defer server.Close()

	client := &UploadClient{
		Endpoint:   server.URL,
		Token:      "test-token",
		HTTPClient: http.DefaultClient,
	}

	_, err := client.Upload([]byte(`{}`), "private", "Test")
	if err == nil {
		t.Error("expected error for 429 response")
	}
}
