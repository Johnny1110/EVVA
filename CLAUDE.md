# evva — Project Vision and Roadmap

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

## Memory model

Two files, two scopes:

- `<workdir>/EVVA.md` — **project memory**. User-authored conventions, repo-specific rules, hot facts about the codebase. Injected into the system prompt at session start. Same role as Claude Code's `CLAUDE.md`.
- `<EVVA_HOME>/USER_PROFILE.md` — **user memory**. Long-running notes about the user: preferences, working style, recurring topics, projects they care about. Curated by a dedicated background agent (Phase 9) that reviews the session transcript at session end and merges new observations into the file under a fixed shape (`## Preferences`, `## Working style`, `## Recurring topics`).

Both files are read on every session start. Either can be missing — the prompt builder skips empty sections cleanly.

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

## Roadmap

Phases are ordered by dependency — earlier phases unblock later ones. Each phase is one focused chunk of work: Go ports of the reference TypeScript, plus the connective tissue (memory, permissions, hooks) that ties the harness together.

### Phase 0 — Sysprompt rework + EVVA.md + USER_PROFILE.md ✅️

Foundation. Every later phase ships prompt strings, so the prompt scaffold needs to be stable first.

- Refactor `internal/agent/sysprompt/` from section toggles to **per-agent prompt builders**. Each agent owns its full harness, mirroring `ref/src/tools/AgentTool/built-in/*Agent.ts`.
- New `internal/memdir/` package. Loads `<workdir>/EVVA.md` and `<EVVA_HOME>/USER_PROFILE.md` and injects them into the sysprompt at session start.
- Rewrite the harness / tool-guide sections against `ref/src/constants/prompts.ts` and the per-tool prompt files.
- Wire cross-references (Read ↔ Edit, Agent ↔ subagent_type list, plan-mode ↔ AskUserQuestion) through string constants so descriptions stay consistent as tools evolve.

### Phase 1 — Filesystem parity (Read / Write / Edit / Glob)

Port `ref/src/tools/FileReadTool / FileEditTool / FileWriteTool / GlobTool` descriptions verbatim; drop evva current Write/Edit/Read tools (many bug in current evva fs tools), can copy claude code design.

- Port descriptions + parameter schemas + implement from `ref/src/tools/Read/Edit/Write/`.
- New `internal/tools/fs/glob.go` — mtime-sorted file matching. Today evva has `shell.Grep` + `shell.Tree` but no dedicated Glob.
- TUI diff render parity for `Edit` and `Write` — match Claude Code's hunk layout.
- Tighten `ReadTracker` semantics to match Claude Code's "must Read before Edit / overwrite-Write."

**Phase 0 dev log — what Phase 1 must keep in sync:**

- Tool names interpolated into agent prompts live in `internal/agent/sysprompt/toolnames.go` (prompt-side constants like `nameRead`, `nameEdit`, `nameWrite`, `nameGrep`, `nameTree`). When Phase 1 changes a wire value in `internal/tools/name.go` or adds a new fs tool, mirror it in `toolnames.go`. Drift is caught by `internal/agent/sysprompt/toolnames_link_test.go` at CI — that test interpolates each canonical `tools.ToolName` into the rendered main prompt and fails if the wire string is absent.
- Add `nameGlob = "glob"` to `toolnames.go` when introducing the Glob tool. Reference it from `main_agent.go:mainToolsGuideSection()` next to `nameTree` / `nameGrep`, and from `explore_agent.go:buildExplorePrompt` (the Explore subagent should prefer Glob over `tree` for broad pattern matching once it lands). Append `tools.GLOB` to the required-names list in `toolnames_link_test.go:TestToolNamesAppearInMainPrompt`.
- The Main agent's tools-guide section in `internal/agent/sysprompt/main_agent.go:mainToolsGuideSection()` describes Read/Write/Edit/Bash usage. After porting the ref TS descriptions, rewrite this section against the new tool guidance so the main agent advertises the new behavior — keep the hardcoded examples (`{"query": "select:task_create,..."}`) in sync with whatever Phase 1's tool descriptions reference.
- `internal/agent/profiles.go:Explore()` lists the active tools for the Explore subagent: currently `READ_FILE, WEB_SEARCH, TREE, GREP, JSON_QUERY`. When Glob lands, swap (or augment) TREE → GLOB. The Explore subagent prompt at `explore_agent.go` also mentions `tree` in its guidelines — update both.
- The new fs tool descriptions should be ported from `ref/src/tools/FileReadTool/prompt.ts`, `FileEditTool/constants.ts`, `FileWriteTool/prompt.ts`, `GlobTool/prompt.ts`. Each ref TS file exports a `*_TOOL_NAME` constant; the prompt-side mirror in `toolnames.go` is evva's equivalent of that pattern (Go can't do the prompt↔tool round-trip without creating an import cycle, which is why the link test exists).

### Phase 1b — Image returns via multimodal `tool_result` blocks

