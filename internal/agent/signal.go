package agent

import (
	"strings"

	"github.com/johnny1110/evva/pkg/llm"
)

// SignalKind tags an AgentSignal. After the daemon refactor only one kind
// remains — every background unit (bash bg, monitor, async subagent)
// flows through SignalDaemon. The constant is kept as a SignalKind value
// (rather than dropping the enum) so future signal kinds — e.g. a UI
// nudge, a remote control event — can plug in without touching the loop.
type SignalKind string

const (
	// SignalDaemon wakes the loop after any daemon emits a signal
	// (lifecycle transition or stream event). The daemon state is the
	// authoritative source — this signal carries no payload; the drain
	// at iter start re-reads from DaemonState.DrainSignals.
	SignalDaemon SignalKind = "daemon"
)

// AgentSignal is the unit the agent's signal channel carries. Today
// SignalDaemon is the only kind — it's wake-only, no payload. New kinds
// would add typed pointer fields here and a matching case in
// emitSignalEvent.
type AgentSignal struct {
	Kind SignalKind
}

// signalChanCap is the buffered capacity of a.signalCh. Sized so a
// burst of monitor events doesn't block the producing goroutine; if
// the buffer fills (~64 in-flight signals) we drop the wake-up signal
// and rely on the next-iter drain — the store / queue is the durable
// backstop, the chan is only the wake-up vehicle.
const signalChanCap = 64

// SendSignal pushes one signal on a.signalCh. Non-blocking — if the
// chan is full the signal is dropped and we log; the loop's drain path
// at iter start still picks up the result because the producer wrote
// the store BEFORE calling SendSignal.
//
// Safe to call from any goroutine; the chan does its own
// synchronisation.
func (a *Agent) SendSignal(sig AgentSignal) {
	if a.signalCh == nil {
		return
	}
	select {
	case a.signalCh <- sig:
	default:
		a.logger.Warn("signal.dropped", "kind", sig.Kind, "reason", "chan_full")
	}
}

// signalPump is the per-agent goroutine started in agent.New that
// listens for AgentSignals and either wakes an idle agent (CAS on
// a.running, spawn a runLoop) or relies on the running loop's
// iteration-boundary drain.
//
// The pump exits cleanly when rootCtx is cancelled (agent.Shutdown
// closes the chan via rootCancel).
func (a *Agent) signalPump() {
	for {
		select {
		case <-a.rootCtx.Done():
			a.logger.Debug("signal.pump.exit", "reason", "root_ctx_done")
			return
		case sig, ok := <-a.signalCh:
			if !ok {
				a.logger.Debug("signal.pump.exit", "reason", "chan_closed")
				return
			}
			a.handleSignal(sig)
		}
	}
}

// handleSignal does two things per signal:
//
//  1. Optionally emit a wire event for the TUI (today SignalDaemon is
//     wake-only because the Observable store path already covers TUI
//     fan-out for daemons; future signal kinds can hook emit here).
//
//  2. If the loop is idle, try to acquire it via CAS on a.running and
//     start a fresh runLoop. The store mutation already happened on the
//     producer side, so the new runLoop's drain at iter start picks it up.
//
// Subagents never wake on signals — only the root agent. Subagent results
// bubble up through event.BubbleUp to the parent's TUI strip; their
// conversation context is rebuilt only by the parent's next dispatch.
func (a *Agent) handleSignal(sig AgentSignal) {
	a.emitSignalEvent(sig)

	if a.IsSubagent() {
		return
	}
	if !a.running.CompareAndSwap(false, true) {
		// Busy path: the live runLoop's next iteration drain pulls the
		// terminal store entry / queue event into the conversation. No
		// further action needed here.
		return
	}
	// Idle path: we own the run flag. Spawn the run on a fresh goroutine
	// so the pump stays free for follow-up signals; clear the flag in
	// defer.
	go a.runFromSignal()
}

// emitSignalEvent fires a typed wire event for each signal kind, if the
// kind has one. SignalDaemon is wake-only because the Observable store
// path (DaemonState → ToolState fanout → KindStoreUpdate) already covers
// TUI fan-out.
func (a *Agent) emitSignalEvent(sig AgentSignal) {
	switch sig.Kind {
	case SignalDaemon:
		// Wake-only — no wire event.
	}
}

// runFromSignal is the idle-wake entry. We already CAS-acquired
// a.running on the pump side; defer-release it on the way out. The
// drain at the top of runLoop folds the queued result / event into
// the session, so we don't seed a user message here ourselves.
func (a *Agent) runFromSignal() {
	defer a.running.Store(false)
	a.logger.Debug("run.signal_wake")
	if _, err := a.runLoop(a.rootCtx); err != nil {
		a.logger.Warn("run.signal_wake.err", "err", err)
	}
}

// signalReminderMessage assembles one RoleUser message body from the
// list of drained reminders, joining with newlines. Empty input
// returns the zero-length llm.Message (caller should short-circuit
// before appending).
func signalReminderMessage(parts []string) llm.Message {
	return llm.Message{Role: llm.RoleUser, Content: strings.Join(parts, "\n")}
}
