package tui

import (
	"fmt"
	"strings"
	"testing"

	"dossier/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// boardModel seeds the store, loads the list, and switches to the board so each
// test starts from the same place a user would.
func boardModel(t *testing.T, store *testStore, width, height int) Model {
	t.Helper()
	m := NewModel(setupTestService(store))
	m.width = width
	m.height = height
	m.recalculateTableLayout()

	newM, _ := m.Update(m.listDossiersCmd()())
	m = newM.(Model)
	m = enterDashboard(t, m)

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = newM.(Model)
	if m.currentView != ViewKanban {
		t.Fatalf("expected ViewKanban after 'b', got %v", m.currentView)
	}
	return m
}

func seedDossier(store *testStore, id, name string, status core.Status, opts ...func(*core.Frontmatter)) {
	fm := core.Frontmatter{
		ID:       id,
		Name:     name,
		Slug:     strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		Status:   status,
		Priority: core.PriorityHigh,
	}
	for _, opt := range opts {
		opt(&fm)
	}
	store.dossiers[id] = &core.Dossier{
		Frontmatter:    fm,
		DistilledState: core.DistilledState{Body: "Distilled " + name},
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, m Model, k string) (Model, tea.Cmd) {
	t.Helper()
	newM, cmd := m.Update(key(k))
	return newM.(Model), cmd
}

func TestGroupByStage(t *testing.T) {
	stages := core.CanonicalStatuses()
	stageIndex := map[core.Status]int{}
	for i, s := range stages {
		stageIndex[s] = i
	}

	tests := []struct {
		name  string
		items []core.ListItem
		want  map[core.Status][]string // stage -> item names, in order
	}{
		{
			name: "canonical statuses bucket in lifecycle order",
			items: []core.ListItem{
				{ID: "1", Name: "a", Status: "spark"},
				{ID: "2", Name: "b", Status: "define"},
				{ID: "3", Name: "c", Status: "delegated"},
				{ID: "4", Name: "d", Status: "review"},
				{ID: "5", Name: "e", Status: "blocked"},
				{ID: "6", Name: "f", Status: "done"},
			},
			want: map[core.Status][]string{
				core.StatusSpark:     {"a"},
				core.StatusDefine:    {"b"},
				core.StatusDelegated: {"c"},
				core.StatusReview:    {"d"},
				core.StatusBlocked:   {"e"},
				core.StatusDone:      {"f"},
			},
		},
		{
			name: "legacy statuses normalize onto canonical stages",
			items: []core.ListItem{
				{ID: "1", Name: "legacy-active", Status: "active"},
				{ID: "2", Name: "legacy-waiting", Status: "waiting"},
				{ID: "3", Name: "legacy-resolved", Status: "resolved"},
				{ID: "4", Name: "legacy-archived", Status: "archived"},
			},
			want: map[core.Status][]string{
				core.StatusDefine:    {"legacy-active"},
				core.StatusDelegated: {"legacy-waiting"},
				core.StatusDone:      {"legacy-resolved", "legacy-archived"},
			},
		},
		{
			name: "placeholder and unknown statuses are skipped",
			items: []core.ListItem{
				{ID: "", Name: "placeholder", Status: "spark"},
				{ID: "1", Name: "bogus", Status: "not-a-status"},
				{ID: "2", Name: "real", Status: "spark"},
			},
			want: map[core.Status][]string{core.StatusSpark: {"real"}},
		},
		{
			name: "input order is preserved inside a column",
			items: []core.ListItem{
				{ID: "1", Name: "first", Status: "review"},
				{ID: "2", Name: "second", Status: "review"},
				{ID: "3", Name: "third", Status: "review"},
			},
			want: map[core.Status][]string{core.StatusReview: {"first", "second", "third"}},
		},
		{
			name:  "empty input yields empty columns",
			items: nil,
			want:  map[core.Status][]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cols := groupByStage(tc.items)
			if len(cols) != len(stages) {
				t.Fatalf("got %d columns, want %d", len(cols), len(stages))
			}
			for stage, wantNames := range tc.want {
				var gotNames []string
				for _, item := range cols[stageIndex[stage]] {
					gotNames = append(gotNames, item.Name)
				}
				if strings.Join(gotNames, ",") != strings.Join(wantNames, ",") {
					t.Errorf("stage %s = %v, want %v", stage, gotNames, wantNames)
				}
			}
			for _, stage := range stages {
				if _, expected := tc.want[stage]; !expected && len(cols[stageIndex[stage]]) != 0 {
					t.Errorf("stage %s should be empty, got %d items", stage, len(cols[stageIndex[stage]]))
				}
			}
		})
	}
}

func TestFitCards(t *testing.T) {
	tests := []struct {
		name               string
		heights            []int
		cursor, budget     int
		wantStart, wantEnd int
	}{
		{name: "everything fits", heights: []int{3, 3, 3}, cursor: 0, budget: 12, wantStart: 0, wantEnd: 3},
		{name: "cursor at top clips the tail", heights: []int{3, 3, 3, 3}, cursor: 0, budget: 6, wantStart: 0, wantEnd: 2},
		{name: "cursor in the middle keeps neighbours", heights: []int{3, 3, 3, 3}, cursor: 2, budget: 6, wantStart: 2, wantEnd: 4},
		{name: "cursor at the last card scrolls up", heights: []int{3, 3, 3, 3}, cursor: 3, budget: 6, wantStart: 2, wantEnd: 4},
		{name: "budget smaller than one card still shows the cursor", heights: []int{5, 5}, cursor: 1, budget: 2, wantStart: 1, wantEnd: 2},
		{name: "zero budget still shows the cursor", heights: []int{3}, cursor: 0, budget: 0, wantStart: 0, wantEnd: 1},
		{name: "empty input", heights: nil, cursor: 0, budget: 10, wantStart: 0, wantEnd: 0},
		{name: "out of range cursor is clamped", heights: []int{3, 3}, cursor: 99, budget: 3, wantStart: 1, wantEnd: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := fitCards(tc.heights, tc.cursor, tc.budget)
			if start != tc.wantStart || end != tc.wantEnd {
				t.Fatalf("fitCards = [%d,%d), want [%d,%d)", start, end, tc.wantStart, tc.wantEnd)
			}
			if len(tc.heights) == 0 {
				return
			}
			cursor := tc.cursor
			if cursor >= len(tc.heights) {
				cursor = len(tc.heights) - 1
			}
			if cursor < start || cursor >= end {
				t.Errorf("cursor %d outside window [%d,%d)", cursor, start, end)
			}
			sum := 0
			for _, h := range tc.heights[start:end] {
				sum += h
			}
			// A single card larger than the budget is allowed to overflow; more
			// than one card must always fit.
			if end-start > 1 && sum > tc.budget {
				t.Errorf("window height %d exceeds budget %d", sum, tc.budget)
			}
		})
	}
}

