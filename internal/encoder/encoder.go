// Package encoder converts a working-directory path into the directory name
// Claude Code uses under ~/.claude/projects/.
//
// Empirical rule (confirmed on macOS, Claude Code 2026-05):
//   - cwd is resolved with realpath (symlinks evaluated)
//   - characters '/' and '.' are replaced with '-'
//   - the result is prefixed with '-'
//
// Example: /private/tmp/test.dir/foo  ->  -private-tmp-test-dir-foo
package encoder

import (
	"os"
	"path/filepath"
	"strings"
)

// osStat is a thin wrapper so ResolveExisting can use the package-level
// indirection while still defaulting to os.Stat.
var osStat = os.Stat

// Encode returns the encoded project directory name for the given absolute path.
// It does NOT resolve symlinks; callers that have a raw cwd should use EncodeResolved.
func Encode(absPath string) string {
	if absPath == "" {
		return ""
	}
	r := strings.NewReplacer("/", "-", ".", "-")
	return r.Replace(absPath)
}

// EncodeResolved resolves symlinks (mirroring Claude Code) before encoding.
// Returns an error only if the path cannot be evaluated.
func EncodeResolved(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Path may not exist yet; fall back to abs.
		resolved = abs
	}
	return Encode(resolved), nil
}

// DecodeCandidates returns plausible source paths for an encoded directory
// name. Encoding is lossy — both '/' and '.' map to '-' — so the encoded
// "-foo-bar-baz" could mean any of /foo/bar/baz, /foo/bar.baz, /foo.bar/baz,
// /foo.bar.baz. The caller (typically) probes each with os.Stat to see which
// actually exists on disk.
//
// To keep the search space bounded, we cap at 10 separator slots (1024
// candidates). Deeper encodings fall back to the all-slashes interpretation.
func DecodeCandidates(enc string) []string {
	s := strings.TrimPrefix(enc, "-")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "-")
	n := len(parts) - 1 // separator count
	if n < 0 {
		return nil
	}
	if n > 10 {
		return []string{"/" + strings.Join(parts, "/")}
	}
	out := make([]string, 0, 1<<n)
	for mask := 0; mask < (1 << n); mask++ {
		var sb strings.Builder
		sb.WriteByte('/')
		sb.WriteString(parts[0])
		for i := 1; i <= n; i++ {
			if mask&(1<<(i-1)) != 0 {
				sb.WriteByte('.')
			} else {
				sb.WriteByte('/')
			}
			sb.WriteString(parts[i])
		}
		out = append(out, sb.String())
	}
	return out
}

// ResolveExisting returns the first decode candidate whose path exists on
// disk, or "" if none does. statFn is exposed for tests; callers normally
// pass nil to use os.Stat.
func ResolveExisting(enc string, statFn func(string) error) string {
	if statFn == nil {
		statFn = func(p string) error {
			_, err := osStat(p)
			return err
		}
	}
	for _, c := range DecodeCandidates(enc) {
		if statFn(c) == nil {
			return c
		}
	}
	return ""
}
