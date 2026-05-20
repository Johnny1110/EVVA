package todo

import (
	"sync"

	"github.com/johnny1110/evva/internal/observable"
)

// TodoStore is the per-agent backing store for the todo_write tool. The
// agent constructs one via toolset.ToolState.TodoStore() and threads it
// into the tool's constructor.
//
// Mutations are guarded by mu; Notify fires after the lock is released so
// observers may freely call back into the store. Safe for concurrent
// access.
type TodoStore struct {
	observable.Observable

	mu    sync.Mutex
	todos []Todo
}

// NewTodoStore returns an empty store ready for use.
func NewTodoStore() *TodoStore {
	return &TodoStore{}
}

// Domain identifies this store on the change stream. Required by the
// observable.Store interface.
func (s *TodoStore) Domain() string { return Domain }

// List returns a snapshot of the current todos in their stored order.
func (s *TodoStore) List() []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Todo, len(s.todos))
	copy(out, s.todos)
	return out
}

// Replace overwrites the list wholesale with the given todos. Emits one
// "replaced" change carrying the new list as Payload so observers can
// re-render from a single event.
func (s *TodoStore) Replace(in []Todo) {
	s.mu.Lock()
	next := make([]Todo, len(in))
	copy(next, in)
	s.todos = next
	snapshot := make([]Todo, len(next))
	copy(snapshot, next)
	s.mu.Unlock()

	s.Notify(observable.Change{
		Domain:  Domain,
		Op:      "replaced",
		Payload: snapshot,
	})
}

// Clear empties the list. Emits one "replaced" change with an empty
// payload so the TUI's auto-fold path collapses the panel without any
// special-case handling.
func (s *TodoStore) Clear() {
	s.mu.Lock()
	had := len(s.todos) > 0
	s.todos = nil
	s.mu.Unlock()
	if !had {
		return
	}
	s.Notify(observable.Change{
		Domain:  Domain,
		Op:      "replaced",
		Payload: []Todo{},
	})
}