func TestKanbanNavigationClampsAtEveryEdge(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "s1", "Spark One", core.StatusSpark)
	seedDossier(store, "s2", "Spark Two", core.StatusSpark)
	seedDossier(store, "s3", "Spark Three", core.StatusSpark)
	seedDossier(store, "d1", "Define One", core.StatusDefine)
	m := boardModel(t, store, 140, 40)

	// Left edge: already at column 0, left is inert.
	m, _ = press(t, m, "left")
	if m.kanbanCol != 0 {
		t.Fatalf("left at the first column moved to %d", m.kanbanCol)
	}

	// Up edge.
	m, _ = press(t, m, "up")
	if m.kanbanRow != 0 {
		t.Fatalf("up at the first card moved to row %d", m.kanbanRow)
	}

	// Down to the bottom of spark, then one more.
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down")
	if m.kanbanRow != 2 {
		t.Fatalf("expected row 2 after two downs, got %d", m.kanbanRow)
	}
	m, _ = press(t, m, "down")
	if m.kanbanRow != 2 {
		t.Fatalf("down at the last card moved to row %d", m.kanbanRow)
	}

	// Right into the shorter define column re-clamps the row.
	m, _ = press(t, m, "right")
	if m.kanbanCol != 1 || m.kanbanRow != 0 {
		t.Fatalf("entering define landed at col=%d row=%d, want 1/0", m.kanbanCol, m.kanbanRow)
	}
	if item, ok := m.selectedKanbanItem(); !ok || item.ID != "d1" {
		t.Fatalf("selection = %+v ok=%v, want d1", item, ok)
	}

	// Right into an empty column: row stays 0 and enter is a no-op.
	m, _ = press(t, m, "right")
	if m.kanbanCol != 2 || m.kanbanRow != 0 {
		t.Fatalf("entering delegated landed at col=%d row=%d, want 2/0", m.kanbanCol, m.kanbanRow)
	}
	if _, ok := m.selectedKanbanItem(); ok {
		t.Fatal("empty column reported a selection")
	}
	m, cmd := press(t, m, "enter")
	if cmd != nil {
		t.Fatal("enter on an empty column returned a command")
	}
	if m.currentView != ViewKanban {
		t.Fatalf("enter on an empty column left the board for %v", m.currentView)
	}
	m, _ = press(t, m, "down")
	if m.kanbanRow != 0 {
		t.Fatalf("down on an empty column moved to row %d", m.kanbanRow)
	}

	// Right edge: walk to the last stage and press right once more.
	for i := 0; i < len(core.CanonicalStatuses()); i++ {
		m, _ = press(t, m, "right")
	}
	if want := len(core.CanonicalStatuses()) - 1; m.kanbanCol != want {
		t.Fatalf("right past the last column landed at %d, want %d", m.kanbanCol, want)
	}
}

