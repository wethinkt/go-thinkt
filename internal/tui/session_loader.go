package tui

import (
	"context"
	"fmt"
	"io"

	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// OpenLazySession opens a session file for viewing. The source is picked
// from the path (and, for an unclaimed .jsonl file, its first records) by
// sources.OpenSessionPath; a file whose source is turned off still opens.
func OpenLazySession(path string) (thinkt.LazySession, error) {
	ls, err := sources.OpenSessionPath(context.Background(), path)
	if err != nil {
		return nil, err
	}
	tuilog.Log.Info("OpenLazySession: opened", "path", path, "source", ls.Metadata().Source)
	return ls, nil
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
