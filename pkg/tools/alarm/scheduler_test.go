package alarm

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/johnny1110/evva/pkg/common"
)

// fireSink collects fired alarms for assertions, safe across the AfterFunc
// goroutine and the test goroutine.
type fireSink struct {
	mu    sync.Mutex
	fired []Fired
}

func (f *fireSink) on(fr Fired) {
	f.mu.Lock()
	f.fired = append(f.fired, fr)
	f.mu.Unlock()
}

func (f *fireSink) snapshot() []Fired {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Fired(nil), f.fired...)
}

func TestScheduler_ArmAndFire(t *testing.T) {
	var sink fireSink
	s := New(Config{OnFire: sink.on})

	if _, err := s.Arm(Alarm{FireAt: time.Now().Add(40 * time.Millisecond), Prompt: "ping"}); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if got := s.Pending(); got != 1 {
		t.Fatalf("Pending after Arm = %d, want 1", got)
	}

	deadline := time.After(2 * time.Second)
	for {
		if len(sink.snapshot()) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("alarm did not fire within 2s")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if got := s.Pending(); got != 0 {
		t.Errorf("Pending after fire = %d, want 0 (one-shot should self-remove)", got)
	}
	if fr := sink.snapshot()[0]; fr.Prompt != "ping" || fr.Late {
		t.Errorf("fired = %+v, want prompt=ping late=false", fr)
	}
}

func TestScheduler_CancelBeforeFire(t *testing.T) {
	var sink fireSink
	s := New(Config{OnFire: sink.on})
	a, err := s.Arm(Alarm{FireAt: time.Now().Add(time.Hour), Prompt: "later"})
	if err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if !s.Cancel(a.ID) {
		t.Fatal("Cancel returned false for a pending alarm")
	}
	if s.Cancel(a.ID) {
		t.Error("second Cancel should report not-found")
	}
	if got := s.Pending(); got != 0 {
		t.Errorf("Pending after cancel = %d, want 0", got)
	}
}

func TestScheduler_RejectsPastTime(t *testing.T) {
	s := New(Config{})
	if _, err := s.Arm(Alarm{FireAt: time.Now().Add(-time.Minute), Prompt: "p"}); err == nil {
		t.Fatal("Arm with past time should error")
	}
	if _, err := s.Arm(Alarm{Prompt: "p"}); err == nil {
		t.Fatal("Arm with zero time should error")
	}
	if _, err := s.Arm(Alarm{FireAt: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("Arm with empty prompt should error")
	}
}

func TestScheduler_MaxPending(t *testing.T) {
	s := New(Config{MaxPending: 2})
	for i := range 2 {
		if _, err := s.Arm(Alarm{FireAt: time.Now().Add(time.Hour), Prompt: "p"}); err != nil {
			t.Fatalf("Arm %d: %v", i, err)
		}
	}
	if _, err := s.Arm(Alarm{FireAt: time.Now().Add(time.Hour), Prompt: "p"}); err == nil {
		t.Fatal("Arm beyond cap should error")
	}
}

func TestScheduler_DurablePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alarms.json")
	t0 := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	s1 := New(Config{StorePath: path, Now: func() time.Time { return t0 }})
	if _, err := s1.Arm(Alarm{FireAt: t0.Add(time.Hour), Prompt: "future", Durable: true}); err != nil {
		t.Fatalf("Arm durable: %v", err)
	}
	// A session-only alarm must NOT survive.
	if _, err := s1.Arm(Alarm{FireAt: t0.Add(time.Hour), Prompt: "ephemeral", Durable: false}); err != nil {
		t.Fatalf("Arm session-only: %v", err)
	}

	// Restart "before" the fire instant: the future durable alarm re-arms, the
	// session-only one is gone, and nothing fires.
	var sink fireSink
	s2 := New(Config{StorePath: path, Now: func() time.Time { return t0 }, OnFire: sink.on})
	pastDue, err := s2.Rearm()
	if err != nil {
		t.Fatalf("Rearm: %v", err)
	}
	if len(pastDue) != 0 {
		t.Errorf("Rearm pastDue = %d, want 0", len(pastDue))
	}
	if got := s2.Pending(); got != 1 {
		t.Errorf("Pending after Rearm = %d, want 1 (only the durable future alarm)", got)
	}
	if len(sink.snapshot()) != 0 {
		t.Error("Rearm must not fire anything")
	}
}

func TestScheduler_PastDueFiresOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alarms.json")
	t0 := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	s1 := New(Config{StorePath: path, Now: func() time.Time { return t0 }})
	if _, err := s1.Arm(Alarm{FireAt: t0.Add(time.Hour), Prompt: "missed", Durable: true}); err != nil {
		t.Fatalf("Arm: %v", err)
	}

	// Restart "after" the fire instant — the alarm came due during downtime.
	var sink fireSink
	now2 := t0.Add(2 * time.Hour)
	s2 := New(Config{StorePath: path, Now: func() time.Time { return now2 }, OnFire: sink.on})

	// Rearm alone returns past-due WITHOUT firing.
	pastDue, err := s2.Rearm()
	if err != nil {
		t.Fatalf("Rearm: %v", err)
	}
	if len(pastDue) != 1 || !pastDue[0].Late || pastDue[0].Prompt != "missed" {
		t.Fatalf("Rearm pastDue = %+v, want one late 'missed'", pastDue)
	}
	if len(sink.snapshot()) != 0 {
		t.Error("Rearm must not fire OnFire")
	}

	// LoadAndRearm fires past-due through OnFire. Re-arm a fresh past-due alarm
	// first (Rearm above already drained the store).
	s3 := New(Config{StorePath: path, Now: func() time.Time { return t0 }})
	if _, err := s3.Arm(Alarm{FireAt: t0.Add(time.Hour), Prompt: "missed2", Durable: true}); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	var sink2 fireSink
	s4 := New(Config{StorePath: path, Now: func() time.Time { return now2 }, OnFire: sink2.on})
	if err := s4.LoadAndRearm(); err != nil {
		t.Fatalf("LoadAndRearm: %v", err)
	}
	got := sink2.snapshot()
	if len(got) != 1 || !got[0].Late || got[0].Prompt != "missed2" {
		t.Fatalf("LoadAndRearm fired = %+v, want one late 'missed2'", got)
	}
}

