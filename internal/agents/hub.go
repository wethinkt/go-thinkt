package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wethinkt/go-thinkt/internal/fingerprint"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// DefaultPollInterval is how often the hub refreshes its agent list.
const DefaultPollInterval = 5 * time.Second

// HubConfig configures the AgentHub.
type HubConfig struct {
	CollectorURLs []string
	PollInterval  time.Duration
	Detector      *thinkt.ActiveSessionDetector
}

// AgentHub merges local and remote agent detection into a single queryable interface.
type AgentHub struct {
	config      HubConfig
	detector    *thinkt.ActiveSessionDetector
	localFP     string
	mu          sync.RWMutex
	agents      []UnifiedAgent
	subscribers []*hubSubscriber
}

type hubSubscriber struct {
	ch     chan AgentEvent
	closed bool
}

// NewHub creates an AgentHub. Call Run() to start background polling.
func NewHub(cfg HubConfig) *AgentHub {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultPollInterval
	}

	localFP, _ := fingerprint.GetFingerprint()

	h := &AgentHub{
		config:   cfg,
		detector: cfg.Detector,
		localFP:  localFP,
	}

	return h
}

// LocalFingerprint returns this machine's fingerprint.
func (h *AgentHub) LocalFingerprint() string {
	return h.localFP
}

// List returns agents matching the filter. Reads from cache, never blocks on network.
func (h *AgentHub) List(filter AgentFilter) []UnifiedAgent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []UnifiedAgent
	for _, a := range h.agents {
		if filter.Matches(a, h.localFP) {
			result = append(result, a)
		}
	}
	return result
}

// FindBySessionID returns the agent with the given session ID, if any.
func (h *AgentHub) FindBySessionID(sessionID string) (UnifiedAgent, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, a := range h.agents {
		if a.SessionID == sessionID || a.ID == sessionID {
			return a, true
		}
	}
	return UnifiedAgent{}, false
}

// Subscribe returns a channel of agent events and an unsubscribe function.
func (h *AgentHub) Subscribe() (<-chan AgentEvent, func()) {
	ch := make(chan AgentEvent, 64)
	sub := &hubSubscriber{ch: ch}

	h.mu.Lock()
	h.subscribers = append(h.subscribers, sub)
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for i, s := range h.subscribers {
			if s == sub {
				h.subscribers = append(h.subscribers[:i], h.subscribers[i+1:]...)
				if !s.closed {
					s.closed = true
					close(s.ch)
				}
				break
			}
		}
	}
}

// notify sends an event to all subscribers. Must be called with h.mu held.
func (h *AgentHub) notify(event AgentEvent) {
	for _, sub := range h.subscribers {
		if sub.closed {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// Skip slow subscribers
		}
	}
}

// Stream opens a live entry stream for the given agent.
// Local agents are tailed from the filesystem; remote agents stream via WebSocket.
func (h *AgentHub) Stream(ctx context.Context, agentID string) (<-chan StreamEntry, error) {
	agent, ok := h.FindBySessionID(agentID)
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	if agent.IsLocal(h.localFP) && agent.SessionPath != "" {
		return StreamLocal(ctx, agent.SessionPath)
	}

	if agent.CollectorURL != "" {
		wsURL := agent.CollectorURL + "/v1/sessions/" + agent.SessionID + "/ws"
		// TODO: pass auth token from config
		return StreamRemote(ctx, wsURL, "")
	}

	return nil, fmt.Errorf("no stream source for agent %s", agentID)
}

// Run starts background polling and blocks until ctx is cancelled.
func (h *AgentHub) Run(ctx context.Context) error {
	// Initial poll
	h.poll(ctx)

	ticker := time.NewTicker(h.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			h.poll(ctx)
		}
	}
}

// PollOnce runs a single poll cycle. Useful for CLI commands that don't need
// continuous polling.
func (h *AgentHub) PollOnce(ctx context.Context) {
	h.poll(ctx)
}

func (h *AgentHub) poll(ctx context.Context) {
	var newAgents []UnifiedAgent

	// Local detection
	if h.detector != nil {
		locals, err := h.detector.Detect(ctx)
		if err != nil {
			tuilog.Log.Warn("Local agent detection failed", "error", err)
		}
		hostname, _ := os.Hostname()
		for _, l := range locals {
			newAgents = append(newAgents, UnifiedAgent{
				ID:          l.SessionID,
				Source:      string(l.Source),
				ProjectPath: l.ProjectPath,
				SessionID:   l.SessionID,
				Hostname:    hostname,
				Status:      "active",
				DetectedAt:  l.DetectedAt,
				LastSeen:    l.DetectedAt,
				MachineID:   h.localFP,
				MachineName: hostname,
				Method:      l.Method,
				IDE:         l.IDE,
				PID:         l.PID,
				SessionPath: l.SessionPath,
			})
		}
	}

	// Remote collectors
	for _, url := range h.config.CollectorURLs {
		remotes := h.fetchRemoteAgents(ctx, url)
		newAgents = append(newAgents, remotes...)
	}

	// Diff and emit events
	h.mu.Lock()
	oldMap := make(map[string]UnifiedAgent)
	for _, a := range h.agents {
		oldMap[a.ID] = a
	}
	newMap := make(map[string]UnifiedAgent)
	for _, a := range newAgents {
		newMap[a.ID] = a
	}

	for id, a := range newMap {
		if _, existed := oldMap[id]; !existed {
			h.notify(AgentEvent{Type: "added", Agent: a})
		} else {
			h.notify(AgentEvent{Type: "updated", Agent: a})
		}
	}
	for id, a := range oldMap {
		if _, exists := newMap[id]; !exists {
			h.notify(AgentEvent{Type: "removed", Agent: a})
		}
	}

	h.agents = newAgents
	h.mu.Unlock()
}

func (h *AgentHub) fetchRemoteAgents(ctx context.Context, collectorURL string) []UnifiedAgent {
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", collectorURL+"/v1/agents", nil)
	if err != nil {
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		tuilog.Log.Warn("Failed to fetch remote agents", "url", collectorURL, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var agents []struct {
		InstanceID    string         `json:"instance_id"`
		Platform      string         `json:"platform"`
		Region        string         `json:"region"`
		Hostname      string         `json:"hostname"`
		Version       string         `json:"version"`
		MachineID     string         `json:"machine_id"`
		StartedAt     time.Time      `json:"started_at"`
		LastHeartbeat time.Time      `json:"last_heartbeat"`
		TraceCount    int64          `json:"trace_count"`
		Project       string         `json:"project"`
		Status        string         `json:"status"`
		Metadata      map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil
	}

	var result []UnifiedAgent
	for _, a := range agents {
		result = append(result, UnifiedAgent{
			ID:           a.InstanceID,
			Source:       a.Platform,
			ProjectPath:  a.Project,
			SessionID:    a.InstanceID, // remote agents use instance ID as session
			Hostname:     a.Hostname,
			Status:       a.Status,
			DetectedAt:   a.StartedAt,
			LastSeen:     a.LastHeartbeat,
			MachineID:    a.MachineID,
			MachineName:  a.Hostname,
			InstanceID:   a.InstanceID,
			Region:       a.Region,
			Version:      a.Version,
			TraceCount:   a.TraceCount,
			CollectorURL: collectorURL,
		})
	}
	return result
}
