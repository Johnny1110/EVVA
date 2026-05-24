package agent_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/johnny1110/evva/pkg/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/event"
	"github.com/johnny1110/evva/pkg/llm"
	"github.com/johnny1110/evva/pkg/tools"
)

// stubClient is a deterministic llm.Client a downstream test registers
// against a custom provider name. The model returns text immediately —
// no tool calls — so the agent loop terminates after one iteration.
type stubClient struct {
	name  string
	model string
}

func (s *stubClient) Name() string  { return s.name }
func (s *stubClient) Model() string { return s.model }
func (s *stubClient) SupportsDeferLoading() bool { return false }
func (s *stubClient) Complete(_ context.Context, _ []llm.Message, _ []tools.Tool) (llm.Response, error) {
	return llm.Response{Content: "downstream: " + s.model}, nil
}
func (s *stubClient) Stream(_ context.Context, _ []llm.Message, _ []tools.Tool, _ llm.ChunkSink) (llm.Response, error) {
	return s.Complete(nil, nil, nil)
}
func (s *stubClient) Apply(...llm.Option) {}

// recordingSink captures every event the agent emits so the test can
// assert on kinds + ordering without involving a TUI.
type recordingSink struct {
	mu     sync.Mutex
	events []event.Kind
	text   string
}

func (r *recordingSink) Emit(e event.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e.Kind)
	if e.Kind == event.KindText && e.Text != nil {
		r.text = e.Text.Text
	}
}

func (r *recordingSink) seen(kinds ...event.Kind) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	have := map[event.Kind]bool{}
	for _, k := range r.events {
		have[k] = true
	}
	for _, k := range kinds {
		if !have[k] {
			return false
		}
	}
	return true
}

// pingTool is the canonical downstream custom tool: takes input, ignores
// it, returns a deterministic string. Demonstrates pkg/tools.State
// reaching downstream code via WithCustomTool.
type pingTool struct{ name string }

func (p pingTool) Name() string            { return p.name }
func (pingTool) Description() string       { return "ping" }
func (pingTool) Schema() json.RawMessage   { return json.RawMessage(`{"type":"object"}`) }
func (pingTool) Execute(_ context.Context, _ *slog.Logger, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "pong"}, nil
}

// TestDownstream_EndToEnd is the proof point for Phase 13: a downstream
// app constructs an agent purely from pkg/ types, registers a custom
// LLM provider + custom tool, runs one turn, and observes events
// through a custom sink. No internal/* imports needed in this file.
func TestDownstream_EndToEnd(t *testing.T) {
	// Step 1 — register a custom LLM provider on the default registry.
	providerName := "downstream_stub"
	if !llm.DefaultRegistry().Has(providerName) {
		err := llm.DefaultRegistry().Register(providerName, func(cfg llm.APIConfig, model string, opts ...llm.Option) (llm.Client, error) {
			return &stubClient{name: providerName, model: model}, nil
		})
		if err != nil {
			t.Fatalf("register provider: %v", err)
		}
	}

	// Step 2 — install API credentials on a fresh Config so buildLLMClient
	// finds the provider entry. Use t.TempDir for AppHome / WorkDir so the
	// test doesn't write to the user's real home directory.
	cfg, err := config.Load(config.LoadOptions{
		AppName: "downstream_test",
		AppHome: t.TempDir(),
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.LLMProviderConfig[providerName] = config.APIConfig{ApiURL: "http://stub", ApiSecret: "fake"}

	// Step 3 — build a profile that targets the stub provider plus the
	// custom tool. ActiveTools is empty so the model isn't dispatched
	// any built-ins for this proof; WithCustomTool will append pingTool.
	customToolName := tools.ToolName("downstream_ping")
	prof, err := agent.NewProfile("downstream", "you are a stub", nil, providerName, constant.Model("stub-model-1"), agent.ProfileOptions{})
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}

	sink := &recordingSink{}

	// Step 4 — assemble the agent with the public options API.
	ag, err := agent.NewWithProfile(prof,
		agent.WithConfig(cfg),
		agent.WithSink(sink),
		agent.WithMaxIterations(5),
		agent.WithCustomTool(customToolName, func(s tools.State) (tools.Tool, error) {
			return pingTool{name: string(customToolName)}, nil
		}),
		agent.WithPermissionMode(agent.PermissionBypass),
	)
	if err != nil {
		t.Fatalf("agent.NewWithProfile: %v", err)
	}

	// Step 5 — run one turn and verify events landed.
	resp, err := ag.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(resp, "downstream:") {
		t.Errorf("expected stub response in run output, got %q", resp)
	}

	if !sink.seen(event.KindRunEnd, event.KindText) {
		t.Errorf("missing run lifecycle events; got %v", sink.events)
	}
	if !strings.Contains(sink.text, "downstream:") {
		t.Errorf("sink should have captured the model text; got %q", sink.text)
	}

	// Step 6 — confirm the agent reports the right model + persona.
	if got, want := ag.Model(), "stub-model-1"; got != want {
		t.Errorf("Model: got %q, want %q", got, want)
	}
}
