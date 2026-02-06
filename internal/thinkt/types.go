// Package thinkt provides a unified interface for accessing AI coding assistant
// session storage (Kimi Code, Claude Code, etc.)
package thinkt

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

// Source identifies the AI coding assistant that created the data.
type Source string

const (
	SourceThinkt  Source = "thinkt"
	SourceClaude  Source = "claude"
	SourceCopilot Source = "copilot"
	SourceGemini  Source = "gemini"
	SourceKimi    Source = "kimi"
)

func (s Source) String() string {
	switch s {
	case SourceThinkt:
		return "thinkt"
	case SourceClaude:
		return "claude"
	case SourceCopilot:
		return "copilot"
	case SourceKimi:
		return "kimi"
	case SourceGemini:
		return "gemini"
	case "":
		return "unknown"
	default:
		return string(s)
	}
}

func (s Source) Description() string {
	switch s {
	case SourceThinkt:
		return "thinkt sessions"
	case SourceClaude:
		return "Claude Code sessions (~/.claude)"
	case SourceCopilot:
		return "GitHub Copilot sessions"
	case SourceKimi:
		return "Kimi Code sessions (~/.kimi)"
	case SourceGemini:
		return "Gemini CLI sessions"
	case "":
		return "unknown source"
	default:
		return string(s) + " sessions"
	}
}

//////////////////////////////////////////////////////////////////////////////

// Workspace identifies a machine/host where sessions originate.
// Like git repos, the same project path may exist on multiple workspaces
// (desktop, laptop, VMs, Fly.io sprites, etc.).
type Workspace struct {
	// ID is a unique identifier for this workspace.
	// Derived from device_id (Kimi), stable_id (Claude), or generated.
	ID string `json:"id"`

	// Name is a human-readable name for this workspace.
	// May be hostname, user-assigned name, or derived.
	Name string `json:"name,omitempty"`

	// Hostname is the machine's hostname if available.
	Hostname string `json:"hostname,omitempty"`

	// Source indicates which tool's storage this workspace represents.
	Source Source `json:"source"`

	// BasePath is the root storage path on this workspace
	// (e.g., ~/.kimi or ~/.claude).
	BasePath string `json:"base_path,omitempty"`
}

// Role identifies the type of message in a conversation.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleTool       Role = "tool"
	RoleSystem     Role = "system"
	RoleSummary    Role = "summary"
	RoleProgress   Role = "progress"
	RoleCheckpoint Role = "checkpoint" // State recovery markers (Kimi _checkpoint, Claude file-history-snapshot)
)

// ContentBlock represents a piece of content within a message.
// Different block types populate different fields.
type ContentBlock struct {
	Type string `json:"type"`

	// Text block
	Text string `json:"text,omitempty"`

	// Thinking block
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"` // For signed/encrypted thinking

	// Tool use block
	ToolUseID string `json:"tool_use_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolInput any    `json:"tool_input,omitempty"`

	// Tool result block
	ToolResult string `json:"tool_result,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`

	// Image/document block
	MediaType string `json:"media_type,omitempty"`
	MediaData string `json:"media_data,omitempty"`
}

// Entry represents a single turn in a conversation.
type Entry struct {
	// Core identification
	UUID       string    `json:"uuid"`
	ParentUUID *string   `json:"parent_uuid,omitempty"` // For branching conversations
	Role       Role      `json:"role"`
	Timestamp  time.Time `json:"timestamp"`

	// Provenance - where this entry came from
	Source        Source `json:"source,omitempty"`          // Which tool created this entry
	WorkspaceID   string `json:"workspace_id,omitempty"`   // Which machine/host
	AgentID       string `json:"agent_id,omitempty"`       // Resolved agent name (e.g., "researcher")
	SourceAgentID string `json:"source_agent_id,omitempty"` // Raw source identifier (e.g., "ab17e07")

	// Content (one of these will be set)
	ContentBlocks []ContentBlock `json:"content_blocks,omitempty"`
	Text          string         `json:"text,omitempty"` // Simple text shortcut

	// Metadata
	Model        string         `json:"model,omitempty"`
	Usage        *TokenUsage    `json:"usage,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"` // Source-specific extras
	GitBranch    string         `json:"git_branch,omitempty"`
	CWD          string         `json:"cwd,omitempty"`
	IsCheckpoint bool           `json:"is_checkpoint,omitempty"`
	IsSidechain  bool           `json:"is_sidechain,omitempty"` // For branched conversations
}

