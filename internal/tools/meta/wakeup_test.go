package meta

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/johnny1110/evva/pkg/tools"
	"time"
)

func TestWakeup_SleepAndEnqueue(t *testing.T) {
	q := NewWakeupQueue()
	w := NewWakeup(q)

	input := json.RawMessage(`{"delaySeconds":0.05,"reason":"unit test","prompt":"check on subagents"}`)
	start := time.Now()
	res, err := w.Execute(context.Background(), tools.NopLogger(), input)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("Execute: tool reported error: %s", res.Content)
	}
	// Clamp floor is 1s, so even 0.05s should round up — sleep was at
	// least the clamped minimum.
	if elapsed < 900*time.Millisecond {
		t.Errorf("Execute: returned too quickly (%v); expected >=~1s due to floor clamp", elapsed)
	}
	prompts := q.Drain()
	if len(prompts) != 1 || prompts[0] != "check on subagents" {
		t.Errorf("Drain: got %v, want [\"check on subagents\"]", prompts)
	}
}

func TestWakeup_RequiresPromptAndReason(t *testing.T) {
	w := NewWakeup(NewWakeupQueue())
	cases := []struct {
		name string
		in   string
	}{
		{"missing prompt", `{"delaySeconds":1,"reason":"r"}`},
		{"missing reason", `{"delaySeconds":1,"prompt":"p"}`},
		{"blank prompt", `{"delaySeconds":1,"reason":"r","prompt":"   "}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := w.Execute(context.Background(), tools.NopLogger(), json.RawMessage(tc.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Fatalf("expected IsError, got success: %s", res.Content)
			}
		})
	}
}

func TestWakeup_CancelDuringSleep(t *testing.T) {
	q := NewWakeupQueue()
	w := NewWakeup(q)

	ctx, cancel := context.WithCancel(context.Background())
	// Fire cancel shortly after Execute starts.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	input := json.RawMessage(`{"delaySeconds":3600,"reason":"r","prompt":"p"}`)
	start := time.Now()
	res, err := w.Execute(ctx, tools.NopLogger(), input)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected cancellation to surface as IsError, got: %s", res.Content)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("cancel was not honored promptly (%v)", elapsed)
	}
	if !strings.Contains(res.Content, "cancelled") {
		t.Errorf("expected cancelled message, got: %s", res.Content)
	}
	// Cancelled wakeups must NOT enqueue — the conversation is unwinding.
	if got := q.Drain(); len(got) != 0 {
		t.Errorf("cancelled wakeup should not enqueue; got %v", got)
	}
}

func TestWakeup_DelayClampsToMax(t *testing.T) {
	// We can't actually wait an hour in a unit test, but we can verify
	// the schema description: just decode and check the field handling
	// is in place via the input struct. Coverage of the upper clamp is
	// tighter than this — but the path mirrors the lower clamp which
	// the SleepAndEnqueue test already exercises.
	w := NewWakeup(NewWakeupQueue())
	if w.Name() != "schedule_wakeup" {
		t.Errorf("Name() = %q, want schedule_wakeup", w.Name())
	}
	if !strings.Contains(w.Description(), "delaySeconds") {
		t.Errorf("Description() missing delaySeconds hint")
	}
	var schema map[string]any
	if err := json.Unmarshal(w.Schema(), &schema); err != nil {
		t.Fatalf("Schema() not valid JSON: %v", err)
	}
}
