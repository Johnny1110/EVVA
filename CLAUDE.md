# evva — Project Vision and Roadmap

---

## Vision

`evva` is a ReAct coding agent for the terminal, written in Go. The architecture follows Claude Code in spirit but keeps the moving parts small on purpose: one narrow `llm.Client` interface bridging multiple providers (Anthropic, DeepSeek, OpenAI, Ollama), one `tools.Tool` interface, one observable store fanning state to any UI implementation, one agent loop.

The unifying idea is **one runtime, many personas, swappable UI**:

- A **persona** is a main-tier agent definition — its own tools, system prompt, model preference, and personality. `evva` (a professional software engineer) is one persona. `nono` (a financial manager), `noen` (a math teacher), and any others a user creates are siblings, not subclasses.
- The same runtime drives every persona. Switching personas is `/profile <name>`, not a new binary.
- A persona can spawn another persona as a subagent for cross-domain work — `evva` can delegate a costing question to `nono` without leaving the session.
- Adding a new LLM provider, tool family, persona, or UI implementation is a one-package change.

`evva` is **not** trying to be a drop-in Claude Code. It borrows the harness shape because that shape is what current frontier models behave best under, and it ports tool descriptions verbatim where reasonable so the model sees prompts close to what it was trained on. Where Go semantics, terminal constraints, or evva's narrower scope justify divergence, it diverges intentionally.

The reference TypeScript source lives at `evva/ref/src/`. Treat it as the source of truth for tool descriptions, harness structure, and agent definitions — port from it, don't reinvent.

---

## Important

`v1.0.0` is cut: the SDK v2 arc is complete and the Stable-tier surface
promise in `docs/sdk-stability.md` is in force — breaking changes to Stable
`pkg/*` packages now require a major bump. Experimental-tier packages
(`pkg/ui/bubbletea`, `pkg/tools/lsp`, `pkg/observable`, `pkg/tools/kits`)
may still change in minor versions.

---

## Agent definitions

All agents — main personas and subagent kinds alike — share one on-disk layout:

```
<EVVA_HOME>/agents/{name}/
├── system_prompt.md
├── tools.yml          # { active: [...], deferred: [...] }
└── meta.yml           # { as: [main, subagent], model: ..., when_to_use: ... }
```

Built-in agents (Main / Explore / Plan / GeneralPurpose) ship as Go-defined `AgentDefinition` structs. User-authored agents are loaded from disk at startup; the loader merges Go + disk into one registry. `agent_type` is a string, not a closed enum, so external projects can register their own personas (e.g. a future `nono` web service registers as a remote agent endpoint).

The `as:` field controls where an agent shows up:

| `as:` value | Visible as |
| --- | --- |
| `[main]` | `/profile` startup picker only |
| `[subagent]` | Agent tool's `subagent_type` list only |
| `[main, subagent]` | Both — used for personas that other personas can delegate to (the `evva → nono` pattern) |

One schema, one loader, two visibility surfaces. This is also the seam Phase 6 (profile switch) uses to enumerate personas.

---

## Roadmap (post-v1.0.0)

`v1.0.0` shipped a complete agent harness and a Stable SDK surface. The
post-v1 roadmap is ordered by one principle, not by dependency:
**finish before expand, integrity before power.** Earlier phases matter
more — a half-wired feature the system prompt *already advertises* is a
worse liability than any missing net-new capability, so finishing those
comes first. Every phase below is additive to the Stable surface, so each
lands as a **minor** release (`v1.1`, `v1.2`, …) under the semver promise
now in force.

### State of v1.0.0 — the evidence base for the order

**Solid / Stable** — agent loop + profiles + subagent spawn; `fs`,
`shell`, `web`, `notebook`, `util` tools; `todo`, `cron`, `daemon`
(background tasks) + `monitor`; plan mode + git `worktree`;
`ask_user_question`; memory (auto-load `EVVA.md` / `USER_PROFILE.md` +
`update_*` tools); pluggable `pkg/permission`; session store + snapshot +
`/compact` + `/resume`; the skill framework (`pkg/skill`); the full SDK v2
surface (one-call `agent.New`, separate-module host proof).

**Experimental** — `pkg/tools/lsp` (~9k LOC + 8 test files — the most
mature), `pkg/ui/bubbletea`, `pkg/observable`, `pkg/tools/kits`.

**Half-built / dangling — these set the priority order below:**

