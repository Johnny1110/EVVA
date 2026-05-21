package toolset

import (
	"encoding/json"
	"testing"

	"github.com/johnny1110/evva/pkg/tools"
)

// TestAllToolSchemasAreValidJSON guards against the class of bug where a
// tool's Schema() returns a json.RawMessage with broken JSON. Such bugs
// don't fail at build time (raw bytes), don't fail at construction
// (Schema isn't called), but blow up at LLM-request-marshal time with
// "invalid character …" — far from the source.
//
// This test builds every known tool name and parses its Schema() as JSON.
// Any breakage is caught at `go test ./...` instead of at runtime.
func TestAllToolSchemasAreValidJSON(t *testing.T) {
	names := []tools.ToolName{
		// fs
		tools.READ_FILE, tools.WRITE_FILE, tools.EDIT_FILE,
		// shell
		tools.BASH, tools.GREP, tools.TREE,
		// meta
		tools.AGENT, tools.TOOL_SEARCH, tools.SKILL, tools.SCHEDULE_WAKEUP,
		// todo
		tools.TODO_WRITE,
		// monitor / mode / notebook
		tools.MONITOR,
		tools.ENTER_PLAN_MODE, tools.EXIT_PLAN_MODE,
		tools.ENTER_WORKTREE, tools.EXIT_WORKTREE,
		tools.NOTEBOOK_EDIT,
		// cron / web / ux
		tools.CRON_CREATE, tools.CRON_LIST, tools.CRON_DELETE, tools.REMOTE_TRIGGER,
		tools.WEB_FETCH, tools.WEB_SEARCH,
		tools.ASK_USER_QUESTION, tools.PUSH_NOTIFICATION,
	}

	state := &ToolState{}
	for _, n := range names {
		built, err := Build([]tools.ToolName{n}, state)
		if err != nil {
			t.Errorf("%s: build failed: %v", n, err)
			continue
		}
		schema := built[0].Schema()
		if len(schema) == 0 {
			// Empty schema is allowed — llm.ToolSchema substitutes a
			// permissive default.
			continue
		}
		var v any
		if err := json.Unmarshal(schema, &v); err != nil {
			t.Errorf("%s: schema is invalid JSON: %v\nschema:\n%s", n, err, schema)
		}
	}
}
