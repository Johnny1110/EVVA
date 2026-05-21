package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/tools"
)

// echoTool is the canonical downstream-style custom tool: takes a payload,
// returns it verbatim. Demonstrates that custom tools see the same
// pkg/tools.State + logger + ctx contract as built-ins. Name is
// parametrised so each test gets a unique registry entry — the global
// pkg/toolset.DefaultRegistry rejects duplicates and tests must not
// share names.
type echoTool struct{ name string }

func (e echoTool) Name() string            { return e.name }
func (echoTool) Description() string       { return "echoes its input back" }
func (echoTool) Schema() json.RawMessage   { return json.RawMessage(`{"type":"object"}`) }
func (echoTool) Execute(_ context.Context, _ *slog.Logger, in json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "echo: " + string(in)}, nil
}

// TestWithCustomTool_RegistersAndExposes builds an agent against a tiny
// profile and threads a custom tool through WithCustomTool. The tool is
// expected to land in the agent's active set so the LLM can invoke it,
// proving the downstream extension path end-to-end.
func TestWithCustomTool_RegistersAndExposes(t *testing.T) {
	// Use a unique name so re-running the suite doesn't trip the
	// "already registered" idempotency guard on the global registry.
	name := tools.ToolName("test_echo_" + t.Name())

	prof := Profile{
		Type:        GENERAL_PURPOSE,
		ActiveTools: []tools.ToolName{},
		LLMProvider: constant.ANTHROPIC,
		LLMModel:    constant.SONNET_4_6,
	}
	withProviderAPI(t, constant.ANTHROPIC.Name, config.APIConfig{
		ApiURL:    constant.ANTHROPIC.ApiUrl,
		ApiSecret: "fake-key",
	})

	a, err := New(nil, prof,
		WithCustomTool(name, func(s tools.State) (tools.Tool, error) {
			return echoTool{name: string(name)}, nil
		}),
	)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}

	tool, ok := a.active[string(name)]
	if !ok {
		t.Fatalf("custom tool %q missing from active set; have %v", name, mapKeys(a.active))
	}
	if !strings.Contains(tool.Description(), "echoes") {
		t.Errorf("unexpected description: %q", tool.Description())
	}

	res, err := tool.Execute(context.Background(), tools.NopLogger(), json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasPrefix(res.Content, "echo: ") {
		t.Errorf("unexpected echo result: %q", res.Content)
	}
}

func mapKeys(m map[string]tools.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