func TestKanbanEnterOpensDetailAndEscReturnsToBoard(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "s1", "Spark One", core.StatusSpark)
	m := boardModel(t, store, 140, 40)

	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("expected enter on a card to return a recall command")
	}
	if !m.loading {
		t.Error("expected the board to show loading while recalling")
	}
	newM, _ := m.Update(cmd())
	m = newM.(Model)
	if m.currentView != ViewDetail {
		t.Fatalf("expected ViewDetail after recall, got %v", m.currentView)
	}

	m, _ = press(t, m, "esc")
	if m.currentView != ViewKanban {
		t.Fatalf("esc from a board-opened detail returned to %v, want ViewKanban", m.currentView)
	}
}

func TestDetailSlugRenameKeepsIDAndReturnsToDetail(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "s1", "Spark One", core.StatusSpark)
	m := boardModel(t, store, 140, 40)

	m, cmd := press(t, m, "enter")
	newM, _ := m.Update(cmd())
	m = newM.(Model)
	if m.currentView != ViewDetail {
		t.Fatalf("expected detail, got %v", m.currentView)
	}
	m, _ = press(t, m, "r")
	if m.currentView != ViewRenameSlug || m.renameSlugInput.Value() != "spark-one" {
		t.Fatalf("rename view = %v, input = %q", m.currentView, m.renameSlugInput.Value())
	}
	m.renameSlugInput.SetValue("clearer-spark")
	m, cmd = press(t, m, "enter")
	if cmd == nil {
		t.Fatal("expected rename command")
	}
	newM, recallCmd := m.Update(cmd())
	m = newM.(Model)
	if recallCmd == nil {
		t.Fatal("expected detail refresh after rename")
	}
	newM, _ = m.Update(recallCmd())
	m = newM.(Model)
	if m.currentView != ViewDetail {
		t.Fatalf("rename returned to %v, want detail", m.currentView)
	}
	if got := store.dossiers["s1"].Frontmatter; got.Slug != "clearer-spark" {
		t.Fatalf("renamed frontmatter = %+v", got)
	}
	if m.recallResult.Frontmatter.ID != "s1" || m.recallResult.Frontmatter.Slug != "clearer-spark" {
		t.Fatalf("detail did not refresh by immutable ID: %+v", m.recallResult.Frontmatter)
	}
}

func TestDetailTitleRenameKeepsSlug(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "s1", "Original Title", core.StatusSpark)
	m := boardModel(t, store, 140, 40)

	m, cmd := press(t, m, "enter")
	newM, _ := m.Update(cmd())
	m = newM.(Model)
	m, _ = press(t, m, "r")
	m, _ = press(t, m, "tab")
	if m.renameField != renameNameField {
		t.Fatal("tab did not select title")
	}
	m.renameNameInput.SetValue("Renamed Title")
	m, cmd = press(t, m, "enter")
	if cmd == nil {
		t.Fatal("expected title rename command")
	}
	newM, recallCmd := m.Update(cmd())
	m = newM.(Model)
	if recallCmd == nil {
		t.Fatal("expected detail refresh after title rename")
	}
	newM, _ = m.Update(recallCmd())
	m = newM.(Model)
	if got := store.dossiers["s1"].Frontmatter; got.Name != "Renamed Title" || got.Slug != "original-title" {
		t.Fatalf("renamed frontmatter = %+v", got)
	}
}

