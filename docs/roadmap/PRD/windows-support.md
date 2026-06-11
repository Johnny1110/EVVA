# PRD — Windows Support — Implementation Plan

> **Audience:** senior engineers implementing this wave.
> **Status:** implemented — WIN-1..WIN-8 on `feature/windows-support`
> (PR #35). WIN-7 triage took 2 rounds; `go test ./...` is green on
> `windows-latest` and the job is required as of 2026-06-12. The triage
> surfaced two real product bugs beyond §3's audit: the per-agent log
> file was never closed (internal/logger — blocked file deletion on
> Windows), and the grep tool's unquoted search root broke on
> backslashes and spaces. Pending: WIN-9 real-hardware validation
> against the v1.7.0-beta.1 assets.
> **Target release:** `v1.7.0` (this wave claims the v1.7 minor per the
> CLAUDE.md wave → minor rule; first Windows binaries ship in
> `v1.7.0-beta.1`).
> **Roadmap source:** operator request 2026-06-11 — "support Windows users
> downloading and using evva".
> **Evaluation provenance:** live-source audit at `dev@f0c07bc` on
> 2026-06-11, including an actual `GOOS=windows GOARCH=amd64 go build ./...`
> run (output reproduced in §3.1). All file:line references verified against
> that commit.

---

## 1. TL;DR

evva today is macOS/Linux only (README.md:20 says so explicitly). The gap to
Windows is **narrower than expected**: the dependency graph is already fully
portable (pure-Go sqlite, Bubble Tea's Windows console stack is in the module
graph), the fs tools already normalize CRLF, glob/permission matching is
already separator-aware, and `pkg/update` already *names and extracts*
`evva-windows-<arch>.zip` / `evva.exe` assets that nothing currently builds.

What actually blocks Windows is one repeated idiom and a handful of seams:

1. **Compile blockers** — the Unix process-tree idiom
   (`SysProcAttr{Setpgid}` + `syscall.Kill(-pid)`) appears in 4 packages
   (`pkg/tools/{shell,monitor,repl,lsp}`) and `cmd/evva` (`Setsid`). These
   are hard `GOOS=windows` compile errors.
2. **The shell question** — the bash tool execs `/bin/sh -c` (so do the
   monitor daemon and the hooks runner). Decision in §4: **require Git
   Bash on Windows**, same call Claude Code made. No PowerShell dialect.
3. **Self-update** — `replaceBinary` promises Windows handling in its
   comment but only implements the Unix rename (self_replace.go:120).
4. **CI/release** — no windows entries in either workflow; no `.exe`; no
   zip packaging.

Plan: one new `pkg/common/proc` package isolates every per-OS process
concern behind 4 functions; a shell resolver picks `/bin/sh` vs Git Bash;
CI gets a cheap cross-compile gate immediately and a `windows-latest` test
job during bring-up; release.yml grows `windows/amd64` (+`arm64`) zip
assets that match the names `pkg/update` already expects.

---

## 2. Goals / non-goals

### Goals

- `GOOS=windows go build ./...` green, enforced by CI on every PR.
- A Windows user can: download `evva-windows-amd64.zip` from GitHub
  Releases (or `go install`), run `evva.exe` in Windows Terminal, and get
  the full TUI + agent loop with working `bash`, `read/write/edit`, `glob`,
  `grep`, `monitor`, `repl`, `lsp`, `web_*`, MCP, skills, and hooks.
- `evva update` works on Windows (download → swap → relaunch picks up the
  new binary).
- `evva service start/stop/status` works on Windows (console-process model;
  no SCM integration).
- `go test ./...` runs on `windows-latest` in CI; tests that are
  legitimately Unix-only carry explicit `runtime.GOOS` skips.

### Non-goals (v1.7)

- **PowerShell/cmd dialect for the bash tool.** The tool stays bash (§4).
- **WSL integration.** Native binary only; WSL users already have the
  Linux build.
- **Package managers** (winget, scoop, chocolatey, MSI). Zip + `go install`
  first; managers can follow once the asset exists.
- **Windows service (SCM) / autostart unit.** `unitFor` already degrades
  gracefully on Windows (unit.go:74) pointing at the manual doc; an SCM or
  Task Scheduler template is a later RP-18 follow-up.
- **windows/386.** amd64 required, arm64 best-effort (§9 Q1).
- **Legacy conhost polish.** Windows Terminal is the supported surface;
  minimum OS is Windows 10 1903+ (VT processing available for plain
  conhost, but rendering fidelity is only validated on Windows Terminal).

---

## 3. Verified current state

### 3.1 Compile blockers (from the actual cross-build)

`GOOS=windows GOARCH=amd64 go build ./...` at `dev@f0c07bc`:

```
pkg/tools/monitor/monitor_daemon.go:178: unknown field Setpgid in struct literal of type syscall.SysProcAttr
pkg/tools/monitor/monitor_daemon.go:183: undefined: syscall.Kill
pkg/tools/repl/repl.go:128:             unknown field Setpgid …
pkg/tools/repl/repl.go:135:             undefined: syscall.Kill
pkg/tools/shell/bash.go:170:            unknown field Setpgid …
pkg/tools/shell/bash.go:180:            undefined: syscall.Kill
pkg/tools/shell/bash_daemon.go:156:     unknown field Setpgid …
pkg/tools/shell/bash_daemon.go:164:     undefined: syscall.Kill
pkg/tools/lsp/client.go:53:             unknown field Setpgid …
```

All nine errors are one idiom: *put the child in its own process group so
timeout/cancel can kill the whole tree* (`Setpgid: true` +
`syscall.Kill(-pid, SIGKILL)` in `cmd.Cancel`). Windows' `SysProcAttr` has
neither field nor function — these packages need a per-OS seam, not a
rewrite.

**Not yet surfaced by the compiler** (its package deps failed first), but
the same class of error:

- cmd/evva/service.go:206 — `SysProcAttr{Setsid: true}` (daemon detach).

**Compiles on Windows but is wrong at runtime:**

- cmd/evva/servicectl.go:100 — liveness probe `proc.Signal(syscall.Signal(0))`
  always errors on Windows → a live daemon reads as dead. (On Windows,
  `os.FindProcess` itself does the existence check.)
- cmd/evva/service.go:283 — graceful stop via `proc.Signal(syscall.SIGTERM)`
  is unsupported on Windows (`Process.Signal` only implements Kill).
- cmd/evva/main.go:110 — `signal.NotifyContext(…, os.Interrupt,
  syscall.SIGTERM)` is fine: the constant exists on Windows, Ctrl+C arrives
  as `os.Interrupt`, SIGTERM is simply never delivered. No change needed.

### 3.2 Runtime-only gaps (compile fine, fail or misbehave on Windows)

| Site | Problem |
|---|---|
| pkg/tools/shell/bash.go:158, bash_daemon.go:152, monitor_daemon.go:177 | hardcoded `exec.CommandContext(…, "/bin/sh", "-c", cmd)` — no `/bin/sh` on Windows |
| pkg/hooks/runner.go:55 | hooks runner hardcodes `shell := "/bin/sh"` — all user hooks dead on Windows |
| pkg/update/self_replace.go:120–135 | `replaceBinary` comment promises a Windows path ("write a helper script…") that is **not implemented**; `os.Rename` over a running `.exe` fails with access-denied |
| pkg/tools/repl/repl.go:169–172 | interpreter resolution tries `python3` → `python`; Windows convention is `python` → `py` (launcher); `python3` rarely exists |
| pkg/tools/lsp/manager.go:277 | `"file://" + filepath.ToSlash(abs)` yields `file://C:/…` — drive-letter URIs need three slashes (`file:///C:/…`); diagnostics.go:219 strips the prefix and would hand back `/C:/…` |
| pkg/tools/fs/fs.go:118 | `resolveUserHome` checks `SUDO_USER` → `HOME` → `user.Current()`. Works on Windows only via the last fallback (`HOME` is normally unset); harmless but should consult `os.UserHomeDir` explicitly |

### 3.3 Already Windows-ready (do not redo)

Verified portable — listed so nobody re-audits or "fixes" these:

- **Self-update asset layer**: `assetNameFor` already returns
  `evva-windows-<arch>.zip` (update.go:100–106); `extractZip` /
  `extractTarGz` already look for an `evva.exe` entry
  (self_replace.go:70,108). Only `replaceBinary` lacks the Windows swap.
- **Config home**: load.go:143 already branches `runtime.GOOS == "windows"`
  for `%USERPROFILE%\.evva`.
- **System prompt**: env section already reports `runtime.GOOS`
  (sysprompt.go:61,92) and `detectShell` already documents the
  Windows-unset-`SHELL` fallback (sysprompt.go:100).
- **CRLF**: the fs read/edit path already normalizes `\r\n` → `\n` on read
  and re-emits the file's original line endings on write
  (encoding.go:70–72, edit.go:503,624–626). Edit-tool exact-match will not
  break on CRLF files.
- **Separators**: glob is `doublestar` over an `fs.FS` with explicit
  `filepath.Separator != '/'` handling (glob.go:254); permission path
  matching splits on `filepath.Separator` (decision.go:319); `~` expansion
  accepts both separators (fs.go:85).
- **grep / tree**: pure Go, no external `rg`/`grep`/`tree` binaries
  (no `exec.Command` in either file).
- **Dependency graph**: `modernc.org/sqlite` (pure Go, Windows-supported),
  Bubble Tea v1.3.10 with `erikgeiser/coninput` (Windows console input)
  already in go.mod; `golang.org/x/sys` v0.42.0 already in the module
  graph (indirect). No cgo anywhere — cross-compiling from the Linux
  runner works (the §3.1 build proves it).
- **TUI**: no `tea.Suspend`/SIGTSTP usage anywhere in pkg/ui — no
  Unix-only TUI feature to port.
- **Autostart**: `unitFor` returns a clear "no autostart unit template for
  windows" error (unit.go:74) — graceful, not a crash.
- **Worktree/MCP spawns**: `git` (worktree.go:406) and MCP stdio servers
  (mcp/transport.go:21) spawn via plain `exec.Command` — `exec.LookPath`
  resolves `.exe`/`.cmd` through `PATHEXT` on Windows. (Quoting caveat for
  `.cmd` shims in §8.)

### 3.4 CI / distribution state

- `.github/workflows/ci.yml` — one `ubuntu-latest` Go job (build, vet,
  swarm tests, `bash scripts/depcheck.sh`) + one web job. **No Windows
  coverage of any kind**, not even a cross-compile.
- `.github/workflows/release.yml` — matrix is darwin/linux × amd64/arm64
  (release.yml:16–24); output name hardcoded `evva` (line 42); packaging
  hardcoded `tar -czf` (line 46). No `.exe`, no zip → **`evva update` on
  Windows would fail today with "no asset found"** even after the binary
  exists, until the matrix grows the entries `assetNameFor` expects.
- README.md:20 — "Currently **macOS and Linux only**".

---

## 4. The shell decision: bash tool on Windows

The user-facing question — *can the bash tool work on Windows?* — gets a
firm **yes, via Git Bash**, with this resolution policy:

**Decision: the `bash` tool requires a bash on Windows; we find Git Bash
and refuse to silently substitute PowerShell/cmd.**

Rationale:

1. **Model behavior.** The whole harness — tool name, tool description,
   the model's training distribution — assumes POSIX sh. A PowerShell
   backend would make the model emit `ls | head`, `&&` chains, `$VAR`,
   heredocs… into a shell with different semantics. Git Bash makes the
   model's existing habits *work* instead of teaching it a dialect.
2. **Precedent.** Claude Code's native Windows port requires Git for
   Windows' bash for exactly this reason. evva borrows the harness shape
   (CLAUDE.md vision); this is part of the shape.
3. **Cost.** Git is already a de-facto prerequisite (worktree tools spawn
   `git`; every evva user is in a git repo). Git for Windows ships
   `bash.exe`. The marginal install burden is ~zero for the target user.

Resolution order (new `shellPath()` used by bash tool, bash daemon,
monitor daemon, hooks runner — replacing all four `/bin/sh` literals):

1. `EVVA_SHELL` env var, if set (operator escape hatch, all platforms).
2. Non-Windows: `/bin/sh` (today's behavior, unchanged).
3. Windows: `bash` on `PATH` (`exec.LookPath`).
4. Windows: derive from git — `exec.LookPath("git")` →
   `<gitdir>\..\bin\bash.exe`; then the well-known installs
   (`C:\Program Files\Git\bin\bash.exe`,
   `%LOCALAPPDATA%\Programs\Git\bin\bash.exe`).
5. Found nothing → the tool returns a clear `IsError` result telling the
   model (and a startup warning telling the user) to install Git for
   Windows. **No cmd/PowerShell fallback.**

Notes:

- Use `<git>\bin\bash.exe`, not `<git>\usr\bin\bash.exe` directly — the
  `bin` shim sets up PATH so child processes see git/unix tools.
- Invocation stays `bash -c "<command>"`; `cmd.Dir` (workdir) and
  timeouts/cancel work identically once WIN-1's kill seam is in.
- On Windows the bash tool's `Description()` gains one sentence ("runs via
  Git Bash; POSIX paths like /c/Users/... map to C:\Users\...") so the
  model knows the dialect. The sysprompt env section already says
  `OS: windows`.

---

## 5. Design

### 5.1 `pkg/common/proc` — the only per-OS process seam

One new package; everything platform-specific about child processes lives
behind it. Files: `proc.go` (docs + shared), `proc_unix.go`
(`//go:build unix`), `proc_windows.go`.

```go
// Group configures cmd (before Start) so the child and all its
// descendants can later be killed as one unit.
//   unix:    SysProcAttr{Setpgid: true}
//   windows: SysProcAttr{CreationFlags: CREATE_NEW_PROCESS_GROUP}
func Group(cmd *exec.Cmd)

// KillTree terminates cmd's whole process tree. Intended as cmd.Cancel.
//   unix:    syscall.Kill(-pid, SIGKILL)
//   windows: exec `taskkill /T /F /PID <pid>` (ships with every Windows)
func KillTree(cmd *exec.Cmd) error

// Detach configures cmd (before Start) to outlive the parent terminal —
// the `evva service start` self-daemonize path.
//   unix:    SysProcAttr{Setsid: true}
//   windows: CreationFlags: CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS
func Detach(cmd *exec.Cmd)

// Alive reports whether pid names a live process.
//   unix:    FindProcess + Signal(0)
//   windows: OpenProcess existence check (FindProcess errs when gone)
func Alive(pid int) bool

// Terminate asks pid to stop.
//   unix:    SIGTERM (graceful — today's behavior)
//   windows: Process.Kill (hard; acceptable v1 — the service is
//            crash-safe by design: resume.go restores sessions, mail,
//            membership, alarms per the RP-18 unit.go comment)
func Terminate(pid int) error
```

Call-site migration (mechanical, behavior-preserving on unix):

| Site | Change |
|---|---|
| bash.go:170,180 / bash_daemon.go:156,164 / monitor_daemon.go:178,183 / repl.go:128,135 | `Group(cmd)` + `cmd.Cancel = func() error { return proc.KillTree(cmd) }` |
| lsp/client.go:53 | `Group(cmd)` (it has no kill-tree Cancel today; keep parity) |
| service.go:206 | `Detach(cmd)` |
| servicectl.go:100 | `Alive(pid)` |
| service.go:283 | `Terminate(pid)` |

`taskkill` vs Job Objects: Job Objects (via promoting `golang.org/x/sys`
to a direct dep) are the *robust* answer but need a handle created at
spawn time, which contorts the API. v1 ships `taskkill /T /F` — stateless,
preinstalled, and what most Go CLIs do. If bring-up shows orphan trees
(taskkill snapshots the tree; a racing fork can escape), upgrade
`proc_windows.go` to job objects **without touching any call site** — the
seam is the point. (§9 Q2.)

### 5.2 Shell resolution

`shellPath() (path string, err error)` per §4, in `pkg/tools/shell`
(exported or duplicated thinly for `pkg/hooks` — implementer's choice;
keep hooks' dependency surface small). Resolution result cached per
process (`sync.OnceValues`).

### 5.3 Self-update on Windows

Replace the unimplemented promise in self_replace.go:120 with the
standard **rename-aside** (no helper script needed — Windows allows
renaming a running `.exe`, only not deleting/overwriting it):

1. `os.Rename(dst, dst+".old")` — move the running exe aside.
2. `os.Rename(src, dst)` — drop the new exe into place.
3. Try `os.Remove(dst+".old")`; expected to fail while running → also
   sweep `*.old` next to the executable at startup (one `os.Remove`
   attempt, ignore errors).
4. On step-2 failure, roll back (`os.Rename(dst+".old", dst)`) and report.

Unix path stays byte-identical. `decompressAndWrite`'s `Chmod(0o755)` is a
no-op on Windows — fine. The temp file from `os.CreateTemp("", …)` may sit
on another volume than the exe; the existing `copyAndRemove` cross-device
fallback covers it, but extend `isCrossDevice` to match Windows'
"The system cannot move the file to a different disk drive." — or simpler,
create the temp file in `filepath.Dir(exe)` on all platforms.

### 5.4 Release pipeline

release.yml additions (still cross-compiled on `ubuntu-latest` — §3.1
proves no cgo):

- Matrix: `windows/amd64` (required), `windows/arm64` (best-effort, §9 Q1).
- Output: `-o evva.exe` when `goos == windows` (binary entry name must be
  exactly `evva.exe` — extractZip looks for it, self_replace.go:108).
- Package: `zip evva-windows-<arch>.zip evva.exe` (asset name must match
  `assetNameFor`, update.go:105: `evva-windows-amd64.zip`).

### 5.5 What deliberately does not change

- Tool semantics, tool names, schemas — identical on Windows.
- The agent loop, session persistence, swarm subsystem — pure Go, no
  per-OS code expected (the §3.1 build only flagged the five sites above).
- `main.go:110` signal handling.
- Permission gate `bash` classifier — Git Bash commands are still bash
  commands; classifier.go needs no Windows dialect.

---

## 6. Work items

**WIN-1 — `pkg/common/proc` + tool-site migration + CI cross-build gate.**
The §5.1 package; migrate the 4 tool packages; add a `windows-cross-build`
step to ci.yml (`GOOS=windows GOARCH=amd64 go build ./...` on the existing
ubuntu job — seconds, no new runner). *Accept:* cross-build green in CI;
`go test ./...` still green on unix; bash/monitor/repl timeout-kill
behavior unchanged on unix (existing tests cover this).

**WIN-2 — service on Windows.** Migrate service.go/servicectl.go to
`Detach`/`Alive`/`Terminate`. *Accept:* on a Windows box: `evva service
start` detaches and survives terminal close; `status` true while live,
false after kill; `stop` terminates and clears runtime files.

**WIN-3 — shell resolution.** §5.2 + replace the four `/bin/sh` literals
(bash.go:158, bash_daemon.go:152, monitor_daemon.go:177, runner.go:55) +
Windows-conditional sentence in the bash tool description + startup
warning when no bash found. *Accept:* on Windows with Git installed, bash
tool round-trips `git status`, pipes, `&&`, background daemons + kill;
with bash absent, tool returns the guidance error instead of spawn noise;
hooks fire.

**WIN-4 — interpreter & LSP & MCP bring-up.** repl resolution order on
Windows: `python` → `py` → `python3` (keep `python3` first elsewhere);
fix LSP URIs for drive letters (manager.go:277 → `file:///C:/…`,
diagnostics.go:219 inverse); verify an `npx`-shim MCP stdio server
launches. *Accept:* repl runs a python + a js snippet on Windows; gopls
session round-trips a definition request with correct URIs; one real
`.cmd`-shimmed MCP server lists tools.

**WIN-5 — home & path audit.** `resolveUserHome` (fs.go:118) gains
`os.UserHomeDir`; sweep for any remaining hardcoded `/`-joins or `/tmp`
(audit at f0c07bc found none outside tests, re-verify at merge time);
ensure logs/`.env`/skills/memdir paths build on `filepath.Join` (spot-check
says yes). *Accept:* `~`-prefixed Read/Write/Glob work on Windows.

**WIN-6 — distribution + self-update.** §5.3 + §5.4. *Accept:* release
workflow on a test tag produces both zips; on Windows, `evva update
v<test>` swaps the binary, relaunch reports the new version, `.old` swept;
on unix, update behavior byte-identical.

**WIN-7 — `windows-latest` CI test job.** Add `go build ./... && go vet
./... && go test ./...` on `windows-latest`; triage failures — fix genuine
portability bugs, add `if runtime.GOOS == "windows" { t.Skip("unix-only:
<reason>") }` only where the *feature* is unix-only (e.g. anything
asserting on `/bin/sh`). depcheck + swarm `-race` stay on ubuntu. Job is
`continue-on-error: true` during bring-up, flipped to required before
`v1.7.0-beta.1`. *Accept:* required and green.

**WIN-8 — docs.** README: delete the macOS/Linux-only line (README.md:20);
add Windows install (zip download + PATH, `go install`), prerequisites
(Git for Windows, Windows Terminal recommended, Win10 1903+), and the
Git-Bash note; user-guide page mirroring it; mention `EVVA_SHELL`.
*Accept:* a fresh Windows user can go from zero to a working session with
the doc alone.

**WIN-9 — validation pass on real hardware → `v1.7.0-beta.1`.** Manual
checklist on a physical/VM Windows 11 box (not just CI): TUI rendering +
input in Windows Terminal (resize, overlays, clipboard via OSC52),
bash/monitor/repl kill-on-timeout leaves no orphan `node.exe`, service
lifecycle, MCP + LSP + hooks, update flow, Ctrl+C interrupt. *Accept:*
checklist recorded in the PR; `pre-release feature` cut of
`v1.7.0-beta.1` with Windows assets attached.

Sequencing: WIN-1 → {WIN-2..WIN-6 in any order, parallel-friendly} →
WIN-7 → WIN-8/9. WIN-1 lands alone first — it is pure refactor on unix
and unblocks everything else.

---

## 7. CI plan summary

| Stage | Change | Cost |
|---|---|---|
| WIN-1 (immediately) | `GOOS=windows go build ./...` step on the existing ubuntu job | ~10 s/PR; catches every future `syscall` regression at compile time |
| WIN-7 (bring-up) | `windows-latest` job: build + vet + `go test ./...`, `continue-on-error` until triaged | windows runners are ~2× slower; full suite still minutes-scale |
| WIN-7 (exit) | flip to required | — |
| WIN-6 | release.yml windows matrix entries + zip packaging | seconds per release |

Workflow steps themselves can keep `shell: bash` on windows runners
(GitHub provides Git Bash there) — no PowerShell rewrite of any step.

---

## 8. Risks & mitigations

| Risk | Mitigation |
|---|---|
| **Path dialect confusion** — Git Bash shows `/c/Users/...`, Go tools emit `C:\Users\...`; model may mix them | Description note (§4); Git Bash accepts `C:/...` forms in most commands; validate in WIN-9; worst case add a sysprompt env hint |
| **`taskkill` misses racing children** | Acceptable v1; seam upgrade to Job Objects without call-site churn (§5.1, §9 Q2) |
| **MS Store `python.exe` alias** opens the Store instead of running | `py` in the resolution order (WIN-4); repl error text already tells the model what failed |
| **`.cmd` shim quoting** (npx-style MCP servers run via cmd.exe; Go quoting has edge cases) | WIN-4 acceptance includes one real shimmed server; document `cmd /c npx` workaround if an edge case bites |
| **Test suite hides unix assumptions** (volume unknown until WIN-7 runs) | `continue-on-error` bring-up window; skip-with-reason policy keeps the suite honest |
| **conhost rendering** on old terminals | Declare Windows Terminal the supported surface (§2 non-goals); termenv already enables VT processing where available |
| **Defender/SmartScreen flags an unsigned downloaded exe** | Document the unblock step in WIN-8; code-signing is out of scope for v1.7 (revisit if uptake warrants) |

---

## 9. Open questions

1. **Ship `windows/arm64`?** Recommend **yes** (one matrix row; Snapdragon
   laptops are growing; cross-compile is free). It ships untested-on-real-
   hardware → label best-effort in release notes.
2. **Job Objects in v1?** Recommend **no** — `taskkill` first, upgrade
   behind the `proc` seam if WIN-9 finds orphans. Promoting `x/sys` to a
   direct dep is pre-approved if so (already in the module graph).
3. **Graceful service stop on Windows?** v1 hard-kills (`Terminate`,
   crash-safe service). If clean drains start mattering, add an
   authenticated HTTP shutdown endpoint to the swarm service — that path
   is platform-neutral and better than process signals anyway.

---

## 10. Rollout

1. WIN-1..WIN-8 merge to `dev` via the normal `feature/*` flow.
2. `pre-release feature` → **`v1.7.0-beta.1`** — first tag whose release
   carries `evva-windows-amd64.zip` (+arm64). This debuts the v1.7 minor
   per the wave → minor map.
3. WIN-9 validation happens against that beta's real assets (`evva update
   v1.7.0-beta.1` on the Windows box exercises the whole §5.3 path).
4. Fixes fold in as `-beta.N` (`hotfix pre-release`); promotion to
   **`v1.7.0`** stable via `release` flips GitHub Latest, at which point
   `evva update` on Windows resolves stable like every other platform.
