package daemon

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnny1110/evva/pkg/observable"
)

// fakeDaemon is the test stand-in for any kind's Daemon implementation.
// All accessor methods are mutex-free because tests drive it from a single
// goroutine; for tests that need concurrent emission we coordinate via
// goroutines + channels at the call sites.
type fakeDaemon struct {
	snap    DaemonSnapshot
	killErr error
	killed  atomic.Bool
	output  string
}

func (d *fakeDaemon) Snapshot() DaemonSnapshot { return d.snap }
func (d *fakeDaemon) Kill(_ context.Context) error {
	d.killed.Store(true)
	if d.killErr != nil {
		return d.killErr
	}
	// Real daemons would flip to Killed via Emit; for tests we mutate
	// directly so a follow-up Get sees the terminal state.
	d.snap.Status = StatusKilled
	d.snap.EndedAt = time.Now()
	return nil
}
func (d *fakeDaemon) Output() string { return d.output }

func newFakeDaemon(id string, kind DaemonKind, status DaemonStatus) *fakeDaemon {
	return &fakeDaemon{
		snap: DaemonSnapshot{
			ID:          id,
			Kind:        kind,
			Status:      status,
			Description: "test daemon " + id,
			StartedAt:   time.Now(),
		},
	}
}

// --- ID generation ------------------------------------------------------

func TestGenerateID_PrefixAndLength(t *testing.T) {
	cases := map[DaemonKind]rune{
		KindLocalBash:  'b',
		KindLocalAgent: 'a',
		KindMonitor:    'm',
		KindDream:      'd',
	}
	for kind, want := range cases {
		id := GenerateID(kind)
		if len(id) != 9 {
			t.Errorf("kind=%s: id length = %d, want 9 (prefix + 8 chars)", kind, len(id))
		}
		if rune(id[0]) != want {
			t.Errorf("kind=%s: prefix = %q, want %q", kind, string(id[0]), string(want))
		}
	}
}

func TestGenerateID_UnknownKindFallsBackToX(t *testing.T) {
	id := GenerateID(DaemonKind("totally-unknown"))
	if rune(id[0]) != 'x' {
		t.Errorf("unknown kind: prefix = %q, want %q", string(id[0]), "x")
	}
}

// --- Status helpers -----------------------------------------------------

func TestIsTerminal(t *testing.T) {
	cases := map[DaemonStatus]bool{
		StatusPending:   false,
		StatusRunning:   false,
		StatusCompleted: true,
		StatusFailed:    true,
		StatusKilled:    true,
	}
	for s, want := range cases {
		if got := IsTerminal(s); got != want {
			t.Errorf("IsTerminal(%q) = %v, want %v", s, got, want)
		}
	}
}

// --- Register / Get / Snapshot ------------------------------------------

func TestNewState_Empty(t *testing.T) {
	s := NewState(nil)
	if s.Len() != 0 {
		t.Errorf("new state Len = %d, want 0", s.Len())
	}
	if got := s.Snapshot(); len(got) != 0 {
		t.Errorf("new state Snapshot len = %d, want 0", len(got))
	}
	if s.HasPending() {
		t.Errorf("new state HasPending = true, want false")
	}
}

func TestRegister_StoresDaemon(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	s.Register(d)
	got, ok := s.Get("b00000001")
	if !ok {
		t.Fatal("Get after Register: ok = false")
	}
	if got.Snapshot().ID != "b00000001" {
		t.Errorf("Get returned wrong daemon: %+v", got.Snapshot())
	}
}

func TestRegister_NilDaemonNoOp(t *testing.T) {
	s := NewState(nil)
	s.Register(nil)
	if s.Len() != 0 {
		t.Errorf("Register(nil) altered state: Len = %d", s.Len())
	}
}

func TestRegister_EmptyIDNoOp(t *testing.T) {
	s := NewState(nil)
	d := &fakeDaemon{snap: DaemonSnapshot{Kind: KindLocalBash, Status: StatusRunning}}
	s.Register(d)
	if s.Len() != 0 {
		t.Errorf("Register(empty id) altered state: Len = %d", s.Len())
	}
}