func TestKanbanHonoursFiltersAndShowsTerminalWork(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "keep", "Keep Me", core.StatusSpark, func(fm *core.Frontmatter) {
		fm.Lead = "Alice"
		fm.Interfaces = []string{"Pricing WBR"}
	})
	seedDossier(store, "done", "Alice Done Work", core.StatusDone, func(fm *core.Frontmatter) {
		fm.Lead = "Alice"
		fm.Interfaces = []string{"Pricing WBR"}
	})
	seedDossier(store, "otherlead", "Bob Topic", core.StatusSpark, func(fm *core.Frontmatter) {
		fm.Lead = "Bob"
		fm.Interfaces = []string{"Pricing WBR"}
	})
	seedDossier(store, "otheriface", "Alice Elsewhere", core.StatusSpark, func(fm *core.Frontmatter) {
		fm.Lead = "Alice"
		fm.Interfaces = []string{"Steerco"}
	})

	m := boardModel(t, store, 140, 40)
	m.leadFilter = leadFilter{kind: filterByName, name: "Alice"}
	m.interfaceFilter = interfaceFilter("Pricing WBR")
	m.applyFilters()

	view := stripANSI(m.View())
	for _, want := range []string{"Keep Me", "Alice Done Work"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected board to show %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Bob Topic", "Alice Elsewhere"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("expected %q to be filtered out, got:\n%s", unwanted, view)
		}
	}

	// The dashboard collapses done work into the extras row; the board must not.
	if m.extrasCount != 1 {
		t.Fatalf("expected the dashboard to hold 1 collapsed extra, got %d", m.extrasCount)
	}
	for _, item := range m.visibleItems {
		if item.ID == "done" {
			t.Fatal("the collapsed dashboard should not list the done dossier")
		}
	}
	doneCol := m.kanbanColumns[len(core.CanonicalStatuses())-1]
	if len(doneCol) != 1 || doneCol[0].ID != "done" {
		t.Fatalf("done column = %+v, want the single done dossier", doneCol)
	}
}

func TestKanbanDoneCardsOmitDescription(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "open", "Open Topic", core.StatusReview, func(fm *core.Frontmatter) {
		fm.Description = "open summary text"
	})
	seedDossier(store, "shut", "Shut Topic", core.StatusDone, func(fm *core.Frontmatter) {
		fm.Description = "done summary text"
	})
	m := boardModel(t, store, 140, 40)

	view := stripANSI(m.View())
	if !strings.Contains(view, "Open Topic") || !strings.Contains(view, "open summary") {
		t.Errorf("expected a non-done card to render name and description, got:\n%s", view)
	}
	if !strings.Contains(view, "Shut Topic") {
		t.Errorf("expected the done card's name, got:\n%s", view)
	}
	if strings.Contains(view, "done summary") {
		t.Errorf("done cards must not render a description, got:\n%s", view)
	}
}

func TestKanbanCardsShowAssignedLeadAsLastRow(t *testing.T) {
	item := core.ListItem{
		ID:          "d1",
		Name:        "Pricing Topic",
		Description: "summary text",
		Lead:        "Alice Smith",
	}

	lines := strings.Split(stripANSI(renderCard(item, 30, false, true)), "\n")
	leadLine := -1
	descriptionLine := -1
	for i, line := range lines {
		if strings.Contains(line, "summary text") {
			descriptionLine = i
		}
		if strings.Contains(line, "Alice") {
			leadLine = i
		}
	}
	if descriptionLine == -1 || leadLine == -1 {
		t.Fatalf("expected card to contain description and lead, got:\n%s", strings.Join(lines, "\n"))
	}
	if leadLine <= descriptionLine {
		t.Fatalf("expected lead to be after the description, got:\n%s", strings.Join(lines, "\n"))
	}
	card := strings.Join(lines, "\n")
	if strings.Contains(card, "Alice Smith") {
		t.Fatalf("expected only the lead's first name, got:\n%s", card)
	}
	if strings.Contains(card, "Lead:") {
		t.Fatalf("expected the lead field name to be omitted, got:\n%s", card)
	}

	unassigned := item
	unassigned.Lead = ""
	if got, want := lipgloss.Height(renderCard(unassigned, 30, false, true)), lipgloss.Height(renderCard(item, 30, false, true))-1; got != want {
		t.Fatalf("unassigned card height = %d, want assigned height minus one (%d)", got, want)
	}
}

