package fs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/johnny1110/evva/internal/tools"
)

// WriteTool creates or overwrites a file. When an Approver is attached
// every mutation pauses for user confirmation: the proposed diff is
// shown and the write only lands on a "yes". A nil approver disables
// the gate (used by tests).
type WriteTool struct {
	tracker  *ReadTracker
	approver Approver
}

// NewWrite creates a WriteTool that enforces the read-before-overwrite guard
// via the given tracker. Pass a non-nil approver to gate writes behind
// user confirmation; nil disables the gate.
func NewWrite(tracker *ReadTracker, approver Approver) *WriteTool {
	return &WriteTool{tracker: tracker, approver: approver}
}

func (t *WriteTool) Name() string { return string(tools.WRITE_FILE) }

func (t *WriteTool) Description() string {
	return "Writes a file to the local filesystem. file_path must be absolute. " +
		"Use this for creating new files or fully overwriting an existing " +
		"one. For partial edits to an existing file, prefer edit_file — it " +
		"preserves surrounding content and is harder to misuse.\n\n" +
		"Overwriting an existing file requires you to have called " +
		"read_file on it first in this session — the tool refuses to " +
		"blindly clobber a file you haven't loaded into context. New " +
		"files (path doesn't exist) need no prior read. Missing parent " +
		"directories are created automatically.\n\n" +
		"Never create documentation files (*.md) or README files unless " +
		"the user explicitly asked for them. Only use emojis if the user " +
		"explicitly requested them."
}

func (t *WriteTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["file_path","content"],
		"properties":{
			"file_path":{"type":"string","description":"Absolute path to the file to write (must be absolute, not relative)."},
			"content":{"type":"string","description":"Full text content to write to the file."}
		}
	}`)
}

type writeInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (t *WriteTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{IsError: true, Content: "write: decode input: " + err.Error()}, nil
	}

	resolved, err := resolvePath(in.FilePath)
	if err != nil {
		return tools.Result{IsError: true, Content: "write: " + err.Error()}, nil
	}

	existedBefore := fileExists(resolved)

	// Read-before-overwrite guard. New files are exempt.
	if existedBefore && t.tracker != nil && !t.tracker.WasRead(resolved) {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf(
				"write: you must use read_file on %s before overwriting it. "+
					"Read the file first so you don't blindly clobber existing "+
					"content; if you want a partial change use edit_file instead.",
				in.FilePath,
			),
		}, nil
	}

	// Capture prior content. We need this for two things on the
	// overwrite path: the proposed diff that the approver renders, and
	// the "was M / now N" summary line on the model-facing result.
	var oldByteCount, oldLineCount int
	var priorContent string
	if existedBefore {
		prior, perr := os.ReadFile(resolved)
		if perr == nil {
			priorContent = string(prior)
			oldByteCount = len(prior)
			oldLineCount = countLines(priorContent)
		}
	}

	// Build the proposed diff up front so the same payload powers both
	// the approval prompt and the final tools.Result.Metadata. New
	// files render every line as an add; overwrites use difflib for a
	// minimal unified diff.
	var diff *FileDiff
	if existedBefore {
		diff = buildOverwriteDiff(resolved, priorContent, in.Content)
	} else {
		diff = buildCreateDiff(resolved, in.Content)
	}

	if t.approver != nil {
		dec, aerr := t.approver.Approve(ctx, diff)
		if aerr != nil {
			return tools.Result{IsError: true, Content: "write: approval failed: " + aerr.Error()}, nil
		}
		if !dec.Approved {
			// Two decline shapes:
			//   - No feedback (user pressed Esc / EOF / blank line) →
			//     surface a real error so the model knows the change
			//     was refused outright.
			//   - With feedback → silently pass the user's redirection
			//     through as a non-error result. No "declined" chrome
			//     in the transcript; the model treats the text as
			//     in-flight instructions and continues the workflow.
			if dec.Feedback == "" {
				return tools.Result{
					IsError: true,
					Content: fmt.Sprintf("write: user declined the change to %s; the file was not modified.", in.FilePath),
				}, nil
			}
			return tools.Result{
				Content: fmt.Sprintf("User asked instead: %s", dec.Feedback),
			}, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: could not create parent dirs: %s", err)}, nil
	}
	if err := os.WriteFile(resolved, []byte(in.Content), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: could not write %s: %s", in.FilePath, err)}, nil
	}

	if t.tracker != nil {
		t.tracker.MarkRead(resolved)
	}

	newLineCount := countLines(in.Content)
	newByteCount := len(in.Content)

	if !existedBefore {
		return tools.Result{
			Content:  fmt.Sprintf("created %s (%d lines, %d bytes)", resolved, newLineCount, newByteCount),
			Metadata: diff,
		}, nil
	}
	return tools.Result{
		Content: fmt.Sprintf("overwrote %s (was %d lines / %d bytes, now %d lines / %d bytes)",
			resolved, oldLineCount, oldByteCount, newLineCount, newByteCount),
		Metadata: diff,
	}, nil
}

// countLines counts the lines in s the way users count them — a final "\n"
// is the terminator of the last line, not a marker for an extra empty line.
// Empty string → 0 lines.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
