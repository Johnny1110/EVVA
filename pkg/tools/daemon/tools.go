package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/johnny1110/evva/pkg/tools"
)

// Names lists every tool name this package contributes. Profile constructors
// concat this into their DeferredTools list (see internal/agent/profiles.go).
func Names() []tools.ToolName {
	return []tools.ToolName{tools.DAEMON_LIST, tools.DAEMON_STOP, tools.DAEMON_OUTPUT}
}

// --- daemon_list --------------------------------------------------------

// ListTool enumerates daemons in the agent's DaemonState. Optional kind
// filter; by default omits terminal entries since the drain evicts them
// shortly after lifecycle emission.
type ListTool struct{ state *DaemonState }

// NewList constructs the tool. state may be nil — Execute reports a clear
// error in that case so the model gets a useful message instead of a panic.
func NewList(state *DaemonState) *ListTool { return &ListTool{state: state} }

func (t *ListTool) Name() string { return string(tools.DAEMON_LIST) }

func (t *ListTool) Description() string {
	return "List background daemons started by this agent. " +
		"A daemon is any long-running unit registered into the agent's daemon state: " +
		"bash run_in_background tasks (b…), monitor streams (m…), and async subagents (a…). " +
		"Returns each daemon's id, kind, status, description, started-at, and kind-specific extras " +
		"(exit code for bash, event count for monitor, type/async for agent). " +
		"Pairs with daemon_output (read captured output) and daemon_stop (terminate)."
}

func (t *ListTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"properties":{
			"kind":{"type":"string","enum":["local_bash","local_agent","monitor"],"description":"Optional filter by daemon kind."},
			"include_terminal":{"type":"boolean","default":false,"description":"Include completed/failed/killed daemons (otherwise only running)."}
		}
	}`)
}

type listInput struct {
	Kind            DaemonKind `json:"kind"`
	IncludeTerminal bool       `json:"include_terminal"`
}

func (t *ListTool) Execute(_ context.Context, logger *slog.Logger, raw json.RawMessage) (tools.Result, error) {
	if t.state == nil {
		return tools.Result{IsError: true, Content: "daemon_list: state unavailable"}, nil
	}
	var in listInput
	if len(raw) > 0 && !isEmptyJSONObject(raw) {
		if err := json.Unmarshal(raw, &in); err != nil {
			return tools.Result{IsError: true, Content: fmt.Sprintf("daemon_list: decode: %v", err)}, nil
		}
	}
	var snaps []DaemonSnapshot
	if in.Kind != "" {
		snaps = t.state.SnapshotByKind(in.Kind)
	} else {
		snaps = t.state.Snapshot()
	}
	filtered := make([]DaemonSnapshot, 0, len(snaps))
	for _, s := range snaps {
		if !in.IncludeTerminal && IsTerminal(s.Status) {
			continue
		}
		filtered = append(filtered, s)
	}
	if len(filtered) == 0 {
		return tools.Result{Content: "no daemons"}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d daemon(s):\n", len(filtered))
	for _, s := range filtered {
		fmt.Fprintf(&b, "- %s [%s/%s] %s started=%s%s\n",
			s.ID,
			s.Kind,
			s.Status,
			s.Description,
			s.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
			formatExtras(s),
		)
	}
	logger.Debug("daemon_list.ok", "count", len(filtered))
	return tools.Result{Content: strings.TrimRight(b.String(), "\n")}, nil
}

// formatExtras renders kind-specific suffix on the daemon_list line.
func formatExtras(s DaemonSnapshot) string {
	switch m := s.Metadata.(type) {
	case LocalBashMeta:
		if m.ExitCode != nil {
			return fmt.Sprintf(" exit=%d", *m.ExitCode)
		}
		return ""
	case LocalAgentMeta:
		async := ""
		if m.Async {
			async = " async"
		}
		if m.AgentType == "" {
			return async
		}
		return fmt.Sprintf(" type=%s%s", m.AgentType, async)
	case MonitorMeta:
		return fmt.Sprintf(" events=%d", m.EventCount)
	default:
		return ""
	}
}

// --- daemon_stop --------------------------------------------------------

// StopTool terminates one daemon by id. Idempotent on already-terminal ids.
type StopTool struct{ state *DaemonState }

// NewStop constructs the tool.
func NewStop(state *DaemonState) *StopTool { return &StopTool{state: state} }

func (t *StopTool) Name() string { return string(tools.DAEMON_STOP) }

func (t *StopTool) Description() string {
	return "Terminate a running daemon by id. Works uniformly across bash background tasks (b…), " +
		"monitors (m…), and async subagents (a…). The daemon's natural teardown emits a killed " +
		"lifecycle which arrives on the agent's next turn as a <system-reminder>. " +
		"Idempotent: returns a no-op on daemons that already reached a terminal status. " +
		"Use after daemon_list to identify the id you want to stop."
}

func (t *StopTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["daemon_id"],
		"properties":{
			"daemon_id":{"type":"string","description":"Daemon id from daemon_list (b… / a… / m…)."}
		}
	}`)
}

