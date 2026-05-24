# v1.1 — Finish the Hooks System — Implementation Plan

> **Audience:** senior engineers implementing this phase.
> **Status:** ready to build.
> **Target release:** `v1.1.0` (additive, minor bump under the Stable-tier promise).
> **Roadmap source:** `CLAUDE.md` → Roadmap → *v1.1 — Finish the hooks system*.

---

## 1. TL;DR — what this phase actually is

The hooks **engine is already written and is essentially complete.**
`internal/hooks/` (9 files, ~1185 LOC) implements a six-event lifecycle
system — config loading, a registry, a per-agent dispatcher, shell +
HTTP backends, and ref-faithful decision parsing. It has been merged but
**never wired in**: `grep -rl "internal/hooks" --include='*.go'` (outside
`ref/`) returns **nothing**. The agent loop never calls it.

Meanwhile the system prompt already tells the model hooks work
(`internal/agent/sysprompt/fragments.go:59`). So today every session
ships a prompt that advertises a feature the runtime silently ignores.
That is the integrity defect this phase closes — and it is the reason
v1.1 is the **top** post-1.0 priority.

**This is therefore an integration phase, not a build phase.** The work is:

1. **Promote** `internal/hooks` → `pkg/hooks` (trivial: zero importers today).
2. **Construct** a registry + dispatcher at agent build time (one new
   `hooks.Load` call beside the existing `permission.Load`, one Option,
   one base-payload factory, subagent inheritance).
3. **Wire** the six fire points into the loop at known anchors.
4. **Test** the package (it has **0** tests) + one loop integration test.
5. **Document** + bump version.

Do **not** rewrite the engine. If something in `internal/hooks` looks
wrong, raise it — but the default posture is "wire what exists."

---

## 2. Inventory — what already exists (do not re-build)

All in `internal/hooks/` (to become `pkg/hooks/`):

| File | Provides | State |
| --- | --- | --- |
| `types.go` | `Event` (6 consts), `Command`, `Config`, `Decision`, `PreToolUseDecision` | complete |
| `payload.go` | `BasePayload` + per-event payloads (snake_case JSON, Claude-Code-compatible) | complete |
| `loader.go` | `Load(workdir, evvaHome) (*Registry, []Warning)` — reads `.evva/settings.json` + `<evvaHome>/settings.json`, the verbatim Claude Code `hooks` block; per-entry validation → `Warning` | complete |
| `registry.go` | `Registry` (`For`, `HasAny`, `ReplaceAll`); concurrency-safe | complete |
| `dispatcher.go` | `Dispatcher` + `NewDispatcher(reg, logger, baseFn, projectDir)`; `Has(e)`; all six `Fire*` methods; **nil-safe** (`d==nil` → noop) | complete |
| `decision.go` | stdout-JSON → `Decision` parsing; `applyPreToolUse` precedence (block, approve→allow, `hookSpecificOutput.permissionDecision` wins, last-write `updatedInput`, concatenated `additionalContext`) | complete |
| `runner.go` | shell subprocess: payload on stdin, env injection, exit-code semantics (0 parse / 1 nonblocking / 2 block / timeout) | complete |
| `http.go` | HTTP webhook backend (sync + async fire-and-forget) | complete |
| `matcher.go` | `matchTool(matcher, name)` via `doublestar` (empty = match-all) | complete |

**Dispatcher API the loop will call:**

```go
func (d *Dispatcher) Has(e Event) bool
func (d *Dispatcher) FireSessionStart(ctx, source, model string) (initialUserMessage, additionalContext string, err error)
func (d *Dispatcher) FireUserPromptSubmit(ctx, prompt string) (additionalContext string, blocked bool, blockReason string, err error)
func (d *Dispatcher) FirePreToolUse(ctx, toolName string, toolInput []byte, toolUseID string) (*PreToolUseDecision, error)
func (d *Dispatcher) FirePostToolUse(ctx, toolName string, toolInput []byte, toolResponse, toolUseID string, isError bool) (additionalContext string, err error)
func (d *Dispatcher) FireStop(ctx, lastMessage string, stopHookActive bool) (blocked bool, reason string, err error)
func (d *Dispatcher) FireNotification(ctx, message, title, ntype string)
```

