package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/skill"
)

func writeCatalogSkill(t *testing.T, root, name, title string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+" "+title+"\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSkillCatalog_LayersExtraDirs(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	cfg, err := config.Load(config.LoadOptions{AppName: "t", AppHome: home, WorkDir: work})
	if err != nil {
		t.Fatal(err)
	}
	writeCatalogSkill(t, cfg.AppHomeSkillsDir, "alpha", "home version")
	shared, member := t.TempDir(), t.TempDir()
	writeCatalogSkill(t, shared, "alpha", "shared version")
	writeCatalogSkill(t, shared, "beta", "shared only")
	writeCatalogSkill(t, member, "alpha", "member version")

	reg := LoadSkillCatalog(cfg, shared, member)

	byName := map[string]skill.SkillMeta{}
	for _, m := range reg.List() {
		byName[m.Name] = m
	}
	if byName["alpha"].Description != "member version" {
		t.Fatalf("member dir must win, got %q", byName["alpha"].Description)
	}
	if _, ok := byName["beta"]; !ok {
		t.Fatalf("shared-only skill must load")
	}
	bundled := false
	for _, m := range reg.List() {
		if m.Source == skill.SourceBundled {
			bundled = true
		}
	}
	if !bundled {
		t.Fatalf("bundled skills must still be present (lowest tier)")
	}
}
