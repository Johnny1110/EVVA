package swarm

// Scheduler turns the three wake sources — message (mailbox has mail), task
// (assigned by the Leader), and timer (a profile.yml `schedule` fires) — into
// Controller.Run calls. When any source triggers and the target agent is idle,
// it composes a synthetic prompt and runs the agent; idle agents burn no
// tokens. Timer wakes are a Supervisor-layer concern (driven via the
// Controller), never the agent sleeping inside its own run.
//
// TODO(SPRD-1-6): implement the wake loop (message/task/timer), per-agent
// state locking against suspend races, and drain stage A (run-boundary mailbox
// injection). Drain stage B (mid-run) lands in SPRD-1-12.
