// Package machine resolves the local machine's identifier used as a key in
// config.yml's per-project paths map.
//
// Resolution order:
//  1. ~/.claude/sync/machine file (first non-empty line, trimmed)
//  2. os.Hostname()
//
// Hostnames are sanitized: lowercased and any character outside [a-z0-9-_]
// is replaced with '-'. This keeps keys stable across machines that report
// e.g. "MacBook-Pro.local".
package machine

import (
	"os"
	"strings"
)

// Resolve returns the machine name. machineFile is the path to the override
// file; if empty or missing, hostname is used.
func Resolve(machineFile string) (string, error) {
	if machineFile != "" {
		b, err := os.ReadFile(machineFile)
		if err == nil {
			if name := firstLine(string(b)); name != "" {
				return Sanitize(name), nil
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	h, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return Sanitize(h), nil
}

// Save writes the override file with the given name.
func Save(machineFile, name string) error {
	return os.WriteFile(machineFile, []byte(Sanitize(name)+"\n"), 0o644)
}

// Sanitize normalizes a machine name to a stable, filesystem-safe key.
func Sanitize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	// Strip a trailing .local (common on macOS).
	s = strings.TrimSuffix(s, ".local")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