// TokenUsage represents token consumption for an entry.
type TokenUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// SessionMeta contains metadata about a session without loading full content.
type SessionMeta struct {
	ID          string    `json:"id"`
	ProjectPath string    `json:"project_path"` // Normalized project path
	FullPath    string    `json:"full_path"`    // Path to session file
	FirstPrompt string    `json:"first_prompt,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	EntryCount  int       `json:"entry_count"`
	FileSize    int64     `json:"file_size"` // Size in bytes
	CreatedAt   time.Time `json:"created_at"`
	ModifiedAt  time.Time `json:"modified_at"`
	GitBranch   string    `json:"git_branch,omitempty"`
	Model       string    `json:"model,omitempty"`
	Source      Source    `json:"source"`       // Which tool (kimi, claude)
	WorkspaceID string    `json:"workspace_id"` // Which machine/host
	ChunkCount  int       `json:"chunk_count"`  // Number of files: 0=unknown, 1=single, 2+=chunked
}

// Session represents a complete conversation session.
type Session struct {
	Meta    SessionMeta `json:"meta"`
	Entries []Entry     `json:"entries"`
}

// Project represents a working directory containing multiple sessions.
// The same project path may exist on multiple workspaces.
type Project struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`         // Display name
	Path         string    `json:"path"`         // Full filesystem path
	DisplayPath  string    `json:"display_path"` // Human-readable path
	SessionCount int       `json:"session_count"`
	LastModified time.Time `json:"last_modified"`
	Source       Source    `json:"source"`       // Which tool
	WorkspaceID  string    `json:"workspace_id"` // Which machine/host
	PathExists   bool      `json:"path_exists"`  // Whether the project directory still exists on disk
}

// Store provides access to projects and sessions from a single workspace.
type Store interface {
	// Source returns the type of this store (kimi, claude).
	Source() Source

	// Workspace returns information about this store's workspace (machine/host).
	Workspace() Workspace

	// ListProjects returns all available projects.
	ListProjects(ctx context.Context) ([]Project, error)

	// GetProject returns a specific project by ID or path.
	GetProject(ctx context.Context, id string) (*Project, error)

	// ListSessions returns sessions for a project.
	ListSessions(ctx context.Context, projectID string) ([]SessionMeta, error)

	// GetSessionMeta returns session metadata without loading entries.
	GetSessionMeta(ctx context.Context, sessionID string) (*SessionMeta, error)

	// LoadSession loads a complete session with all entries.
	// Use with caution on large sessions.
	LoadSession(ctx context.Context, sessionID string) (*Session, error)

	// OpenSession returns a reader for streaming session entries.
	// Preferred for large sessions or when only partial access is needed.
	OpenSession(ctx context.Context, sessionID string) (SessionReader, error)
}

// SessionReader provides streaming access to session entries.
// Implementations may use lazy loading for efficiency.
type SessionReader interface {
	// ReadNext returns the next entry, or io.EOF when done.
	ReadNext() (*Entry, error)

	// Metadata returns session metadata.
	Metadata() SessionMeta

	// Close releases any resources.
	Close() error
}

// EntryIterator is a callback-based iterator for entries.
type EntryIterator func(entry Entry) bool // return false to stop iteration

// LazySession extends SessionReader with windowed loading capabilities.
type LazySession interface {
	// Metadata returns the session metadata.
	Metadata() SessionMeta

	// Entries returns all currently loaded entries.
	Entries() []Entry

	// EntryCount returns the number of loaded entries.
	EntryCount() int

	// HasMore returns true if there are more entries to load.
	HasMore() bool

	// LoadMore loads additional entries up to a content limit.
	// Returns the number of new entries loaded.
	LoadMore(maxContentBytes int) (int, error)

	// LoadAll loads all remaining entries.
	LoadAll() error

	// Progress returns loading progress (0.0 to 1.0).
	Progress() float64

	// Close releases any resources.
	Close() error
}

