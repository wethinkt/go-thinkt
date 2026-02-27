package collect

import (
	"sync"
	"time"
)

// StaleAgentThreshold is how long without a heartbeat before an agent is stale.
const StaleAgentThreshold = 5 * time.Minute

// AgentRegistry tracks registered exporter agents and their heartbeats.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentInfo
}

// NewAgentRegistry creates a new in-memory agent registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*AgentInfo),
	}
}

// Register adds or updates an agent in the registry. Returns the agent info.
func (r *AgentRegistry) Register(reg AgentRegistration) *AgentInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	info, exists := r.agents[reg.InstanceID]
	if exists {
		// Update existing registration
		info.Platform = reg.Platform
		info.Region = reg.Region
		info.Hostname = reg.Hostname
		info.Version = reg.Version
		info.Project = reg.Project
		info.MachineID = reg.MachineID
		info.LastHeartbeat = now
		info.Status = "active"
		if reg.Metadata != nil {
			info.Metadata = reg.Metadata
		}
		return info
	}

	info = &AgentInfo{
		InstanceID:    reg.InstanceID,
		Platform:      reg.Platform,
		Region:        reg.Region,
		Hostname:      reg.Hostname,
		Version:       reg.Version,
		StartedAt:     reg.StartedAt,
		LastHeartbeat: now,
		Project:       reg.Project,
		Status:        "active",
		MachineID:     reg.MachineID,
		Metadata:      reg.Metadata,
	}
	r.agents[reg.InstanceID] = info
	return info
}

// Heartbeat updates the last heartbeat time for an agent.
// If the agent isn't registered, creates a minimal entry so actively
// shipping exporters stay visible even without explicit registration.
func (r *AgentRegistry) Heartbeat(instanceID string) bool {
	if instanceID == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	info, ok := r.agents[instanceID]
	if !ok {
		r.agents[instanceID] = &AgentInfo{
			InstanceID:    instanceID,
			LastHeartbeat: now,
			Status:        "active",
		}
		return false
	}
	info.LastHeartbeat = now
	info.Status = "active"
	return true
}

// IncrementTraceCount adds to the trace count for an agent.
func (r *AgentRegistry) IncrementTraceCount(instanceID string, count int64) {
	if instanceID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	info, ok := r.agents[instanceID]
	if !ok {
		r.agents[instanceID] = &AgentInfo{
			InstanceID:    instanceID,
			LastHeartbeat: now,
			TraceCount:    count,
			Status:        "active",
		}
		return
	}
	info.TraceCount += count
	info.LastHeartbeat = now
	info.Status = "active"
}

// List returns a snapshot of all registered agents with current status.
func (r *AgentRegistry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	result := make([]AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		cp := *info
		if now.Sub(cp.LastHeartbeat) > StaleAgentThreshold {
			cp.Status = "stale"
		}
		result = append(result, cp)
	}
	return result
}

// CleanStale removes agents that have not sent a heartbeat within maxAge.
// Returns the number of agents removed.
func (r *AgentRegistry) CleanStale(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	removed := 0
	for id, info := range r.agents {
		if now.Sub(info.LastHeartbeat) > maxAge {
			delete(r.agents, id)
			removed++
		}
	}
	return removed
}

// Count returns the total and active agent counts.
func (r *AgentRegistry) Count() (total, active int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	total = len(r.agents)
	for _, info := range r.agents {
		if now.Sub(info.LastHeartbeat) <= StaleAgentThreshold {
			active++
		}
	}
	return total, active
}
