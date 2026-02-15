package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

const (
	maxRetries     = 3
	initialBackoff = 1 * time.Second
	shipTimeout    = 30 * time.Second
)

// Shipper sends trace payloads to a remote collector via HTTP POST.
type Shipper struct {
	collectorURL string
	apiKey       string
	client       *http.Client
}

// NewShipper creates a new Shipper targeting the given collector URL.
func NewShipper(collectorURL, apiKey string) *Shipper {
	return &Shipper{
		collectorURL: collectorURL,
		apiKey:       apiKey,
		client: &http.Client{
			Timeout: shipTimeout,
		},
	}
}

// Ship sends a TracePayload to the collector with retry and exponential backoff.
func (s *Shipper) Ship(ctx context.Context, payload TracePayload) (*ShipResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	start := time.Now()
	var lastErr error
	var statusCode int

	backoff := initialBackoff
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			tuilog.Log.Debug("Retrying ship", "attempt", attempt, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return &ShipResult{
					Entries:    len(payload.Entries),
					StatusCode: 0,
					Error:      ctx.Err(),
					Duration:   time.Since(start),
				}, ctx.Err()
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.collectorURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if s.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.apiKey)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		statusCode = resp.StatusCode
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if statusCode >= 200 && statusCode < 300 {
			result := &ShipResult{
				Entries:    len(payload.Entries),
				StatusCode: statusCode,
				Duration:   time.Since(start),
			}
			tuilog.Log.Debug("Ship succeeded",
				"entries", result.Entries,
				"status", statusCode,
				"duration", result.Duration,
			)
			return result, nil
		}

		// Don't retry on client errors (4xx) except 429
		if statusCode >= 400 && statusCode < 500 && statusCode != http.StatusTooManyRequests {
			lastErr = fmt.Errorf("collector returned %d", statusCode)
			break
		}

		lastErr = fmt.Errorf("collector returned %d", statusCode)
	}

	result := &ShipResult{
		Entries:    len(payload.Entries),
		StatusCode: statusCode,
		Error:      lastErr,
		Duration:   time.Since(start),
	}
	return result, lastErr
}

// Ping checks if the collector is reachable by sending a GET to the collector URL.
func (s *Shipper) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.collectorURL, nil)
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}

	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("ping collector: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("collector unhealthy: %d", resp.StatusCode)
	}

	return nil
}