// Prompt represents an extracted user prompt with metadata.
type Prompt struct {
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	UUID      string    `json:"uuid"`
}

// SessionFilter provides criteria for filtering sessions.
type SessionFilter struct {
	ProjectPath string
	GitBranch   string
	Model       string
	Source      Source // Filter by tool (kimi, claude)
	WorkspaceID string // Filter by workspace
	After       *time.Time
	Before      *time.Time
	Limit       int
}

// StoreRegistry manages multiple stores (Kimi, Claude, etc.)
// and their optional team stores.
type StoreRegistry struct {
	stores     map[Source]Store
	teamStores map[Source]TeamStore
	mu         sync.RWMutex
}

// NewRegistry creates a new store registry.
func NewRegistry() *StoreRegistry {
	return &StoreRegistry{
		stores:     make(map[Source]Store),
		teamStores: make(map[Source]TeamStore),
	}
}

// Register adds a store to the registry.
func (r *StoreRegistry) Register(store Store) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stores[store.Source()] = store
}

// RegisterTeamStore adds a team store to the registry.
func (r *StoreRegistry) RegisterTeamStore(ts TeamStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teamStores[ts.Source()] = ts
}

// Get returns a store by source type.
func (r *StoreRegistry) Get(source Source) (Store, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.stores[source]
	return s, ok
}

// GetTeamStore returns a team store by source type.
func (r *StoreRegistry) GetTeamStore(source Source) (TeamStore, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ts, ok := r.teamStores[source]
	return ts, ok
}

// TeamStores returns all registered team stores.
func (r *StoreRegistry) TeamStores() []TeamStore {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]TeamStore, 0, len(r.teamStores))
	for _, ts := range r.teamStores {
		result = append(result, ts)
	}
	return result
}

// All returns all registered stores.
func (r *StoreRegistry) All() []Store {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Store, 0, len(r.stores))
	for _, s := range r.stores {
		result = append(result, s)
	}
	return result
}

// Sources returns all registered source types.
func (r *StoreRegistry) Sources() []Source {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Source, 0, len(r.stores))
	for s := range r.stores {
		result = append(result, s)
	}
	return result
}

// AvailableSources returns sources that have data (projects/sessions).
func (r *StoreRegistry) AvailableSources(ctx context.Context) []Source {
	var available []Source
	for _, store := range r.All() {
		projects, err := store.ListProjects(ctx)
		if err == nil && len(projects) > 0 {
			available = append(available, store.Source())
		}
	}
	return available
}

// ListAllProjects returns projects from all registered stores.
// It checks whether each project's directory still exists on disk.
func (r *StoreRegistry) ListAllProjects(ctx context.Context) ([]Project, error) {
	var all []Project
	for _, store := range r.All() {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue // Log error but don't fail entirely
		}
		for i := range projects {
			_, err := os.Stat(projects[i].Path)
			projects[i].PathExists = err == nil
		}
		all = append(all, projects...)
	}
	return all, nil
}

// SourceInfo provides information about a source for display.
type SourceInfo struct {
	Source       Source `json:"source"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Available    bool   `json:"available"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	BasePath     string `json:"base_path,omitempty"`
	ProjectCount int    `json:"project_count,omitempty"`
}

// FindProjectForPath returns the project whose Path matches or contains the given path.
// If multiple projects match (e.g., nested directories), returns the most specific match
// (longest path). Returns nil if no matching project is found.
//
// This is useful for CLI commands that want to automatically scope to the current
// working directory, similar to how git commands work within a repository.
func (r *StoreRegistry) FindProjectForPath(ctx context.Context, path string) *Project {
	projects, err := r.ListAllProjects(ctx)
	if err != nil {
		return nil
	}

	var best *Project
	for _, p := range projects {
		// Check if the given path is within this project's path
		if strings.HasPrefix(path, p.Path) {
			// Ensure it's a proper path prefix (not just a string prefix)
			// e.g., /foo/bar should not match /foo/barbaz
			if len(path) == len(p.Path) || path[len(p.Path)] == '/' {
				if best == nil || len(p.Path) > len(best.Path) {
					match := p // Create a copy to avoid loop variable issues
					best = &match
				}
			}
		}
	}
	return best
}

