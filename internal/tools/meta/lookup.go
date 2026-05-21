package meta

import "github.com/johnny1110/evva/pkg/tools"

// DeferredLookup is the agent-layer dependency the TOOL_SEARCH tool reads
// to enumerate and describe the deferred tools the current profile permits.
//
// Like SubagentSpawner, this interface lives in meta so the agent layer can
// implement it without causing the cycle that would arise from meta
// importing agent. ToolSearchTool resolves the lookup at Execute time
// (late binding) so the agent can install itself after construction.
type DeferredLookup interface {
	// DeferredNames returns the tool names the profile allows the model to
	// lazy-load. Order is implementation-defined and may vary between calls
	// (e.g. a map iteration); TOOL_SEARCH sorts what it returns to the model.
	DeferredNames() []tools.ToolName

	// Describe returns the LLM-facing metadata for a deferred tool name.
	// Returns an error if the name is unknown to the underlying catalog.
	Describe(name tools.ToolName) (tools.Descriptor, error)
}