`PreToolUseDecision` (the only non-obvious return shape):

```go
type PreToolUseDecision struct {
    PermissionDecision string // "" | "allow" | "deny" | "ask"
    Reason             string
    UpdatedInput       []byte // new tool input JSON, nil if unchanged
    AdditionalContext  string
    Blocked            bool
    BlockReason        string
}
```

---

## 3. Goal & acceptance criteria

**Goal:** a user who configures hooks in `settings.json` sees them fire at
all six events, with the documented effects, for the main agent and
subagents — and the system prompt's hooks promise becomes true.

Ship is complete when **all** of these pass:

- **A1 — PreToolUse block.** A `PreToolUse` hook with `matcher:"bash"`
  that exits 2 (or returns `{"decision":"block"}`) prevents the `bash`
  tool from executing; the model receives an `IsError` result carrying
  the block reason. The permission gate is **not** consulted for that call.
- **A2 — PreToolUse mutate.** A `PreToolUse` hook returning
  `hookSpecificOutput.updatedInput` changes the arguments the tool
  actually executes with.
- **A3 — PreToolUse permission override.** A hook returning
  `permissionDecision:"allow"` lets a call that would otherwise prompt run
  without asking; `"deny"` blocks it without asking.
- **A4 — PostToolUse context.** A `PostToolUse` hook's `additionalContext`
  is appended to that tool's result content and is visible to the model on
  the next turn.
- **A5 — SessionStart.** A `SessionStart` hook's `additionalContext` /
  `initialUserMessage` is present at the start of the first turn.
- **A6 — UserPromptSubmit.** `additionalContext` is appended to the
  prompt; a blocking hook drops the prompt with its reason surfaced.
- **A7 — Stop re-entry guard.** A blocking `Stop` hook re-enters the loop
  exactly once (cannot infinite-loop; `stop_hook_active` guards the
  second pass).
- **A8 — Resilience.** A malformed `settings.json` yields a `Warning`
  surfaced like permission/registry warnings; the agent still starts.
- **A9 — Subagents.** A spawned subagent fires hooks too, with
  `agent_id` / `agent_type` set to the **subagent's** identity in the
  payload.
- **A10 — Zero-cost when unused.** A session with no configured hooks does
  no extra subprocess work and builds no payloads (guard on
  `Dispatcher.Has(event)`).
- **A11 — Public + tested.** `pkg/hooks` compiles as a public package;
  `go test ./...` is green; new tests cover matcher / decision / loader /
  runner / http / dispatcher + one agent-loop integration test.

---

## 4. Work breakdown (ordered)

### Task 0 — Promote `internal/hooks` → `pkg/hooks`

Do this **first** — it is free. Nothing imports `internal/hooks` today, so
the move needs **no** import rewrites anywhere.

```bash
git mv internal/hooks pkg/hooks
# package clause stays `package hooks`; imports are stdlib + doublestar
# (doublestar is already a dependency via pkg/permission).
go build ./...   # should be green with zero edits
```

Rationale for moving first: every subsequent edit in `internal/agent`
should import the **final** path (`pkg/hooks`), so we don't touch import
lines twice. The package has no `internal/*` dependency, so it is a valid
Stable-candidate public package (tier decision deferred to Task 4).

### Task 1 — Construct the registry + dispatcher at build time

**1a. Load the registry in the converged constructor.**
In `pkg/agent/agent.go` `New(...)`, beside the existing permission load
(`permStore, _ = permission.Load(appCfg.WorkDir, appCfg.AppHome)`,
currently ~line 136), add:

```go
hookReg, hookWarns := hooks.Load(appCfg.WorkDir, appCfg.AppHome)
```

Append `WithHookRegistry(hookReg)` to the `base []Option` slice (next to
`WithPermissionStore(permStore)`). Surface `hookWarns` through the **same
path** the constructor already uses for `regWarns` / `memWarns` (startup
log / sink notice — match whatever those do; do not invent a new channel).

> `NewWithProfile` (the à-la-carte constructor) does **not** auto-load —
> hosts opt in via `WithHookRegistry`. This mirrors how `NewWithProfile`
> treats the permission store. Document in `extending.md` (Task 4).

