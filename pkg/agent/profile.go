package agent

import (
	agent_impl "github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/llm"
	"github.com/johnny1110/evva/pkg/tools"
)

// Profile is the public alias for the internal agent profile. Downstream
// apps construct one via NewProfile (or via an evva-bundled persona
// constructor like Main when running inside the evva binary).
//
// The struct's fields are accessible so downstream tests and tools can
// inspect a profile, but the constructor remains the recommended entry
// point — it normalizes optional fields and handles the AgentType
// classification (which is internal evva concept downstream apps don't
// need to think about).
type Profile = agent_impl.Profile

// ProfileOptions tunes NewProfile. Zero-value fields fall back to
// sensible defaults: streaming off, no deferred tools, no extra llm
// options beyond the system prompt.
type ProfileOptions struct {
	// DeferredTools is the names the LLM sees by description-only; the
	// model must invoke TOOL_SEARCH to fetch their full schemas. Empty
	// means every active tool is eagerly available.
	DeferredTools []tools.ToolName

	// Stream selects the streaming completion path. Defaults to false
	// — most downstream apps want the buffered Complete path until they
	// build a streaming UI.
	Stream bool

	// LLMOptions are forwarded to the LLM client. The system prompt is
	// already supplied via the SystemPrompt argument to NewProfile and
	// does not need to be repeated here.
	LLMOptions []llm.Option
}

// NewProfile builds a Profile a downstream app can pass into
// NewWithProfile. The system prompt is wrapped into an llm.WithSystem
// option automatically.
//
// providerName must match a name registered on pkg/llm.DefaultRegistry
// ("anthropic", "deepseek", "ollama", or a downstream-registered name).
// model is the model id sent to the provider; empty is allowed but
// downstream factories typically expect a concrete id.
//
// The profile is classified as GENERAL_PURPOSE — evva's internal type
// system reserves MAIN/EXPLORE/PLAN for the bundled personas. Downstream
// apps that want richer behavior (memory injection, skills, plan-mode)
// should ship an on-disk agent definition under <AppHome>/agents/
// instead of constructing a Profile in code.
//
// providerName does not need to be in pkg/constant.GetAllProviders() —
// downstream apps register their own provider via
// pkg/llm.DefaultRegistry().Register and pass the same name here. When
// the name isn't in the bundled constants, an LLMProvider stub is
// synthesised on the fly so the rest of the agent loop has a value to
// log + introspect.
func NewProfile(name, systemPrompt string, activeTools []tools.ToolName, providerName, model string, opts ProfileOptions) (Profile, error) {
	if providerName == "" {
		return Profile{}, &unknownProviderError{name: providerName}
	}
	provider, ok := constant.GetProvider(providerName)
	if !ok {
		provider = constant.LLMProvider{Name: providerName}
	}

	llmOpts := append([]llm.Option{}, opts.LLMOptions...)
	if systemPrompt != "" {
		llmOpts = append(llmOpts, llm.WithSystem(systemPrompt))
	}

	return Profile{
		Type:          agent_impl.GENERAL_PURPOSE,
		SystemPrompt:  systemPrompt,
		ActiveTools:   activeTools,
		DeferredTools: opts.DeferredTools,
		LLMProvider:   provider,
		LLMModel:      constant.Model(model),
		LLMOptions:    llmOpts,
		Stream:        opts.Stream,
	}, nil
}

type unknownProviderError struct{ name string }

func (e *unknownProviderError) Error() string {
	if e.name == "" {
		return "agent: provider name is required"
	}
	return "agent: unknown provider " + e.name
}
