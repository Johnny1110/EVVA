package cron

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/johnny1110/evva/pkg/tools"
	"github.com/johnny1110/evva/pkg/tools/alarm"
)

func mustExec(t *testing.T, tool tools.Tool, input string) tools.Result {
	t.Helper()
	res, err := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s.Execute: unexpected Go error: %v", tool.Name(), err)
	}
	return res
}

// newSched builds a scheduler wired to the real cron engine, jitter disabled.
func newSched() *alarm.Scheduler {
	return alarm.New(alarm.Config{CronNext: Next, Jitter: func() time.Duration { return 0 }})
}

func TestNext(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 30, 0, 0, time.Local)
	got, err := Next("*/5 * * * *", now)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if want := time.Date(2026, 6, 17, 10, 35, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
	if _, err := Next("nonsense", now); err == nil {
		t.Error("Next on a bad expression should error")
	}
}

func TestCronCreate_Recurring(t *testing.T) {
	s := newSched()
	res := mustExec(t, NewCreate(s), `{"cron":"*/5 * * * *","prompt":"check CI"}`)
	if res.IsError {
		t.Fatalf("create: %s", res.Content)
	}
	a, ok := res.Metadata.(alarm.Alarm)
	if !ok {
		t.Fatalf("Metadata is not an alarm.Alarm: %T", res.Metadata)
	}
	if a.CronExpr != "*/5 * * * *" || !a.Recurring {
		t.Errorf("alarm = %+v, want recurring cron */5", a)
	}
	if a.Expiry.IsZero() {
		t.Error("recurring cron should get a 7-day expiry")
	}
	if a.Durable {
		t.Error("durable should default to false for cron")
	}
	if s.Pending() != 1 {
		t.Errorf("Pending = %d, want 1", s.Pending())
	}
	if !strings.Contains(res.Content, a.ID) || !strings.Contains(res.Content, "recurring") {
		t.Errorf("content %q should mention the id and 'recurring'", res.Content)
	}
}

func TestCronCreate_OneShot(t *testing.T) {
	s := newSched()
	res := mustExec(t, NewCreate(s), `{"cron":"30 14 * * *","prompt":"once","recurring":false}`)
	if res.IsError {
		t.Fatalf("create: %s", res.Content)
	}
	a := res.Metadata.(alarm.Alarm)
	if a.Recurring {
		t.Error("recurring=false should be honored")
	}
	if !a.Expiry.IsZero() {
		t.Error("one-shot cron should not get an expiry")
	}
	if !strings.Contains(res.Content, "one-shot") {
		t.Errorf("content %q should say 'one-shot'", res.Content)
	}
}

func TestCronCreate_Durable(t *testing.T) {
	a := mustExec(t, NewCreate(newSched()), `{"cron":"0 9 * * *","prompt":"p","durable":true}`).Metadata.(alarm.Alarm)
	if !a.Durable {
		t.Error("durable=true should be honored")
	}
}

func TestCronCreate_Rejects(t *testing.T) {
	s := newSched()
	tool := NewCreate(s)
	cases := []struct{ name, in string }{
		{"bad expression", `{"cron":"not a cron","prompt":"p"}`},
		{"too few fields", `{"cron":"* * *","prompt":"p"}`},
		{"missing cron", `{"prompt":"p"}`},
		{"blank cron", `{"cron":"   ","prompt":"p"}`},
		{"missing prompt", `{"cron":"*/5 * * * *"}`},
		{"blank prompt", `{"cron":"*/5 * * * *","prompt":" "}`},
		{"impossible date", `{"cron":"0 0 30 2 *","prompt":"p"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if res := mustExec(t, tool, c.in); !res.IsError {
				t.Fatalf("expected IsError, got: %s", res.Content)
			}
		})
	}
	if s.Pending() != 0 {
		t.Errorf("no job should have armed; Pending = %d", s.Pending())
	}
}

func TestCronCreate_NilScheduler(t *testing.T) {
	res := mustExec(t, NewCreate(nil), `{"cron":"*/5 * * * *","prompt":"p"}`)
	if !res.IsError || !strings.Contains(res.Content, "root-agent only") {
		t.Errorf("nil scheduler should error with 'root-agent only', got: %s", res.Content)
	}
}

func TestCronList_FiltersToCronOnly(t *testing.T) {
	s := newSched()
	create, list := NewCreate(s), NewList(s)

	if res := mustExec(t, list, `{}`); res.IsError || !strings.Contains(res.Content, "No pending cron") {
		t.Fatalf("empty list = %q", res.Content)
	}

	cronID := mustExec(t, create, `{"cron":"*/5 * * * *","prompt":"cron-job"}`).Metadata.(alarm.Alarm).ID
	// A plain alarm armed directly on the shared scheduler must not leak into cron_list.
	if _, err := s.Arm(alarm.Alarm{FireAt: time.Now().Add(time.Hour), Prompt: "plain-alarm"}); err != nil {
		t.Fatalf("Arm alarm: %v", err)
	}

	res := mustExec(t, list, `{}`)
	if res.IsError {
		t.Fatalf("list: %s", res.Content)
	}
	if !strings.Contains(res.Content, cronID) || !strings.Contains(res.Content, "cron-job") {
		t.Errorf("cron_list should show the cron job: %q", res.Content)
	}
	if strings.Contains(res.Content, "plain-alarm") {
		t.Errorf("cron_list must NOT show the plain alarm: %q", res.Content)
	}
	jobs := res.Metadata.([]alarm.Alarm)
	if len(jobs) != 1 || jobs[0].Prompt != "cron-job" {
		t.Errorf("list metadata = %+v, want only the cron job", jobs)
	}
}

func TestCronDelete(t *testing.T) {
	s := newSched()
	create, del := NewCreate(s), NewDelete(s)
	id := mustExec(t, create, `{"cron":"*/5 * * * *","prompt":"p"}`).Metadata.(alarm.Alarm).ID

	if res := mustExec(t, del, `{"id":"alm_nope"}`); !res.IsError {
		t.Errorf("delete unknown id should error, got: %s", res.Content)
	}
	if res := mustExec(t, del, `{"id":"`+id+`"}`); res.IsError {
		t.Errorf("delete real id should succeed, got: %s", res.Content)
	}
	if s.Pending() != 0 {
		t.Errorf("Pending after delete = %d, want 0", s.Pending())
	}
	if res := mustExec(t, del, `{"id":"  "}`); !res.IsError {
		t.Error("delete blank id should error")
	}
}

func TestCronTools_NilSchedulerErrorsCleanly(t *testing.T) {
	for _, tool := range []tools.Tool{NewList(nil), NewDelete(nil)} {
		if res := mustExec(t, tool, `{}`); !res.IsError {
			t.Errorf("%s with nil scheduler should error", tool.Name())
		}
	}
}
