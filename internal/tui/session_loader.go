package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/codex"
	"github.com/wethinkt/go-thinkt/internal/sources/copilot"
	"github.com/wethinkt/go-thinkt/internal/sources/gemini"
	"github.com/wethinkt/go-thinkt/internal/sources/kimi"
	"github.com/wethinkt/go-thinkt/internal/sources/qwen"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// OpenLazySession opens a session file for viewing, auto-detecting the format.
// This path-only variant uses best-effort format detection.
func OpenLazySession(path string) (thinkt.LazySession, error) {
	tuilog.Log.Info("OpenLazySession: opening", "path", path)
	switch {
	case kimi.IsSessionPath(path):
		tuilog.Log.Info("OpenLazySession: detected as kimi session")
		return openKimiLazySession(path)
	case codex.IsSessionPath(path):
		tuilog.Log.Info("OpenLazySession: detected as codex session")
		return openCodexLazySession(path)
	case copilot.IsSessionPath(path):
		tuilog.Log.Info("OpenLazySession: detected as copilot session")
		return openCopilotLazySession(path)
	case qwen.IsSessionPath(path):
		tuilog.Log.Info("OpenLazySession: detected as qwen session")
		return openQwenLazySession(path)
	case gemini.IsSessionPath(path):
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
		case "codex":
			tuilog.Log.Info("OpenLazySession: detected as codex session")
			return openCodexLazySession(path)
		case "qwen":
			tuilog.Log.Info("OpenLazySession: detected as qwen session")
			return openQwenLazySession(path)
		default:
			return nil, fmt.Errorf("unsupported jsonl session format: %s", path)
		}
	default:
		return nil, fmt.Errorf("unsupported session format: %s", path)
	}
}

// OpenLazySessionWithRegistry opens a session from a file path and, if direct
// detection fails, falls back to registry-based source resolution.
func OpenLazySessionWithRegistry(path string, registry *thinkt.StoreRegistry) (thinkt.LazySession, error) {
	// Fast path: for normal TUI navigation we already have a concrete session
	// file path, so open it directly to avoid expensive registry-wide scans.
	ls, directErr := OpenLazySession(path)
	if directErr == nil {
		return ls, nil
	}

	if registry != nil {
		ls, regErr := registry.OpenLazySessionByPath(context.Background(), path)
		if regErr == nil {
			return ls, nil
		}
		return nil, fmt.Errorf("open session: direct detection failed: %v; registry lookup failed: %w", directErr, regErr)
	}

	return nil, directErr
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

	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxScannerCapacity)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}

		// Qwen entries have "sessionId" alongside "type" and "message".
		if _, hasSessionID := obj["sessionId"]; hasSessionID {
			return "qwen", nil
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
			case t == "session_meta":
				return "codex", nil
			case t == "event_msg":
				if payload, ok := obj["payload"].(map[string]any); ok {
					switch payloadType, _ := payload["type"].(string); payloadType {
					case "user_message", "agent_message", "agent_reasoning", "turn_context", "turn_aborted", "token_count":
						return "codex", nil
					}
				}
			case t == "response_item":
				if payload, ok := obj["payload"].(map[string]any); ok {
					switch payloadType, _ := payload["type"].(string); payloadType {
					case "message", "reasoning", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output":
						return "codex", nil
					}
				}
			}
		}
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

// openQwenLazySession opens a Qwen session via the source store.
func openQwenLazySession(path string) (thinkt.LazySession, error) {
	store := qwen.NewStore("")

	reader, err := store.OpenSession(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("open qwen session: %w", err)
	}
	return thinkt.NewLazySession(reader)
}

// openCodexLazySession opens a Codex session via the source store.
func openCodexLazySession(path string) (thinkt.LazySession, error) {
	baseDir := findCodexBaseDir(path)
	store := codex.NewStore(baseDir)

	reader, err := store.OpenSession(context.Background(), path)
	if err != nil {
		return nil, fmt.Errorf("open codex session: %w", err)
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

// findCodexBaseDir finds the Codex base directory from a session path.
// Path format: ~/.codex/sessions/YYYY/MM/DD/*.jsonl
func findCodexBaseDir(sessionPath string) string {
	dir := filepath.Dir(sessionPath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		if filepath.Base(dir) == ".codex" || filepath.Base(dir) == "codex" {
			return dir
		}
		if filepath.Base(parent) == ".codex" {
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
			for _, block := range entry.ContentBlocks {
				if (block.Type == "image" || block.Type == "document") && block.MediaData != "" {
					// Try sixel rendering
					imgStr, err := encodeImage(block.MediaData, 80, 8)
					if err == nil {
						fmt.Fprint(w, imgStr)
						fmt.Fprintln(w)
					} else {
						// Fallback to text placeholder
						rawBytes := len(block.MediaData) * 3 / 4
						dims := decodeImageDimensions(block.MediaType, block.MediaData)
						if dims != "" {
							fmt.Fprintf(w, "[%s %s, %s]\n\n", block.MediaType, dims, formatByteSize(rawBytes))
						} else {
							fmt.Fprintf(w, "[%s %s]\n\n", block.MediaType, formatByteSize(rawBytes))
						}
					}
				}
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
