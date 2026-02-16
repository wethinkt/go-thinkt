package thinkt

// ResumeInfo describes how to resume (exec into) a session's original CLI tool.
type ResumeInfo struct {
	Command string   // absolute path to binary
	Args    []string // argv (including argv[0])
	Dir     string   // working directory to run in (empty = current)
}

// SessionResumer is optionally implemented by Store implementations that
// support resuming a session in the original CLI tool (e.g., claude --resume).
type SessionResumer interface {
	ResumeCommand(session SessionMeta) (*ResumeInfo, error)
}
