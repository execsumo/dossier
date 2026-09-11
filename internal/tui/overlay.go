package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"dossier/internal/core"

	lipglossv2 "charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"
	lipgloss "github.com/charmbracelet/lipgloss"
)

var (
	overlayPanelStyle = lipglossv2.NewStyle().
				Border(lipglossv2.RoundedBorder()).
				BorderForeground(lipglossv2.Color(modalAccentHex)).
				Background(lipglossv2.Color(modalBackgroundColor)).
				Padding(1, 2)
	overlayTitleStyle = lipglossv2.NewStyle().
				Foreground(lipglossv2.Color("#B18CFF")).
				Bold(true)
	overlayLinkStyle = lipglossv2.NewStyle().
				Foreground(lipglossv2.Color("#B8A1FF")).
				Underline(true)
	overlayEmptyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D6D3F0")).
				Italic(true)
	overlayHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B9B6D6"))
	overlayMutedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B9B6D6"))
	overlaySectionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B18CFF")).
				Bold(true)
)

type externalLinkEntry struct {
	link     core.ExternalLink
	monitors bool
}

type externalLinkRow struct {
	section    string
	entry      externalLinkEntry
	entryIndex int
	empty      bool
}

func isOverlayView(v View) bool {
	switch v {
	case ViewLeadSelector, ViewEdit, ViewLinkInput, ViewLinkSelector, ViewMergeSelector, ViewMergeConflictResolver, ViewRenameSlug, ViewArtifactIndex, ViewArtifactContent, ViewLinks, ViewContracts:
		return true
	default:
		return false
	}
}

func (m Model) hasOverlay() bool {
	return len(m.overlayStack) > 0
}

func (m *Model) pushOverlay(v View) {
	if !isOverlayView(v) {
		m.currentView = v
		return
	}
	if len(m.overlayStack) == 0 {
		m.overlayBase = m.currentView
	}
	m.overlayStack = append(m.overlayStack, v)
	m.currentView = v
}

func (m *Model) popOverlay() {
	if len(m.overlayStack) == 0 {
		// A view is never allowed to masquerade as an overlay without a stack
		// entry. This fallback keeps Escape useful for models assembled directly
		// by callers and prevents a stale modal from trapping navigation.
		if isOverlayView(m.currentView) {
			if isOverlayView(m.previousView) {
				m.currentView = m.overlayBase
			} else {
				m.currentView = m.previousView
			}
		}
		return
	}
	m.overlayStack = m.overlayStack[:len(m.overlayStack)-1]
	if len(m.overlayStack) == 0 {
		m.currentView = m.overlayBase
		return
	}
	m.currentView = m.overlayStack[len(m.overlayStack)-1]
}

func (m *Model) dismissOverlays() {
	m.overlayStack = nil
	m.currentView = m.overlayBase
}

// replaceTopOverlay changes only the active overlay. It is used when an
// asynchronous service result changes a selector into a conflict/details
// surface, preserving the parent and the rest of the stack.
func (m *Model) replaceTopOverlay(v View) {
	if !isOverlayView(v) {
		return
	}
	if len(m.overlayStack) == 0 {
		m.pushOverlay(v)
		return
	}
	m.overlayStack[len(m.overlayStack)-1] = v
	m.currentView = v
}

// renderLayeredView renders the ordinary surface first, then applies each
// overlay in order. Keeping the stack here means nested artifact content can
// return to the artifact index while retaining the dossier detail underneath.
func (m Model) renderLayeredView() string {
	base := m
	base.overlayStack = nil
	base.currentView = m.overlayBase
	base.suppressFooter = true
	rendered := clipScreenHeight(base.renderNormalView(), m.height)
	for _, overlay := range m.overlayStack {
		rendered = m.renderOverlay(rendered, overlay)
	}
	return rendered
}