func TestParseFireTime(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"2026-09-11 12:31:50", time.Date(2026, 9, 11, 12, 31, 50, 0, loc), true},
		{"2026-09-11T12:31:50", time.Date(2026, 9, 11, 12, 31, 50, 0, loc), true},
		{"2026-09-11 12:31", time.Date(2026, 9, 11, 12, 31, 0, 0, loc), true},
		{"2026-09-11T12:31:50Z", time.Date(2026, 9, 11, 12, 31, 50, 0, time.UTC), true},
		{"garbage", time.Time{}, false},
	}
	for _, c := range cases {
		got, err := ParseFireTime(c.in, loc)
		if c.ok && err != nil {
			t.Errorf("ParseFireTime(%q): unexpected err %v", c.in, err)
			continue
		}
		if !c.ok {
			if err == nil {
				t.Errorf("ParseFireTime(%q): expected error", c.in)
			}
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseFireTime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFiredMessage(t *testing.T) {
	at := time.Date(2026, 9, 11, 12, 31, 50, 0, time.Local)
	plain := Fired{Alarm: Alarm{Prompt: "do the thing"}}
	if got := plain.Message(); got != "⏰ Alarm fired\ndo the thing" {
		t.Errorf("plain Message() = %q", got)
	}
	labeled := Fired{Alarm: Alarm{Prompt: "p", Label: "deploy"}}
	if got := labeled.Message(); got != "⏰ Alarm fired [deploy]\np" {
		t.Errorf("labeled Message() = %q", got)
	}
	timed := Fired{Alarm: Alarm{Prompt: "p", FireAt: at}}
	if got, want := timed.Message(), fmt.Sprintf("⏰ Alarm fired — %s\np", common.Stamp(at)); got != want {
		t.Errorf("timed Message() = %q, want %q", got, want)
	}
	late := Fired{Alarm: Alarm{Prompt: "p", FireAt: at}, Late: true}
	want := fmt.Sprintf("⏰ Alarm fired (late — was due %s)\np", common.Stamp(at))
	if got := late.Message(); got != want {
		t.Errorf("late Message() = %q, want %q", got, want)
	}
}

// --- cron (recurring) -------------------------------------------------------

// fastCronNext is a fake cron evaluator that fires d after `after`, decoupling
// the scheduler's recurring machinery from the minute-resolution cron engine so
// tests run in milliseconds. zeroJitter disables load-spreading for prompt fires.
func fastCronNext(d time.Duration) func(string, time.Time) (time.Time, error) {
	return func(_ string, after time.Time) (time.Time, error) { return after.Add(d), nil }
}

func zeroJitter() time.Duration { return 0 }

// waitFor polls cond until true or fails after timeout.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition not met within timeout")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestScheduler_RecurringRearm(t *testing.T) {
	var sink fireSink
	s := New(Config{OnFire: sink.on, CronNext: fastCronNext(20 * time.Millisecond), Jitter: zeroJitter})

	a, err := s.Arm(Alarm{CronExpr: "*/1 * * * *", Recurring: true, Prompt: "tick"})
	if err != nil {
		t.Fatalf("Arm recurring: %v", err)
	}
	if a.Expiry.IsZero() {
		t.Error("recurring cron should get a default expiry")
	}

	// At least three fires proves it re-arms rather than firing once.
	waitFor(t, func() bool { return len(sink.snapshot()) >= 3 }, 2*time.Second)
	if got := s.Pending(); got != 1 {
		t.Errorf("Pending while recurring = %d, want 1 (stays armed)", got)
	}
	for i, f := range sink.snapshot() {
		if !f.Recurring || f.CronExpr == "" || f.NextFire.IsZero() {
			t.Errorf("fire %d = %+v, want recurring cron with NextFire set", i, f)
		}
	}

	if !s.Cancel(a.ID) {
		t.Fatal("Cancel returned false for a pending cron job")
	}
	if got := s.Pending(); got != 0 {
		t.Errorf("Pending after cancel = %d, want 0", got)
	}
}

func TestScheduler_OneShotCronFiresOnce(t *testing.T) {
	var sink fireSink
	s := New(Config{OnFire: sink.on, CronNext: fastCronNext(20 * time.Millisecond), Jitter: zeroJitter})

	a, err := s.Arm(Alarm{CronExpr: "30 14 * * *", Recurring: false, Prompt: "once"})
	if err != nil {
		t.Fatalf("Arm one-shot cron: %v", err)
	}
	if !a.Expiry.IsZero() {
		t.Error("one-shot cron should not get an expiry")
	}

	waitFor(t, func() bool { return len(sink.snapshot()) >= 1 }, 2*time.Second)
	time.Sleep(60 * time.Millisecond) // room to (wrongly) re-fire
	got := sink.snapshot()
	if len(got) != 1 {
		t.Fatalf("one-shot cron fired %d times, want 1", len(got))
	}
	if got[0].Recurring || got[0].CronExpr == "" {
		t.Errorf("one-shot fire = %+v, want a non-recurring cron entry", got[0])
	}
	if s.Pending() != 0 {
		t.Errorf("Pending after one-shot fire = %d, want 0 (self-removed)", s.Pending())
	}
}

func TestScheduler_RecurringAutoExpiry(t *testing.T) {
	var sink fireSink
	t0 := time.Now()
	s := New(Config{
		OnFire:   sink.on,
		CronNext: fastCronNext(20 * time.Millisecond),
		Jitter:   zeroJitter,
		Now:      func() time.Time { return t0 },
	})
	// Expiry already in the past → the first fire is the final one (no re-arm).
	if _, err := s.Arm(Alarm{CronExpr: "*/1 * * * *", Recurring: true, Prompt: "stale", Expiry: t0.Add(-time.Hour)}); err != nil {
		t.Fatalf("Arm: %v", err)
	}

	waitFor(t, func() bool { return len(sink.snapshot()) >= 1 }, 2*time.Second)
	time.Sleep(60 * time.Millisecond)
	if got := len(sink.snapshot()); got != 1 {
		t.Errorf("expired job fired %d times, want exactly 1", got)
	}
	if s.Pending() != 0 {
		t.Errorf("Pending after expiry = %d, want 0 (auto-deleted)", s.Pending())
	}
	if sink.snapshot()[0].Recurring {
		t.Error("expired final fire must not be flagged recurring")
	}
}

func TestScheduler_RecurringRejectedWithoutCronNext(t *testing.T) {
	s := New(Config{}) // CronNext nil
	if _, err := s.Arm(Alarm{CronExpr: "*/5 * * * *", Recurring: true, Prompt: "p"}); err == nil {
		t.Fatal("Arm with a cron expr but no CronNext should error")
	}
}

func TestScheduler_DurableCronRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alarms.json")
	t0 := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	// Far-future next so nothing fires during the test.
	cfg := func(sink *fireSink) Config {
		c := Config{StorePath: path, Now: func() time.Time { return t0 }, CronNext: fastCronNext(time.Hour), Jitter: zeroJitter}
		if sink != nil {
			c.OnFire = sink.on
		}
		return c
	}

	s1 := New(cfg(nil))
	if _, err := s1.Arm(Alarm{CronExpr: "0 9 * * *", Recurring: true, Prompt: "daily", Durable: true}); err != nil {
		t.Fatalf("Arm durable recurring: %v", err)
	}
	// A session-only cron must NOT survive a restart.
	if _, err := s1.Arm(Alarm{CronExpr: "0 9 * * *", Recurring: true, Prompt: "ephemeral", Durable: false}); err != nil {
		t.Fatalf("Arm session cron: %v", err)
	}

	var sink fireSink
	s2 := New(cfg(&sink))
	pastDue, err := s2.Rearm()
	if err != nil {
		t.Fatalf("Rearm: %v", err)
	}
	if len(pastDue) != 0 {
		t.Errorf("Rearm pastDue = %d, want 0 (recurring jobs never fire late)", len(pastDue))
	}
	if got := s2.Pending(); got != 1 {
		t.Fatalf("Pending after Rearm = %d, want 1 (only the durable cron)", got)
	}
	got := s2.List()
	if len(got) != 1 || got[0].CronExpr != "0 9 * * *" || !got[0].Recurring || got[0].Prompt != "daily" {
		t.Errorf("re-armed cron = %+v, want durable recurring 'daily'", got)
	}
	if len(sink.snapshot()) != 0 {
		t.Error("Rearm must not fire anything")
	}
}

