# LSP Module Integration — Feasibility Analysis & Development Plan

> **Status:** Draft for review  
> **Date:** 2026-05-24  
> **Author:** evva (coding agent)  
> **Target:** evva v0.4.x or later

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Reference Architecture Analysis](#2-reference-architecture-analysis)
3. [evva Architecture Gap Analysis](#3-evva-architecture-gap-analysis)
4. [Feasibility Assessment](#4-feasibility-assessment)
5. [Technical Design](#5-technical-design)
6. [Implementation Plan](#6-implementation-plan)
7. [Risk Analysis & Mitigation](#7-risk-analysis--mitigation)
8. [Open Questions](#8-open-questions)
9. [Appendix: Key Reference Files](#9-appendix-key-reference-files)

---

## 1. Executive Summary

### 1.1 What is an LSP Module?

An LSP (Language Server Protocol) module would give evva deep, semantic understanding of code. Instead of grepping for strings or guessing symbol locations, the agent could ask a language server running as a subprocess: "Where is `UserService` defined?", "Show me all callers of `authenticate`", "What type is this variable?" — and get precise, compiler-grade answers.

This is a significant capability upgrade. Currently evva's code exploration relies on `grep`, `glob`, `read`, and `tree` — lexical tools that can't reason about scope, types, or symbol relationships. An LSP module bridges that gap.

### 1.2 Bottom Line

**Feasible, with low-to-medium complexity for a core MVP (Phase 1).** evva's daemon infrastructure, tool system, and event bus already provide the plumbing needed — no architectural rewrites required. The primary new work is a Go-native JSON-RPC 2.0 over stdio transport and LSP protocol type definitions. Estimated scope: ~1,500–2,500 lines of new Go code for Phase 1, spread across 3–4 new packages.

### 1.3 Recommended Approach

A phased rollout:

| Phase | Scope | Effort | Delivers |
|---|---|---|---|
| **Phase 1 — Core LSP Client** | JSON-RPC transport, server lifecycle, basic operations (definition, references, hover, document symbols) | ~2 weeks | Agent can query LSP servers |
| **Phase 2 — Diagnostics** | Passive `textDocument/publishDiagnostics`, dedup, volume limiting, delivery as daemon signals | ~1 week | Real-time error/warning feedback |
| **Phase 3 — Advanced Operations** | Workspace symbols, call hierarchy, go-to-implementation | ~1 week | Full feature parity with Claude Code |
| **Phase 4 — Server Discovery** | Auto-detection from project configs, marketplace integration (future) | ~1–2 weeks | Zero-config LSP |

---

## 2. Reference Architecture Analysis

The reference implementation (Claude Code, TypeScript) uses a five-layer architecture spanning ~4,500 lines across 16 files. Below is a detailed breakdown.

### 2.1 Layer Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  LSPTool.ts              AI-Facing Tool Layer                │
│  - 9 operations (definition, references, hover, symbols...)  │
│  - Input validation, filesystem permissions, gitignore filter│
│  - Waits for server init, opens files via didOpen            │
│  - Formats results for LLM display                           │
└──────────────────────────┬───────────────────────────────────┘
                           │
┌──────────────────────────▼───────────────────────────────────┐
│  manager.ts              Global Singleton Orchestrator       │
│  - initializeLspServerManager() / shutdown / reinitialize    │
│  - isLspConnected() for tool enablement                      │
│  - waitForInitialization() API                               │
└──────────────────────────┬───────────────────────────────────┘
                           │
┌──────────────────────────▼───────────────────────────────────┐
│  LSPServerManager.ts     File-to-Server Router               │
│  - extension → server name → LSPServerInstance map           │
│  - ensureServerStarted(filePath) — lazy start                │
│  - didOpen / didChange / didSave / didClose sync             │
│  - workspace/configuration handler                           │
└──────────────────────────┬───────────────────────────────────┘
                           │
┌──────────────────────────▼───────────────────────────────────┐
│  LSPServerInstance.ts    Single Server Lifecycle             │
│  - State machine: stopped→starting→running→stopping→error    │
│  - Crash recovery (maxRestarts cap)                          │
│  - Request retry with exponential backoff (ContentModified)  │
│  - Startup timeout racing                                    │
└──────────────────────────┬───────────────────────────────────┘
                           │
┌──────────────────────────▼───────────────────────────────────┐
│  LSPClient.ts            JSON-RPC Transport                  │
│  - spawn() child process (stdio pipes)                       │
│  - vscode-jsonrpc (StreamMessageReader/Writer)               │
│  - initialize handshake (capabilities exchange)              │
│  - sendRequest / sendNotification / onNotification           │
└──────────────────────────────────────────────────────────────┘
```

### 2.2 LSP Operations Implemented

| Operation | LSP Method | Two-Step? |
|---|---|---|
| `goToDefinition` | `textDocument/definition` | No |
| `findReferences` | `textDocument/references` | No |
| `hover` | `textDocument/hover` | No |
| `documentSymbol` | `textDocument/documentSymbol` | No |
| `workspaceSymbol` | `workspace/symbol` | No |
| `goToImplementation` | `textDocument/implementation` | No |
| `prepareCallHierarchy` | `textDocument/prepareCallHierarchy` | Yes (→ incoming/outgoing) |
| `incomingCalls` | `callHierarchy/incomingCalls` | Yes |
| `outgoingCalls` | `callHierarchy/outgoingCalls` | Yes |

**Notably absent:** Completion, code actions, signature help, code lens, formatting — these were deemed lower priority for a coding agent that primarily reads and navigates code.

### 2.3 Diagnostics Pathway (Passive, Async)

```
LSP Server ──publishDiagnostics──► passiveFeedback.ts
  └─ formatDiagnosticsForAttachment()
  └─ registerPendingLSPDiagnostic()  → LSPDiagnosticRegistry
  └─ checkForLSPDiagnostics()        → query pipeline
  └─ deduplication + volume limiting → delivered as attachments
```

Key limits in the reference:
- **Per-file cap:** 10 diagnostics
- **Total cap:** 30 diagnostics
- **Dedup:** Hashing `{message, severity, range, source, code}`; cross-turn LRU of 500 delivered keys
- **Delivery:** Attachments injected into the conversation context

### 2.4 Server Configuration

LSP servers are discovered from plugins, not hardcoded:

```typescript
// Config shape
interface LspServerConfig {
  command: string                          // e.g. "typescript-language-server"
  args?: string[]                          // e.g. ["--stdio"]
  extensionToLanguage: Record<string, string>  // e.g. { ".ts": "typescript" }
  transport?: 'stdio' | 'socket'          // only stdio implemented
  env?: Record<string, string>
  initializationOptions?: unknown
  startupTimeout?: number
  maxRestarts?: number                    // default 3
}
```

Server names are scoped: `plugin:{pluginName}:{serverName}` to avoid collisions.

### 2.5 Key Design Decisions Worth Porting

1. **Lazy server start** — servers launch only when `ensureServerStarted()` is called for a concrete file, not at agent boot. This avoids starting 10+ servers unnecessarily.
2. **State machine per server** — explicit `stopped | starting | running | stopping | error` states prevent race conditions during concurrent tool calls.
3. **Crash recovery with cap** — servers that crash repeatedly hit `maxRestarts` and stop retrying. Prevents infinite restart loops.
4. **`ContentModified` retry** — rust-analyzer sends this during indexing; the reference retries with exponential backoff (500ms → 1000ms → 2000ms).
5. **Diagnostics dedup** — same diagnostic from same file/server isn't re-delivered across turns.
6. **Volume limiting** — diagnostics are capped per-file and globally so they don't flood the LLM context.

### 2.6 What NOT to Port

1. **Plugin-based server discovery (Phase 1)** — evva doesn't have a plugin system yet. Start with a simple config file.
2. **`vscode-jsonrpc` dependency** — evva is Go, not Node.js. A ~200-line JSON-RPC 2.0 implementation over stdio is sufficient.
3. **`workspace/configuration` handler** — the reference returns `null` for every config item. Skip entirely; evva can omit the capability declaration.
4. **React Ink UI components** — evva's TUI is Bubble Tea; LSP results render as standard tool result text. No special UI needed initially.

---

## 3. evva Architecture Gap Analysis

### 3.1 What Already Exists (Ready to Use)

| Capability | Where | Ready? |
|---|---|---|
| Long-running subprocess management | `pkg/tools/daemon/` — `Daemon` interface, `DaemonState`, `Kind*` constants, signal draining | Yes |
| Parallel tool execution | `internal/agent/loop.go` — goroutine-per-tool dispatch | Yes |
| Event publication to UI | `pkg/event/` — `Event` envelope, `Sink` contract | Yes |
| Tool registration & discovery | `pkg/toolset/` — `Registry`, `ToolName`, active/deferred/resolved phases | Yes |
| Permission gating | `internal/permission/` — gate, broker, matcher | Yes |
| Provider-agnostic LLM interface | `pkg/llm/` — `Client`, message types | Yes |
| Config loading | `pkg/config/` — YAML + env | Yes |
| User-facing tool descriptions | `pkg/tools/tool.go` — `Description()` method | Yes |

### 3.2 What Must Be Built

| Capability | Complexity | Notes |
|---|---|---|
| **JSON-RPC 2.0 transport over stdio** | Medium | ~200–400 lines. Go has no dominant library; write a focused one. Must handle header-based message framing (`Content-Length: ...\r\n\r\n`), request/response correlation, notification dispatch. |
| **LSP protocol types** | Low-Medium | ~300–500 lines of Go structs. Can be generated from the LSP metamodel or hand-written for the subset we need. `go.lsp.dev/protocol` exists but may be overkill. |
| **LSP server lifecycle manager** | Medium | ~400–600 lines. State machine, lazy start, crash recovery, file-to-server routing. Analogous to `LSPServerInstance.ts` + `LSPServerManager.ts` merged. |
| **LSP tool (`lsp_request` or similar)** | Medium | ~400–600 lines. Implements `tools.Tool`, dispatches to correct LSP method, formats results. |
| **Diagnostic delivery pathway** | Medium | ~300–500 lines. Daemon signal handler, dedup registry, volume limiting, context injection. |
| **LSP server config format** | Low | ~100–200 lines. YAML-based config file, extension-to-server mapping, command/args/env. |

### 3.3 What Does NOT Need to Change

- **Agent loop** (`internal/agent/loop.go`) — daemon signals already flow through `drainDaemonSignals()`. LSP diagnostics arrive as signals, no loop changes needed.
- **Event system** (`pkg/event/`) — existing `KindStoreUpdate` and `BgResult`/`DrainBackgroundTask` events carry daemon lifecycle updates. Add `LSPMeta` to daemon snapshot types.
- **UI contract** (`pkg/ui/`) — LSP results are text; UI renders them like any tool result.
- **LLM providers** — zero changes. The LLM sees LSP as just another tool.
- **Permission system** — existing `PreToolUse` hooks and gate can gate LSP server launches if needed.
- **Config system** — LSP config is a new YAML file, loaded alongside existing config.

---

## 4. Feasibility Assessment

### 4.1 Technical Feasibility: HIGH

Go's standard library provides everything needed:
- `os/exec` — subprocess spawning with stdin/stdout pipes (already used by `bash.go`)
- `bufio` — line-based reading for JSON-RPC header parsing
- `encoding/json` — message serialization
- `sync` — mutexes, WaitGroups for concurrent request tracking
- `context` — cancellation propagation

The JSON-RPC framing is straightforward: each message is prefixed with `Content-Length: N\r\n\r\n` followed by N bytes of JSON. A reader goroutine per server is sufficient.

### 4.2 Architectural Compatibility: HIGH

The LSP module fits naturally into evva's design:

```
LSP Server Process ──stdio──► lspDaemon (implements daemon.Daemon)
  ├─ JSON-RPC messages → responses routed to pending requests
  └─ notifications (diagnostics) → daemon signals → agent loop → LLM context

LSP Tool (implements tools.Tool)
  ├─ Agent calls lsp_request with {operation, filePath, line, character}
  ├─ Tool resolves server via extension map
  ├─ Tool ensures server started (lazy)
  └─ Tool sends LSP request → formats result → returns Result{Content, Metadata}
```

This mirrors the existing `bash` tool's relationship with `bashDaemon` — a proven pattern in the codebase.

### 4.3 Effort Estimate

| Component | Lines (Go) | Days |
|---|---|---|
| JSON-RPC transport | 200–350 | 2–3 |
| LSP protocol types | 300–500 | 2–3 |
| Server lifecycle + manager | 400–600 | 3–4 |
| LSP tool + formatters | 400–600 | 3–4 |
| Diagnostics pathway | 300–500 | 2–3 |
| Config format + loader | 100–200 | 1–2 |
| Tests | 400–600 | 2–3 |
| **Total (Phase 1)** | **~2,100–3,350** | **15–22** |

### 4.4 Key Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| JSON-RPC edge cases (chunked messages, malformed headers) | Medium | Medium | Thorough fuzz testing; reference LSP spec test suite |
| Server-specific quirks (rust-analyzer, gopls, typescript-language-server) | High | Low | Port `ContentModified` retry; test with top 3 servers |
| LSP server binary not installed | High | Low | Graceful error messages; recommend install commands |
| Go stdlib JSON performance for large responses | Low | Low | Most LSP responses are small; streaming for workspace/symbol if needed |
| `go.lsp.dev/protocol` dependency maintenance | Low | Low | Vendor a minimal type subset ourselves |

### 4.5 Go Ecosystem Assessment

**JSON-RPC libraries evaluated:**

| Library | Pros | Cons |
|---|---|---|
| `net/rpc/jsonrpc` (stdlib) | No dependency; reliable | Not JSON-RPC 2.0; no notification support |
| `github.com/sourcegraph/jsonrpc2` | Full 2.0; async handler; used in production (Sourcegraph) | Extra dependency; might be heavier than needed |
| Write our own | Zero deps; precise fit; ~200 lines | Maintenance burden; bug surface |

**Recommendation:** Start with `github.com/sourcegraph/jsonrpc2` for Phase 1 (it's battle-tested in a Go code intelligence platform). It provides async request/response correlation, notification dispatch, and proper error codes. Re-evaluate replacing with an in-house implementation in Phase 2 if dependency weight becomes a concern.

**LSP type libraries:**

| Library | Pros | Cons |
|---|---|---|
| `go.lsp.dev/protocol` | Complete LSP 3.17 types; generated from metamodel | Heavy dependency chain; may lag spec |
| Hand-written subset | Zero deps; only the types we use; lightweight | Manual maintenance; risk of omission |

**Recommendation:** Hand-write a minimal type set (~20 structs) covering the 9 operations we implement plus diagnostics. LSP types are stable and well-documented; the maintenance burden is low. This avoids a dependency that pulls in the entire LSP metamodel.

---

## 5. Technical Design

### 5.1 New Package Layout

```
pkg/tools/lsp/                      # Public LSP tool package
├── lsp.go                          # ToolName constants, Names(), family registration
├── client.go                       # JSON-RPC 2.0 transport over stdio
├── server.go                       # LSP server lifecycle (state machine, lazy start, crash recovery)
├── manager.go                      # Extension-to-server routing, file sync (didOpen/Change/Save/Close)
├── tool.go                         # tools.Tool implementation (lsp_request)
├── formatters.go                   # Result formatters for each operation
├── diagnostics.go                  # publishDiagnostics handler, dedup, volume limiting
├── config.go                       # LSP server config struct + YAML loader
├── protocol/                       # LSP protocol types
│   ├── types.go                    # Core types: Position, Range, Location, TextDocumentIdentifier...
│   ├── methods.go                  # Method constants + param/result types
│   └── capabilities.go             # ServerCapabilities, ClientCapabilities
├── client_test.go
├── server_test.go
├── manager_test.go
├── tool_test.go
└── diagnostics_test.go
```

### 5.2 Daemon Integration

Add `KindLSP` to the daemon system:

```go
// pkg/tools/daemon/kind.go
const (
    KindBash   DaemonKind = "local_bash"
    KindAgent  DaemonKind = "local_agent"
    KindMonitor DaemonKind = "monitor"
    KindLSP    DaemonKind = "lsp"        // NEW
)
// ID prefix: "l" (e.g., "l1", "l2")
```

The `lspDaemon` struct:

```go
type lspDaemon struct {
    id       string
    server   *LSPServer               // wraps client + lifecycle
    exitCode int
    err      error
    mu       sync.RWMutex
}
```

Implements `daemon.Daemon`:
- `Snapshot()` → `DaemonSnapshot{Kind: KindLSP, Status, Extras: LSPMeta{...}}`
- `Kill(ctx)` → sends `shutdown` + `exit` to LSP, kills process
- `Output()` → recent log output

### 5.3 JSON-RPC Transport Design

```go
// client.go — core transport

type Client struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser

    mu       sync.Mutex
    pending  map[json.RawMessage]chan *Response  // request ID → response channel
    handlers map[string]NotificationHandler      // method → handler
    nextID   int64

    connCtx    context.Context
    connCancel context.CancelFunc
}

func Start(ctx context.Context, command string, args []string) (*Client, error)
func (c *Client) Request(ctx context.Context, method string, params any) (json.RawMessage, error)
func (c *Client) Notify(ctx context.Context, method string, params any) error
func (c *Client) OnNotify(method string, handler func(params json.RawMessage))
func (c *Client) Close() error
```

Key design points:
- **Header-based framing:** Read `Content-Length: N\r\n\r\n`, then read N bytes of JSON. This is the standard LSP framing.
- **Request correlation:** `id` field (int64, monotonically increasing). Response carries the same `id`. A `map[id]chan` routes responses to callers.
- **Concurrent-safe:** Mutex protects the pending map and nextID. A single reader goroutine dispatches incoming messages.
- **Graceful shutdown:** Send `shutdown` request → wait for response → send `exit` notification → close pipes → kill process.

### 5.4 Server Lifecycle Design

```go
// server.go

type State int
const (
    StateStopped  State = iota
    StateStarting
    StateRunning
    StateStopping
    StateError
)

type Server struct {
    Name        string
    Config      LspServerConfig
    Client      *Client
    State       State
    Capabilities ServerCapabilities

    restartCount   int
    maxRestarts    int           // default 3
    startupTimeout time.Duration // default 30s

    mu sync.RWMutex
}

func (s *Server) Start(ctx context.Context) error
func (s *Server) Stop(ctx context.Context) error
func (s *Server) Restart(ctx context.Context) error
func (s *Server) IsHealthy() bool
func (s *Server) Request(ctx context.Context, method string, params any) (json.RawMessage, error)
func (s *Server) Notify(ctx context.Context, method string, params any) error

// State transitions:
//   Stopped → (Start) → Starting → (initialize response) → Running
//   Running → (Stop) → Stopping → (process dead) → Stopped
//   Running → (crash) → Error → (restart) → Starting → Running
//   Error → (maxRestarts exceeded) → Stopped (permanent)
```

### 5.5 Manager Design

```go
// manager.go

type Manager struct {
    servers map[string]*Server          // server name → server
    extMap  map[string]string            // file extension → server name
    openFiles map[string]string          // file URI → language ID

    daemonState *daemon.DaemonState      // for registering LSP daemons

    mu sync.RWMutex
}

func NewManager(configs []LspServerConfig, ds *daemon.DaemonState) *Manager

// Core operations
func (m *Manager) ServerForFile(filePath string) (*Server, bool)
func (m *Manager) EnsureServerStarted(ctx context.Context, filePath string) (*Server, error)
func (m *Manager) Request(ctx context.Context, filePath, method string, params any) (json.RawMessage, error)

// File synchronization (textDocument/didOpen, didChange, didSave, didClose)
func (m *Manager) OpenFile(ctx context.Context, filePath, content string) error
func (m *Manager) ChangeFile(ctx context.Context, filePath, content string) error
func (m *Manager) CloseFile(ctx context.Context, filePath string) error

// Lifecycle
func (m *Manager) Initialize(ctx context.Context) error
func (m *Manager) Shutdown(ctx context.Context) error
```

### 5.6 Tool Design

```go
// tool.go

// Single tool with an "operation" discriminator
type lspTool struct {
    manager *Manager
    workDir string
}

func (t *lspTool) Name() string        { return "lsp_request" }
func (t *lspTool) Description() string { /* detailed prompt for the LLM */ }
func (t *lspTool) Schema() json.RawMessage { /* JSON Schema with oneOf on operation */ }

func (t *lspTool) Execute(ctx context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
    // 1. Parse operation from input
    // 2. Validate filePath exists (if applicable)
    // 3. res := t.manager.EnsureServerStarted(ctx, filePath)
    // 4. Open file via didOpen (if using textDocument/* methods)
    // 5. Dispatch to correct LSP method based on operation
    // 6. Format result via formatters.go
    // 7. Return tools.Result{Content, Metadata}
}
```

Tool schema (discriminated union, 4 operations for Phase 1):

```json
{
  "type": "object",
  "required": ["operation"],
  "oneOf": [
    {
      "properties": {
        "operation": {"const": "go_to_definition"},
        "filePath": {"type": "string"},
        "line": {"type": "integer"},
        "character": {"type": "integer"}
      }
    },
    {
      "properties": {
        "operation": {"const": "find_references"},
        "filePath": {"type": "string"},
        "line": {"type": "integer"},
        "character": {"type": "integer"}
      }
    },
    {
      "properties": {
        "operation": {"const": "hover"},
        "filePath": {"type": "string"},
        "line": {"type": "integer"},
        "character": {"type": "integer"}
      }
    },
    {
      "properties": {
        "operation": {"const": "document_symbols"},
        "filePath": {"type": "string"}
      }
    }
  ]
}
```

### 5.7 Diagnostics Design

Diagnostics arrive as `textDocument/publishDiagnostics` notifications from the LSP server. They follow a different path from request/response operations — they are passive, not solicited by the agent.

```
LSP Server
  └─ publishDiagnostics notification
     └─ Client.OnNotify("textDocument/publishDiagnostics", handler)
        └─ diagnostics.go: handlePublishDiagnostics(params)
           └─ registry.Register(serverName, fileURI, diagnostics)
              └─ daemon signal emitted → agent loop drains it
                 └─ next turn: diagnostics injected as system reminder
```

The `DiagnosticRegistry`:

```go
type DiagnosticRegistry struct {
    pending       []PendingDiagnostic
    delivered     *lru.Cache[string, bool]   // fileURI + diagnosticKey → seen
    maxPerFile    int                          // 10
    maxTotal      int                          // 30
    mu            sync.Mutex
}

func (r *DiagnosticRegistry) Register(serverName, fileURI string, diags []Diagnostic)
func (r *DiagnosticRegistry) Drain() []PendingDiagnostic     // called by agent loop
func (r *DiagnosticRegistry) ClearFile(fileURI string)       // when file edited
```

**Delivery format** (injected as `<system-reminder>`):

```
LSP diagnostics from gopls for internal/agent/loop.go:
  [Error] Line 89: undefined: drainDaemonSignals
  [Warning] Line 142: unused variable 'result'
```

### 5.8 Configuration Format

```yaml
# ~/.evva/lsp_servers.yml  (or <project>/.evva/lsp_servers.yml)

servers:
  gopls:
    command: gopls
    args: []
    extensions:
      ".go": "go"
    env:
      GOPATH: "${HOME}/go"
    startupTimeout: "30s"
    maxRestarts: 3

  typescript:
    command: typescript-language-server
    args: ["--stdio"]
    extensions:
      ".ts": "typescript"
      ".tsx": "typescriptreact"
      ".js": "javascript"
      ".jsx": "javascriptreact"
    startupTimeout: "60s"

  rust-analyzer:
    command: rust-analyzer
    args: []
    extensions:
      ".rs": "rust"
    env:
      RUST_SRC_PATH: "/path/to/rust/src"
```

- **Project-level** config (`.evva/lsp_servers.yml`) overrides **user-level** config (`~/.evva/lsp_servers.yml`).
- Server names with same key at project level replace the user-level definition.
- Environment variable expansion (`${VAR}` and `${HOME}`) in `command`, `args`, and `env` values.

### 5.9 Integration with Agent Profiles

`lsp_request` is a **deferred** tool in the Main profile:

```go
// internal/agent/profiles.go — Main()
DeferredTools: append(existing,
    tools.ToolName("lsp_request"),
)
```

This means:
- The tool name appears in the LLM's tool list but without its full schema.
- The LLM uses `tool_search` to discover the schema when it needs LSP capabilities.
- This saves context window space — LSP is powerful but not needed every turn.

### 5.10 Initialization Handshake

When a server starts, the `initialize` request declares evva's capabilities:

```go
params := InitializeParams{
    ProcessID: os.Getpid(),
    RootURI:   workDirURI,
    Capabilities: ClientCapabilities{
        Workspace: nil,       // Don't claim workspace support
        TextDocument: &TextDocumentClientCapabilities{
            Synchronization: &TextDocumentSyncClientCapabilities{
                DidSave: true,
            },
            PublishDiagnostics: &PublishDiagnosticsClientCapabilities{
                RelatedInformation: true,
            },
            Hover: &HoverClientCapabilities{
                ContentFormat: []MarkupKind{"markdown", "plaintext"},
            },
            Definition: &DefinitionClientCapabilities{
                LinkSupport: true,
            },
            References: &ReferencesClientCapabilities{},
            DocumentSymbol: &DocumentSymbolClientCapabilities{
                HierarchicalDocumentSymbolSupport: true,
            },
            CallHierarchy: &CallHierarchyClientCapabilities{},   // Phase 3
        },
        General: &GeneralClientCapabilities{
            PositionEncodings: []PositionEncodingKind{"utf-16"},
        },
    },
}
```

Position encoding is `utf-16` because that's what most LSP servers expect (matching JavaScript/TypeScript string indexing). The tool converts Go source positions (byte offsets or rune counts) to UTF-16 code units before sending.

---

## 6. Implementation Plan

### Phase 1 — Core LSP Client (MVP)

**Goal:** Agent can query an LSP server for definition, references, hover, and document symbols.

**Tasks:**

| # | Task | Files | Dependencies |
|---|---|---|---|
| 1.1 | Add `KindLSP` to daemon kind constants | `pkg/tools/daemon/kind.go` | None |
| 1.2 | Add `lsp_request` ToolName constant + deferred tags | `pkg/tools/name.go`, `pkg/toolset/tags.go` | 1.1 |
| 1.3 | Implement LSP protocol types | `pkg/tools/lsp/protocol/*.go` | None |
| 1.4 | Implement JSON-RPC 2.0 over stdio transport | `pkg/tools/lsp/client.go` | 1.3 |
| 1.5 | Implement LSP server lifecycle (state machine, start/stop/restart) | `pkg/tools/lsp/server.go` | 1.4 |
| 1.6 | Implement extension-to-server manager (routing, file sync) | `pkg/tools/lsp/manager.go` | 1.5 |
| 1.7 | Implement LSP config loader (YAML) | `pkg/tools/lsp/config.go` | None |
| 1.8 | Implement lspDaemon (daemon.Daemon adapter) | `pkg/tools/lsp/daemon.go` | 1.1, 1.5 |
| 1.9 | Implement lspTool (tools.Tool: definition, references, hover, documentSymbols) | `pkg/tools/lsp/tool.go` | 1.6 |
| 1.10 | Implement result formatters | `pkg/tools/lsp/formatters.go` | 1.9 |
| 1.11 | Register tool factory in builtins | `internal/toolset/builtins.go` | 1.9 |
| 1.12 | Add to Main profile deferred tools | `internal/agent/profiles.go` | 1.2 |
| 1.13 | Add daemon signal handling for LSP lifecycle | `internal/agent/drain_daemons.go` | 1.8 |
| 1.14 | Write tests (client, server, manager, tool) | `pkg/tools/lsp/*_test.go` | All above |
| 1.15 | Integration test with gopls on evva's own codebase | Manual | All above |

**Verification criteria:**
- `evva` can call `lsp_request` with `operation: "go_to_definition"` on a Go file and receive correct location data from gopls.
- Server starts lazily on first request.
- Server stops cleanly on agent shutdown.
- Concurrent tool calls to the same server don't race.
- Server crash is handled gracefully (logged, state set to error, restart on next request).

### Phase 2 — Diagnostics

**Goal:** LSP diagnostics (errors, warnings) appear automatically in the conversation context.

| # | Task | Dependencies |
|---|---|---|
| 2.1 | Implement `textDocument/publishDiagnostics` notification handler | 1.5 |
| 2.2 | Implement DiagnosticRegistry (dedup, volume limiting) | 2.1 |
| 2.3 | Emit diagnostics as daemon signals | 1.8, 2.2 |
| 2.4 | Drain diagnostics in agent loop (inject as system reminder) | 2.3 |
| 2.5 | Clear delivered diagnostics when file is edited | 2.2 |
| 2.6 | Write tests | All above |

**Verification criteria:**
- After agent reads a file, diagnostics for that file appear in the next context window.
- Same diagnostic is not repeated across turns.
- Per-file cap (10) and total cap (30) are enforced.
- Editing a file resets delivered diagnostics for that file.

### Phase 3 — Advanced Operations

**Goal:** Full feature parity with Claude Code's LSP tool.

| # | Task | Dependencies |
|---|---|---|
| 3.1 | Add `workspaceSymbol` operation | 1.9 |
| 3.2 | Add `goToImplementation` operation | 1.9 |
| 3.3 | Add `prepareCallHierarchy` + `incomingCalls` + `outgoingCalls` (two-step) | 1.9 |
| 3.4 | Update tool schema with new operations | 1.9 |
| 3.5 | Update formatters | 3.1–3.3 |
| 3.6 | Write tests | All above |

**Verification criteria:**
- All 9 operations work against gopls, typescript-language-server, and rust-analyzer (if available).

### Phase 4 — Server Discovery & UX

**Goal:** Zero-config LSP for common languages; polished error messages.

| # | Task | Dependencies |
|---|---|---|
| 4.1 | Auto-detect common LSP servers from PATH + project files (`go.mod`, `package.json`, `Cargo.toml`) | 1.7 |
| 4.2 | Graceful "server not found" messages with install instructions | 1.6 |
| 4.3 | LSP server startup status in UI (using existing daemon monitor strip) | 1.8 |
| 4.4 | Documentation + user guide | All above |

### Phase 1 Detailed Timeline

```
Week 1:
  Day 1–2: Protocol types (1.3) + JSON-RPC transport (1.4)
  Day 3–4: Server lifecycle (1.5) + config loader (1.7)
  Day 5: Manager (1.6)

Week 2:
  Day 1–2: lspDaemon (1.8) + lspTool (1.9) + formatters (1.10)
  Day 3: Registration (1.11, 1.12, 1.13) — wiring into agent
  Day 4–5: Tests (1.14) + integration (1.15)
```

---

## 7. Risk Analysis & Mitigation

### Risk 1: JSON-RPC framing edge cases

**Severity:** Medium  
**Likelihood:** Medium  

Some LSP servers send messages in chunks (the `Content-Length` header arrives, then the body arrives in a subsequent `Read()`). The reader goroutine must handle partial reads correctly.

**Mitigation:** Use `io.ReadFull` for the body after reading the header. Write a fuzz test that sends random chunk boundaries.

### Risk 2: UTF-16 position encoding

**Severity:** Medium  
**Likelihood:** High  

LSP uses UTF-16 code units for positions. Go source files are UTF-8. Characters outside the BMP (e.g., emoji in string literals) have different lengths in UTF-8 vs UTF-16.

**Mitigation:** Implement a UTF-8 → UTF-16 offset converter. Most files contain only BMP characters, so this is a correctness edge case rather than a daily problem. The converter can be ~30 lines.

### Risk 3: Server hangs during shutdown

**Severity:** Low  
**Likelihood:** Medium  

Some LSP servers don't respond to the `shutdown` request promptly.

**Mitigation:** Shutdown is time-boxed (5s default). If the server doesn't respond, the process is killed forcefully. This matches the reference implementation's behavior.

### Risk 4: Multiple concurrent requests to a single server

**Severity:** Low  
**Likelihood:** Low  

LSP servers can handle multiple concurrent requests (JSON-RPC supports it). The reference sends requests sequentially to avoid confusing the LLM. evva's tool dispatch is parallel by default.

**Mitigation:** The `Client.Request` method is naturally concurrent-safe (each request has a unique ID). No action needed — this is actually an advantage for speed.

### Risk 5: Go library dependencies

**Severity:** Low  
**Likelihood:** Low  

If we use `github.com/sourcegraph/jsonrpc2`, it adds an external dependency.

**Mitigation:** The dependency is mature (used by Sourcegraph in production). If it becomes a problem, replacing it with a ~200-line internal implementation is straightforward — the interface surface is small.

### Risk 6: LSP server binary not installed

**Severity:** Low  
**Likelihood:** High (for new users)

**Mitigation:** Clear error messages: `gopls not found in PATH. Install with: go install golang.org/x/tools/gopls@latest`. Phase 4 adds auto-detection to make this less likely.

---

## 8. Open Questions

1. **Should `lsp_request` be active or deferred?**  
   Recommendation: deferred. LSP is powerful but not needed every turn. Deferred loading saves LLM context. The tool can be brought active via `tool_search` when the agent needs it.

2. **One tool vs. multiple tools?**  
   The reference uses a single `LSP` tool with an `operation` discriminator. This is better than separate tools (`lsp_definition`, `lsp_references`, etc.) because it keeps the tool list manageable and groups related functionality.

3. **Should diagnostics go through the daemon system or a separate channel?**  
   Recommendation: daemon system. It already handles async subprocess output, lifecycle tracking, and UI updates. Diagnostics are a natural fit — they're "output" from a daemon.

4. **Project-level vs. user-level LSP config?**  
   Both. Project-level `.evva/lsp_servers.yml` takes precedence. User-level `~/.evva/lsp_servers.yml` is the fallback. This lets projects pin specific server versions while users set defaults.

5. **Should we support socket transport?**  
   Not in Phase 1. The reference only implements stdio, and all major LSP servers support stdio. Socket transport adds complexity without clear benefit for a CLI agent.

6. **Should `lsp_request` count toward the tool-use limit?**  
   Yes. It's a standard tool call. Unlike the reference (which has special `isLsp` handling), evva can treat it uniformly.

---

## 9. Appendix: Key Reference Files

For developers implementing this plan, these reference files are the most relevant:

| File | Lines | Relevance |
|---|---|---|
| `ref/src/services/lsp/LSPClient.ts` | 447 | JSON-RPC transport, spawn, shutdown |
| `ref/src/services/lsp/LSPServerInstance.ts` | 511 | State machine, crash recovery, retry |
| `ref/src/services/lsp/LSPServerManager.ts` | 420 | File-to-server routing, didOpen/Change/Close |
| `ref/src/services/lsp/manager.ts` | 289 | Global orchestrator singleton |
| `ref/src/services/lsp/LSPDiagnosticRegistry.ts` | 386 | Dedup, volume limiting, cross-turn tracking |
| `ref/src/services/lsp/passiveFeedback.ts` | 328 | publishDiagnostics → attachment conversion |
| `ref/src/tools/LSPTool/LSPTool.ts` | 860 | AI-facing tool, input validation, operation dispatch |
| `ref/src/tools/LSPTool/formatters.ts` | 592 | Human-readable output formatting |
| `ref/src/tools/LSPTool/schemas.ts` | 215 | Zod discriminated union schema |
| `ref/src/services/lsp/config.ts` | 79 | Config loading from plugins |
| `ref/src/utils/plugins/lspPluginIntegration.ts` | 387 | LSP server loading from plugin manifests |

Porting guidance: read these files from top to bottom in the order listed. The TypeScript patterns translate naturally to Go — classes become structs with methods, async/await becomes goroutines + channels, Zod schemas become `encoding/json` unmarshaling.

---

## Summary of Recommendations

1. **Start with Phase 1** — core LSP client with 4 operations. This is the minimum viable integration and proves the architecture.
2. **Use the daemon system** — don't build a separate lifecycle tracker. evva's daemon infrastructure is already designed for long-running subprocesses.
3. **Hand-write protocol types** — the subset we need (~20 structs) is manageable and avoids a heavy dependency.
4. **Deferred tool** — `lsp_request` should be deferred to save context window space.
5. **Lazy server start** — servers launch only when a file of a matching extension is queried.
6. **Project + user config** — YAML-based, with project-level override.
7. **Test against gopls first** — it's the most relevant for a Go codebase, and it's the easiest to install (`go install golang.org/x/tools/gopls@latest`).