func TestKanbanToggleAndTextInputIsolation(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "s1", "Spark One", core.StatusSpark)
	m := boardModel(t, store, 140, 40)

	// 'b' toggles back to the dashboard, and the return path follows it.
	m, _ = press(t, m, "b")
	if m.currentView != ViewDashboard || m.listView != ViewDashboard {
		t.Fatalf("'b' from the board gave currentView=%v listView=%v", m.currentView, m.listView)
	}
	m, _ = press(t, m, "b")
	if m.currentView != ViewKanban || m.listView != ViewKanban {
		t.Fatalf("'b' from the dashboard gave currentView=%v listView=%v", m.currentView, m.listView)
	}

	// esc is the other way out of the board.
	m, _ = press(t, m, "esc")
	if m.currentView != ViewDashboard {
		t.Fatalf("esc from the board gave %v", m.currentView)
	}

	// 'b' inside a text field must type, not toggle. The editor's lead row is
	// the board's reachable text input.
	m, _ = press(t, m, "b")
	m, _ = press(t, m, "e")
	if m.currentView != ViewEdit {
		t.Fatalf("expected ViewEdit, got %v", m.currentView)
	}
	m.editFocus = editFieldLead
	m.syncEditFocus()
	m.leadInput.SetValue("")
	m, _ = press(t, m, "b")
	if got := m.leadInput.Value(); got != "b" {
		t.Errorf("lead input = %q, want the typed character %q", got, "b")
	}
	if m.currentView != ViewEdit {
		t.Fatalf("typing 'b' left the editor for %v", m.currentView)
	}

	// Saving from the board returns to the board, not the dashboard.
	newM, cmd := m.Update(key("enter"))
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected enter to return a save command")
	}
	newM, _ = m.Update(cmd())
	m = newM.(Model)
	if m.currentView != ViewKanban {
		t.Fatalf("saving from the board returned to %v, want ViewKanban", m.currentView)
	}
}

func TestKanbanNarrowTerminalWindowsStages(t *testing.T) {
	store := newTestStore()
	for i, stage := range core.CanonicalStatuses() {
		seedDossier(store, fmt.Sprintf("d%d", i), fmt.Sprintf("Topic %s", stage), stage)
	}
	const width, height = 80, 24
	m := boardModel(t, store, width, height)

	// 80 columns fits 3 of the 6 stages at the 20-column minimum
	// (3*20 + 2 gutters = 62; a fourth would need 83).
	start, end := m.kanbanStageWindow()
	if end-start != 3 {
		t.Fatalf("expected 3 visible stages at width %d, got %d", width, end-start)
	}

	for _, col := range []int{0, 3, 5} {
		m.kanbanCol = col
		m.clampKanbanCursor()
		start, end = m.kanbanStageWindow()
		if col < start || col >= end {
			t.Fatalf("selected stage %d outside the window [%d,%d)", col, start, end)
		}

		view := m.View()
		clean := stripANSI(view)

		// The subtitle is composed with the stage note and then fitted to the
		// width, so at 80 columns the note is cut off (see the note-visibility
		// gap flagged alongside this test). Assert the composition instead: the
		// rendered subtitle must be a prefix of the full text including the note.
		full := fmt.Sprintf(" %s — Board · Lead: %s · Interface: %s · stages %d–%d of 6",
			subheadline, m.leadFilter.label(), m.interfaceFilter.label(), start+1, end)
		subtitle := strings.TrimSuffix(stripANSI(strings.Split(view, "\n")[1]), "…")
		if !strings.HasPrefix(full, subtitle) {
			t.Errorf("col %d: subtitle %q is not a width-fitted prefix of %q", col, subtitle, full)
		}
		if strings.Contains(clean, strings.ToUpper(string(core.CanonicalStatuses()[0]))) && start != 0 {
			t.Errorf("stage %q rendered outside the window", core.CanonicalStatuses()[0])
		}

		lines := strings.Split(view, "\n")
		if len(lines) != height {
			t.Fatalf("col %d: board rendered %d lines, want %d", col, len(lines), height)
		}
		last := stripANSI(lines[len(lines)-1])
		if strings.TrimSpace(last) == "" {
			t.Errorf("col %d: expected the footer on the bottom line, got %q", col, last)
		}
	}

	// At a width that is still windowed but roomy enough for the whole subtitle,
	// the stage note must render in full — the width fitting above trims the note
	// only once the line genuinely does not fit.
	wide := m
	newM, _ := wide.Update(tea.WindowSizeMsg{Width: 110, Height: height})
	wide = newM.(Model)
	wide.kanbanCol = 0
	wide.clampKanbanCursor()
	start, end = wide.kanbanStageWindow()
	if end-start >= len(core.CanonicalStatuses()) {
		t.Fatalf("expected width 110 to still window stages, got %d of %d", end-start, len(core.CanonicalStatuses()))
	}
	if note := fmt.Sprintf(" · stages %d–%d of 6", start+1, end); !strings.Contains(stripANSI(wide.View()), note) {
		t.Errorf("expected the full stage note %q at width 110, got:\n%s", note, stripANSI(wide.View()))
	}
}

