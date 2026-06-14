# How to integrate EVVA agent in your Go project?

> Languages: **English** ｜ [正體中文](../zh-tw/integration.md)

<br>

---

<br>

evva's agent runtime is embeddable. A Go program outside this repo can import `pkg/agent` and assemble its own ReAct agent — with a custom LLM provider, custom tools, a custom event sink, and a non-default home directory — without forking evva and without touching the agent loop.

Everything below uses **only** `pkg/*` imports. Go's `internal/` rule enforces this at compile time: a downstream module that accidentally reaches into `evva/internal/...` won't build.

### 1. Add evva to your module

```bash
go get github.com/johnny1110/evva
```

### 2. Pick your providers

Blank-import `pkg/llm/builtins` to register Anthropic / DeepSeek / Ollama on the default registry; or register a custom provider yourself.

```go
import (
    _ "github.com/johnny1110/evva/pkg/llm/builtins" // anthropic/deepseek/ollama
    "github.com/johnny1110/evva/pkg/llm"
)

// Optional: register your own LLM client.
func registerGemini() {
    llm.DefaultRegistry().MustRegister("gemini",
        func(cfg llm.APIConfig, model string, opts ...llm.Option) (llm.Client, error) {
        return newGeminiClient(cfg, model, opts...), nil
    })
}
```

Your `llm.Client` implementation satisfies six methods: `Name()`, `Model()`, `SupportsDeferLoading()`, `Complete(ctx, msgs, tools)`, `Stream(ctx, msgs, tools, sink)`, and `Apply(opts...)`. `SupportsDeferLoading()` reports whether the provider natively supports `defer_loading` (return `false` if unsure — the agent then keeps the tools array stable to preserve prompt caching). See `pkg/llm/client.go` for the contract.

### 3. Load a Config with your own AppHome

`config.LoadDefault()` boots against `~/.evva/` for compatibility with the bundled CLI. Downstream apps build their own:

```go
import "github.com/johnny1110/evva/pkg/config"

cfg, err := config.Load(config.LoadOptions{
    AppName: "myapp",
    AppHome: filepath.Join(home, ".myapp"),
})

// Install the API key for whichever provider you picked.
cfg.LLMProviderConfig["anthropic"] = config.APIConfig{
    ApiURL:    "https://api.anthropic.com",
    ApiSecret: os.Getenv("ANTHROPIC_API_KEY"),
}
```

Two agents with different `*config.Config` pointers coexist in one process — there is no global singleton inside the agent loop.

### 4. Author a custom tool (optional)

A tool satisfies `pkg/tools.Tool`: `Name()`, `Description()`, `Schema()`, and `Execute(ctx, logger, input)`. The factory receives a `pkg/tools.State` so the tool can read the active `*config.Config` and the agent's workdir.

```go
import (
    "github.com/johnny1110/evva/pkg/tools"
)

type pingTool struct{}

func (pingTool) Name() string            { return "ping" }
func (pingTool) Description() string     { return "respond with pong" }
func (pingTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (pingTool) Execute(_ context.Context, _ *slog.Logger, _ json.RawMessage) (tools.Result, error) {
    return tools.Result{Content: "pong"}, nil
}
```

### 5. Consume events with your own sink

A `pkg/event.Sink` is one method: `Emit(event.Event)`. Fan out to multiple sinks with `event.Multi{Sinks: [...]}` or wrap a parent's sink with `event.BubbleUp` to rewrite `ParentID`.

```go
import "github.com/johnny1110/evva/pkg/event"

type stdoutSink struct{}

func (stdoutSink) Emit(e event.Event) {
    switch e.Kind {
    case event.KindText:
        if e.Text != nil {
            fmt.Println(e.Text.Text)
        }
    case event.KindToolUseStart:
        if e.ToolUseStart != nil {
            fmt.Printf("→ %s\n", e.ToolUseStart.Name)
        }
    }
}
```

### 6. Build the agent

There are two constructors. Pick by how much you want wired for you.

