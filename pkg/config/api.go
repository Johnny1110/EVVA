package config

import "github.com/johnny1110/evva/pkg/constant"

// APIConfig carries the per-provider credentials an LLM client needs to talk
// to its backend. The host (cmd/evva or a downstream consumer) constructs
// one APIConfig per provider from whatever config source it uses (YAML,
// env vars, secret manager) and passes it to the registry-resolved
// ClientFactory in pkg/llm.
//
// Defined in pkg/config rather than pkg/llm to avoid a cycle:
// pkg/llm imports pkg/tools, pkg/tools imports pkg/config (for the
// State.Config() return type). pkg/config sits at the bottom and is
// imported by both pkg/llm (via alias) and pkg/tools.
type APIConfig struct {
	ApiURL    string
	ApiSecret string
	Models    []constant.Model
}