Phase 1 ships text-only reads (UTF-8 text, PDF page extraction, Jupyter cell rendering) because returning **image bytes** to the LLM requires a cross-cutting refactor that goes beyond `internal/tools/fs/`. This phase delivers that refactor.

Today `tools.Result.Content` is a plain `string`, and every provider serializes `tool_result` blocks as text-only (see `internal/llm/claude/client.go` — `Content: tr.Content` is a string field, no `[{type:"image", source:{...}}]` support). Until that changes, `read` of a `.png`/`.jpg` will return an "image reads not yet supported" error pointing at this phase.

Work:

- Widen `tools.Result` (and the wire-shape `llm.ToolResult`) to carry a content **list** of typed blocks — text and image at minimum — alongside the existing `IsError` + `Metadata` fields.
- Update each provider's tool-result serialization:
  - **Anthropic** (`internal/llm/claude/client.go`): emit `content: [{type:"text",...},{type:"image",source:{type:"base64",media_type,data}}]`.
  - **DeepSeek / OpenAI / Ollama**: providers that don't natively accept multimodal tool results need a documented fallback (text caption + base-64-as-text, or refusal). Decide per-provider.
- Extend `internal/tools/fs/read.go` to dispatch on image MIME (`.png`/`.jpg`/`.jpeg`/`.gif`/`.webp`) and emit a base-64-encoded image block. Resize/downsample to a token budget if needed (mirror `ref/src/tools/FileReadTool/imageUtils.ts`).
- Round-trip multimodal tool results through `internal/session/` so transcripts persist correctly.
- TUI: render the inline image block in the transcript (terminal protocol support is best-effort — Kitty / iTerm2 / Sixel where available, fallback to "[image: <path>, <bytes>B]").

Prerequisite for any "look at this screenshot" workflow. Out of Phase 1 because it touches four LLM clients and the session store, not just `fs/`.

### Phase 2 — ToolSearch + AgentTool polish + agent loader

Both tools already exist in evva (`internal/tools/meta/`) and roughly match Claude Code's behavior. This phase finishes parity and lays the **extensibility seam** Phase 6 and external projects depend on.

- Port 1:1 the latest ToolSearch (`ref/src/tools/ToolSearchTool`).
- Port 1:1 the AgentTool (`ref/src/tools/AgentTool`), including the "writing the prompt" / "never delegate understanding" guidance.
- New `internal/agent/loader/` — reads `<EVVA_HOME>/agents/{name}/` definitions and registers them. Built-ins stay as Go-defined structs; the loader merges Go + disk into one `AgentRegistry`.
- Replace `toolset.buildOne`'s hard-coded switch (currently ~370 LOC closed enum) with a `Registry.Register(name, factory)` API so external projects can register their own tools at startup.

### Phase 3 — Permission system + Bash classifier + safe/auto modes

Unblocks plan mode (Phase 7) and worktree (Phase 10). Plan mode is a permission mode, not a standalone tool pair.

Design questions resolved at the start of this phase:

- Rule grammar — glob? regex? per-tool? Reference: `ref/src/utils/permissions/permissionRuleParser.ts`.
- Storage scope — per-project (`.evva/permissions.json`) vs per-user vs per-session.
- Lifecycle — ask-once vs always-allow vs deny; mode transitions (`default: accept_edits` → `plan` → `bypass`).
- Override flow — equivalent of `--dangerously-skip-permissions`, sandbox flag, etc.
- Subagent inheritance — follow the ref source code design.

Work:

- New `internal/permission/` — rule grammar, mode state machine, pre-tool-use hook in the agent loop.
- Port `ref/src/tools/BashTool/bashClassifier.ts` + `dangerousPatterns.ts` into `internal/tools/shell/classifier.go`.
- TUI: approval prompt component under `components/approval/`, mode indicator in the status bar.
- Modes: `default = accept_edits | plan | bypass | auto`.

### Phase 4 — Hooks system

Compositional with permissions. Lets users wire validation, auto-format, custom logging, or block known-bad commands without touching evva's source.

- New `internal/hooks/` — event types (`SessionStart`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `Notification`), dispatcher, settings-file bindings.
- Wire hook invocations into `internal/agent/loop.go` between iterations and around tool dispatch.

### Phase 5 — TodoWrite (replaces current task_* tools)

evva's current `internal/tools/task/` is **conceptually TodoWrite** — in-session ephemeral planning. The six-tool layout (`task_create`, `task_get`, `task_list`, `task_update`, `task_output`, `task_stop`) doesn't match Claude Code's design and conflates planning with background-process management. Rebuild it.

