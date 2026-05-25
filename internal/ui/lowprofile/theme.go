package lowprofile

import "github.com/charmbracelet/lipgloss"

// Black & gold palette — muted, warm, premium.
const (
	bg      = "#0E0E0E" // deep charcoal
	fg      = "#D4C5A9" // warm parchment
	surface = "#1C1C1C" // dark panel
	muted   = "#6B6358" // warm grey
	dim     = "#4A453A" // darker grey

	gold      = "#C9A94E" // rich gold — primary accent
	goldLight = "#E0C870" // bright gold
	goldDim   = "#7A6B3A" // muted gold

	copper = "#B87333" // tool calls
	sage   = "#8FA87A" // tool results
	forest = "#5B8C3E" // success
	red    = "#C23B3B" // errors
	tawny  = "#8B7355" // secondary

	diffAddBg    = "#1A2214"
	diffRemoveBg = "#2A1414"
)

// Theme holds every styled surface. One instance, passed by pointer.
type Theme struct {
	// Transcript blocks
	UserPrompt lipgloss.Style
	Assistant  lipgloss.Style
	Thinking   lipgloss.Style
	ToolCall   lipgloss.Style
	ToolOK     lipgloss.Style
	ToolErr    lipgloss.Style
	ToolResult lipgloss.Style
	ErrorBanner lipgloss.Style
	System     lipgloss.Style
	Compacting lipgloss.Style
	Draining   lipgloss.Style

	// Diff
	DiffAdd     lipgloss.Style
	DiffRemove  lipgloss.Style
	DiffContext lipgloss.Style
	DiffHeader  lipgloss.Style

	// Status bar
	StatusBar   lipgloss.Style
	StatusKey   lipgloss.Style
	StatusValue lipgloss.Style
	StatusSep   lipgloss.Style
	StatusPill  lipgloss.Style

	// Overlays
	OverlayBox   lipgloss.Style
	OverlayTitle lipgloss.Style
	OverlayRow   lipgloss.Style
	OverlaySel   lipgloss.Style

	// Input
	InputPrompt lipgloss.Style
	InputText   lipgloss.Style
	InputCursor lipgloss.Style

	// Chrome
	Rule      lipgloss.Style
	DimText   lipgloss.Style
	PasteChip lipgloss.Style

	// Panels
	PanelHeader lipgloss.Style
	PanelRow    lipgloss.Style
}

// NewTheme returns a fresh black & gold theme.
func NewTheme() *Theme {
	t := &Theme{}

	t.UserPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(gold)).Bold(true)
	t.Assistant = lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	t.Thinking = lipgloss.NewStyle().Foreground(lipgloss.Color(dim)).Italic(true)
	t.ToolCall = lipgloss.NewStyle().Foreground(lipgloss.Color(copper)).Bold(true)
	t.ToolOK = lipgloss.NewStyle().Foreground(lipgloss.Color(forest)).Bold(true)
	t.ToolErr = lipgloss.NewStyle().Foreground(lipgloss.Color(red)).Bold(true)
	t.ToolResult = lipgloss.NewStyle().Foreground(lipgloss.Color(sage))
	t.ErrorBanner = lipgloss.NewStyle().Foreground(lipgloss.Color(red)).Bold(true)
	t.System = lipgloss.NewStyle().Foreground(lipgloss.Color(dim))
	t.Compacting = lipgloss.NewStyle().Foreground(lipgloss.Color(goldDim)).Bold(true)
	t.Draining = lipgloss.NewStyle().Foreground(lipgloss.Color(tawny)).Bold(true)

	t.DiffAdd = lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(diffAddBg)).Bold(true)
	t.DiffRemove = lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(diffRemoveBg)).Bold(true)
	t.DiffContext = lipgloss.NewStyle().Foreground(lipgloss.Color(muted))
	t.DiffHeader = lipgloss.NewStyle().Foreground(lipgloss.Color(tawny)).Italic(true)

	t.StatusBar = lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(surface)).Padding(0, 1)
	t.StatusKey = lipgloss.NewStyle().Foreground(lipgloss.Color(muted))
	t.StatusValue = lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Bold(true)
	t.StatusSep = lipgloss.NewStyle().Foreground(lipgloss.Color(tawny)).Bold(true)
	t.StatusPill = lipgloss.NewStyle().Bold(true)

	t.OverlayBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(gold)).
		Padding(1, 2)
	t.OverlayTitle = lipgloss.NewStyle().Foreground(lipgloss.Color(gold)).Bold(true)
	t.OverlayRow = lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	t.OverlaySel = lipgloss.NewStyle().Foreground(lipgloss.Color(gold)).Bold(true)

	t.InputPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(gold)).Bold(true)
	t.InputText = lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	t.InputCursor = lipgloss.NewStyle().Foreground(lipgloss.Color(gold))

	t.Rule = lipgloss.NewStyle().Foreground(lipgloss.Color(dim))
	t.DimText = lipgloss.NewStyle().Foreground(lipgloss.Color(muted))
	t.PasteChip = lipgloss.NewStyle().Foreground(lipgloss.Color(tawny)).Italic(true)

	t.PanelHeader = lipgloss.NewStyle().Foreground(lipgloss.Color(gold)).Bold(true)
	t.PanelRow = lipgloss.NewStyle().Foreground(lipgloss.Color(fg))

	return t
}
