package collect

import (
	"fmt"
	"strings"
	"time"
)

// validRoles is the set of accepted entry roles.
var validRoles = map[string]bool{
	"user":      true,
	"assistant": true,
	"tool_use":  true,
	"tool_result": true,
	"system":    true,
}

// NormalizeRequest validates and cleans an ingest request in place.
// It returns an error if the request is fundamentally invalid.
// Individual entries that fail validation are removed and their count returned.
func NormalizeRequest(req *IngestRequest) (dropped int, err error) {
	if req.SessionID == "" {
		return 0, fmt.Errorf("session_id is required")
	}
	if req.Source == "" {
		return 0, fmt.Errorf("source is required")
	}
	if len(req.Entries) == 0 {
		return 0, fmt.Errorf("entries must not be empty")
	}

	req.Source = strings.ToLower(strings.TrimSpace(req.Source))
	req.ProjectPath = strings.TrimSpace(req.ProjectPath)
	req.InstanceID = strings.TrimSpace(req.InstanceID)

	valid := make([]IngestEntry, 0, len(req.Entries))
	for i := range req.Entries {
		e := &req.Entries[i]
		if err := normalizeEntry(e); err != nil {
			dropped++
			continue
		}
		valid = append(valid, *e)
	}
	req.Entries = valid
	return dropped, nil
}

// normalizeEntry validates and cleans a single entry.
func normalizeEntry(e *IngestEntry) error {
	if e.UUID == "" {
		return fmt.Errorf("entry uuid is required")
	}
	e.Role = strings.ToLower(strings.TrimSpace(e.Role))
	if !validRoles[e.Role] {
		return fmt.Errorf("invalid role: %q", e.Role)
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	e.Model = strings.TrimSpace(e.Model)
	e.ToolName = strings.TrimSpace(e.ToolName)
	if e.InputTokens < 0 {
		e.InputTokens = 0
	}
	if e.OutputTokens < 0 {
		e.OutputTokens = 0
	}
	if e.ThinkingLen < 0 {
		e.ThinkingLen = 0
	}

	// Derive classification flags if not already set by the exporter.
	// This ensures older exporters that don't send these flags still get
	// correct classification from whatever data they do send.
	if !e.HasThinking && e.ThinkingLen > 0 {
		e.HasThinking = true
	}
	if !e.HasToolUse && e.ToolName != "" {
		e.HasToolUse = true
	}
	return nil
}
