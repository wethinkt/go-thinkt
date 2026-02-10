package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/copilot"
	"github.com/wethinkt/go-thinkt/internal/sources/gemini"
	"github.com/wethinkt/go-thinkt/internal/sources/kimi"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// OpenLazySession opens a session file for viewing, auto-detecting the format.
// This path-only variant uses best-effort format detection.
func OpenLazySession(path string) (thinkt.LazySession, error) {
	tuilog.Log.Info("OpenLazySession: opening", "path", path)
	switch {
	case isKimiSession(path):
		tuilog.Log.Info("OpenLazySession: detected as kimi session")
		return openKimiLazySession(path)
	case isCopilotSession(path):
		tuilog.Log.Info("OpenLazySession: detected as copilot session")
		return openCopilotLazySession(path)
	case isGeminiSession(path):
		tuilog.Log.Info("OpenLazySession: detected as gemini session")
		return openGeminiLazySession(path)
	case isJSONLSession(path):
		format, err := detectJSONLSessionType(path)
		if err != nil {
			return nil, fmt.Errorf("detect jsonl format: %w", err)
		}
		switch format {
		case "claude":
			tuilog.Log.Info("OpenLazySession: detected as claude session")
			return openClaudeLazySession(path)
		default:
			return nil, fmt.Errorf("unsupported jsonl session format: %s", path)
		}
	default:
		return nil, fmt.Errorf("unsupported session format: %s", path)
	}
}

// OpenLazySessionWithRegistry resolves the owning source store for a path
// first, then falls back to path-based detection.
func OpenLazySessionWithRegistry(path string, registry *thinkt.StoreRegistry) (thinkt.LazySession, error) {
	if registry != nil {
		ls, err := registry.OpenLazySessionByPath(context.Background(), path)
		if err == nil {
			return ls, nil
		}
		tuilog.Log.Warn("OpenLazySessionWithRegistry: falling back to format detection", "path", path, "error", err)
	}
	return OpenLazySession(path)
}

// isKimiSession detects if the path is a Kimi session file.
// Kimi sessions are stored as context.jsonl in ~/.kimi/sessions/{hash}/{uuid}/
func isKimiSession(path string) bool {
	base := filepath.Base(path)
	return base == "context.jsonl" || (strings.HasPrefix(base, "context_sub_") && strings.HasSuffix(base, ".jsonl"))
}

// isCopilotSession detects Copilot events.jsonl files.
func isCopilotSession(path string) bool {
	return filepath.Base(path) == "events.jsonl"
}

// isGeminiSession detects Gemini chat JSON files.
func isGeminiSession(path string) bool {
	return filepath.Ext(path) == ".json"
}

func isJSONLSession(path string) bool {
	return filepath.Ext(path) == ".jsonl"
}

func detectJSONLSessionType(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}

		// Claude JSONL entries include both "type" and "message" on message lines.
		if _, hasType := obj["type"]; hasType {
			if _, hasMessage := obj["message"]; hasMessage {
				return "claude", nil
			}
		}

		// Some Claude entries (e.g. snapshots) omit "message"; use UUID+type as a secondary hint.
		if t, ok := obj["type"].(string); ok {
			switch {
			case t == "summary":
				return "claude", nil
			case t == "user" || t == "assistant" || t == "system" || t == "tool" || t == "progress":
				if _, hasUUID := obj["uuid"]; hasUUID {
					return "claude", nil
				}
			case strings.HasPrefix(t, "file-history-"):
				return "claude", nil
			}
		}

		return "unknown", nil
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "unknown", nil
}

// openClaudeLazySession opens a Claude session using the existing lazy loader.
func openClaudeLazySession(path string) (thinkt.LazySession, error) {
	// Claude's LazySession now implements thinkt.LazySession
	return claude.OpenLazySession(path)
}

// openKimiLazySession opens a Kimi session and wraps it in a lazy loader.
func openKimiLazySession(path string) (thinkt.LazySession, error) {
	// Extract the session ID from the path
	// Path format: ~/.kimi/sessions/{hash}/{uuid}/context.jsonl
	sessionDir := filepath.Dir(path)
	uuid := filepath.Base(sessionDir)
	hashDir := filepath.Dir(sessionDir)
	hash := filepath.Base(hashDir)
	sessionID := hash + "/" + uuid

	// Find the Kimi base directory
	kimiBase := findKimiBaseDir(path)
	if kimiBase == "" {
		// Fallback: try to get from environment or default
		kimiBase, _ = kimi.DefaultDir()
	}

	// Create a Kimi store
	store := kimi.NewStore(kimiBase)

	// Open the session reader
	ctx := context.Background()
	reader, err := store.OpenSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("open kimi session: %w", err)
	}

	// Wrap in a lazy session
	return thinkt.NewLazySession(reader)
}

