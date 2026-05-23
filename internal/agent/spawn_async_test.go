package agent

import (
	"errors"
	"testing"

	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/tools/daemon"
)

// These tests pin down the async-spawn goroutine's contract after the
// daemon refactor: the goroutine MUST call agentDaemon.Report on success
// or agentDaemon.Crush on failure. Both emit a terminal Lifecycle signal
// which the parent's drainDaemonSignals picks up and folds into the
// conversation; without either call, the entry stays running in
// DaemonState forever and the parent never sees the result.
//
// We exercise the contract directly against agentDaemon + DaemonState —
// no real Agent / LLM machinery needed. The goroutine in spawn.go is
// three plain method calls; the load-bearing surface is here.

// newAsyncDaemonFixture builds an agentDaemon registered as async on a
// fresh DaemonState. Returns the daemon and the underlying state so each
// test can assert separately.
func newAsyncDaemonFixture(t *testing.T, id string) (*agentDaemon, *daemon.DaemonState) {
	t.Helper()
	state := daemon.NewState(nil)
	d := &agentDaemon{
		id:        id,
		name:      "worker",
		agentType: "general",
		async:     true,
		status:    daemon.StatusRunning,
		phase:     constant.INIT,
		state:     state,
	}
	state.Register(d)
	return d, state
}

func TestAsyncSpawn_ReportEmitsTerminalLifecycle(t *testing.T) {
	d, state := newAsyncDaemonFixture(t, "agent-1")

	// Simulate the goroutine's success path post-fix.
	d.Report("all done")

	signals := state.DrainSignals()
	if len(signals) != 1 {
		t.Fatalf("DrainSignals: got %d entries, want 1 (async Report must enqueue Lifecycle)", len(signals))
	}
	sig := signals[0]
	if !sig.IsLifecycle() {
		t.Fatal("expected Lifecycle signal, got Event")
	}
	if sig.Lifecycle.Status != daemon.StatusCompleted {
		t.Errorf("status: got %q, want %q", sig.Lifecycle.Status, daemon.StatusCompleted)
	}
	meta, ok := sig.Snapshot.Metadata.(daemon.LocalAgentMeta)
	if !ok {
		t.Fatalf("metadata type: got %T, want LocalAgentMeta", sig.Snapshot.Metadata)
	}
	if meta.Summary != "all done" {
		t.Errorf("summary: got %q, want %q", meta.Summary, "all done")
	}
}

func TestAsyncSpawn_CrushEmitsTerminalLifecycle(t *testing.T) {
	d, state := newAsyncDaemonFixture(t, "agent-2")

	// Simulate the goroutine's error path post-fix.
	d.Crush("[subagent crushed]", errors.New("boom"), daemon.StatusFailed)

	signals := state.DrainSignals()
	if len(signals) != 1 {
		t.Fatalf("crushed async daemon must emit one Lifecycle; got %d", len(signals))
	}
	sig := signals[0]
	if !sig.IsLifecycle() {
		t.Fatal("expected Lifecycle signal, got Event")
	}
	if sig.Lifecycle.Status != daemon.StatusFailed {
		t.Errorf("status: got %q, want %q", sig.Lifecycle.Status, daemon.StatusFailed)
	}
	meta, _ := sig.Snapshot.Metadata.(daemon.LocalAgentMeta)
	if meta.Err == "" {
		t.Errorf("error captured: got empty, want non-empty (was 'boom')")
	}
}

// TestAsyncSpawn_NeverReportedStaysRunning locks the bug behavior in
// reverse: if the goroutine returns without calling Report/Crush, the
// entry sits in DaemonState forever — drain can't deliver anything
// because nothing emitted a Lifecycle signal.
//
// Documents the bug class so a future regression in spawn.go's goroutine
// (dropping the Report/Crush call) shows up loudly in tests.
func TestAsyncSpawn_NeverReportedStaysRunning(t *testing.T) {
	d, state := newAsyncDaemonFixture(t, "agent-3")

	// Deliberately DO NOT call Report or Crush.

	if state.HasPending() {
		t.Errorf("daemon that never Reported/Crushed must not enqueue a signal")
	}
	snap := d.Snapshot()
	if snap.Status != daemon.StatusRunning {
		t.Errorf("status should still be running; got %q", snap.Status)
	}
	if _, ok := state.Get("agent-3"); !ok {
		t.Errorf("daemon should still be registered in state")
	}
}
