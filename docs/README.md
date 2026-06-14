# evva documentation

This directory holds evva's documentation, grouped by audience. Start here.

| If you want to… | Go to |
|---|---|
| **Install and use evva** (TUI, config, commands) | [user-guide/](user-guide/README.md) |
| **Run a multi-agent swarm** | [user-guide/swarm/](user-guide/swarm/en.md) · [agent-guide/](user-guide/agent-guide/) |
| **Embed evva or extend it** (SDK, custom UI, stability) | [contributing/](contributing/README.md) |
| **Understand the architecture** (vision, SDK & package layout) | [architecture.md](architecture.md) |
| **Contribute code** (setup, PR flow) | [../CONTRIBUTING.md](../CONTRIBUTING.md) |
| **See what's planned / shipping** | [roadmap/](roadmap/README.md) |

## Layout

```
docs/
├── architecture.md # vision + architecture (the pkg/ + internal/ reference)
├── user-guide/     # END-USER — install, TUI, config, single-agent + swarm guides
│   ├── en/  zh-tw/ # localized single-agent guides (TUI, LSP, integration, Windows, service)
│   ├── swarm/      # 0→hero swarm walkthrough (en / zh)
│   └── agent-guide/# swarm field reference (every manifest field, tool, pattern)
├── contributing/   # CONTRIBUTOR — embed evva, SDK stability, implement a UI
├── roadmap/        # INTERNAL PLANNING — PRDs, design docs, wave plans, Veronica
├── reference/      # RESEARCH NOTES — Claude Code tool/prompt notes we port from
├── testing/        # manual / acceptance test scenarios
└── assets/         # logo + screenshots used by the docs
```

## Also at the repo root

- **[README.md](../README.md)** — project front door (install, quick start, features).
- **[CLAUDE.md](../CLAUDE.md)** / **[EVVA.md](../EVVA.md)** — the agent instruction files (coding
  conventions & release workflow): CLAUDE.md is what Claude Code reads, EVVA.md is what evva
  reads; kept in sync.
- **[CONTRIBUTING.md](../CONTRIBUTING.md)** — how to build, test, and submit changes.
- **[CHANGELOG.md](../CHANGELOG.md)** — release history.

> **Note for doc edits:** `user-guide/agent-guide/` is fetched at runtime by a bundled skill
> via a hardcoded `raw.githubusercontent.com` URL — **do not move or rename it.** See
> [../CONTRIBUTING.md](../CONTRIBUTING.md#documentation-changes).
