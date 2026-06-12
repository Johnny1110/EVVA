package agent

import "testing"

func TestRegistry_PromptSuffixRoundTrip(t *testing.T) {
	reg := NewAgentRegistry()
	reg.Register(AgentDefinition{Name: "p", As: []string{"main"}, SystemPrompt: "b", PromptSuffix: "## team protocol"})
	got, ok := reg.Get("p")
	if !ok {
		t.Fatal("persona p not found")
	}
	if got.PromptSuffix != "## team protocol" {
		t.Fatalf("public registry dropped PromptSuffix: %q", got.PromptSuffix)
	}
}