// openCopilotLazySession opens a Copilot session via the source store.
func openCopilotLazySession(path string) (thinkt.LazySession, error) {
	baseDir := findCopilotBaseDir(path)
	store := copilot.NewStore(baseDir)

	reader, err := store.OpenSession(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("open copilot session: %w", err)
	}
	return thinkt.NewLazySession(reader)
}

// openGeminiLazySession opens a Gemini session via the source store.
func openGeminiLazySession(path string) (thinkt.LazySession, error) {
	baseDir := findGeminiBaseDir(path)
	store := gemini.NewStore(baseDir)

	reader, err := store.OpenSession(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("open gemini session: %w", err)
	}
	return thinkt.NewLazySession(reader)
}

// findKimiBaseDir finds the Kimi base directory from a session path.
// Path format: ~/.kimi/sessions/{hash}/{uuid}/context.jsonl
func findKimiBaseDir(sessionPath string) string {
	// Walk up looking for "sessions" directory inside .kimi
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		if filepath.Base(dir) == "sessions" {
			// Check if parent is .kimi
			if filepath.Base(parent) == ".kimi" {
				return parent
			}
			// Also check for kimi (without dot) - some configs may use this
			if filepath.Base(parent) == "kimi" {
				return parent
			}
		}
		dir = parent
	}
	return ""
}

// findCopilotBaseDir finds the Copilot base directory from a session path.
// Path format: ~/.copilot/session-state/{session-id}/events.jsonl
func findCopilotBaseDir(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if filepath.Base(dir) == "session-state" {
			return parent
		}
		dir = parent
	}
	return ""
}

// findGeminiBaseDir finds the Gemini base directory from a session path.
// Path format: ~/.gemini/tmp/{project-hash}/chats/{chat-id}.json
func findGeminiBaseDir(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if filepath.Base(parent) == ".gemini" || filepath.Base(parent) == "gemini" {
			return parent
		}
		dir = parent
	}
	return ""
}

// SessionInfo holds metadata about a loaded session.
type SessionInfo struct {
	Meta       thinkt.SessionMeta
	EntryCount int
	HasMore    bool
	Progress   float64
}

// GetSessionInfo returns info about a lazy session.
func GetSessionInfo(ls thinkt.LazySession) SessionInfo {
	return SessionInfo{
		Meta:       ls.Metadata(),
		EntryCount: ls.EntryCount(),
		HasMore:    ls.HasMore(),
		Progress:   ls.Progress(),
	}
}

// ViewSessionRaw outputs a session's content as raw text without decoration.
// This is used for the --raw flag in the sessions view command.
func ViewSessionRaw(path string, w io.Writer) error {
	return ViewSessionRawWithRegistry(path, nil, w)
}

// ViewSessionRawWithRegistry outputs a session's content as raw text without decoration.
func ViewSessionRawWithRegistry(path string, registry *thinkt.StoreRegistry, w io.Writer) error {
	// Open the session
	ls, err := OpenLazySessionWithRegistry(path, registry)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer ls.Close()

	// Load all content
	if err := ls.LoadAll(); err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Output entries as raw text
	entries := ls.Entries()
	for _, entry := range entries {
		switch entry.Role {
		case thinkt.RoleUser:
			if entry.Text != "" {
				fmt.Fprintln(w, entry.Text)
				fmt.Fprintln(w)
			}
		case thinkt.RoleAssistant:
			// For assistant, output text content
			if entry.Text != "" {
				fmt.Fprintln(w, entry.Text)
				fmt.Fprintln(w)
			}
			// Also output tool_use blocks
			for _, block := range entry.ContentBlocks {
				switch block.Type {
				case "thinking":
					if block.Thinking != "" {
						fmt.Fprintln(w, block.Thinking)
						fmt.Fprintln(w)
					}
				case "tool_use":
					fmt.Fprintf(w, "[Tool: %s]\n", block.ToolName)
					fmt.Fprintln(w)
				case "tool_result":
					if block.ToolResult != "" {
						fmt.Fprintln(w, block.ToolResult)
						fmt.Fprintln(w)
					}
				}
			}
		}
	}

	return nil
}
