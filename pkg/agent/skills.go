package agent

import (
	agent_impl "github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/skill"
)

// LoadSkillCatalog builds the skill catalog a persona would load on its own —
// bundled first-party skills, <appHome>/skills, <workdir>/.evva/skills — then
// overlays extraDirs in order (later wins). Hosts that resolve a persona's
// catalog plus host-specific layers (e.g. a swarm overlaying its shared and
// member-local skill dirs) call this instead of re-implementing the merge.
func LoadSkillCatalog(cfg *config.Config, extraDirs ...string) *skill.Registry {
	if cfg == nil {
		cfg = config.Get()
	}
	return agent_impl.LoadSkillCatalog(cfg, extraDirs...)
}
