package daemon

import "context"

// Daemon is the polymorphic contract every kind of background unit
// satisfies. Three methods, one for each concern:
//
//   - Snapshot — read state without exposing internals.
//   - Kill     — cooperative cancellation (kind-specific: ctx cancel for
//     bash/monitor, abort signal for agent, ...).
//   - Output   — kind-specific formatted text for daemon_output.
//
// Implementations live next to their owning tool: bashDaemon in
// pkg/tools/shell, monitorDaemon in pkg/tools/monitor, agentDaemon in
// internal/agent. Each implementation owns its own goroutine and calls
// DaemonState.Emit on lifecycle transitions / stream lines.
type Daemon interface {
	Snapshot() DaemonSnapshot
	Kill(ctx context.Context) error
	Output() string
}
