package agents

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

const (
	maxReconnectDelay   = 30 * time.Second
	baseReconnectDelay  = 1 * time.Second
	maxConsecutiveFails = 5
)

// StreamRemote connects to a collector WebSocket endpoint and streams entries.
// It reconnects automatically on disconnect with exponential backoff.
// The token is sent as a Bearer Authorization header (for non-browser clients).
func StreamRemote(ctx context.Context, wsURL string, token string) (<-chan StreamEntry, error) {
	ch := make(chan StreamEntry, 64)
	go streamRemoteLoop(ctx, wsURL, token, ch)
	return ch, nil
}

func streamRemoteLoop(ctx context.Context, wsURL string, token string, ch chan<- StreamEntry) {
	defer close(ch)

	var lastTimestamp time.Time
	consecutiveFails := 0

	for {
		if ctx.Err() != nil {
			return
		}

		err := streamRemoteOnce(ctx, wsURL, token, lastTimestamp, ch, &lastTimestamp)
		if ctx.Err() != nil {
			return
		}

		consecutiveFails++
		if err != nil {
			tuilog.Log.Warn("WebSocket stream disconnected", "error", err, "failures", consecutiveFails)
		}

		if consecutiveFails >= maxConsecutiveFails {
			select {
			case ch <- StreamEntry{
				Timestamp: time.Now(),
				Role:      "system",
				Text:      "Connection lost, retrying...",
				Synthetic: true,
			}:
			case <-ctx.Done():
				return
			}
		}

		// Exponential backoff
		delay := time.Duration(float64(baseReconnectDelay) * math.Pow(2, float64(min(consecutiveFails-1, 5))))
		if delay > maxReconnectDelay {
			delay = maxReconnectDelay
		}

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}
}

func streamRemoteOnce(ctx context.Context, wsURL string, token string, after time.Time, ch chan<- StreamEntry, lastTS *time.Time) error {
	url := wsURL
	if !after.IsZero() {
		url += "?after=" + after.Format(time.RFC3339Nano)
	}

	opts := &websocket.DialOptions{}
	if token != "" {
		opts.HTTPHeader = http.Header{
			"Authorization": []string{"Bearer " + token},
		}
	}

	conn, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}

		var entry StreamEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			tuilog.Log.Debug("Failed to parse WS entry", "error", err)
			continue
		}

		if !entry.Timestamp.IsZero() {
			*lastTS = entry.Timestamp
		}

		select {
		case ch <- entry:
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "client closing")
			return ctx.Err()
		}
	}
}
