# Veronica — multi-agent swarm project

Veronica is evva's in-repo multi-agent swarm subsystem (`internal/swarm/`, the `:8888` web
workstation). This folder is its **planning home**. User-facing guides and runnable examples
have moved out (see below).

## Design & direction

| Doc | What |
|---|---|
| [veronica-design-v1.md](veronica-design-v1.md) | Core design: architecture, mental model, process model, message bus, task ledger. |
| [direction-flat-comms.md](direction-flat-comms.md) | Design direction — flat messaging (any member can message any member). |
| [roadmap.md](roadmap.md) | Project roadmap overview. |
| [health-check-2026-06-10.md](health-check-2026-06-10.md) | Latest health check + wave-4 triggers. |

## Phases

| Doc | What |
|---|---|
| [prd-phase1-swarm.md](prd-phase1-swarm.md) | Phase 1 PRD — swarm infrastructure. |
| [prd-phase2-trader-team.md](prd-phase2-trader-team.md) | Phase 2 PRD — trader-team validation. |
| [phase-1-dod-checklist.md](phase-1-dod-checklist.md) | Phase 1 definition-of-done checklist. |
| [phase-1-sub-tickets/](phase-1-sub-tickets/) | `SPRD-1-*` build breakdown for Phase 1. |

## Refinement & exploration

- **[refine-plan/](refine-plan/)** — the `RP-1`…`RP-29` refinement tickets, organized into
  five waves (see its [README](refine-plan/README.md)); includes the `fe-v2/` web UI 2.0 track.
- **[explore/](explore/)** — `EX-1`…`EX-6` exploratory spikes that graduate into RP tickets.
- **[_archive/](_archive/)** — superseded / AI-generated planning artifacts (e.g.
  persona-members planning). Paths inside predate the docs reorg; treat as historical.

## Moved out of this folder

- **User guides** → [../../user-guide/swarm/en.md](../../user-guide/swarm/en.md) ·
  [zh.md](../../user-guide/swarm/zh-tw.md)
- **Example swarms** → [`../../../examples/evva-swarm/starter/`](../../../examples/evva-swarm/starter/)
  (3-member) and [`tech-team/`](../../../examples/evva-swarm/tech-team/) (7-member).
