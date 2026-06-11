package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/johnny1110/evva/pkg/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/llm"
)

// These tests are the v2.3 acceptance gate: a downstream host owns the persona
// catalog — registering an in-code persona, loading on-disk personas, driving
// the /profile picker, and switching personas — using only pkg/* imports.
// This file imports zero internal/*.

// seedDiskAgent writes a minimal valid on-disk persona under
// <home>/agents/<name>/ (system_prompt.md + tools.yml + meta.yml).
func seedDiskAgent(t *testing.T, home, name string, as []string) {
	t.Helper()
	dir := filepath.Join(home, "agents", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	write := func(file, body string) {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
	write("system_prompt.md", "You are "+name+", a test persona.")
	write("tools.yml", "active: []\ndeferred: []\n")
	asYaml := ""
	for _, a := range as {
		asYaml += "  - " + a + "\n"
	}
	write("meta.yml", "as:\n"+asYaml+"when_to_use: a disk-loaded test persona\n")
}

func personaNames(defs []agent.AgentDefinition) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}

func hasPersona(defs []agent.AgentDefinition, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

func TestDownstream_PersonaRegistry(t *testing.T) {
	home := t.TempDir()
	seedDiskAgent(t, home, "disk-persona", []string{"main", "subagent"})

	reg, warns := agent.BuildAgentRegistry(home)
	if len(warns) != 0 {
		t.Fatalf("unexpected load warnings: %v", warns)
	}

	// A persona authored in the host's own Go code.
	reg.Register(agent.AgentDefinition{
		Name:         "nono",
		WhenToUse:    "financial questions",
		As:           []string{"main", "subagent"},
		InjectMemory: true,
		SystemPrompt: "You are nono, a financial manager.",
	})

	main := reg.ListMain()
	for _, want := range []string{"evva", "nono", "disk-persona"} {
		if !hasPersona(main, want) {
			t.Errorf("ListMain missing %q; got %v", want, personaNames(main))
		}
	}

	sub := reg.ListSubagent()
	for _, want := range []string{"nono", "disk-persona", "explore", "general-purpose"} {
		if !hasPersona(sub, want) {
			t.Errorf("ListSubagent missing %q; got %v", want, personaNames(sub))
		}
	}

	got, ok := reg.Get("nono")
	if !ok {
		t.Fatal("Get(nono) not found")
	}
	if got.SystemPrompt != "You are nono, a financial manager." {
		t.Errorf("round-trip SystemPrompt = %q", got.SystemPrompt)
	}
	if !got.IsMain() || !got.IsSubagent() {
		t.Errorf("nono should be main+subagent; As=%v", got.As)
	}

	// LoadDiskAgents returns the disk persona (with its body) and no built-ins.
	disk, _ := agent.LoadDiskAgents(home)
	if !hasPersona(disk, "disk-persona") {
		t.Errorf("LoadDiskAgents missing disk-persona; got %v", personaNames(disk))
	}
	if hasPersona(disk, "evva") {
		t.Errorf("LoadDiskAgents should not include built-ins; got %v", personaNames(disk))
	}
}

func TestDownstream_PersonaPickerAndSwitch(t *testing.T) {
	const providerName = "persona_stub"
	if !llm.DefaultRegistry().Has(providerName) {
		err := llm.DefaultRegistry().Register(providerName, func(_ llm.APIConfig, model string, _ ...llm.Option) (llm.Client, error) {
			return &stubClient{name: providerName, model: model}, nil
		})
		if err != nil {
			t.Fatalf("register provider: %v", err)
		}
	}

	home := t.TempDir()
	seedDiskAgent(t, home, "disk-persona", []string{"main", "subagent"})

	cfg, err := config.Load(config.LoadOptions{AppName: "persona_test", AppHome: home, WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.LLMProviderConfig[providerName] = config.APIConfig{ApiURL: "http://stub", ApiSecret: "fake"}
	cfg.DefaultProvider = constant.LLMProvider{Name: providerName, Models: []constant.Model{constant.Model("stub-model")}}
	cfg.DefaultModel = constant.Model("stub-model")

	reg, _ := agent.BuildAgentRegistry(home)
	reg.Register(agent.AgentDefinition{
		Name:         "nono",
		WhenToUse:    "financial questions",
		As:           []string{"main"},
		SystemPrompt: "You are nono.",
	})

	// Resolve the initial profile through the public path — no internal types.
	prof, _, err := agent.ResolveMainProfile(cfg, reg, "evva")
	if err != nil {
		t.Fatalf("ResolveMainProfile: %v", err)
	}

	ag, err := agent.NewWithProfile(prof,
		agent.WithConfig(cfg),
		agent.WithPersonaRegistry(reg),
		agent.WithPersona("evva"),
		agent.WithPermissionMode(agent.PermissionBypass),
	)
	if err != nil {
		t.Fatalf("NewWithProfile: %v", err)
	}
	t.Cleanup(ag.Shutdown) // releases the agent log file — Windows can't delete it open

	// The /profile picker enumerates the registry's main personas.
	picker := ag.ListMainProfiles()
	have := map[string]bool{}
	for _, p := range picker {
		have[p.Name] = true
	}
	for _, want := range []string{"evva", "nono", "disk-persona"} {
		if !have[want] {
			t.Errorf("/profile picker missing %q; got %v", want, picker)
		}
	}

	// Switching to a host-registered persona rebuilds the agent under it.
	if err := ag.SwitchProfile("nono"); err != nil {
		t.Fatalf("SwitchProfile(nono): %v", err)
	}
	if got := ag.ProfileName(); got != "nono" {
		t.Errorf("ProfileName after switch = %q, want nono", got)
	}
}
