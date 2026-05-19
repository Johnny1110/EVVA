# EVVAgent — User Guide

## Table of Contents

- [1. Overview — TUI at a Glance](#1-overview--tui-at-a-glance)
- [2. Slash Commands](#2-slash-commands)
  - [/config — Runtime Settings](#config--runtime-settings)
  - [/model — Switch Provider/Model](#model--switch-providermodel)
- [3. Keybindings](#3-keybindings)
- [4. Yank Mode — Copying from the Transcript](#4-yank-mode--copying-from-the-transcript)
- [5. Transcript Search](#5-transcript-search)
- [6. Permission System](#6-permission-system)
  - [Permission Modes](#permission-modes)
  - [Approval Prompts](#approval-prompts)
  - [Permission Rules](#permission-rules)
- [7. Sub-agents](#7-sub-agents)
- [8. Configuration Reference](#8-configuration-reference)
  - [evva-config.yml](#evva-configyml)
  - [.env](#env-optional)
  - [CLI Flags](#cli-flags)
- [9. Modes — TUI vs CLI](#9-modes--tui-vs-cli)
- [10. Logs](#10-logs)

---

## 1. Overview — TUI at a Glance

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

Panels collapse to zero height when empty. The status bar is always visible at the bottom.

---

## 2. Slash Commands

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
| `/config` | open the settings form |
| `/model` | switch LLM provider / model — **clears conversation history** |
| `/clear` | clear the transcript (keeps the banner) |
| `/exit`, `/quit` | quit |

### /config — Runtime Settings

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
| `Enter` | edit the focused field (booleans toggle in-place) |
| `Enter` (in editor) | apply and save |
| `Esc` | cancel the edit (or close the panel from list mode) |

API key fields open a password-masked editor; pasting works (display stays masked).

**Live-applied** (takes effect immediately):

- `max_iterations` — the loop's safety cap
- `display_thinking` — toggles thinking blocks in the transcript
- `auto_compact_threshold` — when context compaction kicks in

**Persisted but next-launch only** (would require rebuilding the client / web tools):

- `max_tokens`, `fetch_max_bytes`, `tavily_api_key`, all `<provider>.api_key`, all `<provider>.api_url`

Every edit writes immediately to `~/.evva/config/evva-config.yml`.

### /model — Switch Provider/Model

Opens a flat list of every `(provider, model)` pair the binary knows about, cursor pre-positioned on the active one:

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

| key | effect |
| --- | --- |
| `↑` / `↓` | navigate the list |
| `Enter` | switch to the highlighted model |
| `Esc` | cancel |

**Important:** switching always clears the session. Anthropic's `ThinkingSignature` is provider-locked — carrying old history across a swap would 400 on the next request. The new choice is also persisted as `default_provider` + `default_model` so your next launch starts there.

Switching is refused if a run is in flight; press Esc first to cancel, then `/model` again.

---

## 3. Keybindings

| key | effect |
| --- | --- |
| `Enter` | submit |
| `Ctrl+J` / `Alt+Enter` | insert newline (multi-line composition) |
| `↑` / `↓` | walk prompt history (when input empty or already navigating) |
| `Esc` | cancel running task / dismiss panel |
| `Ctrl+C` | once: cancel running task · idle: quit |
| `Ctrl+D` | quit (when input is empty) |
| `Ctrl+O` | toggle expand-all tool results (fold/unfold long bash + read output) |
| `Ctrl+Y` | open **yank mode** — pick a block and copy its clean content |
| `Ctrl+F` | open **transcript search** — type a query, `Enter`/`n` cycles matches |
| `Shift+Tab` | cycle the **permission mode** — `default → accept_edits → plan → bypass → …` |
| `PgUp` / `PgDown` / `Home` / `End` | scroll transcript |
| mouse wheel | scroll transcript |

---

## 4. Yank Mode — Copying from the Transcript

The transcript renders each block with a left-edge timeline gutter (`│`, `├─`, etc.) so the conversation reads as a structured stream. The downside: a normal terminal drag-select copies whatever is visually on screen — gutter glyphs included. Pasting that into another window gives you something like:

```
▶ who are you?
│
│ I'm evva — an interactive coding assistant…
│
```

To copy clean content without the chrome, evva ships a **yank mode** that knows about block boundaries. It's the canonical clean-copy path; on terminals that don't fully support clipboard escapes, it's also the only one that works at all.

**Open with `Ctrl+Y`.** A cyan-bold gutter accent appears on one block at a time; the contextual hint above the status bar shows your cursor position (`yank 3/5`) and the key map.

| key | effect |
| --- | --- |
| `j` / `↓` | next block (newer) |
| `k` / `↑` | previous block (older) |
| `g` | jump to the first block |
| `G` | jump to the last block |
| `Enter` / `c` | copy the focused block's clean text to the system clipboard |
| `e` | toggle expand-all on this block only (handy for long tool results before copying) |
| `q` / `Esc` | exit yank mode (clears the accent) |
| `Ctrl+C` | exit + quit evva |

**What gets copied.** Each block exposes a `PlainText()` view that strips ANSI escapes and gutter glyphs. For a user prompt that's the prompt text. For assistant text it's the markdown source (not the rendered output). For a tool block it's the call head (`◢ name(...)`) followed by the result body. The status bar flashes `copied N chars` on success.

**How it gets there — OSC52.** Yank mode writes the payload to your clipboard using the [OSC52](https://wezfurlong.org/wezterm/escape-sequences.html#operating-system-command-sequences) terminal escape sequence. No external library, no `pbcopy` shell-out. The terminal forwards the escape to the OS clipboard.

| terminal | works out of the box? |
| --- | --- |
| **iTerm2** | yes (default) |
| **kitty** | yes |
| **WezTerm** | yes |
| **Alacritty** | yes |
| **Ghostty** | yes |
| **Apple Terminal.app** | no by default — enable `Edit → Allow clipboard access` or switch terminals |
| **tmux** | yes if `set -g set-clipboard on` |
| **GNU screen** | mostly broken; use Ctrl+Y from inside a host terminal instead |

If the write fails (payload too large at >100 KB, terminal blocked it), the status bar shows `clipboard: <error>` and yank mode stays open so you can try a different block.

**Why not native drag-select?** evva turns on mouse capture so the wheel can scroll the transcript. That trade-off means drag-and-drop copy stops happening natively — and even when modern terminals honor a `Shift`/`Alt`+drag escape hatch, the resulting selection still includes the rendered gutter glyphs (since they're part of what's painted on screen). Yank mode is the workflow that round-trips clean content out of the program.

---

## 5. Transcript Search

Press `Ctrl+F` to open the search bar. Type your query and press `Enter` to jump to the first match. Press `n` to cycle forward through matches, or `N` (Shift+n) to cycle backward. Press `Esc` to close the search bar.

---

## 6. Permission System

### Permission Modes

evva gates every tool call through a **permission mode**. Four modes, cycled with `Shift+Tab`:

| mode | auto-allowed without asking | best for |
| --- | --- | --- |
| **`default`** | Read-only tools (`read`, `tree`, `grep`, `glob`, `web_*`, `json_query`, `calc`), agent self-coordination (`agent`, `task_*`, `skill`, `tool_search`, `ask_user_question`), and **read-only bash commands** (`ls`, `cat`, `head`, `grep`, `git status`, `git log`, …). File writes and any other bash command **ask**. | Beginners, sensitive work, default stance |
| **`accept_edits`** | Same as `default` + file edits (`edit`, `write`, `notebook_edit`) + common filesystem bash commands (`mkdir`, `touch`, `mv`, `cp`, `rmdir`, `ln`, `chmod`, `chown`). | Iterating on code under review |
| **`plan`** | Same read-only safelist as `default`. Anything outside that set is **denied outright** (no prompt). | Exploring a codebase before deciding what to change |
| **`bypass`** | Everything. Dangerous-command classification still logs in the background, but never blocks. | **Isolated containers and VMs only** — propagates to subagents |

The active mode shows in the status bar as a colored badge (`⛨ plan`, `⛨ bypass`, …). `default` collapses the cell so the bar isn't noisy.

**Starting in a specific mode:**

```bash
evva -permission-mode=plan                # safest: investigate first
evva -permission-mode=accept_edits        # auto-apply edits + safe fs cmds
evva -permission-mode=bypass              # no prompts; sandboxed envs only
```

The CLI flag takes precedence; a persistent default lives in `evva-config.yml`:

```yaml
permission_mode: default     # default | accept_edits | plan | bypass
```

### Approval Prompts

In `default` / `accept_edits` / `plan` modes, anything that needs your approval opens a modal:

```
┌─ APPROVAL ─────────────────────────────────────────┐
│ tool: bash                                         │
│ mode: default  risk: dangerous (sudo)              │
│ reason: matches dangerous prefix                   │
│                                                    │
│ input: sudo rm /tmp/evil-file                      │
│                                                    │
│ ▶ [1] Allow once                                   │
│   [2] Allow for this session                       │
│   [3] Deny                                         │
│                                                    │
│ [↑↓] choose · [Enter] confirm · [Esc] deny         │
└────────────────────────────────────────────────────┘
```

| key | effect |
| --- | --- |
| `↑` / `↓` | move between buttons |
| `1` / `a` | Allow once — runs this call only |
| `2` / `s` | Allow for this session — also adds an in-memory rule so similar calls don't re-prompt |
| `3` / `d` | Deny — Enter again to type an optional reason for the model |
| `Enter` | confirm the highlighted choice (or commit a deny reason) |
| `Esc` | shortcut for deny |
| `Ctrl+C` | deny + quit |

**"Allow for this session"** picks a sensible rule shape from the call: for `bash` it stores the first token (so approving `git status` allows future `git …` calls, not arbitrary commands); for `read`/`write`/`edit` it stores the file path; other tools become tool-wide. Session rules vanish when you quit; persist them by hand-editing `permissions.json`.

Parallel approvals (the agent emitting two `bash` calls in one turn) stack — resolve the top one and the next surfaces.

### Permission Rules

Rules persist your approvals so you don't see the same prompt twice across runs. Two scopes:

- `<workdir>/.evva/permissions.json` — **project**: lives with the repo, share via git if you want
- `~/.evva/permissions.json` — **user**: applies in every working directory

Format:

```json
{
  "permissions": {
    "allow": [
      "bash(git:*)",
      "bash(npm:*)",
      "read(src/**)",
      "edit",
      "tree"
    ],
    "deny": [
      "bash(sudo:*)",
      "bash(rm -rf /)"
    ],
    "ask": [
      "bash(npm publish)"
    ]
  }
}
```

**Rule grammar**: `ToolName` matches every call to that tool. `ToolName(content)` adds a content match:

| tool | content syntax | examples |
| --- | --- | --- |
| `bash` | `prefix:*`, `pattern *`, `git *`, or exact command | `bash(git:*)`, `bash(npm install *)`, `bash(make build)` |
| `read`, `write`, `edit`, `notebook_edit` | doublestar glob against the `file_path` | `read(src/**)`, `write(./tmp/*.txt)`, `edit(**/*.go)` |
| anything else | exact string match against the raw input | rare; prefer tool-wide rules |

**Precedence:**

1. `bypass` mode — always allow, rules ignored.
2. **deny rules** — checked first, win over allow in every non-bypass mode.
3. **ask rules** — force a prompt even if a broader allow (or mode safelist) would have matched.
4. `plan` mode + tool not in read-only safelist → **deny** (no prompt).
5. Read-only / self-coordination safelist → allow.
6. Bash + classifier says read-only (`ls`, `cat`, `git status`, …) → allow.
7. `accept_edits` only: `edit`/`write`/`notebook_edit` → allow; bash common-fs command (`mkdir`/`mv`/`cp`/…) → allow.
8. **allow rules** — match → run.
9. Fallback — ask.

Source priority within each behavior (deny/ask/allow) is `session > project > user`, so a session "allow for this session" beats a user-scope rule but never beats a deny.

---

## 7. Sub-agents

The root agent can spawn sub-agents (`explore` for read-only inspection, `general-purpose` for write-capable). Active sub-agents appear as chips in a horizontal strip above the input. Async sub-agents finish in the background — their summaries land as a synthetic user message at the top of the next iteration, so the conversation picks them up automatically.

You don't drive sub-agents yourself; the model decides when to spawn one. Two-layer hierarchy by design (sub-agents can't spawn sub-agents).

---

## 8. Configuration Reference

### evva-config.yml

Path: `~/.evva/config/evva-config.yml`. Created automatically on first launch. Edit live via `/config` in the TUI, or by hand:

```yaml
# Agent loop
max_iterations: 30
max_tokens: 4096
auto_compact_threshold: 0.8
display_thinking: true

# Default model used at startup (overwritten by /model swap)
default_provider: deepseek
default_model: deepseek-v4-pro

# Permission stance at startup. Cycle at runtime with Shift+Tab; -permission-mode CLI flag overrides.
permission_mode: default     # default | accept_edits | plan | bypass

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

### .env (optional)

Place in your working directory or at `~/.evva/.env`. Only used for deployment / logging knobs — never user preferences:

```bash
APP_ENV=dev            # dev | prod
LOG_LEVEL=info         # debug | info | warn | error
LOG_FORMAT=text        # text | json
LOG_DIR=               # empty → stdout; path → write log files there
SKILLS_DIR=skills      # subpath under ~/.evva/
USER_PROFILE=user_profile.md
```

### CLI Flags

```bash
evva                                # interactive TUI (when stdout is a TTY)
evva -temp 0.7                      # sampling temperature (default unset)
evva -max-tokens 2048               # per-completion output cap (overrides YAML)
evva -max-iters 40                  # loop iteration cap (overrides YAML)
evva -permission-mode=plan          # boot in plan mode (read-only)
evva -permission-mode=bypass        # boot with the gate disabled
evva -no-tui "explain loop.go"      # one-shot plain-text mode
echo "list files in /tmp" | evva -no-tui   # piped prompt
```

---

## 9. Modes — TUI vs CLI

**Interactive TUI** (default when stdout is a TTY). Transcript, panels, status bar, the works.

**Plain CLI** (`-no-tui`, or when stdout is piped). One-shot flow: read a prompt from args/stdin → run the agent → stream events as plain text → exit. CLI mode has no interactive approval surface — any call that would prompt is **denied automatically** with a hint to pass `-permission-mode=bypass` or add a rule to `permissions.json`. Useful for scripts and CI.

---

## 10. Logs

Per-agent JSON logs land under `log/<agent-id>/<agent-id>.log` by default. Set `LOG_DIR` in `.env` to redirect, or leave it unset to also stream to stdout. `LOG_LEVEL=debug` exposes every iteration's `turn.start` / `llm.call` / `tool.dispatch` / `tool.result` lines — handy when debugging an agent that's stuck or looping.