**1b. Add the Option.** In `internal/agent/options.go`, mirror
`WithPermissionStore`:

```go
// WithHookRegistry installs the lifecycle-hook registry. nil is safe —
// the dispatcher noops when no registry is present. Shared across the
// root and its subagents (subagents inherit via spawn.go) so one
// settings.json load drives the whole agent tree.
func WithHookRegistry(r *hooks.Registry) Option {
    return func(a *Agent) { a.hookRegistry = r }
}
```

**1c. Agent fields.** In `internal/agent/agent.go`, beside
`permissionStore` / `permissionBroker` (~lines 155-157):

```go
hookRegistry   *hooks.Registry
hookDispatcher *hooks.Dispatcher
```

**1d. Build the dispatcher in `New`.** After `wireBrokers(a)`
(`internal/agent/agent.go:412`), add — **note: not root-only.** Every
agent (root *and* each subagent) builds its **own** dispatcher so the
base-payload factory bakes in that agent's identity, while the
`*Registry` is shared:

```go
a.hookDispatcher = hooks.NewDispatcher(
    a.hookRegistry,        // shared; may be nil → dispatcher noops
    a.logger,
    a.hookBaseFactory,     // see 1e
    a.workdir,             // becomes EVVA_PROJECT_DIR in hook env
)
```

`NewDispatcher` and every `Fire*`/`Has` are nil-safe, so a nil registry
needs no guarding at call sites.

**1e. Base-payload factory.** Add a small method (new file
`internal/agent/hooks_wiring.go` is the natural home):

```go
func (a *Agent) hookBaseFactory() hooks.BasePayload {
    return hooks.BasePayload{
        SessionID:      a.sessionID(),               // see note below
        TranscriptPath: a.sessionTranscriptPath(),   // optional; "" ok
        Cwd:            a.workdir,
        PermissionMode: string(a.PermissionMode()),
        AgentID:        a.ID,
        AgentType:      a.profile.Type.String(),
    }
}
```

> **Open item (minor, non-blocking):** there is no obvious stable
> `Session.ID` accessor today (`internal/session/session.go`). Source
> `SessionID` / `TranscriptPath` from the persistence layer
> (`internal/agent/persist.go`); if no stable id exists yet, use `a.ID`
> as the session id for v1.1 and note it. `BasePayload` builds fine with
> `TranscriptPath:""` (it is `omitempty`).

**1f. Subagent inheritance.** In `internal/agent/spawn.go` (~lines 80-85,
where the child opts already include `WithPermissionStore(a.permissionStore)`),
add:

```go
WithHookRegistry(a.hookRegistry), // share the parent's loaded hooks
```

The child's own `New` then builds its own dispatcher (1d) with its own
`hookBaseFactory`, so subagent payloads carry the subagent's
`agent_id`/`agent_type` (satisfies **A9**).

### Task 2 — Wire the six fire points

All anchors are current line numbers; re-confirm before editing.

#### 2a. SessionStart — once per session, main agent only
**Where:** `internal/agent/loop.go` `Run` (line 41), before appending the
user prompt (line 62).
**How:** fire only on the first `Run` of a session (add a `sync.Once` or
an `a.sessionStarted` bool; `Continue` must **not** re-fire it). Source =
`"startup"`. Prepend `initialUserMessage` as a synthetic `RoleUser`
message; fold `additionalContext` into the first turn (append after the
prompt, or as a leading reminder like `computePlanModeAttachments` does).
Skip for subagents (`a.IsSubagent()`).

```go
if !a.IsSubagent() && a.hookDispatcher.Has(hooks.EventSessionStart) && a.firstRun() {
    initMsg, addCtx, err := a.hookDispatcher.FireSessionStart(ctx, "startup", a.profile.LLMModel.Name)
    // err → log, continue (hooks are non-fatal)
    if initMsg != "" { a.session.Append(llm.Message{Role: llm.RoleUser, Content: initMsg}) }
    if addCtx != "" { /* append to the upcoming prompt */ }
}
```

#### 2b. UserPromptSubmit — main agent
**Where:** `internal/agent/loop.go` `Run`, immediately before line 62
(`a.session.Append(... RoleUser, Content: prompt)`).
**How:** if `blocked` → do **not** append the prompt; surface
`blockReason` (text event + early return). Else append `additionalContext`
to the prompt before appending the message.

