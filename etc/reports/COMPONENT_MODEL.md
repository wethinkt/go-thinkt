# Component Model Analysis

This report analyzes the current component architecture and proposes a path toward embeddable, composable views.

---

## Part 1: Current Architecture

### 1.1 System Boundaries

```
┌─────────────────────────────────────────────────────────────────────┐
│                    thinking-tracer (TypeScript)                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
│  │  3D View    │  │  Metrics    │  │  Detail     │                 │
│  │  (Three.js) │  │  Dashboard  │  │  Panel      │                 │
│  └─────────────┘  └─────────────┘  └─────────────┘                 │
│                          ↑                                          │
│                     JSONL Files                                     │
└─────────────────────────────────────────────────────────────────────┘
                           ↑
                      File System
                           ↑
┌─────────────────────────────────────────────────────────────────────┐
│                   thinking-tracer-tools (Go)                         │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                        TUI (BubbleTea)                       │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────────────┐  │   │
│  │  │ Projects │  │ Sessions │  │ Content (Viewport)       │  │   │
│  │  │ Column   │  │ Column   │  │ ├─ Renderer (Markdown)   │  │   │
│  │  │          │  │          │  │ └─ LazySession           │  │   │
│  │  └──────────┘  └──────────┘  └──────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                          ↑                                          │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Store Layer (thinkt)                      │   │
│  │  ┌─────────────┐        ┌─────────────┐                     │   │
│  │  │ Claude Store│        │ Kimi Store  │                     │   │
│  │  └─────────────┘        └─────────────┘                     │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### 1.2 Current Component Inventory

#### Data Layer (`internal/thinkt`)
| Component | Type | Embeddable | Notes |
|-----------|------|------------|-------|
| `Store` interface | Abstract | Yes | Clean interface for data access |
| `StoreRegistry` | Concrete | Yes | Manages multiple stores |
| `Entry`, `Session`, `Project` | Types | Yes | Pure data types |
| `SessionReader` | Interface | Yes | Streaming abstraction |

#### Claude-Specific (`internal/claude`)
| Component | Type | Embeddable | Notes |
|-----------|------|------------|-------|
| `Parser` | Concrete | Yes | JSONL parsing |
| `LazySession` | Concrete | Yes | Incremental loading |
| `Store` | Concrete | Yes | Implements thinkt.Store |

#### Kimi-Specific (`internal/kimi`)
| Component | Type | Embeddable | Notes |
|-----------|------|------------|-------|
| `Store` | Concrete | Yes | Implements thinkt.Store |
| `kimiReader` | Concrete | Yes | Chunked session support |

#### TUI Layer (`internal/tui`)
| Component | Type | Embeddable | Notes |
|-----------|------|------------|-------|
| `Model` | BubbleTea | No | Tightly coupled orchestrator |
| `projectsModel` | BubbleTea | Partial | Requires BubbleTea context |
| `sessionsModel` | BubbleTea | Partial | Requires BubbleTea context |
| `contentModel` | BubbleTea | Partial | Requires BubbleTea context |
| `RenderSession()` | Pure function | **Yes** | Only depends on Session + width |
| `ViewerModel` | BubbleTea | Yes | Standalone program |
| `SessionPickerModel` | BubbleTea | Yes | Standalone program |
| `ConfirmModel` | BubbleTea | Yes | Standalone dialog |

### 1.3 Component Coupling Analysis

**Tightly Coupled:**
```
Model
├── BubbleTea runtime (tea.Model interface)
├── Terminal dimensions (tea.WindowSizeMsg)
├── Keyboard handling (key.Matches)
├── All three column models (direct field access)
└── Message types (custom msg structs)
```

**Loosely Coupled:**
```
RenderSession(session *Session, width int) string
├── Only needs Session data + width
├── Pure function, no side effects
├── Returns styled string
└── Could be used by any renderer
```

**Well Abstracted:**
```
Store interface
├── ListProjects(ctx) → []Project
├── ListSessions(ctx, projectID) → []SessionMeta
├── LoadSession(ctx, sessionID) → *Session
└── OpenSession(ctx, sessionID) → SessionReader
```

---

## Part 2: Componentization Opportunities

### 2.1 Proposed Component Hierarchy

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Embeddable Components                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    View Components                           │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │   │
│  │  │Conversation │  │  3D View    │  │  Metrics View       │ │   │
│  │  │   Pane      │  │  (WebGL)    │  │  (Charts)           │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────┘ │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │   │
│  │  │  Timeline   │  │  Tree View  │  │  Search Results     │ │   │
│  │  │   View      │  │  (Branches) │  │  View               │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              ↑                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                   Render Components                          │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │   │
│  │  │  Markdown   │  │  JSON       │  │  Syntax             │ │   │
│  │  │  Renderer   │  │  Renderer   │  │  Highlighter        │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────┘ │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │   │
│  │  │  Thinking   │  │  Tool Call  │  │  Diff               │ │   │
│  │  │  Block      │  │  Block      │  │  Renderer           │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                              ↑                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Data Components                           │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │   │
│  │  │  Session    │  │  Entry      │  │  Content Block      │ │   │
│  │  │  Provider   │  │  Iterator   │  │  Parser             │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────┘ │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │   │
│  │  │  Store      │  │  Filter     │  │  Aggregator         │ │   │
│  │  │  Registry   │  │  Pipeline   │  │  (Stats)            │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 Interface Contracts for Embeddable Views

#### Conversation Pane Interface
```go
// ConversationPane renders a conversation for display.
type ConversationPane interface {
    // SetSession sets the session to display.
    SetSession(session *Session)

    // SetEntryFilter filters which entries to show.
    SetEntryFilter(filter func(Entry) bool)

    // SetRenderOptions configures rendering.
    SetRenderOptions(opts RenderOptions)

    // Render produces output for the target.
    Render(target RenderTarget) error

    // OnEntryClick registers a callback for entry selection.
    OnEntryClick(callback func(entry *Entry))
}

