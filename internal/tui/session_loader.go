package tui

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/kimi"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// OpenLazySession opens a session file for viewing, auto-detecting the format.
// It supports both Claude (.jsonl) and Kimi (context.jsonl) formats.
func OpenLazySession(path string) (thinkt.LazySession, error) {
	tuilog.Log.Info("OpenLazySession: opening", "path", path)
	// Detect format based on file path patterns
	if isKimiSession(path) {
		tuilog.Log.Info("OpenLazySession: detected as kimi session")
		return openKimiLazySession(path)
	}

	// Default to Claude format
	tuilog.Log.Info("OpenLazySession: detected as claude session")
	return openClaudeLazySession(path)
}

// isKimiSession detects if the path is a Kimi session file.
// Kimi sessions are stored as context.jsonl in ~/.kimi/sessions/{hash}/{uuid}/
func isKimiSession(path string) bool {
	// Check if the path contains ".kimi/sessions" or ends with context.jsonl in a session directory
	base := filepath.Base(path)
	if base == "context.jsonl" || base == "context_sub_1.jsonl" {
		return true
	}
	return false
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
	// Open the session
	ls, err := OpenLazySession(path)
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
