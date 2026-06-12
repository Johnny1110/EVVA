package swarm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/swarm/agentdef"
	"github.com/johnny1110/evva/pkg/agent"
	"github.com/johnny1110/evva/pkg/skill"
)

func personaLoaded(name string, role agentdef.Role) agentdef.Loaded {
	return agentdef.Loaded{
		Def:         agent.AgentDefinition{Name: name},
		FromPersona: true,
		Role:        role,
		Skills:      skill.NewRegistry(),
	}
}

func dirLoaded(name string, role agentdef.Role) agentdef.Loaded {
	return agentdef.Loaded{
		Def:    agent.AgentDefinition{Name: name, SystemPrompt: "You are " + name + ".", Model: stubModel},
		Skills: skill.NewRegistry(),
		Role:   role,
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func TestNewSpace_BuiltinPersonaWorker(t *testing.T) {
	cfg := stubConfig(t)
	ld := personaLoaded("evva", agentdef.RoleWorker)
	ld.Def.WhenToUse = "resident engineer"
	sp, err := NewSpace("s1", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), ld}, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace with persona member: %v", err)
	}
	defer sp.Shutdown()

	def, ok := sp.reg.Get("evva")
	if !ok {
		t.Fatal("composed evva def missing from space registry")
	}
	if !def.LongRunning || !def.AdvertiseSkills {
		t.Fatalf("persona def must be swarm-hardened: %+v", def)
	}
	for _, want := range []string{"# Your place in the swarm", "## Your role: a worker", "## Your long-term memory"} {
		if !strings.Contains(def.PromptSuffix, want) {
			t.Fatalf("PromptSuffix missing section %q", want)
		}
	}
	var found bool
	for _, mv := range sp.Roster.Snapshot() {
		if mv.Name == "evva" {
			found = true
			if mv.WhenToUse != "resident engineer" {
				t.Fatalf("roster when_to_use = %q", mv.WhenToUse)
			}
		}
	}
	if !found {
		t.Fatal("evva not on the roster")
	}
	if dir := agentdef.MemoryDir(sp.Workdir, agentdef.RoleWorker, "evva"); !dirExists(dir) {
		t.Fatalf("member memory dir not created: %s", dir)
	}
	if !sp.isPersonaMember("evva") {
		t.Fatal("space must track persona membership")
	}
}

func TestNewSpace_UnknownPersonaFails(t *testing.T) {
	cfg := stubConfig(t)
	_, err := NewSpace("s2", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), personaLoaded("ghost", agentdef.RoleWorker)}, nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("want unknown-persona error naming ghost, got %v", err)
	}
}

func TestNewSpace_NonMainPersonaFails(t *testing.T) {
	cfg := stubConfig(t)
	_, err := NewSpace("s3", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), personaLoaded("explore", agentdef.RoleWorker)}, nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "main-tier") {
		t.Fatalf("want non-main-tier error, got %v", err)
	}
}

func TestMemberSkillRegistry_PersonaLayers(t *testing.T) {
	cfg := stubConfig(t)
	write := func(root, name, title string) {
		t.Helper()
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+" "+title+"\nbody"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(cfg.AppHomeSkillsDir, "alpha", "persona version")
	write(agentdef.SharedSkillsDir(cfg.WorkDir), "alpha", "shared version")
	write(agentdef.SharedSkillsDir(cfg.WorkDir), "beta", "shared only")
	write(agentdef.SkillsDir(cfg.WorkDir, agentdef.RoleWorker, "evva"), "alpha", "member version")

	sp, err := NewSpace("s4", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), personaLoaded("evva", agentdef.RoleWorker)}, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Shutdown()

	reg := sp.memberSkillRegistry(true, agentdef.RoleWorker, "evva")
	byName := map[string]skill.SkillMeta{}
	for _, m := range reg.List() {
		byName[m.Name] = m
	}
	if byName["alpha"].Description != "member version" {
		t.Fatalf("member dir must win: %q", byName["alpha"].Description)
	}
	if _, ok := byName["beta"]; !ok {
		t.Fatal("shared skill must load for persona members")
	}
}
