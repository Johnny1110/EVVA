package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/tools"
)

// Phase 1 analysis — task tool Execute paths:
//   - CreateTool: decode err / empty subject / happy → ID in summary
//   - GetTool: decode err / missing taskId surface / found → JSON body
//   - ListTool: empty store / sorted by numeric ID suffix (t1<t2<t10)
//   - UpdateTool: decode err / missing taskId / invalid status / not-found /
//     happy → updated summary
//   - OutputTool / StopTool: always error "not implemented"
//   - Names() returns the canonical six
//   - IsTaskToolName matches the six and rejects others

func TestTaskNames_CanonicalSix(t *testing.T) {
	names := Names()
	if len(names) != 6 {
		t.Fatalf("Names() len: got %d, want 6", len(names))
	}
	for _, n := range names {
		if !IsTaskToolName(n) {
			t.Errorf("IsTaskToolName(%q) = false, want true", n)
		}
	}
}

// --- CreateTool ----------------------------------------------------------

func TestCreateTool_DecodeError(t *testing.T) {
	tool := NewCreate(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{not json`))
	if !res.IsError || !strings.Contains(res.Content, "decode") {
		t.Errorf("expected decode error; got %q", res.Content)
	}
}

func TestCreateTool_EmptySubjectRejected(t *testing.T) {
	tool := NewCreate(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(`{"subject":"   ","description":"x"}`))
	if !res.IsError || !strings.Contains(res.Content, "required") {
		t.Errorf("expected 'required' rejection; got %q", res.Content)
	}
}

func TestCreateTool_HappyPath(t *testing.T) {
	store := NewTaskGroup()
	tool := NewCreate(store)

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(`{"subject":"do thing","description":"why","activeForm":"Doing thing"}`))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "ID: t1") {
		t.Errorf("expected 'ID: t1' in summary; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "status: pending") {
		t.Errorf("expected 'status: pending'; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "do thing") {
		t.Errorf("expected subject in summary; got %q", res.Content)
	}
	// And the store actually has it.
	if got, _ := store.Get("t1"); got.Subject != "do thing" {
		t.Errorf("store missing the task: %+v", got)
	}
}

// --- GetTool -------------------------------------------------------------

func TestGetTool_DecodeError(t *testing.T) {
	tool := NewGet(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{nope`))
	if !res.IsError || !strings.Contains(res.Content, "decode") {
		t.Errorf("expected decode error; got %q", res.Content)
	}
}

func TestGetTool_NotFoundIsError(t *testing.T) {
	tool := NewGet(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{"taskId":"t99"}`))
	if !res.IsError || !strings.Contains(res.Content, "no task") {
		t.Errorf("expected 'no task' err; got %q", res.Content)
	}
}

func TestGetTool_FoundReturnsJSON(t *testing.T) {
	store := NewTaskGroup()
	store.Create(Task{Subject: "find me", Description: "details"})
	tool := NewGet(store)

	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{"taskId":"t1"}`))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// Result should be valid JSON we can re-parse.
	var got Task
	if err := json.Unmarshal([]byte(res.Content), &got); err != nil {
		t.Fatalf("result is not valid JSON: %v\nbody=%s", err, res.Content)
	}
	if got.Subject != "find me" || got.Description != "details" {
		t.Errorf("decoded task differs: %+v", got)
	}
}

// --- ListTool ------------------------------------------------------------

func TestListTool_EmptyStoreReturnsEmptyArray(t *testing.T) {
	tool := NewList(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{}`))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	body := strings.TrimSpace(res.Content)
	if body != "[]" {
		t.Errorf("empty list body: got %q, want %q", body, "[]")
	}
}

func TestListTool_SortsByNumericIDSuffix(t *testing.T) {
	store := NewTaskGroup()
	for i := 0; i < 11; i++ {
		store.Create(Task{Subject: "x"})
	}
	tool := NewList(store)

	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{}`))
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}

	var out []map[string]any
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("not JSON: %v\nbody=%s", err, res.Content)
	}
	if len(out) != 11 {
		t.Fatalf("list len: got %d, want 11", len(out))
	}
	// Sorted numerically: t1 < t2 < ... < t10 < t11. Lexicographic
	// sort would put t10 before t2 — the test would catch that
	// regression if idLess broke.
	wantOrder := []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8", "t9", "t10", "t11"}
	for i, id := range wantOrder {
		if got := out[i]["id"]; got != id {
			t.Errorf("position %d: got %v, want %s", i, got, id)
		}
	}
}

