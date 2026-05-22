package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/johnny1110/evva/pkg/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/event"
	"github.com/johnny1110/evva/pkg/llm"
	"github.com/johnny1110/evva/pkg/tools"
)

// exampleConfig loads a throwaway Config under /tmp so the example
// works without touching the user's real ~/.evva home.
func exampleConfig() *config.Config {
	tmp := filepath.Join(os.TempDir(), "evva-example-agent")
	cfg, _ := config.Load(config.LoadOptions{
		AppName: "example",
		AppHome: tmp,
		WorkDir: tmp,
	})
	return cfg
}

// echoLLM is a minimal stand-in for examples. Production callers
// import `_ "github.com/johnny1110/evva/pkg/llm/builtins"` to get
// Anthropic / DeepSeek / Ollama pre-registered instead.
type echoLLM struct{ model string }

func (e *echoLLM) Name() string  { return "echo" }
func (e *echoLLM) Model() string { return e.model }
func (e *echoLLM) Complete(_ context.Context, msgs []llm.Message, _ []tools.Tool) (llm.Response, error) {
	return llm.Response{Content: "echo: ok"}, nil
}
func (e *echoLLM) Stream(ctx context.Context, m []llm.Message, t []tools.Tool, _ llm.ChunkSink) (llm.Response, error) {
	return e.Complete(ctx, m, t)
}
func (*echoLLM) Apply(...llm.Option) {}

// examplePingTool answers any input with "pong". Demonstrates the public
// pkg/tools.Tool interface a downstream consumer satisfies.
type examplePingTool struct{}

func (examplePingTool) Name() string            { return "ping" }
func (examplePingTool) Description() string     { return "respond with pong" }
func (examplePingTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (examplePingTool) Execute(_ context.Context, _ *slog.Logger, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{Content: "pong"}, nil
}

// ExampleNewProfile shows the basic Profile construction. Tool names
// come from pkg/tools; the model arg takes a typed constant.Model so
// typos become compile errors. Tool kit, persona, prompt, provider,
// model — that's everything the Profile carries.
func ExampleNewProfile() {
	prof, _ := agent.NewProfile(
		"my-agent",
		"You are a brisk assistant.",
		[]tools.ToolName{tools.READ_FILE, tools.BASH},
		"deepseek",
		constant.DEEPSEEK_V4_PRO,
		agent.ProfileOptions{},
	)
	fmt.Println("prompt:", prof.SystemPrompt)
	fmt.Println("provider:", prof.LLMProvider.Name)
	fmt.Println("model:", string(prof.LLMModel))
	// Output:
	// prompt: You are a brisk assistant.
	// provider: deepseek
	// model: deepseek-v4-pro
}

// ExampleNewWithProfile builds a complete agent and runs one turn.
// The flow: register a provider, build a Config with credentials,
// build a Profile, construct the agent with options, then call Run.
//
// WithHeadlessBypass is the Phase 19c convenience — required for
// downstream apps that don't render an approval UI, otherwise every
// tool call needing approval would be auto-denied.
func ExampleNewWithProfile() {
	// 1. Register an echo provider for the example.
	if !llm.DefaultRegistry().Has("echo-example") {
		_ = llm.DefaultRegistry().Register("echo-example",
			func(_ llm.APIConfig, model string, _ ...llm.Option) (llm.Client, error) {
				return &echoLLM{model: model}, nil
			})
	}

	// 2. Build a config with credentials. Real apps use config.Load
	//    with their own AppName / AppHome; this example uses an inline
	//    Config so the example doesn't pollute the filesystem.
	cfg := exampleConfig()
	_ = cfg.SetProviderCredentials("echo-example", "http://example", "n/a")

	// 3. Build the Profile + Agent.
	prof, _ := agent.NewProfile("example", "you echo",
		nil, "echo-example", constant.Model("v0"), agent.ProfileOptions{})

	collected := []string{}
	sink := event.SinkFunc(func(e event.Event) {
		if e.Kind == event.KindText && e.Text != nil {
			collected = append(collected, e.Text.Text)
		}
	})

	ag, err := agent.NewWithProfile(prof,
		agent.WithConfig(cfg),
		agent.WithSink(sink),
		agent.WithMaxIterations(3),
		agent.WithHeadlessBypass(),
		agent.WithCustomTool("ping", func(tools.State) (tools.Tool, error) { return examplePingTool{}, nil }),
	)
	if err != nil {
		fmt.Println("construct error:", err)
		return
	}

	resp, _ := ag.Run(context.Background(), "hi")
	fmt.Println("final:", resp)
	// Output: final: echo: ok
}

// ExampleWithHeadlessBypass shows the named option for the bypass
// permission mode. Equivalent to WithPermissionModeTyped(PermissionBypass)
// but more discoverable — non-interactive hosts should always use this.
func ExampleWithHeadlessBypass() {
	_ = agent.WithHeadlessBypass()
	fmt.Println("see godoc for security notes")
	// Output: see godoc for security notes
}
