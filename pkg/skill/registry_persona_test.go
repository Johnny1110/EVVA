package skill

import (
	"testing"
)

func TestLoadDir_OverridesAndLabels(t *testing.T) {
	home, extra := t.TempDir(), t.TempDir()
	writeSkill(t, home, "alpha", "# alpha home version\nbody")
	writeSkill(t, extra, "alpha", "# alpha swarm version\nbody")
	writeSkill(t, extra, "beta", "# beta swarm only\nbody")

	r, err := LoadRegistry(home, "")
	if err != nil {
		t.Fatal(err)
	}
	r.LoadDir(extra, SourceSwarm)

	var alpha, beta *SkillMeta
	for _, m := range r.List() {
		m := m
		switch m.Name {
		case "alpha":
			alpha = &m
		case "beta":
			beta = &m
		}
	}
	if alpha == nil || beta == nil {
		t.Fatalf("want alpha+beta in registry, got %v", r.List())
	}
	if alpha.Source != SourceSwarm || alpha.Description != "swarm version" {
		t.Fatalf("alpha not overridden by swarm dir: %+v", *alpha)
	}
	if len(r.Warnings) == 0 {
		t.Fatalf("override should record a shadow warning")
	}
}
