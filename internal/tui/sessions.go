package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fikret/claude-sync/internal/paths"
	"github.com/fikret/claude-sync/internal/sessions"
)

type sessionsModel struct {
	p paths.Paths

	w, h int

	project string // label shown in header
	// directDir, when non-empty, makes the tab read JSONLs from that directory
	// directly (used for stray entries whose data hasn't been migrated into
	// _shared/ yet). When empty, the tab reads from p.Shared/<project>.
	directDir string
	items     []sessions.Session
	cursor    int

	// User-adjustable offset added to the default split. shift+up/down nudges
	// it; bounds enforced when computing the actual layout.
	listHDelta int

	// Bottom preview pane
	preview        viewport.Model
	previewLoaded  string // path of currently-loaded preview, to avoid reload thrash
	previewFocused bool   // when true, key input scrolls preview instead of list
}

func newSessionsModel(p paths.Paths, project string) sessionsModel {
	vp := viewport.New(0, 0)
	return sessionsModel{p: p, project: project, preview: vp}
}

func (m sessionsModel) Init() tea.Cmd { return nil }

func (m *sessionsModel) SetProject(name string) {
	m.project = name
	m.directDir = ""
	m.cursor = 0
	m.previewLoaded = ""
}

// SetStrayView puts the tab into read-from-anywhere mode for stray dirs.
// label is shown in the header; dir is the absolute path to read JSONLs from.
func (m *sessionsModel) SetStrayView(label, dir string) {
	m.project = label
	m.directDir = dir
	m.cursor = 0
	m.previewLoaded = ""
}

func (m sessionsModel) refresh() tea.Cmd { return sessionsRefreshCmd(m.p, m.project, m.directDir) }

func sessionsRefreshCmd(pp paths.Paths, project, directDir string) tea.Cmd {
	return func() tea.Msg {
		var ss []sessions.Session
		var err error
		switch {
		case directDir != "":
			ss, err = sessions.ListDir(directDir, project)
		case project == "":
			ss, err = sessions.ListAll(pp.Shared)
		default:
			ss, err = sessions.ListProject(pp.Shared, project)
		}
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		return sessionsLoadedMsg{items: ss}
	}
}

type sessionsLoadedMsg struct{ items []sessions.Session }

func (m sessionsModel) update(msg tea.Msg) (sessionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		previewW := msg.Width - 4
		// Compute preview height to match view() math.
		_, previewH := m.computeSplitForTwoPanes(msg.Height - 3)
		if previewW < 10 {
			previewW = 10
		}
		if previewH < 2 {
			previewH = 2
		}
		m.preview.Width = previewW
		m.preview.Height = previewH
		return m, nil

	case sessionsLoadedMsg:
		m.items = msg.items
		if m.cursor >= len(m.items) {
			m.cursor = 0
		}
		m.previewLoaded = ""
		if len(m.items) == 0 {
			m.preview.SetContent("")
			return m, nil
		}
		return m, m.loadPreview()

	case previewMsg:
		m.preview.SetContent(msg.content)
		m.preview.GotoTop()
		m.previewLoaded = msg.path
		return m, nil

	case tea.KeyMsg:
		// Preview-focused mode: every key scrolls the viewport. Esc and
		// backspace return focus to the list. Action keys (R/A/D) are
		// intentionally ignored so the user has to step out before mutating.
		if m.previewFocused {
			switch msg.String() {
			case "esc", "backspace":
				m.previewFocused = false
				return m, nil
			}
			var c tea.Cmd
			m.preview, c = m.preview.Update(msg)
			return m, c
		}

		switch msg.String() {
		case "backspace":
			// Backspace from list focus jumps back to the Projects tab —
			// the symmetric inverse of "enter on a project to drill in".
			return m, func() tea.Msg { return switchToTabMsg{Tab: TabProjects} }
		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			m.previewFocused = true
			return m, nil
		case "shift+up":
			m.listHDelta--
			return m, nil
		case "shift+down":
			m.listHDelta++
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				return m, m.loadPreview()
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				return m, m.loadPreview()
			}
		case "pgup":
			step := m.listVisibleRows()
			m.cursor -= step
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, m.loadPreview()
		case "pgdown":
			step := m.listVisibleRows()
			m.cursor += step
			if m.cursor > len(m.items)-1 {
				m.cursor = len(m.items) - 1
			}
			return m, m.loadPreview()
		case "n":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				return m, m.loadPreview()
			}
		case "p":
			if m.cursor > 0 {
				m.cursor--
				return m, m.loadPreview()
			}
		case "R":
			if m.cursor >= len(m.items) {
				return m, nil
			}
			s := m.items[m.cursor]
			path := s.Path
			pp := m.p
			project := m.project
			direct := m.directDir
			return m, func() tea.Msg {
				modal := NewInputModal(
					"Rename session",
					"Friendly title:",
					"e.g. Refactor auth flow",
					s.Title,
					func(title string) tea.Cmd {
						title = strings.TrimSpace(title)
						return tea.Sequence(
							func() tea.Msg {
								if err := sessions.Rename(path, title); err != nil {
									return flashMsg{text: err.Error(), err: true}
								}
								return nil
							},
							sessionsRefreshCmd(pp, project, direct),
							flash("renamed"),
						)
					},
				)
				return openModalMsg{M: modal}
			}
		case "A":
			if m.cursor >= len(m.items) {
				return m, nil
			}
			s := m.items[m.cursor]
			path := s.Path
			now := !s.Archived
			return m, tea.Sequence(
				func() tea.Msg {
					if err := sessions.SetArchived(path, now); err != nil {
						return flashMsg{text: err.Error(), err: true}
					}
					return nil
				},
				m.refresh(),
				flash(map[bool]string{true: "archived", false: "unarchived"}[now]),
			)
		case "D":
			if m.cursor >= len(m.items) {
				return m, nil
			}
			s := m.items[m.cursor]
			path := s.Path
			pp := m.p
			project := m.project
			direct := m.directDir
			label := s.Title
			if label == "" {
				label = s.UUID[:8]
			}
			return m, func() tea.Msg {
				modal := NewConfirmModal(
					"Delete session",
					"Delete \""+label+"\"? This removes the JSONL and its meta sidecar.",
					func(yes bool) tea.Cmd {
						if !yes {
							return flash("cancelled")
						}
						return tea.Sequence(
							func() tea.Msg {
								if err := sessions.Delete(path); err != nil {
									return flashMsg{text: err.Error(), err: true}
								}
								return nil
							},
							sessionsRefreshCmd(pp, project, direct),
							flash("deleted"),
						)
					},
				)
				return openModalMsg{M: modal}
			}
		}
		// In list-focused mode we deliberately do NOT forward unhandled keys
		// to the preview viewport — keys only act on the focused pane, which
		// keeps navigation predictable (up/down won't accidentally scroll the
		// preview while the user is just browsing the list).
		return m, nil
	}
	return m, nil
}

