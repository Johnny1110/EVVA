package swarm

// Supervisor owns one space's lifecycle: AddMember (hot-load), Freeze/Unfreeze
// (cold-storage, never delete), Suspend/Resume (cancel/restart a run context),
// HaltAll (the Phase 2 friday kill switch), and restart-reload (re-queue
// unread messages + ResumeSession on boot). It holds the per-agent run
// contexts so a suspend is a deterministic ctx-cancel.
//
// TODO(SPRD-1-6): implement Supervisor lifecycle + the per-run recover()
// guard. TODO(SPRD-1-11): boot reconcile (unread reload + ResumeSession).
