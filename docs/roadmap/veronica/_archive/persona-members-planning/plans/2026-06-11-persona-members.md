# Persona Members (RP-29) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a swarm manifest member reference a registry main-tier persona (`persona: <name>`) so the persona joins the swarm as leader or worker with its full prompt/tools/skills plus the swarm team protocol; then onboard `evva` into the sunday swarm as resident engineer (config-only, second repo).

**Architecture:** New `PromptSuffix` field rides the AgentDefinition chain (sysprompt ↔ AgentSpec ↔ pkg DTO) so the swarm can append its team protocol to internally-assembled prompts and every prompt re-render keeps it. `agentdef` learns a persona member shape (no disk dir); `space.registerDef` composes the persona def from the space registry. Swarm-resident (`LongRunning`) profiles drop solo self-scheduling tools. Skills merge five layers via a new public `LoadSkillCatalog`.

**Tech Stack:** Go 1.x (EVVA repo, branch `feature/persona-members`), YAML/Markdown only in SUNDAY repo (branch `main`).

**Specs:** `EVVA/docs/superpowers/specs/2026-06-11-persona-members-design.md` and `SUNDAY/docs/superpowers/specs/2026-06-11-evva-engineer-onboarding-design.md`.

**Conventions for every task below:**
- Repo for Tasks 1–12: `/workspace/EVVA` on branch `feature/persona-members`. Tasks 13–14: `/workspace/SUNDAY` on `main`.
- After each implementation step run `go build ./...`; after each task run the named package tests AND `gofmt -l internal pkg` (expect no output). Test commands run from `/workspace/EVVA`.
- Commit after every task with the message given in the task.

---

### Task 1: pkg/skill — exported LoadDir + SourceSwarm

**Files:**
- Modify: `pkg/skill/registry.go` (Source constants block ~line 45; below `LoadRegistry` ~line 176)
- Test: `pkg/skill/registry_persona_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, root, name, title string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+" "+title+"\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDir_OverridesAndLabels(t *testing.T) {
	home, extra := t.TempDir(), t.TempDir()
	writeSkill(t, home, "alpha", "home version")
	writeSkill(t, extra, "alpha", "swarm version")
	writeSkill(t, extra, "beta", "swarm only")

	r, err := LoadRegistry(home, "")
	if err != nil {
		t.Fatal(err)
	}
	r.LoadDir(extra, SourceSwarm)

	var alpha, beta *SkillMeta
	for _, m := range r.List() {
		m := m
		switch m.Name {
		case "alpha":
			alpha = &m
		case "beta":
			beta = &m
		}
	}
	if alpha == nil || beta == nil {
		t.Fatalf("want alpha+beta in registry, got %v", r.List())
	}
	if alpha.Source != SourceSwarm || alpha.Description != "swarm version" {
		t.Fatalf("alpha not overridden by swarm dir: %+v", *alpha)
	}
	if len(r.Warnings) == 0 {
		t.Fatalf("override should record a shadow warning")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/skill/ -run TestLoadDir_OverridesAndLabels -v`
Expected: FAIL — `r.LoadDir undefined` and `undefined: SourceSwarm` (compile error).

- [ ] **Step 3: Implement**

In the `const` block that defines `SourceHome` / `SourceWorkDir` / `SourceProgrammatic` / `SourceBundled`, add:

```go
	// SourceSwarm marks a skill loaded from a swarm space's extra dirs (the
	// space-shared skills dir or a member's own skills dir) when composing a
	// persona member's catalog (RP-29). Same precedence rule as any later
	// loadDir call: it overrides earlier-loaded tiers and records a warning.
	SourceSwarm SkillSource = "swarm"
```

Below `LoadRegistry`, add the exported wrapper:

```go
// LoadDir scans one extra skills root into an existing registry, labeling
// entries with src. Later calls override earlier entries of the same name
// (the LoadRegistry precedence rule); a missing dir is a no-op. Public so a
// host can layer extra catalogs — e.g. a swarm space overlaying its shared
// and member-local skill dirs onto a persona's own catalog (RP-29).
func (r *Registry) LoadDir(root string, src SkillSource) {
	r.loadDir(root, src)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/skill/ -v`
Expected: PASS (all package tests).

- [ ] **Step 5: Commit**

```bash
cd /workspace/EVVA && git add pkg/skill/ && git commit -m "feat(skill): exported Registry.LoadDir + SourceSwarm for layered catalogs"
```

---

### Task 2: sysprompt — PromptSuffix field + OmitSkillAuthoring gate

**Files:**
- Modify: `internal/agent/sysprompt/agent_def.go` (AgentDefinition struct, after `LongRunning` ~line 66)
- Modify: `internal/agent/sysprompt/sysprompt.go` (PromptContext struct, after `OmitDate` ~line 62)
- Modify: `internal/agent/sysprompt/main_agent.go:67` (`skillsSection(ctx.Skills, false)`)
- Test: `internal/agent/sysprompt/main_agent_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `internal/agent/sysprompt/main_agent_test.go`:

```go
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
```

(Add `"strings"` to the test file imports if absent.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/sysprompt/ -run TestMainPrompt_OmitSkillAuthoring -v`
Expected: FAIL — `ctx.OmitSkillAuthoring undefined` (compile error).

- [ ] **Step 3: Implement**

`agent_def.go` — add after the `LongRunning` field:

```go
	// PromptSuffix, when non-empty, is appended verbatim (after one blank
	// line) to the END of the fully composed system prompt — built-in and
	// disk-composed paths alike. It is the seam a swarm uses to attach its
	// team protocol to a persona whose prompt is assembled internally and so
	// cannot be pre-concatenated into a body string (RP-29). Living on the
	// definition (not an agent option) means every prompt re-render —
	// ReloadSkills, MCP discovery, SwitchProfile — re-reads it from the
	// registry and keeps it.
	PromptSuffix string
```

`sysprompt.go` — add after the `OmitDate` field:

```go
	// OmitSkillAuthoring drops the "How to create a skill" guidance from the
	// main prompt's skills section. Long-running swarm-resident personas set
	// it (via AgentDefinition.LongRunning) for the same reason disk swarm
	// members do (RP-10-3): a member with write/bash should not be invited to
	// author SKILL.md mid-run.
	OmitSkillAuthoring bool
```

`main_agent.go:67` — change:

```go
		skillsSection(ctx.Skills, ctx.OmitSkillAuthoring),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/sysprompt/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/sysprompt/ && git commit -m "feat(sysprompt): AgentDefinition.PromptSuffix + OmitSkillAuthoring gate"
```

---

### Task 3: PromptSuffix through AgentSpec and the public DTO

**Files:**
- Modify: `internal/agent/registry.go` (AgentSpec struct ~line 17; `DefinitionFromSpec` ~line 34; `SpecFromDefinition` ~line 55)
- Modify: `pkg/agent/persona.go` (AgentDefinition struct after `LongRunning`; `toSpec`; `definitionFromSpec`)
- Test: `internal/agent/registry_test.go` (append), `pkg/agent/persona_suffix_test.go` (new)

- [ ] **Step 1: Write the failing tests**

Append to `internal/agent/registry_test.go`:

```go
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
```

Create `pkg/agent/persona_suffix_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/ -run TestAgentSpec_PromptSuffixRoundTrip -v ; go test ./pkg/agent/ -run TestRegistry_PromptSuffixRoundTrip -v`
Expected: both FAIL with `unknown field PromptSuffix` (compile errors).

- [ ] **Step 3: Implement**

`internal/agent/registry.go` — `AgentSpec`: add `PromptSuffix string` after `LongRunning bool`. In `DefinitionFromSpec` add `PromptSuffix: spec.PromptSuffix,`; in `SpecFromDefinition` add `PromptSuffix: def.PromptSuffix,`.

`pkg/agent/persona.go` — `AgentDefinition`: add after `LongRunning`:

```go
	// PromptSuffix, when non-empty, is appended after the persona's fully
	// composed system prompt. The swarm subsystem sets it on persona members
	// to attach the team protocol; ordinary personas leave it empty.
	PromptSuffix string
```

In `toSpec` add `PromptSuffix: d.PromptSuffix,`; in `definitionFromSpec` add `PromptSuffix: s.PromptSuffix,`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/ -run PromptSuffix -v && go test ./pkg/agent/ -run PromptSuffix -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/registry.go pkg/agent/persona.go internal/agent/registry_test.go pkg/agent/persona_suffix_test.go
git commit -m "feat(agent): thread PromptSuffix through AgentSpec and the public persona DTO"
```

---

### Task 4: profiles.go — mainProfileForDef (suffix, OmitDate, solo-tool strip)

**Files:**
- Modify: `internal/agent/profiles.go` (`mainProfile` ~line 148; `resolveMainProfileWithExtra` "evva" branch ~line 263; new helpers at file end)
- Test: `internal/agent/persona_member_profile_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
package agent