func (m Model) renderOverlay(background string, v View) string {
	content := m.renderOverlayContent(v)
	context := m.recallResult.Frontmatter.Name
	if v == ViewEdit && m.targetName != "" {
		context = m.targetName
	}
	if (v == ViewMergeSelector || v == ViewMergeConflictResolver) && m.mergeSourceName != "" {
		context = m.mergeSourceName
	}
	if v == ViewLeadSelector {
		context = "Dashboard"
		if m.overlayBase == ViewKanban {
			context = "Board"
		}
	}
	if context == "" {
		context = "Dossier"
	}
	title := fmt.Sprintf("%s · %s · Esc back", context, m.overlayLabel(v))

	panelWidth := m.width - 8
	if v == ViewEdit {
		panelWidth = m.width - 2
	}
	maxPanelWidth := 96
	if v == ViewEdit {
		maxPanelWidth = 120
	}
	if panelWidth > maxPanelWidth {
		panelWidth = maxPanelWidth
	}
	if panelWidth < 32 {
		panelWidth = 32
	}
	footer := renderModalFooter(v)
	if lipgloss.Width(footer) > panelWidth {
		footer = compactModalFooter(v)
	}
	if footer != "" {
		content += "\n\n" + footer
	}
	footerHeight := 0
	if footer != "" {
		footerHeight = lipgloss.Height(footer)
	}
	// A panel has a title, spacing, border, and padding outside its content.
	// Budget the content before rendering rather than clipping the finished
	// compositor, which used to remove the footer and trap the user.
	content = fitScreen(content, panelWidth, m.height-6, footerHeight)
	panel := overlayPanelStyle.Width(panelWidth).Render(
		overlayTitleStyle.Render(title) + "\n\n" + content,
	)

	x := (m.width - lipglossv2.Width(panel)) / 2
	if x < 0 {
		x = 0
	}
	y := (m.height - lipglossv2.Height(panel)) / 2
	if y < 0 {
		y = 0
	}

	backgroundLayer := lipglossv2.NewLayer(background).X(0).Y(0).Z(0)
	overlayLayer := lipglossv2.NewLayer(panel).X(x).Y(y).Z(1)
	return clipScreenHeight(lipglossv2.NewCompositor(backgroundLayer, overlayLayer).Render(), m.height)
}

func compactModalFooter(v View) string {
	var text string
	switch v {
	case ViewLeadSelector:
		text = "enter apply · ←/→ column"
	case ViewLinkInput:
		text = "enter find target"
	case ViewLinkSelector:
		text = "enter choose target"
	case ViewMergeSelector:
		text = "enter merge"
	case ViewMergeConflictResolver:
		text = "enter apply · tab action"
	case ViewRenameSlug:
		text = "enter save · tab next"
	case ViewEdit:
		text = "enter save · tab next"
	case ViewArtifactIndex:
		text = "enter view artifact"
	case ViewLinks:
		text = "enter open link"
	case ViewContracts:
		text = "↑/↓ scroll"
	default:
		return ""
	}
	return overlayHintStyle.Render(text)
}

