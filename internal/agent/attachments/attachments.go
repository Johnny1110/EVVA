// Package attachments computes per-turn <system-reminder>-wrapped
// messages the agent loop prepends to the user's incoming prompts.
//
// Today the only attachment the package produces is the plan-mode
// reminder family (full / sparse / exit / re-entry). The package is
// scoped to that responsibility on purpose — every "this state should
// be re-injected on every user prompt" feature in the future (e.g. a
// transcript-classifier reminder, an auto-mode reminder, an MCP-server
// status reminder) can sit alongside as a sibling computer and the
// state_machine wiring stays one call.
//
// Ports the design of ref/src/utils/attachments.ts:getPlanModeAttachments
// and ref/src/utils/messages.ts:getPlanModeInstructions, adapted to
// evva's per-agent PlanModeState (internal/permission). The throttle and
// reminder-cycle defaults match ref: TURNS_BETWEEN_ATTACHMENTS=4,
// FULL_REMINDER_EVERY_N=5.
package attachments

import (
	"fmt"
	"strings"

	"github.com/johnny1110/evva/internal/permission"
)

// Workflow chooses which plan-mode workflow the reminders describe.
// "interview" — the iterative pair-planning workflow: explore code,
// update the plan file as understanding grows, ask the user when you
// hit ambiguity, repeat. Fewer moving parts, default for evva.
// "v2" — the 5-phase workflow from ref Claude Code: Explore subagents
// in parallel, Plan subagents for design, then Review, Final Plan,
// ExitPlanMode. Better for very large refactors with multiple valid
// approaches. Opt-in via config.PlanModeWorkflow.
type Workflow string

const (
	WorkflowInterview Workflow = "interview"
	WorkflowV2        Workflow = "v2"
)

// Tunables. Default values mirror ref's PLAN_MODE_ATTACHMENT_CONFIG.
const (
	// TurnsBetweenAttachments is the throttle window. After the first
	// reminder fires, the next reminder waits until at least this many
	// user-prompt turns have elapsed without one. Keeps the per-turn
	// token cost bounded on long plan-mode sessions.
	TurnsBetweenAttachments = 4

	// FullReminderEveryN is the cycle window. Reminder #1, #1+N, #1+2N…
	// are FULL; every other reminder is the sparse one-liner. The first
	// turn in plan mode is always FULL by construction (state's
	// attachmentsSinceExit==0 on entry).
	FullReminderEveryN = 5
)

// Input is the snapshot ComputePlanMode reads on each user-prompt turn.
// State holds the cycle / throttle counters and the one-shot flags
// (pending-exit, has-exited). Mode + PlanFilePath + PlanExists feed the
// reminder text. WorkflowVariant chooses between interview / v2 prompt
// bodies; empty defaults to interview.
type Input struct {
	State           *permission.PlanModeState
	Mode            permission.Mode
	PlanFilePath    string
	PlanExists      bool
	WorkflowVariant Workflow
}

