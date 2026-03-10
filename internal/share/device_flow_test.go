package share

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDeviceFlowRequestCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/device/code" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(DeviceCodeResponse{
			DeviceCode:      "device-123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer server.Close()

	client := NewDeviceFlowClient(server.URL)
	resp, err := client.RequestCode()
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if resp.UserCode != "ABCD-1234" {
		t.Errorf("user_code = %q, want %q", resp.UserCode, "ABCD-1234")
	}
	if resp.DeviceCode != "device-123" {
		t.Errorf("device_code = %q, want %q", resp.DeviceCode, "device-123")
	}
}

func TestDeviceFlowPollForToken(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count <= 1 {
			// First call: still pending
			w.WriteHeader(202)
			json.NewEncoder(w).Encode(TokenResponse{Error: "authorization_pending"})
			return
		}
		// Second call: success
		json.NewEncoder(w).Encode(TokenResponse{
			Token: "session-token-xyz",
			User: struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			}{ID: "user-1", Username: "testuser"},
		})
	}))
	defer server.Close()

	client := NewDeviceFlowClient(server.URL)
	// Use interval=0 for fast test (will still sleep 0 seconds)
	resp, err := client.PollForToken("device-123", 0)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if resp.Token != "session-token-xyz" {
		t.Errorf("token = %q, want %q", resp.Token, "session-token-xyz")
	}
	if resp.User.Username != "testuser" {
		t.Errorf("username = %q, want %q", resp.User.Username, "testuser")
	}
}
