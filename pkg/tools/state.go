package tools

import "github.com/johnny1110/evva/pkg/config"

// State is the narrow surface a tool factory receives from the registry
// when building a per-agent instance. Carries only what downstream tools
// can rely on across evva versions — the runtime Config (TavilyAPIKey,
// FetchMaxBytes, AppHome, custom fields) and the agent's working dir.
//
// Evva-internal tools (meta/AGENT, mode/EnterPlanMode, todo/Write, fs/*)
// need richer state — the subagent spawner, the plan controller, the
// ReadTracker, etc. Those factories type-assert State to the concrete
// internal toolset.ToolState. Downstream-authored tools should not
// reach for that assertion; rely on Config() instead.
type State interface {
	// Config returns the runtime configuration the agent was built with.
	// Tools that need runtime settings read fields off this pointer at
	// Execute time so the /config form's mutations take effect.
	// Returns nil when no Config was installed (rare; only in tests or
	// narrow harnesses).
	Config() *config.Config

	// Workdir returns the process working directory the agent captured
	// at construction. Convenience over reading Config().WorkDir so
	// downstream tools don't have to nil-check the Config pointer.
	// Returns "" when neither WithConfig nor a process workdir is set.
	Workdir() string
}
