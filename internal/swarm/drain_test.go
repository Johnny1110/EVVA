package swarm

import (
	"context"
	"strings"
	"testing"

	"github.com/johnny1110/evva/internal/swarm/store"
)

// TestInboxDrainerReadsAndMarks: the swarm drainer pulls a delivered message off
// the mailbox, formats it, marks it read, and is empty on the next poll.
func TestInboxDrainerReadsAndMarks(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("s", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	defer sp.Shutdown()

	d := newInboxDrainer("worker-a", sp.Bus, sp.Store)

	// Empty inbox → non-blocking no-op.
	if _, ok := d.Drain(context.Background()); ok {
		t.Fatal("drain of an empty mailbox should return ok=false")
	}

	uuid, err := sp.Bus.Send(store.Message{Sender: "leader", Recipient: "worker-a", Subject: "halt", Body: "stop now"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	msg, ok := d.Drain(context.Background())
	if !ok {
		t.Fatal("drain should have returned the delivered message")
	}
	if !strings.Contains(msg, "leader") || !strings.Contains(msg, "stop now") || !strings.Contains(msg, "halt") {
		t.Errorf("formatted message = %q, want sender/subject/body", msg)
	}

	got, err := sp.Store.GetMessage(uuid)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if got.ReadAt == nil {
		t.Error("drained message was not marked read")
	}

	// Nothing left to drain.
	if _, ok := d.Drain(context.Background()); ok {
		t.Error("second drain should be empty")
	}
}

// TestInboxDrainerSkipsAlreadyRead: a hint for a message already consumed by
// drain A (marked read) is skipped, so no message is folded twice.
func TestInboxDrainerSkipsAlreadyRead(t *testing.T) {
	cfg := stubConfig(t)
	sp, err := NewSpace("s", testManifest(), testLoaded(), nil, cfg)
	if err != nil {
		t.Fatalf("NewSpace: %v", err)
	}
	defer sp.Shutdown()

	uuid, err := sp.Bus.Send(store.Message{Sender: "leader", Recipient: "worker-a", Body: "already handled"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := sp.Store.MarkRead(uuid); err != nil { // drain A got here first
		t.Fatalf("mark read: %v", err)
	}

	d := newInboxDrainer("worker-a", sp.Bus, sp.Store)
	if _, ok := d.Drain(context.Background()); ok {
		t.Error("drainer should skip a message that is already read")
	}
}
