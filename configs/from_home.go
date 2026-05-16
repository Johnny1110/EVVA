package config

import (
	"os"

	"github.com/johnny1110/evva/internal/constant"
)

// setupGlobalParam ensures the global config directories exist. All
// user-tunable values are now sourced from evva-config.yml in load();
// this function only handles directory provisioning.
func setupGlobalParam(cfg *AppConfig) {
	_ = os.MkdirAll(cfg.EvvaHome, 0o755)
	_ = os.MkdirAll(cfg.EvvaHomeSkillsDir, 0o755)
}

// setupLLMProviderConfig wires per-provider credentials from the YAML
// file config. Providers with an empty api_url fall back to the
// constant's built-in default. Anthropic/DeepSeek/OpenAI need an api_key
// to be listed; Ollama is local and key-less.
func setupLLMProviderConfig(cfg *AppConfig, fc FileConfig) {
	cfg.LLMProviderConfig = map[string]LLMProviderAPIConfig{}

	register := func(provider constant.LLMProvider, fileEntry FileProviderConfig, requireKey bool) {
		//if requireKey && fileEntry.APIKey == "" {
		//	return
		//}
		url := fileEntry.APIURL
		if url == "" {
			url = provider.ApiUrl
		}
		cfg.LLMProviderConfig[provider.Name] = LLMProviderAPIConfig{
			ApiURL:    url,
			ApiSecret: fileEntry.APIKey,
			Models:    provider.Models,
		}
	}

	register(constant.OLLAMA, fc.Providers[constant.OLLAMA.Name], false)
	register(constant.ANTHROPIC, fc.Providers[constant.ANTHROPIC.Name], true)
	register(constant.DEEPSEEK, fc.Providers[constant.DEEPSEEK.Name], true)
	register(constant.OPENAI, fc.Providers[constant.OPENAI.Name], true)
}

type LLMProviderAPIConfig struct {
	ApiURL    string
	ApiSecret string
	Models    []constant.Model
}
