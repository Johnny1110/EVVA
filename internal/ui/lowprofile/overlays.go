package lowprofile

import (
	"context"
	"fmt"
	"strings"

	"github.com/johnny1110/evva/pkg/event"
	"github.com/johnny1110/evva/pkg/ui"
)

// overlay is the interface every modal panel implements.
type overlay interface {
	View(width int, th *Theme) string
	HandleKey(key string) (done bool)
}

// --- approval overlay ---

type approvalOverlay struct {
	ctrl    ui.Controller
	payload event.ApprovalNeededPayload
	choice  int // 0=deny, 1=allow once, 2=allow session
}

func newApprovalOverlay(ctrl ui.Controller, p event.ApprovalNeededPayload) *approvalOverlay {
	return &approvalOverlay{ctrl: ctrl, payload: p}
}

func (o *approvalOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("⚑ Permission Required"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(th.OverlayRow.Render(fmt.Sprintf("Tool: %s", o.payload.ToolName)))
	b.WriteByte('\n')
	if o.payload.InputDescription != "" {
		b.WriteString(th.DimText.Render(truncate(o.payload.InputDescription, 120)))
		b.WriteByte('\n')
	}
	if o.payload.Reason != "" {
		b.WriteString(th.DimText.Render("Reason: " + o.payload.Reason))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	choices := []string{"Deny", "Allow once", "Allow this session"}
	for i, c := range choices {
		marker := "  "
		style := th.OverlayRow
		if i == o.choice {
			marker = "▸ "
			style = th.OverlaySel
		}
		b.WriteString(style.Render(marker + c))
		b.WriteByte('\n')
	}
	b.WriteString(th.DimText.Render("[↑↓] pick · [Enter] confirm"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *approvalOverlay) HandleKey(key string) bool {
	switch key {
	case "up":
		if o.choice > 0 {
			o.choice--
		}
	case "down":
		if o.choice < 2 {
			o.choice++
		}
	case "enter":
		behaviors := []string{"deny", "allow", "allow"}
		_ = o.ctrl.RespondPermission(o.payload.RequestID, ui.PermissionDecision{
			Behavior: behaviors[o.choice],
			Reason:   "user selected",
		})
		return true
	case "esc":
		_ = o.ctrl.RespondPermission(o.payload.RequestID, ui.PermissionDecision{
			Behavior: "deny",
			Reason:   "dismissed",
		})
		return true
	}
	return false
}

// --- question overlay ---

type questionOverlay struct {
	ctrl    ui.Controller
	payload event.QuestionNeededPayload
	answers map[string]string
	focus   int // which question is focused
}

func newQuestionOverlay(ctrl ui.Controller, p event.QuestionNeededPayload) *questionOverlay {
	return &questionOverlay{ctrl: ctrl, payload: p, answers: map[string]string{}}
}

func (o *questionOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("? Question"))
	b.WriteByte('\n')
	for i, q := range o.payload.Questions {
		b.WriteByte('\n')
		prefix := "  "
		if i == o.focus {
			prefix = "▸ "
		}
		b.WriteString(th.OverlaySel.Render(prefix + q.Question))
		b.WriteByte('\n')
		for _, opt := range q.Options {
			b.WriteString(th.DimText.Render("    · " + opt.Label))
			b.WriteByte('\n')
		}
	}
	b.WriteString(th.DimText.Render("[Enter] submit · [Esc] cancel"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *questionOverlay) HandleKey(key string) bool {
	switch key {
	case "up":
		if o.focus > 0 {
			o.focus--
		}
	case "down":
		if o.focus < len(o.payload.Questions)-1 {
			o.focus++
		}
	case "enter":
		resp := ui.QuestionResponse{Answers: make(map[string]string)}
		for _, q := range o.payload.Questions {
			if len(q.Options) > 0 {
				resp.Answers[q.Question] = q.Options[0].Label
			}
		}
		_ = o.ctrl.RespondQuestion(o.payload.RequestID, resp)
		return true
	case "esc":
		_ = o.ctrl.RespondQuestion(o.payload.RequestID, ui.QuestionResponse{})
		return true
	}
	return false
}

// --- config overlay ---

type configOverlay struct {
	ctrl ui.Controller
}

func newConfigOverlay(ctrl ui.Controller) *configOverlay {
	return &configOverlay{ctrl: ctrl}
}

func (o *configOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("⚙ Config"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("Use /config in the input to edit settings."))
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("Available: max_iterations, permission_mode"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("[Esc] close"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *configOverlay) HandleKey(key string) bool {
	return key == "esc" || key == "enter"
}

// --- model overlay ---

type modelOverlay struct {
	ctrl ui.Controller
	idx  int
}

func newModelOverlay(ctrl ui.Controller) *modelOverlay {
	return &modelOverlay{ctrl: ctrl}
}

func (o *modelOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("⚡ Switch Model"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("Use /model in the input to switch providers."))
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("Current: " + o.ctrl.Model()))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("[Esc] close"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *modelOverlay) HandleKey(key string) bool {
	return key == "esc" || key == "enter"
}

// --- profile overlay ---

type profileOverlay struct {
	ctrl   ui.Controller
	idx    int
	choices []ui.ProfileChoice
}

func newProfileOverlay(ctrl ui.Controller) *profileOverlay {
	return &profileOverlay{ctrl: ctrl, choices: ctrl.ListMainProfiles()}
}

func (o *profileOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("👤 Switch Profile"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	for i, c := range o.choices {
		marker := "  "
		style := th.OverlayRow
		if i == o.idx {
			marker = "▸ "
			style = th.OverlaySel
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%-20s %s", marker, c.Name, c.WhenToUse)))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("[↑↓] pick · [Enter] switch · [Esc] cancel"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *profileOverlay) HandleKey(key string) bool {
	switch key {
	case "up":
		if o.idx > 0 {
			o.idx--
		}
	case "down":
		if o.idx < len(o.choices)-1 {
			o.idx++
		}
	case "enter":
		if o.idx < len(o.choices) {
			_ = o.ctrl.SwitchProfile(o.choices[o.idx].Name)
		}
		return true
	case "esc":
		return true
	}
	return false
}

// --- compact overlay ---

type compactOverlay struct {
	ctrl ui.Controller
	kind int // 0=micro, 1=full
}

func newCompactOverlay(ctrl ui.Controller) *compactOverlay {
	return &compactOverlay{ctrl: ctrl}
}

func (o *compactOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("↻ Compact"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	kinds := []string{"micro", "full"}
	for i, k := range kinds {
		marker := "  "
		style := th.OverlayRow
		if i == o.kind {
			marker = "▸ "
			style = th.OverlaySel
		}
		b.WriteString(style.Render(marker + k))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("[↑↓] pick · [Enter] run · [Esc] cancel"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *compactOverlay) HandleKey(key string) bool {
	switch key {
	case "up":
		if o.kind > 0 {
			o.kind--
		}
	case "down":
		if o.kind < 1 {
			o.kind++
		}
	case "enter":
		kinds := []string{"micro", "full"}
		_ = o.ctrl.Compact(context.Background(), kinds[o.kind])
		return true
	case "esc":
		return true
	}
	return false
}

// --- effort overlay ---

type effortOverlay struct {
	ctrl  ui.Controller
	level int // 0=low, 1=medium, 2=high, 3=ultra
}

func newEffortOverlay(ctrl ui.Controller) *effortOverlay {
	e := &effortOverlay{ctrl: ctrl}
	switch ctrl.Effort() {
	case "medium":
		e.level = 1
	case "high":
		e.level = 2
	case "ultra":
		e.level = 3
	}
	return e
}

func (o *effortOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("🔥 Effort Level"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	levels := []string{"low", "medium", "high", "ultra"}
	for i, l := range levels {
		marker := "  "
		style := th.OverlayRow
		if i == o.level {
			marker = "▸ "
			style = th.OverlaySel
		}
		b.WriteString(style.Render(marker + l))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("[↑↓] pick · [Enter] set · [Esc] cancel"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *effortOverlay) HandleKey(key string) bool {
	switch key {
	case "up":
		if o.level > 0 {
			o.level--
		}
	case "down":
		if o.level < 3 {
			o.level++
		}
	case "enter":
		levels := []string{"low", "medium", "high", "ultra"}
		_ = o.ctrl.SetEffort(levels[o.level])
		return true
	case "esc":
		return true
	}
	return false
}

// --- resume overlay ---

type resumeOverlay struct {
	ctrl    ui.Controller
	idx     int
	entries []ui.SessionInfo
}

func newResumeOverlay(ctrl ui.Controller) *resumeOverlay {
	entries, _ := ctrl.ListSessions()
	return &resumeOverlay{ctrl: ctrl, entries: entries}
}

func (o *resumeOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("📋 Resume Session"))
	b.WriteByte('\n')
	if len(o.entries) == 0 {
		b.WriteByte('\n')
		b.WriteString(th.DimText.Render("  No saved sessions."))
	} else {
		b.WriteByte('\n')
		for i, e := range o.entries {
			marker := "  "
			style := th.OverlayRow
			if i == o.idx {
				marker = "▸ "
				style = th.OverlaySel
			}
			preview := truncate(e.FirstUserPrompt, 60)
			b.WriteString(style.Render(fmt.Sprintf("%s%s — %s", marker, e.Profile, preview)))
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("[↑↓] pick · [Enter] resume · [Esc] cancel"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *resumeOverlay) HandleKey(key string) bool {
	switch key {
	case "up":
		if o.idx > 0 {
			o.idx--
		}
	case "down":
		if o.idx < len(o.entries)-1 {
			o.idx++
		}
	case "enter":
		if o.idx < len(o.entries) {
			_ = o.ctrl.ResumeSession(o.entries[o.idx].ID)
		}
		return true
	case "esc":
		return true
	}
	return false
}

// --- update overlay ---

type updateOverlay struct{}

func newUpdateOverlay() *updateOverlay {
	return &updateOverlay{}
}

func (o *updateOverlay) View(width int, th *Theme) string {
	var b strings.Builder
	b.WriteString(th.OverlayTitle.Render("⬆ Update"))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("Run evva update from the command line to check for updates."))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(th.DimText.Render("[Esc] close"))
	return th.OverlayBox.Width(width - 4).Render(b.String())
}

func (o *updateOverlay) HandleKey(key string) bool {
	return key == "esc" || key == "enter"
}
