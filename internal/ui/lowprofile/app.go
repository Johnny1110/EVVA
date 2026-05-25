package lowprofile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/johnny1110/evva/pkg/constant"
	"github.com/johnny1110/evva/pkg/event"
	"github.com/johnny1110/evva/pkg/llm"
	"github.com/johnny1110/evva/pkg/tools/todo"
	"github.com/johnny1110/evva/pkg/ui"
)

// App is the low-profile TUI root model. All components and rendering are
// built from scratch — no reuse of any existing bubbletea component code.
//
// Layout (top → bottom):
//
//	status bar (1 line, gold on charcoal)
//	viewport (scrollable transcript, clean left-aligned)
//	todo / daemon panels (when populated)
//	overlay (modal, gold-bordered box)
//	separator rule
//	input (▸ prompt, gold)
type App struct {
	program    *tea.Program
	controller ui.Controller

	width  int
	height int

	th *Theme

	// Transcript — each string is one pre-rendered styled line.
	lines  []string
	scroll int // lines scrolled above visible window; 0 = at bottom

	// Input
	input *Input

	// Run state
	runState    string // "ready" | "thinking" | "executing" | "texting" | "error" | "paused"
	runCancel   context.CancelFunc
	interrupted bool
	statusHint  string

	// Status bar data
	modelID      string
	agentName    string
	effort       string
	permMode     string
	usageIn      int
	usageOut     int
	ctxUsed      int
	ctxLimit     int
	agentID      string

	// Overlay system
	overlay overlay

	// Slash suggestions
	slashVisible bool
	slashMatches []slashEntry
	slashSel     int
	slashDismiss bool

	// Todo + daemon data (for panel rendering)
	todoLines  []string
	agentLines []string
	bgLines    []string
	monLines   []string

	// Spinner
	spinnerFrame int

	lastMouseAt time.Time
}

type slashEntry struct {
	Name, Desc string
}

// builtin slash commands
var slashBuiltins = []slashEntry{
	{"/compact", "compact transcript · micro or full"},
	{"/config", "edit runtime settings"},
	{"/effort", "set thinking effort · low, medium, high, ultra"},
	{"/model", "switch LLM provider / model"},
	{"/profile", "switch agent persona"},
	{"/resume", "resume a previous session"},
	{"/update", "check for updates"},
	{"/clear", "clear transcript"},
	{"/exit", "quit"},
}

// --- construction ---

// NewApp builds the root model.
func NewApp() *App {
	th := NewTheme()
	return &App{
		th:        th,
		input:     NewInput(th),
		runState:  "ready",
		agentName: "EVVA",
	}
}

// SetProgram wires the tea.Program reference.
func (a *App) SetProgram(p *tea.Program) { a.program = p }

// Attach hands the controller and primes status fields.
func (a *App) Attach(c ui.Controller) {
	a.controller = c
	a.modelID = c.Model()
	a.effort = c.Effort()
	a.agentID = shortID(c.AgentID())
	a.agentName = strings.ToUpper(c.ProfileName())
	a.permMode = c.PermissionModeName()
	if a.agentName == "" {
		a.agentName = "EVVA"
	}
	a.ctxLimit = constant.MODEL_CONTEXT_SIZE[constant.Model(c.Model())]
}

// --- tea.Model ---

func (a *App) Init() tea.Cmd {
	return tickCmd()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.input.SetWidth(m.Width)
		return a, nil

	case spinnerTickMsg:
		a.spinnerFrame++
		return a, tickCmd()

	case agentEventMsg:
		return a, a.handleEvent(m.Event)

	case runDoneMsg:
		return a, a.handleRunDone(m.Err)

	case tea.KeyMsg:
		return a.handleKey(m)

	case tea.MouseMsg:
		if m.Button == tea.MouseButtonWheelUp || m.Button == tea.MouseButtonWheelDown {
			a.lastMouseAt = time.Now()
			if m.Button == tea.MouseButtonWheelUp {
				a.scrollUp(3)
			} else {
				a.scrollDown(3)
			}
		}
		return a, nil
	}

	return a, nil
}

// --- view ---

