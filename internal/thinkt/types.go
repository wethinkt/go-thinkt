// Package thinkt provides a unified interface for accessing AI coding assistant
// session storage (Kimi Code, Claude Code, etc.)
package thinkt

import (
	"context"
	"time"
)

// Source identifies the AI coding assistant that created the data.
type Source string

const (
	SourceKimi   Source = "kimi"
	SourceClaude Source = "claude"
)

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
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
	RoleSummary   Role = "summary"
	RoleProgress  Role = "progress"
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
	Source      Source `json:"source,omitempty"`       // Which tool created this entry
	WorkspaceID string `json:"workspace_id,omitempty"` // Which machine/host

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
	ID           string    `json:"id"`
	ProjectPath  string    `json:"project_path"` // Normalized project path
	FullPath     string    `json:"full_path"`    // Path to session file
	FirstPrompt  string    `json:"first_prompt,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	EntryCount   int       `json:"entry_count"`
	CreatedAt    time.Time `json:"created_at"`
	ModifiedAt   time.Time `json:"modified_at"`
	GitBranch    string    `json:"git_branch,omitempty"`
	Model        string    `json:"model,omitempty"`
	Source       Source    `json:"source"`       // Which tool (kimi, claude)
	WorkspaceID  string    `json:"workspace_id"` // Which machine/host
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
	Name         string    `json:"name"`          // Display name
	Path         string    `json:"path"`          // Full filesystem path
	DisplayPath  string    `json:"display_path"`  // Human-readable path
	SessionCount int       `json:"session_count"`
	LastModified time.Time `json:"last_modified"`
	Source       Source    `json:"source"`       // Which tool
	WorkspaceID  string    `json:"workspace_id"` // Which machine/host
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
	SessionReader

	// HasMore returns true if there are more entries to load.
	HasMore() bool

	// LoadMore loads additional entries up to a content limit.
	// Returns the number of new entries loaded.
	LoadMore(maxContentBytes int) (int, error)

	// LoadAll loads all remaining entries.
	LoadAll() error

	// Progress returns loading progress (0.0 to 1.0).
	Progress() float64
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
	Source      Source     // Filter by tool (kimi, claude)
	WorkspaceID string     // Filter by workspace
	After       *time.Time
	Before      *time.Time
	Limit       int
}

// StoreRegistry manages multiple stores (Kimi, Claude, etc.).
type StoreRegistry struct {
	stores map[Source]Store
}

// NewRegistry creates a new store registry.
func NewRegistry() *StoreRegistry {
	return &StoreRegistry{
		stores: make(map[Source]Store),
	}
}

// Register adds a store to the registry.
func (r *StoreRegistry) Register(store Store) {
	r.stores[store.Source()] = store
}

// Get returns a store by source type.
func (r *StoreRegistry) Get(source Source) (Store, bool) {
	s, ok := r.stores[source]
	return s, ok
}

// All returns all registered stores.
func (r *StoreRegistry) All() []Store {
	result := make([]Store, 0, len(r.stores))
	for _, s := range r.stores {
		result = append(result, s)
	}
	return result
}

// ListAllProjects returns projects from all registered stores.
func (r *StoreRegistry) ListAllProjects(ctx context.Context) ([]Project, error) {
	var all []Project
	for _, store := range r.stores {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue // Log error but don't fail entirely
		}
		all = append(all, projects...)
	}
	return all, nil
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
