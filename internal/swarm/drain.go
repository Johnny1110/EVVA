package swarm

import (
	"context"
	"fmt"

	"github.com/johnny1110/evva/internal/swarm/bus"
	"github.com/johnny1110/evva/internal/swarm/store"
)

// inboxDrainer is the swarm's consumer of the public pkg/agent inbox-drainer
// seam (SPRD-1-12, drain B). Wired onto every member at construction, it lets a
// BUSY agent fold an incoming message into its current run instead of waiting
// for the run to end (drain A, which the supervisor does between runs).
//
// It is non-blocking by contract: one select-with-default per iteration
// boundary, so an agent with an empty mailbox pays ~nothing. The mailbox carries
// only a UUID hint — the message body comes from the durable store, the single
// source of truth — and a hint whose row is already read (handled by drain A or
// a stale duplicate hint) is skipped, so no message is ever folded twice.
type inboxDrainer struct {
	name  string
	bus   *bus.Bus
	store *store.Store
}

func newInboxDrainer(name string, b *bus.Bus, st *store.Store) *inboxDrainer {
	return &inboxDrainer{name: name, bus: b, store: st}
}

// Drain implements agent.Drainer: a non-blocking peek at the member's mailbox.
func (d *inboxDrainer) Drain(_ context.Context) (string, bool) {
	inbox := d.bus.Inbox(d.name)
	if inbox == nil {
		return "", false
	}
	select {
	case uuid := <-inbox:
		msg, err := d.store.GetMessage(uuid)
		if err != nil {
			return "", false
		}
		if msg.ReadAt != nil {
			return "", false // already handled (drain A, or a stale duplicate hint)
		}
		_ = d.store.MarkRead(uuid)
		return formatIncoming(msg), true
	default:
		return "", false
	}
}

// formatIncoming renders one message as the synthetic user turn the model sees
// mid-run — labelled so the model knows it is an out-of-band teammate message,
// not the user.
func formatIncoming(m store.Message) string {
	if m.Subject != "" {
		return fmt.Sprintf("[Incoming message from %s — re: %s]\n%s", m.Sender, m.Subject, m.Body)
	}
	return fmt.Sprintf("[Incoming message from %s]\n%s", m.Sender, m.Body)
}
