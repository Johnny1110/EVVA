package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/swarm"
)

// writeScheduledSwarmFixture is writeSwarmFixture with a manifest-declared
// worker cadence, so the RP-20 tests can tell a manifest seed from a runtime
// override across the service lifecycle.
func writeScheduledSwarmFixture(t *testing.T, name, workerCron string) string {
	t.Helper()
	dir := t.TempDir()
	must := func(p, content string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("evva-swarm.yml",
		"name: "+name+"\nleader:\n  agent: leader\nworkers:\n  - agent: worker\n    schedule:\n      cron: \""+workerCron+"\"\n      prompt: patrol\nsettings:\n  permission_mode: bypass\n  max_iterations: 3\n")
	must("agents/main/leader/system_prompt.md", "You are the leader.")
	must("agents/sub/worker/system_prompt.md", "You are a worker.")
	return dir
}

func scheduleInfo(t *testing.T, svc *Service, id, member string) (swarm.ScheduleOrigin, string, bool) {
	t.Helper()
	ent, ok := svc.entry(id)
	if !ok {
		t.Fatalf("space %q not live", id)
	}
	sch, origin, ok := ent.space.ScheduleInfoFor(member)
	return origin, sch.Cron, ok
}

// RP-20 acceptance, end to end through the service:
//  1. an operator schedule_set survives service death + Reconcile, marked runtime;
//  2. a clear survives a stop/run cycle (tombstone — the manifest seed must not
//     resurrect);
//  3. re-registering the workdir (`evva swarm .`) discards runtime overrides
//     and restores manifest authority.
func TestScheduleDurabilityAcrossServiceLifecycle(t *testing.T) {
	stateDir := t.TempDir()
	appHome := t.TempDir()
	loadCfg := stubLoadConfigInto(appHome)
	dir := writeScheduledSwarmFixture(t, "sched-team", "*/10 * * * *")

	// --- first process: register fresh, then the operator retunes the cadence.
	svc1 := New("127.0.0.1:0")
	svc1.SetStateDir(stateDir)
	svc1.loadConfig = loadCfg
	id, err := svc1.Register(dir, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if origin, cron, ok := scheduleInfo(t, svc1, id, "worker"); !ok || cron != "*/10 * * * *" || origin.Runtime {
		t.Fatalf("fresh register: cron=%q origin=%+v ok=%v, want the manifest seed", cron, origin, ok)
	}
	if err := svc1.SetSchedule(id, "worker", "*/5 * * * *", "tighter patrol"); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	svc1.Stop() // process dies mid-flight

	// --- second process: Reconcile rebuilds — the runtime cadence must hold.
	svc2 := New("127.0.0.1:0")
	svc2.SetStateDir(stateDir)
	svc2.loadConfig = loadCfg
	defer svc2.Stop()
	if err := svc2.Reconcile(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if origin, cron, ok := scheduleInfo(t, svc2, id, "worker"); !ok || cron != "*/5 * * * *" || !origin.Runtime {
		t.Fatalf("after restart: cron=%q origin=%+v ok=%v, want the runtime */5", cron, origin, ok)
	}

	// --- clear, then bounce the space: the tombstone must beat the manifest.
	if err := svc2.ClearSchedule(id, "worker"); err != nil {
		t.Fatalf("ClearSchedule: %v", err)
	}
	if err := svc2.StopSpace(id); err != nil {
		t.Fatalf("StopSpace: %v", err)
	}
	if _, err := svc2.RunSpace(id); err != nil {
		t.Fatalf("RunSpace: %v", err)
	}
	if _, cron, ok := scheduleInfo(t, svc2, id, "worker"); ok {
		t.Fatalf("after clear+bounce: cron=%q still set, want the tombstone to hold", cron)
	}

	// --- re-register the workdir: operator intent — manifest authority returns.
	if err := svc2.RemoveSpace(id); err != nil {
		t.Fatalf("RemoveSpace: %v", err)
	}
	id2, err := svc2.Register(dir, "")
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if origin, cron, ok := scheduleInfo(t, svc2, id2, "worker"); !ok || cron != "*/10 * * * *" || origin.Runtime {
		t.Fatalf("after re-register: cron=%q origin=%+v ok=%v, want the manifest seed back", cron, origin, ok)
	}
}

// RP-20 §5: an operator schedule edit lands in the RP-17 event log as a
// schedule_change line (the leader path self-audits via its tool_use events).
func TestOperatorScheduleChangeAudited(t *testing.T) {
	svc := New("127.0.0.1:0")
	defer svc.Stop()

	m := stubManifest()
	m.Settings.EventLog = true
	cfg := stubConfig(t)
	id, err := svc.register("sp-sched-audit", "sched-audit", m, stubLoaded(), cfg, false)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := svc.SetSchedule(id, "worker", "*/30 * * * *", "patrol"); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	if err := svc.ClearSchedule(id, "worker"); err != nil {
		t.Fatalf("ClearSchedule: %v", err)
	}

	evDir := filepath.Join(cfg.WorkDir, ".vero", "events")
	var content string
	waitForCond(t, func() bool {
		entries, err := os.ReadDir(evDir)
		if err != nil || len(entries) == 0 {
			return false
		}
		b, _ := os.ReadFile(filepath.Join(evDir, entries[0].Name()))
		content = string(b)
		return strings.Contains(content, `"action":"set"`) && strings.Contains(content, `"action":"clear"`)
	})
	if !strings.Contains(content, `"kind":"schedule_change"`) ||
		!strings.Contains(content, `"member":"worker"`) ||
		!strings.Contains(content, `"source":"operator"`) {
		t.Fatalf("event log lacks the schedule_change line:\n%s", content)
	}
}
