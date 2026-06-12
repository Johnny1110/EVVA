package agentdef

import (
	"testing"
)

// testdata/agents/ has main/leader + sub/{frontend-dev,backend-dev}.
func TestBuildAll_PersonaMemberNeedsNoDir(t *testing.T) {
	m := Manifest{
		Leader: Member{Agent: "leader"},
		Workers: []Member{
			{Agent: "ghost-persona", FromPersona: true, Model: "m1", Effort: "ultra", WhenToUse: "engineer",
				PermissionMode: "bypass"},
		},
	}
	loaded, _, err := NewLoader().BuildAll("testdata", m)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want leader + persona, got %d", len(loaded))
	}
	p := loaded[1]
	if !p.FromPersona || p.Def.Name != "ghost-persona" {
		t.Fatalf("persona Loaded wrong: %+v", p)
	}
	if p.Def.Model != "m1" || p.Effort != "ultra" || p.Def.WhenToUse != "engineer" || p.PermissionMode != "bypass" {
		t.Fatalf("manifest fields not carried: %+v", p)
	}
	if p.Skills == nil {
		t.Fatalf("persona Loaded must carry a non-nil (empty) skill registry")
	}
}

func TestBuildAll_DirMemberManifestOverrides(t *testing.T) {
	m := Manifest{
		Leader: Member{Agent: "leader"},
		Workers: []Member{
			{Agent: "frontend-dev", Model: "override-model", Effort: "low", WhenToUse: "override desc"},
		},
	}
	loaded, _, err := NewLoader().BuildAll("testdata", m)
	if err != nil {
		t.Fatal(err)
	}
	w := loaded[1]
	if w.Def.Model != "override-model" || w.Effort != "low" || w.Def.WhenToUse != "override desc" {
		t.Fatalf("manifest must override profile.yml: model=%q effort=%q wtu=%q", w.Def.Model, w.Effort, w.Def.WhenToUse)
	}
}