> **Scope note:** the primary path is the `Run` entry. Prompts that land
> mid-run via `drainUserPrompts` (`state_machine.go:103`) are a **secondary**
> path — firing the hook there too is desirable but lower priority; if you
> defer it, note it explicitly in the PR. Do not silently skip it.

#### 2c. PreToolUse — before the permission gate (the load-bearing one)
**Where:** `internal/agent/state_machine.go` `execTool` (line 270), in the
gap **after** the `resolveToolErr` block (ends line 303) and **before**
`a.permissionGate(ctx, call)` (line 305).

**Composition with the gate** — this is the one real design seam. Today
both `permissionGate` (line 364) and `tool.Execute` (line 310) read
`call.Input`. To thread `updatedInput` cleanly **without** rewriting the
recorded assistant `ToolCall` (the transcript should show what the model
asked), introduce a local `effectiveInput` and pass it explicitly:

```go
effectiveInput := call.Input
var postCtx string // PreToolUse may also carry AdditionalContext for later

if a.hookDispatcher.Has(hooks.EventPreToolUse) {
    dec, err := a.hookDispatcher.FirePreToolUse(ctx, call.Name, effectiveInput, call.ID)
    if err != nil { a.logger.Warn("hooks.pretooluse", "err", err) } // non-fatal
    if dec != nil {
        if dec.Blocked {
            return a.toolError(call, dec.BlockReason), nil // skip gate + execute (A1)
        }
        if len(dec.UpdatedInput) > 0 { effectiveInput = dec.UpdatedInput } // A2
        if dec.AdditionalContext != "" { postCtx = dec.AdditionalContext }
        // A3: permissionDecision overrides the gate (see below)
    }
    // pass dec.PermissionDecision into the gate decision
}
```

Then refactor `permissionGate` to accept the effective input and an
optional override:

```go
func (a *Agent) permissionGate(ctx, call, effectiveInput []byte, override string) (bool, *llm.ToolResult)
```

- `override == "deny"` → return deny result immediately (no `Decide`, no broker).
- `override == "allow"` → return `(false, nil)` immediately (skip gate).
- `override == "ask"` → run the gate but force the `BehaviorAsk` branch
  even if a rule would allow.
- `override == ""` → unchanged behavior. Build `permission.ToolCall{Input: effectiveInput}`.

Pass `effectiveInput` to `tool.Execute` (line 310) as well.

#### 2d. PostToolUse — after the tool returns
**Where:** `internal/agent/state_machine.go` `execTool`, after the
successful `tool.Execute` (after line 318) and before constructing the
returned `*llm.ToolResult` (line 338).

```go
content := result.Content
if a.hookDispatcher.Has(hooks.EventPostToolUse) {
    if add, _ := a.hookDispatcher.FirePostToolUse(ctx, call.Name, effectiveInput, result.Content, call.ID, result.IsError); add != "" {
        content = content + "\n" + add // A4
    }
}
// also fold in PreToolUse's postCtx if you chose to defer it to here
```

Use `content` for both the emitted `KindToolUseResult` event (line 327)
and the returned `ToolResult.Content` (line 340), so the UI and the model
see the same augmented text.

> **Concurrency note:** `dispatchToolCalls` runs `execTool` for every call
> in a turn **in parallel** (`loop.go:319-327`). So Pre/PostToolUse hooks
> for *different* tools in one turn run concurrently; hooks *within* one
> tool's chain run sequentially (the dispatcher guarantees that). Keep all
> per-call state local (`effectiveInput`, `content`, `postCtx`) — never
> mutate shared `resp.ToolCalls` elements. This is why we use a local
> `effectiveInput` rather than writing back to `call.Input`.

#### 2e. Stop — terminal turn, main agent, re-entry-guarded
**Where:** `internal/agent/loop.go` `runLoop`, the terminal branch at
line 146 (`if len(resp.ToolCalls) == 0`), before `a.done(iter, resp)`
(line 172) — and **after** the existing `hasPendingSignals()` continue
(lines 153-163), so signal-driven re-entry keeps priority.

