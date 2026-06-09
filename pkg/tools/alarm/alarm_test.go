package alarm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/johnny1110/evva/pkg/tools"
)

func mustExec(t *testing.T, tool tools.Tool, input string) tools.Result {
	t.Helper()
	res, err := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s.Execute: unexpected Go error: %v", tool.Name(), err)
	}
	return res
}

func TestCreateTool_ArmsAndReports(t *testing.T) {
	s := New(Config{})
	tool := NewCreate(s)

	at := time.Now().Add(2 * time.Hour).Format("2006-01-02 15:04:05")
	res := mustExec(t, tool, `{"at":"`+at+`","prompt":"check the deploy","label":"deploy"}`)
	if res.IsError {
		t.Fatalf("create: unexpected error: %s", res.Content)
	}
	if s.Pending() != 1 {
		t.Fatalf("Pending = %d, want 1", s.Pending())
	}
	a, ok := res.Metadata.(Alarm)
	if !ok {
		t.Fatalf("Metadata is not an Alarm: %T", res.Metadata)
	}
	if a.Label != "deploy" || !a.Durable {
		t.Errorf("alarm = %+v, want label=deploy durable=true (default)", a)
	}
	if !strings.Contains(res.Content, a.ID) {
		t.Errorf("content %q should mention id %q", res.Content, a.ID)
	}
}

func TestCreateTool_DurableDefaultAndOverride(t *testing.T) {
	s := New(Config{})
	tool := NewCreate(s)
	at := time.Now().Add(time.Hour).Format(time.RFC3339)

	res := mustExec(t, tool, `{"at":"`+at+`","prompt":"p","durable":false}`)
	if res.IsError {
		t.Fatalf("create: %s", res.Content)
	}
	a := res.Metadata.(Alarm)
	if a.Durable {
		t.Error("durable=false should be honored")
	}
}

func TestCreateTool_Rejects(t *testing.T) {
	s := New(Config{})
	tool := NewCreate(s)
	cases := []struct {
		name, in string
	}{
		{"missing prompt", `{"at":"2999-01-01 00:00:00"}`},
		{"blank prompt", `{"at":"2999-01-01 00:00:00","prompt":"  "}`},
		{"bad time", `{"at":"not-a-time","prompt":"p"}`},
		{"past time", `{"at":"2000-01-01 00:00:00","prompt":"p"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := mustExec(t, tool, c.in)
			if !res.IsError {
				t.Fatalf("expected IsError, got: %s", res.Content)
			}
		})
	}
	if s.Pending() != 0 {
		t.Errorf("no alarm should have been armed; Pending = %d", s.Pending())
	}
}

func TestListAndCancelTools(t *testing.T) {
	s := New(Config{})
	create, list, cancel := NewCreate(s), NewList(s), NewCancel(s)

	// Empty list.
	if res := mustExec(t, list, `{}`); res.IsError || !strings.Contains(res.Content, "No pending") {
		t.Fatalf("empty list = %q (err=%v)", res.Content, res.IsError)
	}

	at := time.Now().Add(time.Hour).Format("2006-01-02 15:04:05")
	created := mustExec(t, create, `{"at":"`+at+`","prompt":"do X","label":"L"}`)
	id := created.Metadata.(Alarm).ID

	res := mustExec(t, list, `{}`)
	if res.IsError || !strings.Contains(res.Content, id) || !strings.Contains(res.Content, "do X") {
		t.Fatalf("list should show the alarm: %q", res.Content)
	}

	// Cancel unknown id.
	if res := mustExec(t, cancel, `{"id":"alm_nope"}`); !res.IsError {
		t.Errorf("cancel unknown id should error, got: %s", res.Content)
	}
	// Cancel real id.
	if res := mustExec(t, cancel, `{"id":"`+id+`"}`); res.IsError {
		t.Errorf("cancel real id should succeed, got: %s", res.Content)
	}
	if s.Pending() != 0 {
		t.Errorf("Pending after cancel = %d, want 0", s.Pending())
	}
}

func TestTools_NilSchedulerErrorsCleanly(t *testing.T) {
	// A subagent profile that somehow built the tool with no scheduler must
	// surface a clean error, not panic.
	for _, tool := range []tools.Tool{NewCreate(nil), NewList(nil), NewCancel(nil)} {
		res := mustExec(t, tool, `{}`)
		if !res.IsError {
			t.Errorf("%s with nil scheduler should error", tool.Name())
		}
	}
}
