package agent

import (
	agent_impl "github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/llm"
)

// ResolveMainProfile builds the initial Profile for a main-tier persona by
// name, ready to pass to NewWithProfile. It auto-loads the skill catalog and
// the EVVA.md / USER_PROFILE.md memory snapshot from cfg, so a host never
// touches the internal memory or skills packages.
//
// reg is the persona catalog (from BuildAgentRegistry, with any Register'd
// personas); a nil registry resolves only the built-in "evva". An empty name
// defaults to "evva". The returned warnings are non-fatal memory-load notes
// (oversize / unreadable files); an unknown or non-main persona name returns
// an error.
//
// Typical use:
//
//	reg, _ := agent.BuildAgentRegistry(cfg.AppHome)
//	reg.Register(myPersona)
//	prof, _, err := agent.ResolveMainProfile(cfg, reg, "nono")
//	ag, _ := agent.NewWithProfile(prof,
//	    agent.WithConfig(cfg),
//	    agent.WithPersonaRegistry(reg),
//	    agent.WithPersona("nono"),
//	    agent.WithSink(myUI),
//	)
func ResolveMainProfile(cfg *config.Config, reg *AgentRegistry, name string, opts ...llm.Option) (Profile, []string, error) {
	if cfg == nil {
		cfg = config.Get()
	}
	var inner *agent_impl.AgentRegistry
	if reg != nil {
		inner = reg.inner
	}
	return agent_impl.ResolveMainProfileAutoMem(cfg, inner, name, opts)
}
