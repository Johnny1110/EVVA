# EVVAgent (evva)

A ReAct coding agent in your terminal. Multi-provider LLM, parallel tool dispatch, async sub-agents, swappable UI.

---

## What is EVVAgent?

`evva` runs a tool-using LLM agent in your terminal. It speaks Anthropic Claude, DeepSeek, OpenAI, and Ollama through one `llm.Client` interface; dispatches multiple tool calls per turn in parallel; tracks tasks and sub-agents through an observable store; and renders into a bubbletea TUI or a plain-text CLI sink.

The architecture is small on purpose — adding a new LLM provider, panel, or UI implementation is roughly one package each.

---

## Install

```bash
git clone https://github.com/johnny1110/evva
cd evva
make install
```

Default install target is `$GOBIN` (or `$GOPATH/bin` when `GOBIN` is unset) — usually already on a Go developer's `PATH`. The `make install` output tells you whether to add it.

Override the location if you want it elsewhere:

```bash
sudo make install PREFIX=/usr/local/bin     # system-wide
make install PREFIX=$HOME/.local/bin        # user-local
```

Verify:

```bash
which evva
evva --help-ish    # any flag triggers the usage line
```

Uninstall removes only the binary; your `~/.evva/` config is preserved:

```bash
make uninstall
```

---

## First run

Just type `evva` from any directory. On the first launch evva auto-creates:

```
~/.evva/
├── config/
│   └── evva-config.yml      # user-tunable settings (auto-created with defaults)
└── skills/                  # optional skill scripts (your own)
```

A one-line stderr notice fires the first time only:

```
evva: wrote new config to ~/.evva/config/evva-config.yml — fill in your API keys to use cloud providers.
```

`~/.evva/.env` is **optional**. If you want to override deployment knobs (`LOG_LEVEL`, `LOG_DIR`, `APP_ENV`, `LOG_FORMAT`, `SKILLS_DIR`, `USER_PROFILE`), create it; otherwise the built-in defaults apply.

### Adding an API key

Two ways:

1. **From inside the TUI:** type `/config`, navigate to `<provider>.api_key`, press Enter, paste your key, press Enter again. Saved immediately.
2. **By hand:** open `~/.evva/config/evva-config.yml` and fill in `providers.<provider>.api_key`.

Cloud providers (Anthropic, DeepSeek, OpenAI) need a key; Ollama is local and key-less.