import (
	"slices"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/agent/sysprompt"
	"github.com/johnny1110/evva/internal/memdir"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/tools"
	"github.com/johnny1110/evva/pkg/tools/alarm"
	"github.com/johnny1110/evva/pkg/tools/cron"
)

func suffixTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(config.LoadOptions{AppName: "t", AppHome: t.TempDir(), WorkDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestMainProfileForDef_SwarmResident(t *testing.T) {
	cfg := suffixTestConfig(t)
	def := sysprompt.MainAgent
	def.LongRunning = true
	def.PromptSuffix = "## SWARM PROTOCOL MARKER"

	p := mainProfileForDef(def, cfg, cfg.DefaultProvider, cfg.DefaultModel, []sysprompt.SkillRef{}, memdir.Snapshot{}, nil, nil)

	if !strings.HasSuffix(p.SystemPrompt, "## SWARM PROTOCOL MARKER") {
		t.Fatalf("suffix must terminate the prompt; tail: %q", p.SystemPrompt[len(p.SystemPrompt)-80:])
	}
	if strings.Contains(p.SystemPrompt, "- Today:") {
		t.Fatalf("LongRunning must omit the date line")
	}
	if strings.Contains(p.SystemPrompt, "How to create a skill") {
		t.Fatalf("LongRunning must omit skill-authoring guidance")
	}
	for _, n := range slices.Concat(alarm.Names(), cron.Names()) {
		if slices.Contains(p.DeferredTools, n) || slices.Contains(p.ActiveTools, n) {
			t.Fatalf("swarm-resident profile must not carry solo scheduling tool %q", n)
		}
	}
	if slices.Contains(p.ActiveTools, tools.SCHEDULE_WAKEUP) {
		t.Fatalf("swarm-resident profile must not carry schedule_wakeup")
	}
}

func TestMainProfile_SoloUnchanged(t *testing.T) {
	cfg := suffixTestConfig(t)
	p := Main(cfg, cfg.DefaultProvider, cfg.DefaultModel, []sysprompt.SkillRef{}, memdir.Snapshot{}, nil)
	if strings.Contains(p.SystemPrompt, "SWARM PROTOCOL") {
		t.Fatalf("solo prompt must carry no swarm suffix")
	}
	if !strings.Contains(p.SystemPrompt, "- Today:") {
		t.Fatalf("solo prompt must keep the date line")
	}
	if !slices.Contains(p.DeferredTools, tools.ALARM_CREATE) {
		t.Fatalf("solo profile must keep solo alarm tools")
	}
	if !slices.Contains(p.ActiveTools, tools.SCHEDULE_WAKEUP) {
		t.Fatalf("solo profile must keep schedule_wakeup")
	}
}

func TestResolveMainProfile_EvvaSuffixFromRegistry(t *testing.T) {
	cfg := suffixTestConfig(t)
	reg, _ := BuildAgentRegistry(t.TempDir())
	def, ok := reg.Get("evva")
	if !ok {
		t.Fatal("built-in evva missing from registry")
	}
	def.LongRunning = true
	def.PromptSuffix = "## SWARM PROTOCOL MARKER"
	reg.Register(def)

	prof, err := resolveMainProfileWithExtra(cfg, reg, "evva", []sysprompt.SkillRef{}, memdir.Snapshot{}, nil, cfg.DefaultProvider, cfg.DefaultModel, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(prof.SystemPrompt, "## SWARM PROTOCOL MARKER") {
		t.Fatalf("registry-composed evva def must surface its suffix")
	}
	if strings.Count(prof.SystemPrompt, "## SWARM PROTOCOL MARKER") != 1 {
		t.Fatalf("suffix must appear exactly once")
	}
	// Re-resolve (the ReloadSkills / MCP re-render path) — suffix must survive.
	again, err := resolveMainProfileWithExtra(cfg, reg, "evva", []sysprompt.SkillRef{}, memdir.Snapshot{}, nil, cfg.DefaultProvider, cfg.DefaultModel, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(again.SystemPrompt, "## SWARM PROTOCOL MARKER") != 1 {
		t.Fatalf("re-render must keep exactly one suffix")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run 'TestMainProfileForDef_SwarmResident|TestResolveMainProfile_EvvaSuffix' -v`
Expected: FAIL — `undefined: mainProfileForDef` (compile error).

- [ ] **Step 3: Implement**

In `profiles.go`, replace the body of `mainProfile` with a delegation and add the def-aware variant plus helpers. The existing `mainProfile` body moves into `mainProfileForDef` with four additions (marked NEW):

```go
// mainProfile is Main with an extra slice of deferred tool names folded in
// (MCP-discovered mcp__<server>__<tool> names). Delegates to mainProfileForDef
// with the pristine built-in definition, so the solo path is byte-identical.
func mainProfile(cfg *config.Config, provider constant.LLMProvider, model constant.Model, skills []sysprompt.SkillRef, mem memdir.Snapshot, options []llm.Option, extraDeferred []tools.ToolName) Profile {
	return mainProfileForDef(sysprompt.MainAgent, cfg, provider, model, skills, mem, options, extraDeferred)
}

// mainProfileForDef builds the built-in evva profile honoring the swarm-set
// flags on def: LongRunning (date-free prompt, no skill-authoring guidance,
// no solo self-scheduling tools) and PromptSuffix (the swarm team protocol,
// appended after the composed prompt). def contributes ONLY those flags —
// the prompt fragments and tool kit are always the built-in Main ones
// (sysprompt.MainAgent), never def's body: a swarm-registered "evva" def
// carries an empty body by design (RP-29).
func mainProfileForDef(def sysprompt.AgentDefinition, cfg *config.Config, provider constant.LLMProvider, model constant.Model, skills []sysprompt.SkillRef, mem memdir.Snapshot, options []llm.Option, extraDeferred []tools.ToolName) Profile {
	// ... existing mainProfile body verbatim, with these four changes:
}
```

The four changes inside the moved body:

1. After `deferredTools = append(deferredTools, extraDeferred...)`:

```go
	if def.LongRunning {
		activeTools = stripTools(activeTools, soloSchedulingTools())
		deferredTools = stripTools(deferredTools, soloSchedulingTools())
	}
```

2. After `ctx := detectContext(cfg)`:

```go
	ctx.OmitDate = def.LongRunning
	ctx.OmitSkillAuthoring = def.LongRunning
```

3. Replace `sp := sysprompt.MainAgent.BuildSystemPrompt(ctx)` with:

```go
	sp := sysprompt.MainAgent.BuildSystemPrompt(ctx)
	if def.PromptSuffix != "" {
		sp += "\n\n" + def.PromptSuffix
	}
```

4. No other line changes (`options = append(options, llm.WithSystem(sp))` etc. stay).

In `resolveMainProfileWithExtra`, change the evva branch (~line 263):

```go
	if def.Name == "evva" {
		return mainProfileForDef(def, cfg, provider, model, skills, mem, options, extraDeferred), nil
	}
```

At the end of the file add:

```go
// soloSchedulingTools are the self-scheduling tools a swarm-resident persona
// must not carry: the swarm injects alarm_set/alarm_clear (and the leader's
// schedule_set), and a parallel solo scheduler would create wake sources the
// roster cannot see. Keyed off AgentDefinition.LongRunning (RP-29).
func soloSchedulingTools() []tools.ToolName {
	out := slices.Concat(alarm.Names(), cron.Names())
	return append(out, tools.SCHEDULE_WAKEUP)
}

// stripTools returns list minus drop, always as a fresh slice.
func stripTools(list, drop []tools.ToolName) []tools.ToolName {
	out := make([]tools.ToolName, 0, len(list))
	for _, t := range list {
		if !slices.Contains(drop, t) {
			out = append(out, t)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/ -run 'TestMainProfile|TestResolveMainProfile_EvvaSuffix' -v`
Expected: PASS. Then `go test ./internal/agent/` — full package PASS (guards solo regressions).

- [ ] **Step 5: Commit**

```bash
git add internal/agent/profiles.go internal/agent/persona_member_profile_test.go
git commit -m "feat(agent): mainProfileForDef — suffix, date-free, solo-scheduling strip for swarm-resident evva"
```

---

### Task 5: disk-persona path — suffix + strip in mainProfileFromDiskAgent

**Files:**
- Modify: `internal/agent/profiles.go` (`mainProfileFromDiskAgent` ~line 297)
- Test: `internal/agent/persona_member_profile_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestDiskMainProfile_SuffixAndStrip(t *testing.T) {
	cfg := suffixTestConfig(t)
	def := sysprompt.AgentDefinition{
		Name: "nono", As: []string{"main"},
		BuildSystemPrompt: func(_ sysprompt.PromptContext) string { return "I am nono." },
		ActiveTools:       []tools.ToolName{tools.READ_FILE, tools.SCHEDULE_WAKEUP},
		DeferredTools:     append([]tools.ToolName{}, alarm.Names()...),
		LongRunning:       true,
		PromptSuffix:      "## SWARM PROTOCOL MARKER",
	}
	p := mainProfileFromDiskAgent(def, cfg, cfg.DefaultProvider, cfg.DefaultModel, nil, memdir.Snapshot{}, nil, nil)
	if !strings.HasSuffix(p.SystemPrompt, "## SWARM PROTOCOL MARKER") {
		t.Fatalf("disk persona suffix missing")
	}
	if slices.Contains(p.ActiveTools, tools.SCHEDULE_WAKEUP) {
		t.Fatalf("strip must remove schedule_wakeup from a long-running disk persona")
	}
	for _, n := range alarm.Names() {
		if slices.Contains(p.DeferredTools, n) {
			t.Fatalf("strip must remove solo alarm tool %q", n)
		}
	}
	if !slices.Contains(p.ActiveTools, tools.READ_FILE) {
		t.Fatalf("unrelated tools must survive the strip")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestDiskMainProfile_SuffixAndStrip -v`
Expected: FAIL — schedule_wakeup still present / suffix missing.

- [ ] **Step 3: Implement**

In `mainProfileFromDiskAgent`, after `deferred := append(append([]tools.ToolName{}, def.DeferredTools...), extraDeferred...)` insert:

```go
	if def.LongRunning {
		def.ActiveTools = stripTools(def.ActiveTools, soloSchedulingTools())
		deferred = stripTools(deferred, soloSchedulingTools())
	}
```

After `sp := sysprompt.ComposeDiskMainPrompt(body, ctx, def)` insert:

```go
	if def.PromptSuffix != "" {
		sp += "\n\n" + def.PromptSuffix
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/ -v`
Expected: PASS (whole package — existing disk-profile tests must stay green; they use LongRunning=false defs, unaffected by the strip).

- [ ] **Step 5: Commit**

```bash
git add internal/agent/profiles.go internal/agent/persona_member_profile_test.go
git commit -m "feat(agent): disk main personas honor PromptSuffix and the LongRunning tool strip"
```

---

### Task 6: LoadSkillCatalog — persona catalog + extra dirs (internal + pkg)

**Files:**
- Modify: `internal/agent/skills.go` (append)
- Create: `pkg/agent/skills.go`
- Test: `internal/agent/skills_catalog_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/skill"
)

func writeCatalogSkill(t *testing.T, root, name, title string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+" "+title+"\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSkillCatalog_LayersExtraDirs(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	cfg, err := config.Load(config.LoadOptions{AppName: "t", AppHome: home, WorkDir: work})
	if err != nil {
		t.Fatal(err)
	}
	writeCatalogSkill(t, cfg.AppHomeSkillsDir, "alpha", "home version")
	shared, member := t.TempDir(), t.TempDir()
	writeCatalogSkill(t, shared, "alpha", "shared version")
	writeCatalogSkill(t, shared, "beta", "shared only")
	writeCatalogSkill(t, member, "alpha", "member version")

	reg := LoadSkillCatalog(cfg, shared, member)

	byName := map[string]skill.SkillMeta{}
	for _, m := range reg.List() {
		byName[m.Name] = m
	}
	if byName["alpha"].Description != "member version" {
		t.Fatalf("member dir must win, got %q", byName["alpha"].Description)
	}
	if _, ok := byName["beta"]; !ok {
		t.Fatalf("shared-only skill must load")
	}
	bundled := false
	for _, m := range reg.List() {
		if m.Source == skill.SourceBundled {
			bundled = true
		}
	}
	if !bundled {
		t.Fatalf("bundled skills must still be present (lowest tier)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestLoadSkillCatalog_LayersExtraDirs -v`
Expected: FAIL — `undefined: LoadSkillCatalog`.

- [ ] **Step 3: Implement**

Append to `internal/agent/skills.go`:

```go
// LoadSkillCatalog builds a persona's full skill catalog — the same
// home+workdir+bundled set solo evva loads (loadDiskSkillRegistry) — then
// overlays extraDirs in order (later wins), labeled SourceSwarm. The swarm
// uses it to compose a persona member's catalog: persona-own skills plus the
// space-shared dir plus the member-local dir (RP-29). Precedence low→high:
// bundled < home < workdir < extraDirs in call order.
func LoadSkillCatalog(cfg *config.Config, extraDirs ...string) *skill.Registry {
	reg := loadDiskSkillRegistry(cfg)
	for _, d := range extraDirs {
		reg.LoadDir(d, skill.SourceSwarm)
	}
	return reg
}
```

Create `pkg/agent/skills.go`:

```go
package agent

import (
	agent_impl "github.com/johnny1110/evva/internal/agent"
	"github.com/johnny1110/evva/pkg/config"
	"github.com/johnny1110/evva/pkg/skill"
)

// LoadSkillCatalog builds the skill catalog a persona would load on its own —
// bundled first-party skills, <appHome>/skills, <workdir>/.evva/skills — then
// overlays extraDirs in order (later wins). Hosts that resolve a persona's
// catalog plus host-specific layers (e.g. a swarm overlaying its shared and
// member-local skill dirs) call this instead of re-implementing the merge.
func LoadSkillCatalog(cfg *config.Config, extraDirs ...string) *skill.Registry {
	if cfg == nil {
		cfg = config.Get()
	}
	return agent_impl.LoadSkillCatalog(cfg, extraDirs...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/ -run TestLoadSkillCatalog -v && go build ./...`
Expected: PASS, clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/skills.go pkg/agent/skills.go internal/agent/skills_catalog_test.go
git commit -m "feat(agent): LoadSkillCatalog — persona skill catalog with layered extra dirs"
```

---

### Task 7: agentdef manifest — persona member shape + model/effort/when_to_use

**Files:**
- Modify: `internal/swarm/agentdef/manifest.go` (Member struct ~line 29; memberYml ~line 130; manifestYml leader/workers conversion in `LoadManifest` ~line 227; `WriteManifest` ~line 326; `toScheduleYml` untouched)
- Test: `internal/swarm/agentdef/manifest_persona_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
package agentdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifestFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "evva-swarm.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadManifest_PersonaMember(t *testing.T) {
	p := writeManifestFile(t, `
name: team
leader:
  agent: lead
workers:
  - agent: dev-a
    model: m-dir
    effort: high
    when_to_use: "dir specialist"
  - persona: helper
    model: m-persona
    effort: ultra
    when_to_use: "resident engineer"
    budget_tokens: -1
`)
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	dir, per := m.Workers[0], m.Workers[1]
	if dir.FromPersona || dir.Model != "m-dir" || dir.Effort != "high" || dir.WhenToUse != "dir specialist" {
		t.Fatalf("dir member overrides wrong: %+v", dir)
	}
	if !per.FromPersona || per.Agent != "helper" || per.Model != "m-persona" || per.Effort != "ultra" || per.WhenToUse != "resident engineer" || per.BudgetTokens != -1 {
		t.Fatalf("persona member wrong: %+v", per)
	}
}

func TestLoadManifest_PersonaLeader(t *testing.T) {
	p := writeManifestFile(t, "leader:\n  persona: boss\n")
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Leader.FromPersona || m.Leader.Agent != "boss" {
		t.Fatalf("persona leader wrong: %+v", m.Leader)
	}
}

func TestLoadManifest_MemberSourceValidation(t *testing.T) {
	cases := map[string]string{
		"both keys":  "leader:\n  agent: a\nworkers:\n  - agent: x\n    persona: x\n",
		"no keys":    "leader:\n  agent: a\nworkers:\n  - schedule: {cron: \"* * * * *\"}\n",
		"bad effort": "leader:\n  agent: a\nworkers:\n  - persona: p\n    effort: turbo\n",
		"dup name":   "leader:\n  agent: a\nworkers:\n  - agent: p\n  - persona: p\n",
	}
	for name, body := range cases {
		if _, err := LoadManifest(writeManifestFile(t, body)); err == nil {
			t.Fatalf("%s: want error, got nil", name)
		}
	}
}

func TestWriteManifest_PersonaRoundTrip(t *testing.T) {
	in := Manifest{
		Name:   "team",
		Leader: Member{Agent: "lead"},
		Workers: []Member{
			{Agent: "helper", FromPersona: true, Model: "m1", Effort: "ultra", WhenToUse: "engineer"},
			{Agent: "dev-a"},
		},
	}
	p := filepath.Join(t.TempDir(), "out.yml")
	if err := WriteManifest(p, in); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if strings.Contains(string(raw), "agent: helper") || !strings.Contains(string(raw), "persona: helper") {
		t.Fatalf("persona member must serialize under the persona key:\n%s", raw)
	}
	out, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	got := out.Workers[0]
	if !got.FromPersona || got.Agent != "helper" || got.Model != "m1" || got.Effort != "ultra" || got.WhenToUse != "engineer" {
		t.Fatalf("round-trip lost persona fields: %+v", got)
	}
	if out.Workers[1].FromPersona {
		t.Fatalf("dir member must stay dir-sourced")
	}
}

// TestLoadManifest_ExternalPath lints a real manifest named by env var —
// `EVVA_MANIFEST_PATH=/path/to/evva-swarm.yml go test ./internal/swarm/agentdef -run ExternalPath`.
// Lets an operator validate a downstream swarm file against this parser.
func TestLoadManifest_ExternalPath(t *testing.T) {
	p := os.Getenv("EVVA_MANIFEST_PATH")
	if p == "" {
		t.Skip("EVVA_MANIFEST_PATH not set")
	}
	if _, err := LoadManifest(p); err != nil {
		t.Fatalf("external manifest invalid: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/swarm/agentdef/ -run 'Persona|MemberSource|ExternalPath' -v`
Expected: FAIL — `unknown field FromPersona` etc. (compile errors).

- [ ] **Step 3: Implement**

`Member` struct — add fields:

```go
	// FromPersona marks a member that references a registry main-tier persona
	// instead of a workdir agents/{main,sub}/<name>/ directory (RP-29). The
	// member NAME stays in Agent for both shapes; the space resolves and
	// composes the persona def at assembly time.
	FromPersona bool
	// Model / Effort / WhenToUse are optional manifest-level overrides. For a
	// persona member they are the only pin point (it has no profile.yml); for
	// a dir member a non-empty value is authoritative over profile.yml — the
	// RP-7 §3.7 schedule precedent: the whole team's config reads in one file.
	Model     string
	Effort    string
	WhenToUse string
```

`memberYml` — add yaml fields (and add `omitempty` to the `agent` tag):

```go
type memberYml struct {
	Agent          string       `yaml:"agent,omitempty"`
	Persona        string       `yaml:"persona,omitempty"`
	Model          string       `yaml:"model,omitempty"`
	Effort         string       `yaml:"effort,omitempty"`
	WhenToUse      string       `yaml:"when_to_use,omitempty"`
	Schedule       *scheduleYml `yaml:"schedule,omitempty"`
	BudgetTokens   int          `yaml:"budget_tokens,omitempty"`
	PermissionMode string       `yaml:"permission_mode,omitempty"` // "" = inherit settings (RP-24)
}
```

Add a shared conversion used by both leader and workers (place above `LoadManifest`):

```go
// memberFromYml validates and converts one manifest member entry. ctx names
// the entry ("leader", `worker "x"`) for error messages. Exactly one of
// agent/persona must be set; effort, schedule, and permission_mode fail fast
// here so a typo rejects the manifest at register time.
func memberFromYml(y memberYml, ctx string) (Member, error) {
	agentName := strings.TrimSpace(y.Agent)
	personaName := strings.TrimSpace(y.Persona)
	if (agentName == "") == (personaName == "") {
		return Member{}, fmt.Errorf("agentdef: manifest %s: exactly one of agent/persona is required", ctx)
	}
	name := agentName
	fromPersona := false
	if personaName != "" {
		name, fromPersona = personaName, true
	}
	sched, err := parseScheduleYml(y.Schedule)
	if err != nil {
		return Member{}, fmt.Errorf("agentdef: manifest %s schedule: %w", ctx, err)
	}
	mode, err := parsePermissionMode(y.PermissionMode)
	if err != nil {
		return Member{}, fmt.Errorf("agentdef: manifest %s permission_mode: %w", ctx, err)
	}
	effort := strings.TrimSpace(y.Effort)
	if effort != "" && llm.ParseEffort(effort) == 0 {
		return Member{}, fmt.Errorf("agentdef: manifest %s: invalid effort %q (want low|medium|high|ultra)", ctx, effort)
	}
	return Member{
		Agent: name, FromPersona: fromPersona,
		Model: strings.TrimSpace(y.Model), Effort: effort, WhenToUse: strings.TrimSpace(y.WhenToUse),
		Schedule: sched, BudgetTokens: y.BudgetTokens, PermissionMode: mode,
	}, nil
}
```

Add `"github.com/johnny1110/evva/pkg/llm"` to the imports.

In `LoadManifest`, replace the leader schedule/permission parsing AND the worker loop with calls to `memberFromYml`:

```go
	leader, err := memberFromYml(y.Leader, "leader")
	if err != nil {
		return Manifest{}, err
	}
	// ... settings parsing stays exactly as today ...
	m := Manifest{
		Name:    y.Name,
		Workdir: y.Workdir,
		Leader:  leader,
		Settings: Settings{ /* unchanged */ },
	}
	for _, w := range y.Workers {
		wm, err := memberFromYml(w, fmt.Sprintf("worker %q", strings.TrimSpace(w.Agent+w.Persona)))
		if err != nil {
			return Manifest{}, err
		}
		m.Workers = append(m.Workers, wm)
	}
```

(Delete the now-unused inline `parseScheduleYml(y.Leader.Schedule)` / `parsePermissionMode(y.Leader.PermissionMode)` calls — `memberFromYml` does both.)

In `WriteManifest`, add a small serializer and use it for leader and workers:

```go
// memberToYml is the inverse of memberFromYml.
func memberToYml(m Member) memberYml {
	y := memberYml{
		Model: m.Model, Effort: m.Effort, WhenToUse: m.WhenToUse,
		Schedule: toScheduleYml(m.Schedule), BudgetTokens: m.BudgetTokens, PermissionMode: m.PermissionMode,
	}
	if m.FromPersona {
		y.Persona = m.Agent
	} else {
		y.Agent = m.Agent
	}
	return y
}
```

Replace `y.Leader = memberYml{...}` with `y.Leader = memberToYml(m.Leader)` and the worker loop body with `y.Workers = append(y.Workers, memberToYml(w))`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/swarm/agentdef/ -v`
Expected: PASS — including all pre-existing manifest tests (they use `agent:` members; behavior unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/swarm/agentdef/manifest.go internal/swarm/agentdef/manifest_persona_test.go
git commit -m "feat(swarm): manifest persona members + model/effort/when_to_use overrides"
```

---

### Task 8: agentdef loader — synthesize persona Loaded, apply dir overrides

**Files:**
- Modify: `internal/swarm/agentdef/loader.go` (`Loaded` struct ~line 29; `BuildAll` ~line 137)
- Test: `internal/swarm/agentdef/loader_persona_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
package agentdef

import (
	"testing"
)

// testdata/agents/ has main/leader + sub/{frontend-dev,backend-dev}.
func TestBuildAll_PersonaMemberNeedsNoDir(t *testing.T) {
	m := Manifest{
		Leader: Member{Agent: "leader"},
		Workers: []Member{
			{Agent: "ghost-persona", FromPersona: true, Model: "m1", Effort: "ultra", WhenToUse: "engineer",
				PermissionMode: "bypass"},
		},
	}
	loaded, _, err := NewLoader().BuildAll("testdata", m)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want leader + persona, got %d", len(loaded))
	}
	p := loaded[1]
	if !p.FromPersona || p.Def.Name != "ghost-persona" {
		t.Fatalf("persona Loaded wrong: %+v", p)
	}
	if p.Def.Model != "m1" || p.Effort != "ultra" || p.Def.WhenToUse != "engineer" || p.PermissionMode != "bypass" {
		t.Fatalf("manifest fields not carried: %+v", p)
	}
	if p.Skills == nil {
		t.Fatalf("persona Loaded must carry a non-nil (empty) skill registry")
	}
}

func TestBuildAll_DirMemberManifestOverrides(t *testing.T) {
	m := Manifest{
		Leader: Member{Agent: "leader"},
		Workers: []Member{
			{Agent: "frontend-dev", Model: "override-model", Effort: "low", WhenToUse: "override desc"},
		},
	}
	loaded, _, err := NewLoader().BuildAll("testdata", m)
	if err != nil {
		t.Fatal(err)
	}
	w := loaded[1]
	if w.Def.Model != "override-model" || w.Effort != "low" || w.Def.WhenToUse != "override desc" {
		t.Fatalf("manifest must override profile.yml: model=%q effort=%q wtu=%q", w.Def.Model, w.Effort, w.Def.WhenToUse)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/swarm/agentdef/ -run TestBuildAll_Persona -v`
Expected: FAIL — `unknown field FromPersona` on Loaded / dir error for ghost-persona.

- [ ] **Step 3: Implement**

`Loaded` struct — add after `PermissionMode`:

```go
	// FromPersona marks a member synthesized from a manifest persona entry
	// (RP-29): no disk dir was read; the space resolves the def from its
	// persona registry at assembly time.
	FromPersona bool
```

In `BuildAll`, change the `add` closure:

```go
	add := func(dir string, role Role, mem Member) error {
		if mem.FromPersona {
			loaded = append(loaded, Loaded{
				Def: agent.AgentDefinition{Name: mem.Agent, WhenToUse: mem.WhenToUse, Model: mem.Model},
				FromPersona: true, Role: role, Schedule: mem.Schedule,
				Effort: mem.Effort, PermissionMode: mem.PermissionMode,
				Skills: skill.NewRegistry(),
			})
			return nil
		}
		one, err := l.Build(dir, role, shared)
		if err != nil {
			return err
		}
		// Manifest schedule is authoritative over the agent's profile.yml (RP-7
		// §3.7) — the whole team's cadence is declared in one versioned file.
		if mem.Schedule != nil {
			one.Schedule = mem.Schedule
		}
		// Same precedence for the RP-29 manifest overrides.
		if mem.Model != "" {
			one.Def.Model = mem.Model
		}
		if mem.Effort != "" {
			one.Effort = mem.Effort
		}
		if mem.WhenToUse != "" {
			one.Def.WhenToUse = mem.WhenToUse
		}
		one.PermissionMode = mem.PermissionMode
		for _, w := range one.Skills.Warnings {
			warnings = append(warnings, Warning{Agent: one.Def.Name, Msg: w})
		}
		loaded = append(loaded, one)
		return nil
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/swarm/agentdef/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/swarm/agentdef/loader.go internal/swarm/agentdef/loader_persona_test.go
git commit -m "feat(swarm): BuildAll synthesizes persona members without a disk dir"
```

---

### Task 9: teamprompt — extract teamProtocolSuffix

**Files:**
- Modify: `internal/swarm/teamprompt.go` (`injectTeamProtocol` ~line 32)
- Test: `internal/swarm/teamprompt_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestTeamProtocolSuffix_MatchesInject(t *testing.T) {
	for _, role := range []agentdef.Role{agentdef.RoleLeader, agentdef.RoleWorker} {
		for _, canWrite := range []bool{true, false} {
			suffix := teamProtocolSuffix("alice", "team", role, canWrite)
			full := injectTeamProtocol("PERSONA BODY", "alice", "team", role, canWrite)
			if want := "PERSONA BODY\n\n" + suffix; full != want {
				t.Fatalf("inject(role=%s,write=%v) must be body + suffix", role, canWrite)
			}
			if got := injectTeamProtocol("", "alice", "team", role, canWrite); got != suffix {
				t.Fatalf("empty persona must yield the bare suffix")
			}
		}
	}
}
```

(Ensure the test file imports `agentdef`; it already does for existing tests.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/swarm/ -run TestTeamProtocolSuffix -v`
Expected: FAIL — `undefined: teamProtocolSuffix`.

- [ ] **Step 3: Implement**

Refactor `injectTeamProtocol` to delegate:

```go
func injectTeamProtocol(persona, name, space string, role agentdef.Role, canWriteMemory bool) string {
	suffix := teamProtocolSuffix(name, space, role, canWriteMemory)
	if p := strings.TrimRight(persona, "\n"); p != "" {
		return p + "\n\n" + suffix
	}
	return suffix
}

// teamProtocolSuffix renders the swarm's collaboration sections WITHOUT a
// persona body: grounding, channel rules, common protocol, role protocol,
// and (for file-writing members) the memory protocol. Dir members get it
// concatenated into their prompt body (injectTeamProtocol); persona members
// get it as AgentDefinition.PromptSuffix so it survives the internally-
// assembled prompt path and every re-render (RP-29).
func teamProtocolSuffix(name, space string, role agentdef.Role, canWriteMemory bool) string {
	var b strings.Builder
	b.WriteString(swarmIdentity(name, space, role))
	b.WriteString("\n\n")
	b.WriteString(communicationProtocol)
	b.WriteString("\n\n")
	b.WriteString(teamProtocolCommon)
	b.WriteString("\n\n")
	if role == agentdef.RoleLeader {
		b.WriteString(leaderProtocol)
	} else {
		b.WriteString(workerProtocol)
	}
	if canWriteMemory {
		b.WriteString("\n\n")
		b.WriteString(memoryProtocol(name, role))
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/swarm/ -run TeamProtocol -v` then `go test ./internal/swarm/`
Expected: PASS — all existing teamprompt tests stay green (output is byte-identical for dir members).

- [ ] **Step 5: Commit**

```bash
git add internal/swarm/teamprompt.go internal/swarm/teamprompt_test.go
git commit -m "refactor(swarm): extract teamProtocolSuffix from injectTeamProtocol"
```

---

### Task 10: space + supervisor — persona member assembly

**Files:**
- Modify: `internal/swarm/space.go` (`SwarmSpace` struct ~line 55; `NewSpace` register loop ~line 169; `registerDef` ~line 184; `constructMember` skills wiring ~line 292; new helpers)
- Modify: `internal/swarm/supervisor.go` (`AddMember` ~line 167; `ReloadMemberSkills` ~line 360)
- Test: `internal/swarm/persona_member_test.go` (new)

- [ ] **Step 1: Write the failing test**

```go
package swarm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/swarm/agentdef"
	"github.com/johnny1110/evva/pkg/agent"
	"github.com/johnny1110/evva/pkg/skill"
)

func personaLoaded(name string, role agentdef.Role) agentdef.Loaded {
	return agentdef.Loaded{
		Def:         agent.AgentDefinition{Name: name},
		FromPersona: true,
		Role:        role,
		Skills:      skill.NewRegistry(),
	}
}

func dirLoaded(name string, role agentdef.Role) agentdef.Loaded {
	return agentdef.Loaded{
		Def:    agent.AgentDefinition{Name: name, SystemPrompt: "You are " + name + ".", Model: stubModel},
		Skills: skill.NewRegistry(),
		Role:   role,
	}
}

func TestNewSpace_BuiltinPersonaWorker(t *testing.T) {
	cfg := stubConfig(t)
	ld := personaLoaded("evva", agentdef.RoleWorker)
	ld.Def.WhenToUse = "resident engineer"
	sp, err := NewSpace("s1", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), ld}, nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace with persona member: %v", err)
	}
	defer sp.Shutdown()

	def, ok := sp.reg.Get("evva")
	if !ok {
		t.Fatal("composed evva def missing from space registry")
	}
	if !def.LongRunning || !def.AdvertiseSkills {
		t.Fatalf("persona def must be swarm-hardened: %+v", def)
	}
	for _, want := range []string{"# Your place in the swarm", "## Your role: a worker", "## Your long-term memory"} {
		if !strings.Contains(def.PromptSuffix, want) {
			t.Fatalf("PromptSuffix missing section %q", want)
		}
	}
	var found bool
	for _, mv := range sp.Roster.Snapshot() {
		if mv.Name == "evva" {
			found = true
			if mv.WhenToUse != "resident engineer" {
				t.Fatalf("roster when_to_use = %q", mv.WhenToUse)
			}
		}
	}
	if !found {
		t.Fatal("evva not on the roster")
	}
	if dir := agentdef.MemoryDir(sp.Workdir, agentdef.RoleWorker, "evva"); !dirExists(dir) {
		t.Fatalf("member memory dir not created: %s", dir)
	}
	if !sp.isPersonaMember("evva") {
		t.Fatal("space must track persona membership")
	}
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func TestNewSpace_UnknownPersonaFails(t *testing.T) {
	cfg := stubConfig(t)
	_, err := NewSpace("s2", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), personaLoaded("ghost", agentdef.RoleWorker)}, nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("want unknown-persona error naming ghost, got %v", err)
	}
}

func TestNewSpace_NonMainPersonaFails(t *testing.T) {
	cfg := stubConfig(t)
	_, err := NewSpace("s3", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), personaLoaded("explore", agentdef.RoleWorker)}, nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "main-tier") {
		t.Fatalf("want non-main-tier error, got %v", err)
	}
}

func TestMemberSkillRegistry_PersonaLayers(t *testing.T) {
	cfg := stubConfig(t)
	write := func(root, name, title string) {
		t.Helper()
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+" "+title+"\nbody"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(cfg.AppHomeSkillsDir, "alpha", "persona version")
	write(agentdef.SharedSkillsDir(cfg.WorkDir), "alpha", "shared version")
	write(agentdef.SharedSkillsDir(cfg.WorkDir), "beta", "shared only")
	write(agentdef.SkillsDir(cfg.WorkDir, agentdef.RoleWorker, "evva"), "alpha", "member version")

	sp, err := NewSpace("s4", testManifest(), []agentdef.Loaded{dirLoaded("leader", agentdef.RoleLeader), personaLoaded("evva", agentdef.RoleWorker)}, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Shutdown()

	reg := sp.memberSkillRegistry(true, agentdef.RoleWorker, "evva")
	byName := map[string]skill.SkillMeta{}
	for _, m := range reg.List() {
		byName[m.Name] = m
	}
	if byName["alpha"].Description != "member version" {
		t.Fatalf("member dir must win: %q", byName["alpha"].Description)
	}
	if _, ok := byName["beta"]; !ok {
		t.Fatal("shared skill must load for persona members")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/swarm/ -run 'Persona|MemberSkillRegistry' -v`
Expected: FAIL — `sp.isPersonaMember undefined`, `sp.memberSkillRegistry undefined`, NewSpace currently registers a body-less def instead of erroring.

- [ ] **Step 3: Implement**

`SwarmSpace` struct — add below `schedMeta`:

```go
	// personaMembers marks members sourced from a registry persona (RP-29):
	// their skill catalog merges the persona's own dirs with the space's, so
	// construction and skill reload must agree on the source set.
	personaMembers map[string]bool
```

`NewSpace` — change the register loop and construct loop:

```go
	for i := range loaded {
		if err := sp.registerDef(&loaded[i]); err != nil {
			sp.Shutdown()
			return nil, fmt.Errorf("swarm: space %q: %w", id, err)
		}
	}
	for _, ld := range loaded {
		if err := sp.constructMember(ld); err != nil {
			sp.Shutdown()
			return nil, fmt.Errorf("swarm: space %q: %w", id, err)
		}
	}
```

`registerDef` — new signature `func (sp *SwarmSpace) registerDef(ld *agentdef.Loaded) error`; keep the existing dir-member body (operating on `def := ld.Def`) returning `nil`, and add the persona branch at the top:

```go
func (sp *SwarmSpace) registerDef(ld *agentdef.Loaded) error {
	if ld.FromPersona {
		return sp.registerPersonaDef(ld)
	}
	def := ld.Def
	// ... existing body unchanged ...
	return nil
}

// registerPersonaDef composes a persona member's definition from the space
// persona registry (built-ins + <appHome>/agents): swarm-harden it the same
// way dir members are (LongRunning, AdvertiseSkills, main-tier), apply the
// manifest's when_to_use/model overrides, and attach the team protocol as
// PromptSuffix — the seam that survives internally-assembled prompts and
// every re-render (RP-29). The composed def is registered space-locally and
// written back onto ld so constructMember and the roster read the effective
// values.
func (sp *SwarmSpace) registerPersonaDef(ld *agentdef.Loaded) error {
	name := ld.Def.Name
	base, ok := sp.reg.Get(name)
	if !ok {
		return fmt.Errorf("persona member %q: no such persona in the registry (built-ins + <appHome>/agents)", name)
	}
	if !base.IsMain() {
		return fmt.Errorf("persona member %q: not a main-tier persona", name)
	}
	def := base
	def.As = ensureMain(def.As)
	def.LongRunning = true
	def.AdvertiseSkills = true
	// Built-ins carry empty tool lists ("constructor defaults", which already
	// include the skill tool); only a disk persona's explicit list needs the
	// swarm-forced skill tool (RP-10-1).
	if len(def.ActiveTools) > 0 {
		def.ActiveTools = ensureTool(def.ActiveTools, tools.SKILL)
	}
	if w := ld.Def.WhenToUse; w != "" {
		def.WhenToUse = w
	}
	if m := ld.Def.Model; m != "" {
		def.Model = m
	}
	canWrite := len(def.ActiveTools) == 0 ||
		slices.Contains(def.ActiveTools, tools.WRITE_FILE) ||
		slices.Contains(def.ActiveTools, tools.EDIT_FILE)
	def.PromptSuffix = teamProtocolSuffix(name, sp.Name, ld.Role, canWrite)
	sp.mu.Lock()
	sp.reg.Register(def)
	if sp.personaMembers == nil {
		sp.personaMembers = map[string]bool{}
	}
	sp.personaMembers[name] = true
	sp.mu.Unlock()
	ld.Def = def
	return nil
}

// isPersonaMember reports whether name was assembled from a registry persona.
func (sp *SwarmSpace) isPersonaMember(name string) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.personaMembers[name]
}
```

`constructMember` — replace `agent.WithSkillRegistry(ld.Skills),` with a persona-aware registry, computed just above the `opts := []agent.Option{...}` block:

```go
	skillsReg := ld.Skills
	if ld.FromPersona {
		skillsReg = sp.memberSkillRegistry(true, ld.Role, name)
	}
```

…and use `agent.WithSkillRegistry(skillsReg),` in opts. Add the helper:

```go
// memberSkillRegistry resolves a member's full skill catalog from disk. Dir
// members load (shared, own) — the RP-26 order. Persona members additionally
// start from the persona's OWN catalog (bundled + appHome + workdir skills,
// via agent.LoadSkillCatalog) with the space layers on top, precedence
// low→high: bundled < home < workdir < shared < member-local. Construction
// and Supervisor.ReloadMemberSkills both call this, so the two never drift.
func (sp *SwarmSpace) memberSkillRegistry(fromPersona bool, role agentdef.Role, name string) *skill.Registry {
	shared := agentdef.SharedSkillsDir(sp.Workdir)
	own := agentdef.SkillsDir(sp.Workdir, role, name)
	if fromPersona {
		return agent.LoadSkillCatalog(sp.cfg, shared, own)
	}
	reg, _ := skill.LoadRegistry(shared, own)
	return reg
}
```

`supervisor.go` — `AddMember`: change `s.sp.registerDef(ld)` to:

```go
	if err := s.sp.registerDef(&ld); err != nil {
		return fmt.Errorf("swarm: add member %q: %w", name, err)
	}
```

`ReloadMemberSkills`: replace the `skill.LoadRegistry(...)` line with:

```go
	reg := s.sp.memberSkillRegistry(s.sp.isPersonaMember(name), role, name)
```

(drop the now-unused `skill` import from supervisor.go if it becomes unused — check with `go build ./...`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/swarm/ -v`
Expected: PASS — new persona tests AND all existing space/supervisor/roster tests (dir-member behavior unchanged).

Note: `TestNewSpace_BuiltinPersonaWorker` constructs a real agent on the full
Main kit against the stub LLM — if it surfaces a missing-config panic from a
tool constructor, fix by extending `stubConfig`, not by shrinking the kit.

- [ ] **Step 5: Commit**

```bash
git add internal/swarm/space.go internal/swarm/supervisor.go internal/swarm/persona_member_test.go
git commit -m "feat(swarm): assemble persona members — registry compose, suffix, layered skills"
```

---

### Task 11: EVVA docs — user guides, RP-29 pointer, CHANGELOG, wave map

**Files:**
- Modify: `docs/roadmap/veronica/user-guide-en.md`, `docs/roadmap/veronica/user-guide-zh.md` (new section near the member-definition / manifest docs)
- Create: `docs/roadmap/veronica/refine-plan/RP-29-persona-members.md`
- Modify: `CHANGELOG.md` (`[Unreleased]` → `### Added`)
- Modify: `CLAUDE.md` (wave → minor map table)

- [ ] **Step 1: Write the user-guide section (EN)**

Append to `user-guide-en.md` (after the section describing `evva-swarm.yml` members; find it with `grep -n "agent:" docs/roadmap/veronica/user-guide-en.md`):

```markdown
## Persona members (RP-29)

A manifest member may reference a **registry main-tier persona** instead of a
workdir agent directory — the built-in `evva`, or any persona under
`<EVVA_HOME>/agents/`. The persona joins with its full identity (its own
system prompt, complete tool kit, installed skills, workdir `EVVA.md`
briefing) plus the swarm team protocol and the role's swarm tools. Works for
the leader and for workers:

```yaml
workers:
  - persona: evva            # exactly one of agent:/persona: per member
    model: deepseek-v4-pro   # optional pin (a persona member has no profile.yml)
    effort: ultra            # low|medium|high|ultra
    when_to_use: "resident engineer"   # roster description
```

Semantics:
- `model:` / `effort:` / `when_to_use:` are also accepted on `agent:` members,
  where a non-empty value overrides profile.yml (the schedule precedence rule).
- Skills merge low→high: bundled < home < workdir < space-shared < member-local.
- Memory is the standard member memory dir (`agents/{main,sub}/<name>/memory/`);
  the persona's solo auto-memory is not bridged.
- Swarm-resident personas drop the solo self-scheduling tools
  (`alarm_create/list/cancel`, `cron_*`, `schedule_wakeup`) — use `alarm_set`/
  `alarm_clear` and the leader's `schedule_set` instead.
- v1 scope: declare persona members in the manifest (register/restart applies
  them); the web add-member form still creates directory members only.
```

- [ ] **Step 2: Write the user-guide section (ZH)**

Append the same section translated to `user-guide-zh.md`:

```markdown
## Persona 成員（RP-29）

manifest 成員可以引用 **registry 裡的 main-tier 人格**（內建 `evva` 或
`<EVVA_HOME>/agents/` 下的自製人格），不需要 workdir 的 agent 目錄。人格以
本尊身分進駐：自己的 system prompt、完整工具組、已安裝 skills、workdir
`EVVA.md` 簡報，外加 swarm teamwork 協議與角色對應的 swarm 工具。leader 與
worker 皆可用：

```yaml
workers:
  - persona: evva            # 每個成員 agent:/persona: 恰好擇一
    model: deepseek-v4-pro   # 選用釘選（persona 成員沒有 profile.yml）
    effort: ultra            # low|medium|high|ultra
    when_to_use: "特派工程師" # roster 顯示的專長
```

語義：
- `model:` / `effort:` / `when_to_use:` 在 `agent:` 成員上也可用，非空時
  蓋過 profile.yml（沿用 schedule 的權威規則）。
- skills 五層合併（低→高）：bundled < home < workdir < space 共享 < 成員私有。
- 記憶 = 標準成員記憶目錄（`agents/{main,sub}/<name>/memory/`）；solo 的
  全域 auto-memory 不橋接。
- 駐 swarm 的人格會剝離 solo 排程工具（`alarm_create/list/cancel`、`cron_*`、
  `schedule_wakeup`）——改用 `alarm_set`/`alarm_clear` 與 leader 的 `schedule_set`。
- v1 範圍：persona 成員寫在 manifest（register/重啟生效）；web 表單僅支援
  目錄成員。
```

- [ ] **Step 3: Create the RP-29 pointer**

`docs/roadmap/veronica/refine-plan/RP-29-persona-members.md`:

```markdown
# RP-29 — Persona members

讓 registry 的 main-tier 人格（內建 evva / EVVA_HOME 自製人格）以本尊身分
加入 swarm 擔任 leader 或 worker：完整 prompt/工具/skills + teamwork 協議。

- 設計定案 spec：[`docs/superpowers/specs/2026-06-11-persona-members-design.md`](../../../superpowers/specs/2026-06-11-persona-members-design.md)
- 實作計畫：[`docs/superpowers/plans/2026-06-11-persona-members.md`](../../../superpowers/plans/2026-06-11-persona-members.md)
- wave：第六波開場；認領 minor **v1.7**。
```

- [ ] **Step 4: CHANGELOG + CLAUDE.md**

`CHANGELOG.md` — under `[Unreleased]`, add (create the `### Added` heading if absent):

```markdown
### Added
- Persona members (RP-29): an `evva-swarm.yml` member may reference a registry
  main-tier persona (`persona: <name>`, leader or worker) — full persona
  identity (prompt/tools/skills + workdir EVVA.md) plus the swarm team
  protocol via the new `AgentDefinition.PromptSuffix`. Manifest members gain
  optional `model:` / `effort:` / `when_to_use:` overrides. Swarm-resident
  (LongRunning) personas drop solo self-scheduling tools (`alarm_*`, `cron_*`,
  `schedule_wakeup`). New public seams: `pkg/agent.LoadSkillCatalog`,
  `pkg/skill.Registry.LoadDir`, `skill.SourceSwarm`,
  `pkg/agent.AgentDefinition.PromptSuffix`.
```

`CLAUDE.md` — wave → minor map: append the row:

```markdown
| v1.7 | Persona members (RP-29) — registry main-tier personas join a swarm as leader/worker with full identity + team protocol |
```

- [ ] **Step 5: Commit**

```bash
git add docs/roadmap/veronica/ CHANGELOG.md CLAUDE.md
git commit -m "docs: persona members (RP-29) — user guides, refine-plan pointer, changelog, wave map"
```

---

### Task 12: EVVA full gate

**Files:** none (verification only)

- [ ] **Step 1: Full build + test + vet + fmt**

Run from `/workspace/EVVA`:

```bash
go build ./... && go vet ./... && gofmt -l pkg internal cmd && go test ./...
```

Expected: build clean, vet clean, `gofmt -l` prints nothing, ALL tests PASS (the suite includes downstream-compile tests that guard the pkg/ SDK surface).

- [ ] **Step 2: Lint the downstream manifest shape (cross-repo check, generic hook)**

```bash
EVVA_MANIFEST_PATH=/workspace/SUNDAY/evva-swarm.yml go test ./internal/swarm/agentdef/ -run TestLoadManifest_ExternalPath -v
```

Expected at THIS point: PASS (the current Sunday manifest has no persona member yet — this baseline proves the parser stays compatible). Re-run after Task 13 to validate the new entry.

- [ ] **Step 3: Commit (only if anything was fixed)**

```bash
git status --short   # expect empty; if fixes were needed: git add -A && git commit -m "fix: full-gate fallout"
```

---

### Task 13: SUNDAY — manifest entry + EVVA.md

**Files:**
- Modify: `/workspace/SUNDAY/evva-swarm.yml` (workers list — insert after the `trader` block; settings `stall_hard_timeout`; header comment)
- Create: `/workspace/SUNDAY/EVVA.md`

Repo: `/workspace/SUNDAY`, branch `main`.

- [ ] **Step 1: Edit `evva-swarm.yml`**

(a) Header comment block — after the trader line, insert:

```yaml
#   evva (persona)   = RESIDENT ENGINEER — friday's software-change tickets for Sunday
#                      (PRD implementation, bug fixes): implement → tests green → commit
#                      main → RUNBOOK restart → verify /health → report back. No trading.
```

(b) In `workers:`, insert after the trader entry (before analyst-flow):

```yaml
  - persona: evva            # 特派工程師 — friday 的 Sunday 軟體改動需求由他實作（PRD 票/bug 修復）
    model: deepseek-v4-pro
    effort: ultra
    when_to_use: "特派工程師 — 接 friday 的 Sunday 軟體改動 ticket（PRD 實作、bug 修復）：實作 → 測試綠 → commit main → 照 RUNBOOK 部署重啟 → 驗 /health → 回報。不下單、不碰交易。"
    # 不設 schedule：純 ticket/訊息驅動（task_assign 即喚醒）；工程量低頻，cron 只會空燒 token
```

(c) In `settings:`, change:

```yaml
  stall_hard_timeout: "2h"       # a run busy >2h is cancelled (mail re-queued, nothing lost); raised from 30m for the
                                 # resident engineer's long code/test/deploy runs — patrols never came near 30m anyway;
                                 # stall warning stays at the 10m default
```

- [ ] **Step 2: Validate the manifest against the new parser**

```bash
cd /workspace/EVVA && EVVA_MANIFEST_PATH=/workspace/SUNDAY/evva-swarm.yml go test ./internal/swarm/agentdef/ -run TestLoadManifest_ExternalPath -v
```

Expected: PASS.

- [ ] **Step 3: Create `/workspace/SUNDAY/EVVA.md`**

```markdown
# Sunday — evva 特派工程師簡報

> 你（evva）以 persona member 身分駐在 sunday swarm，擔任**特派工程師**，聽指揮官
> **friday** 調度。你的工作：Sunday（本 repo 的 Python web app）的軟體改動——
> `docs/prd/` 的 PRD 票實作、bug 修復。**先讀 [CLAUDE.md](CLAUDE.md)**（不變量
> 與專案結構的完整版）；本檔是你的行規與 SOP。

## 工程鐵則（違反 = 退件）

1. **8 條不變量**（CLAUDE.md「不可違反的不變量」）逐條確認後才動工。最常踩的：
   行情主網/交易測試網雙 ccxt 分離；所有 API 免 token；list 一律分頁信封；
   無 Postgres/Redis（唯一狀態 = sqlite + RLock 寫鎖）；純邏輯 stdlib-only、
   重依賴惰性 import；新 SQLite store 沿用 RLock 寫鎖模式。
2. `engine/.env`（testnet 金鑰）**永不 commit**。
3. **不碰 `../evva`**（evva runtime 是另一個專案，另有人管）。
4. **不下單、不碰 `/api/perp` 交易端點**——你是工程師，不是交易員。
5. 不改 friday 憲法（`/api/memory/friday`）與隊友的記憶目錄。

## SOP：一張票的生命週期

1. **接票**：工作來自 friday 的 task（`my_tasks`）；票通常引用 `docs/prd/PRD-*.md`。
   先讀票與 PRD，再讀相關程式碼。票寫不清楚 → `send_message` 問 friday，不猜。
2. **實作**：測試貼著程式碼寫（`tests/test_*.py`）；conventional commit
   （`feat`/`fix`/`chore`/`docs`/`refactor`/`test`）。
3. **驗證**：`./scripts/run-tests.sh` 全綠才算完成。改到 HTTP 契約 → 對 running
   engine 跑 `./scripts/smoke.sh`；動到 UI → `cd engine/sunday/web && npm run build`
   重建 `dist/` 並一起 commit。
4. **交付（部署）**：commit 到 `main` → **先 `send_message` 知會 friday 要重啟**
   （有在途交易/劇烈行情時聽他的擇時）→ 照 [RUNBOOK.md](RUNBOOK.md) 重啟
   （engine venv + `python -m sunday`）→ 驗 `GET /health` 200 + 抽查相關端點
   → 回報 friday：**票號 + commit hash + 測試證據 + 重啟確認**。
5. **失敗路徑**：部署後 `/health` 不通或 smoke 失敗 → 立刻 `git revert` 壞 commit
   → 回滾重啟 → 如實回報 friday 與票（不粉飾）。連 revert 都救不回 → 通知
   friday 用 `POST /api/reports` 升級 User。

## 環境備忘

- 引擎跑在 `:7777`（`python -m sunday`，無 systemd）；重啟 = 全隊短暫斷交易所，
  所以部署視窗要先知會。
- 測試分層見 RUNBOOK §0：純邏輯單元測試到處能跑；ccxt/ws/dashboard 要在 host
  驗（smoke + 瀏覽器）。
- 你的長期記憶在 `agents/sub/evva/memory/`（機制見系統注入的記憶協議）；修過的
  坑、repo 的非顯而易見事實，收工前記下來。
```

- [ ] **Step 4: Commit**

```bash
cd /workspace/SUNDAY && git add evva-swarm.yml EVVA.md
git commit -m "feat(swarm): onboard evva as resident engineer (persona member) + engineer briefing"
```

---

### Task 14: SUNDAY — friday/docs integration

**Files:**
- Modify: `/workspace/SUNDAY/agents/main/friday/system_prompt.md` (roster table ~line 36; 「有需求就開票」 ~line 114)
- Modify: `/workspace/SUNDAY/agents/skills/prd-ticket/SKILL.md` (intro line)
- Modify: `/workspace/SUNDAY/docs/workflow.md` (§1 diagram, §3 table, §7 table)
- Modify: `/workspace/SUNDAY/CLAUDE.md` (「現況」 swarm 消費端段落)

- [ ] **Step 1: friday roster table** — in the table after the `trader` row, insert:

```markdown
| **evva** | 特派工程師：Sunday 軟體改動 | Sunday 缺陷/缺功能 → 開 PRD 票 + task 派他：他實作、測試綠、commit、部署重啟、回報。**他重啟前會知會你**——劇烈行情/在途交易時可叫他等。驗收看測試證據 + `GET /health` + 抽查相關端點，不蓋橡皮章 |
```

- [ ] **Step 2: friday「有需求就開票」一節** — replace the section (lines ~114-117):

```markdown
## 有需求就開票（然後派工程師）

覺得 Sunday 缺端點/缺數據/該優化 → 載入共享的 `prd-ticket` skill 照格式開票，
**接著開 task 派給 evva 實作**（票是規格、task 是派工）。鼓勵隊友也開票——這個
平台是為你們打造的。
**團隊發現任何 BUG：寫 PRD 開 bug 單 + task 派 evva 修復；影響交易的同時
`POST /api/reports` 緊急通報 User。evva 修不動或重啟救不回的，才升級 Boss。**
```

- [ ] **Step 3: prd-ticket SKILL.md** — change the intro sentence:

Old: `Sunday 是專門為這支團隊打造的——儘管提，後續會有人實作。`
New: `Sunday 是專門為這支團隊打造的——儘管提，特派工程師 evva 會接票實作（friday 派工；修不動的才升級 Boss）。`

- [ ] **Step 4: docs/workflow.md** — three edits:

(a) §1 diagram: in the worker fan-out line add evva — change the two lines:

```
   │        ┌────────┬─────────┼─────────┬─────────┬───────────┬──────────┬────────┐
   │        ▼        ▼         ▼         ▼         ▼           ▼          ▼        ▼
   │    trader  analyst-flow analyst-news researcher risk-monitor reviewer watchdog evva
   │   (執行台) (技術面/指數) (戰術新聞)  (戰略前瞻)  (風控巡檢)  (每日復盤)(看門狗)(工程師)
```

(b) §3 table (after watchdog row):

```markdown
| **evva**（persona member） | friday 的 ticket / 訊息（無 cron） | **特派工程師**：接 Sunday 軟體改動 ticket（PRD 實作、bug 修復）→ 實作 → 測試綠 → commit main → 照 RUNBOOK 重啟（先知會 friday）→ 驗 `/health` → 回報；失敗 revert 回滾 | 不下單、不碰交易端點；不改憲法 |
```

Also update the §3 heading 「角色盤點（1 leader + 7 workers）」→「角色盤點（1 leader + 8 workers）」.

(c) §7 table — `docs/PRD/` row: change 讀者 from 「開發者」 to 「evva（特派工程師）與開發者」.

- [ ] **Step 5: CLAUDE.md 現況** — in the swarm consumer bullet (「swarm 消費端…1 leader friday + 7 workers」), change `7 workers` to `8 workers`, and append to that bullet:

```markdown
  evva 以 persona member 身分駐隊擔任**特派工程師**（接 friday 的 PRD/bug ticket：
  實作 → 測試綠 → commit → RUNBOOK 重啟 → 回報；見 `EVVA.md` 與
  docs/superpowers/specs/2026-06-11-evva-engineer-onboarding-design.md）。
```

- [ ] **Step 6: Commit**

```bash
cd /workspace/SUNDAY && git add agents/main/friday/system_prompt.md agents/skills/prd-ticket/SKILL.md docs/workflow.md CLAUDE.md
git commit -m "docs: wire resident engineer evva into friday's command loop and the workflow map"
```

---

## Post-plan notes (not tasks)

- **No pushes.** EVVA's release workflow reserves pushes for the four operator
  release commands; the feature branch stays local until the operator opens a
  PR to `dev`. SUNDAY `main` is a local workflow.
- **Host rollout order** (operator, out of repo scope): merge EVVA feature →
  update host `evva` binary → re-register the sunday swarm → run the SUNDAY
  spec §8 acceptance (assign a trivial ticket end-to-end).
- **Deliberately out of v1:** web add-member for personas, persona replicas,
  remote persona endpoints, solo auto-memory bridging.
