package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// joinModalColumns joins side-by-side modal boxes while keeping every cell
// belonging to the layout on the modal background. lipgloss.JoinHorizontal
// uses plain spaces when it pads a shorter block; inside a layered modal those
// spaces can expose the terminal's background (and look like black patches).
func joinModalColumns(columns ...string) string {
	if len(columns) == 0 {
		return ""
	}
	if len(columns) == 1 {
		return columns[0]
	}

	blocks := make([][]string, len(columns))
	widths := make([]int, len(columns))
	maxHeight := 0
	for i, column := range columns {
		blocks[i] = strings.Split(column, "\n")
		widths[i] = lipgloss.Width(column)
		if len(blocks[i]) > maxHeight {
			maxHeight = len(blocks[i])
		}
	}

	var joined strings.Builder
	for row := 0; row < maxHeight; row++ {
		for column, lines := range blocks {
			line := ""
			if row < len(lines) {
				line = lines[row]
			}
			joined.WriteString(line)
			if padding := widths[column] - lipgloss.Width(line); padding > 0 {
				joined.WriteString(modalFillStyle.Render(strings.Repeat(" ", padding)))
			}
		}
		if row < maxHeight-1 {
			joined.WriteByte('\n')
		}
	}
	return joined.String()
}
