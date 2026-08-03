// Package tui implements the step-by-step wizard.
//
// The flow is a single Bubble Tea program driving a list of Steps. Each step
// owns its own state and writes its answer back into the shared model.Spec, so
// going back and forth never loses an answer.
package tui

import "github.com/charmbracelet/lipgloss"

// Palette. Adaptive colours keep the wizard readable on light and dark
// terminals without asking which one is in use.
var (
	colAccent = lipgloss.AdaptiveColor{Light: "#3B5BDB", Dark: "#91A7FF"}
	colMuted  = lipgloss.AdaptiveColor{Light: "#6C757D", Dark: "#909296"}
	colOK     = lipgloss.AdaptiveColor{Light: "#2B8A3E", Dark: "#8CE99A"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#E67700", Dark: "#FFD43B"}
	colErr    = lipgloss.AdaptiveColor{Light: "#C92A2A", Dark: "#FF8787"}
	colText   = lipgloss.AdaptiveColor{Light: "#212529", Dark: "#E9ECEF"}
)

var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleStep  = lipgloss.NewStyle().Foreground(colMuted)
	styleHelp  = lipgloss.NewStyle().Foreground(colMuted)
	styleErr   = lipgloss.NewStyle().Foreground(colErr).Bold(true)
	styleWarn  = lipgloss.NewStyle().Foreground(colWarn)
	styleOK    = lipgloss.NewStyle().Foreground(colOK)
	styleMuted = lipgloss.NewStyle().Foreground(colMuted)
	styleText  = lipgloss.NewStyle().Foreground(colText)
	styleFocus = lipgloss.NewStyle().Foreground(colAccent).Bold(true)

	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colMuted).
			Padding(0, 1)
)

// Glyphs. Plain ASCII fallbacks are not attempted: every terminal that can run
// a Bubble Tea program can render these.
const (
	glyphCursor    = "›"
	glyphChecked   = "◉"
	glyphUnchecked = "○"
	glyphRadioOn   = "●"
	glyphRadioOff  = "○"
	glyphDone      = "✔"
	glyphWarn      = "!"
	glyphInfo      = "·"
)

// Banner is the wizard header, shown once at the top of every screen.
func Banner(title string) string {
	return styleTitle.Render(title)
}