type RenderTarget interface {
    // Terminal targets
    WriteTerminal(width int) string

    // Web targets
    WriteHTML() string
    WriteJSON() []byte
}

type RenderOptions struct {
    ShowThinking     bool
    ThinkingMaxLen   int
    ShowToolCalls    bool
    ShowToolResults  bool
    ShowTimestamps   bool
    ShowTokenUsage   bool
    SyntaxHighlight  bool
    Theme            string
}
```

#### Metrics View Interface
```go
// MetricsView provides statistics about sessions.
type MetricsView interface {
    // SetSessions sets the sessions to analyze.
    SetSessions(sessions []*Session)

    // GetTurnMetrics returns per-turn statistics.
    GetTurnMetrics() []TurnMetrics

    // GetSessionMetrics returns session-level statistics.
    GetSessionMetrics() SessionMetrics

    // Render produces output for the target.
    Render(target RenderTarget, chartType ChartType) error
}

type TurnMetrics struct {
    TurnIndex      int
    InputTokens    int
    OutputTokens   int
    ThinkingBlocks int
    ToolCalls      int
    ContentLength  int
    Duration       time.Duration
}

type SessionMetrics struct {
    TotalTurns       int
    TotalTokens      int
    TotalThinking    int
    TotalToolCalls   int
    AverageTurnSize  int
    Duration         time.Duration
    Models           []string
}
```

#### 3D View Interface (for thinking-tracer integration)
```go
// ThreeDView provides data for 3D visualization.
type ThreeDView interface {
    // SetSession sets the session to visualize.
    SetSession(session *Session)

    // GetSpiralData returns data for spiral layout.
    GetSpiralData() []SpiralCluster

    // GetNodeData returns data for individual nodes.
    GetNodeData() []NodeData

    // ExportForThreeJS produces JSON for Three.js consumption.
    ExportForThreeJS() ([]byte, error)
}

type SpiralCluster struct {
    TurnIndex   int
    Position    Vector3
    UserEntry   *Entry
    AssistEntry *Entry
    Expanded    bool
}

type NodeData struct {
    ID          string
    Type        string // "user", "assistant", "thinking", "tool_use", "tool_result"
    Content     string
    Position    Vector3
    Parent      string
    Children    []string
}
```

### 2.3 Rendering Pipeline Refactor

**Current:**
```
Session → RenderSession() → Styled String → Viewport
```

**Proposed:**
```
Session
    ↓
EntryStream (filtered, windowed)
    ↓
