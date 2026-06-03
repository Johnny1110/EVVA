package swarm

// Roster is the per-space, thread-safe directory of members:
// name -> { Controller, role, membership(active|frozen), runStatus(idle|busy|
// suspended), currentTask, whenToUse }. It is the single source of truth that
// feeds both the list_members tool and the web API (/api/swarm/:id) — one
// store, two read surfaces.
//
// TODO(SPRD-1-4): implement Roster with a Snapshot() accessor for
// list_members + webapi, per-space name scoping (collisions error at
// construction), and the two orthogonal status dimensions (membership vs run).