// ComputePlanMode returns the <system-reminder>-wrapped reminder texts
// to prepend to the next user prompt. May return nil. Mutates Input.State
// to record what was emitted, and to advance throttle / one-shot counters.
//
// Order (when multiple fire on the same turn):
//
//  1. Exit reminder (if a pending exit reminder is queued from a
//     plan→non-plan transition).
//  2. Re-entry reminder (if HasExited was true and we're back in plan
//     mode; one-shot).
//  3. Plan-mode workflow reminder (FULL or sparse, subject to the
//     throttle).
//
// Each returned string is already wrapped in <system-reminder> tags;
// callers append them as individual RoleUser messages to the session.
func ComputePlanMode(in Input) []string {
	var out []string

	// (1) Exit reminder fires regardless of current mode. The transition
	// hub queues this on every plan→non-plan transition; consuming it
	// here clears the flag so the next user prompt does not see it
	// again.
	if in.State.ConsumePendingExitReminder() {
		out = append(out, wrap(exitReminderBody(in.PlanFilePath)))
	}

	if in.Mode != permission.ModePlan {
		return out
	}

	// (2) Re-entry reminder — one-shot. Fires only when the session has
	// previously exited plan mode and is now back in it (e.g. the user
	// Shift+Tabbed back into plan, or called enter_plan_mode again).
	if in.State.ConsumeReentry() {
		out = append(out, wrap(reentryReminderBody(in.PlanFilePath)))
	}

	// (3) Plan-mode workflow reminder. The first turn in plan mode
	// (attachmentsSinceExit == 0) always emits FULL; subsequent turns
	// throttle by TurnsBetweenAttachments.
	emitted := in.State.AttachmentsSinceExit()
	if emitted == 0 {
		out = append(out, wrap(workflowReminder(in, /*full=*/ true)))
		in.State.RecordAttachmentEmitted()
		return out
	}
	// Bump-then-check so "TurnsBetweenAttachments=4" means "fire on the
	// 4th turn since the last emission". Mirrors ref's turnCount-based
	// throttle (turnCount in ref is scanned from the message history;
	// here we maintain it on the state holder).
	in.State.RecordTurnWithoutAttachment()
	if in.State.TurnsSinceLastAttachment() < TurnsBetweenAttachments {
		return out
	}
	// Decide full vs sparse from the projected count (count after this
	// emission). Mirrors ref: attachmentCount % N == 1 → full.
	projected := emitted + 1
	full := projected%FullReminderEveryN == 1
	out = append(out, wrap(workflowReminder(in, full)))
	in.State.RecordAttachmentEmitted()
	return out
}

// wrap brackets a reminder body in <system-reminder> tags. Tools and
// fs/read share this convention so the model parses every reminder
// the same way; consolidating the wrapper here means we never ship a
// reminder without the tags.
func wrap(body string) string {
	return "<system-reminder>\n" + strings.TrimRight(body, "\n") + "\n</system-reminder>"
}

// workflowReminder returns the body of a plan-mode reminder. Picks
// between FULL and SPARSE, and between interview / v2 workflow variants.
func workflowReminder(in Input, full bool) string {
	if !full {
		return sparseReminderBody(in.PlanFilePath)
	}
	if in.WorkflowVariant == WorkflowV2 {
		return v2FullReminderBody(in.PlanFilePath, in.PlanExists)
	}
	return interviewFullReminderBody(in.PlanFilePath, in.PlanExists)
}

// planFileInfo renders the one-line note about whether the plan file
// already exists, mirroring ref's getPlanModeV2Instructions branch.
func planFileInfo(planFilePath string, planExists bool) string {
	if planExists {
		return fmt.Sprintf("A plan file already exists at %s. You can read it and make incremental edits using the `edit` tool.", planFilePath)
	}
	return fmt.Sprintf("No plan file exists yet. You should create your plan at %s using the `write` tool.", planFilePath)
}