type stopInput struct {
	DaemonID string `json:"daemon_id"`
}

func (t *StopTool) Execute(ctx context.Context, logger *slog.Logger, raw json.RawMessage) (tools.Result, error) {
	if t.state == nil {
		return tools.Result{IsError: true, Content: "daemon_stop: state unavailable"}, nil
	}
	var in stopInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("daemon_stop: decode: %v", err)}, nil
	}
	id := strings.TrimSpace(in.DaemonID)
	if id == "" {
		return tools.Result{IsError: true, Content: "daemon_stop: daemon_id is required"}, nil
	}
	snap, err := t.state.Stop(ctx, id)
	switch {
	case errors.Is(err, ErrDaemonNotFound):
		return tools.Result{IsError: true, Content: fmt.Sprintf("daemon_stop: %q not found", id)}, nil
	case errors.Is(err, ErrAlreadyTerminal):
		return tools.Result{Content: fmt.Sprintf("daemon_stop: %s already %s (no-op)", id, snap.Status)}, nil
	case err != nil:
		return tools.Result{IsError: true, Content: fmt.Sprintf("daemon_stop: kill failed: %v", err)}, nil
	}
	logger.Info("daemon_stop.ok", "id", id, "kind", snap.Kind)
	return tools.Result{Content: fmt.Sprintf(
		"daemon_stop: %s terminating; you will receive a killed lifecycle when it exits.",
		id)}, nil
}

// --- daemon_output ------------------------------------------------------

// OutputTool returns the kind-specific formatted output for one daemon.
type OutputTool struct{ state *DaemonState }

// NewOutput constructs the tool.
func NewOutput(state *DaemonState) *OutputTool { return &OutputTool{state: state} }

func (t *OutputTool) Name() string { return string(tools.DAEMON_OUTPUT) }

func (t *OutputTool) Description() string {
	return "Return the captured output of one daemon. Format is kind-specific: " +
		"bash daemons return stdout+stderr tail (capped 64 KiB), monitors return the last N event lines, " +
		"agent daemons return the prompt and the final summary when terminal. " +
		"Optional tail limits the result to the last N lines."
}

func (t *OutputTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["daemon_id"],
		"properties":{
			"daemon_id":{"type":"string","description":"Daemon id from daemon_list."},
			"tail":{"type":"number","minimum":1,"description":"Return only the last N lines."}
		}
	}`)
}

type outputInput struct {
	DaemonID string `json:"daemon_id"`
	Tail     *int   `json:"tail"`
}

func (t *OutputTool) Execute(_ context.Context, logger *slog.Logger, raw json.RawMessage) (tools.Result, error) {
	if t.state == nil {
		return tools.Result{IsError: true, Content: "daemon_output: state unavailable"}, nil
	}
	var in outputInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("daemon_output: decode: %v", err)}, nil
	}
	id := strings.TrimSpace(in.DaemonID)
	if id == "" {
		return tools.Result{IsError: true, Content: "daemon_output: daemon_id is required"}, nil
	}
	d, ok := t.state.Get(id)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("daemon_output: %q not found", id)}, nil
	}
	text := d.Output()
	if in.Tail != nil && *in.Tail > 0 {
		text = tailLines(text, *in.Tail)
	}
	logger.Debug("daemon_output.ok", "id", id, "bytes", len(text))
	return tools.Result{Content: text}, nil
}

// tailLines returns the last n lines of s. When s has fewer than n lines,
// returns s unchanged.
func tailLines(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// isEmptyJSONObject reports whether raw decodes to "{}" with optional
// whitespace. Lets daemon_list accept both an empty payload and a literal
// empty object without erroring on either.
func isEmptyJSONObject(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return t == "" || t == "{}" || t == "null"
}