// --- UpdateTool ----------------------------------------------------------

func TestUpdateTool_DecodeError(t *testing.T) {
	tool := NewUpdate(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{nope`))
	if !res.IsError || !strings.Contains(res.Content, "decode") {
		t.Errorf("expected decode error; got %q", res.Content)
	}
}

func TestUpdateTool_RejectsMissingTaskId(t *testing.T) {
	tool := NewUpdate(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{}`))
	if !res.IsError || !strings.Contains(res.Content, "taskId is required") {
		t.Errorf("expected 'taskId is required'; got %q", res.Content)
	}
}

func TestUpdateTool_InvalidStatus(t *testing.T) {
	store := NewTaskGroup()
	store.Create(Task{Subject: "x"})
	tool := NewUpdate(store)

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(`{"taskId":"t1","status":"BOGUS"}`))
	if !res.IsError || !strings.Contains(res.Content, "invalid status") {
		t.Errorf("expected 'invalid status'; got %q", res.Content)
	}
}

func TestUpdateTool_NotFound(t *testing.T) {
	tool := NewUpdate(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(`{"taskId":"t-nope"}`))
	if !res.IsError || !strings.Contains(res.Content, "no task with id") {
		t.Errorf("expected 'no task with id'; got %q", res.Content)
	}
}

func TestUpdateTool_HappyPath(t *testing.T) {
	store := NewTaskGroup()
	store.Create(Task{Subject: "orig"})
	tool := NewUpdate(store)

	res, _ := tool.Execute(context.Background(), tools.NopLogger(),
		json.RawMessage(`{"taskId":"t1","status":"in_progress","subject":"renamed"}`))

	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "status=in_progress") {
		t.Errorf("missing new status in summary; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "renamed") {
		t.Errorf("missing new subject in summary; got %q", res.Content)
	}
	after, _ := store.Get("t1")
	if after.Status != StatusInProgress || after.Subject != "renamed" {
		t.Errorf("store not updated: %+v", after)
	}
}

// --- OutputTool / StopTool (stubs) ---------------------------------------

func TestOutputTool_NotImplemented(t *testing.T) {
	tool := NewOutput(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{"task_id":"t1"}`))
	if !res.IsError || !strings.Contains(res.Content, "not implemented") {
		t.Errorf("expected 'not implemented'; got %q", res.Content)
	}
}

func TestStopTool_NotImplemented(t *testing.T) {
	tool := NewStop(NewTaskGroup())
	res, _ := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{"task_id":"t1"}`))
	if !res.IsError || !strings.Contains(res.Content, "not implemented") {
		t.Errorf("expected 'not implemented'; got %q", res.Content)
	}
}

// --- helpers (idLess / parseID) ------------------------------------------

func TestParseID(t *testing.T) {
	cases := map[string]struct {
		want int
		ok   bool
	}{
		"t1":   {1, true},
		"t99":  {99, true},
		"t0":   {0, true},
		"":     {0, false},
		"t":    {0, false},
		"tab":  {0, false},
		"x10":  {0, false},
		"t1.5": {0, false},
	}
	for in, exp := range cases {
		n, ok := parseID(in)
		if n != exp.want || ok != exp.ok {
			t.Errorf("parseID(%q) = (%d,%v), want (%d,%v)", in, n, ok, exp.want, exp.ok)
		}
	}
}

func TestIdLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"t1", "t2", true},
		{"t2", "t10", true},   // numeric, not lexicographic
		{"t10", "t2", false},
		{"t1", "t1", false},
		// "junk" not parseable → fallback to lexicographic; 'j' < 't' → true.
		{"junk", "t1", true},
		{"a", "b", true},
	}
	for _, tc := range cases {
		if got := idLess(tc.a, tc.b); got != tc.want {
			t.Errorf("idLess(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
