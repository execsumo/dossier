package tui

import (
	"strings"
	"testing"

	"dossier/internal/core"

	"github.com/charmbracelet/lipgloss"
)

func TestHotRefreshKeepsEveryDetailOverlayClosable(t *testing.T) {
	tests := []struct {
		name string
		open func(*testing.T, Model) Model
		want View
	}{
		{
			name: "links",
			open: func(t *testing.T, m Model) Model {
				m, _ = press(t, m, "l")
				return m
			},
			want: ViewLinks,
		},
		{
			name: "contracts",
			open: func(t *testing.T, m Model) Model {
				m, _ = press(t, m, "d")
				return m
			},
			want: ViewContracts,
		},
		{
			name: "edit",
			open: func(t *testing.T, m Model) Model {
				m, _ = press(t, m, "e")
				return m
			},
			want: ViewEdit,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore()
			seedDossier(store, "dos1", "Refreshable", core.StatusSpark)
			m := detailModel(t, store, "dos1", 100, 30)
			m = tc.open(t, m)
			if m.currentView != tc.want || len(m.overlayStack) != 1 {
				t.Fatalf("opened view=%v stack=%d, want %v over detail", m.currentView, len(m.overlayStack), tc.want)
			}

			newM, _ := m.Update(m.recallDossierCmd("dos1")())
			m = newM.(Model)
			if m.currentView != tc.want || len(m.overlayStack) != 1 || m.overlayStack[0] != tc.want {
				t.Fatalf("refresh changed overlay: view=%v stack=%v", m.currentView, m.overlayStack)
			}

			m, _ = press(t, m, "esc")
			if m.currentView != ViewDetail || len(m.overlayStack) != 0 {
				t.Fatalf("escape after refresh gave view=%v stack=%d", m.currentView, len(m.overlayStack))
			}
		})
	}
}

func TestLateRecallCannotReplaceNewerSelection(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "first", "First", core.StatusSpark)
	seedDossier(store, "second", "Second", core.StatusSpark)
	m := dashboardModel(t, store, 100, 30)

	first := m.recallDossierCmd("first")
	second := m.recallDossierCmd("second")

	newM, _ := m.Update(first())
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Fatalf("late recall changed view to %v", m.currentView)
	}
	if m.recallResult.Frontmatter.ID != "" {
		t.Fatalf("late recall populated %q", m.recallResult.Frontmatter.ID)
	}

	newM, _ = m.Update(second())
	m = newM.(Model)
	if m.currentView != ViewDetail || m.recallResult.Frontmatter.ID != "second" {
		t.Fatalf("newer recall state = view %v dossier %q", m.currentView, m.recallResult.Frontmatter.ID)
	}
}

func TestListRefreshRestoresSelectionByImmutableID(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "first", "First", core.StatusSpark)
	seedDossier(store, "second", "Second", core.StatusSpark)
	m := dashboardModel(t, store, 100, 30)
	m.table.SetCursor(1)
	selected, ok := m.selectedListItem()
	if !ok {
		t.Fatal("test setup did not select a dossier")
	}

	// A priority change reorders the list, but the user should stay on the same
	// immutable dossier rather than the same row number.
	store.dossiers["first"].Frontmatter.Priority = core.PriorityMax
	newM, _ := m.Update(m.listDossiersCmd()())
	m = newM.(Model)
	got, ok := m.selectedListItem()
	if !ok || got.ID != selected.ID {
		t.Fatalf("refresh selected %+v, want %q", got, selected.ID)
	}
}

func TestListRowsCarryRevisionForOptimisticEdits(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "dos1", "Revisioned", core.StatusSpark)
	m := dashboardModel(t, store, 100, 30)
	item, ok := m.selectedListItem()
	if !ok {
		t.Fatal("test setup did not select a dossier")
	}
	if item.Revision != "rev1" {
		t.Fatalf("list revision = %q, want rev1", item.Revision)
	}
	target := targetFromListItem(item)
	if target.baseRevision != "rev1" {
		t.Fatalf("editor target revision = %q, want rev1", target.baseRevision)
	}
}

func TestResultWarningsAndNextActionsRenderInBoundedStatusArea(t *testing.T) {
	m := NewModel(setupTestService(newTestStore()))
	m.width = 60
	m.height = 20
	m.applyResultStatus(
		[]core.Warning{"Transcript capture is unavailable in this session."},
		[]core.NextAction{"Run dossier doctor before the next handoff."},
	)
	view := stripANSI(m.View())
	if !strings.Contains(view, "Transcript capture") || !strings.Contains(view, "Run dossier doctor") {
		t.Fatalf("status envelope not visible:\n%s", view)
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if width := lipgloss.Width(line); width > m.width {
			t.Errorf("line %d is %d cells wide, want <= %d: %q", i, width, m.width, stripANSI(line))
		}
	}
	if lines := len(strings.Split(view, "\n")); lines > m.height {
		t.Fatalf("status area rendered %d lines, want <= %d", lines, m.height)
	}
}

func TestResponsiveFallbackAndModalBudgets(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "dos1", "Responsive", core.StatusSpark)

	small := dashboardModel(t, store, 40, 10)
	smallView := stripANSI(small.View())
	if !strings.Contains(smallView, "Terminal too small") {
		t.Fatalf("small terminal did not show fallback:\n%s", smallView)
	}
	if lines := len(strings.Split(small.View(), "\n")); lines != 10 {
		t.Fatalf("small terminal rendered %d lines, want 10", lines)
	}

	m := dashboardModel(t, store, 60, 20)
	m, _ = press(t, m, "f")
	filterView := m.View()
	if lines := len(strings.Split(filterView, "\n")); lines > m.height {
		t.Fatalf("filter modal rendered %d lines, want <= %d", lines, m.height)
	}
	if !strings.Contains(stripANSI(filterView), "apply") {
		t.Fatalf("filter modal lost its action footer:\n%s", stripANSI(filterView))
	}

	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "e")
	editorView := m.View()
	if lines := len(strings.Split(editorView, "\n")); lines > m.height {
		t.Fatalf("editor modal rendered %d lines, want <= %d", lines, m.height)
	}
	if !strings.Contains(stripANSI(editorView), "save") {
		t.Fatalf("editor modal lost its save action:\n%s", stripANSI(editorView))
	}
}