func TestSnapshot_SortedByStartedAt(t *testing.T) {
	s := NewState(nil)
	t0 := time.Now()

	older := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	older.snap.StartedAt = t0.Add(-2 * time.Minute)

	newer := newFakeDaemon("b00000002", KindLocalBash, StatusRunning)
	newer.snap.StartedAt = t0

	mid := newFakeDaemon("b00000003", KindLocalBash, StatusRunning)
	mid.snap.StartedAt = t0.Add(-1 * time.Minute)

	// Register in non-chronological order to ensure sort, not insertion order.
	s.Register(newer)
	s.Register(older)
	s.Register(mid)

	got := s.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].ID != "b00000001" || got[1].ID != "b00000003" || got[2].ID != "b00000002" {
		t.Errorf("not sorted by StartedAt: ids = %s, %s, %s", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestSnapshotByKind_Filters(t *testing.T) {
	s := NewState(nil)
	s.Register(newFakeDaemon("b00000001", KindLocalBash, StatusRunning))
	s.Register(newFakeDaemon("m00000001", KindMonitor, StatusRunning))
	s.Register(newFakeDaemon("a00000001", KindLocalAgent, StatusRunning))

	if got := s.SnapshotByKind(KindLocalBash); len(got) != 1 || got[0].ID != "b00000001" {
		t.Errorf("bash filter: %+v", got)
	}
	if got := s.SnapshotByKind(KindMonitor); len(got) != 1 || got[0].ID != "m00000001" {
		t.Errorf("monitor filter: %+v", got)
	}
	if got := s.SnapshotByKind(KindDream); len(got) != 0 {
		t.Errorf("absent kind: expected empty, got %+v", got)
	}
}

// --- Evict --------------------------------------------------------------

func TestEvict_RemovesDaemon(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusCompleted)
	s.Register(d)
	s.Evict("b00000001")
	if _, ok := s.Get("b00000001"); ok {
		t.Errorf("Evict didn't remove daemon")
	}
}

func TestEvict_UnknownIDNoOp(t *testing.T) {
	s := NewState(nil)
	s.Evict("nonexistent")
	if s.Len() != 0 {
		t.Errorf("Evict(unknown) altered state")
	}
}

// --- Stop ---------------------------------------------------------------

func TestStop_NotFound(t *testing.T) {
	s := NewState(nil)
	_, err := s.Stop(context.Background(), "nonexistent")
	if !errors.Is(err, ErrDaemonNotFound) {
		t.Errorf("Stop(unknown) err = %v, want ErrDaemonNotFound", err)
	}
}

func TestStop_AlreadyTerminal(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusCompleted)
	s.Register(d)
	snap, err := s.Stop(context.Background(), "b00000001")
	if !errors.Is(err, ErrAlreadyTerminal) {
		t.Errorf("Stop(terminal) err = %v, want ErrAlreadyTerminal", err)
	}
	if snap.ID != "b00000001" {
		t.Errorf("snap.ID = %q, want b00000001", snap.ID)
	}
	if d.killed.Load() {
		t.Errorf("terminal daemon's Kill was called")
	}
}

func TestStop_Success(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	s.Register(d)
	snap, err := s.Stop(context.Background(), "b00000001")
	if err != nil {
		t.Fatalf("Stop err = %v", err)
	}
	if !d.killed.Load() {
		t.Errorf("Kill was not called on the daemon")
	}
	// Returned snapshot is the pre-kill state (Running) — the terminal
	// transition happens after Kill via the daemon's own emit. Tests for
	// the post-kill state belong with kind-specific daemons, not the store.
	if snap.Status != StatusRunning {
		t.Errorf("Stop returned status = %q, want %q (pre-kill snapshot)", snap.Status, StatusRunning)
	}
}

func TestStop_KillError(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	d.killErr = errors.New("simulated kill failure")
	s.Register(d)
	_, err := s.Stop(context.Background(), "b00000001")
	if err == nil || errors.Is(err, ErrDaemonNotFound) || errors.Is(err, ErrAlreadyTerminal) {
		t.Errorf("Stop with kill error = %v, want a generic kill error", err)
	}
}

// --- Emit / Drain -------------------------------------------------------

func TestEmit_LifecycleSignal(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	s.Register(d)

	d.snap.Status = StatusCompleted
	d.snap.EndedAt = time.Now()
	s.Emit(NewLifecycleSignal(d, StatusCompleted))

	if !s.HasPending() {
		t.Fatal("HasPending = false after Emit")
	}
	signals := s.DrainSignals()
	if len(signals) != 1 {
		t.Fatalf("DrainSignals returned %d, want 1", len(signals))
	}
	sig := signals[0]
	if !sig.IsLifecycle() {
		t.Errorf("expected Lifecycle signal, got Event")
	}
	if sig.Lifecycle.Status != StatusCompleted {
		t.Errorf("Lifecycle.Status = %q, want %q", sig.Lifecycle.Status, StatusCompleted)
	}
	if sig.Snapshot.Status != StatusCompleted {
		t.Errorf("Snapshot.Status = %q, want %q", sig.Snapshot.Status, StatusCompleted)
	}
}

