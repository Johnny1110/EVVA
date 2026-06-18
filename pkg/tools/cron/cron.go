// Package cron hosts the recurring-schedule prompt tools: cron_create,
// cron_list, cron_delete. They share the alarm package's Scheduler — a cron
// job is an alarm.Alarm with a non-empty CronExpr — so cron jobs and one-shot
// alarms coexist in one timer set, one durable store, and one fire path
// (WakeupQueue enqueue + idle-wake) the agent already wires for alarms.
//
// remote_trigger lives here for parity with the reference source but is an
// unrelated HTTP-client feature and remains a stub.
package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/johnny1110/evva/pkg/common"
	"github.com/johnny1110/evva/pkg/tools"
	"github.com/johnny1110/evva/pkg/tools/alarm"
)

// Names lists every tool name this package contributes.
func Names() []tools.ToolName {
	return []tools.ToolName{
		tools.CRON_CREATE, tools.CRON_LIST, tools.CRON_DELETE, tools.REMOTE_TRIGGER,
	}
}

// Next returns the first activation strictly after `after` for a 5-field cron
// expression. It is the adapter the toolset wires into the alarm scheduler's
// CronNext seam, keeping the scheduler itself free of any cron-engine import.
func Next(expr string, after time.Time) (time.Time, error) {
	e, err := ParseExpr(expr)
	if err != nil {
		return time.Time{}, err
	}
	return e.NextAfter(after)
}

// --- cron_create ------------------------------------------------------------

// CreateTool implements CRON_CREATE. Execute is non-blocking: it arms a
// recurring (or one-shot) timer on the shared alarm Scheduler and returns.
type CreateTool struct{ sched *alarm.Scheduler }

// NewCreate constructs a CRON_CREATE tool bound to the shared scheduler — the
// same instance whose fire callback Enqueues the prompt and wakes the loop.
func NewCreate(s *alarm.Scheduler) *CreateTool { return &CreateTool{sched: s} }

func (t *CreateTool) Name() string { return string(tools.CRON_CREATE) }

func (t *CreateTool) Description() string {
	return "Schedule a prompt to be enqueued on a recurring wall-clock pattern, re-entering the conversation with that prompt as a fresh user message each time it fires.\n\n" +
		"Uses a standard 5-field cron expression in your local timezone (" + common.ZoneLabel() + "): \"M H DoM Mon DoW\". " +
		"For a single trigger at one exact instant, use alarm_create instead — cron is for repeating schedules.\n\n" +
		"Avoid :00 and :30 minute marks when you can — pick off-minutes like 7 or 57 to spread load. " +
		"Recurring jobs auto-expire 7 days after creation. Jobs fire only while the REPL is idle. " +
		"Session-only by default; pass `durable: true` to persist across restarts."
}

