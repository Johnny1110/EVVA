package meta

import (
	"context"
	"encoding/json"
	"testing"
)

// stubSpawner is a SubagentSpawner that returns a fixed type list and
// never actually spawns. Used to drive the AGENT tool's dynamic schema
// without standing up a real agent.
type stubSpawner struct {
	types []string
}

func (s *stubSpawner) Spawn(_ context.Context, _ SpawnRequest) (string, error) {
	return "", nil
}

func (s *stubSpawner) SubagentTypes() []string {
	return s.types
}

func TestAgentTool_Schema_FallsBackWhenLookupNil(t *testing.T) {
	tool := NewAgent(nil, NewSpawnGroup())
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	props := schema["properties"].(map[string]any)
	st := props["subagent_type"].(map[string]any)
	enum := st["enum"].([]any)
	got := make([]string, len(enum))
	for i, v := range enum {
		got[i] = v.(string)
	}
	if len(got) != 3 || got[0] != "explore" || got[1] != "plan" || got[2] != "general-purpose" {
		t.Errorf("fallback enum: want [explore plan general-purpose], got %v", got)
	}
}

func TestAgentTool_Schema_PullsDynamicEnumFromSpawner(t *testing.T) {
	stub := &stubSpawner{types: []string{"explore", "general-purpose", "nono", "reviewer"}}
	tool := NewAgent(func() SubagentSpawner { return stub }, NewSpawnGroup())
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	props := schema["properties"].(map[string]any)
	st := props["subagent_type"].(map[string]any)
	enum := st["enum"].([]any)
	if len(enum) != 4 {
		t.Fatalf("dynamic enum length: want 4, got %d (%v)", len(enum), enum)
	}
	want := map[string]bool{"explore": true, "general-purpose": true, "nono": true, "reviewer": true}
	for _, v := range enum {
		s := v.(string)
		if !want[s] {
			t.Errorf("unexpected enum entry %q", s)
		}
	}
}

func TestAgentTool_Schema_EmptyDynamicListFallsBack(t *testing.T) {
	stub := &stubSpawner{types: nil}
	tool := NewAgent(func() SubagentSpawner { return stub }, NewSpawnGroup())
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	props := schema["properties"].(map[string]any)
	st := props["subagent_type"].(map[string]any)
	enum := st["enum"].([]any)
	if len(enum) != 3 {
		t.Errorf("empty dynamic enum should fall back to 3 builtins, got %d", len(enum))
	}
}
