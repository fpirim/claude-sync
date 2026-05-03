// Package tui hosts the Bubble Tea program that fronts claude-sync.
//
// The root Model owns three sub-models (one per visible tab) and a small
// amount of global state (active tab, terminal size, last error). Sub-models
// are ordinary tea.Model implementations; we forward window resize and a
// Refresh message to each as needed.
package tui

import (
	"fmt"

	"github.com/fpirim/claude-sync/internal/config"
	"github.com/fpirim/claude-sync/internal/fsops"
	"github.com/fpirim/claude-sync/internal/paths"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// reconcileCmd runs a Repair in the background (file lock + filesystem ops)
// and returns reconciledMsg on success, flashMsg on error.
func reconcileCmd(pp paths.Paths, cfg config.Config, machine string) tea.Cmd {
	return func() tea.Msg {
		lock, err := config.AcquireLock(pp.Lock)
		if err != nil {
			return flashMsg{text: "lock: " + err.Error(), err: true}
		}
		defer lock.Release()
		if _, err := fsops.Repair(pp, cfg, machine, false); err != nil {
			return flashMsg{text: "repair: " + err.Error(), err: true}
		}
		return reconciledMsg{}
	}
}

// Tab identifies the active pane.
type Tab int

const (
	TabProjects Tab = iota
	TabSessions
	TabSync
	TabConfig
)

func (t Tab) Label() string {
	switch t {
	case TabProjects:
		return "Projects"
	case TabSessions:
		return "Sessions"
	case TabSync:
		return "Sync"
	case TabConfig:
		return "Config"
	}
	return "?"
}

// Number returns the 1-based key the user types to jump to this tab.
func (t Tab) Number() int {
	switch t {
	case TabProjects:
		return 1
	case TabSessions:
		return 2
	case TabSync:
		return 3
	case TabConfig:
		return 4
	}
	return 0
}

// Model is the root Bubble Tea model.
type Model struct {
	P       paths.Paths
	Cfg     config.Config
	Machine string

	w, h int

	active   Tab
	projects projectsModel
	sessions sessionsModel
	sync     syncModel
	cfgTab   configModel

	// Active modal overlay. Non-nil suppresses global shortcuts and routes
	// all key input to the modal; Esc always dismisses.
	modal *Modal

	flash    string // transient status line
	flashErr bool
}

// New constructs the root model.
func New(p paths.Paths, cfg config.Config, machine string) Model {
	model := Model{
		P: p, Cfg: cfg, Machine: machine,
		active: TabProjects,
	}
	model.projects = newProjectsModel(p, cfg, machine)
	model.sessions = newSessionsModel(p, "")
	model.sync = newSyncModel(p, &model.Cfg)
	model.cfgTab = newConfigModel(p)
	return model
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.projects.Init(), m.sessions.Init(), m.sync.Init(), m.cfgTab.Init())
}

// flashMsg is dispatched by sub-models to set the footer status line.
type flashMsg struct {
	text string
	err  bool
}

func flash(text string) tea.Cmd        { return func() tea.Msg { return flashMsg{text: text} } }
func flashErr(text string) tea.Cmd     { return func() tea.Msg { return flashMsg{text: text, err: true} } }

// switchToSessionsMsg is fired when projects pane wants to drill in.
// directDir is non-empty for stray entries: Tab 2 reads from that path
// instead of from _shared/<project>.
type switchToSessionsMsg struct {
	project   string
	directDir string
}

// switchToTabMsg lets a sub-model jump to another tab (e.g. backspace from
// Sessions to go back to Projects).
type switchToTabMsg struct{ Tab Tab }

// reloadConfigMsg asks the root to re-read config.yml from disk.
type reloadConfigMsg struct{}