func clipScreenHeight(content string, height int) string {
	if height <= 0 {
		return content
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// modalTitle is the canonical title for every modal. Titles use the same
// action-oriented, title-case vocabulary whether the view is rendered as an
// overlay or directly (for example, in a narrow-terminal fallback).
func modalTitle(v View) string {
	switch v {
	case ViewLeadSelector:
		return "Filter Dossiers"
	case ViewLinkInput:
		return "Add Link"
	case ViewLinkSelector:
		return "Choose Link Target"
	case ViewMergeSelector:
		return "Merge Dossiers"
	case ViewMergeConflictResolver:
		return "Resolve Merge Conflict"
	case ViewRenameSlug:
		return "Rename Dossier"
	case ViewEdit:
		return "Edit Dossier"
	case ViewLinks:
		return "View Links"
	case ViewArtifactIndex:
		return "Browse Artifacts"
	case ViewArtifactContent:
		return "View Artifact"
	case ViewContracts:
		return "Delegation Contracts"
	default:
		return "Details"
	}
}

func (m Model) overlayLabel(v View) string {
	return modalTitle(v)
}

// renderModalFooter is the one footer format used inside modal panels. It is
// sourced from the same bindings as the direct-view footer, keeping both render
// paths consistent while omitting obvious navigation commands.
func renderModalFooter(v View) string {
	bindings := modalHelpBindings(v)
	if len(bindings) == 0 {
		return ""
	}
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		help := binding.Help()
		parts = append(parts, help.Key+" "+help.Desc)
	}
	return overlayHintStyle.Render(strings.Join(parts, " · "))
}

func renderModalTip(text string) string {
	return overlayHintStyle.Render("Tip: " + text)
}

func (m Model) renderOverlayContent(v View) string {
	switch v {
	case ViewLeadSelector:
		return m.renderFilterOverlay()
	case ViewLinkInput:
		return m.renderLinkInput()
	case ViewLinkSelector:
		return m.renderLinkSelector()
	case ViewMergeSelector:
		return m.renderMergeSelector()
	case ViewMergeConflictResolver:
		return m.renderMergeConflictResolver()
	case ViewRenameSlug:
		return m.renderSlugRename()
	case ViewEdit:
		return m.renderEditor()
	case ViewLinks:
		return m.renderExternalLinks()
	case ViewContracts:
		return m.contractsViewport.View()
	case ViewArtifactIndex:
		return m.renderArtifactIndexBody()
	case ViewArtifactContent:
		return m.artifactViewport.View()
	default:
		return ""
	}
}

func (m Model) renderFilterOverlay() string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	columnWidth := (width - 12) / 2
	if columnWidth < 18 {
		columnWidth = 18
	}
	if columnWidth > 34 {
		columnWidth = 34
	}

	leadLabels := make([]string, len(m.leadResults))
	for i, option := range m.leadResults {
		leadLabels[i] = option.filter.label()
	}
	interfaceLabels := make([]string, len(m.interfaceOptions))
	for i, option := range m.interfaceOptions {
		interfaceLabels[i] = option.label
	}
	columns := []string{
		renderFilterColumn("Lead", leadLabels, m.leadCursor, m.filterColumn == 0, columnWidth, m.filterOptionRows()),
		renderFilterColumn("Interface", interfaceLabels, m.interfaceCursor, m.filterColumn == 1, columnWidth, m.filterOptionRows()),
	}
	return joinModalColumns(columns...)
}

func (m Model) filterOptionRows() int {
	rows := m.height - 18
	if rows < 2 {
		rows = 2
	}
	return rows
}

