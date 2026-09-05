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

func (m Model) helpKeyMap(v View) help.KeyMap {
	common := []bubbleskey.Binding{
		tuiHelpKey("q", "quit"),
		tuiHelpKey("?", "more help"),
	}

	var contextual []bubbleskey.Binding
	var shortContextual []bubbleskey.Binding
	switch v {
	case ViewDashboard:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "lead filter"),
			tuiHelpKey("i", "interface filter"), tuiHelpKey("v", "view"),
			tuiHelpKey("e", "edit"), tuiHelpKey("k", "link"),
			tuiHelpKey("m", "merge"), tuiHelpKey("c", "open agent"),
		}
		shortContextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "filters"),
			tuiHelpKey("v", "view"),
		}
	case ViewKanban:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "lead filter"),
			tuiHelpKey("i", "interface filter"), tuiHelpKey("v", "dashboard"),
			tuiHelpKey("e", "edit"), tuiHelpKey("c", "open agent"),
		}
		shortContextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "filters"),
			tuiHelpKey("v", "dashboard"),
		}
	case ViewDetail:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("e", "edit"), tuiHelpKey("r", "rename"),
			tuiHelpKey("a", "artifacts"), tuiHelpKey("l", "links"), tuiHelpKey("o", "open in editor"),
			tuiHelpKey("c", "open agent"), tuiHelpKey("v", "view"),
		}
		shortContextual = []bubbleskey.Binding{
			tuiHelpKey("a", "artifacts"), tuiHelpKey("l", "links"), tuiHelpKey("v", "view"),
		}
	case ViewLeadSelector:
		contextual = []bubbleskey.Binding{tuiHelpKey("←/→", "column"), tuiHelpKey("↑/↓", "move"), tuiHelpKey("esc", "cancel"), tuiHelpKey("enter", "apply")}
		shortContextual = contextual
	case ViewInterfaceSelector:
		contextual = []bubbleskey.Binding{tuiHelpKey("↑/↓", "move"), tuiHelpKey("enter", "apply"), tuiHelpKey("esc", "cancel")}
		shortContextual = contextual
	case ViewArtifactIndex:
		contextual = []bubbleskey.Binding{tuiHelpKey("esc", "back"), tuiHelpKey("enter", "view artifact")}
		shortContextual = []bubbleskey.Binding{tuiHelpKey("esc", "back")}
	case ViewArtifactContent:
		contextual = []bubbleskey.Binding{tuiHelpKey("esc", "back")}
		shortContextual = contextual
	case ViewLinks:
		contextual = []bubbleskey.Binding{tuiHelpKey("enter", "open link"), tuiHelpKey("esc", "close")}
		shortContextual = contextual
	default:
		contextual = []bubbleskey.Binding{tuiHelpKey("esc", "back")}
		shortContextual = contextual
	}

	short := append(append([]bubbleskey.Binding{}, shortContextual...), common...)
	return tuiKeyMap{short: short, full: [][]bubbleskey.Binding{contextual, common}}
}
