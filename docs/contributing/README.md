# Contributor & embedder docs

Reference material for developers who want to **embed evva as a library** or **extend its
runtime**. For getting set up to contribute (build, test, PR flow), start with the root
**[CONTRIBUTING.md](../../CONTRIBUTING.md)**.

| Doc | What it covers |
|---|---|
| [extending.md](extending.md) | Embedding evva's agent runtime as a library: every public `pkg/` package, its exports, and when to use each. |
| [sdk-stability.md](sdk-stability.md) | The stability contract per `pkg/` tier (Stable / Experimental / Internal) and what "breaking" means. |
| [implementing-a-ui.md](implementing-a-ui.md) | Build a custom terminal UI (or any frontend) against the `ui.UI` + `event.Sink` + `ui.Controller` contract. |

## See also

- **[../architecture.md](../architecture.md)** — vision & architecture: the full `pkg/` /
  `internal/` package tables and key boundaries.
- **[../../CLAUDE.md](../../CLAUDE.md)** / **[../../EVVA.md](../../EVVA.md)** — the agent
  instruction files: coding conventions and the release workflow (CLAUDE.md for Claude Code,
  EVVA.md for evva; kept in sync).
- **[../../examples/](../../examples/)** — `minimal-host` and `full-host` embedding examples.
