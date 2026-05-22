package agent

import (
	agent_impl "github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/event"
	pubtoolset "github.com/johnny1110/evva/pkg/toolset"
	"github.com/johnny1110/evva/pkg/tools"
)

// Option mutates an Agent during construction. Downstream callers build
// options via the public With* functions in this package; the internal
// Option type is aliased here so options compose with the bundled cmd/evva
// wiring without duplicate type definitions.
type Option = agent_impl.Option

// WithSink installs the event consumer the agent emits into. nil sinks
// become event.Discard at emit time; pass event.Multi{...} to fan out to
// several consumers (e.g. a TUI plus a JSON-over-stdout bridge).
func WithSink(s event.Sink) Option {
	return agent_impl.WithSink(s)
}

// WithConfig installs the runtime configuration the agent reads from.
// Subagents inherit the parent's config; downstream apps that run
// multiple agents with different AppHome dirs pass distinct *config.Config
// pointers per agent.
//
// Omitting WithConfig boots the agent against config.Get() — the
// historical singleton — so cmd/evva and existing callers don't need to
// change.
func WithConfig(cfg *config.Config) Option {
	return agent_impl.WithConfig(cfg)
}

// WithMaxIterations overrides the agent's loop cap. Pass 0 to keep the
// cfg-derived default. Values in (0, 2) are clamped to 2.
func WithMaxIterations(n int) Option {
	return agent_impl.WithMaxIterations(n)
}

// WithName sets a human-readable label on the agent. Surfaced in logs
// and in subagent panels.
func WithName(name string) Option {
	return agent_impl.WithName(name)
}

// WithStream toggles streaming completions. Overrides the Profile.Stream
// field; useful for tests and downstream apps that want to force the
// buffered or chunked path without rebuilding the profile.
func WithStream(stream bool) Option {
	return agent_impl.WithStream(stream)
}

// WithCustomTool registers a downstream-authored tool on the
// pkg/toolset.DefaultRegistry and adds it to the agent's active list.
// The factory receives the agent's pkg/tools.State at build time so the
// tool can read Config() and Workdir().
//
// Registration is idempotent across agents — calling WithCustomTool with
// the same name in two New calls registers the factory once and reuses
// it.
func WithCustomTool(name tools.ToolName, factory pubtoolset.ToolFactory) Option {
	return agent_impl.WithCustomTool(name, factory)
}

