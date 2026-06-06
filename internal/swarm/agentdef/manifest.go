package agentdef

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest is a parsed evva-swarm.yml: the swarm name, its workdir, the leader,
// the workers, and space-wide settings. No replicas — every member name must be
// unique within the space.
type Manifest struct {
	Name     string
	Workdir  string
	Leader   Member
	Workers  []Member
	Settings Settings
}

// Member names an agent definition under agents/{main,sub}/{agent}/, with an
// optional timer schedule. A manifest schedule is authoritative over the agent's
// own profile.yml (RP-7 §3.7): the whole team's cadence lives in one
// version-controlled file rather than scattered across each profile.
type Member struct {
	Agent    string
	Schedule *Schedule // nil when the manifest declares none for this member
}

// Settings are space-wide knobs from the manifest.
type Settings struct {
	PermissionMode string
	MaxIterations  int
}

// scheduleYml is the on-disk schedule block shared by the manifest's leader and
// workers (and mirrored by profile.yml). Exactly one of cron/every is set.
type scheduleYml struct {
	Cron   string `yaml:"cron"`
	Every  string `yaml:"every"`
	Prompt string `yaml:"prompt"`
}

// manifestYml is the on-disk schema for evva-swarm.yml (design §4.4).
type manifestYml struct {
	Name    string `yaml:"name"`
	Workdir string `yaml:"workdir"`
	Leader  struct {
		Agent    string       `yaml:"agent"`
		Schedule *scheduleYml `yaml:"schedule"`
	} `yaml:"leader"`
	Workers []struct {
		Agent    string       `yaml:"agent"`
		Schedule *scheduleYml `yaml:"schedule"`
	} `yaml:"workers"`
	Settings struct {
		PermissionMode string `yaml:"permission_mode"`
		MaxIterations  int    `yaml:"max_iterations"`
	} `yaml:"settings"`
}

// parseScheduleYml turns an optional on-disk schedule block into a *Schedule,
// validating the cron at load time (a bad spec fails the whole manifest, not the
// first tick). nil block → nil schedule.
func parseScheduleYml(y *scheduleYml) (*Schedule, error) {
	if y == nil {
		return nil, nil
	}
	s, err := parseSchedule(y.Cron, y.Every)
	if err != nil {
		return nil, err
	}
	s.Prompt = y.Prompt
	return &s, nil
}

// LoadManifest reads and validates an evva-swarm.yml.
func LoadManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("agentdef: read manifest: %w", err)
	}
	var y manifestYml
	if err := yaml.Unmarshal(b, &y); err != nil {
		return Manifest{}, fmt.Errorf("agentdef: parse manifest %s: %w", path, err)
	}

	leaderSched, err := parseScheduleYml(y.Leader.Schedule)
	if err != nil {
		return Manifest{}, fmt.Errorf("agentdef: manifest leader %q schedule: %w", y.Leader.Agent, err)
	}
	m := Manifest{
		Name:     y.Name,
		Workdir:  y.Workdir,
		Leader:   Member{Agent: y.Leader.Agent, Schedule: leaderSched},
		Settings: Settings{PermissionMode: y.Settings.PermissionMode, MaxIterations: y.Settings.MaxIterations},
	}
	for _, w := range y.Workers {
		ws, err := parseScheduleYml(w.Schedule)
		if err != nil {
			return Manifest{}, fmt.Errorf("agentdef: manifest worker %q schedule: %w", w.Agent, err)
		}
		m.Workers = append(m.Workers, Member{Agent: w.Agent, Schedule: ws})
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// validate enforces a leader and unique non-empty member names (leader +
// workers) — no replicas (design decision ⑦). The space name is OPTIONAL
// (Docker-style): when the manifest omits it, the service assigns one (an
// explicit `--name`, else a generated handle). So name is NOT validated here.
func (m Manifest) validate() error {
	if strings.TrimSpace(m.Leader.Agent) == "" {
		return fmt.Errorf("agentdef: manifest: leader.agent is required")
	}
	seen := map[string]bool{m.Leader.Agent: true}
	for i, w := range m.Workers {
		if strings.TrimSpace(w.Agent) == "" {
			return fmt.Errorf("agentdef: manifest: workers[%d].agent is empty", i)
		}
		if seen[w.Agent] {
			return fmt.Errorf("agentdef: manifest: duplicate agent name %q (no replicas — give each member a distinct name)", w.Agent)
		}
		seen[w.Agent] = true
	}
	return nil
}
