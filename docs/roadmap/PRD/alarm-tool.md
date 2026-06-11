# PRD — Alarm Tool — Implementation Plan

> **Audience:** senior engineers implementing this phase.
> **Status:** ✅ IMPLEMENTED on `feature/alarm` (see "As-built" below). All tests + `go vet` green.
> **Target release:** `[Unreleased]` (next beta).
> **Roadmap source:** `CLAUDE.md` → "one runtime, many personas" tool surface.
> **Reference source:** no `ref/src` analogue — this is an evva-native capability built on the existing wakeup + signal-wake primitives.

---

## 0. As-built (what shipped vs. this plan)

Two requirements landed after the original plan below and changed the shape; the
technical analysis in §1–§2 (why `schedule_wakeup`/`cron` can't do it, and the
`WakeupQueue` + signal-wake mechanism) is still accurate and load-bearing.

1. **Deferred, not active.** The solo `evva` profile carries `alarm_*` in its
   **DeferredTools** list (loaded on demand via `tool_search`), not ActiveTools.
   It is taught in the main system prompt (`sysprompt.mainToolsGuideSection`, an
   `## Alarms` block contrasting it with `schedule_wakeup`/cron) and ranked in
   `pkg/toolset` tags+hints. Durable alarms are still re-armed at boot regardless
   of whether the model loads the tool (correctness) — gated by
   `profileAllowsAlarm`.

2. **Swarm support (two thin tool layers over one shared `Scheduler`).** Because
   `WithCustomTool` shares the global registry and a swarm member's runs are
   driven by the supervisor (inbox-mail + poke), the swarm needs its own tools
   and delivery path — same-naming would collide and the WakeupQueue path is
   wrong for a member. As built:
   - **Solo (`evva`):** `alarm_create`/`alarm_list`/`alarm_cancel`
     (`pkg/tools/alarm`, ToolState-wired). Fire → own `WakeupQueue` + `SignalAlarm`.
   - **Swarm (all members):** `alarm_set` + `alarm_clear`
     (`internal/swarm/tools/alarm.go`, MemberContext-wired). Fire → durable
     `bus.Send` to the target member, waking its run loop like any teammate
     message. Self-alarm for every member; **leader-only** to target another.
     Pending alarms fold into `list_members`. The space owns one shared
     `alarm.Scheduler` (persisted at `<workdir>/alarms.json`, re-armed in
     `Supervisor.Start`). Distinct from `schedule_set` (recurring cron,
     leader-only, no self).

   The shared `pkg/tools/alarm.Scheduler` gained `Target`/`Origin` on `Alarm`
   (multi-agent routing; empty in solo use) and a `Rearm()`/`LoadAndRearm()`
   split so the solo TUI defers past-due delivery to the next run (no autonomous
   boot run) while the swarm fires past-due immediately as durable mail.

**As-built file list:** `pkg/tools/alarm/{scheduler,alarm}.go` (+ tests),
`pkg/tools/name.go`, `internal/agent/{signal,agent,profiles}.go`,
`internal/toolset/{toolset,builtins}.go`, `pkg/toolset/tags.go`,
`internal/agent/sysprompt/{toolnames,main_agent}.go`,
`internal/swarm/{space,supervisor,teamprompt}.go`,
`internal/swarm/tools/{alarm,set,messaging}.go` (+ tests), `CHANGELOG.md`.

---

## 1. TL;DR — what this phase actually is

We want the agent to set an **alarm at an absolute wall-clock instant** —
e.g. `2026-09-11 12:31:50` — and be **proactively woken** at that instant,
even if the agent has been idle for months and the process has restarted in
between.

The existing `schedule_wakeup` tool **cannot** do this, and neither can the
(stubbed) `cron_*` family. The gap is real, but the hard machinery already
exists — we only need a new tool surface plus a small non-blocking,
persistent, absolute-time scheduler. The *delivery* and *idle-wake* paths are
reused verbatim.

**Why `schedule_wakeup` (`internal/tools/meta/wakeup.go`) is the wrong tool:**

