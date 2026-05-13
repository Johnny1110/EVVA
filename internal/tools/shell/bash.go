package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/johnny1110/evva/internal/tools"
)

// Default and maximum timeouts. The maximum mirrors the schema's documented
// 600 000 ms cap; anything larger is clamped on input.
const (
	defaultBashTimeout = 2 * time.Minute
	maxBashTimeout     = 10 * time.Minute
)

// Bash is the singleton BashTool. Stateless — every invocation spawns a
// fresh shell process so one instance suffices across all agents.
var Bash tools.Tool = &BashTool{}

type BashTool struct{}

func NewBash() *BashTool { return &BashTool{} }

func (t *BashTool) Name() string { return string(tools.BASH) }

func (t *BashTool) Description() string {
	return "Executes a given bash command and returns its combined stdout+stderr output.\n\n" +
		"The working directory persists between commands, but shell state (env vars, aliases) does not — " +
		"each call runs in a fresh shell.\n\n" +
		"Prefer dedicated tools when one fits: Read for known paths, Edit for edits, Write for new files. " +
		"Reserve Bash for shell-only operations.\n\n" +
		"Timeout defaults to 120000 ms (2 min), max 600000 ms (10 min). " +
		"run_in_background and dangerouslyDisableSandbox are reserved for future implementations and currently rejected."
}

func (t *BashTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["command"],
		"properties":{
			"command":{"type":"string","description":"The command to execute"},
			"description":{"type":"string","description":"Clear, concise description of what this command does in active voice."},
			"timeout":{"type":"number","description":"Optional timeout in milliseconds (max 600000, default 120000)"},
			"run_in_background":{"type":"boolean","description":"Reserved — currently rejected. Use Monitor for background streaming once available."},
			"dangerouslyDisableSandbox":{"type":"boolean","description":"Reserved — currently rejected."}
		}
	}`)
}

type bashInput struct {
	Command                   string  `json:"command"`
	Description               string  `json:"description"`
	Timeout                   *int64  `json:"timeout"`
	RunInBackground           bool    `json:"run_in_background"`
	DangerouslyDisableSandbox bool    `json:"dangerouslyDisableSandbox"`
	_                         float64 // silence unused-field warnings if any
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("bash: decode input: %v", err)}, nil
	}
	if strings.TrimSpace(in.Command) == "" {
		return tools.Result{IsError: true, Content: "bash: command is required"}, nil
	}
	if in.RunInBackground {
		return tools.Result{
			IsError: true,
			Content: "bash: run_in_background is not implemented yet — use Monitor (deferred) when it lands",
		}, nil
	}
	if in.DangerouslyDisableSandbox {
		return tools.Result{
			IsError: true,
			Content: "bash: dangerouslyDisableSandbox is reserved and currently rejected",
		}, nil
	}

	timeout := defaultBashTimeout
	if in.Timeout != nil {
		ms := time.Duration(*in.Timeout) * time.Millisecond
		switch {
		case ms <= 0:
			timeout = defaultBashTimeout
		case ms > maxBashTimeout:
			timeout = maxBashTimeout
		default:
			timeout = ms
		}
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "/bin/sh", "-c", in.Command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	out := buf.String()

	// Distinguish timeout from generic exit-status failure for the model.
	if cctx.Err() == context.DeadlineExceeded {
		msg := fmt.Sprintf("bash: command timed out after %s\n--- partial output ---\n%s", timeout, out)
		return tools.Result{IsError: true, Content: msg}, nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		// Caller cancellation — propagate via Go error so the loop returns
		// llm.ErrInterrupted to the CLI.
		return tools.Result{IsError: true, Content: "bash: cancelled"}, ctx.Err()
	}

	if err != nil {
		// Non-zero exit. Include the output and the exit-code suffix so the
		// model can reason about the failure.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := fmt.Sprintf("%s\n--- exit code %d ---", out, exitErr.ExitCode())
			return tools.Result{IsError: true, Content: msg}, nil
		}
		// Spawn-level error (binary not found, etc.) — surface as IsError;
		// the model can suggest a different command.
		return tools.Result{IsError: true, Content: fmt.Sprintf("bash: %v", err)}, nil
	}

	return tools.Result{Content: out}, nil
}