**`agent.New(Config, ...Option)` — the one-call path.** A declarative
`Config` resolves a persona (falling back to `evva`), auto-loads
`EVVA.md` / `USER_PROFILE.md` memory and the skill catalog, loads the
permission store, and installs the approval/question brokers. Best when
you want the batteries-included experience (and the bundled TUI):

```go
import "github.com/johnny1110/evva/pkg/agent"

ag, _ := agent.New(agent.Config{
    AppConfig:      cfg,
    PermissionMode: "bypass", // "" → YAML → "default"
}, agent.WithSink(stdoutSink{}), agent.WithMaxIterations(20))

resp, _ := ag.Run(context.Background(), "list files under /tmp")
```

For an interactive terminal UI, construct the bundled `pkg/ui/bubbletea`
UI, pass it as the sink, and hand `ag.Controller()` to `tui.Attach` — see
the full-host example below. Register your own personas with
`agent.BuildAgentRegistry` + `reg.Register(agent.AgentDefinition{...})` and
pass `Config.Personas` + `Config.Persona`.

**`agent.NewWithProfile(profile, ...Option)` — the à-la-carte path.** Wires
only what you pass. Build a profile with `NewProfile`, then add config,
sink, custom tools, and permission stance by hand:

```go
prof, _ := agent.NewProfile(
    "myapp",
    "you are a concise assistant",
    []tools.ToolName{tools.READ_FILE, tools.BASH},
    "anthropic", "claude-sonnet-4-6",
    agent.ProfileOptions{},
)

ag, _ := agent.NewWithProfile(prof,
    agent.WithConfig(cfg),
    agent.WithSink(stdoutSink{}),
    agent.WithMaxIterations(20),
    agent.WithHeadlessBypass(), // or WithPermissionMode(agent.PermissionBypass) — typed constants
    agent.WithCustomTool("ping", func(tools.State) (tools.Tool, error) {
        return pingTool{}, nil
    }),
)

resp, err := ag.Run(context.Background(), "list files under /tmp")
```

### Full working examples

Two runnable downstream consumers, both with **zero `internal/*` imports**:

- [`examples/full-host/`](../../../examples/full-host/main.go) — the canonical full host: the bundled TUI + personas + permissions via the one-call constructor, in a **separate Go module** (so Go's `internal/` rule compiler-enforces the pkg-only boundary).
- [`examples/minimal-host/`](../../../examples/minimal-host/main.go) — a tiny host: a custom LLM provider, custom tool, custom sink, and a programmatic skill, run for one turn via `NewWithProfile`.

```bash
go run ./examples/full-host     # or ./examples/minimal-host
```

### What you can't change

These are deliberately not part of the public surface:

- **Event kinds.** `pkg/event.Kind` constants are fixed at the evva version you import. Adding a new kind requires a code change in evva.
- **Agent loop logic.** The `LLM call → dispatch tools → fold results → repeat` shape lives in `internal/agent/loop.go` and is not configurable.
- **Sysprompt internals.** Inject your own full system prompt via `NewProfile(..., systemPrompt, ...)`; evva's bundled prompt builders are not exported.

If one of these is blocking your use case, fork or file an issue.

### See also

- [`docs/contributing/extending.md`](../../../docs/contributing/extending.md) — full reference covering every public package, every extension point, and the things you can't override.
- [`docs/contributing/sdk-stability.md`](../../../docs/contributing/sdk-stability.md) — per-package stability tiers and how to pin evva in `go.mod`.
- [`examples/full-host/main.go`](../../../examples/full-host/main.go) — full host (TUI + personas + permissions), separate module.
- [`examples/minimal-host/main.go`](../../../examples/minimal-host/main.go) — tiny host via `NewWithProfile`.
- [`pkg/agent/downstream_test.go`](../../../pkg/agent/downstream_test.go) + [`converged_downstream_test.go`](../../../pkg/agent/converged_downstream_test.go) — copy-paste test templates for both constructors.
