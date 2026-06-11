package swarm

import (
	"strings"
	"testing"
	"time"

	"github.com/johnny1110/evva/internal/swarm/agentdef"
)

// RP-7: a leader-set schedule takes effect on the RUNNING loop immediately (not
// only at startMemberLoop), the wake carries the custom prompt + current time,
// and ClearSchedule stops the wakes.
func TestSetScheduleLiveAndClear(t *testing.T) {
	sp, ctls := ctlSpace(t, map[string]agentdef.Role{"worker": agentdef.RoleWorker})
	sup := startSup(t, sp)

	// worker starts unscheduled — idle, no runs.
	time.Sleep(30 * time.Millisecond)
	if got := ctls["worker"].runs.Load(); got != 0 {
		t.Fatalf("unscheduled worker ran %d times, want 0", got)
	}

	if err := sup.SetSchedule("worker", agentdef.Schedule{Every: 20 * time.Millisecond, Prompt: "do the patrol"}); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	waitFor(t, 2*time.Second, "worker wakes on the live schedule", func() bool { return ctls["worker"].runs.Load() >= 2 })

	if p := ctls["worker"].lastPrompt(); !strings.Contains(p, "<system-reminder>currenttime: ") || !strings.Contains(p, "do the patrol") {
		t.Errorf("wake prompt = %q, want currenttime + the custom prompt", p)
	}

	// Clear: the wakes must stop. Allow at most one already in flight at clear.
	if err := sup.ClearSchedule("worker"); err != nil {
		t.Fatalf("ClearSchedule: %v", err)
	}
	n := ctls["worker"].runs.Load()
	time.Sleep(120 * time.Millisecond) // several Every windows
	if got := ctls["worker"].runs.Load(); got > n+1 {
		t.Errorf("worker ran %d more times after clear, want it stopped (was %d)", got-n, n)
	}
	if _, ok := sp.ScheduleFor("worker"); ok {
		t.Errorf("ScheduleFor(worker) still set after ClearSchedule")
	}
}

// RP-7 §3.6: a timer tick that lands while the member is already running is
// SKIPPED, not queued — no catch-up run piles up behind a long task.
func TestScheduledWakeSkippedWhileBusy(t *testing.T) {
	sp, ctls := ctlSpace(t, map[string]agentdef.Role{"busy": agentdef.RoleWorker})
	ctls["busy"].block = true // first run blocks until teardown cancels the ctx
	sup := startSup(t, sp)

	if err := sup.SetSchedule("busy", agentdef.Schedule{Every: 10 * time.Millisecond}); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	waitFor(t, time.Second, "first scheduled wake starts and blocks", func() bool { return ctls["busy"].inFlight.Load() == 1 })

	// Many ticks pass while the member is busy; none may queue a second run.
	time.Sleep(100 * time.Millisecond)
	if got := ctls["busy"].runs.Load(); got != 1 {
		t.Errorf("busy member ran %d times, want exactly 1 (later ticks skipped, not queued)", got)
	}
}

// RP-7 AC#6: a leader-set schedule survives a service restart (runtime.json).
func TestScheduleSetSurvivesRestart(t *testing.T) {
	cfg := stubConfig(t)

	sp1, err := NewSpace("sched-set", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #1: %v", err)
	}
	sup1 := NewSupervisor(sp1) // wires sp1.super; no Start needed to persist
	sch := agentdef.Schedule{Cron: "0 9 * * 1", Prompt: "weekly report"}
	if err := sup1.SetSchedule("worker-a", sch); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	sp1.Shutdown()

	sp2, err := NewSpace("sched-set", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #2: %v", err)
	}
	sp2.Reload()
	defer sp2.Shutdown()

	got, ok := sp2.ScheduleFor("worker-a")
	if !ok || got.Cron != sch.Cron || got.Prompt != sch.Prompt {
		t.Errorf("restored schedule = %+v ok=%v, want %+v", got, ok, sch)
	}
}

// RP-20: editing the manifest takes effect for members whose schedule was
// never touched at runtime. Under the pre-RP-20 whole-map persistence, ANY
// persistRuntime (a freeze, the budget meter) froze the manifest seeds into
// runtime.json and silently overrode later manifest edits on restart.
func TestManifestScheduleEditAppliesAfterRestart(t *testing.T) {
	cfg := stubConfig(t)

	loaded := testLoaded()
	loaded[1].Schedule = &agentdef.Schedule{Cron: "*/10 * * * *", Prompt: "patrol"}

	sp1, err := NewSpace("sched-edit", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #1: %v", err)
	}
	sup1 := NewSupervisor(sp1)
	// An unrelated runtime change persists runtime.json — the old hijack trigger.
	if err := sup1.Freeze("worker-b"); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	sp1.Shutdown()

	// Operator edits the manifest cadence while the service is down.
	edited := testLoaded()
	edited[1].Schedule = &agentdef.Schedule{Cron: "*/5 * * * *", Prompt: "patrol"}

	sp2, err := NewSpace("sched-edit", testManifest(), edited, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #2: %v", err)
	}
	sp2.Reload()
	defer sp2.Shutdown()

	got, origin, ok := sp2.ScheduleInfoFor("worker-a")
	if !ok || got.Cron != "*/5 * * * *" {
		t.Errorf("schedule after manifest edit = %+v ok=%v, want the edited */5 cadence", got, ok)
	}
	if origin.Runtime {
		t.Errorf("origin = %+v, want manifest", origin)
	}
}

