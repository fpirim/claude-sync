package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// leftTruncate keeps the right end of s so it fits in w visible columns,
// prepending "…" when content was dropped. ANSI-aware: styling on the kept
// portion is preserved.
func leftTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	width := lipgloss.Width(s)
	if width <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	// Keep the rightmost (w-1) cols.
	keep := ansi.Cut(s, width-(w-1), width)
	return "…" + keep
}

// rightTruncate keeps the left end, ANSI-aware, with "…" suffix.
func rightTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}