func TestKanbanRendersAtDegenerateSizes(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "s1", "Spark One", core.StatusSpark, func(fm *core.Frontmatter) {
		fm.Description = strings.Repeat("a very long summary that must wrap ", 10)
	})
	seedDossier(store, "s2", "Spark Two", core.StatusSpark)
	seedDossier(store, "s3", "Spark Three", core.StatusSpark)
	seedDossier(store, "b1", "Blocked One", core.StatusBlocked)
	m := boardModel(t, store, 140, 40)

	for _, dim := range []struct{ w, h int }{{1, 1}, {1, 40}, {140, 1}, {2, 3}, {20, 5}, {40, 10}, {0, 0}} {
		for col := 0; col < len(core.CanonicalStatuses()); col++ {
			m.width, m.height = dim.w, dim.h
			m.kanbanCol = col
			m.clampKanbanCursor()
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%dx%d col %d panicked: %v", dim.w, dim.h, col, r)
					}
				}()
				_ = m.View()
				_ = m.renderKanban()
			}()
		}
	}
}

func TestKanbanClipsLongColumnsWithMoreMarkers(t *testing.T) {
	store := newTestStore()
	for i := 0; i < 12; i++ {
		seedDossier(store, fmt.Sprintf("s%d", i), fmt.Sprintf("Spark %02d", i), core.StatusSpark)
	}
	const height = 20
	m := boardModel(t, store, 140, height)

	view := m.View()
	if lines := strings.Split(view, "\n"); len(lines) != height {
		t.Fatalf("board rendered %d lines, want %d", len(lines), height)
	}
	clean := stripANSI(view)
	if !strings.Contains(clean, "more") {
		t.Errorf("expected a clipping marker in a column that overflows, got:\n%s", clean)
	}

	// Walking to the bottom card must keep it visible, marker included. The
	// column's order is the list's, so the selected item is read back from the
	// model rather than assumed.
	for i := 0; i < 11; i++ {
		m, _ = press(t, m, "down")
	}
	if m.kanbanRow != 11 {
		t.Fatalf("expected the cursor on the last card, got row %d", m.kanbanRow)
	}
	selected, ok := m.selectedKanbanItem()
	if !ok {
		t.Fatal("expected a selected card at the bottom of the column")
	}
	clean = stripANSI(m.View())
	if !strings.Contains(clean, selected.Name) {
		t.Errorf("expected the selected bottom card %q to stay in view, got:\n%s", selected.Name, clean)
	}
	if !strings.Contains(clean, "↑") {
		t.Errorf("expected an upward clipping marker, got:\n%s", clean)
	}
}