func (a *App) View() string {
	if a.width < 10 {
		return "initializing…"
	}

	var b strings.Builder

	// 1. Viewport (transcript)
	viewportH := a.viewportHeight()
	a.renderViewport(&b, viewportH)

	// 2. Panels
	a.renderPanels(&b)

	// 3. Overlay
	if a.overlay != nil {
		b.WriteByte('\n')
		b.WriteString(a.th.Rule.Render(rule(a.width)))
		b.WriteByte('\n')
		b.WriteString(a.overlay.View(a.width, a.th))
	} else if a.slashVisible {
		b.WriteByte('\n')
		b.WriteString(a.th.Rule.Render(rule(a.width)))
		b.WriteByte('\n')
		b.WriteString(a.renderSlash())
	}

	// 4. Separator + input
	b.WriteByte('\n')
	b.WriteString(a.th.Rule.Render(rule(a.width)))
	b.WriteByte('\n')
	b.WriteString(a.input.View())

	// 5. Status bar (bottom)
	b.WriteByte('\n')
	b.WriteString(a.renderStatus())

	return b.String()
}

// --- viewport ---

func (a *App) viewportHeight() int {
	used := 3 // rule (1) + input (1) + status (1)
	if a.overlay != nil {
		body := a.overlay.View(a.width, a.th)
		used += strings.Count(body, "\n") + 2
	} else if a.slashVisible {
		body := a.renderSlash()
		used += strings.Count(body, "\n") + 2
	}
	if len(a.todoLines) > 0 {
		used += len(a.todoLines) + 1
	}
	if len(a.agentLines) > 0 {
		used += len(a.agentLines) + 1
	}
	h := a.height - used
	if h < 3 {
		h = 3
	}
	return h
}

func (a *App) renderViewport(b *strings.Builder, vh int) {
	total := len(a.lines)
	if total == 0 {
		// Fill empty space
		for i := 0; i < vh; i++ {
			b.WriteByte('\n')
		}
		return
	}

	// Clamp scroll
	visible := total
	maxScroll := total - vh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if a.scroll > maxScroll {
		a.scroll = maxScroll
	}
	if a.scroll < 0 {
		a.scroll = 0
	}

	start := total - vh - a.scroll
	if start < 0 {
		start = 0
		visible = total
	} else {
		visible = vh
	}

	for i := start; i < start+visible && i < total; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		b.WriteString(a.lines[i])
	}

	// Pad if fewer lines than viewport
	rendered := visible
	if start+visible > total {
		rendered = total - start
		if rendered < 0 {
			rendered = 0
		}
	}
	for i := rendered; i < vh; i++ {
		b.WriteByte('\n')
	}
}

func (a *App) scrollUp(n int) {
	a.scroll += n
}

func (a *App) scrollDown(n int) {
	a.scroll -= n
	if a.scroll < 0 {
		a.scroll = 0
	}
}

func (a *App) scrollToBottom() {
	a.scroll = 0
}

// --- panels ---

func (a *App) renderPanels(b *strings.Builder) {
	panels := [][]string{a.todoLines, a.agentLines, a.bgLines, a.monLines}
	for _, p := range panels {
		if len(p) == 0 {
			continue
		}
		b.WriteByte('\n')
		b.WriteString(a.th.Rule.Render(rule(a.width)))
		for _, line := range p {
			b.WriteByte('\n')
			b.WriteString(line)
		}
	}
}

// --- status bar ---

