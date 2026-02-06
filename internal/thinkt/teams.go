package thinkt

import "context"

// TeamStore provides access to team configuration and coordination data.
// This is separate from Store because teams are an overlay concept —
// they reference agents whose sessions exist in an underlying Store.
type TeamStore interface {
	// Source returns which source this team store is for.
	Source() Source

	// ListTeams returns all discovered teams.
	ListTeams(ctx context.Context) ([]Team, error)

	// GetTeam returns a team by name, with resolved member-to-session mappings.
	GetTeam(ctx context.Context, name string) (*Team, error)

	// GetTeamTasks returns the shared task board for a team.
	GetTeamTasks(ctx context.Context, teamName string) ([]TeamTask, error)

	// GetTeamMessages returns inbox messages for a team member.
	GetTeamMessages(ctx context.Context, teamName, memberName string) ([]TeamMessage, error)
}

// TeamStoreFactory is an optional interface that StoreFactory implementations
// can satisfy to indicate they support team/multi-agent discovery.
// During source discovery, factories that implement this interface will
// have their team stores created and registered automatically.
type TeamStoreFactory interface {
	// CreateTeamStore creates a TeamStore for this source.
	// Returns (nil, nil) if the source supports teams but none are available.
	CreateTeamStore() (TeamStore, error)
}
