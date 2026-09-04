package tui

// Kanban board — the second home surface.
//
// The board answers a question the dashboard table cannot: "where is everything
// stuck?". It is a pure projection of the same filtered dossier set the
// dashboard shows, bucketed by canonical stage, so it introduces no state of its
// own beyond a cursor. Grouping happens once per data/filter change in
// applyFilters, never per frame; everything in this file is either pure or a
// read-only render over the model.
//
// Two deliberate differences from the dashboard:
//   - the board ignores the dashboard's "extras collapsed" rule. A Done column
//     that hid done work would misname itself.
//   - Done cards show the title only. Terminal work costs vertical space that
//     open work needs more, and a finished dossier's summary is one keypress
//     away in the detail view.

import (
	"fmt"
	"strconv"
	"strings"

	"dossier/internal/core"

	"github.com/charmbracelet/lipgloss"
)

const (
	// kanbanGutter is the blank column between two stage columns.
	kanbanGutter = 1
	// kanbanMinColWidth is the narrowest column that still fits a bordered card
	// with a readable title. Below this the board windows stages instead of
	// shrinking them, so cards never degrade into unreadable slivers.
	kanbanMinColWidth = 20
	// kanbanMaxDescLines caps a card's summary so one verbose dossier cannot
	// push every other card in its column off-screen.
	kanbanMaxDescLines = 3
)

var (
	kanbanCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(darkGray).
			Padding(0, 1)

	kanbanCardSelectedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(purple).
				Padding(0, 1)

	kanbanCardTitleStyle = lipgloss.NewStyle().Bold(true)
)

// stageStyle maps a lifecycle status to its accent colour. Legacy statuses share
// the style of the canonical stage they normalize to, so a store written before
// the rename still colours consistently.
func stageStyle(s core.Status) lipgloss.Style {
	switch core.NormalizeStatus(s) {
	case core.StatusSpark:
		return statusSparkStyle
	case core.StatusDefine:
		return statusDefineStyle
	case core.StatusDelegated:
		return statusDelegatedStyle
	case core.StatusReview:
		return statusReviewStyle
	case core.StatusBlocked:
		return statusBlockedStyle
	case core.StatusDone:
		return statusDoneStyle
	}
	return metaValueStyle
}

// groupByStage buckets items into one slice per canonical stage, in lifecycle
// order. Legacy statuses are normalized first so active/waiting/resolved/archived
// dossiers land in define/delegated/done/done rather than vanishing. Placeholder
// rows (empty ID) and unrecognisable statuses are dropped — the board only shows
// work it can honestly place. Input order is preserved within a column, which is
// what makes the list's tier/priority/due sort carry over for free.
func groupByStage(items []core.ListItem) [][]core.ListItem {
	stages := core.CanonicalStatuses()
	index := make(map[core.Status]int, len(stages))
	for i, s := range stages {
		index[s] = i
	}

	cols := make([][]core.ListItem, len(stages))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		stage := core.NormalizeStatus(core.Status(item.Status))
		i, ok := index[stage]
		if !ok {
			continue
		}
		cols[i] = append(cols[i], item)
	}
	return cols
}

// clampKanbanCursor keeps the selection inside the board after the columns are
// rebuilt — a filter change can empty the column the cursor was parked in.
func (m *Model) clampKanbanCursor() {
	stages := len(core.CanonicalStatuses())
	if m.kanbanCol < 0 {
		m.kanbanCol = 0
	}
	if m.kanbanCol > stages-1 {
		m.kanbanCol = stages - 1
	}
	rows := 0
	if m.kanbanCol < len(m.kanbanColumns) {
		rows = len(m.kanbanColumns[m.kanbanCol])
	}
	if m.kanbanRow > rows-1 {
		m.kanbanRow = rows - 1
	}
	if m.kanbanRow < 0 {
		m.kanbanRow = 0
	}
}

// moveKanbanColumn steps the cursor sideways. A board has edges, so this clamps
// rather than wrapping: overshooting left should not teleport the user to Done.
func (m *Model) moveKanbanColumn(delta int) {
	m.kanbanCol += delta
	m.clampKanbanCursor()
}

// moveKanbanRow steps the cursor within the current column, clamped. On an empty
// column it is a no-op because clampKanbanCursor pins the row at 0.
func (m *Model) moveKanbanRow(delta int) {
	m.kanbanRow += delta
	m.clampKanbanCursor()
}

