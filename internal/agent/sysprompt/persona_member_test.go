package sysprompt

import (
	"strings"
	"testing"
)

func TestMainPrompt_OmitSkillAuthoring(t *testing.T) {
	ctx := PromptContext{
		OS: "linux", Shell: "bash", WorkDir: "/srv/app", EvvaHome: "/home/u/.evva",
		Skills: []SkillRef{{Name: "demo", Description: "a demo skill"}},
	}
	if got := MainAgent.BuildSystemPrompt(ctx); !strings.Contains(got, "How to create a skill") {
		t.Fatalf("default main prompt must keep skill-authoring guidance")
	}
	ctx.OmitSkillAuthoring = true
	if got := MainAgent.BuildSystemPrompt(ctx); strings.Contains(got, "How to create a skill") {
		t.Fatalf("OmitSkillAuthoring must drop the authoring guidance")
	}
}
