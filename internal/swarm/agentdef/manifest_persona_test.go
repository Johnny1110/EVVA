package agentdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifestFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "evva-swarm.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadManifest_PersonaMember(t *testing.T) {
	p := writeManifestFile(t, `
name: team
leader:
  agent: lead
workers:
  - agent: dev-a
    model: m-dir
    effort: high
    when_to_use: "dir specialist"
  - persona: helper
    model: m-persona
    effort: ultra
    when_to_use: "resident engineer"
    budget_tokens: -1
`)
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	dir, per := m.Workers[0], m.Workers[1]
	if dir.FromPersona || dir.Model != "m-dir" || dir.Effort != "high" || dir.WhenToUse != "dir specialist" {
		t.Fatalf("dir member overrides wrong: %+v", dir)
	}
	if !per.FromPersona || per.Agent != "helper" || per.Model != "m-persona" || per.Effort != "ultra" || per.WhenToUse != "resident engineer" || per.BudgetTokens != -1 {
		t.Fatalf("persona member wrong: %+v", per)
	}
}

func TestLoadManifest_PersonaLeader(t *testing.T) {
	p := writeManifestFile(t, "leader:\n  persona: boss\n")
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Leader.FromPersona || m.Leader.Agent != "boss" {
		t.Fatalf("persona leader wrong: %+v", m.Leader)
	}
}

func TestLoadManifest_MemberSourceValidation(t *testing.T) {
	cases := map[string]string{
		"both keys":  "leader:\n  agent: a\nworkers:\n  - agent: x\n    persona: x\n",
		"no keys":    "leader:\n  agent: a\nworkers:\n  - schedule: {cron: \"* * * * *\"}\n",
		"bad effort": "leader:\n  agent: a\nworkers:\n  - persona: p\n    effort: turbo\n",
		"dup name":   "leader:\n  agent: a\nworkers:\n  - agent: p\n  - persona: p\n",
	}
	for name, body := range cases {
		if _, err := LoadManifest(writeManifestFile(t, body)); err == nil {
			t.Fatalf("%s: want error, got nil", name)
		}
	}
}

func TestWriteManifest_PersonaRoundTrip(t *testing.T) {
	in := Manifest{
		Name:   "team",
		Leader: Member{Agent: "lead"},
		Workers: []Member{
			{Agent: "helper", FromPersona: true, Model: "m1", Effort: "ultra", WhenToUse: "engineer"},
			{Agent: "dev-a"},
		},
	}
	p := filepath.Join(t.TempDir(), "out.yml")
	if err := WriteManifest(p, in); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if strings.Contains(string(raw), "agent: helper") || !strings.Contains(string(raw), "persona: helper") {
		t.Fatalf("persona member must serialize under the persona key:\n%s", raw)
	}
	out, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	got := out.Workers[0]
	if !got.FromPersona || got.Agent != "helper" || got.Model != "m1" || got.Effort != "ultra" || got.WhenToUse != "engineer" {
		t.Fatalf("round-trip lost persona fields: %+v", got)
	}
	if out.Workers[1].FromPersona {
		t.Fatalf("dir member must stay dir-sourced")
	}
}

// TestLoadManifest_ExternalPath lints a real manifest named by env var —
// `EVVA_MANIFEST_PATH=/path/to/evva-swarm.yml go test ./internal/swarm/agentdef -run ExternalPath`.
// Lets an operator validate a downstream swarm file against this parser.
func TestLoadManifest_ExternalPath(t *testing.T) {
	p := os.Getenv("EVVA_MANIFEST_PATH")
	if p == "" {
		t.Skip("EVVA_MANIFEST_PATH not set")
	}
	if _, err := LoadManifest(p); err != nil {
		t.Fatalf("external manifest invalid: %v", err)
	}
}