func renderFilterColumn(title string, options []string, cursor int, focused bool, width, visibleRows int) string {
	var sb strings.Builder
	start, end := centeredWindow(len(options), cursor, visibleRows)
	if start > 0 {
		sb.WriteString(overlayHintStyle.Render(fmt.Sprintf("↑ %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		option := options[i]
		marker := "( )"
		if i == cursor {
			marker = "(•)"
		}
		line := marker + " " + truncateCell(option, width-7)
		if i == cursor {
			sb.WriteString(focusedItemStyle.Copy().Background(modalBackground).Padding(0).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	if end < len(options) {
		sb.WriteString(overlayHintStyle.Render(fmt.Sprintf("↓ %d more below", len(options)-end)))
		sb.WriteString("\n")
	}
	if len(options) == 0 {
		sb.WriteString(overlayEmptyStyle.Render("No options"))
	}
	border := darkGray
	if focused {
		border = purple
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(modalBackground).
		Padding(0, 1).
		Width(width).
		Render(metaLabelStyle.Render(title) + "\n\n" + strings.TrimRight(sb.String(), "\n"))
}

func (m Model) externalLinkEntries() []externalLinkEntry {
	entries := make([]externalLinkEntry, 0, len(m.recallResult.ActiveMonitors)+len(m.recallResult.References))
	for _, link := range m.recallResult.ActiveMonitors {
		entries = append(entries, externalLinkEntry{link: link, monitors: true})
	}
	for _, link := range m.recallResult.References {
		entries = append(entries, externalLinkEntry{link: link})
	}
	return entries
}

func externalLinkRows(entries []externalLinkEntry) []externalLinkRow {
	rows := make([]externalLinkRow, 0, len(entries)+4)
	rows = append(rows, externalLinkRow{section: "Active Monitors"})
	monitorCount := 0
	for _, entry := range entries {
		if entry.monitors {
			rows = append(rows, externalLinkRow{entry: entry, entryIndex: monitorCount})
			monitorCount++
		}
	}
	if monitorCount == 0 {
		rows = append(rows, externalLinkRow{empty: true, entry: externalLinkEntry{monitors: true}})
	}

	rows = append(rows, externalLinkRow{section: "References"})
	referenceCount := 0
	for _, entry := range entries {
		if !entry.monitors {
			rows = append(rows, externalLinkRow{entry: entry, entryIndex: monitorCount + referenceCount})
			referenceCount++
		}
	}
	if referenceCount == 0 {
		rows = append(rows, externalLinkRow{empty: true})
	}
	return rows
}

func (m Model) renderExternalLinks() string {
	entries := m.externalLinkEntries()
	rows := externalLinkRows(entries)
	selectedRow := 0
	if len(entries) > 0 {
		for i, row := range rows {
			if !row.empty && row.section == "" && row.entryIndex == m.externalLinkCursor {
				selectedRow = i
				break
			}
		}
	}

	start, end := centeredWindow(len(rows), selectedRow, m.overlayListVisibleRows())
	var sb strings.Builder
	if start > 0 {
		sb.WriteString(overlayHintStyle.Render(fmt.Sprintf("↑ %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		row := rows[i]
		if row.section != "" {
			sb.WriteString(overlaySectionStyle.Render(row.section))
			sb.WriteString("\n")
			continue
		}
		if row.empty {
			message := "No references recorded."
			if row.entry.monitors {
				message = "No active monitors recorded."
			}
			sb.WriteString("  " + overlayEmptyStyle.Render(message))
			sb.WriteString("\n")
			continue
		}
		link := row.entry.link
		prefix := "  "
		if row.entryIndex == m.externalLinkCursor {
			prefix = "> "
		}
		label := fmt.Sprintf("[%s: %s]", link.Kind, link.Label)
		clickable := overlayLinkStyle.Hyperlink(link.URL).Render(label)
		line := prefix + clickable
		if link.Description != "" {
			line += " — " + link.Description
		}
		if row.entry.monitors && link.LastPolled != "" {
			line += "  " + overlayMutedStyle.Render("Last polled: "+link.LastPolled)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if end < len(rows) {
		sb.WriteString(overlayHintStyle.Render(fmt.Sprintf("↓ %d more below", len(rows)-end)))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m Model) overlayListVisibleRows() int {
	// Reserve the modal frame, title, spacing, and the contextual footer. The
	// compositor keeps the base surface full-screen, so an overlay must fit
	// inside the same terminal bounds rather than relying on clipping afterward.
	rows := m.height - 18
	if rows < 3 {
		rows = 3
	}
	return rows
}

func (m Model) renderArtifactIndexBody() string {
	if len(m.artifactIndex) == 0 {
		return overlayEmptyStyle.Render("No artifacts archived for this dossier.")
	}
	rows := m.artifactVisibleRows()
	if m.hasOverlay() {
		rows = m.overlayListVisibleRows()
	}
	start, end := centeredWindow(len(m.artifactIndex), m.artifactCursor, rows)
	var sb strings.Builder
	if start > 0 {
		sb.WriteString(overlayHintStyle.Render(fmt.Sprintf("↑ %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		a := m.artifactIndex[i]
		cited := "uncited"
		if a.Cited {
			cited = "cited"
		}
		row := fmt.Sprintf("%-28s %-18s %6d lines  %-8s %s", a.ID, a.Type, a.Lines, cited, a.Title)
		if i == m.artifactCursor {
			sb.WriteString(focusedItemStyle.Render("> " + row))
		} else {
			sb.WriteString("  " + row)
		}
		sb.WriteString("\n")
	}
	if end < len(m.artifactIndex) {
		sb.WriteString(overlayHintStyle.Render(fmt.Sprintf("↓ %d more below", len(m.artifactIndex)-end)))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// renderContractsChecklist renders the person-specific terms of each
// Delegation Contract. It deliberately names open fields rather than producing
// a completeness score: readiness is a judgment, not a form-filling metric.
func renderContractsChecklist(contracts []core.DelegationContract) string {
	if len(contracts) == 0 {
		return overlayEmptyStyle.Render("No delegation contracts recorded for this dossier.")
	}
	var sb strings.Builder
	for i, c := range contracts {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		header := c.Label
		if c.Owner != "" {
			header += " — owner: " + c.Owner
		}
		if c.AcceptedDate != "" {
			header += ", accepted " + c.AcceptedDate
		}
		sb.WriteString(overlaySectionStyle.Render(header))
		if c.Complete() {
			sb.WriteString("  ")
			sb.WriteString(lipgloss.NewStyle().Foreground(vibrantGreen).Render("ready"))
		} else {
			sb.WriteString("  ")
			sb.WriteString(overlayMutedStyle.Render("open: " + strings.Join(c.OpenFields(), ", ")))
		}
		sb.WriteString("\n")
		if c.Legacy {
			sb.WriteString(overlayMutedStyle.Render("Legacy contract: move Objective, Context, Success Criteria, Validation, and Constraints into the canonical Dossier; confirm Return Expectations."))
			sb.WriteString("\n")
		}
		for _, f := range c.Fields {
			sb.WriteString(renderContractFieldRow(f))
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// renderContractFieldRow renders one checklist line. Status, not text length,
// decides the box: [decided] checks it, [proposed] is an expected open item,
// and missing/untagged are schema violations flagged distinctly from a
// deliberate "not yet agreed" so an author's oversight doesn't read as if the
// field were knowingly left open.
func renderContractFieldRow(f core.ContractField) string {
	label := lipgloss.NewStyle().Bold(true).Width(18).Render(f.Label + ":")

	var box, text string
	switch f.Status {
	case core.ContractFieldDecided:
		box = lipgloss.NewStyle().Foreground(vibrantGreen).Render("[x]")
		text = f.Text
	case core.ContractFieldProposed:
		box = lipgloss.NewStyle().Foreground(warningGold).Render("[ ]")
		text = f.Text
		if text == "" {
			text = overlayMutedStyle.Render("(proposed — not yet agreed)")
		}
	case core.ContractFieldUntagged:
		box = lipgloss.NewStyle().Foreground(vibrantRed).Render("[ ]")
		text = f.Text + "  " + overlayMutedStyle.Render("(untagged — fix the [decided]/[proposed] tag)")
	default: // core.ContractFieldMissing
		box = lipgloss.NewStyle().Foreground(vibrantRed).Render("[ ]")
		text = overlayMutedStyle.Render("(not written)")
	}
	return "  " + box + " " + label + " " + text
}

func (m Model) openSelectedExternalLink() tea.Cmd {
	links := m.externalLinkEntries()
	if m.externalLinkCursor < 0 || m.externalLinkCursor >= len(links) {
		return nil
	}
	parsed, err := url.Parse(links[m.externalLinkCursor].link.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return func() tea.Msg {
			return errMsg(fmt.Errorf("external link must be an absolute HTTP(S) URL: %q", links[m.externalLinkCursor].link.URL))
		}
	}
	if m.openURL != nil {
		return m.openURL(parsed.String())
	}
	return nil
}

func launchExternalURL(rawURL string) tea.Cmd {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{rawURL}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		command = "xdg-open"
		args = []string{rawURL}
	}
	return tea.ExecProcess(exec.Command(command, args...), func(err error) tea.Msg {
		if err != nil {
			return errMsg(fmt.Errorf("open external link: %w", err))
		}
		return nil
	})
}
