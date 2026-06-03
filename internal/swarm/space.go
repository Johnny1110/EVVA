package swarm

// SwarmSpace is the live, isolated unit of a swarm: one workdir, one
// .vero/vero.db, one bus, one Roster, and the constructed leader + worker
// agent handles. Two spaces in the same process share nothing — separate
// stores, separate buses, separate rosters — and agent names are scoped per
// space (two spaces may both have a "leader"). Tasks and messages never cross
// a space boundary.
//
// TODO(SPRD-1-4): assemble a SwarmSpace from []agentdef.Loaded via agent.New,
// build the per-space Roster, and wire each agent's event.Sink so events are
// stamped (spaceID, AgentID) before fan-out. Every Controller.Run must run
// inside a recover()-guarded goroutine (invariant #3) so one agent panic
// degrades that run only, never the process.
