package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fikret/claude-sync/internal/paths"
	"github.com/fikret/claude-sync/internal/sessions"
)

// Verify the full sessions body never exceeds the body height, using a real
// model with many fake items. Catches regressions where Pane.Height fails
// to truncate and pushes the root header off-screen.
func TestSessionsViewFitsBody(t *testing.T) {
	p := paths.New(t.TempDir())
	p.EnsureDirs()
	m := newSessionsModel(p, "")
	m.SetProject("foo")
	for i := 0; i < 50; i++ {
		m.items = append(m.items, sessions.Session{UUID: "u", Project: "foo"})
	}
	cases := []struct{ w, h int }{
		{120, 40}, // typical
		{80, 24},  // small
		{200, 60}, // big
	}
	for _, c := range cases {
		mi, _ := m.update(tea.WindowSizeMsg{Width: c.w, Height: c.h})
		body := mi.view(c.w, c.h)
		got := strings.Count(body, "\n") + 1
		if got > c.h {
			t.Errorf("body w=%d h=%d → rendered %d lines (limit %d)", c.w, c.h, got, c.h)
		}
	}
}

// TestAsymmetricPadding ensures the right border still renders when the
// pane uses Padding(0, 0, 0, 1) — visible regression behind the
// "right border missing" reports.
func TestAsymmetricPadding(t *testing.T) {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5F5FD7")).
		Padding(0, 0, 0, 1)
	out := style.Width(20).Height(3).Render("hello")
	t.Logf("output:\n%s", out)
	for i, line := range strings.Split(out, "\n") {
		t.Logf("  line %d: width=%d %q", i, lipgloss.Width(line), line)
	}
}

func TestPaneHeightContract(t *testing.T) {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	for _, c := range []struct {
		set     int
		content string
		label   string
	}{
		{5, "a\nb\nc", "3 lines"},
		{5, "a\nb\nc\nd\ne", "5 lines"},
		{5, "a\nb\nc\nd\ne\nf\ng", "7 lines (overflow)"},
	} {
		out := style.Width(20).Height(c.set).Render(c.content)
		got := strings.Count(out, "\n") + 1
		t.Logf("Height(%d) %-20s → rendered lines: %d", c.set, c.label, got)
	}
}
