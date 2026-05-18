# EVVA ReAct coding agent

## What is EVVAgent?

`evva` runs a tool-using LLM agent in your terminal. It speaks Anthropic Claude, DeepSeek, OpenAI, and Ollama through one `llm.Client` interface; dispatches multiple tool calls per turn in parallel; tracks tasks and sub-agents through an observable store; and renders into a bubbletea TUI or a plain-text CLI sink.

The architecture is small on purpose — adding a new LLM provider, panel, or UI implementation is roughly one package each. evva is written in golang.

### EVVA_HOME Dir 

```
~/.evva/
├── config/
│   └── evva-config.yml      # user-tunable settings (auto-created with defaults)
└── skills/                  # optional skill scripts (your own)
```

`~/.evva/.env` is **optional**. If you want to override deployment knobs (`LOG_LEVEL`, `LOG_DIR`, `APP_ENV`, `LOG_FORMAT`, `SKILLS_DIR`, `USER_PROFILE`), create it; otherwise the built-in defaults apply.

<br>

## Using EVVAgent

### TUI at a glance

```
┌──────────────────────────────────────────────────────────────┐
│ banner box / transcript                                      │
│                                                              │
│  ▶ user prompt                                               │
│  assistant text…                                             │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│ ▰ TASKS         (only when non-empty)                        │
│   ▶ wire migration                                           │
├──────────────────────────────────────────────────────────────┤
│ ‹⠹ explorer› ‹▶ writer› ‹✔ reviewer›   ← active sub-agents   │
├──────────────────────────────────────────────────────────────┤
│ overlay panels: /config · /model · approval · suggestions    │
├──────────────────────────────────────────────────────────────┤
│ > input                                                      │
├──────────────────────────────────────────────────────────────┤
│ ‹⠋ RUN› ◆ evva ◆ ▸ model ◆ in N out M ◆ CTX ▰▰▱…▱ 12%        │
└──────────────────────────────────────────────────────────────┘
```

Panels collapse to zero height when empty. Status bar always at the bottom.

<br>


### Keybindings (main input)

| key | effect |
| --- | --- |
| `Enter` | submit |
| `Ctrl+J` / `Alt+Enter` | insert newline (multi-line composition) |
| `↑` / `↓` | walk prompt history (when input empty or already navigating) |
| `Esc` | cancel running task / dismiss panel |
| `Ctrl+C` | once: cancel running task · idle: quit |
| `Ctrl+D` | quit (when input is empty) |
| `Ctrl+O` | toggle expand-all tool results (fold/unfold long bash + read output) |
| `Ctrl+Y` | open **yank mode** — pick a block and copy its clean content (see below) |
| `Ctrl+F` | open **transcript search** — type a query, `Enter`/`n` cycles matches |
| `PgUp` / `PgDown` / `Home` / `End` | scroll transcript |
| mouse wheel | scroll transcript |

### Copying from the transcript — **yank mode**

The transcript renders each block with a left-edge timeline gutter (`│`, `├─`, etc.) so the conversation reads as a structured stream. The downside: a normal terminal drag-select copies whatever is visually on screen — gutter glyphs included. Pasting that into another window gives you something like:

## Configuration

### `~/.evva/config/evva-config.yml`

User-tunable settings. Created automatically on first launch. Edit live via `/config` in the TUI, or by hand:

```yaml
# Agent loop
max_iterations: 30
max_tokens: 4096
auto_compact_threshold: 0.8
display_thinking: true

# Default model used at startup (overwritten by /model swap)
default_provider: deepseek
default_model: deepseek-v4-pro

# Web tooling
fetch_max_bytes: 100000
tavily_api_key: ""

# Per-provider credentials. Empty api_url falls back to the constant's default.
providers:
  anthropic: { api_key: "", api_url: "" }
  deepseek:  { api_key: "", api_url: "" }
  openai:    { api_key: "", api_url: "" }
  ollama:    { api_url: "" }
```

### `.env` (optional)

Place in your working directory or at `~/.evva/.env`. Only used for deployment / logging knobs — never user preferences:

```bash
APP_ENV=dev            # dev | prod
LOG_LEVEL=info         # debug | info | warn | error
LOG_FORMAT=text        # text | json
LOG_DIR=               # empty → stdout; path → write log files there
SKILLS_DIR=skills      # subpath under ~/.evva/
USER_PROFILE=user_profile.md
```