// loadPreview reads the JSONL of the current cursor and renders it as a
// human-readable transcript (user/assistant turns) into the viewport.
// Lazy: only re-renders when the selected file changes.
func (m sessionsModel) loadPreview() tea.Cmd {
	if m.cursor >= len(m.items) {
		return nil
	}
	path := m.items[m.cursor].Path
	if path == m.previewLoaded {
		return nil
	}
	width := m.preview.Width - 2
	return func() tea.Msg {
		turns, err := sessions.LoadTranscript(path, 0)
		if err != nil {
			return previewMsg{path: path, content: "(read error: " + err.Error() + ")"}
		}
		if len(turns) == 0 {
			return previewMsg{path: path, content: Styles.Muted.Render("(no user/assistant messages)")}
		}
		styler := func(role string) string {
			switch role {
			case "user":
				return roleUserStyle.Render(" you ")
			case "assistant":
				return roleAsstStyle.Render(" claude ")
			}
			return role
		}
		content := sessions.FormatTranscript(turns, width, styler)
		return previewMsg{path: path, content: content}
	}
}

// Role labels — picked so they pop against any terminal background.
var (
	roleUserStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12"))
	roleAsstStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("0")).Background(lipgloss.Color("10"))
)

type previewMsg struct{ path, content string }

func (m sessionsModel) view(w, h int) string {
	header := Styles.Header.Render("Sessions")
	if m.project != "" {
		header += Styles.Muted.Render("  ·  " + m.project)
	} else {
		header += Styles.Muted.Render("  ·  (all projects — pick one in tab 1)")
	}

	// Layout: top pane (border without bottom) + connector line + bottom pane
	// (border without top). Total render = (listRowsH+1+2) + 1 + (previewH+2)
	// where the +1 inside top pane is the inline header, and the +1 between
	// is the connector. We want this == h:
	//   listRowsH + 1 + 2 + 1 + previewH + 2 - 1 (top pane has no bottom) - 1 (bottom no top) = h
	// Simplifies to: listRowsH + previewH + 4 = h
	// (top pane: top border + header + listRows = 1+1+listRowsH lines
	//  connector: 1 line
	//  bottom pane: previewH + bottom border = previewH+1 lines)
	innerSpace := h - 3 // 2 outer borders (top + bottom) + 1 connector
	listRowsH, previewH := m.computeSplitForTwoPanes(innerSpace)

	contentW := w - 4 // border 2 + left padding 1 + scrollbar 1
	if contentW < 10 {
		contentW = 10
	}
	innerW := w - 2 // pane render width = innerW + 2 = w

	// Active pane gets the full indigo; inactive uses a darker shade so the
	// user sees at a glance which side responds to keys.
	topColor := colorIndigo
	bottomColor := colorIndigoDim
	if m.previewFocused {
		topColor = colorIndigoDim
		bottomColor = colorIndigo
	}
	base := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 0, 0, 1)
	topStyle := base.
		BorderForeground(topColor).
		BorderTop(true).BorderLeft(true).BorderRight(true).BorderBottom(false)
	bottomStyle := base.
		BorderForeground(bottomColor).
		BorderTop(false).BorderLeft(true).BorderRight(true).BorderBottom(true)

	rows, listOffset := m.renderListWindow(contentW, listRowsH)
	listBar := scrollbar(listRowsH, len(m.items), listOffset)
	listBody := lipgloss.JoinHorizontal(lipgloss.Top,
		padToWidth(rows, contentW, listRowsH), listBar)
	topInside := lipgloss.JoinVertical(lipgloss.Left, header, listBody)
	topPaneInnerH := listRowsH + 1 // header (1) + listRows
	topPane := topStyle.Width(innerW).Height(topPaneInnerH).Render(clipToHeight(topInside, topPaneInnerH))

	previewBody := m.preview.View()
	previewBar := scrollbar(previewH, m.preview.TotalLineCount(), m.preview.YOffset)
	previewCombo := lipgloss.JoinHorizontal(lipgloss.Top,
		padToWidth(previewBody, contentW, previewH), previewBar)
	bottomPane := bottomStyle.Width(innerW).Height(previewH).Render(clipToHeight(previewCombo, previewH))

	// Connector spans both panes' borders. Use the same color as the active
	// pane so the highlight reads as one continuous box around the focused
	// section's edge, with the connector serving as its visual base/top.
	connColor := topColor
	if m.previewFocused {
		connColor = bottomColor
	}
	connector := lipgloss.NewStyle().Foreground(connColor).
		Render("├" + strings.Repeat("─", innerW) + "┤")

	out := lipgloss.JoinVertical(lipgloss.Left, topPane, connector, bottomPane)
	return clipToHeight(out, h)
}

