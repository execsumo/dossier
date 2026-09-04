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
	switch v {
	case ViewDashboard:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "lead filter"),
			tuiHelpKey("i", "interface filter"), tuiHelpKey("b", "board"),
			tuiHelpKey("e", "edit"), tuiHelpKey("k", "link"),
			tuiHelpKey("m", "merge"), tuiHelpKey("c", "Claude"),
		}
	case ViewKanban:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("/", "search"), tuiHelpKey("f", "lead filter"),
			tuiHelpKey("i", "interface filter"), tuiHelpKey("b", "table"),
			tuiHelpKey("e", "edit"), tuiHelpKey("c", "Claude"),
		}
	case ViewDetail:
		contextual = []bubbleskey.Binding{
			tuiHelpKey("e", "edit"), tuiHelpKey("r", "rename"),
			tuiHelpKey("a", "artifacts"), tuiHelpKey("o", "open in editor"),
			tuiHelpKey("c", "Claude"),
		}
	case ViewLeadSelector:
		contextual = []bubbleskey.Binding{tuiHelpKey("esc", "cancel"), tuiHelpKey("enter", "select")}
	default:
		contextual = []bubbleskey.Binding{tuiHelpKey("esc", "back")}
	}

	short := append(append([]bubbleskey.Binding{}, common...), contextual...)
	return tuiKeyMap{short: short, full: [][]bubbleskey.Binding{contextual, common}}
}
