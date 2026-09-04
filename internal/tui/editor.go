package tui

import (
	"context"
	"fmt"
	"strings"

	"dossier/internal/core"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// editField enumerates the combined editor's rows in focus order. The order is
// the order a triage decision is actually made in: where the work is, how much
// it matters, when it is due, who has it, and what happens next.
type editField int

const (
	editFieldStage editField = iota
	editFieldPriority
	editFieldDue
	editFieldLead
	editFieldNextAction
	editFieldCount
)

// editLabelWidth is the gutter the field labels sit in.
const editLabelWidth = 13

// priorityOptions is the cycle order shared by the editor row and cyclePriority.
var priorityOptions = []core.Priority{core.PriorityLow, core.PriorityMedium, core.PriorityHigh, core.PriorityMax}

// startEdit opens the combined editor on t. Every field a dossier is triaged by
// lives here, so one press of e and one save covers what used to be four keys,
// four screens and four separate revisions.
func (m *Model) startEdit(t targetDossier) {
	m.previousView = m.currentView
	m.currentView = ViewEdit
	m.targetID = t.id
	m.targetName = t.name
	m.targetBaseRevision = t.baseRevision
	// Kept verbatim so save can send only the fields the user actually touched;
	// an untouched field must not appear in the audit log as a change.
	m.editOriginal = t

	m.editFocus = editFieldStage
	m.editStatus = core.NormalizeStatus(t.status)
	m.editPriority = t.priority
	if !m.editPriority.IsValid() {
		m.editPriority = core.PriorityHigh
	}

	m.dueDateInput = textinput.New()
	m.dueDateInput.Placeholder = "YYYY-MM-DD"
	m.dueDateInput.SetValue(t.dueDate)
	m.dueDateInput.Width = 40

	m.leadInput = textinput.New()
	m.leadInput.Placeholder = "e.g. Alice"
	m.leadInput.SetValue(t.lead)
	m.leadInput.Width = 40
	m.leadSuggestions = nil
	m.leadSuggestionText = ""

	m.nextActionInput = textinput.New()
	// Set the existing value before applying the limit so legacy overlong
	// actions are not silently truncated; the user can edit them down to size.
	m.nextActionInput.SetValue(t.nextAction)
	m.nextActionInput.CharLimit = core.MaxNextActionLength
	m.nextActionInput.Width = 60

	m.syncEditFocus()
}

// syncEditFocus gives the keyboard to the text input on the focused row, if the
// focused row has one. Exactly one input may be focused at a time or two cursors
// blink at once.
func (m *Model) syncEditFocus() {
	m.dueDateInput.Blur()
	m.leadInput.Blur()
	m.nextActionInput.Blur()
	switch m.editFocus {
	case editFieldDue:
		m.dueDateInput.Focus()
	case editFieldLead:
		m.leadInput.Focus()
	case editFieldNextAction:
		m.nextActionInput.Focus()
	}
}

// moveEditFocus steps between rows, wrapping. Wrapping is safe here in a way it
// is not on the board: a form is a ring of fields, not a space with edges.
func (m *Model) moveEditFocus(delta int) {
	m.editFocus = editField((int(m.editFocus) + delta + int(editFieldCount)) % int(editFieldCount))
	m.syncEditFocus()
}

// cycleEditValue changes the focused row's value, for the rows that hold a fixed
// set rather than free text. Text rows return false so the keypress falls
// through to the input, where left/right must still move the cursor.
func (m *Model) cycleEditValue(forward bool) bool {
	switch m.editFocus {
	case editFieldStage:
		m.editStatus = cycleStatus(m.editStatus, forward)
		return true
	case editFieldPriority:
		m.editPriority = cyclePriority(m.editPriority, forward)
		return true
	}
	return false
}

// cycleStatus steps through the canonical stages, wrapping. An unrecognised
// current stage lands on the first one rather than sticking.
func cycleStatus(curr core.Status, forward bool) core.Status {
	opts := core.CanonicalStatuses()
	idx := -1
	for i, o := range opts {
		if o == curr {
			idx = i
			break
		}
	}
	if idx < 0 {
		return opts[0]
	}
	if forward {
		return opts[(idx+1)%len(opts)]
	}
	return opts[(idx-1+len(opts))%len(opts)]
}

// editUpdates is the frontmatter patch for the fields the user actually changed.
// An unchanged field is absent rather than rewritten, so the audit log records
// the decision that was made instead of every field the form happened to show.
func (m Model) editUpdates() map[string]any {
	updates := map[string]any{}
	if m.editStatus != core.NormalizeStatus(m.editOriginal.status) {
		updates["status"] = string(m.editStatus)
	}
	if m.editPriority != m.editOriginal.priority {
		updates["priority"] = string(m.editPriority)
	}
	if v := m.dueDateInput.Value(); v != m.editOriginal.dueDate {
		updates["due_date"] = v
	}
	if v := m.leadInput.Value(); v != m.editOriginal.lead {
		updates["lead"] = v
	}
	if v := m.nextActionInput.Value(); v != m.editOriginal.nextAction {
		updates["next_action"] = v
	}
	return updates
}

// saveEditCmd writes every changed field in one Save. One save is the point of
// the combined form: a triage pass that used to cost three revisions and three
// audit entries is now one of each.
func (m Model) saveEditCmd(id string, baseRev core.Revision, updates map[string]any) tea.Cmd {
	return func() tea.Msg {
		_, err := m.svc.Save(context.Background(), core.SaveReq{
			ID:                 id,
			BaseRevision:       baseRev,
			FrontmatterUpdates: updates,
		})
		return mutationResultMsg{err: err, prevView: m.previousView, targetID: id}
	}
}

// updateEditor handles a keypress inside the combined editor.
func (m Model) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "esc":
		m.currentView = m.previousView
		return m, nil
	case "up", "shift+tab":
		m.moveEditFocus(-1)
		return m, nil
	case "down":
		m.moveEditFocus(1)
		return m, nil
	case "tab":
		// On the lead row tab has a better job than moving on: taking the
		// completion. It only advances once there is nothing left to accept.
		if m.editFocus == editFieldLead && m.acceptLeadSuggestion() {
			return m, nil
		}
		m.moveEditFocus(1)
		return m, nil
	case "left":
		if m.cycleEditValue(false) {
			return m, nil
		}
	case "right":
		if m.cycleEditValue(true) {
			return m, nil
		}
	case "enter":
		if m.editFocus == editFieldLead && m.acceptLeadSuggestion() {
			return m, nil
		}
		updates := m.editUpdates()
		if len(updates) == 0 {
			// Nothing changed, so there is nothing to write. Saving anyway would
			// mint a revision that records no decision.
			m.currentView = m.previousView
			return m, nil
		}
		m.loading = true
		m.err = nil
		return m, m.saveEditCmd(m.targetID, m.targetBaseRevision, updates)
	}

	switch m.editFocus {
	case editFieldDue:
		m.dueDateInput, cmd = m.dueDateInput.Update(msg)
	case editFieldLead:
		m.leadInput, cmd = m.leadInput.Update(msg)
		m.updateLeadSuggestions()
	case editFieldNextAction:
		m.nextActionInput, cmd = m.nextActionInput.Update(msg)
	}
	return m, cmd
}