// computeSplitForTwoPanes splits available content rows between the list
// portion and the preview portion of the two-pane sessions layout.
// totalRows = listRowsH + previewH + 1 (header inside top pane).
func (m sessionsModel) computeSplitForTwoPanes(totalRows int) (int, int) {
	if totalRows < 4 {
		totalRows = 4
	}
	rows := totalRows - 1 // 1 line for the inline header inside top pane
	defaultListRows := sessionsListHeight(m.h-2) - 1
	listRowsH := defaultListRows + m.listHDelta
	if listRowsH < 2 {
		listRowsH = 2
	}
	if listRowsH > rows-2 {
		listRowsH = rows - 2
	}
	previewH := rows - listRowsH
	if previewH < 2 {
		previewH = 2
	}
	return listRowsH, previewH
}

// listVisibleRows estimates how many rows the list currently shows — used by
// pgup/pgdn to advance by a page worth of items.
func (m sessionsModel) listVisibleRows() int {
	r, _ := m.computeSplitForTwoPanes(m.h - 2 - 3)
	if r < 1 {
		r = 1
	}
	return r
}

// padToWidth right-pads each line of s to exactly w columns and ensures the
// block has h rows (extra blank rows below). Required so JoinHorizontal with
// the scrollbar column produces straight columns without ragged ends.
func padToWidth(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		visible := lipgloss.Width(lines[i])
		if visible < w {
			lines[i] += strings.Repeat(" ", w-visible)
		}
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

// sessionsListHeight reserves ~40% of the body height for the list, capped
// to a sensible range so neither pane collapses on very tall or short windows.
func sessionsListHeight(bodyH int) int {
	h := bodyH * 4 / 10
	if h < 6 {
		h = 6
	}
	if h > 18 {
		h = 18
	}
	return h
}

// renderListWindow renders a sliding window of sessions sized to height rows,
// keeping the cursor visible. Returns the body text and the index of the
// first visible row (so callers can sync a scrollbar).
func (m sessionsModel) renderListWindow(width, height int) (string, int) {
	if len(m.items) == 0 {
		return Styles.Muted.Render("(no sessions)"), 0
	}
	if height < 1 {
		height = 1
	}
	n := len(m.items)
	// Center the cursor in the window when possible.
	start := m.cursor - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > n {
		end = n
		start = end - height
		if start < 0 {
			start = 0
		}
	}
	var sb strings.Builder
	for i := start; i < end; i++ {
		s := m.items[i]
		title := s.Title
		if title == "" {
			if s.Preview != "" {
				title = s.Preview
			} else {
				title = "(empty)"
			}
		}
		if len(title) > width-25 && width > 30 {
			title = title[:width-25] + "…"
		}
		date := s.LastAt.Format("01-02 15:04")
		flag := " "
		if s.Archived {
			flag = "a"
		}
		line := fmt.Sprintf("%s %-11s %3d  %s", flag, date, s.MsgCount, title)
		if i == m.cursor {
			// Width(width) makes the indigo background span the full row,
			// not just the text. Without it the highlight stops where the
			// title ends.
			line = Styles.Selected.Width(width).Render("▸ " + line)
		} else {
			line = "  " + line
		}
		sb.WriteString(line)
		if i != end-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String(), start
}

// previewMsg handler is wired in update() via type switch — but we forgot to
// add it. Fold into update() below by intercepting the message.
func (m sessionsModel) handlePreview(msg previewMsg) sessionsModel {
	m.preview.SetContent(msg.content)
	m.preview.GotoTop()
	m.previewLoaded = msg.path
	return m
}

// Force-include duration import for go vet sanity if we add timing later.
var _ = time.Second
