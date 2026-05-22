package llm_test

import (
	"context"
	"fmt"

	"github.com/johnny1110/evva/pkg/llm"
	"github.com/johnny1110/evva/pkg/tools"
)

// nullClient is a deterministic stand-in for examples. Replace with a
// real provider in production downstream code (or import
// `_ "github.com/johnny1110/evva/pkg/llm/builtins"` for the bundled
// Anthropic / DeepSeek / Ollama clients).
type nullClient struct{ model string }

func (n *nullClient) Name() string  { return "null" }
func (n *nullClient) Model() string { return n.model }
func (n *nullClient) Complete(context.Context, []llm.Message, []tools.Tool) (llm.Response, error) {
	return llm.Response{Content: "ok"}, nil
}
func (n *nullClient) Stream(ctx context.Context, m []llm.Message, t []tools.Tool, _ llm.ChunkSink) (llm.Response, error) {
	return n.Complete(ctx, m, t)
}
func (*nullClient) Apply(...llm.Option) {}

// ExampleRegistry_Register demonstrates the canonical "add a custom
// LLM provider" pattern. Pick a unique name, supply a ClientFactory
// that wraps your provider's constructor, and the agent layer can
// then build clients for that provider by name through the same
// machinery the bundled providers use.
func ExampleRegistry_Register() {
	if err := llm.DefaultRegistry().Register("null-example",
		func(_ llm.APIConfig, model string, _ ...llm.Option) (llm.Client, error) {
			return &nullClient{model: model}, nil
		},
	); err != nil {
		fmt.Println("register error:", err)
		return
	}
	client, err := llm.DefaultRegistry().Build("null-example", "v0",
		llm.APIConfig{}, nil)
	if err != nil {
		fmt.Println("build error:", err)
		return
	}
	fmt.Println("provider:", client.Name(), "model:", client.Model())
	// Output: provider: null model: v0
}