// renderEditRow lays out one label + value line. The focused row is marked in
// the gutter and its label highlighted, so which field the arrows will act on is
// readable without a cursor to look for.
func renderEditRow(label, value string, focused bool) string {
	// focusedItemStyle carries its own horizontal padding, so the label is styled
	// alone and the gutter stays plain — otherwise the focused row's value column
	// slides one cell right of every other row's.
	if focused {
		return " ▸" + focusedItemStyle.Render(padCell(label, editLabelWidth-2)) + value
	}
	return "   " + padCell(label, editLabelWidth) + value
}

// renderChoiceRow renders a fixed set of options with the selected one bracketed.
func renderChoiceRow[T ~string](options []T, selected T) string {
	parts := make([]string, 0, len(options))
	for _, o := range options {
		if o == selected {
			parts = append(parts, activeOptionStyle.Render("["+string(o)+"]"))
			continue
		}
		parts = append(parts, " "+mutedStyle.Render(string(o))+" ")
	}
	return strings.Join(parts, " ")
}

// renderEditor draws the combined editor. Every field a dossier is triaged by is
// on screen at once, which is the whole reason the four single-field screens are
// gone: the decisions are made together, so they should be visible together.
func (m Model) renderEditor() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Edit %s\n\n", lipgloss.NewStyle().Foreground(vibrantGreen).Bold(true).Render(m.targetName)))

	sb.WriteString(renderEditRow("Stage", renderChoiceRow(core.CanonicalStatuses(), m.editStatus), m.editFocus == editFieldStage))
	sb.WriteString("\n")
	sb.WriteString(renderEditRow("Priority", renderChoiceRow(priorityOptions, m.editPriority), m.editFocus == editFieldPriority))
	sb.WriteString("\n")
	sb.WriteString(renderEditRow("Due date", m.editTextValue(editFieldDue, m.dueDateInput.View(), m.dueDateInput.Value()), m.editFocus == editFieldDue))
	sb.WriteString("\n")

	lead := m.editTextValue(editFieldLead, m.leadInput.View(), m.leadInput.Value())
	if m.editFocus == editFieldLead && m.leadSuggestionText != "" {
		lead += mutedStyle.Render(fmt.Sprintf("  suggestion: %s (tab)", m.leadSuggestionText))
	}
	sb.WriteString(renderEditRow("Lead", lead, m.editFocus == editFieldLead))
	sb.WriteString("\n")
	sb.WriteString(renderEditRow("Next action", m.editTextValue(editFieldNextAction, m.nextActionInput.View(), m.nextActionInput.Value()), m.editFocus == editFieldNextAction))
	sb.WriteString("\n\n")

	sb.WriteString(mutedStyle.Render("↑/↓ field • ←/→ change • enter save • esc cancel"))
	return editorBoxStyle.Render(sb.String())
}

// editTextValue shows a text row's live input when it holds the keyboard and its
// plain value otherwise, so only the focused row blinks a cursor.
func (m Model) editTextValue(field editField, view, value string) string {
	if m.editFocus == field {
		return view
	}
	if value == "" {
		return mutedStyle.Render("(empty)")
	}
	return metaValueStyle.Render(value)
}