// interviewFullReminderBody is the iterative pair-planning workflow
// reminder — ported from ref's getPlanModeInterviewInstructions
// (ref/src/utils/messages.ts:3323-3378). Default for evva: simpler
// loop, no mandatory subagent delegation, scales gracefully to small
// and large tasks alike.
func interviewFullReminderBody(planFilePath string, planExists bool) string {
	return fmt.Sprintf(`Plan mode is active. The user indicated that they do not want you to execute yet — you MUST NOT make any edits (with the exception of the plan file mentioned below), run any non-readonly tools (including changing configs or making commits), or otherwise make any changes to the system. This supersedes any other instructions you have received.

## Plan File Info:
%s

## Iterative Planning Workflow

You are pair-planning with the user. Explore the code to build context, ask the user questions when you hit decisions you can't make alone, and write your findings into the plan file as you go. The plan file (above) is the ONLY file you may edit — it starts as a rough skeleton and gradually becomes the final plan.

### The Loop

Repeat this cycle until the plan is complete:

1. **Explore** — Use `+"`read`"+`, `+"`glob`"+`, `+"`grep`"+`, `+"`tree`"+` to read code. Look for existing functions, utilities, and patterns to reuse. You can use the `+"`agent`"+` tool with `+"`subagent_type=\"explore\"`"+` to parallelize complex searches without filling your context, though for straightforward queries direct tools are simpler.
2. **Update the plan file** — After each discovery, immediately capture what you learned. Don't wait until the end.
3. **Ask the user** — When you hit an ambiguity or decision you can't resolve from code alone, use `+"`ask_user_question`"+`. Then go back to step 1.

### First Turn

Start by quickly scanning a few key files to form an initial understanding of the task scope. Then write a skeleton plan (headers and rough notes) and ask the user your first round of questions. Don't explore exhaustively before engaging the user.

### Asking Good Questions

- Never ask what you could find out by reading the code.
- Batch related questions together (use multi-question `+"`ask_user_question`"+` calls).
- Focus on things only the user can answer: requirements, preferences, tradeoffs, edge case priorities.
- Scale depth to the task — a vague feature request needs many rounds; a focused bug fix may need one or none.

### Plan File Structure

Your plan file should be divided into clear sections using markdown headers, based on the request. Fill out these sections as you go.
- Begin with a **Context** section: explain why this change is being made — the problem or need it addresses, what prompted it, and the intended outcome.
- Include only your recommended approach, not all alternatives.
- Ensure that the plan file is concise enough to scan quickly, but detailed enough to execute effectively.
- Include the paths of critical files to be modified.
- Reference existing functions and utilities you found that should be reused, with their file paths.
- Include a verification section describing how to test the changes end-to-end (run the code, run tests).

### When to Converge

Your plan is ready when you've addressed all ambiguities and it covers: what to change, which files to modify, what existing code to reuse (with file paths), and how to verify the changes. Call `+"`exit_plan_mode`"+` when the plan is ready for approval.

### Ending Your Turn

Your turn should only end by either:
- Using `+"`ask_user_question`"+` to gather more information.
- Calling `+"`exit_plan_mode`"+` when the plan is ready for approval.

**Important:** Use `+"`exit_plan_mode`"+` to request plan approval. Do NOT ask about plan approval via text or `+"`ask_user_question`"+`.`, planFileInfo(planFilePath, planExists))
}