func (t *CreateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["cron","prompt"],
		"properties":{
			"cron":{"type":"string","description":"Standard 5-field cron expression in local time: \"M H DoM Mon DoW\" (e.g. \"*/5 * * * *\" = every 5 minutes, \"30 14 28 2 *\" = Feb 28 at 2:30pm local once)."},
			"prompt":{"type":"string","description":"The prompt to enqueue at each fire time. Write it self-contained — it will make sense with no other context."},
			"recurring":{"type":"boolean","description":"true (default) = fire on every cron match until deleted or auto-expired after 7 days. false = fire once at the next match, then auto-delete."},
			"durable":{"type":"boolean","description":"true = persist to disk and survive restarts. false (default) = in-memory only, dies when this session ends."}
		}
	}`)
}

type createInput struct {
	Cron      string `json:"cron"`
	Prompt    string `json:"prompt"`
	Recurring *bool  `json:"recurring"` // pointer: omitted defaults to true
	Durable   *bool  `json:"durable"`   // pointer: omitted defaults to false
}

func (t *CreateTool) Execute(_ context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
	if t.sched == nil {
		return tools.Result{IsError: true, Content: "cron_create: no scheduler configured (cron is root-agent only)"}, nil
	}
	var in createInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("cron_create: decode: %v", err)}, nil
	}
	expr := strings.TrimSpace(in.Cron)
	if expr == "" {
		return tools.Result{IsError: true, Content: "cron_create: cron expression is required"}, nil
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return tools.Result{IsError: true, Content: "cron_create: prompt is required"}, nil
	}
	// Validate up front for a precise error before touching the scheduler.
	if _, err := ParseExpr(expr); err != nil {
		return tools.Result{IsError: true, Content: "cron_create: " + err.Error()}, nil
	}
	recurring := true
	if in.Recurring != nil {
		recurring = *in.Recurring
	}
	durable := false
	if in.Durable != nil {
		durable = *in.Durable
	}
	a, err := t.sched.Arm(alarm.Alarm{
		CronExpr:  expr,
		Recurring: recurring,
		Prompt:    in.Prompt,
		Durable:   durable,
	})
	if err != nil {
		return tools.Result{IsError: true, Content: "cron_create: " + err.Error()}, nil
	}
	logger.Debug("cron.armed", "id", a.ID, "cron", a.CronExpr, "recurring", recurring,
		"fire_at", a.FireAt.Format(time.RFC3339), "durable", durable)
	kind := "recurring"
	if !recurring {
		kind = "one-shot"
	}
	until := time.Until(a.FireAt).Round(time.Second)
	var b strings.Builder
	fmt.Fprintf(&b, "cron job %s scheduled (%s): %q — next fire %s (in %s)%s.",
		a.ID, kind, a.CronExpr, common.Stamp(a.FireAt), until, durableSuffix(durable))
	if recurring {
		fmt.Fprintf(&b, " Auto-expires %s.", common.Stamp(a.Expiry))
	}
	return tools.Result{Content: b.String(), Metadata: a}, nil
}

// --- cron_list --------------------------------------------------------------

// ListTool implements CRON_LIST.
type ListTool struct{ sched *alarm.Scheduler }

// NewList constructs a CRON_LIST tool bound to the shared scheduler.
func NewList(s *alarm.Scheduler) *ListTool { return &ListTool{sched: s} }

func (t *ListTool) Name() string { return string(tools.CRON_LIST) }

func (t *ListTool) Description() string {
	return "List all pending cron jobs set via cron_create — id, expression, next fire time, time remaining, recurring/one-shot, durability, and prompt. One-shot alarms set via alarm_create are listed separately by alarm_list."
}

func (t *ListTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)
}

func (t *ListTool) Execute(_ context.Context, _ *slog.Logger, _ json.RawMessage) (tools.Result, error) {
	if t.sched == nil {
		return tools.Result{IsError: true, Content: "cron_list: no scheduler configured"}, nil
	}
	var jobs []alarm.Alarm
	for _, a := range t.sched.List() {
		if a.CronExpr != "" {
			jobs = append(jobs, a)
		}
	}
	if len(jobs) == 0 {
		return tools.Result{Content: "No pending cron jobs."}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d pending cron job(s):\n", len(jobs))
	now := time.Now()
	for _, a := range jobs {
		kind := "recurring"
		if !a.Recurring {
			kind = "one-shot"
		}
		fmt.Fprintf(&b, "- %s — %q (%s) — next %s (in %s)%s\n    %s\n",
			a.ID, a.CronExpr, kind, common.Stamp(a.FireAt),
			a.FireAt.Sub(now).Round(time.Second), durableSuffix(a.Durable), truncate(a.Prompt, 100))
	}
	return tools.Result{Content: strings.TrimRight(b.String(), "\n"), Metadata: jobs}, nil
}

// --- cron_delete ------------------------------------------------------------

// DeleteTool implements CRON_DELETE.
type DeleteTool struct{ sched *alarm.Scheduler }

// NewDelete constructs a CRON_DELETE tool bound to the shared scheduler.
func NewDelete(s *alarm.Scheduler) *DeleteTool { return &DeleteTool{sched: s} }

func (t *DeleteTool) Name() string { return string(tools.CRON_DELETE) }

func (t *DeleteTool) Description() string {
	return "Cancel a pending cron job by id (from cron_create or cron_list) so it never fires again."
}

func (t *DeleteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["id"],
		"properties":{"id":{"type":"string","description":"Job id returned by cron_create / shown by cron_list."}}
	}`)
}

func (t *DeleteTool) Execute(_ context.Context, logger *slog.Logger, input json.RawMessage) (tools.Result, error) {
	if t.sched == nil {
		return tools.Result{IsError: true, Content: "cron_delete: no scheduler configured"}, nil
	}
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("cron_delete: decode: %v", err)}, nil
	}
	if strings.TrimSpace(in.ID) == "" {
		return tools.Result{IsError: true, Content: "cron_delete: id is required"}, nil
	}
	if t.sched.Cancel(in.ID) {
		logger.Debug("cron.cancelled", "id", in.ID)
		return tools.Result{Content: fmt.Sprintf("cron job %s cancelled.", in.ID)}, nil
	}
	return tools.Result{IsError: true, Content: fmt.Sprintf("cron_delete: no pending cron job with id %q", in.ID)}, nil
}

// --- remote_trigger (stub) --------------------------------------------------

// Trigger remains a stub — remote_trigger is an unrelated remote-API client,
// out of scope for cron scheduling (PRD §5.6/§6).
var Trigger tools.Tool = tools.NewStub(
	tools.REMOTE_TRIGGER,
	"Call the remote-trigger API. Use this instead of curl — the OAuth token is added automatically in-process and never exposed. "+
		"Actions: list (GET all), get (GET one), create (POST new — requires body), "+
		"update (POST partial update — requires body), run (POST /run — optional body). "+
		"Returns raw JSON from the API.",
	`{
		"type":"object",
		"additionalProperties":false,
		"required":["action"],
		"properties":{
			"action":{"type":"string","enum":["list","get","create","update","run"],"description":"API operation to perform."},
			"trigger_id":{"type":"string","pattern":"^[\\w-]+$","description":"Required for get, update, and run."},
			"body":{"type":"object","additionalProperties":{},"propertyNames":{"type":"string"},"description":"Required for create and update; optional for run."}
		}
	}`,
)

// durableSuffix renders the durability marker shared by create + list output.
func durableSuffix(d bool) string {
	if d {
		return " [durable]"
	}
	return " [session-only]"
}

// truncate clips s to n runes, appending an ellipsis when shortened.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
