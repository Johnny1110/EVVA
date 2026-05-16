package bubbletea

import "github.com/charmbracelet/lipgloss"

// NEON TOKYO palette — 24-bit truecolor. Primary chrome is electric
// cyan + cool grey + electric violet; red is reserved exclusively for
// real fault signals (tool errors, removed diff lines, error banners,
// crushed/interrupted subagents). Hot pink is intentionally kept off
// most surfaces — a panel header in red-adjacent hue reads as "this
// is broken" even when nothing is wrong, which the user found jarring.
//
// Every lipgloss color in the TUI maps to one of these constants so
// the theme has a single source of truth. Truecolor is unconditional —
// evva targets modern terminals.
const (
	// Surface
	paletteBg  lipgloss.Color = "#0A0A14" // abyssal navy — terminal bg
	paletteFg  lipgloss.Color = "#E2E2FF" // cool fog white
	paletteDim lipgloss.Color = "#1B1B2F" // panel dim

	// Cool grey scale — used for chrome that should sit back, not shout.
	paletteMuted lipgloss.Color = "#7A7E94" // slightly brightened so DimText / footer / gutter remain legible
	paletteThink lipgloss.Color = "#5E627A" // thinking block — dim cool grey, "agent's secret muttered to itself" feel; legible but quiet enough to feel overheard

	// Primary accents (duotone — cyan-led, violet-supported)
	paletteCyan   lipgloss.Color = "#05D9E8" // electric cyan — primary chrome
	palettePurple lipgloss.Color = "#B967FF" // electric violet — secondary accent / separators / paste chip

	// Supporting hues — used only for the lifecycle vocabulary they
	// represent, never as plain chrome
	paletteBrown     lipgloss.Color = "#B87333" // copper brown — tool calls / executing (was citrus orange; brown reads "warm + earthy" without competing with the neon chrome)
	paletteSky       lipgloss.Color = "#69B4FF" // soft sky blue — tool result content; lighter than primary cyan so the brown call line stays the heading and the result reads as a quieter output stream
	paletteYellow    lipgloss.Color = "#FAFC4E" // sulfur — compacting / paused
	paletteGreen     lipgloss.Color = "#39FF14" // acid green — success / diff add
	paletteRed       lipgloss.Color = "#FF003C" // glitch red — ERRORS ONLY (tool err / diff remove / crushed / interrupted)
	paletteLightBlue lipgloss.Color = "#7DF9FF" // cyan glow — thinking spinner

	// Diff line backgrounds — 淡綠 / 淡紅: desaturated mid-pastel
	// green / red. Visible enough to read as colored bars across the
	// transcript without being so bright that they fight the rest of
	// the chrome. Foreground keeps the bright neon green / red so the
	// text still pops.
	paletteAddBg    lipgloss.Color = "#5A8B6B" // 淡綠 — soft pale green
	paletteRemoveBg lipgloss.Color = "#8B5A65" // 淡紅 — soft pale red

	// Hot pink kept on the palette but used only when an accent must
	// genuinely pop (greeting flourish, paste chip optional). NOT on
	// chrome surfaces.
	paletteMagenta lipgloss.Color = "#FF2A6D"

	// Cursor: cyan glow — typing point reads as "live input channel"
	paletteCursor lipgloss.Color = "#05D9E8"

	// paletteWhite is retained as an alias of fg for any consumer that
	// wants to spell intent explicitly.
	paletteWhite lipgloss.Color = "#E2E2FF"

	// paletteBlue is kept for any future "info" cell that wants a
	// distinct blue from cyan — currently unused on surfaces.
	paletteBlue lipgloss.Color = "#5D5FEF"
)

