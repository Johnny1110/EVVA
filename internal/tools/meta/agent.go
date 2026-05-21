package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/johnny1110/evva/pkg/constant"

	"github.com/johnny1110/evva/internal/observable"
	"github.com/johnny1110/evva/pkg/tools"
)

// SpawnerLookup is the function shape a ToolState method (or any closure)
// satisfies to provide late-bound access to a SubagentSpawner. AgentTool
// keeps the lookup, not the spawner, so the order in which the agent and
// the tool are constructed doesn't matter — the spawner can be installed
// after the tool already exists.
type SpawnerLookup func() SubagentSpawner

// SpawnGroupDomain is the observable.Change.Domain value carried by every
// SpawnGroup change. Subscribers switch on this string and type-assert
// Change.Payload to SubagentSnapshot.
const SpawnGroupDomain = "subagent"

// SubagentSnapshot is the typed payload carried in observable.Change.Payload
// for every "subagent" domain change. Each notification ships a full snapshot
// so consumers can render the row without keeping their own state.
//
// Async marks subagents whose results must be picked up via
// SpawnGroup.DrainCompleted (the main agent loop does this between turns).
// Sync subagents deliver their result through the tool return channel and
// are Remove'd as soon as the spawner finishes, so they never sit in
// DrainCompleted's queue.
type SubagentSnapshot struct {
	Name    string
	ID      string
	Type    string // "explore", "general-purpose", ...
	Status  string
	Async   bool
	JobDesc string // prompt summary
	Summary string // result summary (set on Report)
	Err     string // error message (set on Crush)
}

type spawnedAgent struct {
	snap SubagentSnapshot
	done bool // true once phase ∈ {done, crushed}
}

// SpawnGroup is the per-agent panel of in-flight subagents. It is an
// observable.Store: every mutation fans through the framework so the TUI
// (and any other subscriber) can re-render without per-store wiring.
//
// Lifecycle: Add → optional Status updates → Report | Crush → Remove (sync) /
// DrainCompleted (async). Sync subagents are short-lived in the panel —
// the spawner calls Remove right after the child returns. Async subagents
// stay in the panel until the parent loop drains them between turns.
type SpawnGroup struct {
	observable.Observable

	mu     sync.Mutex
	agents map[string]*spawnedAgent
	order  []string // insertion order for stable Drain
}

func NewSpawnGroup() *SpawnGroup {
	return &SpawnGroup{agents: map[string]*spawnedAgent{}}
}

// Domain identifies this store on the change stream.
func (g *SpawnGroup) Domain() string { return SpawnGroupDomain }

// Add records a new subagent in the init phase. async marks subagents
// whose result will be delivered through DrainCompleted instead of the
// usual tool-return path.
func (g *SpawnGroup) Add(name, id, agentType, jobDesc string, async bool) {
	snap := SubagentSnapshot{
		Name:    name,
		ID:      id,
		Type:    agentType,
		Status:  constant.INIT.String(),
		Async:   async,
		JobDesc: jobDesc,
	}
	g.mu.Lock()
	g.agents[id] = &spawnedAgent{snap: snap}
	g.order = append(g.order, id)
	g.mu.Unlock()

	g.Notify(observable.Change{Domain: SpawnGroupDomain, Op: "added", ID: id, Payload: snap})
}

// Status updates the lifecycle phase of an in-flight subagent and notifies
// observers. No-op when the id is unknown.
func (g *SpawnGroup) Status(id string, status constant.AgentStatus) {
	g.mu.Lock()
	a, ok := g.agents[id]
	if !ok {
		g.mu.Unlock()
		return
	}
	a.snap.Status = status.String()
	snap := a.snap
	g.mu.Unlock()

	g.Notify(observable.Change{Domain: SpawnGroupDomain, Op: "status", ID: id, Payload: snap})
}

// Report marks a subagent as completed and records its result summary.
// Async subagents in this state are picked up by DrainCompleted; sync
// subagents are immediately Remove'd by the spawner.
func (g *SpawnGroup) Report(id, summary string) {
	g.mu.Lock()
	a, ok := g.agents[id]
	if !ok {
		g.mu.Unlock()
		return
	}
	a.snap.Status = constant.READY_REPORT.String()
	a.snap.Summary = summary
	a.done = true
	snap := a.snap
	g.mu.Unlock()

	g.Notify(observable.Change{Domain: SpawnGroupDomain, Op: "report", ID: id, Payload: snap})
}

