package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fikret/claude-sync/internal/config"
	"github.com/fikret/claude-sync/internal/paths"
)

func TestRootRenders(t *testing.T) {
	p := paths.New(t.TempDir())
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Projects["foo"] = config.Project{Paths: map[string]string{"test-host": "/Users/fikret/foo"}}

	m := New(p, cfg, "test-host")
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mi.(Model)

	out := m.View()
	for _, want := range []string{"claude-sync", "Projects", "Sessions", "Config", "test-host"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestTabSwitch(t *testing.T) {
	p := paths.New(t.TempDir())
	p.EnsureDirs()
	m := New(p, config.Default(), "test-host")
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = mi.(Model)

	mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if mi.(Model).active != TabSessions {
		t.Errorf("expected TabSessions, got %v", mi.(Model).active)
	}
	mi, _ = mi.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	if mi.(Model).active != TabConfig {
		t.Errorf("expected TabConfig, got %v", mi.(Model).active)
	}
}
