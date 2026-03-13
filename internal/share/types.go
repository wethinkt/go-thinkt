package share

// Session represents a shared session (maps to OpenAPI Session schema).
type Session struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Title            string   `json:"title"`
	Visibility       string   `json:"visibility"`
	SizeBytes        int      `json:"size_bytes"`
	LikesCount       int      `json:"likes_count"`
	CreatedAt        string   `json:"created_at"`
	Tags             []string `json:"tags"`
	OwnerName        string   `json:"owner_name,omitempty"`
	Liked            bool     `json:"liked,omitempty"`
	Favorited        bool     `json:"favorited,omitempty"`
	ModerationStatus string   `json:"moderation_status,omitempty"`
	GitCommit        string   `json:"git_commit,omitempty"`
	GitRepoURL       string   `json:"git_repo_url,omitempty"`
	WorkspaceID      string   `json:"workspace_id,omitempty"`
	GitBranch        string   `json:"git_branch,omitempty"`
	GitTag           string   `json:"git_tag,omitempty"`
	Platform         string   `json:"platform,omitempty"`
}

type SessionList struct {
	Sessions []Session `json:"traces"`
}

type ExploreResponse struct {
	Sessions []Session `json:"traces"`
	Page     int       `json:"page"`
}

type Profile struct {
	User  ProfileUser  `json:"user"`
	Stats ProfileStats `json:"stats"`
	Likes []SessionLike `json:"likes"`
	Tags  []TagCount   `json:"tags"`
}

type ProfileUser struct {
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	Image           *string `json:"image"`
	TermsAcceptedAt *string `json:"terms_accepted_at"`
}

type ProfileStats struct {
	TotalSessions   int `json:"total_sessions"`
	PublicSessions  int `json:"public_sessions"`
	PrivateSessions int `json:"private_sessions"`
	TotalBytes      int `json:"total_bytes"`
}

type SessionLike struct {
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	OwnerName string `json:"owner_name"`
	CreatedAt string `json:"created_at"`
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type TagCloud struct {
	Tags []TagCount `json:"tags"`
}

type LikeResponse struct {
	Liked      bool `json:"liked"`
	LikesCount int  `json:"likes_count"`
}

type FavoriteResponse struct {
	Favorited bool `json:"favorited"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
