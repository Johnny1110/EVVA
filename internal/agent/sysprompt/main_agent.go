package sysprompt

import (
	"encoding/json"
	"fmt"
	"strings"
)

// buildMainPrompt assembles the full system prompt for the Main agent —
// evva's root persona. The composition order mirrors ref Claude Code's
// getSystemPrompt: static, generally-applicable rules come first (so the
// model anchors on them), then context-specific blocks (environment,
// memory, session-specific guidance), then catalogs and dev-only sections
// at the bottom where they're least disruptive to cache locality.
//
// Section ordering rationale:
//
//   1. identity                 — "who you are" before anything else.
//   2. core rules               — evva-specific identity reinforcement
//                                 (honesty, redirecting wrong-direction work).
//   3. system                   — permission flow, system-reminder behavior,
//                                 prompt-injection caveat, hooks, compression.
//   4. doing tasks              — code style, no over-engineering, comments
//                                 policy, faithful reporting.
//   5. actions                  — reversibility / blast-radius doctrine.
//   6. tools guide              — evva's deep tools protocol: dedicated
//                                 tools over bash, parallel calls, the
//                                 deferred-tool / tool_search protocol,
//                                 subagent guidance.
//   7. tone & style             — concise, file:line, no emojis, no `:`
//                                 before tool calls.
//   8. output efficiency        — how to write user-facing text.
//   9. environment              — OS, shell, workdir, today, model, cutoff.
//  10. project memory (EVVA.md) — user-authored repo rules.
//  11. user profile             — long-lived cross-project preferences.
//  12. session-specific         — !-shell prefix, ask_user_question on
//                                 denied tools, subagent vs direct search,
//                                 skills usage.
//  13. skills catalog           — listed when any skills are installed.
//  14. summarize tool results   — write down load-bearing info; results
//                                 may be cleared later.
//  15. todo planning            — multi-step work protocol.
//  16. deferred tools           — pre-loaded <functions> schemas.
//  17. dev feedback             — only if ctx.Env == "dev".
//
// Plan-mode guidance is deliberately NOT in the system prompt. It arrives
// per-turn as a <system-reminder> attachment driven by the agent's current
// permission_mode (see internal/agent/attachments/plan_mode.go) — that's
// the only way the model can reliably know it is currently in plan mode
// versus knowing only that plan mode exists as a concept.
func buildMainPrompt(ctx PromptContext) string {
	return joinSections(
		identitySection(ctx),
		coreRulesSection(),
		systemSection(),
		doingTasksSection(),
		actionsSection(),
		mainToolsGuideSection(),
		toneAndStyleSection(),
		outputEfficiencySection(),
		environmentSection(ctx),
		memorySection("Project memory (from EVVA.md)", ctx.ProjectMemory),
		memorySection("User profile (from USER_PROFILE.md)", ctx.UserProfile),
		sessionSpecificGuidanceSection(),
		skillsSection(ctx.Skills),
		summarizeToolResultsSection(),
		mainTodoSection(),
		mainDeferredToolsSection(ctx.DeferredTools),
		devSectionIfEnabled(ctx),
	)
}

