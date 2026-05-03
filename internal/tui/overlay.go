package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// overlay places fg on top of bg starting at (x, y), preserving any ANSI
// styling on bg outside the fg footprint. Both strings may contain colored
// text. Lines in bg shorter than (x + fg-line-width) are right-padded with
// spaces so the modal lands cleanly even on narrow rows.
func overlay(bg, fg string, x, y int) string {
	if fg == "" {
		return bg
	}
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	fgW := lipgloss.Width(fg)

	for i, fgLine := range fgLines {
		row := y + i
		if row < 0 {
			continue
		}
		// Extend bg with blank lines if the modal pokes past its bottom.
		for len(bgLines) <= row {
			bgLines = append(bgLines, "")
		}
		bgLine := bgLines[row]
		bgW := lipgloss.Width(bgLine)
		if bgW < x+fgW {
			bgLine += strings.Repeat(" ", x+fgW-bgW)
		}
		// ansi.Cut(s, lo, hi) keeps the visible columns [lo, hi).
		left := ansi.Cut(bgLine, 0, x)
		right := ansi.Cut(bgLine, x+fgW, lipgloss.Width(bgLine))
		bgLines[row] = left + fgLine + right
	}
	return strings.Join(bgLines, "\n")
}

// centerOverlay overlays fg centered inside a w×h area on top of bg.
func centerOverlay(bg, fg string, w, h int) string {
	fgW := lipgloss.Width(fg)
	fgH := strings.Count(fg, "\n") + 1
	x := (w - fgW) / 2
	y := (h - fgH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return overlay(bg, fg, x, y)
}
