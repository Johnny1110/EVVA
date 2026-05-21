package toolset

import (
	"fmt"
	"sort"
	"sync"

	"github.com/johnny1110/evva/pkg/tools"
)

// ToolFactory builds one tool instance against the supplied state. The
// state parameter exposes the per-agent runtime config and workdir;
// evva-internal factories type-assert it to the concrete
// internal/toolset.ToolState to reach private accessors (ReadTracker,
// subagent spawner, etc.).
//
// Factories must not retain the State beyond the call — the tool itself
// either captures the late-binding closures it needs (e.g. the AGENT
// tool's spawner lookup) or holds the cfg / workdir values it received.
type ToolFactory func(state tools.State) (tools.Tool, error)

// Registry maps tool names to factories. Downstream apps register their
// own tools by calling DefaultRegistry().Register at startup before the
// first agent is constructed; evva's bundled tools are registered by
// internal/toolset during init.
//
// Registry is safe for concurrent use. Register fails on duplicate names —
// silently overwriting would let a typo route an LLM-facing name to the
// wrong implementation. Use MustRegister at init time when a duplicate
// is a programming bug.
type Registry struct {
	mu        sync.RWMutex
	factories map[tools.ToolName]ToolFactory
}

// NewRegistry returns an empty registry. Most callers want DefaultRegistry
// instead — it's the one evva's bundled tools register into.
func NewRegistry() *Registry {
	return &Registry{factories: map[tools.ToolName]ToolFactory{}}
}

// Register associates a factory with a tool name. Returns an error if name
// is already registered or factory is nil.
func (r *Registry) Register(name tools.ToolName, factory ToolFactory) error {
	if name == "" {
		return fmt.Errorf("toolset: cannot register empty tool name")
	}
	if factory == nil {
		return fmt.Errorf("toolset: nil factory for %q", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.factories[name]; dup {
		return fmt.Errorf("toolset: duplicate registration for %q", name)
	}
	r.factories[name] = factory
	return nil
}

// MustRegister wraps Register and panics on error. Use only at init time
// where a duplicate or nil factory is a programmer bug.
func (r *Registry) MustRegister(name tools.ToolName, factory ToolFactory) {
	if err := r.Register(name, factory); err != nil {
		panic(err)
	}
}

// Build instantiates the named tool against state. Returns an error for
// unregistered names — there is no silent fallback.
func (r *Registry) Build(name tools.ToolName, state tools.State) (tools.Tool, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("toolset: unknown tool name - %q (not in tool factory)", name)
	}
	return factory(state)
}

// Has reports whether name is registered.
func (r *Registry) Has(name tools.ToolName) bool {
	r.mu.RLock()
	_, ok := r.factories[name]
	r.mu.RUnlock()
	return ok
}

// Names returns every registered name, sorted lexicographically. Useful for
// tests + diagnostic output. Mutating the returned slice is safe.
func (r *Registry) Names() []tools.ToolName {
	r.mu.RLock()
	out := make([]tools.ToolName, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

// DefaultRegistry returns the process-wide registry. Pre-population is the
// caller's responsibility — evva's bundled tools register themselves via
// internal/toolset's init function (which cmd/evva imports transitively
// through internal/agent). Downstream apps consume the same registry to
// register custom tools:
//
//	toolset.DefaultRegistry().Register("my_tool", myFactory)
//
// before the first agent is constructed.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
	})
	return defaultRegistry
}
