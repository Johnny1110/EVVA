package agent

import "github.com/johnny1110/evva/internal/agent/event"

// Option mutates an Agent during construction. Options are applied after the
// profile is materialized but before the LLM client is initialized so any
// option can influence either side without ordering surprises.
type Option func(*Agent)

// DefaultMaxIterations is the loop's safety cap. Hitting it emits a
// KindIterLimit event and pauses the agent; the caller may invoke
// Continue(ctx) to keep going.
const DefaultMaxIterations = 25

// WithSink installs the event consumer. nil sinks become event.Discard at
// emit-time; pass event.Multi{...} to fan out to several consumers.
func WithSink(s event.Sink) Option {
	return func(a *Agent) {
		a.sink = s
	}
}

// WithMaxIterations overrides DefaultMaxIterations. Pass 0 to use the
// default. Negative values are clamped to 1 (single-turn).
func WithMaxIterations(n int) Option {
	return func(a *Agent) {
		switch {
		case n == 0:
			a.maxIters = DefaultMaxIterations
		case n < 0:
			a.maxIters = 1
		default:
			a.maxIters = n
		}
	}
}

// asSubagent marks the agent as a subagent of parentID. Subagents:
//   - cannot spawn further subagents (meta.Agent rejects from a subagent),
//   - have their events tagged with ParentID=parentID via event.BubbleUp.
//
// Unexported on purpose — only the AGENT tool's Execute should set this.
func asSubagent(parentID string) Option {
	return func(a *Agent) {
		a.parent = parentID
	}
}
