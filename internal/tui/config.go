package tui

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fikret/claude-sync/internal/paths"
)

type configModel struct {
	p     paths.Paths
	w, h  int
	vp    viewport.Model
	loaded bool
}

func newConfigModel(p paths.Paths) configModel {
	return configModel{p: p, vp: viewport.New(0, 0)}
}

func (m configModel) Init() tea.Cmd { return m.load() }

func (m configModel) load() tea.Cmd {
	return func() tea.Msg {
		b, err := os.ReadFile(m.p.Config)
		if err != nil {
			return configContentMsg{content: "(no config yet — run `claude-sync add` or use Tab 1)"}
		}
		return configContentMsg{content: string(b)}
	}
}

func (m configModel) refresh() tea.Cmd { return m.load() }

type configContentMsg struct{ content string }

type editorFinishedMsg struct{ err error }

func (m configModel) update(msg tea.Msg) (configModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		// Mirror view(): paneW=w, contentW=w-4, innerH = h-2 with 1 header line inside.
		m.vp.Width = max(msg.Width-4, 10)
		m.vp.Height = max(msg.Height-3, 4) // 1 header + 2 border
		return m, nil

	case configContentMsg:
		m.vp.SetContent(msg.content)
		m.loaded = true
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			return m, flashErr("editor: " + msg.err.Error())
		}
		// Reload local viewer AND propagate to root so projects/sessions
		// see the new config. Order matters: viewer first, then global.
		return m, tea.Sequence(
			m.load(),
			func() tea.Msg { return reloadConfigMsg{} },
			flash("config reloaded"),
		)

	case tea.KeyMsg:
		switch msg.String() {
		case "e":
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			c := exec.Command(editor, m.p.Config)
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return editorFinishedMsg{err: err}
			})
		}
		// Forward to viewport for scroll keys.
		var c tea.Cmd
		m.vp, c = m.vp.Update(msg)
		return m, c
	}
	return m, nil
}

func (m configModel) view(w, h int) string {
	header := Styles.Header.Render("config.yml") + Styles.Muted.Render("  ·  "+m.p.Config)
	innerH := h - 2
	innerW := w - 2
	body := m.vp.View()
	inside := lipgloss.JoinVertical(lipgloss.Left, header, body)
	pane := Styles.PaneActive.Padding(0, 0, 0, 1).Width(innerW).Height(innerH).Render(clipToHeight(inside, innerH))
	return clipToHeight(pane, h)
}

func _unused() string { return fmt.Sprint("") }