// SourceStatus returns status information for all registered sources.
func (r *StoreRegistry) SourceStatus(ctx context.Context) []SourceInfo {
	var infos []SourceInfo
	for _, store := range r.All() {
		ws := store.Workspace()
		info := SourceInfo{
			Source:   store.Source(),
			Name:     string(store.Source()),
			BasePath: ws.BasePath,
		}

		// Get project count to determine availability
		projects, err := store.ListProjects(ctx)
		if err == nil {
			info.Available = len(projects) > 0
			info.ProjectCount = len(projects)
			info.WorkspaceID = ws.ID
		}

		// Add descriptions
		info.Name = store.Source().String()

		infos = append(infos, info)
	}
	return infos
}

// EntryWriter writes entries to an output format (for export/conversion).
type EntryWriter interface {
	WriteEntry(entry Entry) error
	Flush() error
}

// EntryReader reads entries from a source (for import/conversion).
type EntryReader interface {
	ReadEntry() (*Entry, error)
}

// Converter converts between storage formats.
type Converter interface {
	Convert(src EntryReader, dst EntryWriter) error
}

//////////////////////////////////////////////////////////////////////////////
// Team types — for multi-agent coordination (Claude Code teams/swarms)
//////////////////////////////////////////////////////////////////////////////

// TeamStatus indicates whether a team is currently active or historically discovered.
type TeamStatus string

const (
	TeamStatusActive   TeamStatus = "active"
	TeamStatusInactive TeamStatus = "inactive"
)

// Team represents a Claude Code team (agent swarm).
// Teams are an overlay concept — they reference agents whose sessions
// already exist in the source (Claude) storage.
type Team struct {
	// Name is the team identifier (directory name under ~/.claude/teams/).
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`

	// LeadAgentID is the config-format agent ID (e.g., "team-lead@myteam").
	LeadAgentID string `json:"lead_agent_id"`
	// LeadSessionID is the UUID of the team lead's session.
	// Subagent sessions live under projects/{projectDir}/{LeadSessionID}/subagents/
	LeadSessionID string `json:"lead_session_id"`

	Members []TeamMember `json:"members"`

	// Source provenance
	Source      Source     `json:"source"`
	WorkspaceID string     `json:"workspace_id,omitempty"`
	Status      TeamStatus `json:"status,omitempty"`
}

// TeamMember represents a member of a team.
type TeamMember struct {
	// Name is the short display name (e.g., "researcher").
	Name string `json:"name"`
	// AgentID is the config-format ID (e.g., "researcher@myteam").
	AgentID string `json:"agent_id"`
	// SourceAgentID is the short hash used in session JSONL entries
	// (e.g., "ab17e07"). Populated when correlating team config with
	// session data. May be empty if not yet resolved.
	SourceAgentID string `json:"source_agent_id,omitempty"`
	// AgentType classifies the member (e.g., "team-lead", "general-purpose").
	AgentType string `json:"agent_type"`
	Model     string `json:"model,omitempty"`
	JoinedAt  time.Time `json:"joined_at"`
	CWD       string `json:"cwd,omitempty"`
	Color     string `json:"color,omitempty"`
	// SessionPath is the path to this member's subagent JSONL file.
	// Populated during discovery. Empty for unresolved members.
	SessionPath string `json:"session_path,omitempty"`
}

// TeamTask represents a task in the team's shared task board.
type TeamTask struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	ActiveForm  string   `json:"active_form,omitempty"`
	Status      string   `json:"status"` // "pending", "in_progress", "completed", "deleted"
	Owner       string   `json:"owner,omitempty"`
	Blocks      []string `json:"blocks,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	IsInternal  bool     `json:"is_internal,omitempty"`
}

// TeamMessage represents a message in a team member's inbox.
type TeamMessage struct {
	From      string    `json:"from"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	Color     string    `json:"color,omitempty"`
	Read      bool      `json:"read"`
}
