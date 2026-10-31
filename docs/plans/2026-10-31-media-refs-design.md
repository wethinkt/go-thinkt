# Media references

**Why.** Each source keeps images and files its own way, and thinkt carries
them differently too: base64 in a block, a text line, a URL as text, or raw
JSON in a tool result. A reader cannot tell what a session attached, whether
two images are the same, or where the bytes live. The user skipped the design
questions, so the calls below are mine.

## Goals

- Every attachment a source records becomes an `image` or `document` block.
- Every such block carries `ContentBlock.Media`: a `thinkt.MediaRef` with the
  file or URL holding the bytes, and for bytes we can see, their SHA-256 and
  size, so the same image is recognisable across sessions and sources.
- `MediaData` stays for inline bytes, since the TUI renders from it.

## Non-goals

- Copying media anywhere, or a media store.
- Reading a referenced file. Paths come from transcripts, so nothing opens them
  in this pass, and the TUI still renders inline images only.
- Stripping `MediaData` from relay payloads. The archive keeps everything, as
  the user decided for the collector.
- Gemini `.json` sessions, whose content is plain text.

## Local tally

| Source | Records | Today |
|---|---|---|
| Claude | base64 `image` in messages and tool results (597; 171 more are in dropped request dumps) | block, no hash |
| Codex | `input_image` data URLs in wire outputs (570), messages (22); MCP `image` parts (27); `ImageView` paths | outputs are raw JSON text; MCP parts are text; paths are text |
| Kimi | `blobref:<type>;<sha256>` in tool results (90), bytes in `agents/<id>/blobs/<sha256>`; data URLs in `tool`-role twins of those results, and in user messages | blobrefs are text; user images dropped |
| Antigravity | `media[]` `{mime_type, uri}` (52), `file://` or bare paths | a text line |
| Gemini | none locally; `inlineData`, `fileData` parts are the documented shape | dropped |

## Shape

```go
type MediaRef struct {
	URI    string `json:"uri,omitempty"`    // file path or URL holding the bytes
	SHA256 string `json:"sha256,omitempty"` // of the decoded bytes
	Size   int64  `json:"size,omitempty"`   // decoded bytes
}
func MediaBlock(mediaType, data string) ContentBlock     // inline base64
func MediaBlockURI(mediaType, uri string) ContentBlock   // elsewhere; file:// is stripped
func MediaBlockURL(url string) (ContentBlock, bool)      // a data: URL, or any other URL
```

`image/*` makes an `image` block, anything else `document`. Hashing happens
once per inline image as the entry is built; bytes that are not valid base64
are hashed as text so a block still has an identity.

Decisions:

- Antigravity keeps its text line as well as the block. Search and any
  renderer reading `Text` keep working.
- An entry that had only `Text` gains a text block ahead of its media, since
  renderers prefer blocks over `Text`.
- Kimi's converter records the SHA-256 from a blobref; the session reader fills
  the URI from the agent's `blobs/` folder, falling back to the main agent's.
- Codex array outputs become text plus media children, replacing the raw JSON.
- Markdown export names the URI.

`thinkt.ParserVersion` goes to 9.

## Tasks

1. `MediaRef` and constructors.
2. Claude. 3. Codex. 4. Kimi. 5. Antigravity. 6. Gemini.
7. Export, real-data count against the tally, docs.

Each task: failing test first. A fresh reviewer reads the branch at the end,
with one fix pass.