func TestEmit_EventSignal(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("m00000001", KindMonitor, StatusRunning)
	s.Register(d)

	s.Emit(NewEventSignal(d, "first line", false))
	s.Emit(NewEventSignal(d, "second line", false))
	s.Emit(NewEventSignal(d, "", true))

	signals := s.DrainSignals()
	if len(signals) != 3 {
		t.Fatalf("DrainSignals returned %d, want 3", len(signals))
	}
	if signals[0].Event.Line != "first line" || signals[1].Event.Line != "second line" {
		t.Errorf("event lines: %+v", signals)
	}
	if !signals[2].Event.Closing {
		t.Errorf("third event Closing = false, want true")
	}
}

func TestDrainSignals_ClearsQueue(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	s.Register(d)
	s.Emit(NewLifecycleSignal(d, StatusCompleted))

	first := s.DrainSignals()
	if len(first) != 1 {
		t.Fatalf("first drain returned %d, want 1", len(first))
	}
	second := s.DrainSignals()
	if second != nil {
		t.Errorf("second drain returned %+v, want nil", second)
	}
	if s.HasPending() {
		t.Errorf("HasPending after drain = true, want false")
	}
}

// --- Wake notifier ------------------------------------------------------

func TestEmit_NotifyFires(t *testing.T) {
	var fired atomic.Int32
	s := NewState(func() { fired.Add(1) })
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	s.Register(d)
	s.Emit(NewLifecycleSignal(d, StatusCompleted))
	if got := fired.Load(); got != 1 {
		t.Errorf("notify fired %d times, want 1", got)
	}
}

func TestEmit_NilNotifyIsSafe(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	s.Register(d)
	// Must not panic.
	s.Emit(NewLifecycleSignal(d, StatusCompleted))
}

// --- Observable fan-out --------------------------------------------------

func TestObservable_RegisterEmitsAdded(t *testing.T) {
	s := NewState(nil)
	var got []observable.Change
	s.Subscribe(func(c observable.Change) { got = append(got, c) })

	s.Register(newFakeDaemon("b00000001", KindLocalBash, StatusRunning))

	if len(got) != 1 {
		t.Fatalf("subscriber received %d changes, want 1", len(got))
	}
	if got[0].Domain != Domain {
		t.Errorf("Domain = %q, want %q", got[0].Domain, Domain)
	}
	if got[0].Op != OpAdded {
		t.Errorf("Op = %q, want %q", got[0].Op, OpAdded)
	}
	if got[0].ID != "b00000001" {
		t.Errorf("ID = %q, want b00000001", got[0].ID)
	}
	if _, ok := got[0].Payload.(DaemonSnapshot); !ok {
		t.Errorf("Payload not a DaemonSnapshot: %T", got[0].Payload)
	}
}

func TestObservable_EvictEmitsRemoved(t *testing.T) {
	s := NewState(nil)
	s.Register(newFakeDaemon("b00000001", KindLocalBash, StatusCompleted))

	var ops []string
	s.Subscribe(func(c observable.Change) { ops = append(ops, c.Op) })

	s.Evict("b00000001")
	if len(ops) != 1 || ops[0] != OpRemoved {
		t.Errorf("ops = %+v, want [removed]", ops)
	}
}

func TestObservable_EmitUsesStatusAsOpForLifecycle(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("b00000001", KindLocalBash, StatusRunning)
	s.Register(d)

	var ops []string
	s.Subscribe(func(c observable.Change) { ops = append(ops, c.Op) })

	d.snap.Status = StatusKilled
	s.Emit(NewLifecycleSignal(d, StatusKilled))

	if len(ops) != 1 || ops[0] != string(StatusKilled) {
		t.Errorf("ops = %+v, want [killed]", ops)
	}
}

func TestObservable_EmitEventUsesOpEvent(t *testing.T) {
	s := NewState(nil)
	d := newFakeDaemon("m00000001", KindMonitor, StatusRunning)
	s.Register(d)

	var ops []string
	s.Subscribe(func(c observable.Change) { ops = append(ops, c.Op) })

	s.Emit(NewEventSignal(d, "hello", false))
	if len(ops) != 1 || ops[0] != OpEvent {
		t.Errorf("ops = %+v, want [event]", ops)
	}
}

func TestDomain_MatchesConst(t *testing.T) {
	s := NewState(nil)
	if s.Domain() != Domain {
		t.Errorf("Domain() = %q, want %q", s.Domain(), Domain)
	}
}
