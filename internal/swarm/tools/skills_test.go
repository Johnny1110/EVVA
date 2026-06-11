package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/swarm"
)

// startedSupervisor wires AND starts a supervisor on a realSpace — skill_publish
// needs live run loops because PublishSharedSkill fans a reload out to every
// member (a wire-only NewSupervisor has no members registered to reload).
func startedSupervisor(t *testing.T, sp *swarm.SwarmSpace) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	sup := swarm.NewSupervisor(sp)
	sup.Start(ctx)
	t.Cleanup(func() {
		cancel()
		sup.Wait()
	})
}

// RP-26 Part B tool surface: skill_publish writes the space-shared dir (and
// only there), demands a description, and routes "name taken" into explicit
// overwrite guidance instead of a dead end.
func TestSkillPublishTool(t *testing.T) {
	sp := realSpace(t)
	startedSupervisor(t, sp)
	tool := newSkillPublish(leaderMC(sp))

	res := exec(t, tool, `{"name":"review-format","description":"the five sections every review needs","body":"1) PnL 2) risk 3) ..."}`)
	if res.IsError {
		t.Fatalf("skill_publish: %s", res.Content)
	}
	if !strings.Contains(res.Content, `Published shared skill "review-format"`) ||
		!strings.Contains(res.Content, "next run boundary") {
		t.Errorf("result should confirm the publish + when it lands: %s", res.Content)
	}
	shared := sp.SharedSkills()
	if len(shared) != 1 || shared[0].Name != "review-format" || !strings.Contains(shared[0].Description, "five sections") {
		t.Fatalf("SharedSkills() = %+v, want the published skill", shared)
	}
	// The leader's publish never touches a member's private skills dir.
	for _, m := range []string{"leader", "worker-a", "worker-b"} {
		if skills, err := sp.MemberSkills(m); err != nil || len(skills) != 0 {
			t.Errorf("%s private skills = %v (err %v), want none — publish writes ONLY the shared dir", m, skills, err)
		}
	}

	// Same name again: refused with overwrite guidance; overwrite:true republishes.
	r := exec(t, tool, `{"name":"review-format","description":"v2","body":"now six"}`)
	if !r.IsError || !strings.Contains(r.Content, "overwrite:true") {
		t.Errorf("duplicate publish should point at overwrite:true, got: %s", r.Content)
	}
	r = exec(t, tool, `{"name":"review-format","description":"v2","body":"now six","overwrite":true}`)
	if r.IsError || !strings.Contains(r.Content, "Republished") {
		t.Errorf("overwrite republish = %+v, want Republished confirmation", r)
	}
	if shared := sp.SharedSkills(); len(shared) != 1 || shared[0].Description != "v2" {
		t.Errorf("SharedSkills() after overwrite = %+v, want the v2 line", shared)
	}

	// Input discipline: name and description are non-negotiable.
	if r := exec(t, tool, `{"name":"  ","description":"d","body":"b"}`); !r.IsError {
		t.Error("blank name should be rejected")
	}
	if r := exec(t, tool, `{"name":"no-desc","description":" ","body":"b"}`); !r.IsError || !strings.Contains(r.Content, "description") {
		t.Errorf("missing description should be rejected with the why, got: %s", r.Content)
	}
	if r := exec(t, tool, `{"name":"sub/dir","description":"d","body":"b"}`); !r.IsError {
		t.Error("path-separator name should be rejected")
	}
}