┌─────────────────────────────────────────┐
│           Render Pipeline               │
│  ┌─────────────┐                        │
│  │ Entry       │→ ContentBlock[]        │
│  │ Expander    │                        │
│  └─────────────┘                        │
│        ↓                                │
│  ┌─────────────┐                        │
│  │ Block       │→ RenderFragment[]      │
│  │ Renderers   │   (text, html, json)   │
│  └─────────────┘                        │
│        ↓                                │
│  ┌─────────────┐                        │
│  │ Layout      │→ Positioned Fragments  │
│  │ Engine      │                        │
│  └─────────────┘                        │
│        ↓                                │
│  ┌─────────────┐                        │
│  │ Target      │→ Final Output          │
│  │ Renderer    │   (terminal/html/json) │
│  └─────────────┘                        │
└─────────────────────────────────────────┘
```

### 2.4 Block Renderer Registry

```go
// BlockRenderer renders a specific content block type.
type BlockRenderer interface {
    // CanRender returns true if this renderer handles the block type.
    CanRender(block ContentBlock) bool

    // Render produces a RenderFragment for the block.
    Render(block ContentBlock, opts RenderOptions) RenderFragment
}

// RenderFragment is a piece of rendered content.
type RenderFragment struct {
    Type     string // "text", "code", "thinking", "tool", "error"
    Content  string
    Style    Style
    Metadata map[string]any
}

// BlockRendererRegistry manages block renderers.
type BlockRendererRegistry struct {
    renderers []BlockRenderer
}

func (r *BlockRendererRegistry) Register(renderer BlockRenderer) {
    r.renderers = append(r.renderers, renderer)
}

func (r *BlockRendererRegistry) Render(block ContentBlock, opts RenderOptions) RenderFragment {
    for _, renderer := range r.renderers {
        if renderer.CanRender(block) {
            return renderer.Render(block, opts)
        }
    }
    return RenderFragment{Type: "unknown", Content: fmt.Sprintf("[%s]", block.Type)}
}

// Built-in renderers
type TextBlockRenderer struct{}
type ThinkingBlockRenderer struct{}
type ToolUseBlockRenderer struct{}
type ToolResultBlockRenderer struct{}
type CodeBlockRenderer struct{}
type ImageBlockRenderer struct{}
```

---

## Part 3: Migration Path

### Phase 1: Extract Pure Rendering (Low Risk)

1. **Create `internal/render` package**
   - Move `RenderSession()` logic
   - Define `BlockRenderer` interface
   - Implement block-specific renderers
   - Support multiple output formats (terminal, HTML, JSON)

2. **Create `internal/view` package**
   - Define view interfaces (`ConversationPane`, `MetricsView`)
   - Implement terminal-based views using existing lipgloss styles
   - Keep BubbleTea integration in `internal/tui`

### Phase 2: Decouple TUI Components (Medium Risk)

3. **Extract column models into reusable components**
   - Create `ListPane` abstraction over `list.Model`
   - Create `ContentPane` abstraction over `viewport.Model`
   - Define message protocols for inter-component communication

4. **Create component factory**
   ```go
   type ComponentFactory interface {
       CreateProjectList(store Store) Component
       CreateSessionList(projectID string, store Store) Component
       CreateContentView(session *Session) Component
   }
   ```

### Phase 3: Cross-Platform Support (Higher Risk)

5. **Add Web rendering target**
   - HTML output from render pipeline
   - JSON export for JavaScript consumption
   - WebSocket protocol for live updates

6. **thinking-tracer integration**
   - Shared data format specification
   - Go server that serves session data as JSON
   - Three.js client consumes via HTTP/WebSocket

---

## Part 4: Proposed Package Structure

```
internal/
├── thinkt/           # Core types and store interface (existing)
│   ├── types.go
│   └── store.go
│
├── claude/           # Claude-specific (existing)
├── kimi/             # Kimi-specific (existing)
│
├── render/           # NEW: Rendering pipeline
│   ├── pipeline.go       # RenderPipeline orchestrator
│   ├── fragment.go       # RenderFragment type
│   ├── options.go        # RenderOptions
│   ├── registry.go       # BlockRendererRegistry
│   │
│   ├── blocks/           # Block renderers
│   │   ├── text.go
│   │   ├── thinking.go
│   │   ├── tool_use.go
│   │   ├── tool_result.go
│   │   ├── code.go
│   │   └── image.go
│   │
│   └── targets/          # Output targets
│       ├── terminal.go   # Lipgloss styling
│       ├── html.go       # HTML output
│       ├── json.go       # JSON output
│       └── markdown.go   # Markdown output
│
├── view/             # NEW: Embeddable views
│   ├── conversation.go   # ConversationPane
│   ├── metrics.go        # MetricsView
│   ├── timeline.go       # TimelineView
│   ├── tree.go           # TreeView (for branches)
│   └── search.go         # SearchResultsView
│
├── tui/              # Terminal UI (existing, refactored)
│   ├── model.go          # Top-level model
│   ├── panes/            # Reusable pane components
│   │   ├── list.go       # Generic list pane
│   │   ├── content.go    # Content viewport pane
│   │   └── header.go     # Header pane
│   ├── styles.go
│   ├── keys.go
│   └── messages.go
│
├── export/           # NEW: Export formats
│   ├── html.go           # Styled HTML export
│   ├── markdown.go       # Markdown export
│   └── threejs.go        # Three.js JSON export
│
└── server/           # NEW: HTTP server for web views
    ├── server.go         # HTTP server
    ├── handlers.go       # API handlers
    └── websocket.go      # Live updates
