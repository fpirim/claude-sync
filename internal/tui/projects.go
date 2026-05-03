package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fikret/claude-sync/internal/config"
	"github.com/fikret/claude-sync/internal/encoder"
	"github.com/fikret/claude-sync/internal/fsops"
	"github.com/fikret/claude-sync/internal/paths"
)

type projectItem struct {
	name      string
	onHere    bool
	localPath string
	linkOK    bool
	files     int
	conflict  bool

	// Stray entries (not in config). When stray=true, name holds a synthetic
	// best-guess label and strayPath is the real on-disk directory.
	stray     bool
	strayKind string // "orphan-empty" | "stranded-data"
	strayPath string
	strayBytes int64
}

func (i projectItem) Title() string {
	if i.stray {
		mark := "○"
		if i.strayKind == "stranded-data" {
			mark = "⚠"
		}
		return fmt.Sprintf("%s %s", mark, i.name)
	}
	mark := "·"
	if i.onHere {
		if i.linkOK {
			mark = "✓"
		} else {
			mark = "✗"
		}
	}
	suffix := ""
	if i.conflict {
		suffix = " ⚠"
	}
	return fmt.Sprintf("%s %s%s", mark, i.name, suffix)
}

func (i projectItem) Description() string {
	if i.stray {
		switch i.strayKind {
		case "orphan-empty":
			return Styles.Muted.Render("empty stray dir · D to delete · " + i.strayPath)
		case "stranded-data":
			return Styles.Warn.Render("unregistered data · a adopt · D delete · ") +
				Styles.Muted.Render(i.strayPath)
		}
		return ""
	}
	if !i.onHere {
		return Styles.Muted.Render("not on this host")
	}
	files := fmt.Sprintf("%d sessions", i.files)
	return Styles.Muted.Render(files+" · ") + i.localPath
}
func (i projectItem) FilterValue() string { return i.name }

// projectDelegate renders projectItem rows. Two-line layout: title on top,
// description below. The selected row gets a full-width white-on-indigo
// inverse highlight that wipes any per-cell coloring from the unselected
// rendering — that way the selection bar stays consistent regardless of
// item type (normal / stranded / not-on-host).
type projectDelegate struct{}

func (d projectDelegate) Height() int  { return 2 }
func (d projectDelegate) Spacing() int { return 0 }
func (d projectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d projectDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(projectItem)
	if !ok {
		return
	}
	rowW := m.Width()
	if rowW < 10 {
		rowW = 10
	}
	// Reserve 1 column for the leading " " prefix below.
	avail := rowW - 1

	// Title: tight, right-truncate so the leading marker stays visible.
	title := rightTruncate(it.Title(), avail)
	// Description carries the path. If long, truncate from the LEFT so the
	// project basename (right side of the path) remains visible.
	desc := leftTruncate(it.Description(), avail)

	if index == m.Index() {
		// On selection we strip pre-applied styling so the inverse highlight
		// reads cleanly without weird color seams in the middle of a row.
		title = ansi.Strip(title)
		desc = ansi.Strip(desc)
		bg := lipgloss.NewStyle().
			Foreground(colorWhite).
			Background(colorIndigo).
			Width(rowW)
		fmt.Fprintln(w, bg.Bold(true).Render(" "+title))
		fmt.Fprint(w, bg.Render(" "+desc))
		return
	}
	plain := lipgloss.NewStyle().Width(rowW)
	fmt.Fprintln(w, plain.Render(" "+title))
	fmt.Fprint(w, plain.Render(" "+desc))
}

type projectsModel struct {
	p       paths.Paths
	cfg     config.Config
	machine string

	w, h int

	list list.Model
}

func newProjectsModel(p paths.Paths, cfg config.Config, machine string) projectsModel {
	delegate := projectDelegate{}
	l := list.New(nil, delegate, 0, 0)
	// Strip every chrome bubbles list ships with: the inner title, the pagination/
	// status bar, AND the bottom help line. ? handles all of those for us.
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	m := projectsModel{p: p, cfg: cfg, machine: machine, list: l}
	m.populate()
	return m
}

func (m projectsModel) Init() tea.Cmd { return nil }

