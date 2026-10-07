package tui

import (
	"strings"
	"testing"

	"dossier/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

func TestLinksOverlayKeepsDossierContextAndPrioritizesMonitors(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "dos1", "Pricing Model", core.StatusReview, func(fm *core.Frontmatter) {})
	store.dossiers["dos1"].DistilledState.Body = `# Pricing Model

## References
- [ticket: PROJ-123](https://jira.example/PROJ-123) — Pricing migration work.

## Active Monitors
- [comms: #pricing-bug](https://slack.example/thread) — Watch approval changes. (Last polled: 2026-09-04)

## Current State
Pricing review is underway.`

	m := detailModel(t, store, "dos1", 100, 30)
	if len(m.recallResult.References) != 1 || len(m.recallResult.ActiveMonitors) != 1 {
		t.Fatalf("recall did not expose parsed external links: refs=%+v monitors=%+v", m.recallResult.References, m.recallResult.ActiveMonitors)
	}

	var opened string
	m.openURL = func(rawURL string) tea.Cmd {
		opened = rawURL
		return nil
	}

	m, _ = press(t, m, "l")
	if m.currentView != ViewLinks || len(m.overlayStack) != 1 {
		t.Fatalf("links overlay state = view %v, stack %d; want ViewLinks, 1", m.currentView, len(m.overlayStack))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Pricing Model · View Links") || !strings.Contains(view, "PROJ-123") {
		t.Fatalf("links overlay lost dossier context or link label:\n%s", view)
	}
	linksBody := stripANSI(m.renderExternalLinks())
	monitorsAt := strings.Index(linksBody, "Active Monitors")
	referencesAt := strings.Index(linksBody, "References")
	if monitorsAt < 0 || referencesAt < 0 || monitorsAt > referencesAt {
		t.Fatalf("links overlay did not prioritize monitors:\n%s", linksBody)
	}
	if !strings.Contains(stripANSI(m.View()), "Last polled: 2026-09-04") {
		t.Fatalf("links overlay did not render polling metadata:\n%s", stripANSI(m.View()))
	}
	m, _ = press(t, m, "enter")
	if opened != "https://slack.example/thread" {
		t.Fatalf("opened first URL = %q, want selected monitor URL", opened)
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "enter")
	if opened != "https://jira.example/PROJ-123" {
		t.Fatalf("opened second URL = %q, want selected reference URL", opened)
	}
	m, _ = press(t, m, "esc")
	if m.currentView != ViewDetail || len(m.overlayStack) != 0 {
		t.Fatalf("closing links overlay = view %v, stack %d; want ViewDetail, 0", m.currentView, len(m.overlayStack))
	}
}

func TestLinksOpenFromDashboardAndKanban(t *testing.T) {
	for _, surface := range []View{ViewDashboard, ViewKanban} {
		t.Run(surface.String(), func(t *testing.T) {
			store := newTestStore()
			seedDossier(store, "dos1", "Planning", core.StatusSpark, func(fm *core.Frontmatter) {})
			store.dossiers["dos1"].DistilledState.Body = `# Planning

## References
- [ticket: PROJ-456](https://jira.example/PROJ-456) — Planning work.`

			var m Model
			if surface == ViewDashboard {
				m = dashboardModel(t, store, 100, 30)
			} else {
				m = boardModel(t, store, 100, 30)
			}
			m, cmd := press(t, m, "l")
			if cmd == nil || !m.loading {
				t.Fatalf("links from %v did not start recall: cmd=%v loading=%v", surface, cmd != nil, m.loading)
			}
			newM, _ := m.Update(cmd())
			m = newM.(Model)
			if m.currentView != ViewLinks || m.overlayBase != surface || len(m.overlayStack) != 1 {
				t.Fatalf("links overlay state = view %v base %v stack %d; want links over %v", m.currentView, m.overlayBase, len(m.overlayStack), surface)
			}
			if !strings.Contains(stripANSI(m.View()), "PROJ-456") {
				t.Fatalf("links overlay from %v missing reference:\n%s", surface, stripANSI(m.View()))
			}
			m, _ = press(t, m, "esc")
			if m.currentView != surface || len(m.overlayStack) != 0 {
				t.Fatalf("closing links from %v = view %v stack %d", surface, m.currentView, len(m.overlayStack))
			}
		})
	}
}

