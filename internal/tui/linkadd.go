package tui

import (
	"context"
	"fmt"
	"strings"

	"dossier/internal/core"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Fields of the Add Reference form, in tab order.
const (
	linkAddURL = iota
	linkAddLabel
	linkAddKind
	linkAddDescription
	linkAddFieldCount
)

var linkAddLabels = [linkAddFieldCount]string{"URL", "Label", "Kind", "About"}

func (m *Model) startLinkAdd() {
	if m.recallResult.Frontmatter.ID == "" {
		return
	}
	placeholders := [linkAddFieldCount]string{"https://…", "defaults to the host", "link", "optional one-line description"}
	for i := range m.linkAddInputs {
		in := textinput.New()
		in.Placeholder = placeholders[i]
		in.Width = 60
		m.linkAddInputs[i] = in
	}
	m.linkAddField = linkAddURL
	m.linkAddInputs[linkAddURL].Focus()
	m.err = nil
	m.pushOverlay(ViewLinkAdd)
}

func (m Model) addReferenceCmd() tea.Cmd {
	id := m.recallResult.Frontmatter.ID
	req := core.AddReferenceReq{
		ID:          id,
		URL:         m.linkAddInputs[linkAddURL].Value(),
		Label:       m.linkAddInputs[linkAddLabel].Value(),
		Kind:        m.linkAddInputs[linkAddKind].Value(),
		Description: m.linkAddInputs[linkAddDescription].Value(),
	}
	requestID := m.nextRequestID()
	return func() tea.Msg {
		res, err := m.svc.AddReference(context.Background(), req)
		return linkAddResultMsg{requestID: requestID, err: err, warnings: res.Warnings, nextActions: res.NextActions, targetID: id}
	}
}

func (m Model) updateLinkAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.popOverlay()
		m.err = nil
		return m, nil
	case "enter":
		m.loading = true
		m.err = nil
		return m, m.addReferenceCmd()
	case "tab", "down", "shift+tab", "up":
		step := 1
		if msg.String() == "shift+tab" || msg.String() == "up" {
			step = linkAddFieldCount - 1
		}
		m.linkAddInputs[m.linkAddField].Blur()
		m.linkAddField = (m.linkAddField + step) % linkAddFieldCount
		m.linkAddInputs[m.linkAddField].Focus()
		return m, nil
	}
	var cmd tea.Cmd
	m.linkAddInputs[m.linkAddField], cmd = m.linkAddInputs[m.linkAddField].Update(msg)
	return m, cmd
}

func (m Model) renderLinkAdd() string {
	var sb strings.Builder
	for i, label := range linkAddLabels {
		prefix := "  "
		if i == m.linkAddField {
			prefix = "> "
		}
		sb.WriteString(fmt.Sprintf("%s%-6s %s\n", prefix, label+":", m.linkAddInputs[i].View()))
	}
	sb.WriteString("\n")
	sb.WriteString(renderModalTip("Saved under References in the Distilled State."))
	return sb.String()
}
