// Package monitor hosts the deferred Monitor tool — a background process
// watcher that streams stdout lines as agent-loop notifications.
//
// The tool spawns the configured shell command in its own process group
// bound to the host's RootCtx (so monitor goroutines survive the LLM call
// that spawned them). Each stdout line becomes a daemon.Event signal on
// the agent's DaemonState; the drain at iter start folds the buffered
// events into a single <system-reminder>. The terminal lifecycle (Killed
// when daemon_stop fires, Completed when the script exits, Failed when
// spawn breaks) flows through the same DaemonState.
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/johnny1110/evva/pkg/tools"
)

// Names lists every tool name this package contributes.
func Names() []tools.ToolName { return []tools.ToolName{tools.MONITOR} }

// timeout defaults / clamps mirror the schema documentation.
const (
	defaultMonitorTimeout = 5 * time.Minute
	maxMonitorTimeout     = 60 * time.Minute
	monitorKillGrace      = 2 * time.Second
)

// MonitorTool spawns a long-running shell command and streams its stdout
// lines back to the agent as daemon.Event signals. host supplies the
// DaemonState + RootCtx + AgentID; without it the tool reports a clean
// error rather than panicking.
type MonitorTool struct {
	host DaemonHost
}

// NewMonitor constructs the production MonitorTool. The toolset factory
// passes the agent's *ToolState as host so per-event delivery routes
// through the agent's daemon signal pump.
func NewMonitor(host DaemonHost) *MonitorTool { return &MonitorTool{host: host} }

func (t *MonitorTool) Name() string { return string(tools.MONITOR) }

func (t *MonitorTool) Description() string {
	return "Start a background monitor that streams events from a long-running script. " +
		"Each stdout line becomes a notification delivered to the agent loop on a later turn. " +
		"Use for per-occurrence events: log watchers, file-change loops, dev-server outputs, " +
		"poll loops that emit one line per signal. " +
		"For a single \"tell me when X is done\" notification, prefer `bash run_in_background:true` instead. " +
		"Use `grep --line-buffered` in pipes so lines flush promptly. " +
		"The monitor stops itself when the underlying process exits OR when daemon_stop is called with its id. " +
		"Persistent monitors run for the lifetime of the session (no timeout); non-persistent monitors honour `timeout_ms`."
}

func (t *MonitorTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["description","timeout_ms","persistent","command"],
		"properties":{
			"command":{"type":"string","description":"Shell command or script. Each stdout line is an event; exit ends the watch."},
			"description":{"type":"string","description":"Short human-readable description of what you are monitoring (shown in notifications and the monitor strip)."},
			"persistent":{"type":"boolean","default":false,"description":"Run for the lifetime of the session (no timeout). Stop with daemon_stop."},
			"timeout_ms":{"type":"number","default":300000,"minimum":1000,"description":"Kill the monitor after this deadline. Default 300000ms, max 3600000ms. Ignored when persistent is true."}
		}
	}`)
}

type monitorInput struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	Persistent  bool   `json:"persistent"`
	TimeoutMs   int64  `json:"timeout_ms"`
}

func (t *MonitorTool) Execute(_ context.Context, logger *slog.Logger, raw json.RawMessage) (tools.Result, error) {
	var in monitorInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("monitor: decode: %v", err)}, nil
	}
	if strings.TrimSpace(in.Command) == "" {
		return tools.Result{IsError: true, Content: "monitor: command is required"}, nil
	}
	if t.host == nil || t.host.DaemonState() == nil {
		return tools.Result{IsError: true, Content: "monitor: host is not configured"}, nil
	}

	timeout := clampMonitorTimeout(in.TimeoutMs)
	d := newMonitorDaemon(
		t.host.RootCtx(),
		t.host.DaemonState(),
		in.Command,
		in.Description,
		t.host.AgentID(),
		in.Persistent,
		timeout,
		logger,
	)
	t.host.DaemonState().Register(d)
	go d.run()

	msg := fmt.Sprintf(
		"Monitor %s started. Stream events will be delivered as notifications; use daemon_stop %s to terminate it.",
		d.ID(), d.ID(),
	)
	return tools.Result{Content: msg}, nil
}

func clampMonitorTimeout(timeoutMs int64) time.Duration {
	dur := time.Duration(timeoutMs) * time.Millisecond
	switch {
	case dur <= 0:
		return defaultMonitorTimeout
	case dur > maxMonitorTimeout:
		return maxMonitorTimeout
	default:
		return dur
	}
}
