package agent

// drain_signals.go used to host kind-specific drain helpers
// (drainBackgroundTaskResults, drainMonitorEvents). After the daemon
// refactor those were consolidated into drainDaemonSignals in
// drain_daemons.go — every kind of background unit (bash bg, monitor,
// async subagent, future remote_agent / dream / ...) flows through that
// single path.
//
// hasPendingSignals is the cross-store check the agent loop uses at the
// terminal turn to decide whether to release the run flag or loop one
// more iteration. Today DaemonState is the only signal-backed store; the
// helper keeps its name and shape so future per-kind stores (if any) can
// add their own pending check here without churning loop.go.
func (a *Agent) hasPendingSignals() bool {
	if a.toolState.HasDaemonState() && a.toolState.DaemonState().HasPending() {
		return true
	}
	return false
}
