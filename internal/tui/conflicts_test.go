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
	makeModel := func(t *testing.T, width int, shared, mine string) (Model, *testStore) {
		t.Helper()
		store := newTestStore()
		seedDossier(store, "dos_conflict", "Conflict Dossier", core.StatusSpark)
		store.dossiers["dos_conflict"].DistilledState.Body = shared
		store.conflicts["conf_tui"] = &core.Conflict{
			ID: "conf_tui", DossierID: "dos_conflict", Kind: "sync_concurrent_edit",
			TS: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), RejectedBody: mine,
		}
		m := dashboardModel(t, store, width, 30)
		m, cmd := press(t, m, "x")
		if cmd == nil {
			t.Fatal("open conflicts did not return a command")
		}
		updated, _ := m.Update(cmd())
		return updated.(Model), store
	}

	long := strings.Repeat("wrap-token ", 20)
	wide, store := makeModel(t, 120, "Decision: price at $40\n"+long, "Decision: price at $45\n"+long)
	wideText := stripANSI(wide.renderConflicts())
	if !strings.Contains(wideText, "Conflict Dossier") || !strings.Contains(wideText, "Shared (current)") || !strings.Contains(wideText, "Yours (preserved)") || !strings.Contains(wideText, "Decision: price at $40") || !strings.Contains(wideText, "Decision: price at $45") {
		t.Fatalf("wide comparison missing content:\n%s", wideText)
	}
	wideSideBySide := false
	for _, line := range strings.Split(wideText, "\n") {
		if strings.Contains(line, "Decision: price at $40") && strings.Contains(line, "Decision: price at $45") {
			wideSideBySide = true
			break
		}
	}
	if !wideSideBySide {
		t.Fatalf("wide comparison is not side-by-side:\n%s", wideText)
	}
	headingLine := ""
	bodyLine := ""
	for _, line := range strings.Split(wideText, "\n") {
		if strings.Contains(line, "Yours (preserved)") {
			headingLine = line
		}
		if strings.Contains(line, "Decision: price at $45") {
			bodyLine = line
		}
	}
	if headingLine == "" || bodyLine == "" || strings.Index(headingLine, "Yours (preserved)") != strings.Index(bodyLine, "Decision: price at $45") {
		t.Fatalf("right column is not aligned:\nheading=%q\nbody=%q", headingLine, bodyLine)
	}
	comparison := wide.renderConflictComparison(wide.conflictDetails["conf_tui"])
	if strings.Count(comparison, "wrap-token") != 40 {
		t.Fatalf("long body was cut instead of wrapped: got %d tokens\n%s", strings.Count(comparison, "wrap-token"), comparison)
	}
	wideView := stripANSI(wide.View())
	for _, line := range strings.Split(wideView, "\n") {
		if strings.TrimSpace(line) == "…" {
			t.Fatalf("overlay contains stray ellipsis row:\n%s\nraw viewport: %q", wideView, wide.conflictViewport.View())
		}
	}
	if !strings.Contains(wideView, "Shared (current)") || !strings.Contains(wideView, "Yours (preserved)") {
		t.Fatalf("wide overlay omitted comparison headings:\n%s", wideView)
	}
	wide, _ = press(t, wide, "d")
	wideText = stripANSI(wide.renderConflicts())
	for _, line := range strings.Split(wideText, "\n") {
		if strings.TrimSpace(line) == "…" {
			t.Fatalf("diff view contains stray ellipsis row:\n%s", wideText)
		}
	}
	if !strings.Contains(wideText, "Diff (shared → yours)") || !strings.Contains(wideText, "- Decision: price at $40") {
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

	narrow, _ := makeModel(t, 70, "shared current\n", "mine\n")
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