// Crush marks a subagent as failed.
func (g *SpawnGroup) Crush(id string, summary string, err error) {
	msg := "subagent crushed"
	if err != nil {
		msg = err.Error()
	}
	g.mu.Lock()
	a, ok := g.agents[id]
	if !ok {
		g.mu.Unlock()
		return
	}
	a.snap.Status = constant.CRUSHED.String()
	a.snap.Err = msg
	a.snap.Summary = summary
	a.done = true
	snap := a.snap
	g.mu.Unlock()

	g.Notify(observable.Change{Domain: SpawnGroupDomain, Op: "crushed", ID: id, Payload: snap})
}

// Remove deletes an entry from the group and notifies observers. Used by
// the spawner for sync subagents (their result is delivered through the
// tool return channel, not through DrainCompleted).
func (g *SpawnGroup) Remove(id string) {
	g.mu.Lock()
	a, ok := g.agents[id]
	if !ok {
		g.mu.Unlock()
		return
	}
	snap := a.snap
	delete(g.agents, id)
	for i, oid := range g.order {
		if oid == id {
			g.order = append(g.order[:i], g.order[i+1:]...)
			break
		}
	}
	g.mu.Unlock()

	g.Notify(observable.Change{Domain: SpawnGroupDomain, Op: "removed", ID: id, Payload: snap})
}

// Snapshot returns a stable copy of every tracked subagent in insertion
// order. Read-only; the panel's drain queue is untouched. UIs poll this
// to render without racing against in-flight goroutines.
func (g *SpawnGroup) Snapshot() []SubagentSnapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]SubagentSnapshot, 0, len(g.order))
	for _, id := range g.order {
		if a, ok := g.agents[id]; ok {
			out = append(out, a.snap)
		}
	}
	return out
}

// DrainCompleted atomically extracts and removes every async subagent that
// has reached a terminal phase (done or crushed). Returned snapshots are
// in insertion order. Each removal emits an Op:"removed" change so the
// TUI clears its row.
//
// Sync subagents are never returned here — the spawner removes them
// directly via Remove as soon as their tool-return path completes.
func (g *SpawnGroup) DrainCompleted() []SubagentSnapshot {
	g.mu.Lock()
	out := make([]SubagentSnapshot, 0)
	keep := make([]string, 0, len(g.order))
	for _, id := range g.order {
		a, ok := g.agents[id]
		if !ok {
			continue
		}
		// collect async agent which is done(crushed or ready_report)
		if a.done && a.snap.Async {
			out = append(out, a.snap)
			delete(g.agents, id)
			continue
		}
		keep = append(keep, id)
	}
	g.order = keep
	g.mu.Unlock()

	for _, snap := range out {
		g.Notify(observable.Change{Domain: SpawnGroupDomain, Op: "removed", ID: snap.ID, Payload: snap})
	}
	return out
}

// AgentTool is the LLM-facing handle for spawning subagents. The actual
// work is delegated to a SubagentSpawner installed by the agent layer.
type AgentTool struct {
	lookup SpawnerLookup
	group  *SpawnGroup
}

// NewAgent constructs an AgentTool that reads its spawner via lookup at
// Execute time. lookup may be nil (yields a clear runtime error if the
// model invokes the tool); it may also return nil (same outcome).
func NewAgent(lookup SpawnerLookup, spawnGroup *SpawnGroup) *AgentTool {
	return &AgentTool{lookup: lookup, group: spawnGroup}
}

func (t *AgentTool) Name() string { return string(tools.AGENT) }