---

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
│ ‹⠋ RUN› ◆ evva ◆ ▸ model ◆ in N out M ◆ CTX ▰▰▱…▱ 12%       │
└──────────────────────────────────────────────────────────────┘
```

Panels collapse to zero height when empty. Status bar always at the bottom.

### Slash commands

Type `/` at the start of the input and a suggestion panel appears. As you type more characters, the list filters by case-insensitive prefix match. When the typed input is an **exact match** for a command, that row turns green with a `✓` — pressing Enter executes it.

| key | effect |
| --- | --- |
| `Tab` | autocomplete to the highlighted suggestion |
| `↑` / `↓` | move the highlighted suggestion |
| `Enter` | submit the current input (executes if it's a valid command) |
| `Esc` | dismiss the suggestion panel for this typing session |

Available commands:

| command | what it does |
| --- | --- |
| `/config` | open the settings form (see below) |
| `/model` | switch LLM provider / model — **clears conversation history** |
| `/clear` | clear the transcript (keeps the banner) |
| `/exit`, `/quit` | quit |

### `/config` — runtime settings

Opens a bordered form listing every editable setting:

```
┌─ /CONFIG ────────────────────────────────────────┐
│ ▶ max_iterations           30                    │
│   max_tokens               4096                  │
│   auto_compact_threshold   0.8                   │
│   display_thinking         true                  │
│   fetch_max_bytes          100000                │
│   tavily_api_key           ****wxyz              │
│   anthropic.api_key        (empty)               │
│   …                                              │
│ [↑↓] navigate · [Enter] edit/toggle · [Esc] close│
└──────────────────────────────────────────────────┘
```

| key | effect |
| --- | --- |
| `↑` / `↓` | move the cursor |
| `Enter` | edit the focused field (booleans toggle in-place, no editor needed) |
| `Enter` (in editor) | apply and save |
| `Esc` | cancel the edit (or close the panel from list mode) |

API key fields open a password-masked editor; pasting works (display stays masked).

**Live-applied** (takes effect immediately):
- `max_iterations` — the loop's safety cap; mutates the running agent
- `display_thinking` — toggles thinking blocks in the transcript
- `auto_compact_threshold` — when context compaction kicks in

**Persisted but next-launch only** (would require rebuilding `llm.Client` / web tools):
- `max_tokens`, `fetch_max_bytes`, `tavily_api_key`, all `<provider>.api_key`, all `<provider>.api_url`

Every edit writes immediately to `~/.evva/config/evva-config.yml`. Closing the panel is a no-op — there's nothing left to commit.

### `/model` — switch provider/model

Opens a flat list of every `(provider, model)` pair the binary knows about, cursor pre-positioned on the active one. Up/Down to navigate, Enter to switch, Esc to cancel.

```
┌─ /MODEL ─────────────────────────────────────────────────────┐
│ Swapping clears the conversation — provider-specific state   │
│ (thinking signatures) can't carry across providers.          │
│                                                              │
│   ollama / qwen3.6                                           │
│   anthropic / claude-sonnet-4-6                              │
│   anthropic / claude-opus-4-7                                │
│ ▶ deepseek / deepseek-v4-pro  (current)                      │
│   deepseek / deepseek-v4-flash                               │
│   openai / gpt-5.5                                           │
│                                                              │
│ [↑↓] navigate · [Enter] switch · [Esc] cancel                │
└──────────────────────────────────────────────────────────────┘
```

**Important:** switching always clears the session. Anthropic's `ThinkingSignature` is provider-locked — carrying old history across a swap would 400 on the next request. The new choice is also persisted as `default_provider` + `default_model` so your next launch starts there.

Switching is refused if a run is in flight; press Esc first to cancel, then `/model` again.

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
| `PgUp` / `PgDown` / `Home` / `End` | scroll transcript |
| mouse wheel | scroll transcript |

### Approval prompts (filesystem writes)

When the agent attempts to `write_file` or `edit_file`, an overlay shows the proposed diff and pauses for confirmation:

```
┌─ APPROVE WRITE /path/to/file ───────────────────┐
│ <unified diff of the proposed change>           │
│                                                 │
│ ▶ Yes, apply this change                        │
│   Yes, and approve all remaining changes this session
│   No — let me tell the agent what to do instead │
└─────────────────────────────────────────────────┘
```

| key | effect |
| --- | --- |
| `↑` / `↓` | move the cursor between options |
| `Enter` | dispatch the selected option |
| `Esc` | shortcut for "tell the agent what to do instead" — switches to a textinput for your redirection |
| `Ctrl+C` | decline + quit |

The "approve all" option is sticky for the remainder of the session. Multiple parallel writes (the agent emitting several `write_file` calls in one turn) queue automatically — you see one prompt at a time, paste works in the feedback field.

### Sub-agents

The root agent can spawn sub-agents (`explore` for read-only inspection, `general-purpose` for write-capable). Active sub-agents appear as chips in a horizontal strip above the input. Async sub-agents finish in the background — their summaries land as a synthetic user message at the top of the next iteration, so the conversation picks them up automatically.

You don't drive sub-agents yourself; the model decides when to spawn one. Two-layer hierarchy by design (sub-agents can't spawn sub-agents).

---

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

### CLI flags

```bash
evva                                # interactive TUI (when stdout is a TTY)
evva -temp 0.7                      # sampling temperature (default unset)
evva -max-tokens 2048               # per-completion output cap (overrides YAML)
evva -max-iters 40                  # loop iteration cap (overrides YAML)
evva -no-tui "explain loop.go"      # one-shot plain-text mode
echo "list files in /tmp" | evva -no-tui   # piped prompt
```

---

## Modes

**Interactive TUI** (default when stdout is a TTY). Transcript, panels, status bar, the works.

**Plain CLI** (`-no-tui`, or when stdout is piped). One-shot flow: read a prompt from args/stdin → run the agent → stream events as plain text → exit. Approval prompts switch to a numbered stdin menu. Useful for scripts and CI.

---

## Logs

Per-agent JSON logs land under `log/<agent-id>/<agent-id>.log` by default. Set `LOG_DIR` in `.env` to redirect, or leave it unset to also stream to stdout. `LOG_LEVEL=debug` exposes every iteration's `turn.start` / `llm.call` / `tool.dispatch` / `tool.result` lines — handy when debugging an agent that's stuck or looping.

---

## Features

**Agent loop**
- ReAct-style: LLM call → parallel tool dispatch → tool results → repeat.
- Multiple `tool_use` blocks per turn, executed concurrently.
- Iteration cap surfaces as a pausable state.
- Cancellable via `ctx`; Esc / Ctrl+C honored end-to-end.

**LLM providers**
- Anthropic Claude (extended thinking + cryptographic signature round-trip).
- DeepSeek (OpenAI-compatible chat, reasoning_content echoed back).
- OpenAI.
- Ollama (local).
- Per-provider option pattern (`WithTemperature`, `WithEffort`, ...).

**Tools**
- File system: `read_file`, `write_file`, `edit_file` — strict-absolute paths, structured `*FileDiff` metadata for diff rendering.
- Shell: `bash`, `grep`, `tree`.
- Tasks (six tools sharing one observable `*task.Store`).
- Meta: `agent` (sub-agents), `tool_search` (lazy schema loading), `skill`, `schedule_wakeup`.
- Plus stubs for `web_*`, `cron_*`, `notebook_edit`, `monitor`, `mode`, `ux`.

**Sub-agents**
- `explore` (read-only) and `general-purpose` presets.
- Sync mode (parent blocks) and async mode (parent continues, result lands on next iteration).
- Two-layer hierarchy: sub-agents can't spawn sub-agents.

**Observable store framework** (`internal/observable`)
- One pub/sub primitive any store can embed. Adding a new panel costs zero edits to the agent or event packages.

**Swappable UI** (`internal/ui`)
- Narrow `UI` and `Controller` interfaces. Reference bubbletea TUI under `internal/ui/bubbletea/`. `-no-tui` falls back to a plain CLI sink.

**Streaming completions** (chunked text + thinking).

**2-level compaction**
- micro: compress tool-result blocks when context budget approaches threshold.
- full: summarize the whole session into a single assistant brief.

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
│   │   ├── claude/  deepseek/  ollama/
│   ├── llmfactory/            # provider factory keyed by constant
│   ├── logger/                # structured slog wrapper + pretty fmt
│   ├── observable/            # pub/sub framework for stores
│   ├── session/               # conversation history + cumulative usage
│   ├── tools/                 # tool interface (Name/Schema/Execute)
│   │   ├── cron/  dev/  fs/  meta/  mode/  monitor/  notebook/
│   │   ├── shell/ task/ ux/   web/
│   ├── toolset/               # tool catalog + ToolState registry
│   └── ui/                    # UI plugin contract
│       └── bubbletea/         # reference TUI implementation
├── log/                       # per-agent runtime logs (gitignored)
├── pkg/common/                # small shared utilities (UUID, ...)
└── scripts/                   # demo / dev scripts
```

Key boundaries:
- `agent` knows about `event.Sink`, never about a concrete UI.
- `tools/*` packages produce `tools.Result` (text + opaque `Metadata`); the UI type-asserts on `Metadata` to render structured payloads.
- `observable` has no dependencies on agent or UI.
- `ui` defines two narrow interfaces; implementations live under it.

---

## Roadmap

### In progress / next up
- System prompts refinement (main / explore / general-purpose / agent-specific).
- Tool implementations: `skill`.

### Planned
- **Multimodal Read**: images, PDFs (with `pages` range), Jupyter notebooks.
- **Overwrite diffs**: proper Myers/Hunt-McIlroy diff for `write_file` overwrites.
- **Per-agent LLM**: sub-agent can use a different provider than its parent.
- **Veronica space**: long-running local sandbox service on `:8080`.
- **Web UI**: a second `UI` implementation served over WebSocket.
- **Session persistence**: `/resume` to reload a session snapshot.

### Known limitations
- Sub-agent hierarchy is exactly two layers (no nested spawning).
- Token counts depend on provider reporting — Ollama only reports prompt / eval, not cache or reasoning splits.
- The TUI transcript grows unbounded in a long session; compaction is on the list above.

---

## License

See [LICENSE](LICENSE).
