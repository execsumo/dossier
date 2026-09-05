package tui

import (
	"context"
	"fmt"

	"dossier/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

type renameField int

const (
	renameSlugField renameField = iota
	renameNameField
)

func (m *Model) startSlugRename() {
	fm := m.recallResult.Frontmatter
	if fm.ID == "" {
		return
	}
	m.previousView = ViewDetail
	m.pushOverlay(ViewRenameSlug)
	m.targetID = fm.ID
	m.targetName = fm.Name
	m.targetBaseRevision = m.recallResult.Revision
	m.renameField = renameSlugField
	m.renameSlugInput.SetValue(fm.Slug)
	m.renameSlugInput.CursorEnd()
	m.renameNameInput.SetValue(fm.Name)
	m.renameNameInput.CursorEnd()
	m.renameNameInput.Blur()
	m.renameSlugInput.Focus()
	m.err = nil
}

func (m Model) renameSlugCmd() tea.Cmd {
	id := m.targetID
	base := m.targetBaseRevision
	field := m.renameField
	slug := m.renameSlugInput.Value()
	name := m.renameNameInput.Value()
	return func() tea.Msg {
		req := core.RenameReq{ID: id, BaseRevision: base}
		if field == renameNameField {
			req.NewName = name
		} else {
			req.NewSlug = slug
		}
		_, err := m.svc.Rename(context.Background(), req)
		return renameSlugResultMsg{err: err, targetID: id}
	}
}

func (m Model) updateSlugRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.renameSlugInput.Blur()
		m.renameNameInput.Blur()
		m.popOverlay()
		m.err = nil
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.renameSlugInput.Blur()
		m.renameNameInput.Blur()
		// There are two fields, so either tab direction toggles the selection.
		m.renameField = 1 - m.renameField
		if m.renameField == renameNameField {
			m.renameNameInput.Focus()
		} else {
			m.renameSlugInput.Focus()
		}
		return m, nil
	case "enter":
		m.loading = true
		m.err = nil
		return m, m.renameSlugCmd()
	case "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	if m.renameField == renameNameField {
		m.renameNameInput, cmd = m.renameNameInput.Update(msg)
	} else {
		m.renameSlugInput, cmd = m.renameSlugInput.Update(msg)
	}
	return m, cmd
}

func (m Model) renderSlugRename() string {
	slugLabel := "  Slug:  "
	nameLabel := "  Title: "
	if m.renameField == renameSlugField {
		slugLabel = "> Slug:  "
	} else {
		nameLabel = "> Title: "
	}
	body := fmt.Sprintf(
		"%s%s\n%s%s\n\n%s",
		slugLabel, m.renameSlugInput.View(),
		nameLabel, m.renameNameInput.View(),
		mutedStyle.Render("Tab switch · Enter save · Esc cancel · slug: lowercase, digits, hyphens."),
	)
	return body
}