func TestScheduler_MixedAlarmAndCronList(t *testing.T) {
	t0 := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	s := New(Config{Now: func() time.Time { return t0 }, CronNext: fastCronNext(30 * time.Minute), Jitter: zeroJitter})

	if _, err := s.Arm(Alarm{FireAt: t0.Add(time.Hour), Prompt: "alarm-late"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Arm(Alarm{CronExpr: "*/30 * * * *", Recurring: true, Prompt: "cron-mid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Arm(Alarm{FireAt: t0.Add(10 * time.Minute), Prompt: "alarm-soon"}); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("List len = %d, want 3", len(list))
	}
	// Sorted by FireAt across both kinds: soon (10m) < cron (30m) < late (60m).
	for i, w := range []string{"alarm-soon", "cron-mid", "alarm-late"} {
		if list[i].Prompt != w {
			t.Errorf("List[%d].Prompt = %q, want %q", i, list[i].Prompt, w)
		}
	}
	crons := 0
	for _, a := range list {
		if a.CronExpr != "" {
			crons++
		}
	}
	if crons != 1 {
		t.Errorf("entries with a cron expr = %d, want 1", crons)
	}
}

func TestDefaultJitterBounds(t *testing.T) {
	for range 1000 {
		if d := defaultJitter(); d < 0 || d >= maxJitter {
			t.Fatalf("defaultJitter() = %v, want [0, %v)", d, maxJitter)
		}
	}
}

func TestFiredMessage_Cron(t *testing.T) {
	at := time.Date(2026, 9, 11, 14, 30, 0, 0, time.Local)
	next := time.Date(2026, 9, 11, 14, 35, 0, 0, time.Local)

	recurring := Fired{Alarm: Alarm{Prompt: "check", CronExpr: "*/5 * * * *", FireAt: at}, Recurring: true, NextFire: next}
	want := fmt.Sprintf("⏰ Cron job fired — %s (recurring, next: %s)\ncheck", common.Stamp(at), common.Stamp(next))
	if got := recurring.Message(); got != want {
		t.Errorf("recurring cron Message() = %q, want %q", got, want)
	}

	oneShot := Fired{Alarm: Alarm{Prompt: "once", CronExpr: "30 14 * * *", FireAt: at}}
	wantOneShot := fmt.Sprintf("⏰ Cron job fired — %s\nonce", common.Stamp(at))
	if got := oneShot.Message(); got != wantOneShot {
		t.Errorf("one-shot cron Message() = %q, want %q", got, wantOneShot)
	}
}
