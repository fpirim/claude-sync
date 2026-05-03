package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Modal is a centered dialog overlay. While the root holds an active modal,
// all key input is routed to it; global shortcuts (q, r, tab, 1/2/4) are
// suppressed so users can type freely. Esc always cancels.
type Modal struct {
	Kind   modalKind
	Title  string
	Prompt string
	Input  textinput.Model

	// OnSubmit is dispatched when the user confirms an input modal.
	OnSubmit func(value string) tea.Cmd
	// OnConfirm is dispatched for confirm modals; argument true=yes, false=no.
	OnConfirm func(yes bool) tea.Cmd
}

type modalKind int

const (
	modalInput modalKind = iota
	modalConfirm
	modalInfo
)

// openModalMsg asks the root to install a modal.
type openModalMsg struct{ M Modal }

// closeModalMsg asks the root to dismiss without firing callbacks.
type closeModalMsg struct{}

// NewInputModal constructs an input dialog with optional default value.
func NewInputModal(title, prompt, placeholder, defaultValue string, onSubmit func(string) tea.Cmd) Modal {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(defaultValue)
	ti.CursorEnd()
	ti.CharLimit = 256
	ti.Width = 40
	return Modal{
		Kind:     modalInput,
		Title:    title,
		Prompt:   prompt,
		Input:    ti,
		OnSubmit: onSubmit,
	}
}

// NewConfirmModal constructs a yes/no dialog.
func NewConfirmModal(title, prompt string, onConfirm func(bool) tea.Cmd) Modal {
	return Modal{
		Kind:      modalConfirm,
		Title:     title,
		Prompt:    prompt,
		OnConfirm: onConfirm,
	}
}

// NewInfoModal constructs a read-only popup. Any key (or esc) closes it.
// Body may be multi-line.
func NewInfoModal(title, body string) Modal {
	return Modal{
		Kind:   modalInfo,
		Title:  title,
		Prompt: body,
	}
}

// Update returns the next modal state, an optional resulting tea.Cmd, and a
// done flag indicating that the modal should be removed from the root.
func (md Modal) Update(msg tea.Msg) (Modal, tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return md, nil, false
	}
	switch key.String() {
	case "esc":
		return md, nil, true
	}
	switch md.Kind {
	case modalInput:
		switch key.String() {
		case "enter":
			val := md.Input.Value()
			var cmd tea.Cmd
			if md.OnSubmit != nil {
				cmd = md.OnSubmit(val)
			}
			return md, cmd, true
		}
		var c tea.Cmd
		md.Input, c = md.Input.Update(msg)
		return md, c, false
	case modalConfirm:
		switch key.String() {
		case "y", "Y":
			var cmd tea.Cmd
			if md.OnConfirm != nil {
				cmd = md.OnConfirm(true)
			}
			return md, cmd, true
		case "n", "N", "enter":
			var cmd tea.Cmd
			if md.OnConfirm != nil {
				cmd = md.OnConfirm(false)
			}
			return md, cmd, true
		}
	case modalInfo:
		// Any key closes (esc already handled earlier).
		return md, nil, true
	}
	return md, nil, false
}

// Init focuses the input on first use.
func (md *Modal) Init() tea.Cmd {
	if md.Kind == modalInput {
		md.Input.Focus()
		return textinput.Blink
	}
	return nil
}

// View renders the dialog content (without centering — the root places it).
func (md Modal) View() string {
	header := Styles.Title.Render(" " + md.Title + " ")
	var body string
	hint := ""
	switch md.Kind {
	case modalInput:
		body = md.Input.View()
		hint = "enter confirm · esc cancel"
	case modalConfirm:
		hint = "y yes · n/enter no · esc cancel"
	case modalInfo:
		hint = "any key closes"
	}
	parts := []string{header}
	if md.Prompt != "" {
		parts = append(parts, md.Prompt)
	}
	if body != "" {
		parts = append(parts, body)
	}
	parts = append(parts, Styles.Muted.Render(hint))
	box := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return modalBox.Render(box)
}

// modalBox is the framed dialog style.
var modalBox = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorIndigo).
	Padding(1, 2).
	Width(56)
