package antigravity

import (
	"os"
	"path/filepath"
)

// Transcript file names inside brain/<id>/.system_generated/logs. The full
// transcript is preferred; the plain one is read when it is the only file.
const (
	fullTranscript  = "transcript_full.jsonl"
	plainTranscript = "transcript.jsonl"
)

// DefaultDir returns the Antigravity CLI base directory:
// THINKT_ANTIGRAVITY_HOME if set, otherwise ~/.gemini/antigravity-cli.
func DefaultDir() (string, error) {
	if dir := os.Getenv("THINKT_ANTIGRAVITY_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "antigravity-cli"), nil
}

// logsDir returns the directory holding a conversation's transcripts.
func logsDir(baseDir, id string) string {
	return filepath.Join(baseDir, "brain", id, ".system_generated", "logs")
}

// sessionFile returns the transcript to read for a conversation: the full
// transcript, else the plain one, else "".
func sessionFile(baseDir, id string) string {
	for _, name := range []string{fullTranscript, plainTranscript} {
		path := filepath.Join(logsDir(baseDir, id), name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

// splitTranscriptPath returns the base directory and conversation id of a
// path of the form <base>/brain/<id>/.system_generated/logs/transcript{,_full}.jsonl.
func splitTranscriptPath(path string) (base, id string, ok bool) {
	clean := filepath.Clean(path)
	if name := filepath.Base(clean); name != fullTranscript && name != plainTranscript {
		return "", "", false
	}
	logs := filepath.Dir(clean)
	generated := filepath.Dir(logs)
	convDir := filepath.Dir(generated)
	brain := filepath.Dir(convDir)
	if filepath.Base(logs) != "logs" || filepath.Base(generated) != ".system_generated" || filepath.Base(brain) != "brain" {
		return "", "", false
	}
	return filepath.Dir(brain), filepath.Base(convDir), true
}

// IsSessionPath reports whether path is a transcript under the Antigravity
// home. Split copies under logs/chunks are not session files.
func IsSessionPath(path string) bool {
	base, _, ok := splitTranscriptPath(path)
	if !ok {
		return false
	}
	home, err := DefaultDir()
	if err != nil {
		return false
	}
	return filepath.Clean(base) == filepath.Clean(home) || sameDir(base, home)
}

// sameDir reports whether two paths name the same directory after
// resolving symlinks.
func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// BaseDirForPath returns the Antigravity base directory a transcript path
// belongs to, or "" when the path is not a transcript.
func BaseDirForPath(path string) string {
	base, _, ok := splitTranscriptPath(path)
	if !ok {
		return ""
	}
	return base
}

// SessionIDForPath returns the conversation id of a transcript path, or "".
func SessionIDForPath(path string) string {
	_, id, ok := splitTranscriptPath(path)
	if !ok {
		return ""
	}
	return id
}
