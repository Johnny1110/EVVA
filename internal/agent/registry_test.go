package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/agent/sysprompt"
)

func TestAgentRegistry_BuiltInsAlwaysPresent(t *testing.T) {
	r, warns := BuildAgentRegistry("") // no disk catalog
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	for _, name := range []string{"evva", "explore", "plan", "general-purpose"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected built-in %q to be present", name)
		}
	}
}

func TestAgentRegistry_GetIsCaseInsensitive(t *testing.T) {
	r, _ := BuildAgentRegistry("")
	if _, ok := r.Get("EXPLORE"); !ok {
		t.Error("expected case-insensitive Get for 'EXPLORE'")
	}
	if _, ok := r.Get("General-Purpose"); !ok {
		t.Error("expected case-insensitive Get for 'General-Purpose'")
	}
}

func TestAgentRegistry_ListMainAndSubagent(t *testing.T) {
	r, _ := BuildAgentRegistry("")

	mains := r.ListMain()
	if len(mains) != 1 || mains[0].Name != "evva" {
		t.Errorf("ListMain: expected only 'evva', got %v", agentNames(mains))
	}

	subs := r.ListSubagent()
	if len(subs) != 3 {
		t.Errorf("ListSubagent: expected 3, got %v", agentNames(subs))
	}
	want := map[string]bool{"explore": true, "plan": true, "general-purpose": true}
	for _, d := range subs {
		if !want[d.Name] {
			t.Errorf("ListSubagent: unexpected %q", d.Name)
		}
	}
}

func TestAgentRegistry_LoadsDiskAgent(t *testing.T) {
	home := t.TempDir()
	agentsDir := filepath.Join(home, "agents")
	writeAgentDir(t, agentsDir, "code-reviewer",
		"You are a code-reviewer agent.\n",
		"active: [read_file, grep]\ndeferred: []\n",
		"as: [subagent]\nwhen_to_use: Reviews code carefully.\n",
	)

	r, warns := BuildAgentRegistry(home)
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	def, ok := r.Get("code-reviewer")
	if !ok {
		t.Fatal("expected code-reviewer to be loaded")
	}
	if !def.IsSubagent() {
		t.Error("expected code-reviewer to be a subagent")
	}
	if def.WhenToUse != "Reviews code carefully." {
		t.Errorf("when_to_use: got %q", def.WhenToUse)
	}

	// Disk agent must surface in ListSubagent.
	subs := r.ListSubagent()
	found := false
	for _, d := range subs {
		if d.Name == "code-reviewer" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected code-reviewer in ListSubagent, got %v", agentNames(subs))
	}
}

func TestAgentRegistry_DiskAgentShadowingBuiltinWarns(t *testing.T) {
	home := t.TempDir()
	agentsDir := filepath.Join(home, "agents")
	writeAgentDir(t, agentsDir, "explore", // colliding name
		"You are NOT the real explore agent.\n",
		"active: []\n",
		"as: [subagent]\n",
	)

	r, warns := BuildAgentRegistry(home)
	if len(warns) == 0 {
		t.Fatal("expected a warning for disk agent shadowing built-in")
	}
	if !strings.Contains(warns[0].Error(), "shadows a built-in") {
		t.Errorf("warning text: got %q", warns[0].Error())
	}

	// Built-in must still win — the disk agent's prompt is not installed.
	def, ok := r.Get("explore")
	if !ok {
		t.Fatal("expected built-in explore still present")
	}
	if def.BuildSystemPrompt == nil {
		t.Fatal("built-in explore lost its BuildSystemPrompt closure")
	}
	got := def.BuildSystemPrompt(sysprompt.PromptContext{})
	if strings.Contains(got, "NOT the real explore agent") {
		t.Error("disk agent's body leaked into the registry — built-in should have won")
	}
}

func TestAgentRegistry_MalformedDiskAgentDoesNotBreakRegistry(t *testing.T) {
	home := t.TempDir()
	agentsDir := filepath.Join(home, "agents")
	writeAgentDir(t, agentsDir, "broken", "",
		"active: []\n",
		"as: [subagent]\n",
	) // empty system_prompt.md is invalid

	r, warns := BuildAgentRegistry(home)
	if len(warns) == 0 {
		t.Fatal("expected a warning for the broken disk agent")
	}
	// Built-ins still present.
	for _, name := range []string{"evva", "explore", "plan", "general-purpose"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("built-in %q missing after broken disk-agent load", name)
		}
	}
	// Broken agent must not appear.
	if _, ok := r.Get("broken"); ok {
		t.Error("expected 'broken' to be dropped")
	}
}

// writeAgentDir is a test helper that writes an agent directory.
// Same shape as loader_test.writeAgent; duplicated to avoid cross-package
// test imports.
func writeAgentDir(t *testing.T, root, name, prompt, toolsBody, metaBody string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if prompt != "" {
		must(filepath.Join(dir, "system_prompt.md"), prompt)
	}
	if toolsBody != "" {
		must(filepath.Join(dir, "tools.yml"), toolsBody)
	}
	if metaBody != "" {
		must(filepath.Join(dir, "meta.yml"), metaBody)
	}
}

func agentNames(defs []sysprompt.AgentDefinition) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}