func (m *projectsModel) populate() {
	scan, _ := fsops.ScanProjects(m.p.Projects, m.p.Shared)
	names := make([]string, 0, len(m.cfg.Projects))
	for n := range m.cfg.Projects {
		names = append(names, n)
	}
	sort.Strings(names)

	// Encoded names that belong to known projects on this host. Anything
	// outside this set in projects/ is a stray.
	allowed := map[string]struct{}{}
	for _, n := range names {
		if mp, ok := m.cfg.Projects[n].Paths[m.machine]; ok && mp != "" {
			allowed[encoder.Encode(mp)] = struct{}{}
		}
	}

	items := make([]list.Item, 0, len(names))
	for _, n := range names {
		pr := m.cfg.Projects[n]
		it := projectItem{name: n}
		if mp, ok := pr.Paths[m.machine]; ok && mp != "" {
			it.onHere = true
			it.localPath = mp
			enc := encoder.Encode(mp)
			if info, ok := scan.EncodedDirs[enc]; ok && info.IsSymlink {
				it.linkOK = true
			}
		}
		if si, ok := scan.SharedProjects[n]; ok {
			it.files = si.NumFiles
			it.conflict = si.HasConflicts
		}
		items = append(items, it)
	}

	// Append strays (real dirs in projects/ not symlinked into _shared and
	// not matching any known project). Sorted by encoded name.
	strayNames := make([]string, 0)
	for enc, info := range scan.EncodedDirs {
		if _, ok := allowed[enc]; ok {
			continue
		}
		// Skip symlinks (they're handled by repair's policy).
		if info.IsSymlink {
			continue
		}
		if !info.IsDir {
			continue
		}
		// For stranded-data (non-empty) strays, hide entries whose source
		// project no longer exists on disk — that's dead history, not
		// something the user can re-adopt productively.
		if !info.Empty && encoder.ResolveExisting(enc, nil) == "" {
			continue
		}
		strayNames = append(strayNames, enc)
	}
	sort.Strings(strayNames)
	for _, enc := range strayNames {
		info := scan.EncodedDirs[enc]
		guess := guessNameFromEncoded(enc)
		kind := "orphan-empty"
		if !info.Empty {
			kind = "stranded-data"
		}
		items = append(items, projectItem{
			name:      guess + " (stray)",
			stray:     true,
			strayKind: kind,
			strayPath: info.Path,
		})
	}

	m.list.SetItems(items)
}

// guessNameFromEncoded picks the last segment of the decoded path as the name.
// Encoding is lossy ('.' and '/' both map to '-'), so this is just a hint.
func guessNameFromEncoded(enc string) string {
	// Drop leading '-'.
	s := strings.TrimPrefix(enc, "-")
	// Best guess: last "-segment" is the project basename.
	parts := strings.Split(s, "-")
	if len(parts) == 0 {
		return enc
	}
	return parts[len(parts)-1]
}

