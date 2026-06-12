package agent

import "testing"

func TestAgentSpec_PromptSuffixRoundTrip(t *testing.T) {
	spec := AgentSpec{Name: "p", As: []string{"main"}, SystemPrompt: "body", PromptSuffix: "## team protocol"}
	def := DefinitionFromSpec(spec)
	if def.PromptSuffix != "## team protocol" {
		t.Fatalf("DefinitionFromSpec dropped PromptSuffix: %q", def.PromptSuffix)
	}
	back := SpecFromDefinition(def)
	if back.PromptSuffix != "## team protocol" {
		t.Fatalf("SpecFromDefinition dropped PromptSuffix: %q", back.PromptSuffix)
	}
}