// selectedKanbanItem returns the card under the cursor, or false when the
// selected column is empty.
func (m Model) selectedKanbanItem() (core.ListItem, bool) {
	if m.kanbanCol < 0 || m.kanbanCol >= len(m.kanbanColumns) {
		return core.ListItem{}, false
	}
	col := m.kanbanColumns[m.kanbanCol]
	if m.kanbanRow < 0 || m.kanbanRow >= len(col) {
		return core.ListItem{}, false
	}
	return col[m.kanbanRow], true
}

// kanbanIsEmpty reports whether the active filters left nothing to show.
func (m Model) kanbanIsEmpty() bool {
	for _, col := range m.kanbanColumns {
		if len(col) > 0 {
			return false
		}
	}
	return true
}

// kanbanVisibleColumns is how many stage columns fit at the current width, at
// the minimum readable column width, clamped to at least one and at most the
// number of stages.
func (m Model) kanbanVisibleColumns() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	n := (w + kanbanGutter) / (kanbanMinColWidth + kanbanGutter)
	if n < 1 {
		n = 1
	}
	if stages := len(core.CanonicalStatuses()); n > stages {
		n = stages
	}
	return n
}

// kanbanStageWindow is the [start, end) slice of stages on screen. It reuses the
// selector's centering so a narrow terminal scrolls stages the same way a long
// list scrolls rows, always keeping the selected column in view.
func (m Model) kanbanStageWindow() (start, end int) {
	return centeredWindow(len(core.CanonicalStatuses()), m.kanbanCol, m.kanbanVisibleColumns())
}

// kanbanColumnWidth divides the terminal between the visible columns and their
// gutters. The floor keeps degenerate terminal sizes from producing negative
// widths; the board will simply overflow rather than panic.
func (m Model) kanbanColumnWidth(visible int) int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	if visible < 1 {
		visible = 1
	}
	colWidth := (w - (visible-1)*kanbanGutter) / visible
	if colWidth < 6 {
		colWidth = 6
	}
	return colWidth
}

// kanbanBodyHeight is the number of lines available for cards beneath each
// column's header and rule. It mirrors recalculateTableLayout's arithmetic (the
// 4 lines of screen chrome, the footer, and the search bar when it is showing)
// and subtracts the board's own two header lines, so the footer converges on the
// bottom of the terminal exactly as it does on the dashboard.
func (m Model) kanbanBodyHeight() int {
	searchH := 0
	if m.searchBarVisible() {
		searchH = 1
	}
	h := m.height - 4 - m.footerHeight(ViewKanban) - searchH - 2
	if h < 3 {
		h = 3
	}
	return h
}

// fitCards picks the window [start, end) of a column's cards that fits budget
// lines while keeping the card at cursor visible. It grows downward first, then
// upward, so a selected card near the top keeps its successors in view.
//
// The cursor's card is always included even when it alone exceeds the budget —
// losing the selection would be worse than overflowing by a line. Callers pass
// cursor = 0 for unselected columns, which reads as "show from the top".
func fitCards(heights []int, cursor, budget int) (start, end int) {
	n := len(heights)
	if n == 0 {
		return 0, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= n {
		cursor = n - 1
	}

	start, end = cursor, cursor+1
	used := heights[cursor]
	for {
		grew := false
		if end < n && used+heights[end] <= budget {
			used += heights[end]
			end++
			grew = true
		}
		if start > 0 && used+heights[start-1] <= budget {
			used += heights[start-1]
			start--
			grew = true
		}
		if !grew {
			return start, end
		}
	}
}

// truncateCell shortens s to at most w display cells, marking the cut with an
// ellipsis. Width is measured with lipgloss so wide runes are counted the way
// the terminal draws them, never with len().
func truncateCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return strings.TrimRight(b.String(), " ") + "…"
}

// ellipsize is truncateCell for content that is known to continue: the result
// always ends in an ellipsis, even when s happened to fit exactly.
func ellipsize(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return truncateCell(s+"…", w)
}

// wrapCell word-wraps plain text to w cells and returns the lines with their
// padding stripped, so callers control the final padding themselves.
func wrapCell(s string, w int) []string {
	if w <= 0 || s == "" {
		return nil
	}
	lines := strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return lines
}

