package tui

import (
	"strings"
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
	if got := store.dossiers["dos_conflict"].DistilledState.Body; got != "mine\n" {
		t.Fatalf("body = %q, want mine newline", got)
	}
}

func TestConflictsOverlayShowsCurrentAndMine(t *testing.T) {
	makeModel := func(t *testing.T, width int) (Model, *testStore) {
		t.Helper()
		store := newTestStore()
		seedDossier(store, "dos_conflict", "Conflict Dossier", core.StatusSpark)
		store.dossiers["dos_conflict"].DistilledState.Body = "shared current\n"
		store.conflicts["conf_tui"] = &core.Conflict{
			ID: "conf_tui", DossierID: "dos_conflict", Kind: "sync_concurrent_edit",
			TS: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), RejectedBody: "mine\n",
		}
		m := dashboardModel(t, store, width, 30)
		m, cmd := press(t, m, "x")
		if cmd == nil {
			t.Fatal("open conflicts did not return a command")
		}
		updated, _ := m.Update(cmd())
		return updated.(Model), store
	}

	wide, store := makeModel(t, 120)
	wideText := stripANSI(wide.renderConflicts())
	if !strings.Contains(wideText, "Conflict Dossier") || !strings.Contains(wideText, "Shared (current)") || !strings.Contains(wideText, "Yours (preserved)") || !strings.Contains(wideText, "shared current") || !strings.Contains(wideText, "mine") {
		t.Fatalf("wide comparison missing content:\n%s", wideText)
	}
	wideSideBySide := false
	for _, line := range strings.Split(wideText, "\n") {
		if strings.Contains(line, "shared current") && strings.Contains(line, "mine") {
			wideSideBySide = true
			break
		}
	}
	if !wideSideBySide {
		t.Fatalf("wide comparison is not side-by-side:\n%s", wideText)
	}
	wideView := stripANSI(wide.View())
	if !strings.Contains(wideView, "Shared (current)") || !strings.Contains(wideView, "Yours (preserved)") {
		t.Fatalf("wide overlay omitted comparison headings:\n%s", wideView)
	}
	wide, _ = press(t, wide, "d")
	wideText = stripANSI(wide.renderConflicts())
	if !strings.Contains(wideText, "Diff (shared → yours)") || !strings.Contains(wideText, "- shared current") {
		t.Fatalf("diff view missing:\n%s", wideText)
	}
	wide, cmd := press(t, wide, "1")
	if cmd == nil {
		t.Fatal("keep shared did not return a command")
	}
	updated, _ := wide.Update(cmd())
	if _, ok := store.resolvedConflicts["conf_tui"]; !ok {
		t.Fatal("keep shared did not resolve the conflict")
	}
	_ = updated

	narrow, _ := makeModel(t, 70)
	narrowText := stripANSI(narrow.renderConflicts())
	sharedBody := strings.Index(narrowText, "shared current")
	mineBody := strings.Index(narrowText, "\nmine")
	if mineBody >= 0 {
		mineBody++
	}
	if sharedBody < 0 || mineBody < 0 || sharedBody == mineBody {
		t.Fatalf("narrow comparison missing stacked bodies:\n%s", narrowText)
	}
	if strings.IndexByte(narrowText[sharedBody:mineBody], '\n') < 0 {
		t.Fatalf("narrow comparison is not stacked:\n%s", narrowText)
	}
	narrowView := stripANSI(narrow.View())
	if !strings.Contains(narrowView, "Shared (current)") || !strings.Contains(narrowView, "Yours (preserved)") {
		t.Fatalf("narrow overlay omitted comparison headings:\n%s", narrowView)
	}
}
