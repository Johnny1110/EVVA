package bubbletea

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/johnny1110/evva/internal/agent/event"
	"github.com/johnny1110/evva/internal/tools/fs"
)

// transcript accumulates a scrollback buffer of human-readable lines from
// the agent's event stream. It is intentionally view-only: foldEvent
// appends pre-styled strings; the parent model is responsible for
// rendering them inside a viewport.
//
// Each entry is one logical "block" (a user prompt, an assistant text
// turn, a single tool call + result, an error banner, ...). Blocks may
// contain internal newlines.
type transcript struct {
	blocks []string
}

// String returns the entire scrollback as one newline-joined buffer.
func (t *transcript) String() string {
	return strings.Join(t.blocks, "\n\n")
}

// appendUserPrompt records a prompt the user just submitted.
func (t *transcript) appendUserPrompt(text string) {
	t.blocks = append(t.blocks, styles.UserPrompt.Render("> "+text))
}

// foldEvent translates one agent event into a transcript entry (or
// updates an in-flight one). Returns true if the transcript changed and
// the viewport should re-render.
func (t *transcript) foldEvent(e event.Event) bool {
	switch e.Kind {
	case event.KindThinking:
		if e.Thinking != nil && e.Thinking.Text != "" {
			t.blocks = append(t.blocks, styles.Thinking.Render("· "+truncate(e.Thinking.Text, 800)))
			return true
		}
	case event.KindText:
		if e.Text != nil && e.Text.Text != "" {
			t.blocks = append(t.blocks, styles.Assistant.Render(e.Text.Text))
			return true
		}
	case event.KindToolUseStart:
		if e.ToolUseStart != nil {
			label := fmt.Sprintf("→ %s %s", e.ToolUseStart.Name, compactInput(e.ToolUseStart.Input))
			t.blocks = append(t.blocks, styles.ToolCall.Render(label))
			return true
		}
	case event.KindToolUseResult:
		if e.ToolUseResult != nil {
			var b strings.Builder
			if e.ToolUseResult.IsError {
				b.WriteString(styles.ToolErr.Render("✗ " + truncate(e.ToolUseResult.Content, 800)))
			} else {
				b.WriteString(styles.ToolOK.Render("✓ " + truncate(e.ToolUseResult.Content, 800)))
			}
			if diff, ok := e.ToolUseResult.Metadata.(*fs.FileDiff); ok && diff != nil {
				b.WriteByte('\n')
				b.WriteString(renderFileDiff(diff))
			}
			t.blocks = append(t.blocks, b.String())
			return true
		}
	case event.KindError:
		if e.Error != nil {
			t.blocks = append(t.blocks, styles.ErrorBanner.Render(fmt.Sprintf("[error:%s] %v", e.Error.Stage, e.Error.Err)))
			return true
		}
	case event.KindRunCancelled:
		t.blocks = append(t.blocks, styles.DimText.Render("[cancelled]"))
		return true
	case event.KindIterLimit:
		if e.IterLimit != nil {
			t.blocks = append(t.blocks, styles.DimText.Render(fmt.Sprintf("[iter-limit] reached %d iterations — press Enter to continue", e.IterLimit.Reached)))
			return true
		}
	}
	return false
}

func compactInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	s := truncate(string(raw), 160)
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
