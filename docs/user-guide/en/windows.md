# evva on Windows

evva ships native Windows binaries (windows/amd64 required tier,
windows/arm64 best-effort) starting with v1.7.0. This page covers what is
different from macOS/Linux; everything not mentioned here works the same.

## Install

1. Download `evva-windows-amd64.zip` (or `-arm64`) from the
   [latest release](https://github.com/johnny1110/evva/releases/latest).
2. Unzip `evva.exe` into a directory on your `PATH`
   (e.g. `%LOCALAPPDATA%\Programs\evva\`).
3. Run `evva` from **Windows Terminal**.

Alternative with Go 1.25+: `go install github.com/johnny1110/evva/cmd/evva@latest`
(lands in `%USERPROFILE%\go\bin`).

If SmartScreen blocks the downloaded exe: right-click → Properties →
**Unblock**. The release binaries are unsigned for now.

`evva update` works as on other platforms. The one Windows quirk is
mechanical and invisible: the running exe is renamed aside to `evva.exe.old`
and the new one takes its place; the leftover `.old` is deleted the next
time evva starts.

## Prerequisites

| What | Why |
| --- | --- |
| [Git for Windows](https://gitforwindows.org) | The agent's `bash` tool, `monitor` daemons, and lifecycle hooks run through its bundled **Git Bash**. evva finds `bash.exe` by deriving it from `git`'s install location, then well-known install dirs, then `PATH` (never `System32\bash.exe` — that's the WSL launcher). |
| Windows Terminal (recommended) | The TUI works on legacy conhost (Windows 10 1903+ for VT processing) but is only validated on Windows Terminal. |
| Python and/or Node (optional) | For the `repl` tool. On Windows the Python lookup prefers the `py` launcher, then `python`, then `python3`. |

Without Git Bash, evva starts and prints a warning; file tools, web tools,
LSP, and MCP keep working, but `bash`/`monitor`/hooks return a clear error
until you install Git for Windows.

**`EVVA_SHELL`** overrides shell autodetection on every platform — point
it at any POSIX shell that accepts `-c` (e.g. an MSYS2 `bash.exe`).

## Shell dialect

The `bash` tool speaks **POSIX bash** on Windows too — that is the whole
point of requiring Git Bash. Inside the shell, drive paths appear in POSIX
form (`/c/Users/me` for `C:\Users\me`); both forms are accepted by git and
most tooling. There is deliberately no PowerShell/cmd mode.

## `evva service` on Windows

`start` / `stop` / `status` work: the daemon detaches from the terminal,
writes its pidfile/token/addr under `%USERPROFILE%\.evva\service\`, and
`stop` terminates it (a hard kill — the service is crash-safe and resumes
sessions, mail, membership, and alarms on restart).

`evva service install-unit` has **no Windows template yet** (launchd and
systemd only). To auto-start the service, create a Task Scheduler task
running `evva service start --foreground` at logon.

## Known limitations (v1.7)

- Release binaries are unsigned (SmartScreen prompt on first run).
- windows/arm64 is cross-compiled but not validated on real hardware.
- No service autostart unit template (see above).
- Process-tree termination uses `taskkill /T`; pathological commands that
  fork faster than the snapshot can theoretically leave orphans — report
  it if you see one.
