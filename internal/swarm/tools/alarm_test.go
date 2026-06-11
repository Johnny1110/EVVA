package tools

import (
	"strings"
	"testing"
)

const farFuture = "2099-01-01 00:00:00"

// A worker may set a one-shot alarm for ITSELF (member omitted): it lands on the
// space scheduler aimed at the caller, sent on its own behalf.
func TestAlarmSet_SelfDefault(t *testing.T) {
	sp := realSpace(t)
	set := newAlarmSet(workerMC(sp, "worker-a"))

	if r := exec(t, set, `{"at":"`+farFuture+`","prompt":"re-check the run","label":"recheck"}`); r.IsError {
		t.Fatalf("alarm_set self: %s", r.Content)
	}
	got := sp.AlarmScheduler().List()
	if len(got) != 1 {
		t.Fatalf("pending alarms = %d, want 1", len(got))
	}
	if got[0].Target != "worker-a" || got[0].Origin != "worker-a" || got[0].Label != "recheck" {
		t.Errorf("alarm = %+v, want target=origin=worker-a label=recheck", got[0])
	}
}

// A worker may NOT target another member — that authority is the leader's. The
// rejection is correctable and nothing is armed.
func TestAlarmSet_WorkerCannotTargetOthers(t *testing.T) {
	sp := realSpace(t)
	set := newAlarmSet(workerMC(sp, "worker-a"))

	r := exec(t, set, `{"at":"`+farFuture+`","prompt":"do it","member":"worker-b"}`)
	if !r.IsError || !strings.Contains(r.Content, "leader") {
		t.Errorf("worker targeting another = %+v, want an error mentioning leader", r)
	}
	if n := sp.AlarmScheduler().Pending(); n != 0 {
		t.Errorf("rejected alarm must not arm; pending = %d", n)
	}
}

// The leader may wake a specific member at an exact time.
func TestAlarmSet_LeaderTargetsMember(t *testing.T) {
	sp := realSpace(t)
	set := newAlarmSet(leaderMC(sp))

	if r := exec(t, set, `{"at":"`+farFuture+`","prompt":"review overnight run","member":"worker-a"}`); r.IsError {
		t.Fatalf("leader alarm_set: %s", r.Content)
	}
	got := sp.AlarmScheduler().List()
	if len(got) != 1 || got[0].Target != "worker-a" || got[0].Origin != "leader" {
		t.Fatalf("alarm = %+v, want target=worker-a origin=leader", got)
	}
}

func TestAlarmSet_RejectsPastAndUnknownMember(t *testing.T) {
	sp := realSpace(t)
	set := newAlarmSet(leaderMC(sp))

	if r := exec(t, set, `{"at":"2000-01-01 00:00:00","prompt":"p"}`); !r.IsError {
		t.Errorf("past time = %+v, want error", r)
	}
	if r := exec(t, set, `{"at":"`+farFuture+`","prompt":"p","member":"ghost"}`); !r.IsError || !strings.Contains(r.Content, "worker-a") {
		t.Errorf("unknown member = %+v, want correctable error listing members", r)
	}
	if r := exec(t, set, `{"at":"`+farFuture+`"}`); !r.IsError {
		t.Errorf("missing prompt = %+v, want error", r)
	}
	if n := sp.AlarmScheduler().Pending(); n != 0 {
		t.Errorf("no rejected alarm should arm; pending = %d", n)
	}
}

// alarm_clear: a member may clear an alarm aimed at it; an unrelated worker may
// not; the id must exist.
func TestAlarmClear_AuthAndCancel(t *testing.T) {
	sp := realSpace(t)
	// Leader arms an alarm aimed at worker-a.
	if r := exec(t, newAlarmSet(leaderMC(sp)), `{"at":"`+farFuture+`","prompt":"x","member":"worker-a"}`); r.IsError {
		t.Fatalf("setup alarm_set: %s", r.Content)
	}
	id := sp.AlarmScheduler().List()[0].ID

	// worker-b (neither origin nor target) cannot clear it.
	if r := exec(t, newAlarmClear(workerMC(sp, "worker-b")), `{"id":"`+id+`"}`); !r.IsError {
		t.Errorf("worker-b clearing another's alarm = %+v, want error", r)
	}
	// Unknown id errors.
	if r := exec(t, newAlarmClear(leaderMC(sp)), `{"id":"alm_nope"}`); !r.IsError {
		t.Errorf("clear unknown id = %+v, want error", r)
	}
	// worker-a (the target) may clear it.
	if r := exec(t, newAlarmClear(workerMC(sp, "worker-a")), `{"id":"`+id+`"}`); r.IsError {
		t.Errorf("target clearing its own alarm = %+v, want success", r)
	}
	if n := sp.AlarmScheduler().Pending(); n != 0 {
		t.Errorf("pending after clear = %d, want 0", n)
	}
}

// list_members surfaces pending one-shot alarms inline so a compacted leader
// re-learns them and has an id source for alarm_clear.
func TestListMembersShowsAlarms(t *testing.T) {
	sp := realSpace(t)
	if r := exec(t, newAlarmSet(leaderMC(sp)), `{"at":"`+farFuture+`","prompt":"x","member":"worker-a","label":"audit"}`); r.IsError {
		t.Fatalf("alarm_set: %s", r.Content)
	}
	res := exec(t, newListMembers(leaderMC(sp)), `{}`)
	if res.IsError {
		t.Fatalf("list_members: %s", res.Content)
	}
	if !strings.Contains(res.Content, "⏰ alm_") || !strings.Contains(res.Content, "audit") {
		t.Errorf("list_members missing worker-a's pending alarm, got:\n%s", res.Content)
	}
}
