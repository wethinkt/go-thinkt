// Package conversation adapts source-neutral entries to a presentation document.
// It preserves conversation identity independently of HTML, terminal cells, or pixels.
package conversation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Origin maps a presentation node back to the original session data. EntryIndex
// is the unfiltered session index; BlockIndex is -1 for a message or Text fallback.
type Origin struct {
	SessionID  string
	EntryUUID  string
	EntryIndex int
	BlockIndex int
}

type Document struct {
	Title    string
	Messages []Message
}

type Message struct {
	ID        string
	Origin    Origin
	Role      thinkt.Role
	Timestamp time.Time
	Source    thinkt.Source
	Model     string
	Blocks    []Block

	// Classification and turn metadata copied from the entry.
	Kind        thinkt.EntryKind
	Visibility  thinkt.Visibility
	TurnID      string
	PromptID    string
	Attribution *thinkt.Attribution
	StopReason  string
	Error       string
	Compaction  *thinkt.CompactionInfo
}

// Block retains semantic content, not layout. Related holds tool call/result
// anchors in this document. MediaData is retained for explicit host decoding;
// the HTML adapter emits only a placeholder, never a file or network request.
type Block struct {
	ID        string
	Origin    Origin
	Kind      string
	Text      string
	ToolID    string
	ToolName  string
	IsError   bool
	MediaType string
	MediaData string
	Related   []string

	// Children are nested parts of a tool result (text, media, diff). Their
	// IDs are "<parent>-child-<k>" and their Origin is the parent's.
	Children       []Block
	ChildSessionID string
}

type BuildOptions struct {
	Title     string
	SessionID string
	// EntryOffset lets separately loaded batches keep their original indices
	// and anchors. Apply display filters after Build, never before it.
	EntryOffset int
}

// Build adapts a batch without truncating content or moving tool results out of
// source order. Relationships are resolved among the entries in this batch.
func Build(entries []thinkt.Entry, opts BuildOptions) (*Document, error) {
	if opts.EntryOffset < 0 {
		return nil, fmt.Errorf("conversation: negative entry offset")
	}
	doc := &Document{Title: opts.Title}
	for i, entry := range entries {
		origin := Origin{opts.SessionID, entry.UUID, opts.EntryOffset + i, -1}
		// Include the source index to disambiguate missing or repeated UUIDs.
		key, _ := json.Marshal([]string{origin.SessionID, string(entry.Source), entry.UUID, strconv.Itoa(origin.EntryIndex)})
		id := fmt.Sprintf("message-%x", sha256.Sum256(key))
		message := Message{ID: id, Origin: origin, Role: entry.Role, Timestamp: entry.Timestamp, Source: entry.Source, Model: entry.Model,
			Kind: entry.Kind, Visibility: entry.Visibility, TurnID: entry.TurnID, PromptID: entry.PromptID, Attribution: entry.Attribution,
			StopReason: entry.StopReason, Error: entry.Error, Compaction: entry.Compaction}
		for j, raw := range entry.ContentBlocks {
			blockOrigin := origin
			blockOrigin.BlockIndex = j
			block, err := buildBlock(raw, id+"-block-"+strconv.Itoa(j), blockOrigin)
			if err != nil {
				return nil, err
			}
			message.Blocks = append(message.Blocks, block)
		}
		if len(entry.ContentBlocks) == 0 && entry.Text != "" {
			message.Blocks = append(message.Blocks, Block{ID: id + "-text", Origin: origin, Kind: "text", Text: entry.Text})
		}
		doc.Messages = append(doc.Messages, message)
	}
	linkTools(doc)
	return doc, nil
}

// buildBlock converts one content block and, recursively, its children.
func buildBlock(raw thinkt.ContentBlock, id string, origin Origin) (Block, error) {
	block := Block{ID: id, Origin: origin, Kind: raw.Type,
		ToolID: raw.ToolUseID, ToolName: raw.ToolName, IsError: raw.IsError,
		MediaType: raw.MediaType, MediaData: raw.MediaData, ChildSessionID: raw.ChildSessionID}
	switch raw.Type {
	case "text", thinkt.BlockTypeDiff:
		block.Text = raw.Text
	case "thinking":
		block.Text = raw.Thinking
	case "tool_use":
		input, err := json.MarshalIndent(raw.ToolInput, "", "  ")
		if err != nil {
			return block, fmt.Errorf("conversation: entry %d block %d tool input: %w", origin.EntryIndex, origin.BlockIndex, err)
		}
		block.Text = string(input)
	case "tool_result":
		block.Text = raw.ToolResult
	case "image", "document":
		// Payload stays in the semantic document for a host asset resolver.
	default:
		// Preserve unfamiliar block data rather than silently dropping it.
		data, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return block, fmt.Errorf("conversation: entry %d block %d: %w", origin.EntryIndex, origin.BlockIndex, err)
		}
		block.Text = string(data)
	}
	for k, child := range raw.Children {
		built, err := buildBlock(child, id+"-child-"+strconv.Itoa(k), origin)
		if err != nil {
			return block, err
		}
		block.Children = append(block.Children, built)
	}
	return block, nil
}

func linkTools(doc *Document) {
	// Pair with the nearest preceding call. A result arriving before its call
	// remains an independent block, as does a result in another loaded batch.
	calls := make(map[string]*Block)
	for i := range doc.Messages {
		for j := range doc.Messages[i].Blocks {
			block := &doc.Messages[i].Blocks[j]
			if block.ToolID == "" {
				continue
			}
			switch block.Kind {
			case "tool_use":
				calls[block.ToolID] = block
			case "tool_result":
				if call := calls[block.ToolID]; call != nil {
					call.Related = append(call.Related, block.ID)
					block.Related = append(block.Related, call.ID)
				}
			}
		}
	}
}
