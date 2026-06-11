package swarm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnny1110/evva/internal/swarm/agentdef"
	"github.com/johnny1110/evva/pkg/agent"
	"github.com/johnny1110/evva/pkg/skill"
	"github.com/johnny1110/evva/pkg/tools"
)

func countTool(list []tools.ToolName, n tools.ToolName) int {
	c := 0
	for _, tn := range list {
		if tn == n {
			c++
		}
	}
	return c
}

// TestRegisterDefForcesSkills (RP-10-1): the swarm forces AdvertiseSkills=true and
// injects the built-in skill tool on EVERY member, overriding the on-disk profile —
// and without duplicating a skill tool a member already declared.
func TestRegisterDefForcesSkills(t *testing.T) {
	cfg := stubConfig(t)
	loaded := []agentdef.Loaded{
		{ // leader explicitly DISABLES advertise + has no skill tool → both forced on
			Def:    agent.AgentDefinition{Name: "leader", SystemPrompt: "lead", AdvertiseSkills: false, ActiveTools: []tools.ToolName{tools.READ_FILE}, Model: stubModel},
			Skills: skill.NewRegistry(), Role: agentdef.RoleLeader,
		},
		{ // worker already lists the skill tool → must not be duplicated
			Def:    agent.AgentDefinition{Name: "worker", SystemPrompt: "work", ActiveTools: []tools.ToolName{tools.SKILL, tools.READ_FILE}, Model: stubModel},
			Skills: skill.NewRegistry(), Role: agentdef.RoleWorker,
		},
	}
	sp, err := NewSpace("s", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	defer sp.Shutdown()

	for _, name := range []string{"leader", "worker"} {
		def, ok := sp.reg.Get(name)
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if !def.AdvertiseSkills {
			t.Errorf("%s: AdvertiseSkills not forced true", name)
		}
		if c := countTool(def.ActiveTools, tools.SKILL); c != 1 {
			t.Errorf("%s: skill tool count = %d, want exactly 1; tools=%v", name, c, def.ActiveTools)
		}
		if c := countTool(def.ActiveTools, tools.READ_FILE); c != 1 {
			t.Errorf("%s: read tool dropped/duplicated (count %d); tools=%v", name, c, def.ActiveTools)
		}
	}
}

// TestReloadMemberSkills (RP-10-4): once a skill is authored on disk, ReloadMemberSkills
// re-scans the member's dir and the live agent's catalog reflects it — an idle member
// applies it at the next run-loop tick (via the serve boundary drain).
func TestReloadMemberSkills(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("s", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	t.Cleanup(sp.Shutdown) // LIFO: runs AFTER startSup's cancel+Wait, so loops are down first
	sup := startSup(t, sp)

	ag, ok := sp.agentOf("worker-a")
	if !ok {
		t.Fatal("worker-a agent missing")
	}
	if len(ag.Skills()) != 0 {
		t.Fatalf("worker-a should start with no skills; got %v", ag.Skills())
	}

	if err := agentdef.WriteSkill(cfg.WorkDir, agentdef.RoleWorker, "worker-a", "newskill", "a fresh skill", "do the thing"); err != nil {
		t.Fatalf("WriteSkill: %v", err)
	}
	if err := sup.ReloadMemberSkills("worker-a"); err != nil {
		t.Fatalf("ReloadMemberSkills: %v", err)
	}

	waitFor(t, 2*time.Second, "worker-a's live catalog reflects the reload", func() bool {
		for _, s := range ag.Skills() {
			if s.Name == "newskill" {
				return true
			}
		}
		return false
	})

	// A delete + reload removes it again.
	if err := agentdef.RemoveSkill(cfg.WorkDir, agentdef.RoleWorker, "worker-a", "newskill"); err != nil {
		t.Fatalf("RemoveSkill: %v", err)
	}
	if err := sup.ReloadMemberSkills("worker-a"); err != nil {
		t.Fatalf("ReloadMemberSkills (remove): %v", err)
	}
	waitFor(t, 2*time.Second, "worker-a's catalog drops the deleted skill", func() bool {
		return len(ag.Skills()) == 0
	})
}

// RP-26 Part A: a skill dropped into the space-shared dir reaches a member's
// live catalog through the same run-boundary reload, and a member's own
// same-named skill keeps winning over the shared copy.
func TestReloadPicksUpSharedSkills(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("s-shared", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	t.Cleanup(sp.Shutdown)
	sup := startSup(t, sp)

	// Author a shared skill on disk (Part A is User-authored: drop a folder in).
	sharedDir := filepath.Join(agentdef.SharedSkillsDir(cfg.WorkDir), "query-sunday")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "SKILL.md"), []byte("# query-sunday the shared edition\n\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	ag, ok := sp.agentOf("worker-a")
	if !ok {
		t.Fatal("worker-a agent missing")
	}
	if err := sup.ReloadMemberSkills("worker-a"); err != nil {
		t.Fatalf("ReloadMemberSkills: %v", err)
	}
	waitFor(t, 2*time.Second, "worker-a's catalog gains the shared skill", func() bool {
		for _, s := range ag.Skills() {
			if s.Name == "query-sunday" && strings.Contains(s.Description, "shared edition") {
				return true
			}
		}
		return false
	})

	// worker-a authors its own query-sunday → the local copy shadows the shared one.
	if err := agentdef.WriteSkill(cfg.WorkDir, agentdef.RoleWorker, "worker-a", "query-sunday", "the local edition", "local body"); err != nil {
		t.Fatalf("WriteSkill: %v", err)
	}
	if err := sup.ReloadMemberSkills("worker-a"); err != nil {
		t.Fatalf("ReloadMemberSkills (local override): %v", err)
	}
	waitFor(t, 2*time.Second, "worker-a's local copy shadows the shared one", func() bool {
		for _, s := range ag.Skills() {
			if s.Name == "query-sunday" && strings.Contains(s.Description, "local edition") {
				return true
			}
		}
		return false
	})
}

// RP-26 Part B: PublishSharedSkill writes the shared dir and fans the reload
// out to EVERY member's live catalog; overwrite republishes a new version;
// RemoveSharedSkill drops it team-wide. This is the seam both the leader's
// skill_publish tool and the operator's web POST/DELETE ride.
func TestPublishSharedSkillReachesAllMembers(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("s-publish", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	t.Cleanup(sp.Shutdown)
	startSup(t, sp)

	all := []string{"leader", "worker-a", "worker-b"}
	hasSkill := func(member, name, descPart string) bool {
		ag, ok := sp.agentOf(member)
		if !ok {
			return false
		}
		for _, s := range ag.Skills() {
			if s.Name == name && strings.Contains(s.Description, descPart) {
				return true
			}
		}
		return false
	}

	if err := sp.PublishSharedSkill("review-format", "the five sections", "1) PnL 2) risk ...", false); err != nil {
		t.Fatalf("PublishSharedSkill: %v", err)
	}
	for _, m := range all {
		waitFor(t, 2*time.Second, m+" catalog gains the published skill", func() bool {
			return hasSkill(m, "review-format", "five sections")
		})
	}
	if got := sp.SharedSkills(); len(got) != 1 || got[0].Name != "review-format" {
		t.Errorf("SharedSkills() = %v, want the one published skill", got)
	}

	// Republish without overwrite is refused; with overwrite, v2 reaches everyone.
	if err := sp.PublishSharedSkill("review-format", "v2", "now six sections", false); !errors.Is(err, agentdef.ErrSkillExists) {
		t.Fatalf("republish without overwrite: err = %v, want ErrSkillExists", err)
	}
	if err := sp.PublishSharedSkill("review-format", "v2", "now six sections", true); err != nil {
		t.Fatalf("PublishSharedSkill (overwrite): %v", err)
	}
	for _, m := range all {
		waitFor(t, 2*time.Second, m+" catalog picks up v2", func() bool {
			return hasSkill(m, "review-format", "v2")
		})
	}

	// The User's final-arbiter delete drops it from every member again.
	if err := sp.RemoveSharedSkill("review-format"); err != nil {
		t.Fatalf("RemoveSharedSkill: %v", err)
	}
	for _, m := range all {
		waitFor(t, 2*time.Second, m+" catalog drops the removed skill", func() bool {
			ag, ok := sp.agentOf(m)
			return ok && len(ag.Skills()) == 0
		})
	}
}
