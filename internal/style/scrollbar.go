package style

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	trackChar = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render("░")
	thumbChar = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render("█")
)

// Scrollbar renders a 1-character-wide vertical scrollbar column.
// height is the track height in rows, totalLines is the total content line
// count, and scrollPercent is 0.0–1.0 (from viewport.ScrollPercent()).
// Returns a blank column when content fits without scrolling.
func Scrollbar(height, totalLines int, scrollPercent float64) string {
	lines := make([]string, height)

	if totalLines <= height || height <= 0 {
		for i := range lines {
			lines[i] = " "
		}
		return strings.Join(lines, "\n")
	}

	thumbSize := height * height / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}

	trackSpace := height - thumbSize
	thumbStart := int(scrollPercent * float64(trackSpace))
	if thumbStart > trackSpace {
		thumbStart = trackSpace
	}

	for i := range lines {
		if i >= thumbStart && i < thumbStart+thumbSize {
			lines[i] = thumbChar
		} else {
			lines[i] = trackChar
		}
	}
	return strings.Join(lines, "\n")
}
