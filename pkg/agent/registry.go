package agent

import (
	agent_impl "github.com/johnny1110/evva/internal/agent"
)

// AgentRegistry is the public catalog of personas the runtime knows about —
// Go-defined built-ins (evva, explore, general-purpose, plan) merged with any
// in-code or on-disk personas you add. Pass it to an agent via
// WithPersonaRegistry; the /profile picker (Agent.ListMainProfiles) and the
// Agent tool's subagent catalog resolve through it.
type AgentRegistry struct {
	inner *agent_impl.AgentRegistry
}

// NewAgentRegistry returns an empty registry. Most callers want
// BuildAgentRegistry, which pre-populates the built-ins + on-disk personas.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{inner: agent_impl.NewAgentRegistry()}
}

// BuildAgentRegistry assembles the runtime catalog: built-ins first, then
// on-disk personas under <evvaHome>/agents/. A disk persona that collides with
// a built-in name is skipped (built-ins win); the returned warnings describe
// any that failed to load. Never returns an error — a missing or malformed
// catalog degrades gracefully.
func BuildAgentRegistry(evvaHome string) (*AgentRegistry, []string) {
	inner, warns := agent_impl.BuildAgentRegistry(evvaHome)
	return &AgentRegistry{inner: inner}, warningStrings(warns)
}

// LoadDiskAgents reads the on-disk personas under <evvaHome>/agents/ (no
// built-ins), returning each as a public AgentDefinition plus any load
// warnings. Useful for inspecting an agents directory before building a full
// registry.
func LoadDiskAgents(evvaHome string) ([]AgentDefinition, []string) {
	specs, warns := agent_impl.LoadDiskAgents(evvaHome)
	return definitionsFromSpecs(specs), warningStrings(warns)
}

// Register adds (or overwrites) a persona. A definition whose Name matches an
// existing entry replaces it.
func (r *AgentRegistry) Register(def AgentDefinition) {
	r.inner.Register(agent_impl.DefinitionFromSpec(def.toSpec()))
}

// Get returns the persona registered under name (case-insensitive). The
// returned definition's SystemPrompt is empty for built-in personas, whose
// prompts are assembled internally.
func (r *AgentRegistry) Get(name string) (AgentDefinition, bool) {
	spec, ok := r.inner.GetSpec(name)
	if !ok {
		return AgentDefinition{}, false
	}
	return definitionFromSpec(spec), true
}

// ListMain returns every persona selectable via /profile, sorted by name.
func (r *AgentRegistry) ListMain() []AgentDefinition {
	return definitionsFromSpecs(r.inner.ListMainSpecs())
}

// ListSubagent returns every persona invokable via the Agent tool, sorted by
// name.
func (r *AgentRegistry) ListSubagent() []AgentDefinition {
	return definitionsFromSpecs(r.inner.ListSubagentSpecs())
}

func definitionsFromSpecs(specs []agent_impl.AgentSpec) []AgentDefinition {
	out := make([]AgentDefinition, len(specs))
	for i, s := range specs {
		out[i] = definitionFromSpec(s)
	}
	return out
}

// warningStrings stringifies a slice of load warnings (loader.Warning, which
// implements error) without naming the internal type — the type parameter is
// inferred, so this file imports no internal/agent/loader.
func warningStrings[T error](warns []T) []string {
	if len(warns) == 0 {
		return nil
	}
	out := make([]string, len(warns))
	for i, w := range warns {
		out[i] = w.Error()
	}
	return out
}
