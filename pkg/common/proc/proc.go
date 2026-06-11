// Package proc is the single per-OS seam for spawning and managing child
// processes. Everything platform-specific about children — process-group
// vs taskkill-tree kill semantics, daemon detach flags, liveness probes,
// termination, and which POSIX shell backs `sh -c`-style tools — lives
// behind this package so call sites stay platform-free.
//
// unix keeps the pre-Windows behavior exactly (Setpgid groups, SIGKILL to
// the negative pid, Setsid detach, signal-0 liveness, SIGTERM stop).
// windows maps each operation to its closest native equivalent; see
// docs/roadmap/PRD/windows-support.md §5.1 for the design and the
// taskkill-vs-job-object tradeoff.
package proc

import "os/exec"

// Group configures cmd (before Start) so the child and all its
// descendants can later be killed as one unit via KillTree.
//
//	unix:    SysProcAttr{Setpgid: true}
//	windows: CreationFlags |= CREATE_NEW_PROCESS_GROUP
func Group(cmd *exec.Cmd) { group(cmd) }

// KillTree terminates cmd's whole process tree. Intended as (part of) a
// cmd.Cancel hook on a command prepared with Group. Best-effort by
// nature — the tree may already be gone; callers typically ignore the
// error and rely on cmd.WaitDelay to reap straggling pipe holders.
// A cmd that never started is a no-op.
//
//	unix:    syscall.Kill(-pid, SIGKILL)
//	windows: taskkill /T /F /PID <pid>
func KillTree(cmd *exec.Cmd) error { return killTree(cmd) }

// Detach configures cmd (before Start) to outlive the parent terminal —
// the `evva service start` self-daemonize path.
//
//	unix:    SysProcAttr{Setsid: true}
//	windows: CreationFlags |= CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS
func Detach(cmd *exec.Cmd) { detach(cmd) }

// Alive reports whether pid names a live process.
//
//	unix:    FindProcess + Signal(0) — existence check, nothing delivered
//	windows: FindProcess opens a real handle and fails when pid is gone
func Alive(pid int) bool { return alive(pid) }

// Terminate asks pid to stop.
//
//	unix:    SIGTERM (graceful)
//	windows: Process.Kill — there is no SIGTERM; hard-stop is acceptable
//	         for evva's own daemons, which are crash-safe by design
//	         (service resume restores sessions, mail, membership, alarms)
func Terminate(pid int) error { return terminate(pid) }
