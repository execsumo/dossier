package tui

import (
	"strings"
	"testing"

	"dossier/internal/core"
	tea "github.com/charmbracelet/bubbletea"
)

func TestExternalLinkOverlaysKeepDossierContextAndOpenSelectedURL(t *testing.T) {
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
	if m.currentView != ViewReferences || len(m.overlayStack) != 1 {
		t.Fatalf("references overlay state = view %v, stack %d; want ViewReferences, 1", m.currentView, len(m.overlayStack))
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Pricing Model · References") || !strings.Contains(view, "PROJ-123") {
		t.Fatalf("references overlay lost dossier context or link label:\n%s", view)
	}
	m, _ = press(t, m, "esc")
	if m.currentView != ViewDetail || len(m.overlayStack) != 0 {
		t.Fatalf("closing references overlay = view %v, stack %d; want ViewDetail, 0", m.currentView, len(m.overlayStack))
	}

	m, _ = press(t, m, "m")
	if m.currentView != ViewActiveMonitors || len(m.overlayStack) != 1 {
		t.Fatalf("monitor overlay state = view %v, stack %d; want ViewActiveMonitors, 1", m.currentView, len(m.overlayStack))
	}
	if !strings.Contains(stripANSI(m.View()), "Last polled: 2026-09-04") {
		t.Fatalf("monitor overlay did not render polling metadata:\n%s", stripANSI(m.View()))
	}
	m, _ = press(t, m, "enter")
	if opened != "https://slack.example/thread" {
		t.Fatalf("opened URL = %q, want selected monitor URL", opened)
	}
	m, _ = press(t, m, "esc")
	if m.currentView != ViewDetail || len(m.overlayStack) != 0 {
		t.Fatalf("closing monitor overlay = view %v, stack %d; want ViewDetail, 0", m.currentView, len(m.overlayStack))
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
	if !strings.Contains(stripANSI(m.View()), "Dashboard · Filters") {
		t.Fatalf("filter overlay did not retain parent context:\n%s", stripANSI(m.View()))
	}
	before := m.interfaceFilter
	m, _ = press(t, m, "tab")
	if m.interfaceFilter == before {
		t.Fatal("tab in filter overlay did not advance the interface filter")
	}
	m, _ = press(t, m, "esc")
	if m.currentView != ViewDashboard || len(m.overlayStack) != 0 {
		t.Fatalf("closing filter overlay = view %v, stack %d; want ViewDashboard, 0", m.currentView, len(m.overlayStack))
	}
}