- **Hooks** (`internal/hooks`, ~1185 LOC, 9 files): a complete six-event
  lifecycle engine — SessionStart, UserPromptSubmit, PreToolUse,
  PostToolUse, Stop, Notification; shell + HTTP backends; designed to
  compose with permissions — that **nothing imports**, so it never fires.
  Yet `sysprompt/fragments.go` already tells the model hooks work. 0 tests,
  private. The worst kind of debt: an advertised promise the runtime
  silently breaks.
- **OpenAI provider**: `pkg/constant/llm.go` declares the `OPENAI`
  provider and a model, but there is **no `pkg/llm/openai`** and
  `pkg/llm/builtins` never registers it — selecting OpenAI fails at
  factory lookup. The Vision lists OpenAI as a first-class provider.
- **MCP**: absent entirely. The tool-search layer is already MCP-aware
  (`meta/fuzzy.go` + `toolsearch.go` parse `mcp__server__tool` names), but
  there is no client, config, discovery, or the four MCP tools.
- **Bundled skills**: only `/commit` ships; `/review`, `/security-review`,
  `/simplify` (named in the old Phase 3) do not — the framework is done,
  only the content is missing.

### v1.1 — Finish the hooks system  *(integrity: deliver an advertised feature)*

The system prompt promises hooks; the engine exists; the only thing
missing is the wiring. Highest priority because every session ships a
prompt that lies to the model today.

- Dispatch from the agent loop: **PreToolUse** *before* the permission
  gate (may return allow/deny/ask to override the gate, or `updatedInput`
  to mutate args first); **PostToolUse** after a tool result (append
  `additionalContext` for the next turn); **SessionStart**,
  **UserPromptSubmit**, **Stop**, **Notification** at their points.
- Load hook config from settings via `pkg/config` (the `hooks:` block:
  matcher → command/http entries).
- Compose with `pkg/permission` (PreToolUse decision precedes the gate).
- Promote `internal/hooks` → **`pkg/hooks`** — it composes with the now-
  public permission store, so downstream hosts need it public.
- Tests: the package has **0** today — add matcher / dispatcher-precedence
  / subprocess / http unit tests plus a loop integration test.

**Acceptance:** a configured PreToolUse `command` hook blocks a `bash`
call before the permission gate; a PostToolUse hook injects context the
model sees next turn; the prompt's hooks promise is finally true; tests green.

### v1.2 — OpenAI provider  *(integrity: complete the Vision's provider matrix)*

Small, cheap, and it removes a crash path. The constant already promises
OpenAI; this makes the promise real.

- New `pkg/llm/openai`: `ProviderName`, `Factory`, and a `Client`
  implementing all six `llm.Client` methods incl. `SupportsDeferLoading()`
  (OpenAI lacks Anthropic's `defer_loading` → return `false`, keeping the
  tools array stable for caching). `pkg/llm/deepseek` is the closest
  template (OpenAI-compatible chat/tools/streaming).
- Register in `pkg/llm/builtins`; reconcile the placeholder model ids in
  `pkg/constant/llm.go` with real ones.

**Acceptance:** `evva` runs a full ReAct turn against OpenAI; provider
parity tests pass; no constant promises an unimplemented provider.

### v1.3 — MCP client support  *(power: the headline net-new capability)*

The last major Claude Code parity gap and the biggest single lever on
"powerful." Framework only — bundled vendor servers stay out (see below).

- MCP server config in `pkg/config` (`mcpServers: {name: …}`), stdio +
  SSE/HTTP transports.
- A client that connects, runs `initialize`, and lists tools + resources.
- **Dynamic registration**: discovered tools register as **deferred**
  tools under the `mcp__server__tool` naming the search layer already
  scores, so `tool_search` surfaces them on demand and prompt caching is
  preserved.
- Port the four tools from `ref/src/tools/`: `MCPTool` (invoke),
  `McpAuthTool` (OAuth/token), `ListMcpResourcesTool`, `ReadMcpResourceTool`.

**Acceptance:** configure a real MCP server (e.g. a filesystem server);
its tools appear via `tool_search` and execute; list/read resources work.

### v1.4 — Bundled skills  *(cheap daily value; framework already exists)*

- Port `/review`, `/security-review`, `/simplify` (`/commit` already
  ships) as on-disk `SKILL.md` under the bundled-skills dir, drawing from
  `ref/src/skills/bundled/` and Claude Code's review skills. No framework
  changes — `pkg/skill` already loads them.

