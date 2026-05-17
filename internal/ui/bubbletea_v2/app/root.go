// Package app is the v2 TUI's top-level tea.Model. It stays thin on
// purpose — focus stack, layout engine, and msg dispatch live here;
// every visual concern lives in a sibling component package.
package app

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/johnny1110/evva/internal/llm"
	"github.com/johnny1110/evva/internal/ui"
	"github.com/johnny1110/evva/internal/ui/bubbletea_v2/components/input"
	"github.com/johnny1110/evva/internal/ui/bubbletea_v2/components/status"
	"github.com/johnny1110/evva/internal/ui/bubbletea_v2/components/transcript"
	"github.com/johnny1110/evva/internal/ui/bubbletea_v2/events"
	"github.com/johnny1110/evva/internal/ui/bubbletea_v2/theme"
	"github.com/johnny1110/evva/pkg/banner"
)

// defaultGreeting is the welcome line rendered inside the banner box
// on startup.
const defaultGreeting = "// neural link established — what shall we build, ʘᴥʘ?"

// App is the v2 root model. M5 adds the bottom HUD (status bar +
// contextual hint), the run-state machine, and the spinner tick.
// The "running" bool from M4 is gone — the State machine owns
// lifecycle; the cancel func still lives here because it's the App
// that drives the goroutine.
type App struct {
	evvaHome   string
	program    *tea.Program
	controller ui.Controller

	width  int
	height int

	theme      *theme.Theme
	transcript *transcript.Transcript
	view       *transcript.View
	input      *input.Input
	status     *status.StatusBar
	state      *status.State

	// runCancel is the cancel func for the in-flight Run, set in
	// startRun and cleared in handleRunDone. Used by the Esc /
	// Ctrl+C handlers to interrupt mid-flight.
	runCancel context.CancelFunc
	// interrupted captures the "user pressed Esc" signal so the
	// RunDoneMsg handler can pick the "interrupted" hint instead
	// of "error: ...". Cleared on next OnSubmit.
	interrupted bool

	startedAt time.Time
}

// New builds a fresh App. The program reference is wired in
// afterwards.
func New(evvaHome string) *App {
	th := theme.Default()
	tr := transcript.New()
	tr.SetTheme(th)
	tr.SetBanner(transcript.BannerSpec{
		Art:      banner.Load(evvaHome),
		Greeting: defaultGreeting,
	})
	v := transcript.NewView(tr)
	in := input.New(th)
	st := status.NewState()
	bar := status.New(st)

	return &App{
		evvaHome:   evvaHome,
		theme:      th,
		transcript: tr,
		view:       v,
		input:      in,
		status:     bar,
		state:      st,
		startedAt:  time.Now(),
	}
}

// SetProgram lets the package-level UI hand the model the program
// reference. Used by the run goroutine to dispatch RunDoneMsg back
// to the bubbletea main loop.
func (a *App) SetProgram(p *tea.Program) { a.program = p }

// Attach hands the model the agent controller and re-renders the
// banner. Also primes the status bar with model + agent id and the
// initial context limit.
func (a *App) Attach(c ui.Controller) {
	a.controller = c
	a.refreshBanner()
	a.status.SetModel(c.Model())
	a.status.SetAgentID(c.AgentID())
	a.status.SetContext(0, status.ContextLimitFor(c.Model()))
	a.view.MarkDirty()
}

func (a *App) refreshBanner() {
	if a.controller == nil {
		return
	}
	id := a.controller.AgentID()
	if len(id) > 8 {
		id = id[:8]
	}
	a.transcript.SetBanner(transcript.BannerSpec{
		Art:      banner.Load(a.evvaHome),
		Greeting: defaultGreeting,
		Info: []transcript.BannerInfo{
			{Label: "agent", Value: id},
			{Label: "model", Value: a.controller.Model()},
			{Label: "started", Value: a.startedAt.Format("2006-01-02 15:04:05")},
		},
	})
}

// Init returns the cursor blink + spinner tick so both animate from
// the first frame.
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.input.BlinkCmd(), status.SpinnerTickCmd())
}

// Update routes incoming messages.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		// Reserve 5 rows for the input (3 textarea + 2 border) and
		// 2 rows for the status bar (hint line + status line).
		viewportH := m.Height - 7
		if viewportH < 1 {
			viewportH = 1
		}
		a.view.SetSize(m.Width, viewportH)
		a.input.SetWidth(m.Width)
		return a, nil

	case events.QuitMsg:
		if a.runCancel != nil {
			a.runCancel()
		}
		return a, tea.Quit

	case events.SpinnerTickMsg:
		// Advance the spinner; re-arm the tick. Cheap enough to run
		// unconditionally — the cache layer prevents per-tick block
		// re-renders unless something actually animates.
		a.state.TickSpinner()
		// If a compaction block is animating, the transcript needs
		// to know about the new frame so its CompactingBlock bumps
		// Rev and re-renders.
		if a.transcript.HasInflightCompacting() {
			a.transcript.SetSpinnerFrame(a.state.Frame())
			a.view.MarkDirty()
		}
		return a, status.SpinnerTickCmd()

	case events.AgentEventMsg:
		// State machine first — sub-phase transitions, sticky
		// terminal states.
		a.state.Apply(m.Event)
		// Per-event side effects on the status bar.
		if m.Event.Usage != nil {
			a.status.SetUsage(m.Event.Usage.Cumulative)
		}
		if a.transcript.IngestEvent(m.Event) {
			a.view.MarkDirty()
		}
		// Update context bar from session — fresh on every event
		// is cheaper than nothing because the meter changes most
		// often around tool returns / turn ends.
		if a.controller != nil {
			a.status.SetContext(
				a.controller.Session().LastTurnInputTokens(),
				status.ContextLimitFor(a.controller.Model()),
			)
		}
		return a, nil

	case events.RunDoneMsg:
		return a.handleRunDone(m.Err)

	case input.SubmitMsg:
		return a.handleSubmit(m)

	case tea.KeyMsg:
		return a.handleKey(m)
	}
	return a, nil
}

