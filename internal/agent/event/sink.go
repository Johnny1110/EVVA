package event

// Sink consumes events emitted by an agent's run loop.
//
// Concurrency: an agent calls Emit serially from a single goroutine
// (the loop's goroutine). Sinks do NOT need internal locking to handle
// events from one agent. Sinks shared across multiple agents (e.g. a
// global logger) must handle concurrent Emit calls themselves.
//
// Emit should be fast — slow sinks block the agent loop. Sinks needing
// network or disk I/O should buffer internally (channel, ring buffer)
// and process asynchronously.
type Sink interface {
	Emit(Event)
}

// Multi fans out one event to many sinks in declared order. A slow sink
// blocks subsequent ones (and the agent loop). This is intentional —
// backpressure beats event loss.
type Multi struct {
	Sinks []Sink
}

// Emit forwards to every contained sink, skipping nil entries.
func (m Multi) Emit(e Event) {
	for _, s := range m.Sinks {
		if s != nil {
			s.Emit(e)
		}
	}
}

// Discard is the no-op sink. Use as the default for tests / silent CLI
// runs where the caller doesn't subscribe to events.
var Discard Sink = discard{}

type discard struct{}

func (discard) Emit(Event) {}

// BubbleUp wraps a parent's Sink so a subagent's events appear in the
// parent's stream with ParentID set to the parent's AgentID. The TUI uses
// this tagging to route nested events into a subagent panel.
//
// BubbleUp does NOT change the subagent's own AgentID — only ParentID.
// The hierarchy is always exactly two layers (subagents cannot spawn
// subagents), so a single rewrite at the boundary is enough.
type BubbleUp struct {
	Parent   Sink
	ParentID string
}

// Emit rewrites the event's ParentID and forwards.
func (b BubbleUp) Emit(e Event) {
	if b.Parent == nil {
		return
	}
	e.ParentID = b.ParentID
	b.Parent.Emit(e)
}

// SinkFunc adapts an ordinary function to the Sink interface — convenient
// for one-off consumers (tests, quick CLI prints).
type SinkFunc func(Event)

// Emit calls the wrapped function.
func (f SinkFunc) Emit(e Event) {
	if f != nil {
		f(e)
	}
}