func (t *AgentTool) Description() string {
	return `Launch a new agent to handle complex, multi-step tasks. Each agent type has specific capabilities and tools available to it.

Available agent types and the tools they have access to:
- explore: Fast read-only search agent for locating code. Use it to find files by pattern (e.g. "src/**/*.go"), grep for symbols or keywords, or answer "where is X defined / which files reference Y." Do NOT use it for code review, cross-file consistency checks, or open-ended analysis — it reads excerpts rather than whole files and will miss content past its read window. When calling, specify search breadth: "quick" for a single targeted lookup, "medium" for moderate exploration, or "very thorough" to search across multiple locations and naming conventions.
- plan: Software architect agent for designing implementation plans. Use when you need a step-by-step plan and a list of critical files to modify, especially during plan-mode Phase 2 (Design). Read-only — returns a written plan, never edits files. Multiple Plan agents in parallel can produce different perspectives (simplicity vs performance vs maintainability) on the same problem.
- general-purpose: General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks. Use when searching for a keyword or file and you are not confident you will find the right match in the first few tries.

When using the agent tool, specify a subagent_type parameter to select which agent type to use. If omitted, the general-purpose agent is used.

When NOT to use the agent tool:
- If you want to read a specific file path, use the read or glob tool instead — finding the match is faster.
- If you are searching for a specific class definition like "class Foo", use grep or glob directly — faster than a subagent round-trip.
- If you are searching for code within a specific file or set of 2–3 files, use read instead — faster.
- Trivial work: typo fixes, single-line edits, status checks. Three messages is faster than one subagent.

Usage notes:
- Always include a short description (3–5 words) summarizing what the agent will do.
- Launch multiple agents concurrently whenever possible — emit several agent tool_use blocks in one assistant turn when the work is independent. They execute in parallel.
- When the agent is done, it returns a single message back to you. The result is NOT visible to the user. To show the user the result, send a text message back with a concise summary.
- You can optionally run agents in the background by setting ` + "`async_mode: true`" + `. When async, the spawner returns an ack immediately and the eventual summary is injected into your next turn — do NOT sleep, poll, or proactively check on its progress. Continue with other work or respond to the user instead.
- Foreground vs background: use foreground (default) when you need the agent's results before you can proceed; use async when you have genuinely independent work to do in parallel.
- Each agent invocation starts fresh — provide a complete task description in ` + "`prompt`" + `.
- The agent's outputs should generally be trusted.
- Clearly tell the agent whether you expect it to write code or just to do research (search, file reads, web fetches, etc.), since it is not aware of the user's intent.
- If the user specifies that they want you to run agents "in parallel", you MUST send a single message with multiple agent tool_use blocks. For example, if you need to launch both a build-validator agent and a test-runner agent in parallel, send a single message with both tool calls.
- ` + "`level: 2`" + ` costs more — only request it when the task genuinely needs deeper reasoning (subtle bug hunts, architectural calls). Routine searches stay at level 1.
- Subagents cannot spawn subagents — the hierarchy is exactly one layer deep.

## Writing the prompt

Brief the agent like a smart colleague who just walked into the room — it hasn't seen this conversation, doesn't know what you've tried, doesn't understand why this task matters.
- Explain what you're trying to accomplish and why.
- Describe what you've already learned or ruled out.
- Give enough context about the surrounding problem that the agent can make judgment calls rather than just following a narrow instruction.
- If you need a short response, say so ("report in under 200 words").
- Lookups: hand over the exact command. Investigations: hand over the question — prescribed steps become dead weight when the premise is wrong.

Terse command-style prompts produce shallow, generic work.

**Never delegate understanding.** Don't write "based on your findings, fix the bug" or "based on the research, implement it." Those phrases push synthesis onto the agent instead of doing it yourself. Write prompts that prove you understood: include file paths, line numbers, what specifically to change.

Example usage:

<example>
user: "What's left on this branch before we can ship?"
assistant: <thinking>A survey question across git state, tests, and config. I'll delegate it and ask for a short report so the raw command output stays out of my context.</thinking>
agent({
  "name": "ship-audit",
  "description": "Branch ship-readiness audit",
  "subagent_type": "general-purpose",
  "prompt": "Audit what's left before this branch can ship. Check: uncommitted changes, commits ahead of main, whether tests exist, whether CI-relevant files changed. Report a punch list — done vs. missing. Under 200 words."
})
<commentary>
The prompt is self-contained: it states the goal, lists what to check, and caps the response length. The agent's report comes back as the tool result; relay the findings to the user.
</commentary>
</example>

<example>
user: "where is the auth middleware wired?"
assistant: <thinking>I'll use explore — read-only, fast, and the answer is a file/line lookup, not a synthesis task.</thinking>
agent({
  "name": "auth-locate",
  "description": "Find auth middleware",
  "subagent_type": "explore",
  "prompt": "Locate the file and exact line where the auth middleware is wired into the HTTP router. Report file:line for both the middleware function definition and its registration. Under 100 words."
})
</example>`
}

