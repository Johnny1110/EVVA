package daemon

// DaemonStatus is the lifecycle state of one daemon.
//
// Transitions:
//
//	Pending  → Running                 (rare — typically a daemon is Running on Register)
//	Running  → Completed               (clean exit / agent reported success)
//	Running  → Failed                  (non-zero exit / agent crashed)
//	Running  → Killed                  (daemon_stop / root ctx cancel)
//
// Terminal statuses (Completed / Failed / Killed) trigger a Lifecycle
// Signal that flows into the agent loop's drain, after which the entry is
// evicted from DaemonState.
type DaemonStatus string

const (
	StatusPending   DaemonStatus = "pending"
	StatusRunning   DaemonStatus = "running"
	StatusCompleted DaemonStatus = "completed"
	StatusFailed    DaemonStatus = "failed"
	StatusKilled    DaemonStatus = "killed"
)

// IsTerminal reports whether the status is one a daemon never leaves.
// Terminal entries are evicted from DaemonState after their Lifecycle signal
// is drained.
func IsTerminal(s DaemonStatus) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusKilled
}