**How:** thread a loop-scoped `stopHookActive bool` (declared above the
`for` at line 86). On the terminal branch:

```go
if !a.IsSubagent() && a.hookDispatcher.Has(hooks.EventStop) {
    blocked, reason, _ := a.hookDispatcher.FireStop(ctx, resp.Content, stopHookActive)
    if blocked && !stopHookActive {
        stopHookActive = true
        a.session.Append(llm.Message{Role: llm.RoleUser, Content: reason})
        continue // re-enter exactly once (A7)
    }
}
```

The dispatcher already refuses to block when `stopHookActive` is true, so
the second terminal pass proceeds to `done()`. Verify this doesn't
double-persist or skip `done()`'s status transition.

#### 2f. Notification — out-of-band side channel (async)
**Where:** fire-and-forget at, minimum:
- iteration limit — `state_machine.go` `limitBreak` (line 250), ntype `"iter_limit"`.
- approval needed — in `wirePermissionBroker`'s sink-emit path
  (`approval.go:40-42`), ntype `"approval_needed"`, so a user can route a
  Slack ping on approvals.

```go
a.hookDispatcher.FireNotification(ctx, msg, title, "iter_limit")
```

`FireNotification` returns nothing and never blocks. Wiring `crush`
(Go-level errors) is optional/nice-to-have — note if deferred.

### Task 3 — Tests (package currently has 0)

Place `*_test.go` next to the code (`pkg/hooks/`), plus one integration
test in `internal/agent/`.

**Package unit tests (`pkg/hooks/`):**
- `matcher_test.go` — empty matcher matches all; literal match; glob;
  bad glob → false.
- `decision_test.go` — parse: empty/non-JSON → empty Decision;
  `continue:false` → block; `decision:"block"`; `decision:"approve"` →
  allow; `hookSpecificOutput.permissionDecision` beats top-level;
  `updatedInput` last-write-wins; `additionalContext` concatenation.
- `loader_test.go` — valid file; missing file → no error; invalid JSON →
  Warning; unknown event → Warning; bad matcher glob → Warning, matcher
  skipped; `type=command` without `command` → Warning; `type=http`
  without `url` → Warning; timeout out of `[1,600]` → Warning; HTTP async
  default = true; **project-before-user merge order**.
- `runner_test.go` — write a tiny shell script to a temp dir; assert exit
  0 stdout parsed, exit 1 ignored (logged), exit 2 → block with stderr
  reason, timeout honored.
- `http_test.go` — `httptest.Server`: 2xx → nil; non-2xx → error in sync
  mode; custom headers/method sent; async returns immediately.
- `dispatcher_test.go` — hand-build a `Registry` (`NewRegistry` +
  `ReplaceAll`) with `command` hooks pointing at `echo '<json>'` scripts;
  exercise `FirePreToolUse` (block short-circuits the chain; `updatedInput`
  threads to the next hook; `permissionDecision` surfaces),
  `FirePostToolUse` (additionalContext concatenated across hooks),
  `FireSessionStart`, `FireUserPromptSubmit` (block), `FireStop`
  (`stopHookActive` suppresses the block on the second call).

**Integration test (`internal/agent/`):** reuse the existing stub-LLM
harness (`stubClient` / `recordingSink`, the `seedDiskAgent` pattern).
Build an agent whose stub emits one `bash` tool call. Point
`WithHookRegistry` at a registry whose `PreToolUse[matcher:bash]` runs a
temp shell script. Assert:
- script exits 2 → tool blocked, gate not reached, `IsError` result has
  the reason (A1);
- script emits `updatedInput` → tool executed with mutated args (A2);
- a `PostToolUse` script's `additionalContext` appears in the recorded
  tool-result content (A4).

### Task 4 — Docs + version

- `docs/sdk-stability.md` — add a `pkg/hooks` row. Recommended tier:
  **Experimental** for v1.1 (it is newly public and the payload shape may
  yet flex); promote to Stable once a downstream consumes it. State the
  rationale in the PR.
- `docs/extending.md` — new "Lifecycle hooks" section: the `settings.json`
  `hooks` block shape, the six events, the decision JSON contract, and the
  `WithHookRegistry` opt-in for `NewWithProfile`. Cross-link from the
  public-package table.