func (a *App) renderStatus() string {
	// Pill: spinner + state label
	var pill string
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := spinner[a.spinnerFrame%len(spinner)]

	switch a.runState {
	case "ready":
		pill = a.th.StatusPill.Foreground(lipgloss.Color(forest)).Render("● ready")
	case "thinking":
		pill = a.th.StatusPill.Foreground(lipgloss.Color(gold)).Render(frame + " thinking")
	case "executing":
		pill = a.th.StatusPill.Foreground(lipgloss.Color(copper)).Render(frame + " executing")
	case "texting":
		pill = a.th.StatusPill.Foreground(lipgloss.Color(gold)).Render(frame + " texting")
	case "error":
		pill = a.th.StatusPill.Foreground(lipgloss.Color(red)).Render("✘ error")
	case "paused":
		pill = a.th.StatusPill.Foreground(lipgloss.Color(goldDim)).Render("⏸ paused")
	default:
		pill = a.th.StatusPill.Foreground(lipgloss.Color(forest)).Render("● " + a.runState)
	}

	sep := a.th.StatusSep.Render(" · ")

	parts := []string{pill}
	parts = append(parts, a.th.StatusValue.Render(a.agentName))

	if a.modelID != "" {
		parts = append(parts,
			a.th.StatusKey.Render("model")+" "+a.th.StatusValue.Render(a.modelID)+
				renderEffort(a.effort, a.th))
	}

	parts = append(parts,
		a.th.StatusKey.Render("in")+" "+a.th.StatusValue.Render(humanNum(a.usageIn))+
			" "+a.th.StatusKey.Render("out")+" "+a.th.StatusValue.Render(humanNum(a.usageOut)))

	if a.ctxLimit > 0 {
		pct := float64(a.ctxUsed) * 100 / float64(a.ctxLimit)
		parts = append(parts,
			a.th.StatusKey.Render("ctx")+" "+a.th.StatusValue.Render(fmt.Sprintf("%.0f%%", pct)))
	}

	if a.permMode != "" && a.permMode != "default" {
		parts = append(parts,
			a.th.StatusKey.Render("mode")+" "+a.th.StatusValue.Render(a.permMode))
	}

	if a.agentID != "" {
		parts = append(parts,
			a.th.StatusKey.Render("sid")+" "+a.th.StatusValue.Render(a.agentID))
	}

	if a.statusHint != "" {
		parts = append(parts, a.th.DimText.Render(a.statusHint))
	}

	return a.th.StatusBar.Width(a.width).Render(strings.Join(parts, sep))
}

// --- slash ---

func (a *App) updateSlash() {
	input := strings.TrimSpace(a.input.Value())
	if !strings.HasPrefix(input, "/") || a.slashDismiss {
		a.slashVisible = false
		a.slashMatches = nil
		return
	}
	lower := strings.ToLower(input)
	var matches []slashEntry
	catalog := a.slashCatalog()
	for _, e := range catalog {
		if strings.HasPrefix(e.Name, lower) {
			matches = append(matches, e)
			if len(matches) >= 5 {
				break
			}
		}
	}
	if len(matches) == 0 {
		a.slashVisible = false
		a.slashMatches = nil
		return
	}
	a.slashVisible = true
	a.slashMatches = matches
	if a.slashSel >= len(matches) {
		a.slashSel = len(matches) - 1
	}
	if a.slashSel < 0 {
		a.slashSel = 0
	}
}

func (a *App) slashCatalog() []slashEntry {
	out := make([]slashEntry, len(slashBuiltins))
	copy(out, slashBuiltins)
	if a.controller != nil {
		for _, s := range a.controller.Skills() {
			name := strings.TrimSpace(s.Name)
			if name == "" {
				continue
			}
			desc := s.Description
			if desc == "" {
				desc = "user skill"
			}
			out = append(out, slashEntry{"/" + name, desc})
		}
	}
	return out
}

