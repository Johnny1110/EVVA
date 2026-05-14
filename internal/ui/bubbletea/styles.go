package bubbletea

import "github.com/charmbracelet/lipgloss"

// styles is the central palette. v1 is deliberately minimal — every color
// is a single 8-bit ANSI value so the UI looks the same in every terminal
// (no 24-bit truecolor required). A future theme system would replace
// these constants with a struct that can be swapped at runtime.
var styles = struct {
	UserPrompt    lipgloss.Style
	Assistant     lipgloss.Style
	Thinking      lipgloss.Style
	ToolCall      lipgloss.Style
	ToolOK        lipgloss.Style
	ToolErr       lipgloss.Style
	DiffAdd       lipgloss.Style
	DiffRemove    lipgloss.Style
	DiffContext   lipgloss.Style
	DiffHeader    lipgloss.Style
	PanelHeader   lipgloss.Style
	PanelRow      lipgloss.Style
	StatusBar     lipgloss.Style
	StatusKey     lipgloss.Style
	StatusValue   lipgloss.Style
	ErrorBanner   lipgloss.Style
	InputBorder   lipgloss.Style
	DimText       lipgloss.Style
}{
	UserPrompt:  lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	Assistant:   lipgloss.NewStyle(),
	Thinking:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true),
	ToolCall:    lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
	ToolOK:      lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
	ToolErr:     lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
	DiffAdd:     lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
	DiffRemove:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
	DiffContext: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	DiffHeader:  lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true),
	PanelHeader: lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true),
	PanelRow:    lipgloss.NewStyle(),
	StatusBar:   lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252")).Padding(0, 1),
	StatusKey:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	StatusValue: lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true),
	ErrorBanner: lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
	InputBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
	DimText:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
}
