package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Project represents a Claude Code project directory.
type Project struct {
	DirName      string    // Raw directory name (e.g., "-Users-evan-brainstm-foo")
	DisplayName  string    // Human-readable name (e.g., "foo")
	FullPath     string    // Decoded project path (e.g., "/Users/evan/brainstm/foo")
	DirPath      string    // Absolute path to the project directory
	SessionCount int       // Number of JSONL session files
	LastModified time.Time // Most recent session modification time
}

// SessionMeta holds lightweight session metadata without loading the full JSONL.
type SessionMeta struct {
	SessionID    string    `json:"sessionId"`
	FullPath     string    `json:"fullPath"`
	FirstPrompt  string    `json:"firstPrompt"`
	Summary      string    `json:"summary"`
	MessageCount int       `json:"messageCount"`
	Created      time.Time `json:"-"`
	Modified     time.Time `json:"-"`
	GitBranch    string    `json:"gitBranch"`
	ProjectPath  string    `json:"projectPath"`
	FileSize     int64     `json:"-"` // File size in bytes
}

// sessionMetaJSON is used for unmarshaling the sessions-index.json entries.
type sessionMetaJSON struct {
	SessionMeta
	CreatedStr  string `json:"created"`
	ModifiedStr string `json:"modified"`
	FileMtime   int64  `json:"fileMtime"`
}

// sessionsIndex represents the sessions-index.json file format.
type sessionsIndex struct {
	Version      int               `json:"version"`
	Entries      []sessionMetaJSON `json:"entries"`
	OriginalPath string            `json:"originalPath"`
}

// DecodeDirName converts a Claude Code hashed directory name to a human-readable
// display name and decoded full path. The directory name format replaces "/" with "-",
// e.g., "-Users-evan-brainstm-foo" decodes to "/Users/evan/brainstm/foo".
func DecodeDirName(dirName string) (displayName string, fullPath string) {
	if dirName == "-" {
		return "~", ""
	}

	// Leading "-" maps to "/"
	if strings.HasPrefix(dirName, "-") {
		fullPath = "/" + strings.ReplaceAll(dirName[1:], "-", "/")
	} else {
		fullPath = strings.ReplaceAll(dirName, "-", "/")
	}

	displayName = filepath.Base(fullPath)
	return displayName, fullPath
}

// ListProjects returns all project directories with decoded display names.
// If baseDir is empty, the default (~/.claude) is used.
func ListProjects(baseDir string) ([]Project, error) {
	projectsDir, err := ProjectsDir(baseDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Get home directory to filter it out
	homeDir, _ := os.UserHomeDir()

	var projects []Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(projectsDir, entry.Name())
		displayName, fullPath := DecodeDirName(entry.Name())

		// Check for sessions-index.json to get the real path
		indexPath := filepath.Join(dirPath, "sessions-index.json")
		if data, err := os.ReadFile(indexPath); err == nil {
			var idx sessionsIndex
			if json.Unmarshal(data, &idx) == nil && idx.OriginalPath != "" {
				fullPath = idx.OriginalPath
				displayName = filepath.Base(fullPath)
			}
		}

		// Skip if this is the user's home directory
		if homeDir != "" && fullPath == homeDir {
			continue
		}

		// Count JSONL files and track latest modification time
		sessionCount := 0
		var lastModified time.Time
		dirEntries, err := os.ReadDir(dirPath)
		if err == nil {
			for _, de := range dirEntries {
				if !de.IsDir() && strings.HasSuffix(de.Name(), ".jsonl") {
					sessionCount++
					if info, err := de.Info(); err == nil {
						if info.ModTime().After(lastModified) {
							lastModified = info.ModTime()
						}
					}
				}
			}
		}

		if sessionCount == 0 {
			continue
		}

		projects = append(projects, Project{
			DirName:      entry.Name(),
			DisplayName:  displayName,
			FullPath:     fullPath,
			DirPath:      dirPath,
			SessionCount: sessionCount,
			LastModified: lastModified,
		})
	}

	// Sort by display name
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].DisplayName < projects[j].DisplayName
	})

	return projects, nil
}

// ListProjectSessions returns session metadata for a project directory.
// Uses sessions-index.json when available for fast metadata access,
// falls back to stat-based listing.
func ListProjectSessions(projectDir string) ([]SessionMeta, error) {
	// Try sessions-index.json first
	indexPath := filepath.Join(projectDir, "sessions-index.json")
	if data, err := os.ReadFile(indexPath); err == nil {
		var idx sessionsIndex
		if err := json.Unmarshal(data, &idx); err == nil {
			return parseIndexEntries(idx.Entries, projectDir), nil
		}
	}

	// Fall back to stat-based listing
	return statBasedSessions(projectDir)
}

func parseIndexEntries(entries []sessionMetaJSON, projectDir string) []SessionMeta {
	var sessions []SessionMeta
	for _, e := range entries {
		meta := e.SessionMeta

		// Parse time strings
		if e.CreatedStr != "" {
			if t, err := time.Parse(time.RFC3339, e.CreatedStr); err == nil {
				meta.Created = t
			}
		}
		if e.ModifiedStr != "" {
			if t, err := time.Parse(time.RFC3339, e.ModifiedStr); err == nil {
				meta.Modified = t
			}
		}
		// Fall back to file mtime
		if meta.Created.IsZero() && e.FileMtime > 0 {
			meta.Modified = time.UnixMilli(e.FileMtime)
		}

		// Construct full path if not set (don't stat - defer to load time)
		if meta.FullPath == "" && meta.SessionID != "" {
			meta.FullPath = filepath.Join(projectDir, meta.SessionID+".jsonl")
		}

		sessions = append(sessions, meta)
	}

	// Sort ascending by created time
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Created.Before(sessions[j].Created)
	})

	return sessions
}

// GetSessionFileInfo stats a session file to get its size.
// Call this only when you need the size (e.g., before loading).
func GetSessionFileInfo(path string) (size int64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func statBasedSessions(projectDir string) ([]SessionMeta, error) {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil, err
	}

	var sessions []SessionMeta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		meta := SessionMeta{
			SessionID: sessionID,
			FullPath:  filepath.Join(projectDir, entry.Name()),
		}

		// Get basic info (this is cached by the OS from ReadDir)
		if info, err := entry.Info(); err == nil {
			meta.Modified = info.ModTime()
			meta.Created = info.ModTime() // Best guess without index
			meta.FileSize = info.Size()
		}

		sessions = append(sessions, meta)
	}

	// Sort ascending by time
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Created.Before(sessions[j].Created)
	})

	return sessions, nil
}
