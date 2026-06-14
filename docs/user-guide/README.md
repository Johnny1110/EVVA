# User guide

Everything you need to install, configure, and operate evva — as a single terminal agent and
as a multi-agent swarm.

## Single-agent (the TUI)

| Topic | English | 正體中文 |
|---|---|---|
| Full usage guide — TUI, slash commands, keybindings, permissions, config | [en/user-guide.md](en/user-guide.md) | [zh-tw/user-guide.md](zh-tw/user-guide.md) |
| Windows setup (Git Bash, `EVVA_SHELL`, SmartScreen) | [en/windows.md](en/windows.md) | [zh-tw/windows.md](zh-tw/windows.md) |
| Run as a background service (launchd / systemd) | [en/service-autostart.md](en/service-autostart.md) | [zh-tw/service-autostart.md](zh-tw/service-autostart.md) |
| LSP integration | [en/lsp.md](en/lsp.md) | [zh-tw/lsp.md](zh-tw/lsp.md) |
| Embed evva in your Go project | [en/integration.md](en/integration.md) | [zh-tw/integration.md](zh-tw/integration.md) |

## Swarm (multi-agent)

- **[swarm/en.md](swarm/en.md)** · **[swarm/zh-tw.md](swarm/zh-tw.md)** — a 0→hero walkthrough:
  concepts, building a swarm from scratch, the web workstation (`:8888`), day-2 ops,
  restart-resume.
- **[agent-guide/](agent-guide/)** — the swarm **field reference**: every manifest field,
  every tool, coordination patterns, and role-specific recipes. Written agent-first (an agent
  can read it to help you build a swarm) but human-readable too.

Ready-to-run example swarms live in [`../../examples/evva-swarm/`](../../examples/evva-swarm/).

> `agent-guide/` is fetched live by the bundled `setup-swarm` skill via a pinned
> `raw.githubusercontent.com/.../docs/user-guide/agent-guide/<path>` URL. **Its path is a
> runtime contract — don't move or rename it.**
