package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Phase 1 analysis — stub.go surface:
//   - NewStub(name, desc, schema) builds a Tool whose accessors return the
//     supplied values verbatim. The schema string is wrapped as
//     json.RawMessage so the LLM contract matches a real tool.
//   - stubTool.Execute always returns IsError=true with content
//     `tool "<name>" is not implemented yet`. The go-level error is always
//     nil — IsError is the channel the model uses.
//
// name.go: ToolName is a string alias with ~30 declared constants spread
// across multiple const blocks. A duplicate value across blocks would
// silently route two LLM-facing names to the same Go identifier — worth a
// defensive uniqueness sanity check.

func TestNewStub_AccessorsReturnSuppliedValues(t *testing.T) {
	const (
		name = ToolName("test_stub")
		desc = "a test tool that does nothing"
	)
	schema := `{"type":"object","properties":{"foo":{"type":"string"}}}`

	tool := NewStub(name, desc, schema)

	if got := tool.Name(); got != string(name) {
		t.Errorf("Name(): got %q, want %q", got, name)
	}
	if got := tool.Description(); got != desc {
		t.Errorf("Description(): got %q, want %q", got, desc)
	}
	if got := string(tool.Schema()); got != schema {
		t.Errorf("Schema(): got %q, want %q", got, schema)
	}
}

func TestNewStub_Execute_ReturnsIsErrorWithName(t *testing.T) {
	tool := NewStub(ToolName("magic_wand"), "wave", `{}`)

	res, err := tool.Execute(context.Background(), NopLogger(), json.RawMessage(`{}`))

	if err != nil {
		t.Errorf("Execute should return nil go error (IsError is the channel); got %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true")
	}
	if !strings.Contains(res.Content, `"magic_wand"`) {
		t.Errorf("error content should mention the tool name; got %q", res.Content)
	}
	if !strings.Contains(res.Content, "not implemented") {
		t.Errorf("error content should say 'not implemented'; got %q", res.Content)
	}
}

func TestNewStub_Execute_IgnoresInput(t *testing.T) {
	// The stub doesn't care what input it gets — any well-formed or
	// malformed bytes should still produce the same "not implemented"
	// error. This lets a model call a deferred tool before its real
	// implementation lands without crashing.
	tool := NewStub(ToolName("noop"), "", `{}`)

	cases := []json.RawMessage{
		nil,
		json.RawMessage(`{}`),
		json.RawMessage(`{"k":"v"}`),
		json.RawMessage(`not even json`),
	}
	for _, in := range cases {
		res, err := tool.Execute(context.Background(), NopLogger(), in)
		if err != nil {
			t.Errorf("input %q: unexpected go err %v", in, err)
		}
		if !res.IsError {
			t.Errorf("input %q: expected IsError=true", in)
		}
	}
}

func TestNewStub_SchemaIsValidJSONWrapper(t *testing.T) {
	// Smoke check: the schema we wrap should still parse as JSON when
	// callers serialize it onto the wire. The wrapper doesn't validate
	// — caller bears that — but a typo in the test input would break
	// other tests that build on NewStub.
	tool := NewStub(ToolName("x"), "", `{"type":"object","properties":{}}`)

	var v map[string]any
	if err := json.Unmarshal(tool.Schema(), &v); err != nil {
		t.Errorf("schema not valid JSON after wrapping: %v", err)
	}
	if v["type"] != "object" {
		t.Errorf("schema content drifted: %+v", v)
	}
}

func TestNewStub_AllowsEmptyDescAndSchema(t *testing.T) {
	// Defensive: building a stub with empty desc / schema must not panic.
	// (Useful for placeholder tools that haven't been spec'd yet.)
	tool := NewStub(ToolName("placeholder"), "", "")
	if tool.Name() != "placeholder" {
		t.Errorf("Name: got %q", tool.Name())
	}
	if tool.Description() != "" {
		t.Errorf("Description should be empty; got %q", tool.Description())
	}
	if len(tool.Schema()) != 0 {
		t.Errorf("Schema should be empty; got %q", tool.Schema())
	}
}

// TestToolNames_AreUnique guards against an easy mistake when growing
// the constant list: two ToolName entries assigned the same string.
// Cross-block declarations wouldn't error at compile time but would
// silently route two Go identifiers to the same LLM-facing name —
// every dispatch on that name would resolve to one implementation,
// shadowing the other.
//
// Add new ToolName constants to the slice below when adding them in
// name.go.
func TestToolNames_AreUnique(t *testing.T) {
	all := []ToolName{
		// Active
		READ_FILE, WRITE_FILE, EDIT_FILE, BASH, AGENT, TOOL_SEARCH, SKILL, SCHEDULE_WAKEUP,
		// Task family
		TASK_CREATE, TASK_GET, TASK_LIST, TASK_UPDATE, TASK_OUTPUT, TASK_STOP,
		// Monitor / mode / notebook
		MONITOR, ENTER_PLAN_MODE, EXIT_PLAN_MODE, ENTER_WORKTREE, EXIT_WORKTREE, NOTEBOOK_EDIT,
		// UX
		ASK_USER_QUESTION, PUSH_NOTIFICATION,
		// Scheduling
		CRON_CREATE, CRON_LIST, CRON_DELETE, REMOTE_TRIGGER,
		// Web
		WEB_FETCH, WEB_SEARCH,
		// Others
		GREP, TREE,
	}
	seen := make(map[ToolName]bool, len(all))
	for _, n := range all {
		if n == "" {
			t.Errorf("ToolName has empty string value (probable typo in name.go)")
			continue
		}
		if seen[n] {
			t.Errorf("duplicate ToolName value %q — two constants resolve to the same wire name", n)
		}
		seen[n] = true
	}
}
