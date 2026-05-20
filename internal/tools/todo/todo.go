// Package todo exposes the single todo_write tool. It owns the in-session
// todo list — a flat list of {content, activeForm, status} entries that the
// agent rewrites in one shot every time the plan changes.
//
// Replaces the previous six-tool task package. Background-process tools
// (Monitor, the future task_output / task_stop pair) live elsewhere; this
// package is purely about ephemeral planning.
package todo

import "github.com/johnny1110/evva/internal/tools"

// Domain is the observable.Change.Domain value every todo-store change
// carries. Consumers switch on this string and type-assert Payload to
// []todo.Todo.
const Domain = "todo"

// Status enumerates the lifecycle states a todo can be in. There is no
// "deleted" state — full-list-replacement makes removal implicit (omit the
// entry from the next write).
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// IsValid reports whether s is one of the canonical statuses.
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted:
		return true
	}
	return false
}

// Todo is one entry in the session todo list. content is the imperative
// form ("Run tests"); activeForm is the present-continuous variant
// ("Running tests") rendered in the spinner while the entry is in_progress.
type Todo struct {
	Content    string
	ActiveForm string
	Status     Status
}

// Names lists every tool name this package contributes. Single-element so
// callers can append it uniformly alongside the other package Names()
// helpers in agent profile composition.
func Names() []tools.ToolName {
	return []tools.ToolName{tools.TODO_WRITE}
}
