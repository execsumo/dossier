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
	editFieldDue editField = iota
	editFieldNextAction
	editFieldStage
	editFieldPriority
	editFieldLead
	editFieldInterfaces
	editFieldCount
)

// priorityOptions is the cycle order shared by the editor row and cyclePriority.
var priorityOptions = []core.Priority{core.PriorityLow, core.PriorityMedium, core.PriorityHigh, core.PriorityMax}

// startEdit opens the combined editor on t. Every field a dossier is triaged by
// lives here, so one press of e and one save covers what used to be four keys,
// four screens and four separate revisions.
func (m *Model) startEdit(t targetDossier) {
	m.previousView = m.currentView
	m.pushOverlay(ViewEdit)
	m.targetID = t.id
	m.targetName = t.name
	m.targetBaseRevision = t.baseRevision
	// Kept verbatim so save can send only the fields the user actually touched;
	// an untouched field must not appear in the audit log as a change.
	m.editOriginal = t

	m.editFocus = editFieldDue
	m.editStatus = core.NormalizeStatus(t.status)
	m.editPriority = t.priority
	if !m.editPriority.IsValid() {
		m.editPriority = core.PriorityHigh
	}

	m.dueDateInput = textinput.New()
	m.dueDateInput.Placeholder = "YYYY-MM-DD"
	m.dueDateInput.SetValue(t.dueDate)
	m.dueDateInput.Width = 40

	m.editLead = t.lead
	m.editInterfaces = append([]string{}, t.interfaces...)
	m.editInterfaceCursor = 0

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
	m.nextActionInput.Blur()
	switch m.editFocus {
	case editFieldDue:
		m.dueDateInput.Focus()
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

// moveEditVertical follows the modal's visual layout. The two text rows are
// stacked above the enum columns; once an enum column is focused, vertical keys
// move within that column's options.
func (m *Model) moveEditVertical(forward bool) {
	switch m.editFocus {
	case editFieldDue:
		if forward {
			m.editFocus = editFieldNextAction
		}
	case editFieldNextAction:
		if forward {
			m.editFocus = editFieldStage
		} else {
			m.editFocus = editFieldDue
		}
	case editFieldStage:
		statuses := core.CanonicalStatuses()
		if !forward && len(statuses) > 0 && m.editStatus == statuses[0] {
			// The first stage is the visual boundary between the enum columns
			// and the text fields above them. Let Up leave the stage column so
			// the form can be traversed back to Next Action and Due Date.
			m.editFocus = editFieldNextAction
		} else {
			m.editStatus = cycleStatus(m.editStatus, forward)
		}
	case editFieldPriority:
		m.editPriority = cyclePriority(m.editPriority, forward)
	case editFieldLead:
		m.cycleLead(forward)
	case editFieldInterfaces:
		m.cycleInterfaceCursor(forward)
	}
	m.syncEditFocus()
}

func (m *Model) moveEditHorizontal(forward bool) {
	order := []editField{editFieldStage, editFieldPriority, editFieldLead, editFieldInterfaces}
	for i, field := range order {
		if m.editFocus != field {
			continue
		}
		if forward {
			m.editFocus = order[(i+1)%len(order)]
		} else {
			m.editFocus = order[(i-1+len(order))%len(order)]
		}
		m.syncEditFocus()
		return
	}
}

func isEnumEditField(field editField) bool {
	return field == editFieldStage || field == editFieldPriority || field == editFieldLead || field == editFieldInterfaces
}

func (m *Model) cycleLead(forward bool) {
	opts := append([]string{""}, m.configuredLeads...)
	idx := 0
	for i, option := range opts {
		if option == m.editLead {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(opts)
	} else {
		idx = (idx - 1 + len(opts)) % len(opts)
	}
	m.editLead = opts[idx]
}

func (m *Model) cycleInterfaceCursor(forward bool) {
	if len(m.configuredInterfaces) == 0 {
		return
	}
	if forward {
		m.editInterfaceCursor = (m.editInterfaceCursor + 1) % len(m.configuredInterfaces)
	} else {
		m.editInterfaceCursor = (m.editInterfaceCursor - 1 + len(m.configuredInterfaces)) % len(m.configuredInterfaces)
	}
}

// cycleEditValue changes the focused row's value, for the rows that hold a fixed
// set rather than free text. The lead row is a fixed enum sourced from config.yaml.
func (m *Model) cycleEditValue(forward bool) bool {
	switch m.editFocus {
	case editFieldStage:
		m.editStatus = cycleStatus(m.editStatus, forward)
		return true
	case editFieldPriority:
		m.editPriority = cyclePriority(m.editPriority, forward)
		return true
	case editFieldLead:
		m.cycleLead(forward)
		return true
	case editFieldInterfaces:
		m.cycleInterfaceCursor(forward)
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
	if m.editLead != m.editOriginal.lead {
		updates["lead"] = m.editLead
	}
	if strings.Join(m.editInterfaces, "|||") != strings.Join(m.editOriginal.interfaces, "|||") {
		updates["interfaces"] = append([]string{}, m.editInterfaces...)
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
		m.popOverlay()
		return m, nil
	case "up", "shift+tab":
		if msg.String() == "shift+tab" {
			m.moveEditFocus(-1)
		} else {
			m.moveEditVertical(false)
		}
		return m, nil
	case "down":
		m.moveEditVertical(true)
		return m, nil
	case "tab":
		m.moveEditFocus(1)
		return m, nil
	case "left":
		if isEnumEditField(m.editFocus) {
			m.moveEditHorizontal(false)
			return m, nil
		}
	case "right":
		if isEnumEditField(m.editFocus) {
			m.moveEditHorizontal(true)
			return m, nil
		}
	case " ":
		if m.editFocus == editFieldInterfaces && len(m.configuredInterfaces) > 0 {
			name := m.configuredInterfaces[m.editInterfaceCursor]
			for i, selected := range m.editInterfaces {
				if selected == name {
					m.editInterfaces = append(m.editInterfaces[:i], m.editInterfaces[i+1:]...)
					return m, nil
				}
			}
			m.editInterfaces = append(m.editInterfaces, name)
			return m, nil
		}
	case "enter":
		updates := m.editUpdates()
		if len(updates) == 0 {
			// Nothing changed, so there is nothing to write. Saving anyway would
			// mint a revision that records no decision.
			m.popOverlay()
			return m, nil
		}
		m.loading = true
		m.err = nil
		return m, m.saveEditCmd(m.targetID, m.targetBaseRevision, updates)
	}

	switch m.editFocus {
	case editFieldDue:
		m.dueDateInput, cmd = m.dueDateInput.Update(msg)
	case editFieldNextAction:
		m.nextActionInput, cmd = m.nextActionInput.Update(msg)
	}
	return m, cmd
}

func renderEditColumn(title, body string, focused bool, width int) string {
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
		Render(metaLabelStyle.Render(title) + "\n\n" + body)
}

func renderEditTextRow(label, value string, focused bool) string {
	prefix := "   "
	if focused {
		prefix = " ▸ "
	}
	return prefix + metaLabelStyle.Render(fmt.Sprintf("%-13s", label+":")) + value
}

func renderEnumColumn[T ~string](title string, options []T, selected T, focused bool, width int) string {
	var sb strings.Builder
	for _, option := range options {
		label := string(option)
		if label == "" {
			label = "(unassigned)"
		}
		marker := "( )"
		if option == selected {
			marker = "(•)"
		}
		line := marker + " " + truncateCell(label, width-7)
		if option == selected && focused {
			sb.WriteString(focusedItemStyle.Copy().Background(modalBackground).Padding(0).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	return renderEditColumn(title, strings.TrimRight(sb.String(), "\n"), focused, width)
}

func renderInterfacesColumn(title string, options, selected []string, cursor int, focused bool, width int) string {
	if len(options) == 0 {
		options = []string{""}
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, value := range selected {
		selectedSet[value] = true
	}
	var sb strings.Builder
	for i, option := range options {
		label := option
		if label == "" {
			label = "(none)"
		}
		marker := "[ ]"
		if selectedSet[option] {
			marker = "[x]"
		}
		line := marker + " " + truncateCell(label, width-7)
		if i == cursor && focused {
			sb.WriteString(focusedItemStyle.Copy().Background(modalBackground).Padding(0).Render(line))
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	return renderEditColumn(title, strings.TrimRight(sb.String(), "\n"), focused, width)
}

// renderEditor draws the combined editor. Every field a dossier is triaged by is
// on screen at once, which is the whole reason the four single-field screens are
// gone: the decisions are made together, so they should be visible together.
func (m Model) renderEditor() string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	columnWidth := (width - 24) / 4
	if columnWidth < 14 {
		columnWidth = 14
	}
	if columnWidth > 22 {
		columnWidth = 22
	}

	leadOptions := append([]string{""}, m.configuredLeads...)
	due := m.editTextValue(editFieldDue, m.dueDateInput.View(), m.dueDateInput.Value())
	next := m.editTextValue(editFieldNextAction, m.nextActionInput.View(), m.nextActionInput.Value())
	columns := []string{
		renderEnumColumn("Stage", core.CanonicalStatuses(), m.editStatus, m.editFocus == editFieldStage, columnWidth),
		renderEnumColumn("Priority", priorityOptions, m.editPriority, m.editFocus == editFieldPriority, columnWidth),
		renderEnumColumn("Lead", leadOptions, m.editLead, m.editFocus == editFieldLead, columnWidth),
		renderInterfacesColumn("Interfaces", m.configuredInterfaces, m.editInterfaces, m.editInterfaceCursor, m.editFocus == editFieldInterfaces, columnWidth),
	}
	var sb strings.Builder
	sb.WriteString(renderEditTextRow("Due date", due, m.editFocus == editFieldDue))
	sb.WriteString("\n")
	sb.WriteString(renderEditTextRow("Next action", next, m.editFocus == editFieldNextAction))
	sb.WriteString("\n\n")
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, columns...))
	sb.WriteString("\n\n")
	sb.WriteString(mutedStyle.Render("↑/↓ field • ←/→ change • space toggle interface • enter save • esc cancel"))
	return strings.TrimRight(sb.String(), "\n")
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