## Logs

Per-agent JSON logs land under `log/<agent-id>/<agent-id>.log` by default. Set `LOG_DIR` in `.env` to redirect, or leave it unset to also stream to stdout. `LOG_LEVEL=debug` exposes every iteration's `turn.start` / `llm.call` / `tool.dispatch` / `tool.result` lines — handy when debugging an agent that's stuck or looping.

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
│   │   ├── claude/  deepseek/  ollama/ .../
│   ├── llmfactory/            # provider factory keyed by constant
│   ├── logger/                # structured slog wrapper + pretty fmt
│   ├── observable/            # pub/sub framework for stores
│   ├── session/               # conversation history + cumulative usage
│   ├── tools/                 # tool interface (Name/Schema/Execute)
│   │   ├── cron/  dev/  fs/  meta/  mode/  monitor/  notebook/
│   │   ├── shell/ task/ ux/   web/
│   ├── toolset/               # tool catalog + ToolState registry
│   └── ui/                    # UI plugin contract
│       ├── bubbletea/         # reference TUI implementation - prototype
│       ├── bubbletea_v2/      # reference TUI implementation v2 - refactor v1
│       └── ...                # user customized TUI implementation (customized layout style)
├── log/                       # per-agent runtime logs (gitignored)
├── pkg/common/                # small shared utilities (UUID, ...)
└── scripts/                   # demo / dev scripts
```

Key boundaries:
- `agent` knows about `event.Sink`, never about a concrete UI.
- `tools/*` packages produce `tools.Result` (text + opaque `Metadata`); the UI type-asserts on `Metadata` to render structured payloads.
- `observable` has no dependencies on agent or UI.
- `ui` defines two narrow interfaces; implementations live under it.

<br>

---

<br>

## Dev Planing

There are full claude-source code put in `evva/ref/src`, refer to claude-code source code and implement evva new feature. also we need using tag to improve the system prompt and workflow prompt.

* [ ] Phase-1: Basic fs tools + GlobTool + GrepTool copy
  * Based on claude-code FileReadTool, FileWriteTool, FileEditTool, revamp evva read, write, edit tool. copy to golang including same tool desc and schema.
  * Tui render of write, edit using the same layout as claude-code

* [ ] Phase-2: ToolSearchTool copy
  * Based on claude-code ToolSearchTool, revamp evva ToolSearch, evva are not complete right now, based on claude-code, put same tools we have into deferred.

* [ ] Phase-3: AgentTool copy
  * Based on claude-code AgentTool, revamp evva AgentTool
  * Redesign Agent: in the future, the evva agent should be able support extend by other project, 
  * Any project extend agent can share Evva toolPackage + customized toolPackage

* [ ] Phase-4: System/Tool Prompt copy
  * Learn from Claude Code Prompt system design, copy the claude code design to evva.
  * update evva system design following claude code. 

* [ ] Phase-5: TodoWrite tool copy
  * Based on claude-code TodoWriteTool, create same tool for evva. and integrate into evva.
  * Tui render should discuss with developers.

* [ ] Phase-6: Task Tools copy
  * Based on claude-code TaskTools(Create, Get, List, Output, Stop, Update), revamp evva task tool. copy to golang including same tool desc and schema.
  * Revamp TaskGroup to fit new TaskTools 

* [ ] Phase-7: Tool use Permission mechanism copy
  * Based on claude-code Tool use Permission mechanism design it for evva, and create a mode swith, safe mode (enable tool use permission check), auto mode (disable tool use permission check)

* [ ] Phase-8: Agent Profile switch
  * Design an agent profile switch feature, different tool list, different system prompt, different harness. user can switch profile before session start.
  * system level agent profile (Explore, General) are now allow to switch by user
  * need a ProfileManager to manage all profile.

* [ ] Phase-9: TeamTool SendMessageTool copy

* [ ] Phase-10: EnterWorktreeTool +ExitWorktreeTool

* [ ] Phase-10: AskUserQuestionTool copy

* [ ] Phase-11: EnterPlanModeTool + ExitPlanMode copy

... in future

## Claude Dev Log