# Contributing to evva

Thanks for your interest in evva — a ReAct coding agent for the terminal, written in Go.
This guide covers getting set up, the development loop, and how changes land. For the
project's vision and architecture see **[docs/architecture.md](docs/architecture.md)**; for
coding conventions and the release workflow see the agent instruction files
**[CLAUDE.md](CLAUDE.md)** / **[EVVA.md](EVVA.md)**.

## Prerequisites

- **Go 1.25+** (the module targets `go 1.25.9`). `go install`/source builds require it.
- **git**, and a POSIX shell. On Windows, the agent's `bash` tool runs through
  [Git for Windows](https://gitforwindows.org); see [docs/user-guide/en/windows.md](docs/user-guide/en/windows.md).
- **Node 18+** only if you're working on the swarm web workstation under `web2/`.

## Build, test, run

The [`Makefile`](Makefile) wraps the common tasks:

```bash
make build        # build ./bin/evva (with version ldflags)
make install      # build + install to $GOBIN (override: make install PREFIX=$HOME/.local/bin)
make run          # go run ./cmd/evva
make test         # go test -race -cover ./...
make vet          # go vet ./...
make fmt          # go fmt ./...
make depcheck     # enforce the internal/swarm import boundary (scripts/depcheck.sh)
make all          # fmt + vet + depcheck + test + build
```

Plain `go build ./... && go test ./...` works too. Tests live next to the code they cover
(`*_test.go`) — there is no parallel `tests/` tree.

### Web workstation (`web2/`)

```bash
cd web2
npm install
npm run dev        # Vite dev server
npm run test       # unit + lib tests
npm run build      # type-check + production build
```

See [`web2/README.md`](web2/README.md) for details.

## Repository layout

A short map (full package-by-package tables are in [docs/architecture.md](docs/architecture.md)):

| Path | What |
|---|---|
| `cmd/evva/` | CLI entry point — TUI plus `service` / `swarm` / `update` subcommands |
| `pkg/` | Stable public SDK (embed evva as a library) — `agent`, `llm`, `tools`, `ui`, … |
| `internal/` | Runtime-specific implementations (not part of the public API) |
| `web2/` | Vue 3 + TypeScript swarm web workstation |
| `examples/` | Embedding hosts + ready-to-run example swarms |
| `docs/` | Documentation — see [docs/README.md](docs/README.md) for the map |
| `ref/` | Reference TypeScript source we port tool descriptions from |

**Public vs. private:** reusable abstractions go in `pkg/`; evva-runtime-specific code goes in
`internal/`. Downstream embedders import `pkg/` and never touch `internal/`. The full set of
conventions (one package per tool family / provider, comment policy, dependency budget) is in
[CLAUDE.md → Project conventions](CLAUDE.md#project-conventions).

## Submitting changes

1. **Branch off `dev`:** `git checkout -b feature/<short-name>`.
2. **Commit with conventional prefixes:** `feat`, `fix`, `chore`, `docs`, `refactor`, `test`.
3. **Keep it green:** `make all` (or at least `go test ./...` + `go vet ./...`) before pushing.
4. **Open a PR targeting `dev`.** Fill in the PR template; link any related issue.
5. After review it's squash/merged into `dev`.

`dev` is the integration branch; releases flow `dev → pre-release → main`. The full tag/release
process (the four release commands, version numbering, CHANGELOG rules) lives in
[CLAUDE.md → Release workflow](CLAUDE.md#release-workflow) and is maintainer-driven — you don't
need it for a normal contribution.

## Documentation changes

Docs live under [`docs/`](docs/README.md), grouped by audience (user-guide / contributing /
roadmap / reference / testing). One caveat worth knowing:

> **`docs/user-guide/agent-guide/` is a runtime fetch contract.** Its path is hardcoded as a
> `raw.githubusercontent.com/.../main/docs/user-guide/agent-guide/<path>` URL inside a bundled
> skill that ships in the binary. **Do not move or rename that subtree** without updating every
> referencing URL — moving it breaks swarm setup for already-released builds.

## Reporting bugs & requesting features

Open a [GitHub issue](https://github.com/johnny1110/evva/issues) using the bug-report or
feature-request template. For security-sensitive reports, please avoid filing a public issue
with exploit details — note that the swarm web workstation binds loopback only and its event
webhook is unauthenticated by design in the current phase (local integrations).