// RP-20 §2.5: provenance — a manifest seed reads as manifest; a runtime set
// reads as runtime with its set-instant, and both survive a restart.
func TestScheduleProvenance(t *testing.T) {
	cfg := stubConfig(t)

	loaded := testLoaded()
	loaded[1].Schedule = &agentdef.Schedule{Cron: "*/10 * * * *"}

	sp1, err := NewSpace("sched-prov", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #1: %v", err)
	}
	if _, origin, ok := sp1.ScheduleInfoFor("worker-a"); !ok || origin.Runtime {
		t.Errorf("manifest seed origin = %+v ok=%v, want manifest", origin, ok)
	}

	sup1 := NewSupervisor(sp1)
	if err := sup1.SetSchedule("worker-b", agentdef.Schedule{Cron: "0 9 * * *", Prompt: "daily"}); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	_, origin, ok := sp1.ScheduleInfoFor("worker-b")
	if !ok || !origin.Runtime || origin.SetAt == 0 {
		t.Fatalf("runtime origin = %+v ok=%v, want runtime with a set instant", origin, ok)
	}
	setAt := origin.SetAt
	sp1.Shutdown()

	sp2, err := NewSpace("sched-prov", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #2: %v", err)
	}
	sp2.Reload()
	defer sp2.Shutdown()

	if _, origin, ok := sp2.ScheduleInfoFor("worker-a"); !ok || origin.Runtime {
		t.Errorf("restored manifest origin = %+v ok=%v, want manifest", origin, ok)
	}
	if _, origin, ok := sp2.ScheduleInfoFor("worker-b"); !ok || !origin.Runtime || origin.SetAt != setAt {
		t.Errorf("restored runtime origin = %+v ok=%v, want runtime set at %d", origin, ok, setAt)
	}
}

// RP-20 upgrade: a pre-RP-20 runtime.json (whole schedules map) is imported
// once into per-member rows with provenance recovered by diffing the manifest
// seeds, and the file is rewritten without the legacy field.
func TestLegacyRuntimeScheduleImport(t *testing.T) {
	cfg := stubConfig(t)

	loaded := testLoaded()
	loaded[1].Schedule = &agentdef.Schedule{Cron: "*/10 * * * *", Prompt: "patrol"} // worker-a
	loaded[2].Schedule = &agentdef.Schedule{Cron: "0 8 * * *", Prompt: "report"}    // worker-b

	// A pre-RP-20 file holds the live map at persist time: worker-a was
	// runtime-retuned to */5 (differs from its seed → import as a runtime
	// row); worker-b — though seeded — is absent (it was runtime-CLEARED →
	// import as a tombstone). A seed-equal entry must import as NOTHING; that
	// path is what keeps TestManifestScheduleEditAppliesAfterRestart green.
	legacy := runtimeState{
		Membership: map[string]string{"leader": "active", "worker-a": "active", "worker-b": "active"},
		Schedules: map[string]agentdef.Schedule{
			"worker-a": {Cron: "*/5 * * * *", Prompt: "patrol"},
		},
	}

	sp, err := NewSpace("sched-legacy", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	// Written after NewSpace (which creates .vero/) and before Reload (which
	// reads it) — exactly when a pre-upgrade file would be sitting there.
	writeRuntime(cfg.WorkDir, legacy)
	sp.Reload()
	defer sp.Shutdown()

	// worker-a: the runtime retune wins, with runtime provenance.
	got, origin, ok := sp.ScheduleInfoFor("worker-a")
	if !ok || got.Cron != "*/5 * * * *" || !origin.Runtime {
		t.Errorf("worker-a = %+v origin=%+v ok=%v, want the runtime */5", got, origin, ok)
	}
	// worker-b: cleared-while-legacy → tombstoned, the manifest seed must not resurrect.
	if got, ok := sp.ScheduleFor("worker-b"); ok {
		t.Errorf("worker-b schedule = %+v, want cleared (legacy absence = tombstone)", got)
	}
	// The legacy field is retired after import.
	if rt := loadRuntime(cfg.WorkDir); rt.Schedules != nil {
		t.Errorf("runtime.json still carries the legacy schedules field: %+v", rt.Schedules)
	}
	// And the import produced durable rows, not just live state.
	rows, err := sp.Store.ListSchedules()
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %+v err=%v, want a runtime row + a tombstone", rows, err)
	}
}

