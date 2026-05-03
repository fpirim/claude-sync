// Package paths centralizes locations under the user's ~/.claude directory.
package paths

import (
	"os"
	"path/filepath"
)

// Paths bundles all directories and files claude-sync touches.
type Paths struct {
	Home       string // ~/.claude
	Projects   string // ~/.claude/projects
	Shared     string // ~/.claude/projects/_shared
	Memory     string // ~/.claude/memory
	Todos      string // ~/.claude/todos
	Commands   string // ~/.claude/commands
	Settings   string // ~/.claude/settings.json
	Sync       string // ~/.claude/sync
	Config     string // ~/.claude/sync/config.yml
	Machine    string // ~/.claude/sync/machine
	Lock       string // ~/.claude/sync/.lock
	Log        string // ~/.claude/sync/claude-sync.log
	StIgnore   string // ~/.claude/.stignore
}

// Default returns the standard Paths rooted at $HOME/.claude.
// HOME override (CLAUDE_HOME) is honored for tests.
func Default() (Paths, error) {
	home := os.Getenv("CLAUDE_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		home = filepath.Join(h, ".claude")
	}
	return New(home), nil
}

// New builds a Paths struct rooted at the given .claude home.
func New(home string) Paths {
	return Paths{
		Home:     home,
		Projects: filepath.Join(home, "projects"),
		Shared:   filepath.Join(home, "projects", "_shared"),
		Memory:   filepath.Join(home, "memory"),
		Todos:    filepath.Join(home, "todos"),
		Commands: filepath.Join(home, "commands"),
		Settings: filepath.Join(home, "settings.json"),
		Sync:     filepath.Join(home, "sync"),
		Config:   filepath.Join(home, "sync", "config.yml"),
		Machine:  filepath.Join(home, "sync", "machine"),
		Lock:     filepath.Join(home, "sync", ".lock"),
		Log:      filepath.Join(home, "sync", "claude-sync.log"),
		StIgnore: filepath.Join(home, ".stignore"),
	}
}

// EnsureDirs creates all directories claude-sync expects to exist.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.Projects, p.Shared, p.Sync} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// SharedProject returns the canonical _shared/<name> directory for a project.
func (p Paths) SharedProject(name string) string {
	return filepath.Join(p.Shared, name)
}

// ProjectLink returns the encoded ~/.claude/projects/<encoded> path.
func (p Paths) ProjectLink(encoded string) string {
	return filepath.Join(p.Projects, encoded)
}