// reconciledMsg is fired after a background Repair completes; it triggers a
// re-populate of the projects pane so any filesystem changes (new symlinks,
// removed orphans) become visible.
type reconciledMsg struct{}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Modal handling first: if a modal is open, route every key to it and
	// suppress global shortcuts so the user can type freely. Non-key msgs
	// still fall through to the regular handlers below.
	if m.modal != nil {
		if _, ok := msg.(tea.KeyMsg); ok {
			next, cmd, done := m.modal.Update(msg)
			if done {
				m.modal = nil
			} else {
				m.modal = &next
			}
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case openModalMsg:
		mm := msg.M
		initCmd := mm.Init()
		m.modal = &mm
		return m, initCmd
	case closeModalMsg:
		m.modal = nil
		return m, nil

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		// Forward to sub-models with a sane body size: subtract header (1) + footer (1).
		body := tea.WindowSizeMsg{Width: msg.Width, Height: max(msg.Height-2, 4)}
		var cmds []tea.Cmd
		var c tea.Cmd
		m.projects, c = m.projects.update(body)
		cmds = append(cmds, c)
		m.sessions, c = m.sessions.update(body)
		cmds = append(cmds, c)
		m.sync, c = m.sync.update(body)
		cmds = append(cmds, c)
		m.cfgTab, c = m.cfgTab.update(body)
		cmds = append(cmds, c)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		// Global keys.
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "1":
			m.active = TabProjects
			return m, nil
		case "2":
			m.active = TabSessions
			return m, nil
		case "3":
			m.active = TabSync
			return m, nil
		case "4":
			m.active = TabConfig
			return m, nil
		case "tab":
			m.active = (m.active + 1) % 4
			return m, nil
		case "?":
			body := helpFor(m.active, m.active == TabSessions && m.sessions.previewFocused)
			modal := NewInfoModal("Shortcuts", body)
			return m, func() tea.Msg { return openModalMsg{M: modal} }
		case "r":
			if cfg, err := config.Load(m.P.Config); err == nil {
				m.Cfg = cfg
				m.projects.cfg = cfg
			}
			m.projects.populate()
			m.flash = "refreshed"
			return m, tea.Batch(
				m.sessions.refresh(),
				m.cfgTab.refresh(),
				reconcileCmd(m.P, m.Cfg, m.Machine),
			)
		}

	case flashMsg:
		m.flash = msg.text
		m.flashErr = msg.err
		return m, nil

	case switchToSessionsMsg:
		if msg.directDir != "" {
			m.sessions.SetStrayView(msg.project, msg.directDir)
		} else {
			m.sessions.SetProject(msg.project)
		}
		m.active = TabSessions
		return m, m.sessions.refresh()

	case switchToTabMsg:
		m.active = msg.Tab
		return m, nil

	case reloadConfigMsg:
		if cfg, err := config.Load(m.P.Config); err == nil {
			m.Cfg = cfg
			m.projects.cfg = cfg
		}
		// projects.populate() has a pointer receiver — m.projects is
		// addressable here, so this mutates the active state in place.
		m.projects.populate()
		// After a config change, reconcile the filesystem (new path -> new
		// symlink, dropped path -> orphan removal) and refresh sessions.
		return m, tea.Batch(
			m.sessions.refresh(),
			reconcileCmd(m.P, m.Cfg, m.Machine),
		)

	case reconciledMsg:
		m.projects.populate()
		return m, m.sessions.refresh()

	// Sub-model-targeted messages must reach their owner regardless of
	// which tab is active, otherwise initial loads (e.g. configContentMsg
	// fired by configModel.Init) get dropped when their tab isn't focused.
	case configContentMsg, editorFinishedMsg:
		var cmd tea.Cmd
		m.cfgTab, cmd = m.cfgTab.update(msg)
		return m, cmd
	case sessionsLoadedMsg, previewMsg, sessionRenamedMsg:
		var cmd tea.Cmd
		m.sessions, cmd = m.sessions.update(msg)
		return m, cmd
	case syncTickMsg, syncSnapshotMsg:
		var cmd tea.Cmd
		m.sync, cmd = m.sync.update(msg)
		return m, cmd
	}

	// Forward to active sub-model.
	var cmd tea.Cmd
	switch m.active {
	case TabProjects:
		m.projects, cmd = m.projects.update(msg)
	case TabSessions:
		m.sessions, cmd = m.sessions.update(msg)
	case TabSync:
		m.sync, cmd = m.sync.update(msg)
	case TabConfig:
		m.cfgTab, cmd = m.cfgTab.update(msg)
	}
	return m, cmd
}

func (m Model) View() string {
	// Header + footer always render — even on the very first frame before
	// WindowSizeMsg arrives. That way the user sees the tab bar from the
	// instant the program starts.
	var body string
	if m.w == 0 {
		body = Styles.Muted.Render("loading…")
	} else {
		switch m.active {
		case TabProjects:
			body = m.projects.view(m.w, m.h-2)
		case TabSessions:
			body = m.sessions.view(m.w, m.h-2)
		case TabSync:
			body = m.sync.view(m.w, m.h-2)
		case TabConfig:
			body = m.cfgTab.view(m.w, m.h-2)
		}
		if m.modal != nil {
			// True overlay: keep the body visible behind the dialog.
			body = centerOverlay(body, m.modal.View(), m.w, m.h-2)
		}
	}
	out := lipgloss.JoinVertical(lipgloss.Left, m.header(), body, m.footer())
	if m.h > 0 {
		out = clipToHeight(out, m.h)
	}
	return out
}

func (m Model) header() string {
	tabs := []Tab{TabProjects, TabSessions, TabSync, TabConfig}
	parts := []string{Styles.Title.Render(" claude-sync ")}
	for _, t := range tabs {
		label := fmt.Sprintf("[%d]%s", t.Number(), t.Label())
		if t == m.active {
			parts = append(parts, Styles.TabActive.Render(label))
		} else {
			parts = append(parts, Styles.TabInactive.Render(label))
		}
	}
	parts = append(parts, Styles.Muted.Render(fmt.Sprintf("  host=%s", m.Machine)))
	row := lipgloss.JoinHorizontal(lipgloss.Center, parts...)
	if m.w <= 0 {
		return row
	}
	return lipgloss.NewStyle().Width(m.w).Render(row)
}

func (m Model) footer() string {
	left := Styles.Footer.Render("? help")
	right := ""
	if m.flash != "" {
		st := Styles.OK
		if m.flashErr {
			st = Styles.Err
		}
		right = st.Render(m.flash)
	}
	pad := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + lipgloss.NewStyle().Width(pad).Render("") + right
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
