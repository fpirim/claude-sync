package tui

import "strings"

// clipToHeight ensures s contains at most h lines. If s has more, the trailing
// lines are dropped — an essential safety net since lipgloss's .Height does
// NOT truncate when content overflows; it just lets the block grow, which
// pushes the surrounding header/footer off-screen.
func clipToHeight(s string, h int) string {
	if h <= 0 {
		return ""
	}
	n := strings.Count(s, "\n") + 1
	if n <= h {
		return s
	}
	lines := strings.SplitN(s, "\n", h+1)
	return strings.Join(lines[:h], "\n")
}
