package todo

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnny1110/evva/pkg/tools"
)

func TestWriteTool_Name(t *testing.T) {
	tool := NewWrite(NewTodoStore())
	if tool.Name() != string(tools.TODO_WRITE) {
		t.Errorf("Name = %q, want %q", tool.Name(), tools.TODO_WRITE)
	}
}

func TestWriteTool_SchemaIsValidJSON(t *testing.T) {
	tool := NewWrite(NewTodoStore())
	var v any
	if err := json.Unmarshal(tool.Schema(), &v); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
}

func TestWriteTool_ExecuteReplaces(t *testing.T) {
	store := NewTodoStore()
	tool := NewWrite(store)
	in := []byte(`{"todos":[
		{"content":"Run tests","activeForm":"Running tests","status":"in_progress"},
		{"content":"Ship the feature","activeForm":"Shipping the feature","status":"pending"}
	]}`)
	res, err := tool.Execute(context.Background(), tools.NopLogger(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	list := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 todos stored, got %d", len(list))
	}
	if list[0].Status != StatusInProgress || list[1].Status != StatusPending {
		t.Errorf("status drift: %#v", list)
	}
	if !strings.Contains(res.Content, "2 total") {
		t.Errorf("result summary missing count: %q", res.Content)
	}
}

func TestWriteTool_ExecuteRejectsInvalidStatus(t *testing.T) {
	tool := NewWrite(NewTodoStore())
	in := []byte(`{"todos":[{"content":"x","activeForm":"X","status":"bogus"}]}`)
	res, err := tool.Execute(context.Background(), tools.NopLogger(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for invalid status, got %s", res.Content)
	}
}

func TestWriteTool_ExecuteRejectsEmptyContent(t *testing.T) {
	tool := NewWrite(NewTodoStore())
	in := []byte(`{"todos":[{"content":"   ","activeForm":"X","status":"pending"}]}`)
	res, err := tool.Execute(context.Background(), tools.NopLogger(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "content is required") {
		t.Errorf("expected content-required error, got %s", res.Content)
	}
}

func TestWriteTool_ExecuteRejectsEmptyActiveForm(t *testing.T) {
	tool := NewWrite(NewTodoStore())
	in := []byte(`{"todos":[{"content":"x","activeForm":"","status":"pending"}]}`)
	res, err := tool.Execute(context.Background(), tools.NopLogger(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "activeForm is required") {
		t.Errorf("expected activeForm-required error, got %s", res.Content)
	}
}

func TestWriteTool_ExecuteEmptyListClears(t *testing.T) {
	store := NewTodoStore()
	store.Replace([]Todo{{Content: "x", ActiveForm: "X", Status: StatusCompleted}})
	tool := NewWrite(store)
	res, err := tool.Execute(context.Background(), tools.NopLogger(), []byte(`{"todos":[]}`))
	if err != nil || res.IsError {
		t.Fatalf("Execute: err=%v res=%s", err, res.Content)
	}
	if len(store.List()) != 0 {
		t.Errorf("empty list should wipe the store, still have %d", len(store.List()))
	}
}

func TestWriteTool_ExecuteDecodeError(t *testing.T) {
	tool := NewWrite(NewTodoStore())
	res, err := tool.Execute(context.Background(), tools.NopLogger(), []byte(`{not json`))
	if err != nil {
		t.Fatalf("Execute should return decode errors via IsError, got err=%v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for bad JSON")
	}
}
