package bubbletea

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/johnny1110/evva/internal/tools/meta"
	"github.com/johnny1110/evva/pkg/tools/todo"
	"github.com/johnny1110/evva/internal/toolset"
)

// agentChipMaxName caps how many characters of a subagent name appear
// inside a chip — anything longer is truncated with an ellipsis so a
// single strip can fit several agents on one row. Names are usually
// short (kebab-case identifiers), so 12 leaves room for "kebab-case"
// or "code-reviewer".
const agentChipMaxName = 12

// renderTodoPanel returns the bottom todo panel string, or "" when the
// store is empty. Each row prefixes a status glyph in the shared palette
// so the visual vocabulary matches the subagent panel and the
// all-todos-complete transcript snapshot.
//
// Width is the available column count; rows are truncated to fit.
func renderTodoPanel(ts *toolset.ToolState, width int) string {
	if ts == nil {
		return ""
	}
	store := ts.TodoStore()
	if store == nil {
		return ""
	}
	rows := store.List()
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(renderPanelHeader("TODOS", width))
	b.WriteByte('\n')
	for _, t := range rows {
		b.WriteString(renderTodoRow(t, width))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderPanelHeader formats a neon HUD section header. Label is
// uppercased and flanked by half-block "scanline" bars so panels feel
// like instrument readouts. Width controls the trailing rail length.
func renderPanelHeader(label string, width int) string {
	left := styles.PanelHeader.Render("▰▰ " + label + " ")
	tailLen := width - len(label) - 4
	if tailLen < 0 {
		tailLen = 0
	}
	tail := styles.Timeline.Render(strings.Repeat("▰", tailLen))
	return left + tail
}

// renderTodoRow formats one todo with its status glyph + label. Labels
// longer than the available width are truncated; the glyph stays visible
// so the user can always read progress at a glance.
func renderTodoRow(t todo.Todo, width int) string {
	g := glyphFor(string(t.Status))
	label := t.Content
	if t.Status == todo.StatusInProgress && t.ActiveForm != "" {
		label = t.ActiveForm
	}
	body := fmt.Sprintf("  %s  %s", styled(g.Symbol, g.Color), label)
	maxLen := width - 6
	if maxLen > 0 && len(label) > maxLen {
		body = fmt.Sprintf("  %s  %s", styled(g.Symbol, g.Color), truncate(label, maxLen))
	}
	return body
}

// AllTodosCompleted reports whether every todo in the store has reached
// StatusCompleted. The TUI watches this on every todo KindStoreUpdate
// to decide when to auto-fold the panel.
//
// Returns false on an empty store so a fresh store doesn't trigger a
// phantom "todos complete" snapshot.
func AllTodosCompleted(ts *toolset.ToolState) bool {
	if ts == nil {
		return false
	}
	store := ts.TodoStore()
	if store == nil {
		return false
	}
	rows := store.List()
	if len(rows) == 0 {
		return false
	}
	for _, t := range rows {
		if t.Status != todo.StatusCompleted {
			return false
		}
	}
	return true
}

// renderTodosCompleteSnapshot renders the green "all todos complete"
// block that gets folded into the transcript when the panel auto-clears.
// Mirrors renderTodoPanel's row shape but uses the green header style
// so the snapshot reads as a definite "done" event.
func renderTodosCompleteSnapshot(ts *toolset.ToolState, width int) string {
	if ts == nil {
		return ""
	}
	store := ts.TodoStore()
	if store == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(styles.TasksDone.Render("✔ TODOS COMPLETE"))
	for _, t := range store.List() {
		b.WriteByte('\n')
		b.WriteString(renderTodoRow(t, width))
	}
	return b.String()
}

// renderAgentStrip returns the horizontal HUD chip strip rendered just
// above the input. Each subagent is one bracketed chip — animated
// spinner / static glyph + truncated name + async tag — colored by
// its lifecycle status. Chips are separated by a soft dim spacer.
//
// Returns "" when no subagents are tracked so the caller can collapse
// the row entirely (zero visual cost when idle).
//
// width is the available column count; if the joined chips exceed it
// they wrap to a fresh row instead of being truncated. Losing visibility
// of a running agent is worse than spending an extra screen row.
func renderAgentStrip(ts *toolset.ToolState, width, frame int) string {
	if ts == nil || !ts.HasAgentGroupPanel() {
		return ""
	}
	rows := ts.AgentGroup().Snapshot()
	if len(rows) == 0 {
		return ""
	}

	spacer := styles.DimText.Render(" ")
	var lines []string
	var current strings.Builder
	currentWidth := 0
	for i, r := range rows {
		chip := renderAgentChip(r, frame)
		chipWidth := lipgloss.Width(chip)
		// 1-col spacer between chips on the same row.
		needWidth := chipWidth
		if currentWidth > 0 {
			needWidth += 1
		}
		if currentWidth > 0 && currentWidth+needWidth > width {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
		}
		if currentWidth > 0 {
			current.WriteString(spacer)
			currentWidth++
		}
		current.WriteString(chip)
		currentWidth += chipWidth
		_ = i
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return strings.Join(lines, "\n")
}

// renderAgentChip formats one subagent as a HUD chip: ‹glyph name›
// with optional async marker. The chevrons + content all share the
// agent's status color so the chip reads as one unit.
func renderAgentChip(r meta.SubagentSnapshot, frame int) string {
	status := strings.ToLower(r.Status)
	glyph := renderStatusGlyph(status, frame)
	name := r.Name
	if name == "" {
		name = r.ID
	}
	if len(name) > agentChipMaxName {
		name = name[:agentChipMaxName-1] + "…"
	}
	c := chipColor(status)
	chev := lipgloss.NewStyle().Foreground(c).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(c)
	asyncTag := ""
	if r.Async {
		asyncTag = styles.DimText.Render("ᵃ")
	}
	return chev.Render("‹") + glyph + " " + nameStyle.Render(name) + asyncTag + chev.Render("›")
}

// chipColor maps a subagent's lifecycle status to its chip border
// color. Mirrors the status pill vocabulary so agent state reads
// consistently across the bottom of the UI.
func chipColor(status string) lipgloss.Color {
	switch status {
	case "thinking", "texting":
		return paletteLightBlue
	case "executing":
		return paletteBrown
	case "draining", "saving":
		return palettePurple
	case "compacting", "max_iters":
		return paletteYellow
	case "ready_report", "idle":
		return paletteGreen
	case "crushed", "interrupted":
		return paletteRed
	case "init":
		return paletteMuted
	default:
		return paletteCyan
	}
}

// renderStatusGlyph picks the right symbol for a status string: the
// spinner frame in the spinner color when the status is active, the
// static palette glyph otherwise. Centralizing here keeps chip rows
// and the all-tasks-complete snapshot visually in sync.
func renderStatusGlyph(status string, frame int) string {
	if style, ok := spinnerGlyphFor(status); ok {
		return style.Render(spinnerFrame(frame))
	}
	g := glyphFor(status)
	return styled(g.Symbol, g.Color)
}


// styled wraps text in a foreground color without spinning up a fresh
// lipgloss.Style for every cell — keeps panel rendering cheap on
// frequent re-renders.
func styled(s string, c lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render(s)
}