// handleRunDone fans the goroutine's exit error into the state
// machine and resets the cancel handle.
func (a *App) handleRunDone(err error) (tea.Model, tea.Cmd) {
	a.runCancel = nil
	interrupted := a.interrupted
	a.interrupted = false

	// Map the agent's interrupted error too — some providers
	// surface llm.ErrInterrupted instead of pure ctx.Cancelled.
	if errors.Is(err, llm.ErrInterrupted) {
		interrupted = true
	}
	a.state.OnRunDone(err, interrupted)
	return a, nil
}

// handleKey routes a key event. Order matters: special keys (quit,
// scroll, expand, history) precede the input textarea so multi-line
// composition with embedded special chars behaves consistently.
func (a *App) handleKey(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.String() {
	case "ctrl+c":
		// Running → cancel the run; idle → quit. Matches v1.
		if a.runCancel != nil {
			a.interrupted = true
			a.runCancel()
			return a, nil
		}
		return a, tea.Quit

	case "esc":
		// Running → cancel. Error → dismiss (matches the "Esc
		// dismiss" hint). Otherwise quit.
		if a.runCancel != nil {
			a.interrupted = true
			a.runCancel()
			return a, nil
		}
		if a.state.Current() == status.StateError {
			a.state.Dismiss()
			return a, nil
		}
		return a, tea.Quit

	case "ctrl+o":
		a.transcript.ToggleExpand()
		a.view.MarkDirty()
		return a, nil

	case "pgup", "pgdown", "home", "end":
		return a, a.view.Update(m)
	}

	cmd := a.input.Update(m)
	return a, cmd
}

// handleSubmit dispatches a SubmitMsg from the Input.
//
//   - Slash commands: /exit /quit /clear handled inline; the rest
//     wait for M7's overlay focus stack.
//   - Empty submit while iter-limit-paused: Continue without
//     appending a new user message.
//   - Empty submit otherwise: no-op.
//   - Regular text: append to transcript, start (or queue) a Run.
func (a *App) handleSubmit(m input.SubmitMsg) (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.ForAgent)

	switch text {
	case "/exit", "/quit", "exit":
		a.input.Reset()
		return a, tea.Quit
	case "/clear":
		a.transcript.Reset()
		a.input.Reset()
		a.state.SetHint("")
		a.view.MarkDirty()
		return a, nil
	}

	// Iter-limit takes precedence over the empty-text check: the
	// hint tells the user "press Enter to continue", and a continue
	// takes no payload.
	if a.state.Current() == status.StateIterLimit {
		a.input.Reset()
		a.startContinue()
		return a, nil
	}

	if text == "" {
		return a, nil
	}

	if a.controller == nil {
		a.state.SetHint("no controller attached")
		return a, nil
	}

	// Mid-run submit: queue the prompt; starting a second Run
	// while one is in flight 400s on every provider.
	if a.runCancel != nil {
		a.transcript.AppendUserPrompt(m.ForView)
		a.input.Reset()
		a.controller.ToolState().UserPromptQueue().Enqueue(m.ForAgent)
		a.state.SetHint("queued — will land at next iteration")
		a.view.MarkDirty()
		return a, nil
	}

	a.transcript.AppendUserPrompt(m.ForView)
	a.input.Reset()
	a.view.MarkDirty()
	a.startRun(m.ForAgent)
	return a, nil
}

// startRun kicks off a Run in a goroutine and transitions the state
// machine to running.
func (a *App) startRun(prompt string) {
	if a.controller == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.runCancel = cancel
	a.state.OnSubmit()

	p := a.program
	go func() {
		_, err := a.controller.Run(ctx, prompt)
		if p != nil {
			p.Send(events.RunDoneMsg{Err: err})
		}
	}()
}

// startContinue resumes an iter-limit-paused run via
// controller.Continue. Same goroutine + RunDoneMsg pattern as
// startRun.
func (a *App) startContinue() {
	if a.controller == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.runCancel = cancel
	a.state.OnSubmit()

	p := a.program
	go func() {
		_, err := a.controller.Continue(ctx)
		if p != nil {
			p.Send(events.RunDoneMsg{Err: err})
		}
	}()
}

// View composes the rendered output:
//
//	viewport / banner / transcript          (scrollable area)
//	input box                               (rounded border)
//	hint                                    (one-liner)
//	status bar                              (HUD)
func (a *App) View() string {
	if a.width == 0 {
		return "initializing…"
	}
	var b strings.Builder
	b.WriteString(a.view.View())
	b.WriteByte('\n')
	b.WriteString(a.input.View())
	b.WriteByte('\n')
	// Hint line above the status bar. M7 will route focus-stack
	// providers through here; for M5 we pass nil and rely on
	// state-override + state-default.
	hint := status.ResolveHint(a.state, nil)
	b.WriteString(a.theme.FooterHint.Render("  " + hint))
	b.WriteByte('\n')
	b.WriteString(a.status.Compose(a.width, a.theme))
	return b.String()
}