| Property | `schedule_wakeup` | What an alarm needs |
|---|---|---|
| Time spec | **relative** `delaySeconds` (`wakeup.go:97-101`) | **absolute** timestamp |
| Max horizon | **3600s = 1 hour** (`wakeup.go:107-110`) | months / years |
| Mechanism | **blocks** `Execute` in a `time.Timer` (`wakeup.go:137-150`) | fire-and-forget, no goroutine held |
| Survives restart | no (in-memory, dies on interrupt — `:140-148`) | yes (durable) |
| Sub-minute precision | n/a (it's a delay) | yes — fire at `:50` seconds |

`schedule_wakeup` works *only because it blocks*: the agent run stays in-flight
during the sleep, so `drainWakeupPrompts` picks up the queued prompt on the
next iteration. That design fundamentally can't span a long, idle, possibly
cross-restart wait.

**Why the `cron_*` family is also not it:**

- `cron_create` / `cron_list` / `cron_delete` are **stubs** — registered as
  "stateless stubs" (`internal/toolset/builtins.go:154-158`) whose `Execute`
  returns `"tool … is not implemented yet"` (`pkg/tools/stub.go:35`).
- Even fully implemented, **standard 5-field cron has no year field and no
  seconds field** (`pkg/tools/cron/cron.go:30`). `2026-09-11 12:31:50` is
  inexpressible: the year can't be pinned (it would recur every Sept 11) and
  `:50` seconds is below cron's minute floor.

`alarm_*` and `cron_*` are **siblings, not rivals**: alarm = "fire **once** at
this **exact instant**"; cron = "fire on this **recurring wall-clock
pattern**". When cron is eventually implemented it can share the same scheduler
(see §5.6).

**The elegant core:** an alarm's fire action is just
`WakeupQueue.Enqueue(prompt)` + `SendSignal(...)`. Both already exist and are
load-bearing in production for daemons and async subagents. We add **zero** new
drain code and **zero** new idle-wake code — only a scheduler that calls them
at the right instant.

---

## 2. Inventory — what already exists (do not re-build)

### 2.1 Delivery channel — `WakeupQueue` + `drainWakeupPrompts`

- `internal/tools/meta/wakeup.go:22-54` — `WakeupQueue` is a mutex-guarded
  `[]string`. `Enqueue(prompt)` appends; `Drain()` returns + clears.
  **In-memory only** (not durable — that's our job to add for alarms).
- `internal/agent/state_machine.go:72-84` — `drainWakeupPrompts()` runs **at
  the top of every loop iteration**, appending each queued prompt as a fresh
  `llm.Message{Role: RoleUser}`. The model sees it exactly as if the user just
  typed it.
- Gated on `a.toolState.HasWakeupQueue()` (`state_machine.go:73`) — so the
  queue must be **allocated** for the drain to run. It is lazily allocated by
  `ToolState.WakeupQueue()` (`internal/toolset/toolset.go:249`); today only the
  `schedule_wakeup` tool's construction triggers it
  (`builtins.go:104-107`). **The alarm tool's construction must also obtain
  the queue** so the drain is live even on a profile that has `alarm` but not
  `schedule_wakeup`.

**Decision:** reuse `WakeupQueue` for alarm delivery. It is semantically
identical (inject a prompt as a fresh user message) and means we touch no drain
code. The scheduler enqueues a *formatted* prompt so a fired alarm is
distinguishable in the transcript (see §4 Task 3).

### 2.2 Idle-wake bridge — `signal.go` (the half `schedule_wakeup` skips)

- `internal/agent/signal.go:46-55` — `SendSignal(AgentSignal)` is
  **goroutine-safe and non-blocking** (buffered chan cap 64; drops + logs if
  full because the store/queue is the durable backstop). **Safe to call from a
  background timer goroutine.**
- `internal/agent/signal.go:64-78` — `signalPump` goroutine, started in
  `agent.New` (`agent.go:461`), lives for `rootCtx`.
- `internal/agent/signal.go:93-109` — `handleSignal`: if the loop is **idle**,
  CAS-acquire `a.running` and spawn a fresh `runFromSignal → runLoop`; if
  **busy**, do nothing — the live loop's next-iteration drain picks the prompt
  up. **Subagents never idle-wake; only the root agent does** (`:96-98`).
- `internal/agent/signal.go:14-22` — `SignalKind` is deliberately kept as an
  open enum precisely so "future signal kinds — e.g. a UI nudge, a remote
  control event — can plug in without touching the loop." **An alarm is exactly
  such a new kind.**

This is the missing half: when an alarm fires while the agent is idle (the
common case for a long-horizon alarm), `SendSignal` starts a fresh run; the
drain folds the prompt in. No polling, no blocked goroutine.

### 2.3 The injection seam — `SignalSender` on `ToolState`

- `internal/toolset/toolset.go:316-325` — `SignalSender{NotifyDaemon,
  RootCtx, AgentID}` is the narrow callback bundle the agent installs so
  background tools can wake the loop **without importing `internal/agent`**
  (which would be an import cycle).
- `internal/agent/agent.go:452-456` — installed in `agent.New`:
  `NotifyDaemon: func() { a.SendSignal(AgentSignal{Kind: SignalDaemon}) }`.
- `internal/toolset/toolset.go:367-377` — `DaemonState`'s notify closure
  indirects through `s.signalSender` **at call time** (not snapshotted at
  construction) to tolerate init ordering. The alarm scheduler follows the
  exact same pattern.

We add one field — `NotifyAlarm func()` — to `SignalSender` and wire it to
`a.SendSignal(AgentSignal{Kind: SignalAlarm})`.

### 2.4 Prior art — the swarm supervisor scheduler

- `internal/swarm/supervisor.go:20-27` — design doctrine: three wake sources
  (message / timer / resume) collapsed into one run loop; **"Idle burns no
  tokens: with no wake there is no Run."**
- `internal/swarm/supervisor.go:52-54` — `defaultTickInterval = time.Second`
  timer resolution; `SetSchedule` / `nextDue` / `poke` model.
- `internal/swarm/agentdef/schedule.go` — a hand-written, dependency-free
  5-field cron engine with `Next(after)`. **Not reused for alarm** (alarm is an
  absolute instant, not a cron pattern — no parsing needed, just `time.Parse` +
  compare), but it confirms the house preference for **no external cron/clock
  dependency**.

The alarm scheduler is a **simpler, single-owner** version of the supervisor's
timer: one process, one root agent, a set of pending absolute instants.

### 2.5 Tool plumbing (shared with every tool)

- `pkg/tools/name.go:38-40` — `SCHEDULE_WAKEUP` constant; `:106-109` — the
  `CRON_*` / `REMOTE_TRIGGER` constants. New `ALARM_*` constants slot beside
  them.
- `pkg/tools/tool.go:18` — the `Tool` interface (`Name/Description/Schema/Execute`).
- `internal/toolset/builtins.go` — central registration; `pkg/toolset/tags.go`
  — tag/profile grouping; `internal/agent/profiles.go` — which profiles get
  which tools.
- Durable-store precedent for the on-disk file path: the cron stub's schema
  references `.evva/scheduled_tasks.json` (`pkg/tools/cron/cron.go:33`).

---

## 3. Goal & acceptance criteria

**Goal.** Give the agent a self-wakeup **alarm clock**: set a one-shot trigger
at an absolute timestamp (second precision), survive restarts, and on fire
re-enter the conversation with a supplied prompt — starting a fresh run if the
agent is idle.

**Acceptance criteria:**

1. **A1 — Absolute time, second precision.** `alarm_create` accepts a wall-clock
   timestamp such as `2026-09-11 12:31:50` (local tz) and `2026-09-11T12:31:50+08:00`
   (RFC3339). The alarm fires within ≤1s of that instant.
2. **A2 — Non-blocking.** `alarm_create.Execute` returns immediately with an
   alarm id; it does **not** sleep. The agent can keep working (or go idle)
   after arming.
3. **A3 — Idle-wake.** When the alarm fires while the agent is idle, a fresh run
   starts automatically and the model sees the alarm prompt as a new user
   message. When it fires mid-run, the prompt lands at the next iteration
   boundary (same as `schedule_wakeup` / daemons).
4. **A4 — Durable across restart.** A durable alarm set before a restart is
   re-armed on startup. An alarm whose instant passed *while the process was
   down* fires **once** on next startup, flagged late (§5.4).
5. **A5 — Manageable.** `alarm_list` shows pending alarms (id, fire time,
   prompt, durable flag, time-until). `alarm_cancel` removes one by id.
6. **A6 — Past-time rejected.** Arming a time in the past returns an error
   (does not silently fire or hang).
7. **A7 — Scoped to root agent.** Only the root agent arms/fires alarms;
   subagents do not (they have no idle-wake — `signal.go:96`). Building the tool
   for a subagent profile is either omitted or a no-op that errors clearly.
8. **A8 — Bounded.** A per-agent cap on simultaneous pending alarms (e.g. 100)
   prevents runaway accumulation; exceeding it errors.
9. **A9 — Tests green** (`go test ./...`) and `go vet ./...` clean.

---

## 4. Work breakdown (ordered)

### Task 0 — Tool name constants

`pkg/tools/name.go` — add beside the `CRON_*` block:

```go
ALARM_CREATE ToolName = "alarm_create"
ALARM_LIST   ToolName = "alarm_list"
ALARM_CANCEL ToolName = "alarm_cancel"
```

### Task 1 — The scheduler (`pkg/tools/alarm/scheduler.go`)

A dependency-free, goroutine-safe, persistent absolute-time scheduler. Public
(`pkg/`) because it's reusable and SDK-embeddable (mirrors `pkg/tools/daemon`).

Core type:

```go
type Alarm struct {
    ID      string    `json:"id"`
    FireAt  time.Time `json:"fire_at"`  // stored UTC, displayed local
    Prompt  string    `json:"prompt"`
    Label   string    `json:"label,omitempty"`
    Durable bool      `json:"durable"`
    Created time.Time `json:"created"`
}

type Scheduler struct {
    mu      sync.Mutex
    alarms  map[string]*armed   // id -> {Alarm, *time.Timer}
    onFire  func(a Alarm)       // injected: enqueue + signal
    path    string              // durable store file ("" = session-only)
    // ...
}

func New(onFire func(Alarm)) *Scheduler
func (s *Scheduler) Arm(a Alarm) (string, error)   // validates future-time, caps count, starts timer, persists
func (s *Scheduler) Cancel(id string) bool
func (s *Scheduler) List() []Alarm                 // sorted by FireAt
func (s *Scheduler) LoadAndRearm() error           // startup recovery (A4)
func (s *Scheduler) Stop()                          // stop all timers (Shutdown)
```

**Timer mechanism:** one `time.Timer` (via `time.AfterFunc`) per pending alarm,
keyed by id. `AfterFunc` gives sub-second precision (A1) and costs nothing while
idle. On fire: remove from map, persist, call `onFire(a)`. (A min-heap + single
re-armed timer is an alternative if we worry about thousands of alarms; given
the A8 cap of ~100, per-alarm `AfterFunc` is simpler and fine. — §5.5.)

**Persistence:** when `path != ""`, every `Arm`/`Cancel`/fire rewrites the JSON
file atomically (write-temp-then-rename). Default path `<EVVA_HOME>/alarms.json`
(consistent with the cron stub's `.evva/scheduled_tasks.json` convention; pick
one and document it).

Unit tests (`scheduler_test.go`): arm-and-fire (short delay), cancel-before-fire,
past-time rejected, count cap, load-and-rearm with a future alarm, load with a
past-due alarm → fires once immediately.

### Task 2 — The tools (`pkg/tools/alarm/alarm.go`)

Three `Tool` implementations bound to one `*Scheduler` (constructor injection,
mirroring `meta.NewWakeup(queue)`):

- **`alarm_create`** — schema:
  ```json
  {
    "type":"object","additionalProperties":false,
    "required":["at","prompt"],
    "properties":{
      "at":{"type":"string","description":"Absolute fire time. Accepts \"2006-01-02 15:04:05\" (local tz) or RFC3339 \"2006-01-02T15:04:05Z07:00\". Must be in the future."},
      "prompt":{"type":"string","description":"The prompt injected as a fresh user message when the alarm fires."},
      "label":{"type":"string","description":"Optional short label shown in alarm_list and the fire banner."},
      "durable":{"type":"boolean","description":"true (default) = survive restarts via on-disk store. false = session-only, dies when this process exits."}
    }
  }
  ```
  Parse `at` (try `2006-01-02 15:04:05` in `time.Local`, then RFC3339), reject
  past times, `Arm`, return `"alarm <id> set for <fire-at local> (in <duration>)"`.
  **Durable defaults to true** — an alarm's whole purpose is a long horizon
  (contrast `schedule_wakeup` for the ephemeral short delay). Document this.

- **`alarm_list`** — no input; returns id, local fire time, time-until, label,
  prompt (truncated), durable flag for each pending alarm.

- **`alarm_cancel`** — `{ "id": string }`; cancels, returns ok/not-found.

Tool descriptions must steer the model: alarm = "fire **once** at an absolute
instant, far in the future is fine, second precision"; explicitly contrast
`schedule_wakeup` ("short relative delay, blocks, ≤1h") so the model picks the
right one.

### Task 3 — Wire the fire action (enqueue + signal)

The agent owns the scheduler instance and supplies `onFire`. New signal kind in
`internal/agent/signal.go`:

```go
const SignalAlarm SignalKind = "alarm"
```

In `agent.New` (`agent.go:452`), extend the installed `SignalSender` with
`NotifyAlarm: func() { a.SendSignal(AgentSignal{Kind: SignalAlarm}) }`, and add
the field to `toolset.SignalSender` (`toolset.go:316`).

`onFire(a Alarm)` does exactly two things (no new drain code):

```go
func(a alarm.Alarm) {
    banner := fmt.Sprintf("⏰ Alarm fired", /* label, set-time */)
    toolState.WakeupQueue().Enqueue(banner + "\n" + a.Prompt)  // §2.1
    toolState.NotifyAlarm()                                     // §2.2/2.3 -> SendSignal
}
```

`handleSignal` already wakes idle / no-ops busy (`signal.go:93-109`);
`drainWakeupPrompts` already folds the prompt in (`state_machine.go:72`).
Optionally give `SignalAlarm` a wire event in `emitSignalEvent`
(`signal.go:115-120`) so the TUI can paint an "⏰ alarm fired" line; daemon is
wake-only there, alarm can carry a small payload if we want the banner in the UI
strip.

### Task 4 — ToolState accessor + scheduler lifecycle

- `internal/toolset/toolset.go` — add an `alarmScheduler *alarm.Scheduler` field
  + `AlarmScheduler()` lazy accessor (mirrors `WakeupQueue()` /
  `DaemonState()`), and `NotifyAlarm()` indirecting through `signalSender` at
  call time (mirror `DaemonState`'s closure, `:367-377`). The lazy accessor must
  ensure `WakeupQueue()` is also allocated (so the drain is live — §2.1).
- `internal/agent` — on startup, after `signalPump` is up, call
  `scheduler.LoadAndRearm()` (A4). On `Shutdown`, call `scheduler.Stop()`.
- `internal/toolset/builtins.go` — register the three tools beside the cron
  block (`:154-158`), each resolving `s.(*ToolState).AlarmScheduler()`.

### Task 5 — Profiles, tags, docs, version

- `internal/agent/profiles.go` + `pkg/toolset/tags.go` — add `alarm_*` to the
  root/evva profile (and any other `[main]` persona that should have it). **Do
  not** add to subagent-only profiles (A7).
- `docs/user-guide/` (en + zh-tw) — short "Alarms" section: format, durability,
  difference from `schedule_wakeup`.
- `CHANGELOG.md` `[Unreleased]` → `### Added`: alarm tool family.
- `pkg/version/version.go` — bump at release time per the release workflow (not
  in the feature PR).

---

## 5. Design decisions & risks

### 5.1 — Reuse `WakeupQueue` for delivery, don't invent an AlarmQueue
The queue is a generic "inject a prompt as a fresh user message" channel, and
its drain already runs every iteration (`state_machine.go:72`). A separate queue
would duplicate the drain and the `HasXQueue` gate for no semantic gain. We
distinguish a fired alarm by **formatting the prompt** (⏰ banner + label), not
by a separate pipe.

### 5.2 — New `SignalAlarm` kind, not reuse of `SignalDaemon`
`SignalKind` is explicitly open for this (`signal.go:11-22`). A distinct kind
costs one constant + one `SignalSender` field, and buys clean telemetry and an
optional TUI affordance (`emitSignalEvent`). Reusing `SignalDaemon` would
conflate alarm fires with daemon lifecycle in logs/UI.

### 5.3 — `time.AfterFunc` per alarm, not a blocking sleep
This is the crux distinction from `schedule_wakeup`. `AfterFunc` registers a
callback with the runtime timer heap and returns immediately — no goroutine
parked, no run held open, survivable horizon is unbounded. The fire callback
runs on its own goroutine and only does the (non-blocking) enqueue+signal.

### 5.4 — Restart recovery & past-due semantics
Durable alarms are re-armed on startup (A4). For an alarm whose `FireAt` already
passed while the process was down, **fire once immediately on startup**, with
the banner flagged `(late, was due <original time>)`. Rationale: silently
dropping a missed alarm is the worse failure for the user's mental model ("I set
it and it never happened"). A config knob (`alarm.fire_missed: true|false`) can
make this tunable later; default fire-once. Document clearly.

### 5.5 — `AfterFunc` map vs. min-heap + single timer
With the A8 cap (~100 pending), one `time.Timer` per alarm is simplest and has
negligible cost. If a future use-case needs thousands, switch the internal
representation to a min-heap + a single re-armed timer **without changing the
tool surface** (the `Scheduler` API stays identical). Noted so the simple
version isn't mistaken for a dead end.

### 5.6 — `alarm_*` is a sibling of `cron_*`, and seeds cron's eventual impl
Alarm = one-shot absolute instant (second precision). Cron = recurring
wall-clock pattern (minute precision). They do not overlap. When `cron_*` is
implemented, it can reuse this `Scheduler` (add a recurring entry whose fire
callback re-arms `Next()` from `internal/swarm/agentdef/schedule.go`'s cron
engine). Building alarm first is the right order: it's the simpler, fully
absolute case and exercises the whole enqueue+signal+persist path.

### 5.7 — `pkg/tools/alarm` (public), not `internal/`
The scheduler + tools are generally useful and SDK-embeddable (a downstream
host embedding evva may want programmatic alarms), matching the
`pkg/tools/daemon` + `pkg/tools/cron` placement. The agent-specific wiring
(`SignalSender.NotifyAlarm`, lifecycle) stays in `internal/agent` /
`internal/toolset`, preserving the no-cycle boundary.

### 5.8 — Timezone
Bare `2006-01-02 15:04:05` is interpreted in **`time.Local`** (the user's tz —
matching cron's "user's local timezone" note, `cron.go:22`). RFC3339 with an
explicit offset is honored as-is. Stored internally as UTC `time.Time`,
displayed local in `alarm_list` and the fire banner. Edge: DST transitions — an
absolute instant is unambiguous once parsed, so DST only matters at parse time
for the bare-local form; document that bare local times around a DST boundary
follow Go's `time.ParseInLocation` semantics.

### 5.9 — Risks
- **Token spend on autonomous wake.** A fired alarm starts a real run = real
  tokens, possibly while the user is away. Mitigations: it's the model arming
  its own alarm (intentional), `alarm_list`/`alarm_cancel` give visibility, A8
  caps count, and the TUI banner makes a wake visible. A future config could
  gate firing behind confirmation; out of scope for v1.
- **Clock changes / sleep/suspend.** `time.AfterFunc` is monotonic-clock based;
  a laptop sleeping past the fire time fires on wake (acceptable). A wall-clock
  jump (NTP correction, manual change) won't retarget an already-armed
  `AfterFunc` — acceptable for v1; the durable store holds the wall-clock
  `FireAt` so a restart re-targets correctly.
- **Idle-wake while an overlay is open** (approval/question/compaction). The
  signal sets `running` and the loop drains when it next can — same behavior
  daemons already have in production; no new handling needed.

---

## 6. Out of scope

- Recurring alarms / cron implementation (separate ticket; §5.6 keeps the door
  open).
- Per-alarm permission gating / confirm-before-fire (§5.9 risk; future config).
- Subagent-owned alarms (A7: root only).
- Cross-session/remote alarms (e.g. a swarm member waking another) — the swarm
  has its own supervisor scheduler.
- Snooze / reschedule of a fired alarm (model can just `alarm_create` again).
- Natural-language time parsing ("tomorrow at noon") — the tool takes an
  explicit timestamp; the model does the NL→timestamp conversion itself.

---

## 7. Verification checklist (PR gate)

- [ ] `alarm_create` with `2026-09-11 12:31:50` arms and `alarm_list` shows it
      with correct local time + time-until. (A1, A5)
- [ ] `alarm_create.Execute` returns immediately (no sleep); manual test arms a
      +3s alarm and confirms the agent prompt is `>` again instantly. (A2)
- [ ] +3s alarm fires while idle → a fresh run starts and the model receives the
      ⏰ prompt as a user message. (A3)
- [ ] +3s alarm fires mid-run → prompt lands at next iteration. (A3)
- [ ] Durable alarm + restart → re-armed and fires. (A4)
- [ ] Durable alarm with past `FireAt` after a simulated downtime → fires once,
      flagged late. (A4, §5.4)
- [ ] `alarm_cancel` removes a pending alarm; firing does not occur. (A5)
- [ ] Past timestamp → clear error, nothing armed. (A6)
- [ ] >100 alarms → cap error. (A8)
- [ ] Subagent profile does not expose `alarm_*` (or errors clearly). (A7)
- [ ] `go test ./...` green; `go vet ./...` clean. (A9)
- [ ] Tool descriptions steer alarm-vs-`schedule_wakeup` correctly (manual read).

---

## 8. File-by-file change list (cheat sheet)

| File | Change |
|---|---|
| `pkg/tools/name.go` | + `ALARM_CREATE/LIST/CANCEL` constants (§4 T0) |
| `pkg/tools/alarm/scheduler.go` | **new** — `Alarm`, `Scheduler`, Arm/Cancel/List/LoadAndRearm/Stop (§4 T1) |
| `pkg/tools/alarm/scheduler_test.go` | **new** — fire/cancel/past/cap/recovery tests |
| `pkg/tools/alarm/alarm.go` | **new** — 3 `Tool` impls bound to `*Scheduler` (§4 T2) |
| `pkg/tools/alarm/alarm_test.go` | **new** — schema/parse/validation tests |
| `internal/agent/signal.go` | + `SignalAlarm` kind; optional `emitSignalEvent` case (§4 T3) |
| `internal/agent/agent.go` | + `NotifyAlarm` in installed `SignalSender` (`:452`); `LoadAndRearm` on boot; `Stop` on shutdown (§4 T3/T4) |
| `internal/toolset/toolset.go` | + `alarmScheduler` field, `AlarmScheduler()` accessor (ensures `WakeupQueue()` allocated), `NotifyAlarm()`; + `NotifyAlarm` field on `SignalSender` (§4 T4) |
| `internal/toolset/builtins.go` | + register 3 alarm tools beside cron (`:154-158`) (§4 T4) |
| `internal/agent/profiles.go`, `pkg/toolset/tags.go` | + `alarm_*` to root/main personas (§4 T5) |
| `docs/user-guide/{en,zh-tw}/…` | + Alarms section (§4 T5) |
| `CHANGELOG.md` | + `### Added` entry (§4 T5) |

---

## 9. Effort estimate (informational)

Small-to-medium. The scheduler + tools (~300–400 LoC incl. tests) is the bulk;
the agent wiring is ~20 LoC across three files because **delivery and idle-wake
are entirely reused**. No new external dependency (stdlib `time` only). Highest-
risk area is restart recovery + past-due semantics (§5.4) — covered by the
recovery tests in Task 1.
