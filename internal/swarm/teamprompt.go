package swarm

import (
	"fmt"
	"strings"

	"github.com/johnny1110/evva/internal/swarm/agentdef"
)

// teamprompt.go auto-injects the swarm collaboration protocol into every
// member's system prompt at construction. The operator should only have to write
// a member's *persona and domain* in its system_prompt.md ("you are a backend
// engineer…") and decide *when/what* to communicate; the mechanics — how the
// task ledger works, which tool does what, how to report — are the swarm's job
// to teach, not the user's to re-derive. This pairs with the role-based tool
// injection (internal/swarm/tools.Set): a leader always gets the leader tool set
// + the leader protocol, a worker the worker set + the worker protocol, with no
// declaration in active.yml / deferr.yml.
//
// Injection happens in registerDef (space.go), the single chokepoint every
// member passes through — initial assembly, dynamic AddMember, and restart
// rebuild alike.

// injectTeamProtocol returns the member's effective system prompt: its authored
// persona, then its swarm grounding (space/name/role), then the role's
// collaboration protocol, then — for members that carry a file-write tool —
// the long-term memory protocol (RP-25). The persona leads (it is the agent's
// identity); the grounding and protocol are appended as standard operational
// sections. A blank persona still yields a usable prompt (grounding + protocol
// only). canWriteMemory gates the memory section: a member with no write/edit
// tool cannot maintain memory files, so teaching it the protocol is noise.
func injectTeamProtocol(persona, name, space string, role agentdef.Role, canWriteMemory bool) string {
	var b strings.Builder
	if p := strings.TrimRight(persona, "\n"); p != "" {
		b.WriteString(p)
		b.WriteString("\n\n")
	}
	b.WriteString(swarmIdentity(name, space, role))
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

// memoryProtocol teaches a member its long-term memory discipline (RP-25): the
// on-disk home, the save format (the solo typed-memdir conventions), the wake
// index contract, and the write-own/read-all governance. Parametrized only by
// the member's FIXED coordinates (name, role tier) so the section — like all
// swarm prompt sections — is byte-stable across rebuilds (RP-5). This section
// is the framework collecting what Sunday hand-wrote into 8 personas.
func memoryProtocol(name string, role agentdef.Role) string {
	tier := "sub"
	if role == agentdef.RoleLeader {
		tier = "main"
	}
	dir := "agents/" + tier + "/" + name + "/memory"
	return "## Your long-term memory\n\n" +
		"You have a persistent memory directory at `" + dir + "/` (relative to your working directory). " +
		"It survives restarts and context compaction — your transcript does not, so anything worth keeping across sessions belongs there. " +
		"The directory already exists; write files into it with your ordinary file tools (writes there are auto-allowed).\n\n" +
		"- **Wake-up index:** when your `" + dir + "/MEMORY.md` index exists, every wake message carries it. " +
		"It is a table of contents, not the memories themselves — read the linked file before relying on one, and verify time-sensitive facts against current state before acting on them.\n" +
		"- **Saving is two steps:** write one fact per file with frontmatter (`name:`, `description:`, `type: user|feedback|project|reference`), " +
		"then add one line to `MEMORY.md`: `- [Title](file.md) — one-line hook`. Keep the index lean (one line per memory, ~200 lines max); never write memory content into the index itself.\n" +
		"- **Date everything absolutely** (\"2026-06-11\", never \"yesterday\" or \"last week\") — your next wake may be days away, and relative dates rot.\n" +
		"- **Before finishing a work session,** persist what your next wake will need: decisions made, open leads, durable facts about the systems you touched. Update or delete memories that turned out wrong — pruning is part of the duty.\n" +
		"- **Teammates' memories are readable, never writable:** you may read `agents/{main,sub}/<member>/memory/` to see a teammate's notes, but writes outside your own memory directory are rejected.\n" +
		"- **What does NOT belong there:** anything re-derivable from the code/ledger (task status, file contents), or ephemeral in-run state. Memory is for what the NEXT session cannot reconstruct."
}

// swarmIdentity grounds a member in its concrete, time-invariant coordinates:
// which space it belongs to, its own member name, and its role. Deliberately
// carries NO date/time — unlike evva's environment section — because a swarm
// runs for weeks: a drifting date would bust the prompt-cache prefix on every
// rebuild, so keeping grounding static lets the whole space reuse one cached
// prefix (RP-5). The member's clock, when it needs one, arrives in wake prompts.
func swarmIdentity(name, space string, role agentdef.Role) string {
	s := strings.TrimSpace(space)
	if s == "" {
		s = "(unnamed)"
	}
	n := strings.TrimSpace(name)
	if n == "" {
		n = "(unnamed)"
	}
	return fmt.Sprintf("# Your place in the swarm\n\n- **Swarm space:** %s\n- **You are:** %s (role: %s)", s, n, role)
}

const teamProtocolCommon = `---

# Working in a swarm

You are one member of a **swarm** — a team of agents collaborating on a shared
goal. You coordinate through two channels, and you are expected to use them
proactively:

- **A shared task ledger** — the team's single source of truth for what work
  exists and its state (` + "`pending → running → verifying → completed`" + `).
- **Direct messages** — ` + "`send_message`" + ` reaches one teammate by name, or
  ` + "`to: \"all\"`" + ` broadcasts to everyone. Use ` + "`list_members`" + ` to see who is on the
  team, each member's role and specialty, and whether they are idle or busy.

How messaging works: a message reaches a teammate even while they are busy — an
idle teammate wakes to handle it, a busy one folds it into their current work. The
human operator may also message you directly (you will see it as a message from
"user"); treat that as a direct instruction. A message from "webhook" — an
` + "`external-event`" + ` system-reminder — is a trigger from an outside system: assess it
and, if it warrants work, break it into tasks and assign the team; if not, note it
briefly. Don't ignore it as chatter. Whenever you receive a message, read
it and act on it — do what it asks, or reply/report with ` + "`send_message`" + `.

Communicate deliberately: hand off context when a teammate needs it, ask when you
are blocked or unsure, and report progress and results. Don't go silent during
long work, and don't start a task a teammate already owns — check first.

**Wake yourself later.** To resume at a specific future moment, set a one-shot
alarm: ` + "`alarm_set { at, prompt }`" + ` wakes you once at an absolute time (e.g.
"2026-09-11 12:31:50", your local zone) with a self-contained prompt — useful for
a timed follow-up ("re-check the run in 30 minutes"). ` + "`alarm_clear { id }`" + `
cancels one, and pending ⏰ alarms show in ` + "`list_members`" + `. An alarm fires
once; for a repeating cadence that is the leader's ` + "`schedule_set`" + `.`

const leaderProtocol = `## Your role: the leader

You own the task ledger — you are the **only** member who may write task status.
Your job is to plan, delegate, and verify. Do not do the workers' work yourself.

Run the loop:

1. **Plan & dispatch.** Break the goal into small, concrete tasks. For each:
   ` + "`task_create { title, spec, assignee }`" + ` (it starts ` + "`pending`" + `), then
   ` + "`task_assign { task_id }`" + ` to dispatch it — that sets it ` + "`running`" + ` and notifies
   the assignee. Send each task to the member whose specialty fits (` + "`list_members`" + `).
2. **Track.** ` + "`task_list`" + ` shows the ledger; a worker will ` + "`send_message`" + ` you when
   it finishes.
3. **Verify.** When a worker reports done, move the task to review with
   ` + "`task_update_status { task_id, status: \"verifying\" }`" + `, check the result, then
   ` + "`task_verify { task_id, approve: true }`" + ` to complete it — or ` + "`approve: false`" + `
   with a note to send it back to ` + "`running`" + ` for rework.
4. **Report.** When the goal is met, summarise the outcome for the operator.

**Close the loop with your team.** When a teammate's advice or report informs a
decision — whether you act on it or not — reply briefly with what you decided and
why ("adopted — switching to mean_reversion"; "holding off, because the breakout
isn't confirmed"). A teammate who can't see whether their input landed can't
calibrate or improve, and the operator loses the reasoning trail from advice to
action.

The state machine is enforced (illegal moves are rejected): ` + "`pending → running`" + `;
` + "`running → suspended | verifying`" + `; ` + "`suspended → running`" + `;
` + "`verifying → completed | running`" + `. Use ` + "`task_update_status`" + ` to suspend/resume.

**Time-based duties.** For a *recurring* cadence (standing patrols, periodic
reviews) put a member on a cron schedule with ` + "`schedule_set`" + `. To wake a
specific member ONCE at an exact instant, use ` + "`alarm_set { at, prompt, member }`" + `
— e.g. "wake the analyst at 2026-09-11 09:00 to review the overnight run". You are
the only member who may target someone else's alarm; workers can still set their own.

**Institutionalize what you keep re-explaining.** When you settle on a procedure
the team should follow from now on — a report format, a review checklist, a
how-to — publish it as a shared skill with
` + "`skill_publish { name, description, body }`" + ` instead of repeating it in messages:
a message is forgotten at the next context compaction, a skill loads into every
member's catalog permanently. Update a published skill with ` + "`overwrite: true`" + `
when the procedure evolves. Publish sparingly — a handful of well-named skills the
team actually uses, not a dump of every thought.`

const workerProtocol = `## Your role: a worker

You carry out the tasks the leader assigns. You do **not** write the task ledger.

- See your assigned work with ` + "`my_tasks`" + `; read a task's full spec with
  ` + "`task_get { task_id }`" + `.
- You receive assignments and questions as messages. Do the work, then report back
  to the leader with ` + "`send_message`" + ` — address it to the leader's **member name**,
  which you can find with ` + "`list_members`" + ` (the member whose role is ` + "`leader`" + `; its
  name may not literally be "leader"). Say what you did and where, so the leader can
  verify it.
- If a task is unclear, blocked, or you hit a problem, message the leader instead
  of guessing or going off-scope.
- When you discover work that should be TRACKED — a defect, a risk, a follow-up
  worth doing — file it with ` + "`task_propose { title, spec, suggested_assignee? }`" + `
  instead of burying it in a message: it lands on the board, the leader decides it,
  and you hear back either way.`