// v2FullReminderBody is the 5-phase workflow reminder — ported from ref's
// getPlanModeV2Instructions (ref/src/utils/messages.ts:3207-3297) with
// the PLAN_PHASE4_CONTROL variant. Opt-in via config.PlanModeWorkflow="v2".
func v2FullReminderBody(planFilePath string, planExists bool) string {
	return fmt.Sprintf(`Plan mode is active. The user indicated that they do not want you to execute yet — you MUST NOT make any edits (with the exception of the plan file mentioned below), run any non-readonly tools (including changing configs or making commits), or otherwise make any changes to the system. This supersedes any other instructions you have received.

## Plan File Info:
%s
You should build your plan incrementally by writing to or editing this file. NOTE that this is the only file you are allowed to edit — other than this you are only allowed to take READ-ONLY actions.

## Plan Workflow

### Phase 1: Initial Understanding

Goal: Gain a comprehensive understanding of the user's request by reading through code and asking them questions. Critical: In this phase you should only use the `+"`explore`"+` subagent type.

1. Focus on understanding the user's request and the code associated with their request. Actively search for existing functions, utilities, and patterns that can be reused — avoid proposing new code when suitable implementations already exist.

2. **Launch up to 3 `+"`explore`"+` agents IN PARALLEL** (single message, multiple `+"`agent`"+` tool_use blocks) to efficiently explore the codebase.
   - Use 1 agent when the task is isolated to known files, the user provided specific file paths, or you're making a small targeted change.
   - Use multiple agents when: the scope is uncertain, multiple areas of the codebase are involved, or you need to understand existing patterns before planning.
   - Quality over quantity — 3 agents maximum, but you should try to use the minimum number of agents necessary (usually just 1).
   - If using multiple agents: Provide each agent with a specific search focus or area to explore. Example: One agent searches for existing implementations, another explores related components, a third investigating testing patterns.

### Phase 2: Design

Goal: Design an implementation approach.

Launch `+"`plan`"+` agent(s) to design the implementation based on the user's intent and your exploration results from Phase 1. You can launch up to 3 agents in parallel.

**Guidelines:**
- **Default**: Launch at least 1 plan agent for most tasks — it helps validate your understanding and consider alternatives.
- **Skip agents**: Only for truly trivial tasks (typo fixes, single-line changes, simple renames).
- **Multiple agents**: Use multiple agents for complex tasks that benefit from different perspectives.

Examples of when to use multiple plan agents:
- The task touches multiple parts of the codebase.
- It's a large refactor or architectural change.
- There are many edge cases to consider.
- You'd benefit from exploring different approaches.

Example perspectives by task type:
- New feature: simplicity vs performance vs maintainability.
- Bug fix: root cause vs workaround vs prevention.
- Refactoring: minimal change vs clean architecture.

In the agent prompt:
- Provide comprehensive background context from Phase 1 exploration including filenames and code path traces.
- Describe requirements and constraints.
- Request a detailed implementation plan.

### Phase 3: Review

Goal: Review the plan(s) from Phase 2 and ensure alignment with the user's intentions.
1. Read the critical files identified by agents to deepen your understanding.
2. Ensure that the plans align with the user's original request.
3. Use `+"`ask_user_question`"+` to clarify any remaining questions with the user.

### Phase 4: Final Plan

Goal: Write your final plan to the plan file (the only file you can edit).
- Begin with a **Context** section: explain why this change is being made — the problem or need it addresses, what prompted it, and the intended outcome.
- Include only your recommended approach, not all alternatives.
- Ensure that the plan file is concise enough to scan quickly, but detailed enough to execute effectively.
- Include the paths of critical files to be modified.
- Reference existing functions and utilities you found that should be reused, with their file paths.
- Include a verification section describing how to test the changes end-to-end (run the code, run tests).

### Phase 5: Call `+"`exit_plan_mode`"+`

At the very end of your turn, once you have asked the user questions and are happy with your final plan file — you should always call `+"`exit_plan_mode`"+` to indicate to the user that you are done planning.

This is critical — your turn should only end with either using the `+"`ask_user_question`"+` tool OR calling `+"`exit_plan_mode`"+`. Do not stop unless it's for these 2 reasons.

**Important:** Use `+"`ask_user_question`"+` ONLY to clarify requirements or choose between approaches. Use `+"`exit_plan_mode`"+` to request plan approval. Do NOT ask about plan approval in any other way — no text questions, no `+"`ask_user_question`"+`. Phrases like "Is this plan okay?", "Should I proceed?", "How does this plan look?", "Any changes before we start?", or similar MUST use `+"`exit_plan_mode`"+`.

NOTE: At any point in time through this workflow you should feel free to ask the user questions or clarifications using the `+"`ask_user_question`"+` tool. Don't make large assumptions about user intent. The goal is to present a well-researched plan to the user, and tie any loose ends before implementation begins.`, planFileInfo(planFilePath, planExists))
}

// sparseReminderBody is the throttle-cycle one-liner reminder — ported
// from ref's getPlanModeV2SparseInstructions.
func sparseReminderBody(planFilePath string) string {
	return fmt.Sprintf("Plan mode still active (see full instructions earlier in conversation). Read-only except plan file (%s). End turns with `ask_user_question` (for clarifications) or `exit_plan_mode` (for plan approval). Never ask about plan approval via text or `ask_user_question`.", planFilePath)
}

// exitReminderBody is the one-shot reminder that fires on plan→non-plan
// transitions. Tells the model the constraints are lifted.
func exitReminderBody(planFilePath string) string {
	if planFilePath == "" {
		return "You have exited plan mode. You can now make edits, run tools, and take actions."
	}
	return fmt.Sprintf("You have exited plan mode. You can now make edits, run tools, and take actions. The plan file is located at %s if you need to reference it.", planFilePath)
}

// reentryReminderBody is the one-shot reminder that fires when the model
// re-enters plan mode after a prior exit. Points at the existing plan
// file so the model picks up where it left off instead of starting from
// scratch.
func reentryReminderBody(planFilePath string) string {
	return fmt.Sprintf("You have re-entered plan mode. The prior plan file at %s already exists; read it before starting a new plan so you can build on the previous work instead of duplicating it.", planFilePath)
}
