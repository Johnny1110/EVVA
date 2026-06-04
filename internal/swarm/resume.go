package swarm

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"

	"github.com/johnny1110/evva/pkg/agent"
)

// resume.go makes a space survive process death (SPRD-1-11). The durable state
// is already split across three stores: the task ledger + messages live in
// vero.db (1-2), and each agent's transcript lives in the SDK session store
// (<AppHome>/sessions/<workdir-slug>/, keyed by persona == member name). The
// only thing neither holds is membership (active vs frozen), so runtime.json
// carries just that. Reload stitches the three back together on a rebuild.

// runtimeState is the per-space membership snapshot persisted to
// <workdir>/.vero/runtime.json so a frozen member comes back frozen, not active.
type runtimeState struct {
	Membership map[string]string `json:"membership"` // name -> "active" | "frozen"
}

func runtimePath(workdir string) string {
	return filepath.Join(workdir, ".vero", "runtime.json")
}

// persistRuntime writes the current roster membership to runtime.json. Called
// whenever membership changes (freeze/unfreeze/add); a best-effort write — a
// failure only costs the frozen-on-restart guarantee, never correctness.
func (sp *SwarmSpace) persistRuntime() {
	rs := runtimeState{Membership: map[string]string{}}
	for _, mv := range sp.Roster.Snapshot() {
		rs.Membership[mv.Name] = string(mv.Membership)
	}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(runtimePath(sp.Workdir), data, 0o644)
}

// loadRuntime reads runtime.json; a missing/corrupt file yields an empty state
// (every member active — the first-boot default).
func loadRuntime(workdir string) runtimeState {
	rs := runtimeState{Membership: map[string]string{}}
	b, err := os.ReadFile(runtimePath(workdir))
	if err != nil {
		return rs
	}
	if err := json.Unmarshal(b, &rs); err != nil || rs.Membership == nil {
		return runtimeState{Membership: map[string]string{}}
	}
	return rs
}

// Reload restores a rebuilt space to where it died:
//
//  1. resume each member's most recent real transcript (its persona session),
//  2. re-queue each member's durable unread mail onto its mailbox, and
//  3. reapply persisted frozen membership.
//
// Call it AFTER NewSpace (agents + mailboxes exist) and BEFORE the Supervisor
// starts the run loops — §6.2's ordering: requeue after the inbox exists, before
// the loop drains it, so no wake is lost. Tasks need no handling: they live in
// vero.db, so a rebuilt space already sees the same ledger (a row left `running`
// is still `running`). Reload is idempotent.
func (sp *SwarmSpace) Reload() {
	sp.mu.Lock()
	members := make(map[string]agent.Agent, len(sp.agents))
	maps.Copy(members, sp.agents)
	sp.mu.Unlock()

	rt := loadRuntime(sp.Workdir)

	for name, ag := range members {
		if id := latestSessionFor(ag, name); id != "" {
			_ = ag.ResumeSession(id)
		}
		if ids, err := sp.Store.UnreadFor(name); err == nil && len(ids) > 0 {
			sp.Bus.Requeue(name, ids)
		}
		if rt.Membership[name] == string(MembershipFrozen) {
			sp.Roster.setMembership(name, MembershipFrozen)
		}
	}
}

// latestSessionFor returns the id of the most recent persisted session that
// belongs to this member (Profile == name) and carries a real transcript
// (MessageCount > 0). ListSessions is mtime-descending, so the first match is
// the newest; "" means the member has no prior transcript (first boot, or it
// never ran). Filtering on a non-empty transcript skips the empty snapshot a
// freshly-constructed agent may have just written.
func latestSessionFor(ag agent.Agent, name string) string {
	rows, _ := ag.ListSessions()
	for _, r := range rows {
		if r.Profile == name && r.MessageCount > 0 {
			return r.ID
		}
	}
	return ""
}
