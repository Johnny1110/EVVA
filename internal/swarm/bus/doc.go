// Package bus is the per-space message bus and mailboxes. Each agent has one
// mailbox channel that carries only message UUIDs (never payloads); the
// SQLite `messages` table is the durable source of truth. Delivery writes the
// message to the store first, then pushes the UUID onto the recipient's
// channel (ordering guarantee). "to: all" broadcasts to every active member.
//
// Carrying only UUIDs is what makes restart-resume trivial: on boot the
// Supervisor reloads unread UUIDs (store.UnreadFor) back onto the channels and
// nothing in flight is lost.
//
// TODO(SPRD-1-5): implement Bus.Send(to, msgUUID), Bus.Inbox(name)
// <-chan string, and broadcast, over the store's message DAO.
package bus
