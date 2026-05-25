// Package lowprofile is evva's low-profile terminal UI. Black & gold theme,
// no banner, no gutter, no sprite. Every component is built from scratch —
// no code shared with the bubbletea TUI implementation.
//
// Wiring:
//
//	tui := lowprofile.New()
//	ag, _ := agent.New(cfg, agent.WithSink(tui), agent.WithRootContext(ctx))
//	tui.Attach(ag.Controller())
//	tui.Run(ctx)
package lowprofile

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/johnny1110/evva/pkg/event"
	"github.com/johnny1110/evva/pkg/ui"
)

// UI implements ui.UI with a black & gold low-profile design.
type UI struct {
	program *tea.Program
	model   *App

	mu         sync.Mutex
	controller ui.Controller
}

// New builds a low-profile UI.
func New() *UI {
	u := &UI{model: NewApp()}
	u.program = tea.NewProgram(u.model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	u.model.SetProgram(u.program)
	return u
}

// Emit satisfies event.Sink. Forwards events into the bubbletea loop.
func (u *UI) Emit(e event.Event) {
	if u.program == nil {
		return
	}
	u.program.Send(agentEventMsg{Event: e})
}

// Attach hands the UI its agent controller.
func (u *UI) Attach(c ui.Controller) {
	u.mu.Lock()
	u.controller = c
	u.model.Attach(c)
	u.mu.Unlock()
}

// Run starts the TUI and blocks until exit.
func (u *UI) Run(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			u.program.Send(tea.Quit())
		case <-done:
		}
	}()
	_, err := u.program.Run()
	close(done)
	return err
}