- Delete `internal/tools/task/` (six tools).
- Delete the `mainTaskPlanningSection()` function from `internal/agent/sysprompt/main_agent.go` and drop `nameTaskCreate` / `nameTaskUpdate` / `nameTaskList` from `internal/agent/sysprompt/toolnames.go`. (Phase 0 moved the task-planning copy out of `sections.go` and into the main-agent builder; the old `sections.go` no longer exists.)
- New `internal/tools/todo/` — single `todo_write` tool: `todos: [{content, activeForm, status}]`, full-list-replacement semantics. Port description from `ref/src/tools/TodoWriteTool/prompt.ts`. Add `nameTodoWrite` to `toolnames.go` and a new `mainTodoSection()` fragment in `main_agent.go`.
- Rename `internal/ui/bubbletea_v2/components/tasks/` → `components/todos/`. Reuse the existing observable store wiring (just rename `TaskGroup` → `TodoStore`).
- The "real" process tools (`Monitor`, `task_output`, `task_stop`) come back in a future phase tied to `Bash run_in_background`.

### Phase 6 — Profile manager + `/profile` switch + cross-persona delegation

This is the **payoff phase** for everything in Phases 0–2: evva, nono, noen become first-class swappable personas, and `evva → nono` delegation works.

- `/profile` slash command + TUI picker (lists every agent in the registry with `as: [evva, nono, ...]`) also rename Main profile to Evva profile, make a default profile into evva-config.yml.
- Profile switch resets the session — provider-locked state (Anthropic `ThinkingSignature`, DeepSeek `reasoning_content`) can't carry across personas, and the system prompt is fully different anyway.
- The Agent tool's `subagent_type` enum becomes the union of every agent with `as: [subagent, ...]` — including personas marked `as: [main, subagent]`. That union is how `evva` ends up able to spawn `nono` as a subagent.
- The "subagents cannot spawn subagents" invariant stays in place.
- TUI refine, add main agent profile name to the status bar (replace curren hardcode evva).

### Phase 7 — Plan mode (EnterPlanMode / ExitPlanMode)

Bundled with Phase 3. Plan mode is `permission_mode: plan` plus a `plan_file` workflow, not a freestanding feature.

- Port `ref/src/tools/EnterPlanModeTool/prompt.ts` + `ExitPlanModeTool/prompt.ts`.
- Implement the Plan agent profile — read-only tools only, plan-file output. The skeleton already exists at `internal/agent/profiles.go`.
- Wire `ExitPlanMode` to restore the previous permission mode (`default` or whatever was active before).

### Phase 8 — AskUserQuestion

UI-heavy port. The tool surface is small; the TUI does most of the work.

- Port `ref/src/tools/AskUserQuestionTool/prompt.ts`.
- TUI: question/answer overlay with single-select, multi-select, and side-by-side preview support (mockups, code snippets, diagrams).
- Wire the answers + annotations back into the tool result envelope.

### Phase 9 — User-profile background agent

The agent that maintains `<EVVA_HOME>/USER_PROFILE.md`.

Design points:

- **Trigger** — `/profile-update` slash command for manual refresh.
- **Tools** — `update_user_profile` (writes to `USER_PROFILE.md`).
- **Output shape** — fixed sections (`## Preferences`, `## Working style`, `## Recurring topics`) so updates merge cleanly. Free-form rewrites drift and become useless within a few sessions.
- **Opt-out** — enabled by default; one-line notice on first session; `/config` toggles it off.

### Phase 10 — Worktree tools (EnterWorktree / ExitWorktree)

Niche. Ship after the higher-leverage phases.

- Port `ref/src/tools/EnterWorktreeTool/prompt.ts` + `ExitWorktreeTool/prompt.ts`.
- Implement `git worktree add / remove` plumbing.
- Wire AgentTool's `isolation: "worktree"` parameter to the same code path.

### Phase  11 - Refine the Agent System Prompt

- port ref system prompt to evva.

### Phase 12 — MCP support + bundled skills (v2 tier)

Closes the gap with Claude Code's plugin/skill ecosystem.

- MCP server config + discovery; dynamic tool registration as deferred tools (so `ToolSearch` picks them up).
- Port `ListMcpResources` / `ReadMcpResource`.
- Bundle a small set of skills inspired by `ref/src/skills/bundled/`: `/commit`, `/review`, `/security-review`, `/simplify`.

---

## Out of scope (v3+)

These deliberately don't appear in the 0–11 roadmap. Listed so contributors don't propose them as Phase additions.

- **Teams / SendMessage** — Claude Code's multi-agent runtime depends on a bridge layer (UDS sockets, remote control, JWT, cross-machine session forwarding). Premature for evva v1; revisit when there's an actual second agent process to talk to.
- **Process tools (`Monitor`, `task_output`, `task_stop`)** — return as a dedicated phase tied to `Bash run_in_background`. Today no one is asking for it.
- **MCP integrations** (Atlassian, Figma, IDE diagnostics) — out of v1 entirely. The MCP framework support (Phase 11) is enough to unblock community plugins; bundled vendor integrations follow once there's demand.

---

## Project conventions

- All source under `internal/` is private. Public extension points live in `pkg/`.
- One package per tool family (`fs`, `shell`, `meta`, etc.). A new tool either goes in an existing family or starts a new family package.
- One package per LLM provider in `internal/llm/`. The `llm.Client` interface is the only public seam.
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