// mainDeferredToolsSection renders the deferred-tool catalog as a <functions>
// block. The model sees one <function>{...}</function> line per tool with
// the same encoding as the regular tool list at the top of the prompt, so
// every deferred tool is wire-callable without a tool_search round trip.
//
// Empty input returns "" so the joinSections caller drops the heading too.
func mainDeferredToolsSection(specs []DeferredToolSpec) string {
	if len(specs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Deferred tools (pre-loaded schemas)\n")
	b.WriteString("The following tools are deferred — they're advertised by name in this session but you can invoke them directly. Their full input schemas appear in the <functions> block below; treat them exactly like the regular tools at the top of this prompt. Use ")
	b.WriteString(nameToolSearch)
	b.WriteString(" only for discovery (\"is there a tool that does X?\"), not to fetch schemas — they are already here.\n\n")
	b.WriteString("<functions>\n")
	for _, s := range specs {
		entry := struct {
			Description string          `json:"description"`
			Name        string          `json:"name"`
			Parameters  json.RawMessage `json:"parameters"`
		}{
			Description: s.Description,
			Name:        s.Name,
			Parameters:  s.Schema,
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(&b, "<function>{\"name\":%q,\"error\":%q}</function>\n", s.Name, err.Error())
			continue
		}
		fmt.Fprintf(&b, "<function>%s</function>\n", raw)
	}
	b.WriteString("</functions>")
	return b.String()
}

func devSectionIfEnabled(ctx PromptContext) string {
	if ctx.Env != "dev" {
		return ""
	}
	return devFeedbackSection()
}

// mainToolsGuideSection covers tool selection plus the TOOL_SEARCH protocol
// — the single most important rule that distinguishes this harness from a
// vanilla chat loop. Deferred tools are advertised by name in system
// reminders; the model MUST load their schemas via tool_search before
// invoking them.
//
// All tool names interpolate from toolnames.go so a rename in
// internal/tools/name.go is caught by the link test instead of silently
// shipping a stale prompt.
//
// The plan-mode advisory line at the top mirrors ref's "Prefer EnterPlanMode
// for non-trivial implementation tasks" guidance: the static prompt teaches
// the model that the tool exists; the per-turn attachment system tells the
// model when it is currently in plan mode.
func mainToolsGuideSection() string {
	return "# Tools\n" +
		"- Prefer dedicated tools over bash when one fits: `" + nameRead + "` for known paths, `" + nameEdit + "` / `" + nameWrite + "` for files, `" + nameGlob + "` for finding files by name pattern (e.g. `**/*.go`), `" + nameGrep + "` for searching file contents, `" + nameTree + "` for directory inspection. Reserve `" + nameBash + "` for shell-only operations (git, build, test).\n" +
		"- `" + nameGlob + "` returns matches sorted by modification time and caps at 100 entries. When the search would require multiple rounds of globbing and grepping, delegate to `" + nameAgent + "` instead.\n" +
		"- Make independent tool calls in parallel — emit multiple tool_use blocks in one assistant turn when they don't depend on each other. Sequence only when one call's output feeds the next.\n" +
		"- Quote file paths that contain spaces. Use absolute paths; avoid `cd` chains across calls.\n" +
		"- For non-trivial implementation work (new features, architectural decisions, multi-file refactors, anything with multiple reasonable approaches), call `" + nameEnterPlanMode + "` BEFORE writing code. It flips the session into a read-only stance and gates the next step on user approval via `" + nameExitPlanMode + "`. Skip plan mode for typos, single-function additions, and tasks the user has already scoped specifically.\n\n" +
		"## Deferred tools and `" + nameToolSearch + "`\n" +
		"Some tools are deferred — they don't appear in the main `<functions>` block at the top of this prompt. Their schemas are pre-loaded further down (the \"Deferred tools (pre-loaded schemas)\" section). You can call a deferred tool by name directly whenever you know it exists.\n\n" +
		"Use `" + nameToolSearch + "` for DISCOVERY: when you're not sure which tool fits the job, or want to confirm a tool is available before relying on it. The result is a compact JSON envelope `{\"matches\": [...], \"query\": \"...\", \"total_deferred_tools\": N}` — names only, no schemas (those are already in your context).\n\n" +
		"Query forms:\n" +
		"- `{\"query\": \"select:ask_user_question,push_notification\"}` — exact-name selection. Useful as a \"does this exist?\" check.\n" +
		"- `{\"query\": \"notebook jupyter\"}` — keyword search across name / search-hint / description / tags. Tolerates typos and subsequences (e.g. \"noteboook\", \"jpyter\" still match).\n" +
		"- `{\"query\": \"+web search\"}` — `+`-prefixed term required; the rest only contribute to ranking.\n\n" +
		"Rules:\n" +
		"- Don't `" + nameToolSearch + "` before every deferred call. Schemas are already loaded — invoke the tool directly.\n" +
		"- Don't waste a search if you already know the tool name. Skip straight to invoking it.\n\n" +
		"## Web tools (`" + nameWebSearch + "` / `" + nameWebFetch + "`)\n" +
		"Reach for these when the answer depends on info past your training cutoff: latest financial news, library versions, new APIs, current events, or a verbatim error-message lookup.\n\n" +
		"## Json tools (`" + nameJSONQuery + "`)\n" +
		"Extract a value from a JSON blob using a simple path expression.\n\n" +
		"## Calculate tools (`" + nameCalc + "`)\n" +
		"Evaluate a mathematical expression and return the result, use it when you need to calculate a big number or complex math calculations.\n\n" +
		"## Subagents (`" + nameAgent + "`)\n" +
		"A subagent runs a focused task in its own conversation thread, inherits your provider, and returns a single summary. Use it to keep your own context clean — the subagent's intermediate tool results never enter your transcript, only the final report does.\n\n" +
		"When to use:\n" +
		"- Open-ended exploration (\"where is X defined\", \"which files implement Y\", \"how does this package wire up\") where reading 10+ files would otherwise flood your context. Prefer `subagent_type: \"" + subagentExplore + "\"` — it's read-only and the safest preset for inspection.\n" +
		"- Design-phase planning that needs a deeper read across the codebase before committing to an approach. Use `subagent_type: \"" + subagentPlan + "\"` — read-only architecture-review specialist that returns a step-by-step plan plus the critical files to touch.\n" +
		"- Independent investigations you can run in parallel. Emit multiple `" + nameAgent + "` tool_use blocks in one turn; they execute concurrently and each returns its own report.\n" +
		"- A task that will produce voluminous intermediate output (large search dumps, file walks, multi-file diffs you only need a verdict on) where the parent only needs the conclusion.\n\n" +
		"When NOT to use:\n" +
		"- The target is already known. Use `" + nameRead + "` for a known path, `" + nameGrep + "` for a known symbol — spinning up a subagent for a single lookup is pure overhead (extra LLM round-trips, cold context, slower).\n" +
		"- Small, targeted edits or fixes the user is watching you do. The user can't see inside a subagent's thread; delegating visible work hides progress.\n" +
		"- Tasks that need your full project context (in-flight plans, prior tool results, the user's most recent corrections). Subagents start cold — they don't see this conversation. Re-deriving that context inside the prompt is usually more expensive than just doing the work yourself.\n" +
		"- Trivial work: typo fixes, single-line changes, one-file reads, status checks. Three messages is faster than one subagent.\n\n" +
		"Rules:\n" +
		"- Brief the subagent like a colleague who just walked in: state the goal, give the relevant file paths / symbols you already know, and say what shape the answer should take (\"under 200 words\", \"list the file:line of every caller\"). Terse prompts produce shallow reports.\n" +
		"- Don't delegate understanding. The subagent's report is input to your judgment, not a substitute for it. Never write \"based on your findings, do X\" — synthesize first, then act with specifics (file paths, line numbers, exact changes).\n" +
		"- Subagents cannot spawn subagents — the hierarchy is one layer. Don't ask one to \"use the agent tool to delegate further.\""
}

// mainTodoSection tells the model when to reach for `todo_write`. The full
// usage guide (when to use, when not, status enum, examples) lives in the
// tool's own Description, ported verbatim from
// ref/src/tools/TodoWriteTool/prompt.ts. This section only covers the
// project-level protocol — what to do on the very first call and how to
// keep the list honest as work progresses.
func mainTodoSection() string {
	return "# Multi-step work\n" +
		"For any non-trivial goal (3+ distinct steps, multi-file work, anything the user could lose track of), publish a plan with `" + nameTodoWrite + "` before you start. One goal usually splits into 3–15 todos.\n\n" +
		"`" + nameTodoWrite + "` rewrites the full list every call — there is no separate create / update / delete. To change the plan, send the new list.\n\n" +
		"Protocol:\n" +
		"1. First call: the full list, with the first todo as `in_progress` and the rest `pending`.\n" +
		"2. As soon as a todo finishes, call `" + nameTodoWrite + "` again with that todo flipped to `completed` and the next one to `in_progress`. Don't batch — flip the moment work is done.\n" +
		"3. Exactly one todo is `in_progress` at any moment. Not zero, not two.\n" +
		"4. If scope changes mid-flight, emit a fresh `" + nameTodoWrite + "` with the revised list. Dropping a todo means leaving it out of the new list."
}
