#!/usr/bin/env bash
#
# depcheck enforces Veronica global invariant #1 — the multi-agent oracle:
# everything under internal/swarm/** must consume agent functionality through
# the public pkg/* surface ONLY, never internal/agent or any other evva
# internal package. The one sanctioned exception is the public inbox-drainer
# seam on pkg/agent (SPRD-1-12), which is public by design.
#
# Keeping the swarm pkg-pure makes it evva's multi-agent completeness oracle:
# if evva's own swarm can be built on pkg/* alone, a third party's can too.
#
# See: docs/veronica/prd-phase1-swarm.md §5.5, docs/veronica/phase-1-sub-tickets/SPRD-1-1.
set -euo pipefail

MODULE="github.com/johnny1110/evva"

# All transitive Go deps of the swarm subsystem.
deps="$(go list -deps ./internal/swarm/... 2>/dev/null)"

# evva-internal deps that are NOT under internal/swarm itself are violations
# (pkg/* and third-party/stdlib are fine; only evva/internal/* is forbidden).
violations="$(printf '%s\n' "$deps" \
  | grep -E "^${MODULE}/internal/" \
  | grep -vE "^${MODULE}/internal/swarm($|/)" || true)"

if [ -n "$violations" ]; then
  echo "FAIL: internal/swarm must import only pkg/* (+ internal/swarm)."
  echo "Forbidden evva-internal imports found in the swarm dependency graph:"
  printf '%s\n' "$violations" | sed 's/^/  - /'
  exit 1
fi

echo "OK: internal/swarm depends only on pkg/* (+ internal/swarm itself)."
