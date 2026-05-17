// Package tasks renders the bottom task panel and the green
// "TASKS COMPLETE" snapshot folded into the transcript when every
// non-deleted task in the store finishes.
//
// Pure rendering — no tea.Model. The App passes the current
// *toolset.ToolState to Render on every frame; the panel reads the
// task store, filters deleted entries, and produces a styled
// multi-line string. Returns "" when there's nothing to show so
// the layout collapses the slot.
package tasks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/johnny1110/evva/internal/tools/task"
	"github.com/johnny1110/evva/internal/toolset"
	"github.com/johnny1110/evva/internal/ui/bubbletea_v2/theme"
)

// Render returns the task panel as a styled string. Empty when no
// non-deleted tasks are tracked.
//
// width caps row length; oversized subjects are truncated with an
// ellipsis. The header is a HUD-style scanline ("▰▰ TASKS ▰▰▰…").
func Render(ts *toolset.ToolState, width int, th *theme.Theme) string {
	if ts == nil {
		return ""
	}
	store := ts.TaskStore()
	if store == nil {
		return ""
	}
	rows := visibleTasks(store)
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(renderHeader("TASKS", width, th))
	b.WriteByte('\n')
	for _, t := range rows {
		b.WriteString(renderRow(t, width, th))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// AllCompleted reports whether every non-deleted task in the store
// has reached StatusCompleted. The App watches this on every task
// KindStoreUpdate to decide when to auto-fold the panel into a
// transcript snapshot.
//
// Returns false on an empty / all-deleted store so a fresh store
// doesn't trigger a phantom snapshot.
func AllCompleted(ts *toolset.ToolState) bool {
	if ts == nil {
		return false
	}
	store := ts.TaskStore()
	if store == nil {
		return false
	}
	any := false
	for _, t := range store.List() {
		if t.Status == task.StatusDeleted {
			continue
		}
		any = true
		if t.Status != task.StatusCompleted {
			return false
		}
	}
	return any
}

// RenderCompleteSnapshot builds the green "TASKS COMPLETE" snapshot
// that gets folded into the transcript when the panel auto-clears.
// Mirrors Render's row shape but uses the green header style so the
// snapshot reads as a definite "done" event in the scrollback.
func RenderCompleteSnapshot(ts *toolset.ToolState, width int, th *theme.Theme) string {
	if ts == nil {
		return ""
	}
	store := ts.TaskStore()
	if store == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(th.TasksDone.Render("✔ TASKS COMPLETE"))
	for _, t := range store.List() {
		if t.Status == task.StatusDeleted {
			continue
		}
		b.WriteByte('\n')
		b.WriteString(renderRow(t, width, th))
	}
	return b.String()
}

// visibleTasks returns the non-deleted tasks from the store in their
// stored order. Pulled into a helper so tests can exercise the
// filter directly.
func visibleTasks(store *task.TaskGroup) []task.Task {
	all := store.List()
	out := make([]task.Task, 0, len(all))
	for _, t := range all {
		if t.Status == task.StatusDeleted {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// renderHeader produces a HUD section header — "▰▰ LABEL ▰▰▰…" with
// the trailing rail padded out to width. Reused (in spirit) by the
// agents strip; the helper lives here because tasks is the only
// caller for now.
func renderHeader(label string, width int, th *theme.Theme) string {
	left := th.PanelHeader.Render("▰▰ " + label + " ")
	// 4 = "▰▰ " (3 cols) + trailing space (1 col).
	tailLen := width - len(label) - 4
	if tailLen < 0 {
		tailLen = 0
	}
	tail := th.Timeline.Render(strings.Repeat("▰", tailLen))
	return left + tail
}

// renderRow formats one task with its lifecycle glyph + subject.
// Subjects longer than the row width are truncated; the glyph
// always stays visible so the user can read progress at a glance.
func renderRow(t task.Task, width int, th *theme.Theme) string {
	g := th.Glyph(string(t.Status))
	glyph := lipgloss.NewStyle().Foreground(g.Color).Render(g.Symbol)

	subject := t.Subject
	// Reserve 6 cols for "  G  " (2 + symbol + 2; "G" prints as 1
	// cell even when the rune is wide — close enough for the
	// hand-tuned truncation cap).
	maxLen := width - 6
	if maxLen > 0 && len(subject) > maxLen {
		subject = truncate(subject, maxLen)
	}
	return fmt.Sprintf("  %s  %s", glyph, subject)
}

func truncate(s string, max int) string {
	if max < 1 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
