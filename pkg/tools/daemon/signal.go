package daemon

import "time"

// Signal is the unit DaemonState's queue carries from per-daemon goroutines
// to the agent loop's drain. Exactly one of Lifecycle / Event is non-nil.
//
// Lifecycle fires once per terminal transition (Completed / Failed /
// Killed) and triggers eviction. Event fires per stream line — used by
// monitors; kinds that don't stream (bash, local_agent) only emit
// Lifecycle.
//
// Snapshot is captured at signal-emission time so drain readers see a
// consistent view even if the daemon is mutating concurrently.
type Signal struct {
	DaemonID string
	Kind     DaemonKind
	At       time.Time
	Snapshot DaemonSnapshot

	Lifecycle *Lifecycle
	Event     *Event
}

// IsLifecycle reports whether this is a lifecycle transition signal.
func (s Signal) IsLifecycle() bool { return s.Lifecycle != nil }

// IsEvent reports whether this is a stream-event signal.
func (s Signal) IsEvent() bool { return s.Event != nil }

// Lifecycle is the variant fired when a daemon enters a terminal status.
type Lifecycle struct {
	Status DaemonStatus
}

// Event is the variant fired per stream line by daemons that stream
// (monitors). Closing marks the final event before the daemon's terminal
// Lifecycle — drain renders it as a distinct phrasing so the model knows
// no more events are coming.
type Event struct {
	Line    string
	Closing bool
}

// NewLifecycleSignal constructs a Lifecycle signal carrying the daemon's
// current snapshot. Daemons call this from their goroutine right before
// state.Emit at terminal transition time.
func NewLifecycleSignal(d Daemon, status DaemonStatus) Signal {
	snap := d.Snapshot()
	return Signal{
		DaemonID:  snap.ID,
		Kind:      snap.Kind,
		At:        time.Now(),
		Snapshot:  snap,
		Lifecycle: &Lifecycle{Status: status},
	}
}

// NewEventSignal constructs an Event signal carrying one stream line.
// Closing=true on the final event before terminal lifecycle.
func NewEventSignal(d Daemon, line string, closing bool) Signal {
	snap := d.Snapshot()
	return Signal{
		DaemonID: snap.ID,
		Kind:     snap.Kind,
		At:       time.Now(),
		Snapshot: snap,
		Event:    &Event{Line: line, Closing: closing},
	}
}
