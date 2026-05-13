// Package event defines the event stream the agent emits while running.
//
// The Event envelope is a discriminated union — every event has a Kind and
// exactly one non-nil typed payload field. This keeps consumer code
// type-safe (no interface{} assertions, no reflection) while still allowing
// one Sink to receive every kind of event the agent might emit.
//
// Sinks (see sink.go) are the consumer side. A TUI, a structured logger,
// and a JSON-over-websocket bridge can each implement Sink and subscribe
// independently of one another — the agent doesn't know about them.
package event

import (
	"encoding/json"
	"time"

	"github.com/johnny1110/evva/internal/llm"
)

// Kind tags every event. New kinds are added by extending this list and the
// matching payload field on Event.
type Kind string

const (
	KindRunStart     Kind = "run_start"
	KindRunResume    Kind = "run_resume"
	KindRunEnd       Kind = "run_end"
	KindRunCancelled Kind = "run_cancelled"
	KindIterLimit    Kind = "iter_limit" // paused — caller may Continue

	KindTurnStart Kind = "turn_start"
	KindTurnEnd   Kind = "turn_end"

	KindThinking Kind = "thinking" // assistant reasoning text
	KindText     Kind = "text"     // assistant final text

	KindToolUseStart  Kind = "tool_use_start"
	KindToolUseResult Kind = "tool_use_result"

	KindError Kind = "error"

	KindTaskUpdate Kind = "task_update" // task panel state change
	KindSubagent   Kind = "subagent"    // subagent lifecycle marker
)

// SubagentPhase distinguishes subagent-started from subagent-ended events.
// The events between them (carrying the same ParentID) belong to the
// subagent's run.
type SubagentPhase int

const (
	SubagentStarted SubagentPhase = iota
	SubagentEnded
)

func (p SubagentPhase) String() string {
	switch p {
	case SubagentStarted:
		return "started"
	case SubagentEnded:
		return "ended"
	default:
		return "unknown"
	}
}

// Event is the envelope. Exactly one of the *Payload fields is non-nil per
// event, matched to Kind. Consumers should switch on Kind and read the
// corresponding field directly — type-safe access, no reflection.
//
// AgentID identifies the emitter. ParentID is empty for the root agent and
// equal to the root's AgentID for subagent events (the hierarchy is always
// exactly two layers — subagents cannot spawn subagents).
type Event struct {
	Kind     Kind
	AgentID  string
	ParentID string
	Time     time.Time

	RunStart      *RunStartPayload      `json:",omitempty"`
	RunResume     *RunResumePayload     `json:",omitempty"`
	RunEnd        *RunEndPayload        `json:",omitempty"`
	IterLimit     *IterLimitPayload     `json:",omitempty"`
	Turn          *TurnPayload          `json:",omitempty"`
	Thinking      *TextPayload          `json:",omitempty"`
	Text          *TextPayload          `json:",omitempty"`
	ToolUseStart  *ToolUseStartPayload  `json:",omitempty"`
	ToolUseResult *ToolUseResultPayload `json:",omitempty"`
	Error         *ErrorPayload         `json:",omitempty"`
	TaskUpdate    *TaskUpdatePayload    `json:",omitempty"`
	Subagent      *SubagentPayload      `json:",omitempty"`
}

// --- payload types ---

type RunStartPayload struct {
	Prompt string
}

type RunResumePayload struct {
	FromMessageIndex int
}

type RunEndPayload struct {
	Final llm.Response
}

// IterLimitPayload is emitted when the loop hits Agent.maxIters. The UI
// should prompt the user (e.g. "press Enter to keep going") and call
// Agent.Continue to resume; the loop is paused, not failed.
type IterLimitPayload struct {
	Reached int
}

type TurnPayload struct {
	Iteration int
}

// TextPayload carries an opaque text chunk — used for both Thinking and
// Text events. With streaming completions this becomes a stream of chunks;
// today it carries the full block.
type TextPayload struct {
	Text string
}

type ToolUseStartPayload struct {
	Name   string
	Input  json.RawMessage
	ToolID string
}

type ToolUseResultPayload struct {
	ToolID  string
	Content string
	IsError bool
}

// ErrorPayload reports a Go-level failure that aborted the loop. Tool errors
// surfaced as Result.IsError do NOT produce this event — they flow through
// ToolUseResult so the model can recover.
type ErrorPayload struct {
	Stage string // "llm" | "tool:<name>" | "loop"
	Err   error
}

type TaskUpdatePayload struct {
	TaskID  string
	Status  string
	Subject string
}

type SubagentPayload struct {
	SubagentID    string
	AgentType     string // "explore", "general", ...
	PromptSummary string
	Phase         SubagentPhase
}
