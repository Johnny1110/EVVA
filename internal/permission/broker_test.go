package permission

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBroker_RequestRespond(t *testing.T) {
	b := NewBroker()
	gotCh := make(chan ApprovalRequest, 1)
	SetOnRequest(b, func(r ApprovalRequest) {
		gotCh <- r
	})

	done := make(chan Decision, 1)
	go func() {
		d, _ := b.Request(context.Background(), ApprovalRequest{ToolName: "bash"})
		done <- d
	}()

	var got ApprovalRequest
	select {
	case got = <-gotCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broker callback")
	}

	if err := b.Respond(got.ID, Decision{Behavior: BehaviorAllow, Reason: "ok"}); err != nil {
		t.Fatalf("respond: %v", err)
	}

	select {
	case d := <-done:
		if d.Behavior != BehaviorAllow {
			t.Errorf("got %v want allow", d.Behavior)
		}
	case <-time.After(time.Second):
		t.Fatal("Request did not return after Respond")
	}
}

func TestBroker_ContextCancellation(t *testing.T) {
	b := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan Decision, 1)
	go func() {
		d, _ := b.Request(ctx, ApprovalRequest{ToolName: "bash"})
		done <- d
	}()

	time.Sleep(10 * time.Millisecond) // let the goroutine reach select
	cancel()

	select {
	case d := <-done:
		if d.Behavior != BehaviorDeny {
			t.Errorf("expected deny on cancel; got %v", d.Behavior)
		}
	case <-time.After(time.Second):
		t.Fatal("Request did not return after cancel")
	}
}

func TestBroker_ConcurrentRequestsAreIndependent(t *testing.T) {
	b := NewBroker()
	var ids []string
	var mu sync.Mutex
	SetOnRequest(b, func(r ApprovalRequest) {
		mu.Lock()
		ids = append(ids, r.ID)
		mu.Unlock()
	})

	const n = 5
	results := make(chan Decision, n)
	for i := 0; i < n; i++ {
		go func() {
			d, _ := b.Request(context.Background(), ApprovalRequest{ToolName: "bash"})
			results <- d
		}()
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(ids)
		mu.Unlock()
		if count == n {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	if len(ids) != n {
		mu.Unlock()
		t.Fatalf("expected %d pending requests; got %d", n, len(ids))
	}
	// Respond in reverse order; every goroutine wakes correctly.
	for i := len(ids) - 1; i >= 0; i-- {
		if err := b.Respond(ids[i], Decision{Behavior: BehaviorAllow}); err != nil {
			mu.Unlock()
			t.Fatalf("respond %s: %v", ids[i], err)
		}
	}
	mu.Unlock()

	for i := 0; i < n; i++ {
		select {
		case <-results:
		case <-time.After(time.Second):
			t.Fatal("a Request did not return after its Respond")
		}
	}
}

func TestBroker_RespondUnknownID(t *testing.T) {
	b := NewBroker()
	if err := b.Respond("bogus", Decision{}); err == nil {
		t.Error("expected error responding to unknown id")
	}
}
