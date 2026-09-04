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
)

var (
	overlayPanelStyle = lipglossv2.NewStyle().
				Border(lipglossv2.RoundedBorder()).
				BorderForeground(lipglossv2.Color("99")).
				Background(lipglossv2.Color("0")).
				Padding(1, 2)
	overlayTitleStyle = lipglossv2.NewStyle().
				Foreground(lipglossv2.Color("99")).
				Bold(true)
	overlayLinkStyle = lipglossv2.NewStyle().
				Foreground(lipglossv2.Color("99")).
				Underline(true)
)

func isOverlayView(v View) bool {
	switch v {
	case ViewLeadSelector, ViewArtifactIndex, ViewArtifactContent, ViewReferences, ViewActiveMonitors:
		return true
	default:
		return false
	}
}

func (m Model) hasOverlay() bool {
	return len(m.overlayStack) > 0
}

func (m *Model) pushOverlay(v View) {
	if len(m.overlayStack) == 0 {
		m.overlayBase = m.currentView
	}
	m.overlayStack = append(m.overlayStack, v)
	m.currentView = v
}

func (m *Model) popOverlay() {
	if len(m.overlayStack) == 0 {
		switch m.currentView {
		case ViewLeadSelector:
			m.currentView = m.previousView
		case ViewArtifactIndex, ViewArtifactContent, ViewReferences, ViewActiveMonitors:
			m.currentView = ViewDetail
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

// renderLayeredView renders the ordinary surface first, then applies each
// overlay in order. Keeping the stack here means nested artifact content can
// return to the artifact index while retaining the dossier detail underneath.
func (m Model) renderLayeredView() string {
	base := m
	base.overlayStack = nil
	base.currentView = m.overlayBase
	rendered := clipScreenHeight(base.renderNormalView(), m.height)
	for _, overlay := range m.overlayStack {
		rendered = m.renderOverlay(rendered, overlay)
	}
	return rendered
}

func (m Model) renderOverlay(background string, v View) string {
	content := m.renderOverlayContent(v)
	context := m.recallResult.Frontmatter.Name
	if v == ViewLeadSelector {
		context = "Dashboard"
		if m.overlayBase == ViewKanban {
			context = "Board"
		}
	}
	if context == "" {
		context = "Dossier"
	}
	title := fmt.Sprintf("%s · %s", context, m.overlayLabel(v))

	panelWidth := m.width - 8
	if panelWidth > 96 {
		panelWidth = 96
	}
	if panelWidth < 32 {
		panelWidth = 32
	}
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

func (m Model) overlayLabel(v View) string {
	switch v {
	case ViewLeadSelector:
		return "Filters"
	case ViewReferences:
		return "References"
	case ViewActiveMonitors:
		return "Active Monitors"
	case ViewArtifactIndex:
		return "Artifacts"
	case ViewArtifactContent:
		return "Artifact Content"
	default:
		return "Details"
	}
}

func (m Model) renderOverlayContent(v View) string {
	switch v {
	case ViewLeadSelector:
		return m.renderFilterOverlay()
	case ViewReferences:
		return m.renderExternalLinks(m.recallResult.References, false)
	case ViewActiveMonitors:
		return m.renderExternalLinks(m.recallResult.ActiveMonitors, true)
	case ViewArtifactIndex:
		return m.renderArtifactIndexBody()
	case ViewArtifactContent:
		return m.artifactViewport.View()
	default:
		return ""
	}
}

func (m Model) renderFilterOverlay() string {
	var sb strings.Builder
	sb.WriteString("Lead\n")
	sb.WriteString(m.leadSearch.View())
	sb.WriteString("\n\n")

	if len(m.leadResults) == 0 {
		sb.WriteString(subtitleStyle.Render("No leads match your search."))
	} else {
		start, end := m.leadWindow()
		if start > 0 {
			sb.WriteString(subtitleStyle.Render(fmt.Sprintf("↑ %d more above", start)))
			sb.WriteString("\n")
		}
		for i := start; i < end; i++ {
			opt := m.leadResults[i]
			cursor := "  "
			if i == m.leadCursor {
				cursor = "> "
			}
			noun := "dossiers"
			if opt.count == 1 {
				noun = "dossier"
			}
			line := fmt.Sprintf("%-24s %d %s", opt.filter.label(), opt.count, noun)
			if i == m.leadCursor {
				sb.WriteString(focusedItemStyle.Render(cursor + line))
			} else {
				sb.WriteString(cursor + line)
			}
			sb.WriteString("\n")
		}
		if end < len(m.leadResults) {
			sb.WriteString(subtitleStyle.Render(fmt.Sprintf("↓ %d more below", len(m.leadResults)-end)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Interface: %s", m.interfaceFilter.label()))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("type to search leads · tab change interface · enter apply · esc cancel"))
	return sb.String()
}

func (m Model) renderExternalLinks(links []core.ExternalLink, monitors bool) string {
	if len(links) == 0 {
		if monitors {
			return subtitleStyle.Render("No active monitors recorded.")
		}
		return subtitleStyle.Render("No references recorded.")
	}

	start, end := centeredWindow(len(links), m.externalLinkCursor, m.overlayListVisibleRows())
	var sb strings.Builder
	if start > 0 {
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf("↑ %d more above", start)))
		sb.WriteString("\n")
	}
	for i := start; i < end; i++ {
		link := links[i]
		prefix := "  "
		if i == m.externalLinkCursor {
			prefix = "> "
		}
		label := fmt.Sprintf("[%s: %s]", link.Kind, link.Label)
		clickable := overlayLinkStyle.Hyperlink(link.URL).Render(label)
		line := prefix + clickable
		if link.Description != "" {
			line += " — " + link.Description
		}
		if monitors && link.LastPolled != "" {
			line += "  " + mutedStyle.Render("Last polled: "+link.LastPolled)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if end < len(links) {
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf("↓ %d more below", len(links)-end)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("enter open link · ↑/↓ move · esc close"))
	return strings.TrimRight(sb.String(), "\n")
}

func (m Model) externalLinkWindow(links []core.ExternalLink) (int, int) {
	return centeredWindow(len(links), m.externalLinkCursor, m.overlayListVisibleRows())
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
		return subtitleStyle.Render("No artifacts archived for this dossier.")
	}
	rows := m.artifactVisibleRows()
	if m.hasOverlay() {
		rows = m.overlayListVisibleRows()
	}
	start, end := centeredWindow(len(m.artifactIndex), m.artifactCursor, rows)
	var sb strings.Builder
	if start > 0 {
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf("↑ %d more above", start)))
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
		sb.WriteString(subtitleStyle.Render(fmt.Sprintf("↓ %d more below", len(m.artifactIndex)-end)))
	}
	if m.hasOverlay() {
		sb.WriteString("\n\n")
		sb.WriteString(subtitleStyle.Render("enter view artifact · ↑/↓ move · esc close"))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m Model) openSelectedExternalLink() tea.Cmd {
	var links []core.ExternalLink
	if m.currentView == ViewReferences {
		links = m.recallResult.References
	} else {
		links = m.recallResult.ActiveMonitors
	}
	if m.externalLinkCursor < 0 || m.externalLinkCursor >= len(links) {
		return nil
	}
	parsed, err := url.Parse(links[m.externalLinkCursor].URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return func() tea.Msg {
			return errMsg(fmt.Errorf("external link must be an absolute HTTP(S) URL: %q", links[m.externalLinkCursor].URL))
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
