package tui

import (
	"strings"
)

// scrollbar returns a vertical bar of the given height. The "thumb" position
// and size reflect what fraction of `total` is currently visible starting at
// `offset`. When all content fits (total <= height) we return blank-but-sized
// padding so layouts that JoinHorizontal it next to a content column still
// align cleanly.
func scrollbar(height, total, offset int) string {
	if height <= 0 {
		return ""
	}
	if total <= height {
		// All content visible → no thumb, just track-less spacing so the
		// column has consistent width.
		return strings.TrimRight(strings.Repeat(" \n", height), "\n")
	}
	thumbH := height * height / total
	if thumbH < 1 {
		thumbH = 1
	}
	maxStart := total - height
	if maxStart < 1 {
		maxStart = 1
	}
	thumbStart := offset * (height - thumbH) / maxStart
	if thumbStart < 0 {
		thumbStart = 0
	}
	if thumbStart > height-thumbH {
		thumbStart = height - thumbH
	}
	var sb strings.Builder
	for i := 0; i < height; i++ {
		if i >= thumbStart && i < thumbStart+thumbH {
			sb.WriteString("█")
		} else {
			sb.WriteString("│")
		}
		if i < height-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