// padCell right-pads s to exactly w cells so columns stay aligned when joined.
// Over-wide input is left alone: silently cutting a styled string would break
// its ANSI sequences.
func padCell(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// renderCard draws one dossier as a bordered card exactly colWidth cells wide.
// The border consumes two cells and the padding two more, so the text has
// colWidth-4 to work with.
func renderCard(item core.ListItem, colWidth int, selected, showDescription bool) string {
	inner := colWidth - 4
	if inner < 1 {
		inner = 1
	}

	lines := []string{kanbanCardTitleStyle.Render(truncateCell(item.Name, inner))}
	if showDescription && item.Description != "" {
		desc := wrapCell(item.Description, inner)
		if len(desc) > kanbanMaxDescLines {
			desc = desc[:kanbanMaxDescLines]
			desc[len(desc)-1] = ellipsize(desc[len(desc)-1], inner)
		}
		for _, line := range desc {
			lines = append(lines, mutedStyle.Render(line))
		}
	}
	if lead := strings.Fields(item.Lead); len(lead) > 0 {
		lines = append(lines, mutedStyle.Render(truncateCell(lead[0], inner)))
	}

	style := kanbanCardStyle
	if selected {
		style = kanbanCardSelectedStyle
	}
	boxWidth := colWidth - 2
	if boxWidth < 1 {
		boxWidth = 1
	}
	return style.Width(boxWidth).Render(strings.Join(lines, "\n"))
}

// renderKanbanColumn builds one stage column as exactly 2+bodyHeight padded
// lines: the stage header, its rule, and the card body. Returning a fixed line
// count is what lets the columns be joined without any of them shifting the
// footer.
func (m Model) renderKanbanColumn(stageIdx, colWidth, bodyHeight int, selected bool) []string {
	stages := core.CanonicalStatuses()
	stage := stages[stageIdx]

	var items []core.ListItem
	if stageIdx < len(m.kanbanColumns) {
		items = m.kanbanColumns[stageIdx]
	}

	label := strings.ToUpper(string(stage))
	count := strconv.Itoa(len(items))
	header := stageStyle(stage).Render(label) + "  " + mutedStyle.Render(count)
	if lipgloss.Width(label+"  "+count) > colWidth {
		header = stageStyle(stage).Render(truncateCell(label, colWidth))
	}

	ruleColor := darkGray
	if selected {
		ruleColor = purple
	}
	rule := lipgloss.NewStyle().Foreground(ruleColor).Render(strings.Repeat("─", colWidth))

	var body []string
	if len(items) == 0 {
		body = append(body, mutedStyle.Render("No dossiers"))
	} else {
		cursor := 0
		if selected {
			cursor = m.kanbanRow
		}
		cards := make([]string, len(items))
		heights := make([]int, len(items))
		for i, item := range items {
			cards[i] = renderCard(item, colWidth, selected && i == cursor, stage != core.StatusDone)
			heights[i] = lipgloss.Height(cards[i])
		}

		// The "N more" markers cost lines too, so budget for them and refit once
		// they are known to be needed. Without this the footer would be pushed
		// off the bottom of the screen exactly when the board is busiest.
		start, end := fitCards(heights, cursor, bodyHeight)
		if reserve := clippedMarkerLines(start, end, len(cards)); reserve > 0 {
			start, end = fitCards(heights, cursor, bodyHeight-reserve)
		}

		if start > 0 {
			body = append(body, mutedStyle.Render(fmt.Sprintf("↑ %d more", start)))
		}
		for i := start; i < end; i++ {
			body = append(body, strings.Split(cards[i], "\n")...)
		}
		if end < len(cards) {
			body = append(body, mutedStyle.Render(fmt.Sprintf("↓ %d more", len(cards)-end)))
		}
	}

	// Clip and pad to the exact budget. Clipping is a last-resort guard for
	// oversized single cards; the fit above normally makes it a no-op.
	if len(body) > bodyHeight {
		body = body[:bodyHeight]
	}
	for len(body) < bodyHeight {
		body = append(body, "")
	}

	lines := make([]string, 0, bodyHeight+2)
	lines = append(lines, header, rule)
	lines = append(lines, body...)
	for i, line := range lines {
		lines[i] = padCell(line, colWidth)
	}
	return lines
}

// clippedMarkerLines is how many "N more" indicator lines a window needs.
func clippedMarkerLines(start, end, total int) int {
	n := 0
	if start > 0 {
		n++
	}
	if end < total {
		n++
	}
	return n
}

// renderKanban draws the whole board. It is pure over the model and safe at any
// terminal size — degenerate widths overflow rather than panic.
func (m Model) renderKanban() string {
	start, end := m.kanbanStageWindow()
	visible := end - start
	if visible < 1 {
		return ""
	}
	colWidth := m.kanbanColumnWidth(visible)
	bodyHeight := m.kanbanBodyHeight()

	blocks := make([]string, 0, visible*2-1)
	gutter := strings.Repeat(" ", kanbanGutter)
	for i := start; i < end; i++ {
		if i > start {
			blocks = append(blocks, strings.TrimRight(strings.Repeat(gutter+"\n", bodyHeight+2), "\n"))
		}
		blocks = append(blocks, strings.Join(m.renderKanbanColumn(i, colWidth, bodyHeight, i == m.kanbanCol), "\n"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}
