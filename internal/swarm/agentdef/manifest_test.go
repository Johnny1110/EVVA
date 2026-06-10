package agentdef

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadManifestHappy(t *testing.T) {
	m, err := LoadManifest(filepath.Join("testdata", "evva-swarm.yml"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Name != "test-eng-team" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Leader.Agent != "leader" {
		t.Errorf("Leader = %q", m.Leader.Agent)
	}
	want := []Member{{Agent: "backend-dev"}, {Agent: "frontend-dev"}}
	if !reflect.DeepEqual(m.Workers, want) {
		t.Errorf("Workers = %+v, want %+v", m.Workers, want)
	}
	if m.Settings.PermissionMode != "default" || m.Settings.MaxIterations != 50 {
		t.Errorf("Settings = %+v", m.Settings)
	}
}

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "evva-swarm.yml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadManifestMissingFile(t *testing.T) {
	if _, err := LoadManifest(filepath.Join("testdata", "nope.yml")); err == nil {
		t.Fatal("want error for missing manifest")
	}
}

func TestLoadManifestDuplicateWorker(t *testing.T) {
	p := writeManifest(t, `
name: dup
leader:
  agent: leader
workers:
  - agent: eng
  - agent: eng
`)
	_, err := LoadManifest(p)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v, want a duplicate-name error", err)
	}
}

func TestLoadManifestWorkerCollidesWithLeader(t *testing.T) {
	p := writeManifest(t, `
name: dup
leader:
  agent: leader
workers:
  - agent: leader
`)
	if _, err := LoadManifest(p); err == nil {
		t.Fatal("want error when a worker reuses the leader's name")
	}
}

func TestLoadManifestMissingLeader(t *testing.T) {
	p := writeManifest(t, `
name: noleader
workers:
  - agent: eng
`)
	if _, err := LoadManifest(p); err == nil {
		t.Fatal("want error when leader.agent is missing")
	}
}

func TestLoadManifestEmptyWorkerName(t *testing.T) {
	p := writeManifest(t, `
name: empty
leader:
  agent: leader
workers:
  - agent: ""
`)
	if _, err := LoadManifest(p); err == nil {
		t.Fatal("want error for an empty worker name")
	}
}

// A missing name is now ACCEPTED (Docker-style): the service assigns a handle
// (--name > manifest name > generated), so the manifest no longer requires one.
func TestLoadManifestMissingNameIsAllowed(t *testing.T) {
	p := writeManifest(t, `
leader:
  agent: leader
`)
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatalf("a nameless manifest should load, got %v", err)
	}
	if m.Name != "" {
		t.Fatalf("Name = %q, want empty (service assigns the handle)", m.Name)
	}
}

func TestManifestBudgetFieldsRoundTrip(t *testing.T) {
	p := writeManifest(t, `
name: budgeted
leader:
  agent: lead
  budget_tokens: -1
workers:
  - agent: w1
    budget_tokens: 250000
  - agent: w2
settings:
  daily_budget_tokens: 1000000
  budget_stay_frozen: true
`)
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Settings.DailyBudgetTokens != 1000000 || !m.Settings.BudgetStayFrozen {
		t.Errorf("Settings = %+v", m.Settings)
	}
	if m.Leader.BudgetTokens != -1 {
		t.Errorf("leader budget = %d, want -1", m.Leader.BudgetTokens)
	}
	if m.Workers[0].BudgetTokens != 250000 || m.Workers[1].BudgetTokens != 0 {
		t.Errorf("worker budgets = %d/%d, want 250000/0", m.Workers[0].BudgetTokens, m.Workers[1].BudgetTokens)
	}

	// WriteManifest must carry the budget fields back out.
	out := filepath.Join(t.TempDir(), "evva-swarm.yml")
	if err := WriteManifest(out, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	m2, err := LoadManifest(out)
	if err != nil {
		t.Fatalf("re-LoadManifest: %v", err)
	}
	if m2.Settings.DailyBudgetTokens != 1000000 || !m2.Settings.BudgetStayFrozen ||
		m2.Leader.BudgetTokens != -1 || m2.Workers[0].BudgetTokens != 250000 {
		t.Errorf("round-trip lost budget fields: %+v / leader %d / w1 %d",
			m2.Settings, m2.Leader.BudgetTokens, m2.Workers[0].BudgetTokens)
	}
}
