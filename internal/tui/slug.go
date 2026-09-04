package tui

import (
	"context"
	"fmt"

	"dossier/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) startSlugRename() {
	fm := m.recallResult.Frontmatter
	if fm.ID == "" {
		return
	}
	m.previousView = ViewDetail
	m.currentView = ViewRenameSlug
	m.targetID = fm.ID
	m.targetName = fm.Name
	m.targetBaseRevision = m.recallResult.Revision
	m.renameSlugInput.SetValue(fm.Slug)
	m.renameSlugInput.CursorEnd()
	m.renameSlugInput.Focus()
	m.err = nil
}

func (m Model) renameSlugCmd() tea.Cmd {
	id := m.targetID
	newSlug := m.renameSlugInput.Value()
	base := m.targetBaseRevision
	return func() tea.Msg {
		_, err := m.svc.RenameSlug(context.Background(), core.RenameSlugReq{
			ID: id, NewSlug: newSlug, BaseRevision: base,
		})
		return renameSlugResultMsg{err: err, targetID: id}
	}
}

func (m Model) updateSlugRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.renameSlugInput.Blur()
		m.currentView = ViewDetail
		m.err = nil
		return m, nil
	case "enter":
		m.loading = true
		m.err = nil
		return m, m.renameSlugCmd()
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.renameSlugInput, cmd = m.renameSlugInput.Update(msg)
	return m, cmd
}

func (m Model) renderSlugRename() string {
	body := fmt.Sprintf(
		"Rename slug for %s\n\n%s\n\nThe dossier ID stays fixed and the old slug remains a working alias.\nThe complete dossier directory moves to the new path.\n\n%s",
		m.targetName,
		m.renameSlugInput.View(),
		mutedStyle.Render("Use lowercase letters, digits, and single hyphens."),
	)
	return editorBoxStyle.Render(body)
}
