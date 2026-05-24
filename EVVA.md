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

## Project conventions

- All source under `internal/` is private. Public extension points live in `pkg/`.
- One package per tool family (`fs`, `shell`, `meta`, etc.). A new tool either goes in an existing family or starts a new family package.
- One package per LLM provider in `internal/llm/`. The `llm.Client` interface is the only public seam.
- Tests live next to the code they cover (`*_test.go`). No parallel `tests/` tree.
- No comments that restate the code. Only comment WHY when the WHY is non-obvious.
- Port tool descriptions from `ref/src/tools/*/prompt.ts` verbatim when reasonable. Diverge only with a clear reason.

---

## Release workflow

Every tagged release follows this sequence. No exceptions — follow it even when the change is a single-line fix.

### 1. Commit the actual changes

```
git add <files> && git commit -m "<type>: <description>"
```

Conventional commit prefixes: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`.

### 2. Bump the version

Edit `pkg/version/version.go` — update the `Version` constant (semver, no leading `v`).

### 3. Update CHANGELOG.md

Three things happen in one edit:

1. Rename the existing `## [Unreleased]` heading to `## [v<new-version>]` (the content that was unreleased now belongs to this release).
2. Insert a fresh `## [Unreleased]` section at the top. If there are known unresolved issues worth flagging, list them here under a `### Known issues` subheading. Otherwise leave it empty.
3. Insert a `## [v<new-version>]` entry (above the one from step 1) with a summary of what this release contains — use `### Added`, `### Fixed`, `### Changed`, `### Breaking` subheadings as appropriate.

Also update the comparison URLs at the bottom of the file — add links for the new version and bump the `[Unreleased]` compare base.

### 4. Commit the version bump

```
git add pkg/version/version.go CHANGELOG.md && git commit -m "chore: bump version to <new-version>"
```

### 5. Tag and push - Github Release:

example:

```
git tag -a v0.2.6-alpha.1 -m "v0.2.6-alpha.1 — Phase 16+17: bash run_in_background + MonitorTool"
git push origin v0.2.6-alpha.1 2>&1 | tail -3
gh release create v0.2.6-alpha.1 --target roadmap/phase-16-17 --prerelease --title "v0.2.6-alpha.1 — Bash run_in_background + MonitorTool"
```

Always ask before pushing — pushing is a shared-state operation.

### Files involved

| File | What changes |
|---|---|
| `pkg/version/version.go` | `Version` constant |
| `CHANGELOG.md` | Relabel `[Unreleased]` → version, add new `[Unreleased]` + version entry, bump compare URLs |

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