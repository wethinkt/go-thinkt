# Conversation presentation adapter

`thinkt.Entry` / `ContentBlock` → `conversation.Document` → shared HTML/CSS → browser or terminal renderer.

This package owns the conversion from conversation semantics to a presentation
document. Source parsing, navigation, asset decoding, and renderer-specific layout
remain with their respective hosts. HTML export and the opt-in embedded HTML
session viewer use this adapter. The default TUI viewer and embedded webapps
retain their existing renderers during this migration.

```go
doc, err := conversation.Build(entries, conversation.BuildOptions{
    Title: "My conversation",
    SessionID: session.ID,
    EntryOffset: offset,
})
if err != nil {
    return err
}
// Self-contained page: shared stylesheet, title, and conversation fragment.
return conversation.WritePage(w, doc, conversation.View{})
```

`WriteHTML` emits only the fragment, for embedding in a host that includes
`Stylesheet()`. Use that same stylesheet and fragment for browser and terminal
consumers. The stylesheet uses scoped classes and block layout rather than
browser-specific flex/grid behavior. Typography can still differ with fonts and
renderer capabilities; this is a shared presentation contract, not a guarantee
of identical pixels.

## Identity and view state

Every message and content block has an anchor and an `Origin` pointing to the
original session, entry UUID, entry index, and block index. Missing or repeated
UUIDs are disambiguated by the source index. To keep anchors stable across
incremental loads, supply the same `SessionID` and original `EntryOffset` for each
batch. Build before filtering; indices must refer to the unfiltered source data.
The source index is part of the identity, so prepending/reordering source entries
requires rebuilding anchors.

Tool results stay in source order. Reciprocal links connect them to the nearest
preceding call with the same tool ID in the supplied batch. Results without a
loaded call remain independent; a host loading separate batches will need to
resolve relationships across them.

`View.HiddenRoles`, `HiddenKinds`, and `Collapsed` select what is displayed
without changing the semantic document. Collapsed block bodies use shared
`display: none` styling. They are still in the generated HTML, so these controls
are presentation preferences, not data access controls. Hosts own toggle input
and regeneration; this initial adapter emits no JavaScript or interactive toggle
controls. The default is fully expanded content, including in HTML exports.

## Content

Text becomes Markdown paragraphs, lists, tables, links, and code. Raw HTML in
Markdown is disabled. Thinking, tool input JSON, and tool results are escaped
literal content, without truncation. Unknown roles and block types remain visible.
Embedded media retains its payload in the semantic document and becomes a
placeholder with a media anchor. Markdown images also become placeholders.
Neither host should fetch assets implicitly; future asset resolvers must provide
decoded media explicitly.

The adapter operates on supplied batches and buffers each HTML output. A future
viewer should keep those batches bounded and preserve source anchors while
virtualizing layout; do not rasterize an entire unbounded session.

## Kinds, visibility, and nested blocks

`Message` carries the entry's `Kind`, `Visibility`, `TurnID`, `Attribution`,
`StopReason`, `Error`, and `Compaction`; the HTML section exposes `data-turn-id`
and `data-kind`, and the metadata line adds attribution, `refusal`, and errors.
`View.HiddenVisibility` hides whole messages by visibility: `WritePage` defaults
it to hide `secondary` (bookkeeping-adjacent entries), `WriteHTML` with a nil
map hides nothing. `Block.Children` holds the nested parts of a tool result
(text, media, `diff`) with IDs `<parent>-child-<k>` and the parent's origin;
they render inside the parent block body, a `diff` as `<pre class="diff">` with
one classed span per line.

## Verification and sample

`testdata/session.json` is a representative exchange with Markdown, thinking,
a tool call/error result, and media. `testdata/conversation.html` is its checked
presentation contract and can be opened directly in a browser.

Run these from the go-thinkt root. The launcher uses the sibling `nthtml` and
`nttypeset` checkouts together in a temporary Go workspace. Local commits do not
need to be pushed; no repository `go.mod` or `go.work` file is changed.

```sh
open internal/conversation/testdata/conversation.html  # macOS browser
sh etc/try-conversation.sh                           # interactive terminal
```

The terminal viewer is now provided by nthtml's own CLI. Press `t` to switch
TTY/styled raster, `b` to compare native/nttypeset rendering, `/` to search,
`n`/`N` for matches, drag and `y` to select/copy, Tab/Enter for links, and `q`
to quit. Full graphics depend on terminal capabilities; TTY mode provides a
text view. This is a development path, not yet `thinkt sessions view`.

For deterministic text and image outputs:

```sh
sh etc/try-conversation.sh -mode tty -columns 72
sh etc/try-conversation.sh -mode png -width 800 -o /tmp/thinkt-native.png
sh etc/try-conversation.sh -mode typeset -width 800 -o /tmp/thinkt-typeset.png
gloss /tmp/thinkt-native.png
gloss /tmp/thinkt-typeset.png
```

