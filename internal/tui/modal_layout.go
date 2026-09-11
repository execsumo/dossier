package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	minSupportedTUIWidth  = 48
	minSupportedTUIHeight = 12
)

func terminalTooSmall(width, height int) bool {
	return width > 0 && height > 0 && (width < minSupportedTUIWidth || height < minSupportedTUIHeight)
}

// fitScreen preserves the footer at the bottom while trimming body rows. All
// normal TUI views pass through this final budget guard, so a long status or
// modal cannot silently push quit/save controls below the terminal.
func fitScreen(content string, width, height, footerHeight int) string {
	if width <= 0 || height <= 0 {
		return content
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "…")
		}
	}
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	if footerHeight < 1 || footerHeight >= height {
		lines = lines[:height]
		return strings.Join(lines, "\n")
	}
	bodyEnd := len(lines) - footerHeight
	bodyBudget := height - footerHeight
	if bodyEnd > bodyBudget {
		lines = append(append([]string{}, lines[:bodyBudget]...), lines[bodyEnd:]...)
	}
	return strings.Join(lines, "\n")
}

func renderTooSmall(width, height int, overlay bool) string {
	if width <= 0 || height <= 0 {
		return "Initializing TUI..."
	}
	lines := []string{
		"DOSSIER TUI",
		"",
		"Terminal too small.",
		fmt.Sprintf("Resize to at least %dx%d.", minSupportedTUIWidth, minSupportedTUIHeight),
		"",
		"q quit · ctrl+c quit",
	}
	if overlay {
		lines = append(lines, "esc back")
	}
	for i := range lines {
		lines[i] = truncateCell(lines[i], width)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

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