// styles is the central style table. Each style is built once at
// package init time. Bold is used liberally on accent cells — neon
// palettes need the extra glyph weight to read as "glowing" on a
// dark surface.
var styles = struct {
	UserPrompt  lipgloss.Style
	Assistant   lipgloss.Style
	Thinking    lipgloss.Style
	ToolCall    lipgloss.Style
	ToolOK      lipgloss.Style
	ToolErr     lipgloss.Style
	ToolResult  lipgloss.Style
	DiffAdd     lipgloss.Style
	DiffRemove  lipgloss.Style
	DiffContext lipgloss.Style
	DiffHeader  lipgloss.Style
	PanelHeader lipgloss.Style
	PanelRow    lipgloss.Style
	PanelBorder lipgloss.Style
	StatusBar   lipgloss.Style
	StatusKey   lipgloss.Style
	StatusValue lipgloss.Style
	StatusSep   lipgloss.Style
	StatePill   lipgloss.Style
	ErrorBanner lipgloss.Style
	InputBorder lipgloss.Style
	DimText     lipgloss.Style
	Banner      lipgloss.Style
	BannerBox   lipgloss.Style
	BannerInfo  lipgloss.Style
	Greeting    lipgloss.Style
	Compacting  lipgloss.Style
	Draining    lipgloss.Style
	FooterHint  lipgloss.Style
	TasksDone   lipgloss.Style
	PasteChip   lipgloss.Style
	Timeline    lipgloss.Style
	TimelineCut lipgloss.Style
	ContextBar  lipgloss.Style
	ContextFill lipgloss.Style
	ContextRail lipgloss.Style
	SpinThink   lipgloss.Style
	SpinExec    lipgloss.Style
}{
	// Chat blocks
	UserPrompt: lipgloss.NewStyle().Foreground(paletteCyan).Bold(true),
	Assistant:  lipgloss.NewStyle().Foreground(paletteFg),
	// Thinking is intentionally quiet: cool light grey italic, no neon.
	// Reads as an internal aside — not the model "shouting at us".
	Thinking: lipgloss.NewStyle().Foreground(paletteThink).Italic(true),

	// Tool vocabulary — brown call, green success, RED reserved for
	// real failures only
	ToolCall:   lipgloss.NewStyle().Foreground(paletteBrown).Bold(true),
	ToolOK:     lipgloss.NewStyle().Foreground(paletteGreen).Bold(true),
	ToolErr:    lipgloss.NewStyle().Foreground(paletteRed).Bold(true),
	// Tool result content: soft sky blue — sits beneath the brown call
	// line as a calm, readable output stream. Diff add/remove keep
	// their dedicated green/red; this only re-themes plain tool result
	// text.
	ToolResult: lipgloss.NewStyle().Foreground(paletteSky),

	// Diff — soft pale green / red bg with bright neon foreground.
	// The hue match between fg and bg makes each row read as a
	// "colored block with neon outline" — recognizable as add/remove
	// without darkening the row into the rest of the chrome. Fill
	// spans the full transcript width via per-row Width() at render
	// time (see renderFileDiff).
	DiffAdd:    lipgloss.NewStyle().Foreground(paletteGreen).Background(paletteAddBg).Bold(true),
	DiffRemove: lipgloss.NewStyle().Foreground(paletteRed).Background(paletteRemoveBg).Bold(true),
	DiffContext: lipgloss.NewStyle().Foreground(paletteMuted),
	DiffHeader:  lipgloss.NewStyle().Foreground(palettePurple).Italic(true),

	// Side panels — cyan headers (was hot pink), neutral content
	PanelHeader: lipgloss.NewStyle().Foreground(paletteCyan).Bold(true),
	PanelRow:    lipgloss.NewStyle().Foreground(paletteFg),
	PanelBorder: lipgloss.NewStyle().Foreground(paletteDim),

	// Status bar — neon HUD, separator violet (was hot pink)
	StatusBar:   lipgloss.NewStyle().Foreground(paletteFg).Background(paletteDim).Padding(0, 1),
	StatusKey:   lipgloss.NewStyle().Foreground(paletteMuted),
	StatusValue: lipgloss.NewStyle().Foreground(paletteFg).Bold(true),
	StatusSep:   lipgloss.NewStyle().Foreground(palettePurple).Bold(true),
	StatePill:   lipgloss.NewStyle().Bold(true),

	// Errors / input / dims — input border now cyan (was hot pink)
	ErrorBanner: lipgloss.NewStyle().Foreground(paletteRed).Bold(true),
	InputBorder: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(paletteCyan).
		Padding(0, 1),
	DimText: lipgloss.NewStyle().Foreground(paletteMuted),

	// Welcome banner — cyan ASCII art, violet border (was hot pink)
	Banner: lipgloss.NewStyle().Foreground(paletteCyan).Bold(true),
	BannerBox: lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(palettePurple).
		Padding(1, 4),
	BannerInfo: lipgloss.NewStyle().Foreground(paletteCyan),
	Greeting:   lipgloss.NewStyle().Foreground(paletteCyan).Bold(true).Italic(true),

	// System lifecycle blocks
	Compacting: lipgloss.NewStyle().Foreground(paletteYellow).Bold(true),
	Draining:   lipgloss.NewStyle().Foreground(palettePurple).Bold(true),
	FooterHint: lipgloss.NewStyle().Foreground(paletteMuted),

	// Tasks / paste / timeline — timeline now sits back as muted grey
	// (was hot pink, which made every transcript line scream "error")
	TasksDone:   lipgloss.NewStyle().Foreground(paletteGreen).Bold(true),
	PasteChip:   lipgloss.NewStyle().Foreground(palettePurple).Italic(true),
	Timeline:    lipgloss.NewStyle().Foreground(paletteMuted),
	TimelineCut: lipgloss.NewStyle().Foreground(palettePurple).Bold(true),

	// Context HUD
	ContextBar:  lipgloss.NewStyle().Foreground(paletteMuted),
	ContextFill: lipgloss.NewStyle().Foreground(paletteCyan).Bold(true),
	ContextRail: lipgloss.NewStyle().Foreground(paletteDim),

	// Spinner color overrides — match the state's neon
	SpinThink: lipgloss.NewStyle().Foreground(paletteLightBlue).Bold(true),
	SpinExec:  lipgloss.NewStyle().Foreground(paletteBrown).Bold(true),
}