func (v View) String() string {
	switch v {
	case ViewDashboard:
		return "dashboard"
	case ViewKanban:
		return "kanban"
	default:
		return "view"
	}
}

func TestFilterOverlayUsesSharedModalNavigation(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "dos1", "Pricing Model", core.StatusSpark, func(fm *core.Frontmatter) {
		fm.Lead = "Alice"
	})
	m := dashboardModel(t, store, 100, 30)

	m, _ = press(t, m, "f")
	if m.currentView != ViewLeadSelector || len(m.overlayStack) != 1 {
		t.Fatalf("filter overlay state = view %v, stack %d; want ViewLeadSelector, 1", m.currentView, len(m.overlayStack))
	}
	if !strings.Contains(stripANSI(m.View()), "Dashboard · Filter Dossiers") {
		t.Fatalf("filter overlay did not retain parent context:\n%s", stripANSI(m.View()))
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "enter")
	if m.leadFilter.label() != "Alice" {
		t.Fatalf("lead selection did not apply: %q", m.leadFilter.label())
	}
	m, _ = press(t, m, "f")
	m, _ = press(t, m, "right")
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "enter")
	if m.interfaceFilter != interfaceFilter(m.configuredInterfaces[0]) {
		t.Fatalf("interface selection did not apply: %q", m.interfaceFilter)
	}
	m, _ = press(t, m, "f")
	m, _ = press(t, m, "esc")
	if m.currentView != ViewDashboard || len(m.overlayStack) != 0 {
		t.Fatalf("closing filter overlay = view %v, stack %d; want ViewDashboard, 0", m.currentView, len(m.overlayStack))
	}
}

func TestLinksOverlayAddsReference(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "dos1", "Pricing Model", core.StatusReview, func(fm *core.Frontmatter) {})
	store.dossiers["dos1"].DistilledState.Body = "# Pricing Model\n\n## Current State\nUnderway.\n"

	m := detailModel(t, store, "dos1", 100, 30)
	m, _ = press(t, m, "l")
	m, _ = press(t, m, "a")
	if m.currentView != ViewLinkAdd || len(m.overlayStack) != 2 {
		t.Fatalf("add form state = view %v, stack %d; want ViewLinkAdd, 2", m.currentView, len(m.overlayStack))
	}
	// "k" and "j" must be typed into the field, not treated as navigation.
	m, _ = press(t, m, "https://example.test/kj")
	m, _ = press(t, m, "tab")
	m, _ = press(t, m, "Spec")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("enter should save the reference")
	}
	updated, cmd := m.Update(cmd())
	m = updated.(Model)
	if m.currentView != ViewLinks || len(m.overlayStack) != 1 {
		t.Fatalf("after save = view %v, stack %d; want ViewLinks, 1", m.currentView, len(m.overlayStack))
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.currentView != ViewLinks {
		t.Fatalf("recall refresh changed view to %v", m.currentView)
	}
	refs := m.recallResult.References
	if len(refs) != 1 || refs[0].URL != "https://example.test/kj" || refs[0].Label != "Spec" {
		t.Fatalf("references after add = %+v", refs)
	}
	if !strings.Contains(stripANSI(m.View()), "Spec") {
		t.Fatalf("links overlay does not show the new reference:\n%s", stripANSI(m.View()))
	}

	// A rejected URL keeps the form open and surfaces the error.
	m, _ = press(t, m, "a")
	m, _ = press(t, m, "not a url")
	m, cmd = press(t, m, "enter")
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.currentView != ViewLinkAdd || m.err == nil {
		t.Fatalf("invalid URL: view %v err %v; want form open with an error", m.currentView, m.err)
	}
}
