package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
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
	Model        string    `json:"model,omitempty"`
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
// display name and decoded full path. The directory name format replaces path
// separators with "-":
//
//   - Unix: "-Users-evan-project" → "/Users/evan/project" (leading "-" = root "/")
//   - Windows: "C-Users-evan-project" → "C:\Users\evan\project" (first segment = drive letter)
//
// Because "-" is ambiguous (it could be a path separator or a literal hyphen in a
// directory name), we validate against the filesystem. If the naive decode doesn't
// produce an existing path, we greedily reconstruct it by checking which segments
// exist on disk.
func DecodeDirName(dirName string) (displayName string, fullPath string) {
	if dirName == "-" {
		return "~", ""
	}

	// Parse segments and determine root prefix.
	// On Unix: leading "-" maps to "/".
	// On Windows: a single-letter first segment is a drive letter (e.g., "C" → "C:\").
	var segments []string
	var prefix string
	sep := string(filepath.Separator)
	if strings.HasPrefix(dirName, "-") {
		segments = strings.Split(dirName[1:], "-")
		prefix = sep // "/" on Unix, "\" on Windows
	} else {
		segments = strings.Split(dirName, "-")
		if runtime.GOOS == "windows" && len(segments) > 0 && len(segments[0]) == 1 {
			// Drive letter: "C-Users-..." → "C:\"
			prefix = segments[0] + ":" + sep
			segments = segments[1:]
		}
	}

	// Fast path: naive decode (all "-" become path separators)
	fullPath = prefix + strings.Join(segments, sep)

	if _, err := os.Stat(fullPath); err == nil {
		displayName = filepath.Base(fullPath)
		return displayName, fullPath
	}

	// Slow path: greedily build the path, joining segments with "-" when
	// a separator split doesn't match an existing directory on disk.
	rebuilt := prefix + segments[0]
	for i := 1; i < len(segments); i++ {
		withHyphen := rebuilt + "-" + segments[i]
		withSep := rebuilt + sep + segments[i]

		// Prefer path separator if that directory exists, otherwise use "-"
		if _, err := os.Stat(withSep); err == nil {
			rebuilt = withSep
		} else if _, err := os.Stat(withHyphen); err == nil {
			rebuilt = withHyphen
		} else {
			rebuilt = withSep
		}
	}

	fullPath = rebuilt
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
		if fullPath == "" {
			fullPath = homeDir // Fallback to home dir if decoding fails (e.g., for "-")
		}

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
// Always enumerates .jsonl files on disk to avoid missing sessions when
// sessions-index.json is stale. Index metadata (timestamps, prompts) is
// used to enrich results when available.
func ListProjectSessions(projectDir string) ([]SessionMeta, error) {
	// Always scan the filesystem for the authoritative list of sessions.
	sessions, err := statBasedSessions(projectDir)
	if err != nil {
		return nil, err
	}

	// Enrich with index metadata when available.
	indexPath := filepath.Join(projectDir, "sessions-index.json")
	if data, err := os.ReadFile(indexPath); err == nil {
		var idx sessionsIndex
		if err := json.Unmarshal(data, &idx); err == nil {
			indexed := parseIndexEntries(idx.Entries, projectDir)
			byID := make(map[string]SessionMeta, len(indexed))
			for _, m := range indexed {
				byID[m.SessionID] = m
			}
			for i, s := range sessions {
				if rich, ok := byID[s.SessionID]; ok {
					if !rich.Created.IsZero() {
						sessions[i].Created = rich.Created
					}
					if !rich.Modified.IsZero() {
						sessions[i].Modified = rich.Modified
					}
					if rich.FirstPrompt != "" {
						sessions[i].FirstPrompt = rich.FirstPrompt
					}
					if rich.Model != "" {
						sessions[i].Model = rich.Model
					}
					if rich.Summary != "" {
						sessions[i].Summary = rich.Summary
					}
					if rich.GitBranch != "" {
						sessions[i].GitBranch = rich.GitBranch
					}
					if rich.MessageCount > 0 {
						sessions[i].MessageCount = rich.MessageCount
					}
					if rich.ProjectPath != "" {
						sessions[i].ProjectPath = rich.ProjectPath
					}
				}
			}
		}
	}

	return sessions, nil
}

// ListProjectSessionsBackfill is like ListProjectSessions but additionally
// opens each JSONL file missing a FirstPrompt or Model to extract them.
// Use this when display-quality metadata is needed and the cost of scanning
// session files is acceptable.
func ListProjectSessionsBackfill(projectDir string) ([]SessionMeta, error) {
	sessions, err := ListProjectSessions(projectDir)
	if err != nil {
		return nil, err
	}

	for i := range sessions {
		if sessions[i].FullPath != "" && (sessions[i].FirstPrompt == "" || !thinkt.IsRealModel(sessions[i].Model)) {
			prompt, model := extractSessionHints(sessions[i].FullPath)
			if sessions[i].FirstPrompt == "" {
				sessions[i].FirstPrompt = prompt
			}
			if !thinkt.IsRealModel(sessions[i].Model) {
				sessions[i].Model = model
			}
		}
	}

	return sessions, nil
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

		// Construct full path if not set
		if meta.FullPath == "" && meta.SessionID != "" {
			meta.FullPath = filepath.Join(projectDir, meta.SessionID+".jsonl")
		}

		// Stat file for size (not in index JSON)
		if meta.FullPath != "" && meta.FileSize == 0 {
			if info, err := os.Stat(meta.FullPath); err == nil {
				meta.FileSize = info.Size()
			}
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

// extractSessionHints reads the first user message and first model from a
// Claude JSONL session file. It scans at most the first 50 lines to keep the
// operation lightweight.
func extractSessionHints(path string) (firstPrompt, model string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, 1*1024*1024)

	for i := 0; i < 50 && scanner.Scan(); i++ {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		lineStr := string(line)

		// Extract first user prompt
		if firstPrompt == "" && strings.Contains(lineStr, `"type":"user"`) {
			var entry struct {
				Type    string          `json:"type"`
				Message json.RawMessage `json:"message"`
			}
			if json.Unmarshal(line, &entry) == nil && entry.Type == "user" {
				var msg UserMessage
				if json.Unmarshal(entry.Message, &msg) == nil {
					firstPrompt = msg.Content.GetText()
				}
			}
		}

		// Extract first real model from assistant entry
		if !thinkt.IsRealModel(model) && strings.Contains(lineStr, `"type":"assistant"`) {
			var entry struct {
				Type    string          `json:"type"`
				Message json.RawMessage `json:"message"`
			}
			if json.Unmarshal(line, &entry) == nil && entry.Type == "assistant" {
				var msg struct {
					Model string `json:"model"`
				}
				if json.Unmarshal(entry.Message, &msg) == nil && thinkt.IsRealModel(msg.Model) {
					model = msg.Model
				}
			}
		}

		if firstPrompt != "" && thinkt.IsRealModel(model) {
			break
		}
	}

	return firstPrompt, model
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
