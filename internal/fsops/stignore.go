package fsops

import (
	"bytes"
	"fmt"
	"os"
)

// StIgnoreContent is the canonical .stignore for the ~/.claude folder.
// Order matters: '!' negations must come AFTER the broader ignore.
const StIgnoreContent = `// managed by claude-sync — do not edit by hand
.credentials.json
// Re-include _shared/ BEFORE the broad projects/ ignore. The "**" suffix
// matters: a bare "!projects/_shared/" matches only the directory, leaving
// files inside still subject to the projects/ rule.
!projects/_shared/**
projects/
statsig/
shell-snapshots/
sync/machine
sync/.lock
sync/.syncthing-key
sync/claude-sync.log
plans/
ide/
plugins/
*.tmp
*.bak
.sync-conflict-*
`

// EnsureStIgnore writes path with the canonical content if it differs.
// Returns true if the file was created or updated.
func EnsureStIgnore(path string) (bool, error) {
	want := []byte(StIgnoreContent)
	got, err := os.ReadFile(path)
	if err == nil && bytes.Equal(got, want) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return true, os.WriteFile(path, want, 0o644)
}