// TestListSubtitlesFitTerminalWidth guards the wrap that line-count assertions
// cannot see: an over-wide subtitle emits no "\n", so it passes a line count
// while a real terminal soft-wraps it and pushes the footer off the bottom.
// Both home surfaces are checked, and width is measured in display cells —
// len() counts bytes, and the box-drawing runes are three bytes each.
func TestListSubtitlesFitTerminalWidth(t *testing.T) {
	store := newTestStore()
	for i, stage := range core.CanonicalStatuses() {
		seedDossier(store, fmt.Sprintf("d%d", i), fmt.Sprintf("A rather long dossier name %s", stage), stage,
			func(fm *core.Frontmatter) {
				fm.Description = "a summary long enough to wrap across several card lines"
				fm.Lead = "Alexandra Featherstonehaugh"
				fm.Interfaces = []string{"Pricing WBR"}
			})
	}

	for _, dim := range []struct{ w, h int }{{80, 24}, {60, 20}} {
		m := boardModel(t, store, dim.w, dim.h)

		for _, surface := range []struct {
			name string
			view View
		}{{"board", ViewKanban}, {"dashboard", ViewDashboard}} {
			m.currentView = surface.view
			m.listView = surface.view

			newM, _ := m.Update(tea.WindowSizeMsg{Width: dim.w, Height: dim.h})
			sized := newM.(Model)

			out := sized.View()
			lines := strings.Split(out, "\n")
			if len(lines) != dim.h {
				t.Errorf("%s %dx%d: rendered %d lines, want %d", surface.name, dim.w, dim.h, len(lines), dim.h)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > dim.w {
					t.Errorf("%s %dx%d: line %d is %d cells wide, want <= %d: %q",
						surface.name, dim.w, dim.h, i, w, dim.w, stripANSI(line))
				}
			}

			// The subtitle is line 1 and must still name the surface after the cut.
			subtitle := stripANSI(lines[1])
			want := "Dashboard"
			if surface.view == ViewKanban {
				want = "Board"
			}
			if !strings.Contains(subtitle, want) {
				t.Errorf("%s %dx%d: subtitle %q lost its surface name", surface.name, dim.w, dim.h, subtitle)
			}
			if !strings.HasSuffix(subtitle, "…") {
				t.Errorf("%s %dx%d: expected the truncated subtitle to end in an ellipsis, got %q",
					surface.name, dim.w, dim.h, subtitle)
			}
		}
	}
}

// TestEmptyStateFitsTerminalWidth covers the same overflow on the screen most
// likely to show it: an empty home surface right after a filter change. The
// message interpolates the lead and interface labels, so a long lead name pushes
// it well past a narrow terminal. Where the message sits relative to the pinned
// headers is TestEmptyStateKeepsHeadersPinned's job; this one only cares that it
// is cut to the terminal width.
func TestEmptyStateFitsTerminalWidth(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "a", "Alpha", core.StatusSpark)

	for _, dim := range []struct{ w, h int }{{80, 24}, {60, 20}} {
		m := boardModel(t, store, dim.w, dim.h)
		// A filter pair that matches nothing, with a lead name long enough to
		// overflow both widths.
		m.leadFilter = leadFilter{kind: filterByName, name: "Alexandra Featherstonehaugh"}
		m.interfaceFilter = interfaceFilter("Pricing WBR")
		m.applyFilters()
		m.populateTableRows()

		for _, surface := range []struct {
			name string
			view View
		}{{"board", ViewKanban}, {"dashboard", ViewDashboard}} {
			m.currentView = surface.view
			m.listView = surface.view

			newM, _ := m.Update(tea.WindowSizeMsg{Width: dim.w, Height: dim.h})
			sized := newM.(Model)

			lines := strings.Split(sized.View(), "\n")
			for i, line := range lines {
				if w := lipgloss.Width(line); w > dim.w {
					t.Errorf("%s %dx%d: line %d is %d cells wide, want <= %d: %q",
						surface.name, dim.w, dim.h, i, w, dim.w, stripANSI(line))
				}
			}

			msgIdx := -1
			for i, line := range lines {
				if strings.HasPrefix(stripANSI(line), " No dossiers for lead:") {
					msgIdx = i
					break
				}
			}
			if msgIdx < 0 {
				t.Fatalf("%s %dx%d: expected the empty-state message, got:\n%s",
					surface.name, dim.w, dim.h, stripANSI(sized.View()))
			}
			if !strings.HasSuffix(stripANSI(lines[msgIdx]), "…") {
				t.Errorf("%s %dx%d: expected the empty-state message to be cut with an ellipsis, got %q",
					surface.name, dim.w, dim.h, stripANSI(lines[msgIdx]))
			}
			// The trailing newline lives outside the fitted text, so the message
			// must still be its own line with the footer below it.
			if msgIdx+1 >= len(lines) {
				t.Fatalf("%s %dx%d: the empty-state message swallowed its newline: %q",
					surface.name, dim.w, dim.h, stripANSI(sized.View()))
			}
			if strings.TrimSpace(stripANSI(lines[len(lines)-1])) == "" {
				t.Errorf("%s %dx%d: expected the footer below the empty-state message, got %q",
					surface.name, dim.w, dim.h, stripANSI(lines[len(lines)-1]))
			}
		}
	}
}
