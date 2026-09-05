package tui

import (
	"github.com/charmbracelet/bubbles/help"
	bubbleskey "github.com/charmbracelet/bubbles/key"
)

// tuiKeyMap adapts the TUI's contextual bindings to Bubbles' help.Model. The
// bindings are presentation-only; Update retains the existing dispatch logic.
type tuiKeyMap struct {
	short []bubbleskey.Binding
	full  [][]bubbleskey.Binding
}

func (m tuiKeyMap) ShortHelp() []bubbleskey.Binding  { return m.short }
func (m tuiKeyMap) FullHelp() [][]bubbleskey.Binding { return m.full }

func tuiHelpKey(keys, description string) bubbleskey.Binding {
	return bubbleskey.NewBinding(bubbleskey.WithKeys(keys), bubbleskey.WithHelp(keys, description))
}

func (m Model) searchHelpKeyMap() help.KeyMap {
	bindings := []bubbleskey.Binding{
		tuiHelpKey("enter", "keep filter"),
		tuiHelpKey("esc", "clear"),
	}
	return tuiKeyMap{short: bindings, full: [][]bubbleskey.Binding{bindings}}
}

// modalHelpBindings contains only controls that add context beyond the modal's
// obvious interaction. Escape/back navigation and routine cursor movement are
// omitted; they are consistent across the TUI and do not need repeating in every modal.
func modalHelpBindings(v View) []bubbleskey.Binding {
	switch v {
	case ViewLeadSelector:
		return []bubbleskey.Binding{tuiHelpKey("←/→", "column"), tuiHelpKey("enter", "apply")}
	case ViewLinkInput:
		return []bubbleskey.Binding{tuiHelpKey("enter", "find target")}
	case ViewLinkSelector:
		return []bubbleskey.Binding{tuiHelpKey("enter", "choose target")}
	case ViewMergeSelector:
		return []bubbleskey.Binding{tuiHelpKey("enter", "merge")}
	case ViewMergeConflictResolver:
		return []bubbleskey.Binding{tuiHelpKey("tab", "choose action"), tuiHelpKey("enter", "apply")}
	case ViewRenameSlug:
		return []bubbleskey.Binding{tuiHelpKey("tab", "next field"), tuiHelpKey("enter", "save")}
	case ViewEdit:
		return []bubbleskey.Binding{tuiHelpKey("tab", "next field"), tuiHelpKey("↑/↓", "change option"), tuiHelpKey("space", "toggle interface")}
	case ViewArtifactIndex:
		return []bubbleskey.Binding{tuiHelpKey("enter", "view artifact")}
	case ViewLinks:
		return []bubbleskey.Binding{tuiHelpKey("enter", "open link")}
	}
	return nil
}

func (m Model) helpKeyMap(v View) help.KeyMap {
	// Modals use a deliberately minimal footer. The parent surface's commands
	// are not actionable while an overlay is open, so do not show them through
	// the modal's own help map.
	if isOverlayView(v) {
		contextual := modalHelpBindings(v)
		return tuiKeyMap{short: contextual, full: [][]bubbleskey.Binding{contextual}}
	}

	common := []bubbleskey.Binding{
		tuiHelpKey("q", "quit"),
		tuiHelpKey("?", "more help"),
	}

	var contextual []bubbleskey.Binding
	var shortContextual []bubbleskey.Binding
	switch v {
	case ViewDashboard:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "filters"),
			tuiHelpKey("v", "view"),
			tuiHelpKey("e", "edit"), tuiHelpKey("k", "add link"),
			tuiHelpKey("l", "links"), tuiHelpKey("m", "merge"), tuiHelpKey("c", "open agent"),
		}
		shortContextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "filters"),
			tuiHelpKey("v", "view"),
		}
	case ViewKanban:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "filters"),
			tuiHelpKey("v", "view"),
			tuiHelpKey("e", "edit"), tuiHelpKey("l", "links"), tuiHelpKey("c", "open agent"),
		}
		shortContextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "filters"),
			tuiHelpKey("v", "view"),
		}
	case ViewDetail:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("e", "edit"), tuiHelpKey("r", "rename"),
			tuiHelpKey("a", "artifacts"), tuiHelpKey("l", "links"), tuiHelpKey("o", "open in editor"),
			tuiHelpKey("k", "add link"), tuiHelpKey("c", "open agent"), tuiHelpKey("v", "view"),
		}
		shortContextual = []bubbleskey.Binding{
			tuiHelpKey("l", "links"), tuiHelpKey("v", "view"),
		}
	default:
		contextual = []bubbleskey.Binding{tuiHelpKey("esc", "back")}
		shortContextual = contextual
	}

	short := append(append([]bubbleskey.Binding{}, shortContextual...), common...)
	return tuiKeyMap{short: short, full: [][]bubbleskey.Binding{contextual, common}}
}