**Acceptance:** all four bundled skills are invocable in the TUI.

### v1.5 — Harden Experimental → Stable

- Per-package tier review in `docs/sdk-stability.md`: `pkg/tools/lsp`
  (most mature) is the prime Stable candidate; confirm or stabilize
  `pkg/observable` and `pkg/tools/kits`; `pkg/ui/bubbletea` likely stays
  Experimental (UI churn) but its contract gets documented.
- For each package promoted to Stable, add a separate-module compile guard
  (the `pkg/agent/downstream_test.go` pattern).

**Acceptance:** every Experimental package has an explicit documented
disposition; promoted ones gain the downstream compile guard.

---

## Out of scope (revisit after v1.x)

Listed so contributors don't propose them as phase additions.

- **Teams / SendMessage** — Claude Code's multi-agent runtime depends on a
  bridge layer (UDS sockets, remote control, JWT, cross-machine session
  forwarding). No second agent process exists to talk to yet.
- **Bundled vendor MCP integrations** (Atlassian, Figma, IDE diagnostics)
  — v1.3 ships the MCP *framework*; specific servers are user-configured,
  not bundled, until there's demand.
- **Cross-platform shell** (Windows PowerShell, `ref/src/tools/PowerShellTool`)
  — evva is bash-first; revisit if Windows demand appears.
- **Minor ref tools** — `ConfigTool`, `BriefTool`, `REPLTool`: no current
  demand; port individually if a use case shows up.

---

## Project conventions

- All source under `internal/` is private. Public extension points live in `pkg/`.
- One package per tool family (`fs`, `shell`, `meta`, etc.). A new tool either goes in an existing family or starts a new family package. Phase 13c moves the broadly-reusable families (`fs`, `shell`, `web`, `util`, `notebook`, `monitor`, `cron`, `todo`) under `pkg/tools/`; evva-runtime-specific families (`meta`, `mode`, `skill`, `ux`, `dev`) stay under `internal/tools/`.
- One package per LLM provider. After Phase 13b they live at `pkg/llm/{claude,deepseek,ollama}/` and register into `pkg/llm.DefaultRegistry()`. The `llm.Client` interface remains the only public seam.
- Tests live next to the code they cover (`*_test.go`). No parallel `tests/` tree.
- No comments that restate the code. Only comment WHY when the WHY is non-obvious.
- Port tool descriptions from `ref/src/tools/*/prompt.ts` verbatim when reasonable. Diverge only with a clear reason.

---

## Project structure

```
evva/
├── cmd/evva/                  # CLI entry point — wires agent + UI
├── configs/                   # config loading (.env + YAML)
├── docs/                      # design notes, tool docs, system prompts
├── internal/
│   ├── agent/                 # agent loop, profiles, spawn
│   │   ├── event/             # event types + sink contract
│   │   └── sysprompt/         # system prompt builder
│   ├── constant/              # provider / model / status enums
│   ├── llm/                   # llm.Client interface + shared params
│   │   ├── claude/  deepseek/  ollama/  ...
│   ├── llmfactory/            # provider factory keyed by constant
│   ├── logger/                # structured slog wrapper + pretty fmt
│   ├── observable/            # pub/sub framework for stores
│   ├── session/               # conversation history + cumulative usage
│   ├── tools/                 # tool interface (Name/Schema/Execute)
│   │   ├── cron/  dev/  fs/  meta/  mode/  monitor/  notebook/
│   │   ├── shell/  skill/  task/  util/  ux/  web/
│   ├── toolset/               # tool catalog + ToolState registry
│   └── ui/                    # UI plugin contract
│       ├── bubbletea/         # reference TUI implementation — prototype
│       ├── bubbletea_v2/      # reference TUI implementation v2 — refactor v1
│       └── ...                # downstream-customized layouts
├── ref/src/                   # Claude Code reference source (read-only)
├── log/                       # per-agent runtime logs (gitignored)
├── pkg/common/                # small shared utilities
└── scripts/                   # demo / dev scripts
```

Key boundaries:

- `agent` knows about `event.Sink`, never about a concrete UI.
- `tools/*` packages produce `tools.Result` (text + opaque `Metadata`); the UI type-asserts on `Metadata` to render structured payloads.
- `observable` has no dependencies on agent or UI.
- `ui` defines narrow interfaces; implementations live under it.

User-facing documentation (install, TUI keybindings, config file shape, log paths) lives in `README.md`. This file is for project vision and the development roadmap.