- `docs/user-guide/en/user-guide.md` (+ `zh-tw` mirror) — a short user
  section: how to declare a hook in `.evva/settings.json`, with a
  copy-paste PreToolUse example. (zh-tw mirror per the project's existing
  bilingual convention.)
- `CHANGELOG.md` — `## [v1.1.0]` entry: "Added: lifecycle hooks
  (SessionStart / UserPromptSubmit / PreToolUse / PostToolUse / Stop /
  Notification), `pkg/hooks`, `agent.WithHookRegistry`."
- `pkg/version/version.go` — `Version = "1.1.0"`.
- `internal/agent/sysprompt/fragments.go:59` — **no change needed**; the
  existing hooks paragraph becomes true once wired. Re-read it against the
  shipped behavior and confirm the wording still holds.

---

## 5. Design decisions & risks (read before coding)

- **Engine is frozen.** Treat `pkg/hooks` internals as done. All new code
  lives in `pkg/agent` (construction) and `internal/agent` (wiring) + tests.
- **`effectiveInput`, not `call.Input` mutation.** Keeps the recorded
  assistant `ToolCall` faithful to what the model emitted while letting
  the tool run with the hook's `updatedInput`. Avoids data races under
  parallel dispatch.
- **Hooks are non-fatal.** Every `Fire*` error is logged and treated as
  pass-through. A broken hook must never abort the agent loop (except the
  *intended* block paths: PreToolUse block, UserPromptSubmit block, Stop
  re-entry).
- **Zero-cost gating.** Guard every fire site with
  `a.hookDispatcher.Has(event)` so a no-hook session builds no payloads
  and spawns no subprocesses (A10).
- **Per-agent dispatcher, shared registry.** Dispatcher construction is
  **not** inside the root-only `wireBrokers` guard — each subagent needs
  its own `baseFn`. Only the `*Registry` is shared down the tree.
- **Stop loop safety.** The `stopHookActive` flag is the only thing
  preventing an infinite re-entry loop. Test it explicitly (A7), and make
  sure it composes with the existing `hasPendingSignals()` continue.
- **`/bin/sh` only.** `runner.go` shells out via `/bin/sh -c` — this is
  POSIX-only and aligns with evva's bash-first stance. Windows shell hooks
  are out of scope (tracked under the roadmap's cross-platform item).
- **Security surface.** Hooks run arbitrary user-configured commands by
  design (same trust model as Claude Code). No sandboxing in v1.1; the
  source is the user's own `settings.json`. Call this out in the docs but
  do not add gating — it would diverge from the reference contract.

---

## 6. Out of scope for v1.1

- New hook **events** beyond the six already defined.
- `PreCompact` / `SessionEnd` / resume / clear `source` values (payload
  comments mention them as "reserved" — leave reserved).
- A public `Broker`-style API for hooks (the question flow's event +
  response pattern is not needed; hooks are config-driven only).
- Hot-reload of `settings.json` mid-session (`Registry.ReplaceAll` exists,
  but no reload trigger is wired — defer).
- Windows / PowerShell hook execution.

---

## 7. Verification checklist (PR gate)

- [ ] `git mv internal/hooks pkg/hooks`; `go build ./...` green with no
      import rewrites.
- [ ] `pkg/agent.New` loads hooks beside `permission.Load`; warnings
      surfaced like `regWarns`/`memWarns`.
- [ ] All six fire points wired at the anchors in §4.2; subagents inherit
      the registry (A9).
- [ ] Acceptance A1–A11 demonstrably pass (A1–A4 + A7 via the integration
      test; A8 via loader test; A10 by inspection of `Has` guards).
- [ ] `go test ./...` green; `go vet ./...` clean.
- [ ] `pkg/hooks` row in `sdk-stability.md`; `extending.md` + user-guide
      (en + zh-tw) updated; `CHANGELOG.md` + `version.go` bumped to 1.1.0.
- [ ] **Manual (needs a TTY — flag for a human):** add a real PreToolUse
      hook to `~/.evva/settings.json`, run evva, confirm it fires and can
      block / mutate a tool call; confirm a no-hook session is unaffected.