func (t *AgentTool) Schema() json.RawMessage {
	types := t.subagentTypes()
	enumJSON, _ := json.Marshal(types)
	return json.RawMessage(fmt.Sprintf(`{
		"type":"object",
		"additionalProperties":false,
		"required":["name", "description","prompt"],
		"properties":{
			"name":{"type":"string","description":"A short nickname"},
			"description":{"type":"string","description":"A short (3-5 word) description of the task"},
			"prompt":{"type":"string","description":"The full task prompt for the sub-agent"},
			"subagent_type":{"type":"string","enum":%s,"description":"Which preset profile to use. Defaults to general-purpose. \"explore\" is read-only and good for codebase inspection."},
			"level":{"type":"integer","enum":[1,2],"default":1,"description":"Model tier within the parent's provider. 1=general, 2=thinking Defaults to 1. Use 2 only when the task genuinely needs deeper reasoning."},
			"async_mode":{"type":"boolean","default":false,"description":"Let the subagent run in the background; the spawner returns an ack immediately and the eventual summary is injected into the parent's next turn."}
		}
	}`, enumJSON))
}

// subagentTypes resolves the enum members for the schema's subagent_type
// field. Reads through the lookup so the registry contents at Schema()
// time win — Phase 6 disk subagents become wire-callable as soon as the
// registry sees them. Falls back to the built-in pair when no spawner is
// installed (tests, degenerate setups) so the schema is always valid.
func (t *AgentTool) subagentTypes() []string {
	if t.lookup != nil {
		if sp := t.lookup(); sp != nil {
			if names := sp.SubagentTypes(); len(names) > 0 {
				return names
			}
		}
	}
	return []string{"explore", "plan", "general-purpose"}
}

type agentInput struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
	Level        int    `json:"level"`
	AsyncMode    bool   `json:"async_mode"`
}

func (t *AgentTool) Execute(ctx context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
	var in agentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("agent: decode: %v", err)}, nil
	}
	if in.Prompt == "" {
		return tools.Result{IsError: true, Content: "agent: prompt is required"}, nil
	}
	if t.lookup == nil {
		return tools.Result{IsError: true, Content: "agent: no spawner lookup configured"}, nil
	}
	spawner := t.lookup() // the spawner should be main(root) agent only.
	if spawner == nil {
		// Likely cause: the AGENT tool was reached from a subagent (the agent layer only installs the spawner on root agents).
		return tools.Result{IsError: true, Content: "agent: subagent spawning is only available from the root agent"}, nil
	}

	kind := in.SubagentType
	if kind == "" {
		kind = "general-purpose"
	}
	logger.Info("subagent.spawn", "kind", kind, "name", in.Name, "async", in.AsyncMode, "level", in.Level)

	out, err := spawner.Spawn(ctx, SpawnRequest{
		Name:      in.Name,
		Kind:      kind,
		Desc:      in.Description,
		Prompt:    in.Prompt,
		Level:     in.Level,
		AsyncMode: in.AsyncMode, // turn this off in dev mode.
	})

	if err != nil {
		if errors.Is(err, ErrSubagentForbidden) {
			// Recoverable — model can ditch the subagent plan and try something else.
			logger.Warn("subagent.fail", "kind", kind, "reason", "forbidden", "err", err)
			return tools.Result{IsError: true, Content: fmt.Sprintf("%s \n [%s]", out, err.Error())}, nil
		}
		// Other errors abort the parent loop — they are Go-level failures
		// (LLM transport, tool panics) the model can't recover from.
		logger.Warn("subagent.fail", "kind", kind, "err", err)
		return tools.Result{IsError: true, Content: fmt.Sprintf("agent: %s %v", out, err)}, err
	}
	// If this is a async mode agent, output will be like "subagent running in background."
	return tools.Result{Content: out}, nil
}