```

---

## Part 5: Integration Points

### 5.1 Embedding a Conversation Pane

```go
// Example: Embed conversation pane in another BubbleTea app
import (
    "github.com/Brain-STM-org/thinking-tracer-tools/internal/view"
    "github.com/Brain-STM-org/thinking-tracer-tools/internal/render"
)

func createConversationPane(session *thinkt.Session) view.ConversationPane {
    pane := view.NewConversationPane()
    pane.SetSession(session)
    pane.SetRenderOptions(render.RenderOptions{
        ShowThinking:   true,
        ThinkingMaxLen: 500,
        ShowToolCalls:  true,
        Theme:          "dark",
    })
    return pane
}

// In BubbleTea model
func (m Model) View() string {
    return m.conversationPane.Render(render.TerminalTarget{Width: m.width})
}
```

### 5.2 Embedding Metrics View

```go
// Example: Generate metrics report
import "github.com/Brain-STM-org/thinking-tracer-tools/internal/view"

func generateMetricsReport(sessions []*thinkt.Session) string {
    metrics := view.NewMetricsView()
    metrics.SetSessions(sessions)

    var buf bytes.Buffer
    metrics.Render(render.MarkdownTarget{Writer: &buf}, view.ChartTypeBar)
    return buf.String()
}
```

### 5.3 thinking-tracer Integration

```go
// Server endpoint for Three.js consumption
func handleSessionData(w http.ResponseWriter, r *http.Request) {
    sessionID := r.URL.Query().Get("session")

    session, _ := store.LoadSession(ctx, sessionID)

    exporter := export.NewThreeJSExporter()
    data, _ := exporter.Export(session)

    w.Header().Set("Content-Type", "application/json")
    w.Write(data)
}
```

---

## Part 6: Summary

### Current State
- Data layer (thinkt, claude, kimi) is **well abstracted**
- Rendering is **partially decoupled** (`RenderSession()` is pure)
- TUI is **tightly coupled** to BubbleTea
- No web/HTML rendering capability
- No structured export for 3D visualization

### Recommended Actions

| Priority | Action | Effort | Impact |
|----------|--------|--------|--------|
| 1 | Extract `internal/render` package | Medium | High |
| 2 | Define view interfaces | Low | Medium |
| 3 | Add HTML/JSON render targets | Medium | High |
| 4 | Create Three.js exporter | Medium | High |
| 5 | Add HTTP server for web views | High | High |
| 6 | Refactor TUI into reusable panes | High | Medium |

### Key Design Principles

1. **Separation of Concerns**: Data → Render → View → Target
2. **Interface-First**: Define contracts before implementations
3. **Progressive Enhancement**: Terminal first, web second
4. **Composability**: Small, focused components that combine
5. **No Tight Coupling**: Views should work without BubbleTea

---

*Report generated: 2026-01-29*
*Based on: thinking-tracer-tools current state*
