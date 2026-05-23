package shell

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnny1110/evva/pkg/tools"
	"github.com/johnny1110/evva/pkg/tools/daemon"
)

// fakeDaemonHost is a minimal DaemonHost for testing Bash run_in_background
// without spinning up a real ToolState. The notify closure increments a
// counter so tests can wait for the bg goroutine to emit its terminal
// Lifecycle signal.
type fakeDaemonHost struct {
	state    *daemon.DaemonState
	ctx      context.Context
	agentID  string
	notifies atomic.Int32
	done     chan struct{}
}

func newFakeDaemonHost(ctx context.Context) *fakeDaemonHost {
	h := &fakeDaemonHost{
		ctx:     ctx,
		agentID: "test-agent",
		done:    make(chan struct{}, 1),
	}
	h.state = daemon.NewState(func() {
		h.notifies.Add(1)
		select {
		case h.done <- struct{}{}:
		default:
		}
	})
	return h
}

func (h *fakeDaemonHost) DaemonState() *daemon.DaemonState { return h.state }
func (h *fakeDaemonHost) RootCtx() context.Context         { return h.ctx }
func (h *fakeDaemonHost) AgentID() string                  { return h.agentID }

func nopLogger() *slog.Logger { return tools.NopLogger() }

// awaitTerminal blocks until the daemon's snapshot reaches a terminal
// status or the deadline elapses. Returns the final snapshot.
func awaitTerminal(t *testing.T, h *fakeDaemonHost, id string, timeout time.Duration) daemon.DaemonSnapshot {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		// Wait for any notify to land, then check status. We may need to
		// loop because the first notify could arrive before the goroutine
		// flips the snapshot (extremely tight race) — re-check on every
		// signal until terminal.
		select {
		case <-h.done:
			d, ok := h.state.Get(id)
			if !ok {
				t.Fatalf("daemon %s missing from state", id)
			}
			snap := d.Snapshot()
			if daemon.IsTerminal(snap.Status) {
				return snap
			}
		case <-deadline.C:
			d, ok := h.state.Get(id)
			if ok {
				return d.Snapshot()
			}
			t.Fatalf("daemon %s did not reach terminal status within %s", id, timeout)
			return daemon.DaemonSnapshot{}
		}
	}
}

func TestBash_RunInBackground_HappyPath(t *testing.T) {
	host := newFakeDaemonHost(context.Background())
	tool := NewBashWithHost("", host)

	res, err := tool.Execute(context.Background(), nopLogger(), json.RawMessage(`{"command":"echo hi","run_in_background":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %q", res.Content)
	}
	if !strings.Contains(res.Content, "started in background") {
		t.Errorf("expected ack message; got %q", res.Content)
	}
	id := extractDaemonID(t, res.Content)

	snap := awaitTerminal(t, host, id, 3*time.Second)
	if snap.Status != daemon.StatusCompleted {
		t.Errorf("status: got %q want %q", snap.Status, daemon.StatusCompleted)
	}
	meta, ok := snap.Metadata.(daemon.LocalBashMeta)
	if !ok {
		t.Fatalf("metadata type: got %T want LocalBashMeta", snap.Metadata)
	}
	if meta.ExitCode == nil || *meta.ExitCode != 0 {
		t.Errorf("exit code: got %v want 0", meta.ExitCode)
	}
	if !strings.Contains(meta.Output, "hi") {
		t.Errorf("output: got %q want contains hi", meta.Output)
	}
}

func TestBash_RunInBackground_FailureCapturesExit(t *testing.T) {
	host := newFakeDaemonHost(context.Background())
	tool := NewBashWithHost("", host)

	res, err := tool.Execute(context.Background(), nopLogger(), json.RawMessage(`{"command":"exit 7","run_in_background":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	id := extractDaemonID(t, res.Content)

	snap := awaitTerminal(t, host, id, 3*time.Second)
	if snap.Status != daemon.StatusFailed {
		t.Errorf("status: got %q want %q", snap.Status, daemon.StatusFailed)
	}
	meta, ok := snap.Metadata.(daemon.LocalBashMeta)
	if !ok {
		t.Fatalf("metadata type: got %T want LocalBashMeta", snap.Metadata)
	}
	if meta.ExitCode == nil || *meta.ExitCode != 7 {
		t.Errorf("exit code: got %v want 7", meta.ExitCode)
	}
}

func TestBash_RunInBackground_StopKills(t *testing.T) {
	host := newFakeDaemonHost(context.Background())
	tool := NewBashWithHost("", host)

	out, err := tool.Execute(context.Background(), nopLogger(), json.RawMessage(`{"command":"sleep 30","run_in_background":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	id := extractDaemonID(t, out.Content)

	// Let the goroutine actually spawn before we Stop it.
	time.Sleep(100 * time.Millisecond)

	if _, err := host.state.Stop(context.Background(), id); err != nil {
		t.Fatalf("Stop should succeed: %v", err)
	}

	snap := awaitTerminal(t, host, id, 5*time.Second)
	if snap.Status != daemon.StatusKilled {
		t.Errorf("status: got %q want %q", snap.Status, daemon.StatusKilled)
	}
}

func TestBash_RunInBackground_RequiresHost(t *testing.T) {
	tool := NewBash("")
	res, err := tool.Execute(context.Background(), nopLogger(), json.RawMessage(`{"command":"echo","run_in_background":true}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError when no host")
	}
	if !strings.Contains(res.Content, "not available") {
		t.Errorf("expected clear error; got %q", res.Content)
	}
}

// extractDaemonID pulls the "b…" id out of the bash ack message.
// Format: "Daemon b… started in background. ...".
func extractDaemonID(t *testing.T, ack string) string {
	t.Helper()
	parts := strings.Fields(ack)
	if len(parts) < 2 {
		t.Fatalf("ack message too short: %q", ack)
	}
	id := parts[1]
	if !strings.HasPrefix(id, "b") {
		t.Fatalf("expected daemon id prefix 'b', got %q", id)
	}
	return id
}

