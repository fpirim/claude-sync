package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fikret/claude-sync/internal/config"
	"github.com/fikret/claude-sync/internal/paths"
	"github.com/fikret/claude-sync/internal/syncthing"
)

// syncModel is the Tab 3 state. It auto-discovers the local Syncthing
// instance, polls a tight set of REST endpoints, and renders the result.
type syncModel struct {
	p   paths.Paths
	cfg *config.Config // pointer so it can be refreshed by root

	w, h int

	client    *syncthing.Client
	clientErr error // last error from discovery / fetch (shown in UI)

	// Cached snapshot — populated by tickMsg.
	st       syncthing.SystemStatus
	ver      syncthing.SystemVersion
	folder   syncthing.FolderStatus
	conns    syncthing.Connections
	devices  []syncthing.Device
	folderID string
	lastFetch time.Time
	completion map[string]float64 // peer device id → completion %
}

func newSyncModel(p paths.Paths, cfg *config.Config) syncModel {
	return syncModel{
		p:        p,
		cfg:      cfg,
		folderID: defaultFolderID(cfg),
		completion: map[string]float64{},
	}
}

func defaultFolderID(cfg *config.Config) string {
	if cfg != nil && cfg.Syncthing.FolderID != "" {
		return cfg.Syncthing.FolderID
	}
	return "claude-home"
}

func (m syncModel) Init() tea.Cmd {
	return tea.Batch(m.connectCmd(), tickCmd())
}

// syncTickMsg fires periodically while the Sync tab is mounted; we use it as
// a poll cadence. The view ignores ticks while not active to avoid wasted
// HTTP traffic, but the framework keeps the timer alive cheaply enough.
type syncTickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return syncTickMsg(t) })
}

// syncSnapshotMsg carries the result of a poll cycle. nil error → all five
// fields populated. err set → discovery or REST call failed.
type syncSnapshotMsg struct {
	st       syncthing.SystemStatus
	ver      syncthing.SystemVersion
	folder   syncthing.FolderStatus
	conns    syncthing.Connections
	devices  []syncthing.Device
	completion map[string]float64
	err      error
}

// connectCmd does discovery once and emits a snapshot. We re-use it on every
// poll cycle — discovery is cheap (a couple of file reads from disk-cache)
// and self-heals if the user (re)starts Syncthing while the TUI is open.
func (m syncModel) connectCmd() tea.Cmd {
	folderID := m.folderID
	return func() tea.Msg {
		base, key, err := syncthing.Discover()
		if err != nil {
			return syncSnapshotMsg{err: fmt.Errorf("discover: %w", err)}
		}
		c := syncthing.New(base, key)
		if err := c.Ping(); err != nil {
			return syncSnapshotMsg{err: fmt.Errorf("ping: %w", err)}
		}
		var snap syncSnapshotMsg
		snap.st, _ = c.SystemStatus()
		snap.ver, _ = c.SystemVersion()
		snap.folder, _ = c.FolderStatus(folderID)
		snap.conns, _ = c.Connections()
		snap.devices, _ = c.Devices()

		// Per-peer completion (skip self).
		snap.completion = map[string]float64{}
		for _, d := range snap.devices {
			if d.DeviceID == snap.st.MyID || d.DeviceID == "" {
				continue
			}
			cm, err := c.Completion(folderID, d.DeviceID)
			if err == nil {
				snap.completion[d.DeviceID] = cm.Completion
			}
		}
		return snap
	}
}

func (m syncModel) update(msg tea.Msg) (syncModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil

	case syncTickMsg:
		// Poll + schedule the next tick.
		return m, tea.Batch(m.connectCmd(), tickCmd())

	case syncSnapshotMsg:
		m.lastFetch = time.Now()
		if msg.err != nil {
			m.clientErr = msg.err
			return m, nil
		}
		m.clientErr = nil
		m.st = msg.st
		m.ver = msg.ver
		m.folder = msg.folder
		m.conns = msg.conns
		m.devices = msg.devices
		m.completion = msg.completion
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			// Manual rescan trigger.
			folderID := m.folderID
			return m, tea.Sequence(
				func() tea.Msg {
					base, key, err := syncthing.Discover()
					if err != nil {
						return flashMsg{text: "no syncthing", err: true}
					}
					c := syncthing.New(base, key)
					if err := c.Scan(folderID); err != nil {
						return flashMsg{text: err.Error(), err: true}
					}
					return nil
				},
				flash("rescan triggered"),
				m.connectCmd(),
			)
		}
	}
	return m, nil
}

func (m syncModel) view(w, h int) string {
	innerH := h - 2
	innerW := w - 2

	var body string
	if m.clientErr != nil {
		body = m.errorView()
	} else if m.lastFetch.IsZero() {
		body = Styles.Muted.Render("connecting to local Syncthing…")
	} else {
		body = m.normalView(innerW - 2) // -2 accounts for left padding + scrollbar (not used here, but uniform)
	}

	pane := Styles.PaneActive.
		Padding(0, 0, 0, 1).
		Width(innerW).
		Height(innerH).
		Render(clipToHeight(body, innerH))
	return clipToHeight(pane, h)
}

func (m syncModel) errorView() string {
	var sb strings.Builder
	sb.WriteString(Styles.Header.Render("Sync — unavailable") + "\n\n")
	sb.WriteString(Styles.Err.Render(m.clientErr.Error()) + "\n\n")
	sb.WriteString(Styles.Muted.Render("Hints:\n"))
	sb.WriteString(Styles.Muted.Render("  · is `syncthing` running? (`brew services list` / `systemctl --user status syncthing`)\n"))
	sb.WriteString(Styles.Muted.Render("  · is the API key reachable in config.xml?\n"))
	sb.WriteString(Styles.Muted.Render("  · re-run `make setup` if first-time install\n"))
	return sb.String()
}