// RP-20 §2.4: a fresh register discards every runtime override — rows and the
// legacy field — so the manifest is authoritative again.
func TestDiscardRuntimeSchedules(t *testing.T) {
	cfg := stubConfig(t)

	loaded := testLoaded()
	loaded[1].Schedule = &agentdef.Schedule{Cron: "*/10 * * * *"}

	sp1, err := NewSpace("sched-discard", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #1: %v", err)
	}
	sup1 := NewSupervisor(sp1)
	if err := sup1.SetSchedule("worker-a", agentdef.Schedule{Cron: "*/2 * * * *"}); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	if err := sup1.ClearSchedule("worker-b"); err != nil {
		t.Fatalf("ClearSchedule: %v", err)
	}
	sp1.Shutdown()

	// Re-register the same workdir: the wipe runs before Reload.
	sp2, err := NewSpace("sched-discard", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #2: %v", err)
	}
	if err := sp2.DiscardRuntimeSchedules(); err != nil {
		t.Fatalf("DiscardRuntimeSchedules: %v", err)
	}
	sp2.Reload()
	defer sp2.Shutdown()

	got, origin, ok := sp2.ScheduleInfoFor("worker-a")
	if !ok || got.Cron != "*/10 * * * *" || origin.Runtime {
		t.Errorf("worker-a after discard = %+v origin=%+v ok=%v, want the manifest seed", got, origin, ok)
	}
	if rows, _ := sp2.Store.ListSchedules(); len(rows) != 0 {
		t.Errorf("rows = %+v, want none after discard", rows)
	}
}

// RP-20: schedule writes for unknown members are rejected — a row for a
// member that doesn't exist would lie dormant and bind a future namesake.
func TestSetScheduleUnknownMemberRejected(t *testing.T) {
	sp, _ := ctlSpace(t, map[string]agentdef.Role{"w": agentdef.RoleWorker})
	sup := NewSupervisor(sp)

	if err := sup.SetSchedule("ghost", agentdef.Schedule{Every: time.Minute}); err == nil {
		t.Error("SetSchedule(ghost) succeeded, want unknown-member rejection")
	}
	if err := sup.ClearSchedule("ghost"); err == nil {
		t.Error("ClearSchedule(ghost) succeeded, want unknown-member rejection")
	}
	if rows, _ := sp.Store.ListSchedules(); len(rows) != 0 {
		t.Errorf("rows = %+v, want none for rejected writes", rows)
	}
}

// RP-20: removing a member deletes its runtime override row — a later re-add
// starts from the manifest, not a cadence set against the old incarnation.
func TestRemoveMemberDeletesScheduleRow(t *testing.T) {
	sp, _ := ctlSpace(t, map[string]agentdef.Role{"leader": agentdef.RoleLeader, "w": agentdef.RoleWorker})
	sup := startSup(t, sp)

	if err := sup.SetSchedule("w", agentdef.Schedule{Every: time.Hour}); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	if rows, _ := sp.Store.ListSchedules(); len(rows) != 1 {
		t.Fatalf("precondition: rows = %+v, want the runtime row", rows)
	}
	if err := sup.RemoveMember("w"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if rows, _ := sp.Store.ListSchedules(); len(rows) != 0 {
		t.Errorf("rows = %+v, want the removed member's row gone", rows)
	}
}

// RP-7 AC#5: a leader CLEAR survives restart and beats a schedule that the
// manifest/profile still declares — the cleared crontab must not resurrect.
func TestScheduleClearSurvivesRestart(t *testing.T) {
	cfg := stubConfig(t)

	loaded := testLoaded()
	// worker-a is declared (as if by manifest/profile) — NewSpace seeds it.
	loaded[1].Schedule = &agentdef.Schedule{Cron: "*/5 * * * *", Prompt: "patrol"}

	sp1, err := NewSpace("sched-clear", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #1: %v", err)
	}
	if _, ok := sp1.ScheduleFor("worker-a"); !ok {
		t.Fatal("worker-a should start scheduled from its declaration")
	}
	sup1 := NewSupervisor(sp1)
	if err := sup1.ClearSchedule("worker-a"); err != nil {
		t.Fatalf("ClearSchedule: %v", err)
	}
	sp1.Shutdown()

	// Restart with the SAME declaration still present.
	sp2, err := NewSpace("sched-clear", testManifest(), loaded, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace #2: %v", err)
	}
	sp2.Reload()
	defer sp2.Shutdown()

	if got, ok := sp2.ScheduleFor("worker-a"); ok {
		t.Errorf("worker-a schedule = %+v, want cleared (persisted clear must beat the re-declared schedule)", got)
	}
}