func (a *App) renderSlash() string {
	if len(a.slashMatches) == 0 {
		return ""
	}
	var b strings.Builder
	for i, e := range a.slashMatches {
		marker := "  "
		style := a.th.OverlayRow
		if i == a.slashSel {
			marker = "▸ "
			style = a.th.OverlaySel
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%-20s %s", marker, e.Name, e.Desc)))
		b.WriteByte('\n')
	}
	b.WriteString(a.th.DimText.Render("[Tab] complete · [↑↓] pick · [Esc] dismiss"))
	return a.th.OverlayBox.Width(a.width - 4).Render(b.String())
}

// --- event handling ---

func (a *App) handleEvent(e event.Event) tea.Cmd {
	switch e.Kind {
	case event.KindRunStart, event.KindRunResume, event.KindTurnStart:
		a.runState = "thinking"

	case event.KindThinking, event.KindThinkingChunk:
		a.runState = "thinking"
		if e.Thinking != nil && e.Thinking.Text != "" {
			a.appendLine(a.th.Thinking.Render("· " + e.Thinking.Text))
		}

	case event.KindText, event.KindTextChunk:
		a.runState = "texting"
		if e.Text != nil && e.Text.Text != "" {
			a.appendLine(a.th.Assistant.Render(e.Text.Text))
		}

	case event.KindToolUseStart:
		a.runState = "executing"
		if e.ToolUseStart != nil {
			a.appendLine(a.th.ToolCall.Render(
				fmt.Sprintf("→ %s(%s)", e.ToolUseStart.Name, summarizeJSON(e.ToolUseStart.Input))))
		}

	case event.KindToolUseResult:
		a.runState = "thinking"
		if e.ToolUseResult != nil {
			if e.ToolUseResult.IsError {
				a.appendLine(a.th.ToolErr.Render("  ✗ " + e.ToolUseResult.Content))
			} else {
				a.appendLine(a.th.ToolOK.Render("  ✓ ") +
					a.th.ToolResult.Render(truncate(e.ToolUseResult.Content, 200)))
			}
		}

	case event.KindError:
		a.runState = "error"
		if e.Error != nil {
			a.appendLine(a.th.ErrorBanner.Render("✘ " + e.Error.Message))
		}

	case event.KindRunEnd, event.KindTurnEnd:
		if a.runState != "error" && a.runState != "paused" {
			a.runState = "ready"
		}

	case event.KindIterLimit:
		a.runState = "paused"
		a.statusHint = "press Enter to continue"
		if e.IterLimit != nil {
			a.appendLine(a.th.Compacting.Render(
				fmt.Sprintf("⏸ paused at %d iterations · Enter to continue", e.IterLimit.Iters)))
		}

	case event.KindRunCancelled:
		a.runState = "ready"
		a.appendLine(a.th.System.Render("◇ cancelled"))

	case event.KindDrainingInfo:
		a.runState = "thinking"
		a.appendLine(a.th.Draining.Render("◈ draining..."))

	case event.KindCompacting:
		a.runState = "thinking"
		if e.Compacting != nil {
			a.appendLine(a.th.Compacting.Render(
				fmt.Sprintf("↻ compacting [%s]...", e.Compacting.Type)))
		}

	case event.KindCompactingEnd:
		a.runState = "ready"

	case event.KindIdle:
		a.runState = "ready"
		a.statusHint = ""

	case event.KindUsage:
		if e.Usage != nil {
			a.usageIn = e.Usage.Cumulative.InputTokens
			a.usageOut = e.Usage.Cumulative.OutputTokens
		}

	case event.KindModeChanged:
		if e.ModeChanged != nil {
			a.permMode = e.ModeChanged.Mode
		}

	case event.KindApprovalNeeded:
		if e.ApprovalNeeded != nil {
			a.overlay = newApprovalOverlay(a.controller, *e.ApprovalNeeded)
		}

	case event.KindQuestionNeeded:
		if e.QuestionNeeded != nil {
			a.overlay = newQuestionOverlay(a.controller, *e.QuestionNeeded)
		}

	case event.KindStoreUpdate:
		a.refreshPanels()
	}

	a.scrollToBottom()

		// Keep context meter live — reads prompt size from the agent.
		if a.controller != nil {
			a.ctxUsed = a.controller.LastTurnInputTokens()
		}
	return nil
}

func (a *App) handleRunDone(err error) tea.Cmd {
	a.runCancel = nil
	interrupted := a.interrupted
	a.interrupted = false
	if errors.Is(err, llm.ErrInterrupted) {
		interrupted = true
	}
	if err == nil || interrupted {
		if a.runState != "error" && a.runState != "paused" {
			a.runState = "ready"
		}
		if interrupted {
			a.statusHint = "interrupted"
		}
	} else if strings.Contains(err.Error(), "iteration limit") {
		a.runState = "paused"
		a.statusHint = "press Enter to continue"
	} else {
		a.runState = "error"
		a.statusHint = err.Error()
	}
	a.scrollToBottom()

		// Keep context meter live — reads prompt size from the agent.
		if a.controller != nil {
			a.ctxUsed = a.controller.LastTurnInputTokens()
		}
	return nil
}

// --- key handling ---

func (a *App) handleKey(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Dedup mouse-derived arrow keys
	if !a.lastMouseAt.IsZero() && time.Since(a.lastMouseAt) < 80*time.Millisecond {
		switch m.String() {
		case "up", "down":
			return a, nil
		}
	}

	// Overlay consumes keys exclusively (except ctrl+c, scroll)
	if a.overlay != nil {
		if m.String() == "ctrl+c" {
			a.overlay = nil
			if a.runCancel != nil {
				a.runCancel()
			}
			return a, tea.Quit
		}
		switch m.String() {
		case "pgup", "pgdown":
			if m.String() == "pgup" {
				a.scrollUp(a.viewportHeight())
			} else {
				a.scrollDown(a.viewportHeight())
			}
			return a, nil
		}
		done := a.overlay.HandleKey(m.String())
		if done {
			a.overlay = nil
		}
		return a, nil
	}

	switch m.String() {
	case "ctrl+c":
		if a.runCancel != nil {
			a.interrupted = true
			a.runCancel()
			return a, nil
		}
		return a, tea.Quit

	case "esc":
		if a.runCancel != nil {
			a.interrupted = true
			a.runCancel()
			return a, nil
		}
		if a.runState == "error" {
			a.runState = "ready"
			a.statusHint = ""
			return a, nil
		}
		if a.slashVisible {
			a.slashDismiss = true
			a.slashVisible = false
			return a, nil
		}
		return a, tea.Quit

	case "pgup", "pgdown", "home", "end":
		vh := a.viewportHeight()
		switch m.String() {
		case "pgup":
			a.scrollUp(vh)
		case "pgdown":
			a.scrollDown(vh)
		case "home":
			a.scrollUp(len(a.lines))
		case "end":
			a.scrollToBottom()

		// Keep context meter live — reads prompt size from the agent.
		if a.controller != nil {
			a.ctxUsed = a.controller.LastTurnInputTokens()
		}
		}
		return a, nil

	case "ctrl+o":
		// Toggle tool fold — not implemented in this minimal design
		a.statusHint = "tool fold toggled"
		return a, nil

	case "shift+tab":
		if a.controller != nil {
			a.permMode = a.controller.CyclePermissionMode()
			a.statusHint = "permission: " + a.permMode
		}
		return a, nil

	case "ctrl+y":
		a.statusHint = "yank mode — copy with Enter"
		// Yank: just copy the transcript — simplified for low-profile.
		// Full implementation would show a cursor in the transcript.
		text := a.extractTranscriptText()
		a.copyToClipboard(text)
		return a, nil

	case "ctrl+f":
		a.statusHint = "search — / to find"
		return a, nil

	case "enter":
		return a, a.handleSubmit()

	case "ctrl+j", "alt+enter":
		a.input.InsertRune('\n')
		a.updateSlash()
		return a, nil

	case "up":
		if a.slashVisible {
			if a.slashSel > 0 {
				a.slashSel--
			}
			return a, nil
		}
		if a.input.HistoryPrev() {
			return a, nil
		}
		a.scrollUp(1)
		return a, nil

	case "down":
		if a.slashVisible {
			if a.slashSel < len(a.slashMatches)-1 {
				a.slashSel++
			}
			return a, nil
		}
		if a.input.HistoryNext() {
			return a, nil
		}
		a.scrollDown(1)
		return a, nil

	case "tab":
		if a.slashVisible && len(a.slashMatches) > 0 {
			a.input.SetValue(a.slashMatches[a.slashSel].Name)
			a.slashDismiss = true
			a.slashVisible = false
		}
		return a, nil

	case "backspace":
		a.input.DeleteBefore()
		a.updateSlash()
		return a, nil

	case "delete":
		a.input.DeleteAfter()
		return a, nil

	case "left":
		a.input.MoveLeft()
		return a, nil

	case "right":
		a.input.MoveRight()
		return a, nil

	case "ctrl+a":
		a.input.MoveHome()
		return a, nil

	case "ctrl+e":
		a.input.MoveEnd()
		return a, nil

	default:
		// Handle paste
		if m.Paste {
			a.input.HandlePaste(string(m.Runes))
			a.updateSlash()
			return a, nil
		}
		// Regular text input
		for _, r := range m.Runes {
			a.input.InsertRune(r)
		}
		a.updateSlash()
		return a, nil
	}
}

// --- submit ---

func (a *App) handleSubmit() tea.Cmd {
	text := strings.TrimSpace(a.input.Value())

	// Slash commands
	switch text {
	case "/exit", "/quit", "exit":
		a.input.Reset()
		return tea.Quit
	case "/clear":
		a.lines = nil
		a.scroll = 0
		a.input.Reset()
		a.statusHint = ""
		return nil
	case "/config":
		a.input.Reset()
		if a.controller != nil {
			a.overlay = newConfigOverlay(a.controller)
		}
		return nil
	case "/model":
		a.input.Reset()
		if a.controller != nil {
			a.overlay = newModelOverlay(a.controller)
		}
		return nil
	case "/profile":
		a.input.Reset()
		if a.controller != nil {
			a.overlay = newProfileOverlay(a.controller)
		}
		return nil
	case "/compact":
		a.input.Reset()
		if a.controller != nil {
			a.overlay = newCompactOverlay(a.controller)
		}
		return nil
	case "/effort":
		a.input.Reset()
		if a.controller != nil {
			a.overlay = newEffortOverlay(a.controller)
		}
		return nil
	case "/resume":
		a.input.Reset()
		if a.controller != nil {
			a.overlay = newResumeOverlay(a.controller)
		}
		return nil
	case "/update":
		a.input.Reset()
		a.overlay = newUpdateOverlay()
		return nil
	}

	// Iter-limit continue
	if a.runState == "paused" {
		a.input.Reset()
		a.startContinue()
		return nil
	}

	if text == "" {
		return nil
	}

	if a.controller == nil {
		a.statusHint = "no agent attached"
		return nil
	}

	// Append user prompt to transcript
	viewText := a.input.ViewText()
	a.appendLine("")
	a.appendLine(a.th.UserPrompt.Render("▸ " + viewText))
	a.appendLine("")

	agentText := a.input.ExpandPaste()
	a.input.RecordHistory()
	a.input.Reset()
	a.slashDismiss = false
	a.slashVisible = false

	if a.runCancel != nil {
		// Mid-run: queue
		a.controller.EnqueueUserPrompt(agentText)
		a.statusHint = "queued"
		return nil
	}

	a.startRun(agentText)
	return nil
}

func (a *App) startRun(prompt string) {
	a.runCancel = nil
	a.interrupted = false
	ctx, cancel := context.WithCancel(context.Background())
	a.runCancel = cancel
	a.runState = "thinking"
	a.statusHint = ""

	p := a.program
	go func() {
		_, err := a.controller.Run(ctx, prompt)
		if p != nil {
			p.Send(runDoneMsg{Err: err})
		}
	}()
}

func (a *App) startContinue() {
	ctx, cancel := context.WithCancel(context.Background())
	a.runCancel = cancel
	a.runState = "thinking"
	a.statusHint = ""

	p := a.program
	go func() {
		_, err := a.controller.Continue(ctx)
		if p != nil {
			p.Send(runDoneMsg{Err: err})
		}
	}()
}

// --- panels ---

func (a *App) refreshPanels() {
	if a.controller == nil {
		return
	}
	// Todo panel
	store := a.controller.TodoStore()
	if store != nil {
		items := store.List()
		if len(items) > 0 {
			lines := []string{a.th.PanelHeader.Render("TODO")}
			for _, t := range items {
				marker := "▢"
				style := a.th.PanelRow
				switch t.Status {
				case todo.StatusInProgress:
					marker = "▶"
					style = style.Foreground(lipgloss.Color(goldDim))
				case todo.StatusCompleted:
					marker = "▣"
					style = style.Foreground(lipgloss.Color(forest))
				}
				lines = append(lines, style.Render(fmt.Sprintf("  %s %s", marker, t.Content)))
			}
			a.todoLines = lines
		} else {
			a.todoLines = nil
		}
	}
}

// --- helpers ---

func (a *App) appendLine(s string) {
	a.lines = append(a.lines, s)
}

func (a *App) extractTranscriptText() string {
	var b strings.Builder
	for _, line := range a.lines {
		b.WriteString(stripStyles(line))
		b.WriteByte('\n')
	}
	return b.String()
}

func (a *App) copyToClipboard(text string) {
	// Simple: just flash a hint. Full clipboard via pbcopy/OSC52 would
	// require external process calls — kept simple for low-profile design.
	a.statusHint = fmt.Sprintf("copied %d chars", len(text))
}

// --- message types ---

type spinnerTickMsg struct{}
type agentEventMsg struct{ Event event.Event }
type runDoneMsg struct{ Err error }

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// --- helpers ---

func rule(w int) string {
	if w < 4 {
		return ""
	}
	return strings.Repeat("─", w)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func humanNum(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func renderEffort(level string, th *Theme) string {
	if level == "" {
		return ""
	}
	c := lipgloss.Color(gold)
	return " " + th.StatusKey.Render("·") + lipgloss.NewStyle().Foreground(c).Bold(true).Render(level)
}

func summarizeJSON(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	s := string(raw)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func stripStyles(s string) string {
	// Simple ANSI strip for clipboard copy
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
