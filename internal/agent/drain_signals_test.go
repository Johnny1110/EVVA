package agent

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/johnny1110/evva/internal/session"
	"github.com/johnny1110/evva/internal/toolset"
	"github.com/johnny1110/evva/pkg/tools/daemon"
)

// newDrainTestAgent builds an agent shell sufficient for drain tests.
// We bypass agent.New because we don't need a real LLM client, tool
// registry, or signal pump — just the session + toolState + logger.
func newDrainTestAgent() *Agent {
	a := &Agent{
		ID:        "drain-test-agent",
		logger:    slog.Default(),
		session:   session.New(),
		toolState: toolset.NewToolState(),
	}
	return a
}

// drainTestDaemon is a tiny in-memory Daemon used solely to drive the
// drain pipeline. It mutates its own snapshot from outside the goroutine
// loop so tests can stage terminal state and then call Emit themselves.
type drainTestDaemon struct {
	snap daemon.DaemonSnapshot
}

func (d *drainTestDaemon) Snapshot() daemon.DaemonSnapshot   { return d.snap }
func (d *drainTestDaemon) Kill(_ context.Context) error      { return nil }
func (d *drainTestDaemon) Output() string                    { return "" }

func newBashTestDaemon(id, desc string, status daemon.DaemonStatus, exit int, output string) *drainTestDaemon {
	e := exit
	return &drainTestDaemon{
		snap: daemon.DaemonSnapshot{
			ID:          id,
			Kind:        daemon.KindLocalBash,
			Status:      status,
			Description: desc,
			StartedAt:   time.Now(),
			Metadata:    daemon.LocalBashMeta{Command: desc, ExitCode: &e, Output: output},
		},
	}
}

func TestDrainDaemonSignals_FoldsBashLifecyclesIntoSession(t *testing.T) {
	a := newDrainTestAgent()
	state := a.toolState.DaemonState()

	completed := newBashTestDaemon("b1", "echo hi", daemon.StatusCompleted, 0, "hi\n")
	failed := newBashTestDaemon("b2", "exit 1", daemon.StatusFailed, 1, "boom")
	state.Register(completed)
	state.Register(failed)
	state.Emit(daemon.NewLifecycleSignal(completed, daemon.StatusCompleted))
	state.Emit(daemon.NewLifecycleSignal(failed, daemon.StatusFailed))

	drained := a.drainDaemonSignals()
	if !drained {
		t.Fatal("drainDaemonSignals should return true when signals present")
	}

	msgs := a.session.Messages
	if len(msgs) != 1 {
		t.Fatalf("session message count: got %d want 1", len(msgs))
	}
	body := msgs[0].Content
	if !strings.Contains(body, "daemon b1 [local_bash] completed") {
		t.Errorf("missing b1 lifecycle: %q", body)
	}
	if !strings.Contains(body, "daemon b2 [local_bash] failed") {
		t.Errorf("missing b2 lifecycle: %q", body)
	}
	if !strings.Contains(body, "exit code 0") || !strings.Contains(body, "exit code 1") {
		t.Errorf("missing exit code lines: %q", body)
	}
	// Terminal daemons should be evicted from the catalog.
	if _, ok := state.Get("b1"); ok {
		t.Error("completed daemon should be evicted after drain")
	}
	if _, ok := state.Get("b2"); ok {
		t.Error("failed daemon should be evicted after drain")
	}
	if state.HasPending() {
		t.Error("HasPending should be false after drain")
	}
}

func TestDrainDaemonSignals_NoopWhenEmpty(t *testing.T) {
	a := newDrainTestAgent()
	if a.drainDaemonSignals() {
		t.Error("drain on empty state should return false")
	}
	if len(a.session.Messages) != 0 {
		t.Error("empty drain must not append to session")
	}
}

// newMonitorTestDaemon stands in for a monitor daemon in drain tests —
// emits Event signals through DaemonState.Emit so the unified drain path
// can be exercised without a real monitorDaemon goroutine.
func newMonitorTestDaemon(id, desc string, status daemon.DaemonStatus) *drainTestDaemon {
	return &drainTestDaemon{
		snap: daemon.DaemonSnapshot{
			ID:          id,
			Kind:        daemon.KindMonitor,
			Status:      status,
			Description: desc,
			StartedAt:   time.Now(),
			Metadata:    daemon.MonitorMeta{Command: desc, EventCount: 2},
		},
	}
}

func TestDrainDaemonSignals_FoldsMonitorEventsAndClosing(t *testing.T) {
	a := newDrainTestAgent()
	state := a.toolState.DaemonState()
	d := newMonitorTestDaemon("m1", "tail -F log", daemon.StatusRunning)
	state.Register(d)
	state.Emit(daemon.NewEventSignal(d, "log line one", false))
	state.Emit(daemon.NewEventSignal(d, "log line two", false))
	state.Emit(daemon.NewEventSignal(d, "", true))

	if !a.drainDaemonSignals() {
		t.Fatal("drain should report true when signals present")
	}
	msgs := a.session.Messages
	if len(msgs) != 1 {
		t.Fatalf("session messages: got %d want 1", len(msgs))
	}
	body := msgs[0].Content
	if !strings.Contains(body, "log line one") || !strings.Contains(body, "log line two") {
		t.Errorf("missing streamed lines: %q", body)
	}
	if !strings.Contains(body, "stream closed") {
		t.Errorf("missing closing marker: %q", body)
	}
	if !strings.Contains(body, "daemon m1 [monitor] events:") {
		t.Errorf("missing per-daemon event header: %q", body)
	}
}

func TestHasPendingSignals_TriggersWhenAny(t *testing.T) {
	a := newDrainTestAgent()
	if a.hasPendingSignals() {
		t.Error("fresh agent should not report pending signals")
	}
	state := a.toolState.DaemonState()
	d := newBashTestDaemon("b1", "echo hi", daemon.StatusRunning, 0, "")
	state.Register(d)
	if a.hasPendingSignals() {
		t.Error("registered (no signal) daemon should not register as pending")
	}
	state.Emit(daemon.NewLifecycleSignal(d, daemon.StatusCompleted))
	if !a.hasPendingSignals() {
		t.Error("pending lifecycle signal should make hasPendingSignals return true")
	}
}
