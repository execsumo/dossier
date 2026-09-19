package tui

import (
	"testing"
	"time"

	"dossier/internal/core"
)

func TestConflictsOverlayResolvesThroughService(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "dos_conflict", "Conflict", core.StatusSpark)
	store.conflicts["conf_tui"] = &core.Conflict{
		ID: "conf_tui", DossierID: "dos_conflict", Kind: "sync_concurrent_edit",
		TS: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), RejectedBody: "mine",
	}
	m := dashboardModel(t, store, 100, 30)
	m, cmd := press(t, m, "x")
	if cmd == nil || m.currentView != ViewConflicts {
		t.Fatalf("open conflicts: view=%v cmd=%v", m.currentView, cmd != nil)
	}
	updated, _ := m.Update(cmd())
	m = updated.(Model)
	if len(m.conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(m.conflicts))
	}
	m, cmd = press(t, m, "2")
	if cmd == nil || !m.loading {
		t.Fatalf("restore mine did not start: cmd=%v loading=%v", cmd != nil, m.loading)
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if _, ok := store.resolvedConflicts["conf_tui"]; !ok {
		t.Fatal("TUI resolution did not archive the conflict")
	}
	if got := store.dossiers["dos_conflict"].DistilledState.Body; got != "mine" {
		t.Fatalf("body = %q, want mine", got)
	}
}