func (m projectsModel) update(msg tea.Msg) (projectsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		// Mirror view(): paneW=w, contentW = paneW - 4, innerH = h - 2.
		contentW := msg.Width - 4
		contentH := msg.Height - 2
		if contentW < 10 {
			contentW = 10
		}
		if contentH < 4 {
			contentH = 4
		}
		m.list.SetSize(contentW, contentH)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "a":
			it, _ := m.list.SelectedItem().(projectItem)
			if it.stray {
				return m, m.openAdoptModal(it)
			}
			return m, m.openAddNameModal()
		case "r":
			return m, m.applyRepair()
		case "D":
			it, ok := m.list.SelectedItem().(projectItem)
			if !ok {
				return m, nil
			}
			if it.stray {
				return m, m.openDeleteStrayModal(it)
			}
			return m, m.openDeleteConfirmModal(it)
		case "enter":
			it, ok := m.list.SelectedItem().(projectItem)
			if !ok {
				return m, nil
			}
			if it.stray {
				if it.strayKind != "stranded-data" {
					// Empty stray has no sessions to view.
					return m, nil
				}
				label := strings.TrimSuffix(it.name, " (stray)") + " (stray)"
				path := it.strayPath
				return m, func() tea.Msg {
					return switchToSessionsMsg{project: label, directDir: path}
				}
			}
			return m, func() tea.Msg { return switchToSessionsMsg{project: it.name} }
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// openAddNameModal opens the first modal of the two-step add flow,
// pre-filled with the currently-selected project's name. This way the
// common case of "add this machine's path to an existing project" needs
// only Enter on the name step.
func (m projectsModel) openAddNameModal() tea.Cmd {
	defaultName := ""
	if it, ok := m.list.SelectedItem().(projectItem); ok {
		defaultName = it.name
	}
	pp := m.p
	machine := m.machine
	cfg := m.cfg
	// existing path on this host for the selected project, if any
	existingPath := ""
	if it, ok := m.list.SelectedItem().(projectItem); ok && it.onHere {
		existingPath = it.localPath
	}
	return func() tea.Msg {
		modal := NewInputModal(
			"Add / update project",
			"Project name:",
			"e.g. my-app",
			defaultName,
			func(name string) tea.Cmd {
				name = strings.TrimSpace(name)
				if name == "" {
					return flashErr("name empty")
				}
				// Path default: previously-recorded path > pwd/<name>.
				def := existingPath
				if def == "" {
					if pr, ok := cfg.Projects[name]; ok {
						if mp, ok := pr.Paths[machine]; ok && mp != "" {
							def = mp
						}
					}
				}
				if def == "" {
					if cwd, err := os.Getwd(); err == nil {
						// Append project name unless pwd basename already matches.
						if filepath.Base(cwd) == name {
							def = cwd
						} else {
							def = filepath.Join(cwd, name)
						}
					}
				}
				return openPathModal(pp, machine, name, def)
			},
		)
		return openModalMsg{M: modal}
	}
}

// openPathModal opens the second step (path entry).
func openPathModal(pp paths.Paths, machine, name, defaultPath string) tea.Cmd {
	return func() tea.Msg {
		modal := NewInputModal(
			"Add / update project",
			fmt.Sprintf("Path on %s for %q:", machine, name),
			"absolute path",
			defaultPath,
			func(path string) tea.Cmd {
				path = strings.TrimSpace(path)
				if path == "" {
					return flashErr("path empty")
				}
				return applyAddRaw(pp, machine, name, path)
			},
		)
		return openModalMsg{M: modal}
	}
}

// applyAddRaw is the package-level worker so it doesn't capture stale
// projectsModel state across modal hops.
func applyAddRaw(pp paths.Paths, machine, name, path string) tea.Cmd {
	work := func() tea.Msg {
		lock, err := config.AcquireLock(pp.Lock)
		if err != nil {
			return flashMsg{text: "lock: " + err.Error(), err: true}
		}
		defer lock.Release()
		cfg, err := config.Load(pp.Config)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		pr, ok := cfg.Projects[name]
		if !ok {
			pr = config.Project{Paths: map[string]string{}}
		}
		if pr.Paths == nil {
			pr.Paths = map[string]string{}
		}
		pr.Paths[machine] = path
		cfg.Projects[name] = pr
		if err := config.Save(pp.Config, cfg); err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		if _, err := fsops.Repair(pp, cfg, machine, false); err != nil {
			return flashMsg{text: "saved but repair failed: " + err.Error(), err: true}
		}
		return reloadConfigMsg{}
	}
	return tea.Sequence(work, flash("saved "+name))
}

// openAdoptModal lets the user assign a project name + path to a stray
// directory under projects/. After accepting, repair migrates the data into
// _shared/<name>/ and replaces the dir with a symlink — same flow as add.
func (m projectsModel) openAdoptModal(it projectItem) tea.Cmd {
	pp := m.p
	machine := m.machine
	guess := strings.TrimSuffix(it.name, " (stray)")
	// Best-effort decode of the encoded path: '-' -> '/'. Lossy but a hint.
	decoded := strings.Replace(strings.TrimPrefix(filepath.Base(it.strayPath), "-"), "-", "/", -1)
	decoded = "/" + decoded
	return func() tea.Msg {
		modal := NewInputModal(
			"Adopt stray directory",
			"Project name:",
			"e.g. my-app",
			guess,
			func(name string) tea.Cmd {
				name = strings.TrimSpace(name)
				if name == "" {
					return flashErr("name empty")
				}
				return openPathModal(pp, machine, name, decoded)
			},
		)
		return openModalMsg{M: modal}
	}
}

// openDeleteStrayModal removes a stray directory from disk. Empty dirs
// disappear silently; data dirs require explicit confirmation since this
// destroys whatever is inside.
func (m projectsModel) openDeleteStrayModal(it projectItem) tea.Cmd {
	pp := m.p
	prompt := "Delete empty directory " + it.strayPath + "?"
	if it.strayKind == "stranded-data" {
		prompt = "DESTROY " + it.strayPath + " and all data inside?\nThis cannot be undone."
	}
	return func() tea.Msg {
		path := it.strayPath
		modal := NewConfirmModal(
			"Delete stray",
			prompt,
			func(yes bool) tea.Cmd {
				if !yes {
					return flash("cancelled")
				}
				return tea.Sequence(
					func() tea.Msg {
						lock, err := config.AcquireLock(pp.Lock)
						if err != nil {
							return flashMsg{text: err.Error(), err: true}
						}
						defer lock.Release()
						if err := os.RemoveAll(path); err != nil {
							return flashMsg{text: err.Error(), err: true}
						}
						return reloadConfigMsg{}
					},
					flash("deleted "+path),
				)
			},
		)
		return openModalMsg{M: modal}
	}
}

func (m projectsModel) openDeleteConfirmModal(it projectItem) tea.Cmd {
	pp := m.p
	machine := m.machine
	return func() tea.Msg {
		var modal Modal
		if it.onHere {
			modal = NewConfirmModal(
				"Remove from this host",
				fmt.Sprintf("Remove %q from %s?\nOther devices keep their entries.", it.name, machine),
				func(yes bool) tea.Cmd {
					if !yes {
						return flash("cancelled")
					}
					return applyRemoveFromMachineRaw(pp, machine, it.name)
				},
			)
		} else {
			modal = NewConfirmModal(
				"Delete entirely from config",
				fmt.Sprintf("Delete %q from config?\nThis syncs to all devices. _shared/ data is kept.", it.name),
				func(yes bool) tea.Cmd {
					if !yes {
						return flash("cancelled")
					}
					return applyDeleteFromConfigRaw(pp, machine, it.name)
				},
			)
		}
		return openModalMsg{M: modal}
	}
}

func applyRemoveFromMachineRaw(pp paths.Paths, machine, name string) tea.Cmd {
	work := func() tea.Msg {
		lock, err := config.AcquireLock(pp.Lock)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		defer lock.Release()
		cfg, err := config.Load(pp.Config)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		pr, ok := cfg.Projects[name]
		if !ok {
			return flashMsg{text: "project not in config", err: true}
		}
		oldPath := pr.Paths[machine]
		delete(pr.Paths, machine)
		// If no machine claims this project anymore, remove the project
		// entry entirely — an empty paths{} record is just noise.
		if len(pr.Paths) == 0 {
			delete(cfg.Projects, name)
		} else {
			cfg.Projects[name] = pr
		}
		if err := config.Save(pp.Config, cfg); err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		removeProjectSymlink(pp, oldPath)
		_, _ = fsops.Repair(pp, cfg, machine, false)
		return reloadConfigMsg{}
	}
	return tea.Sequence(work, flash("removed from "+machine))
}

func applyDeleteFromConfigRaw(pp paths.Paths, machine, name string) tea.Cmd {
	work := func() tea.Msg {
		lock, err := config.AcquireLock(pp.Lock)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		defer lock.Release()
		cfg, err := config.Load(pp.Config)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		oldPath := ""
		if pr, ok := cfg.Projects[name]; ok {
			oldPath = pr.Paths[machine]
		}
		delete(cfg.Projects, name)
		if err := config.Save(pp.Config, cfg); err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		removeProjectSymlink(pp, oldPath)
		_, _ = fsops.Repair(pp, cfg, machine, false)
		return reloadConfigMsg{}
	}
	return tea.Sequence(work, flash("deleted "+name+" from config"))
}

// removeProjectSymlink deletes ~/.claude/projects/<encoded-of-localPath> if it
// exists and is a symlink. Safe to call with empty path.
func removeProjectSymlink(pp paths.Paths, localPath string) {
	if localPath == "" {
		return
	}
	enc := encoder.Encode(resolveMaybe(localPath))
	link := pp.ProjectLink(enc)
	if info, err := os.Lstat(link); err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(link)
	}
}

