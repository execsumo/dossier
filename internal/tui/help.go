package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpSection is one titled group of bindings in the ? overlay. Grouping by
// where a key works — rather than by what it does — is what lets a footer stay
// short: the overlay is where "which surface owns this verb" gets answered.
type helpSection struct {
	title string
	keys  [][2]string // {key, what it does}
}

// helpSections is the single source of truth for the overlay. Navigation keys
// are deliberately absent: arrows, enter and esc explain themselves, and listing
// them would cost the overlay the height it needs to fit an 80x24 terminal.
var helpSections = []helpSection{
	{"Anywhere", [][2]string{
		{"e", "edit stage, priority,"},
		{"", "due, lead, next action"},
		{"c", "open in Claude"},
		{"q", "quit"},
		{"?", "this help"},
	}},
	{"Dashboard and board", [][2]string{
		{"/", "search as you type"},
		{"f", "filter by lead"},
		{"i", "cycle the interface filter"},
		{"b", "switch table / board"},
	}},
	{"Dashboard only", [][2]string{
		{"k", "link to another dossier"},
		{"m", "merge into another dossier"},
	}},
	{"Detail only", [][2]string{
		{"r", "rename title or canonical slug"},
		{"a", "browse archived artifacts"},
		{"o", "open dossier.md in $EDITOR"},
	}},
}

const (
	// helpKeyWidth is the gutter the key glyph sits in, wide enough for "ctrl+f"
	// should a chord ever be listed.
	helpKeyWidth = 8
	// helpColumnGutter separates the two columns when both fit.
	helpColumnGutter = 4
	// helpMinColumnWidth is the narrowest a column may get before the overlay
	// gives up on two of them and stacks the sections instead. It is set so an
	// 80x24 terminal — and a cramped 60-wide one — still splits, because a
	// single column does not fit 24 rows of screen.
	helpMinColumnWidth = 28
)

// renderHelpSection lays one section out as title + indented rows, each fitted
// to width so a long description can never wrap and push the footer off screen.
func renderHelpSection(sec helpSection, width int) []string {
	lines := make([]string, 0, len(sec.keys)+1)
	lines = append(lines, metaLabelStyle.Render(truncateCell(" "+sec.title, width)))
	for _, kv := range sec.keys {
		row := "  " + padCell(kv[0], helpKeyWidth) + kv[1]
		lines = append(lines, mutedStyle.Render(truncateCell(row, width)))
	}
	return lines
}

// helpColumns splits the sections into the columns that fit at width. Two
// columns is the point of the split: it keeps the whole reference on one screen
// at 80x24, which is what makes "any key closes it" an honest promise.
func helpColumns(width int) [][]helpSection {
	if width < helpMinColumnWidth*2+helpColumnGutter {
		return [][]helpSection{helpSections}
	}
	// Balance by rendered height rather than section count, so the two columns
	// stay even as sections gain or lose keys.
	total := 0
	for _, sec := range helpSections {
		total += len(sec.keys) + 2 // title + keys + trailing blank
	}
	left, right := []helpSection{}, []helpSection{}
	used := 0
	for _, sec := range helpSections {
		if used*2 < total {
			left = append(left, sec)
			used += len(sec.keys) + 2
			continue
		}
		right = append(right, sec)
	}
	if len(right) == 0 {
		return [][]helpSection{helpSections}
	}
	return [][]helpSection{left, right}
}

// helpBodyHeight mirrors recalculateTableLayout's arithmetic (the 4 lines of
// screen chrome plus the footer) so the overlay converges on the bottom of the
// terminal exactly as the dashboard and board do.
func (m Model) helpBodyHeight() int {
	h := m.height - 4 - m.footerHeight(ViewHelp)
	if h < 3 {
		h = 3
	}
	return h
}

// renderHelp draws the keybinding reference. It is pure over the model and, like
// the board, safe at any terminal size — a degenerate width stacks rather than
// panics.
func (m Model) renderHelp() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	cols := helpColumns(w)
	colWidth := w
	if len(cols) > 1 {
		colWidth = (w - (len(cols)-1)*helpColumnGutter) / len(cols)
	}

	blocks := make([]string, 0, len(cols))
	for _, sections := range cols {
		var lines []string
		for i, sec := range sections {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, renderHelpSection(sec, colWidth)...)
		}
		for i, line := range lines {
			lines[i] = padCell(line, colWidth)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	gutter := strings.Repeat(" ", helpColumnGutter)
	joined := make([]string, 0, len(blocks)*2-1)
	for i, b := range blocks {
		if i > 0 {
			joined = append(joined, gutter)
		}
		joined = append(joined, b)
	}

	// Pad to the body budget so the footer sits on the bottom line. Clipping is
	// a last-resort guard for a terminal too short to hold the reference at all;
	// the two-column split above normally makes it a no-op.
	lines := strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, joined...), "\n")
	budget := m.helpBodyHeight()
	if len(lines) > budget {
		lines = lines[:budget]
	}
	for len(lines) < budget {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
