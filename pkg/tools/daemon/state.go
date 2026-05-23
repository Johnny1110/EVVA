package daemon

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/johnny1110/evva/pkg/observable"
)

// Domain is the observable.Change.Domain value DaemonState emits. The TUI
// strips and any other subscriber match this string to route renders.
const Domain = "daemons"

// Observable Op values DaemonState emits. Subscribers switch on these.
// Lifecycle transitions emit the status string directly ("running" /
// "completed" / "failed" / "killed") so subscribers can match a single
// case per terminal state.
const (
	OpAdded   = "added"   // Register
	OpRemoved = "removed" // Evict
	OpEvent   = "event"   // Signal.Event variant
)

// Errors returned by Stop. Tools type-assert to disambiguate response.
var (
	ErrDaemonNotFound  = errors.New("daemon not found")
	ErrAlreadyTerminal = errors.New("daemon already terminal")
)

// DaemonState is the single source of truth for one agent's daemons.
// Replaces the previous trio of BgTaskStore, MonitorTaskStore, and
// SpawnGroup. Holds:
//
//   - daemons map[id]Daemon — for lookup, kill, output.
//   - signals []Signal      — unified queue (Lifecycle + Event), drained
//     at each agent loop iter start.
//
// Embeds *observable.Observable so TUI strips receive per-change
// notifications with domain="daemons".
type DaemonState struct {
	mu      sync.RWMutex
	daemons map[string]Daemon
	signals []Signal
	*observable.Observable

	// notify wakes the agent loop when a signal arrives. No-arg by design:
	// the durable backstop is the signal queue, so the wake-up only needs
	// to fire the CAS+run-loop entry. May be nil in tests.
	notify func()
}

// NewState constructs an empty DaemonState. notify is the agent's
// signal-pump wake function — pass nil in tests that don't need
// wake semantics.
func NewState(notify func()) *DaemonState {
	return &DaemonState{
		daemons:    map[string]Daemon{},
		Observable: &observable.Observable{},
		notify:     notify,
	}
}

// Domain implements observable.Store.
func (s *DaemonState) Domain() string { return Domain }

// Register inserts d into the catalog. Emits an "added" observable.Change
// so TUI strips can render the new chip immediately. No-op when d is nil
// or its snapshot has an empty id.
//
// Idempotent: a second Register of the same id overwrites silently —
// daemons own their own transition timing, the store does not enforce it.
func (s *DaemonState) Register(d Daemon) {
	if d == nil {
		return
	}
	snap := d.Snapshot()
	if snap.ID == "" {
		return
	}
	s.mu.Lock()
	s.daemons[snap.ID] = d
	s.mu.Unlock()
	s.Notify(observable.Change{
		Domain:  Domain,
		Op:      OpAdded,
		ID:      snap.ID,
		Payload: snap,
	})
}

// Evict removes a daemon from the catalog. Emits a "removed" Change so
// strips can drop the row. Called by the agent loop's drain after a
// terminal Lifecycle signal has been folded into the conversation.
// No-op when the id is unknown.
func (s *DaemonState) Evict(id string) {
	s.mu.Lock()
	d, ok := s.daemons[id]
	if ok {
		delete(s.daemons, id)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	s.Notify(observable.Change{
		Domain:  Domain,
		Op:      OpRemoved,
		ID:      id,
		Payload: d.Snapshot(),
	})
}

// Get returns the daemon for id. ok=false when unknown.
func (s *DaemonState) Get(id string) (Daemon, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.daemons[id]
	return d, ok
}

// Snapshot returns every daemon's snapshot sorted by StartedAt. Used by
// daemon_list and TUI strips that need a static view.
func (s *DaemonState) Snapshot() []DaemonSnapshot {
	s.mu.RLock()
	out := make([]DaemonSnapshot, 0, len(s.daemons))
	for _, d := range s.daemons {
		out = append(out, d.Snapshot())
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out
}

// SnapshotByKind returns only the daemons matching the given kind, sorted
// by StartedAt. Used by daemon_list's kind filter and per-kind TUI strips.
func (s *DaemonState) SnapshotByKind(k DaemonKind) []DaemonSnapshot {
	all := s.Snapshot()
	out := all[:0]
	for _, snap := range all {
		if snap.Kind == k {
			out = append(out, snap)
		}
	}
	return out
}

// Stop is daemon_stop's entry point. Looks up id, asserts it's not already
// terminal, then calls daemon.Kill(ctx). The daemon's own goroutine is
// responsible for the subsequent terminal Lifecycle emission — Stop does
// not synthesise it.
//
// Returns the pre-kill snapshot. Errors: ErrDaemonNotFound,
// ErrAlreadyTerminal, or any error from Kill.
func (s *DaemonState) Stop(ctx context.Context, id string) (DaemonSnapshot, error) {
	d, ok := s.Get(id)
	if !ok {
		return DaemonSnapshot{}, ErrDaemonNotFound
	}
	snap := d.Snapshot()
	if IsTerminal(snap.Status) {
		return snap, ErrAlreadyTerminal
	}
	if err := d.Kill(ctx); err != nil {
		return snap, err
	}
	return snap, nil
}

// Emit is the entry point for daemon goroutines. Pushes the signal onto
// the queue, fans an observable Change for the matching Op, and fires the
// agent's wake-up function so an idle loop boots.
//
// The queue is the durable backstop — even if the wake-up is dropped
// (channel full), the next iter's drain still picks it up. This ordering
// invariant (queue first, then notify) is what lets the agent loop's
// CAS-based wake be lossy without losing daemon results.
func (s *DaemonState) Emit(sig Signal) {
	s.mu.Lock()
	s.signals = append(s.signals, sig)
	s.mu.Unlock()

	op := OpEvent
	if sig.Lifecycle != nil {
		op = string(sig.Lifecycle.Status)
	}
	s.Notify(observable.Change{
		Domain:  Domain,
		Op:      op,
		ID:      sig.DaemonID,
		Payload: sig.Snapshot,
	})

	if s.notify != nil {
		s.notify()
	}
}

// DrainSignals pulls every queued signal and clears the queue. Called by
// the agent loop at iter start; the drain helper folds them into the
// conversation as <system-reminder> blocks.
func (s *DaemonState) DrainSignals() []Signal {
	s.mu.Lock()
	if len(s.signals) == 0 {
		s.mu.Unlock()
		return nil
	}
	out := s.signals
	s.signals = nil
	s.mu.Unlock()
	return out
}

// HasPending reports whether any undrained signals remain. The agent loop
// checks this before releasing the run flag on a terminal turn — pending
// signals force one more iteration so the model sees the result.
func (s *DaemonState) HasPending() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.signals) > 0
}

// Len returns the number of registered daemons (running + terminal).
// Cheap; used by HasDaemonState-style accessors and by tests.
func (s *DaemonState) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.daemons)
}