// resolveMaybe mirrors fsops.resolveMaybe; kept here to avoid an import cycle
// (projects.go already imports fsops, but the helper there is unexported).
func resolveMaybe(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return abs
}

func (m projectsModel) applyRepair() tea.Cmd {
	type repairResult struct {
		applied int
		err     error
	}
	work := func() tea.Msg {
		lock, err := config.AcquireLock(m.p.Lock)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		defer lock.Release()
		res, err := fsops.Repair(m.p, m.cfg, m.machine, false)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		applied := 0
		for _, a := range res.Actions {
			if a.Kind != "noop" {
				applied++
			}
		}
		return repairResult{applied: applied}
	}
	_ = repairResult{} // silence if unused (kept local for future explicit handling)
	return tea.Sequence(work, flash("repair done"), func() tea.Msg { return reloadConfigMsg{} })
}

func (m projectsModel) view(w, h int) string {
	// lipgloss Width/Height args refer to content+padding (not the outer
	// border). To get a total rendered width of w, pass w-2.
	paneStyle := Styles.PaneActive.Padding(0, 0, 0, 1)
	innerH := h - 2     // pane render height = innerH + 2 = h
	innerW := w - 2     // pane render width = innerW + 2 = w
	contentW := w - 4   // border 2 + left padding 1 + scrollbar 1
	if contentW < 10 {
		contentW = 10
	}

	total := len(m.list.VisibleItems())
	visibleRows := innerH / 2
	bar := scrollbar(innerH, total*2, m.list.Index()*2-visibleRows)

	body := padToWidth(m.list.View(), contentW, innerH)
	combo := lipgloss.JoinHorizontal(lipgloss.Top, body, bar)
	pane := paneStyle.Width(innerW).Height(innerH).Render(clipToHeight(combo, innerH))
	return clipToHeight(pane, h)
}

