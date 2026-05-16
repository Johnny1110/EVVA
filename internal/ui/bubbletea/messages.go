package bubbletea

import (
	"github.com/johnny1110/evva/internal/agent/event"
	"github.com/johnny1110/evva/internal/tools/fs"
)

// approvalRequestMsg is dispatched from a tool goroutine (via the UI's
// fs.Approver impl) when an fs mutation needs user confirmation. The
// model stashes diff for rendering and reply for the eventual Decision.
// reply is buffered (cap 1) so a late Send after a ctx cancellation is
// harmless rather than deadlock.
type approvalRequestMsg struct {
	diff  *fs.FileDiff
	reply chan<- fs.Decision
}

// approvalCancelMsg tells the bubbletea loop to drop any pending
// approval — used when the tool's context fires before the user has
// answered. The approver has already returned ctx.Err() to its caller;
// this just clears the on-screen prompt.
type approvalCancelMsg struct{}

// eventMsg wraps an agent event for delivery into bubbletea's Update loop.
// Emit (called from the agent goroutine) wraps each event in this and
// calls tea.Program.Send so the message lands on the bubbletea main
// goroutine — model state is only mutated there.
type eventMsg struct {
	Event event.Event
}

// runDoneMsg is the bubbletea-side notification that a Controller.Run /
// Continue call has returned. It carries the error (if any) so Update can
// re-enable input, surface failures, and decide whether to ask the user
// to Continue on iter-limit.
type runDoneMsg struct {
	Err error
}

// quitMsg is dispatched when the user has decided to exit the UI. It is
// separate from tea.Quit so we can run any final cleanup in Update
// (cancel in-flight agent run, drain logs) before returning tea.Quit.
type quitMsg struct{}

// spinnerTickMsg drives the braille-dot spinner animation. Update
// increments the frame counter and schedules the next tick so the
// status bar and subagent panel keep cycling. The tick is also the
// re-render heartbeat for time-sensitive UI elements.
type spinnerTickMsg struct{}