The typeset output now preserves nested card decoration and source IDs. Native
and bridged fonts, margins, line breaking, and table appearance can still differ.
The bridge CLI uses the document's resolved palette. `-height 1000` renders only
an initial typeset viewport, rather than its full bounded extent.

Opening the HTML directly in today's gloss still takes its HTML-to-Markdown
conversion path, which drops CSS. View PNG output to inspect styling until gloss
gains its planned styled HTML path.

To exercise a real session, build thinkt and export the most recent session for
this project (or provide a session ID as a positional argument):

```sh
go build -o ./bin/thinkt ./cmd/thinkt
./bin/thinkt export --project "$PWD" -f html -o /tmp/thinkt-session.html </dev/null
sh etc/try-conversation.sh /tmp/thinkt-session.html -mode view
```

The stdin redirection uses the non-interactive export path rather than the
wizard. Normal first-run source setup still applies. The fixture is deliberately
small; real sessions that exceed nthtml's document budgets will need bounded
sections in the future viewer rather than raised limits or a full-session raster.

```sh
go test ./internal/conversation ./internal/export
# After reviewing an intentional presentation change:
go test ./internal/conversation -run TestHTMLGolden -update
```

The fixture is exercised through `document.Parse` → `Document.TTY`,
`document.Render`, and `nthtml/typeset.BuildWithSources` → nttypeset layout/raster.
Use `sh etc/try-conversation.sh` to compare all three in one viewer.

## Local upstream status (2026-10-26)

- nttypeset `dc9276f`: nested section decoration, resolved block formatting,
  opaque source IDs, anchor bounds, and source lookup.
- nthtml `db867c8`: styled extraction and source mappings across native, TTY,
  and nttypeset paths, plus arbitrary-file CLI viewing and comparison.
- Gloss's styled HTML task remains outstanding.

The local upstream commits were verified against the thinkt fixture using both
checkouts. The root module now pins the nthtml commit for embedded viewing.
Until these renderer versions are published, use the local launcher below;
ordinary builds outside a local workspace cannot fetch the unpublished commit.

## Embedded session viewer

```sh
# From go-thinkt: temporary workspace, local renderer checkouts, HTML viewer on.
sh etc/try-thinkt.sh
sh etc/try-thinkt.sh sessions view /absolute/path/to/session.jsonl
```

The launcher enables `THINKT_VIEWER=html` and combines all three local modules,
with version-specific workspace overrides for the unpublished module metadata.
It changes no repository module/workspace files. To build manually in a local
workspace, include go-thinkt, nthtml, and nttypeset, and replace the pinned
unpublished versions in that workspace. The existing viewer is still the default
when running thinkt without `THINKT_VIEWER=html`.

The embedded viewer is connected to session-picker selections, search-result
selections, and standalone session viewing. It starts in TTY mode; `t` switches
to styled raster. Normal source loading is unchanged. Only up to eight entries
are presented at once, shrinking the group if renderer budgets require it.
Content is not truncated: a single entry exceeding the renderer's budgets
reports an error. Readers load incrementally and close on Back, Quit, or program
exit, including when a load/layout result is still in flight.

Keys:

- `[` / `]`: previous/next group; `Alt-Left` / `Alt-Right`: previous/next session
  when multiple sessions were opened.
- `J` / `K`: next/previous visible message anchor in the current group.
- `1` user, `2` assistant text, `3` thinking, `4` tools (including tool-role
  results), `5` media, `6` other roles: visibility filters.
- `c`: collapse/expand the active thinking/tool section. Media remains a
  placeholder pending an explicit asset resolver.
- `/`, `n` / `N`: search within the current group; drag/`y`: select/copy;
  Tab/Enter: local links. Cross-group tool pairing is not yet resolved.
- `Esc`: dismiss search/selection first, then return to the picker;
  `q` / `Ctrl-C`: quit; `w`: open the session in the running web interface.

Resize, filters, and display-mode changes restore the active source anchor (or
its visible message when a block is hidden), rather than restoring a percentage
of rendered lines. Graphics use the shell's one-row header offset. Search inputs
own their keys so typing a digit or `q` does not toggle a filter or quit.

The standalone document comparison launcher above remains useful for CSS work;
its `b` native/bridge toggle is distinct from the embedded session viewer, which
currently uses nthtml's native renderer and its TTY mode.

## Upstream handoffs

Copyable task prompts are saved in the repo:

- [nttypeset: generic styled sections and source identity](../../etc/prompts/conversation-nttypeset.txt)
- [nthtml: preserve styles and anchors through its bridge](../../etc/prompts/conversation-nthtml.txt)
- [gloss: styled HTML viewing without a Markdown round trip](../../etc/prompts/conversation-gloss.txt)

The nttypeset and nthtml prompts have been implemented in the local commits
listed above; retain these prompts as handoff context. Gloss's native nthtml
viewing path remains to be implemented.
Each prompt limits edits to its own repository and asks for concrete API,
validation, and dependency/version handoffs.
