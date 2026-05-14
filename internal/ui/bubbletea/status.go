package bubbletea

import (
	"fmt"
	"strings"

	"github.com/johnny1110/evva/internal/llm"
)

// runState enumerates the agent's high-level state from the UI's
// perspective. Drives status-bar text and input-disable logic.
type runState int

const (
	stateIdle runState = iota
	stateRunning
	stateIterLimit
)

func (s runState) String() string {
	switch s {
	case stateRunning:
		return "running"
	case stateIterLimit:
		return "paused (iter-limit)"
	default:
		return "idle"
	}
}

// renderStatusBar formats the bottom status line: model, cumulative
// tokens, agent state. width is the terminal width so the bar can pad
// to fit.
func renderStatusBar(width int, model string, usage llm.Usage, state runState, hint string) string {
	parts := []string{
		styles.StatusKey.Render("model ") + styles.StatusValue.Render(model),
		styles.StatusKey.Render("in ") + styles.StatusValue.Render(fmt.Sprintf("%d", usage.InputTokens)) +
			styles.StatusKey.Render(" out ") + styles.StatusValue.Render(fmt.Sprintf("%d", usage.OutputTokens)),
		styles.StatusKey.Render("state ") + styles.StatusValue.Render(state.String()),
	}
	if hint != "" {
		parts = append(parts, styles.StatusKey.Render(hint))
	}
	body := strings.Join(parts, "  ·  ")
	return styles.StatusBar.Width(width).Render(body)
}
