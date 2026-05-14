package tools

import (
	"context"
	"encoding/json"
)

// Tool is the contract every tool must satisfy.
// Stateless tools are typically package-level singletons (shell.Bash).
// Stateful tools receive backing state via constructor (fs.NewRead, task.NewCreate).
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

// Result is what every tool returns to the agent.
//
// Metadata is an optional, tool-specific structured payload that flows
// through to the event sink (carried on ToolUseResultPayload.Metadata) so
// UIs can render richer detail than the human-readable Content string
// allows. Stays opaque to the agent layer — the UI type-asserts on it.
// Common payloads today:
//   - *fs.FileDiff for write_file / edit_file mutations
//
// LLM-facing tool results carry only Content + IsError; Metadata never
// goes to the model.
type Result struct {
	Content  string
	IsError  bool
	Metadata any
}

// Call is what the LLM emits when it wants to invoke a tool.
type Call struct {
	ID    string
	Name  string
	Input json.RawMessage
}
