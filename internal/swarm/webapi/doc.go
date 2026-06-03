// Package webapi serves the swarm workstation API: REST snapshots
// (/api/swarms, /api/swarm/:id, /api/tasks, /api/agents/:name/transcript,
// /api/messages) and a WebSocket bridge that streams each agent's
// pkg/event.Event out to the browser, fanned out by (spaceID, AgentID).
// Inbound, the browser drives each agent's pkg/agent Controller (Run,
// RespondPermission, RespondQuestion) and the Supervisor (suspend, add,
// freeze, halt).
//
// It implements pkg/event.Sink per agent and composes with event.Multi to
// also tee events to the log. The pkg/event doc already anticipates "a
// JSON-over-websocket bridge" — this is it.
//
// TODO(SPRD-1-8): implement the HTTP/WS handlers, the per-(space,agent) sink
// fan-out, and the REST snapshots.
package webapi
