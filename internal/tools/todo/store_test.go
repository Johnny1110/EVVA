package todo

import (
	"sync"
	"testing"

	"github.com/johnny1110/evva/internal/observable"
)

func TestTodoStore_ReplaceEmitsOneChange(t *testing.T) {
	s := NewTodoStore()
	var got []observable.Change
	var mu sync.Mutex
	s.Subscribe(func(c observable.Change) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, c)
	})

	s.Replace([]Todo{
		{Content: "a", ActiveForm: "A", Status: StatusInProgress},
		{Content: "b", ActiveForm: "B", Status: StatusPending},
	})

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].Domain != Domain || got[0].Op != "replaced" {
		t.Errorf("unexpected change: %+v", got[0])
	}
	payload, ok := got[0].Payload.([]Todo)
	if !ok || len(payload) != 2 {
		t.Fatalf("payload shape mismatch: %#v", got[0].Payload)
	}
	if payload[0].Content != "a" || payload[1].Status != StatusPending {
		t.Errorf("payload content drift: %#v", payload)
	}
}

func TestTodoStore_ListReturnsCopy(t *testing.T) {
	s := NewTodoStore()
	s.Replace([]Todo{{Content: "x", ActiveForm: "X", Status: StatusPending}})
	out := s.List()
	out[0].Content = "mutated"
	again := s.List()
	if again[0].Content != "x" {
		t.Errorf("List did not return a defensive copy: %#v", again)
	}
}

func TestTodoStore_ClearEmitsWhenNonEmpty(t *testing.T) {
	s := NewTodoStore()
	s.Replace([]Todo{{Content: "x", ActiveForm: "X", Status: StatusCompleted}})

	var ops []string
	s.Subscribe(func(c observable.Change) { ops = append(ops, c.Op) })
	s.Clear()

	if len(s.List()) != 0 {
		t.Errorf("Clear left items in store: %#v", s.List())
	}
	if len(ops) != 1 || ops[0] != "replaced" {
		t.Errorf("expected one 'replaced' op on Clear, got %v", ops)
	}
}

func TestTodoStore_ClearNoopOnEmpty(t *testing.T) {
	s := NewTodoStore()
	var calls int
	s.Subscribe(func(c observable.Change) { calls++ })
	s.Clear()
	if calls != 0 {
		t.Errorf("Clear on empty store should not Notify, got %d calls", calls)
	}
}

func TestStatus_IsValid(t *testing.T) {
	for _, s := range []Status{StatusPending, StatusInProgress, StatusCompleted} {
		if !s.IsValid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []Status{"", "deleted", "wat"} {
		if Status(s).IsValid() {
			t.Errorf("%q should be invalid", s)
		}
	}
}
