package tui

import "github.com/charmbracelet/lipgloss"

// Styles is a small palette shared across tabs.
// Brand palette. The accent color matches the blue-violet that bubbles list's
// default title bar uses (ANSI 256 color 62 ≈ #5F5FD7) — visually distinct
// against most terminal backgrounds without being so dark that it reads as
// black. Kept centralized so tabs, list rows, and the modal border share it.
var (
	colorIndigo    = lipgloss.Color("#5F5FD7")
	colorIndigoDim = lipgloss.Color("#2D2D6B") // darker shade for inactive panes
	colorWhite     = lipgloss.Color("#FFFFFF")
)

var Styles = struct {
	Title       lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	Footer      lipgloss.Style
	Pane        lipgloss.Style
	PaneActive  lipgloss.Style
	Muted       lipgloss.Style
	OK          lipgloss.Style
	Warn        lipgloss.Style
	Err         lipgloss.Style
	Key         lipgloss.Style
	Header      lipgloss.Style
	Selected    lipgloss.Style
}{
	Title:       lipgloss.NewStyle().Bold(true).Foreground(colorIndigo),
	TabActive:   lipgloss.NewStyle().Bold(true).Foreground(colorWhite).Background(colorIndigo).Padding(0, 1),
	TabInactive: lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1),
	Footer:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	Pane:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	PaneActive:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorIndigo).Padding(0, 1),
	Muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	OK:          lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
	Warn:        lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
	Err:         lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
	Key:         lipgloss.NewStyle().Bold(true).Foreground(colorIndigo),
	Header:      lipgloss.NewStyle().Bold(true).Underline(true),
	Selected:    lipgloss.NewStyle().Bold(true).Foreground(colorWhite).Background(colorIndigo),
}