func (m syncModel) normalView(innerW int) string {
	var sb strings.Builder

	// --- Self ---
	myShort := shortID(m.st.MyID)
	ver := m.ver.Version
	if ver == "" {
		ver = "unknown"
	}
	sb.WriteString(Styles.Header.Render("This device") + "\n")
	sb.WriteString(fmt.Sprintf("  %s  %s  uptime %s\n",
		Styles.Key.Render(myShort), Styles.Muted.Render(ver), formatUptime(m.st.Uptime)))
	sb.WriteString("\n")

	// --- Folder ---
	state := m.folder.State
	stateStyle := Styles.Muted
	switch state {
	case "idle":
		stateStyle = Styles.OK
	case "syncing", "scanning":
		stateStyle = Styles.Warn
	case "error":
		stateStyle = Styles.Err
	}
	sb.WriteString(Styles.Header.Render("Folder ") + Styles.Key.Render(m.folderID) + "\n")
	sb.WriteString(fmt.Sprintf("  state: %s   global: %d files / %s   local: %d / %s\n",
		stateStyle.Render(state),
		m.folder.GlobalFiles, humanBytes(m.folder.GlobalBytes),
		m.folder.LocalFiles, humanBytes(m.folder.LocalBytes),
	))
	if m.folder.NeedFiles > 0 || m.folder.NeedBytes > 0 {
		sb.WriteString(fmt.Sprintf("  need: %d files / %s\n",
			m.folder.NeedFiles, humanBytes(m.folder.NeedBytes)))
	}
	if m.folder.Errors > 0 {
		sb.WriteString(Styles.Err.Render(fmt.Sprintf("  errors: %d\n", m.folder.Errors)))
	}
	sb.WriteString("\n")

	// --- Peers ---
	sb.WriteString(Styles.Header.Render("Peers") + "\n")
	peers := m.peerRows()
	if len(peers) == 0 {
		sb.WriteString(Styles.Muted.Render("  (no peers configured)\n"))
	} else {
		for _, p := range peers {
			sb.WriteString("  " + p + "\n")
		}
	}
	sb.WriteString("\n")
	sb.WriteString(Styles.Muted.Render(fmt.Sprintf("  last poll: %s ago  ·  s = rescan",
		time.Since(m.lastFetch).Round(time.Second))))
	return sb.String()
}

// peerRows builds one rendered line per peer (excluding self).
func (m syncModel) peerRows() []string {
	type row struct{ name, id, status, address string; pct float64 }
	var rows []row
	for _, d := range m.devices {
		if d.DeviceID == m.st.MyID {
			continue
		}
		c := m.conns.Connections[d.DeviceID]
		status := "offline"
		if c.Connected {
			status = "online"
		}
		if c.Paused {
			status = "paused"
		}
		rows = append(rows, row{
			name:    d.Name,
			id:      shortID(d.DeviceID),
			status:  status,
			address: c.Address,
			pct:     m.completion[d.DeviceID],
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	out := make([]string, 0, len(rows))
	for _, r := range rows {
		dot := Styles.Muted.Render("○")
		statusStyle := Styles.Muted
		if r.status == "online" {
			dot = Styles.OK.Render("●")
			statusStyle = Styles.OK
		}
		if r.status == "paused" {
			dot = Styles.Warn.Render("◌")
			statusStyle = Styles.Warn
		}
		pct := "—"
		if r.status == "online" {
			pct = fmt.Sprintf("%3.0f%%", r.pct)
		}
		addr := r.address
		if addr == "" {
			addr = "(no address)"
		}
		out = append(out, fmt.Sprintf("%s %-12s  %s  %s  %s  %s",
			dot,
			truncRune(r.name, 12),
			Styles.Muted.Render(r.id),
			statusStyle.Render(fmt.Sprintf("%-7s", r.status)),
			pct,
			Styles.Muted.Render(addr),
		))
	}
	return out
}

// shortID keeps the leading group of a Syncthing device ID (7 chars + ellipsis).
func shortID(id string) string {
	if id == "" {
		return "—"
	}
	if i := strings.IndexByte(id, '-'); i > 0 {
		return id[:i] + "…"
	}
	if len(id) > 8 {
		return id[:8] + "…"
	}
	return id
}

// formatUptime renders Syncthing's seconds-since-start as "1h 23m" etc.
func formatUptime(secs int) string {
	if secs <= 0 {
		return "?"
	}
	d := time.Duration(secs) * time.Second
	if d > 24*time.Hour {
		return fmt.Sprintf("%dd %dh", int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour))
	}
	if d > time.Hour {
		return fmt.Sprintf("%dh %dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
	}
	if d > time.Minute {
		return fmt.Sprintf("%dm %ds", int(d/time.Minute), int(d%time.Minute/time.Second))
	}
	return d.String()
}

// humanBytes renders byte counts in a compact form.
func humanBytes(b int64) string {
	const k = 1024
	switch {
	case b < k:
		return fmt.Sprintf("%dB", b)
	case b < k*k:
		return fmt.Sprintf("%.1fKB", float64(b)/k)
	case b < k*k*k:
		return fmt.Sprintf("%.1fMB", float64(b)/(k*k))
	default:
		return fmt.Sprintf("%.2fGB", float64(b)/(k*k*k))
	}
}

func truncRune(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Force-include lipgloss to silence unused-import linting if styling is
// removed during edits; the package is referenced via Styles.* above.
var _ = lipgloss.RoundedBorder
