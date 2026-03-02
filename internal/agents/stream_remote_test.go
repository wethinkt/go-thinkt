package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRemoteStream_ReceivesEntries(t *testing.T) {
	// Create a test WebSocket server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Log("accept error:", err)
			return
		}
		defer conn.CloseNow() //nolint:errcheck

		entry := StreamEntry{
			Timestamp: time.Now(),
			Role:      "assistant",
			Text:      "test response",
			Model:     "claude-sonnet-4-5-20250929",
		}
		data, _ := json.Marshal(entry)
		_ = conn.Write(r.Context(), websocket.MessageText, data)

		// Keep connection open briefly
		time.Sleep(500 * time.Millisecond)
		conn.Close(websocket.StatusNormalClosure, "done")
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := StreamRemote(ctx, wsURL, "")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-ch:
		if e.Role != "assistant" || e.Text != "test response" {
			t.Errorf("unexpected entry: %+v", e)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for entry")
	}
}
