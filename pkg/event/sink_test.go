package event

import "testing"

// Phase 1 analysis — sink surface:
//   - Multi.Emit fans out to every Sinks entry in declared order; nil
//     entries are silently skipped (NOT a panic).
//   - Discard.Emit is a no-op — never panics, never blocks.
//   - BubbleUp.Emit rewrites the event's ParentID to its own ParentID
//     field and forwards to Parent. nil Parent is silently dropped (no
//     panic). AgentID is preserved untouched.
//   - SinkFunc.Emit forwards to the underlying function; a nil
//     SinkFunc is silently dropped.

// recordingSink captures every event it receives, in order.
type recordingSink struct {
	got []Event
}

func (r *recordingSink) Emit(e Event) {
	r.got = append(r.got, e)
}

func TestMulti_FansOutToAllSinksInOrder(t *testing.T) {
	a := &recordingSink{}
	b := &recordingSink{}
	c := &recordingSink{}
	multi := Multi{Sinks: []Sink{a, b, c}}

	multi.Emit(Event{Kind: KindRunStart, AgentID: "agent-1"})

	for name, sink := range map[string]*recordingSink{"a": a, "b": b, "c": c} {
		if len(sink.got) != 1 {
			t.Errorf("%s: got %d events, want 1", name, len(sink.got))
			continue
		}
		if sink.got[0].Kind != KindRunStart || sink.got[0].AgentID != "agent-1" {
			t.Errorf("%s: got %+v", name, sink.got[0])
		}
	}
}

func TestMulti_SkipsNilSinks(t *testing.T) {
	// Must not panic on a nil entry; non-nil entries still receive.
	a := &recordingSink{}
	c := &recordingSink{}
	multi := Multi{Sinks: []Sink{nil, a, nil, c, nil}}

	multi.Emit(Event{Kind: KindText})

	if len(a.got) != 1 || len(c.got) != 1 {
		t.Errorf("non-nil sinks did not receive event; a=%d c=%d", len(a.got), len(c.got))
	}
}

func TestMulti_EmptyListIsNoOp(t *testing.T) {
	multi := Multi{}
	// Must not panic.
	multi.Emit(Event{Kind: KindRunEnd})

	multi2 := Multi{Sinks: nil}
	multi2.Emit(Event{Kind: KindRunEnd})
}

func TestDiscard_NeverPanics(t *testing.T) {
	// Smoke: every Kind discarded silently.
	for _, k := range []Kind{KindRunStart, KindText, KindError, KindStoreUpdate} {
		Discard.Emit(Event{Kind: k})
	}
}

func TestBubbleUp_RewritesParentIDAndForwards(t *testing.T) {
	parent := &recordingSink{}
	bubble := BubbleUp{Parent: parent, ParentID: "parent-007"}

	bubble.Emit(Event{
		Kind:     KindRunStart,
		AgentID:  "child-042",
		ParentID: "stale-or-empty",
	})

	if len(parent.got) != 1 {
		t.Fatalf("parent sink received %d events, want 1", len(parent.got))
	}
	got := parent.got[0]
	if got.ParentID != "parent-007" {
		t.Errorf("ParentID rewritten incorrectly: got %q, want %q", got.ParentID, "parent-007")
	}
	if got.AgentID != "child-042" {
		t.Errorf("AgentID must NOT be rewritten by BubbleUp: got %q, want %q", got.AgentID, "child-042")
	}
	if got.Kind != KindRunStart {
		t.Errorf("Kind drifted: got %q, want %q", got.Kind, KindRunStart)
	}
}

func TestBubbleUp_NilParentIsSilent(t *testing.T) {
	// BubbleUp with a nil Parent must not panic — it's a clean drop.
	bubble := BubbleUp{Parent: nil, ParentID: "p"}
	bubble.Emit(Event{Kind: KindRunStart, AgentID: "x"})
}

func TestBubbleUp_PreservesPayload(t *testing.T) {
	// Payload fields should pass through; BubbleUp only touches ParentID.
	parent := &recordingSink{}
	bubble := BubbleUp{Parent: parent, ParentID: "p"}

	bubble.Emit(Event{
		Kind:    KindText,
		AgentID: "child",
		Text:    &TextPayload{Text: "hello"},
	})

	got := parent.got[0]
	if got.Text == nil || got.Text.Text != "hello" {
		t.Errorf("Text payload lost: got %+v", got.Text)
	}
}

func TestBubbleUp_OverridesPreExistingParentID(t *testing.T) {
	// If the event already carries a ParentID (shouldn't, but defensive),
	// BubbleUp's value wins.
	parent := &recordingSink{}
	bubble := BubbleUp{Parent: parent, ParentID: "real-parent"}

	bubble.Emit(Event{Kind: KindText, AgentID: "x", ParentID: "stale"})

	if got := parent.got[0].ParentID; got != "real-parent" {
		t.Errorf("stale ParentID not overridden: got %q, want %q", got, "real-parent")
	}
}

func TestSinkFunc_ForwardsCallVerbatim(t *testing.T) {
	var got []Event
	sink := SinkFunc(func(e Event) { got = append(got, e) })

	sink.Emit(Event{Kind: KindRunStart, AgentID: "x"})
	sink.Emit(Event{Kind: KindRunEnd, AgentID: "x"})

	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].Kind != KindRunStart || got[1].Kind != KindRunEnd {
		t.Errorf("kinds drifted: got %+v", got)
	}
}

func TestSinkFunc_NilFunctionIsSilent(t *testing.T) {
	// A nil SinkFunc must be safe to call (zero-value sink semantics).
	var sink SinkFunc
	sink.Emit(Event{Kind: KindText})
}

// TestMulti_AndBubbleUpComposeCleanly is an integration-shaped smoke
// test: a subagent's BubbleUp wraps a Multi that contains both a TUI
// sink and a logger sink. The event should reach both with ParentID
// rewritten exactly once.
func TestMulti_AndBubbleUpComposeCleanly(t *testing.T) {
	tui := &recordingSink{}
	logger := &recordingSink{}
	parent := Multi{Sinks: []Sink{tui, logger}}
	bubble := BubbleUp{Parent: parent, ParentID: "root"}

	bubble.Emit(Event{Kind: KindText, AgentID: "sub"})

	for name, sink := range map[string]*recordingSink{"tui": tui, "logger": logger} {
		if len(sink.got) != 1 {
			t.Errorf("%s len: got %d, want 1", name, len(sink.got))
			continue
		}
		if sink.got[0].ParentID != "root" {
			t.Errorf("%s ParentID: got %q, want %q", name, sink.got[0].ParentID, "root")
		}
		if sink.got[0].AgentID != "sub" {
			t.Errorf("%s AgentID: got %q, want %q", name, sink.got[0].AgentID, "sub")
		}
	}
}
