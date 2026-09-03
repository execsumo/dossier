package tui

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var ansiRegex = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func TestSearchFiltersDashboardAndKanbanFromSameSet(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "billing", "Billing Review", core.StatusSpark, func(fm *core.Frontmatter) {
		fm.Description = "Quarterly forecast"
	})
	seedDossier(store, "roadmap", "Product Roadmap", core.StatusSpark)
	m := boardModel(t, store, 120, 40)
	m.currentView = ViewDashboard
	m.listView = ViewDashboard
	m.searchActive = true
	m.searchInput.Focus()
	m.searchInput.SetValue("forecast")
	m.searchQuery = core.NewQuery(m.searchInput.Value())
	m.applyFilters()
	if len(m.visibleItems) != 1 || m.visibleItems[0].ID != "billing" {
		t.Fatalf("dashboard search results = %+v, want billing", m.visibleItems)
	}
	dashboardIDs := []string{m.visibleItems[0].ID}
	m.currentView = ViewKanban
	m.listView = ViewKanban
	m.applyFilters()
	var boardIDs []string
	for _, column := range m.kanbanColumns {
		for _, item := range column {
			boardIDs = append(boardIDs, item.ID)
		}
	}
	if len(boardIDs) != 1 || boardIDs[0] != dashboardIDs[0] {
		t.Fatalf("kanban search results = %v, want %v", boardIDs, dashboardIDs)
	}
}

func TestSearchIncludesCollapsedExtrasWithoutMutatingExpansion(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "old", "Archived Billing", core.StatusDone)
	m := boardModel(t, store, 120, 40)
	m.currentView = ViewDashboard
	m.listView = ViewDashboard
	m.extrasExpanded = false
	m.searchActive = true
	m.searchInput.SetValue("archived")
	m.searchQuery = core.NewQuery(m.searchInput.Value())
	m.applyFilters()
	if m.extrasExpanded {
		t.Fatal("search must not mutate extrasExpanded")
	}
	if len(m.visibleItems) != 1 || m.visibleItems[0].ID != "old" {
		t.Fatalf("search results = %+v, want archived dossier", m.visibleItems)
	}
}

// Quit must stay reachable from inside the search box: ctrl+c is otherwise
// swallowed by the text input, leaving no way to kill the TUI while filtering.
func TestSearchModeCtrlCStillQuits(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "one", "One Topic", core.StatusSpark)
	m := boardModel(t, store, 120, 40)
	m.currentView = ViewDashboard
	m.listView = ViewDashboard
	m, _ = press(t, m, "/")
	if !m.searchActive {
		t.Fatal("/ did not enter search mode")
	}
	_, cmd := press(t, m, "ctrl+c")
	if cmd == nil {
		t.Fatal("ctrl+c returned no command while searching")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c while searching = %T, want tea.QuitMsg", cmd())
	}
}

func TestSearchModeLifecycleAndKeyIsolation(t *testing.T) {
	store := newTestStore()
	seedDossier(store, "one", "One Topic", core.StatusSpark)
	seedDossier(store, "two", "Second Topic", core.StatusSpark)
	m := boardModel(t, store, 120, 40)
	m.currentView = ViewDashboard
	m.listView = ViewDashboard
	m, _ = press(t, m, "/")
	if !m.searchActive {
		t.Fatal("/ did not enter search mode")
	}
	searchCursor := m.table.Cursor()
	m, _ = press(t, m, "s")
	if m.searchInput.Value() != "s" || m.currentView != ViewDashboard || m.table.Cursor() != searchCursor {
		t.Fatalf("search key leaked to another handler: value=%q view=%v cursor=%d before=%d", m.searchInput.Value(), m.currentView, m.table.Cursor(), searchCursor)
	}
	m, _ = press(t, m, "tab")
	if m.searchActive || m.searchQuery.IsEmpty() {
		t.Fatalf("tab should commit the query: active=%v queryEmpty=%v", m.searchActive, m.searchQuery.IsEmpty())
	}
	m, _ = press(t, m, "/")
	m, _ = press(t, m, "esc")
	if m.searchActive || !m.searchQuery.IsEmpty() || m.searchInput.Value() != "" {
		t.Fatalf("esc should clear search: active=%v queryEmpty=%v value=%q", m.searchActive, m.searchQuery.IsEmpty(), m.searchInput.Value())
	}
}

// enterDashboard keeps tests that explicitly construct a lead-selector model
// compatible with the dashboard-focused startup behavior.
func enterDashboard(t *testing.T, m Model) Model {
	t.Helper()
	if m.currentView != ViewLeadSelector {
		return m
	}
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return newM.(Model)
}

type testClock struct{}

func (testClock) Now() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}

type testTokenizer struct{}

func (testTokenizer) Estimate(t string) int {
	return len(t)
}

type testSearcher struct{}

func (testSearcher) Search(ctx context.Context, q string, s core.SearchScope) ([]core.Hit, error) {
	return nil, nil
}

type testHarnessRegistry struct{}

func (testHarnessRegistry) All() []core.Harness {
	return nil
}

func (testHarnessRegistry) Get(name string) (core.Harness, error) {
	return nil, nil
}

type testStore struct {
	dossiers  map[string]*core.Dossier
	bindings  map[string]*core.SessionBinding
	conflicts map[string]*core.Conflict
	artifacts map[string][]core.Artifact
	auditLogs map[string][]core.AuditEvent
}

func newTestStore() *testStore {
	return &testStore{
		dossiers:  make(map[string]*core.Dossier),
		bindings:  make(map[string]*core.SessionBinding),
		conflicts: make(map[string]*core.Conflict),
		artifacts: make(map[string][]core.Artifact),
		auditLogs: make(map[string][]core.AuditEvent),
	}
}

func (s *testStore) Init() error { return nil }

func (s *testStore) Read(id string) (*core.Dossier, core.Revision, error) {
	d, ok := s.dossiers[id]
	if !ok {
		// Try searching by slug
		for _, dos := range s.dossiers {
			if dos.Frontmatter.Slug == id {
				return dos, "rev1", nil
			}
		}
		return nil, "", fmt.Errorf("not found")
	}
	return d, "rev1", nil
}

func (s *testStore) ReadRevision(id string, rev core.Revision) (*core.Dossier, error) {
	d, _, err := s.Read(id)
	return d, err
}

func (s *testStore) List(filter string) ([]core.Frontmatter, error) {
	var list []core.Frontmatter
	for _, d := range s.dossiers {
		list = append(list, d.Frontmatter)
	}
	return list, nil
}

func (s *testStore) Write(d *core.Dossier, base core.Revision) (core.Revision, error) {
	s.dossiers[d.Frontmatter.ID] = d
	return "rev_new", nil
}

func (s *testStore) WriteArtifact(dossierID string, a *core.Artifact) error {
	s.artifacts[dossierID] = append(s.artifacts[dossierID], *a)
	return nil
}

func (s *testStore) ReadArtifact(dossierID string, artifactID string) (*core.Artifact, error) {
	for _, a := range s.artifacts[dossierID] {
		if a.ID == artifactID {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (s *testStore) ListArtifacts(dossierID string) ([]core.Artifact, error) {
	return s.artifacts[dossierID], nil
}

func (s *testStore) AppendAudit(dossierID string, e core.AuditEvent) error {
	s.auditLogs[dossierID] = append(s.auditLogs[dossierID], e)
	return nil
}

func (s *testStore) ReadAuditLog(dossierID string) ([]core.AuditEvent, error) {
	return s.auditLogs[dossierID], nil
}

func (s *testStore) ValidateAuditShards(dossierID string) []string {
	return nil
}

func (s *testStore) ValidateArtifactFiles(dossierID string) []string {
	return nil
}

func (s *testStore) EnsureAuditDir(dossierID string) error {
	return nil
}

func (s *testStore) WriteSessionStash(dossierID, author, sessionID, content string) error {
	return nil
}

func (s *testStore) SaveSessionBinding(binding *core.SessionBinding) error {
	s.bindings[binding.SessionBindingID] = binding
	return nil
}

func (s *testStore) GetSessionBinding(sessionID string) (*core.SessionBinding, error) {
	b, ok := s.bindings[sessionID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return b, nil
}

func (s *testStore) ClearSessionBinding(sessionID string) error {
	delete(s.bindings, sessionID)
	return nil
}

func (s *testStore) WriteConflict(conflict *core.Conflict) error {
	s.conflicts[conflict.ID] = conflict
	return nil
}

func (s *testStore) ReadConflict(conflictID string) (*core.Conflict, error) {
	c, ok := s.conflicts[conflictID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return c, nil
}

func (s *testStore) ListConflicts() ([]core.Conflict, error) {
	var list []core.Conflict
	for _, c := range s.conflicts {
		list = append(list, *c)
	}
	return list, nil
}

func (s *testStore) WriteLibraryContext(data core.LibraryData) error { return nil }

func setupTestService(store core.Store) *core.Service {
	return core.NewService(
		store,
		testSearcher{},
		testTokenizer{},
		testHarnessRegistry{},
		testClock{},
		core.Config{DossierHome: "/tmp/dossier_home"},
		nil,
	)
}

func TestInterfaceFilter(t *testing.T) {
	item := core.ListItem{Interfaces: []string{"Pricing WBR", "Steerco"}}
	if !interfaceFilter("Pricing WBR").matches(item) {
		t.Fatal("expected matching interface to pass")
	}
	if interfaceFilter("1:1").matches(item) {
		t.Fatal("expected non-matching interface to be filtered")
	}
	if !interfaceFilter("").matches(item) {
		t.Fatal("expected empty interface filter to match all")
	}
}

func TestNextInterfaceFilterCyclesCanonicalOrder(t *testing.T) {
	current := interfaceFilter("")
	for _, want := range core.DefaultDiscussionInterfaces() {
		current = nextInterfaceFilter(current)
		if current != interfaceFilter(want) {
			t.Fatalf("next filter = %q, want %q", current, want)
		}
	}
	if nextInterfaceFilter(current) != "" {
		t.Fatal("expected interface filter cycle to return to All")
	}

	custom := []string{"Planning", "Customer Review"}
	if got := nextInterfaceFilter("", custom); got != "Planning" {
		t.Fatalf("custom interface cycle started at %q", got)
	}
	if got := nextInterfaceFilter("Planning", custom); got != "Customer Review" {
		t.Fatalf("custom interface cycle continued at %q", got)
	}
	if got := nextInterfaceFilter("Customer Review", custom); got != "" {
		t.Fatalf("custom interface cycle ended at %q, want All", got)
	}
	if got := nextInterfaceFilter("", []string{}); got != "" {
		t.Fatalf("empty interface list produced %q, want All", got)
	}
}

func TestTUI_Dashboard(t *testing.T) {
	store := newTestStore()
	// Seed a dossier
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Project Alpha",
			Slug:   "project-alpha",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
	}
	svc := setupTestService(store)

	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	// Trigger Init cmd
	initCmd := m.Init()
	if initCmd == nil {
		t.Fatal("expected Init cmd to not be nil")
	}

	// The TUI now opens directly on the dashboard while the list loads.
	if m.currentView != ViewDashboard {
		t.Fatalf("expected startup view to be ViewDashboard, got %v", m.currentView)
	}
	viewStr := m.View()
	if !strings.Contains(viewStr, "Loading dossiers") {
		t.Errorf("expected dashboard view to contain loading indicator, got:\n%s", viewStr)
	}

	// Perform the async load manually
	listMsg := m.listDossiersCmd()()

	// Update the model with results
	var newM tea.Model
	newM, _ = m.Update(listMsg)

	updatedModel := newM.(Model)
	if len(updatedModel.items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(updatedModel.items))
	}

	if updatedModel.currentView != ViewDashboard {
		t.Fatalf("expected loaded startup view to remain ViewDashboard, got %v", updatedModel.currentView)
	}

	// Trigger a mock window resize to initialize columns and height
	newM, _ = updatedModel.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	resizedModel := newM.(Model)

	// Verify dossier name is rendered
	viewWithItems := resizedModel.View()
	if !strings.Contains(viewWithItems, "Project Alpha") {
		t.Errorf("expected view to contain 'Project Alpha', got:\n%s", viewWithItems)
	}
}

func TestTUI_Detail(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Project Alpha",
			Slug:   "project-alpha",
			Status: core.StatusActive, Priority: core.PriorityHigh,
			Interfaces: []string{"Pricing WBR", "1:1"},
		},
		DistilledState: core.DistilledState{
			Body: "This is the distilled state of Alpha",
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	// Load list items
	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m = enterDashboard(t, m)

	// Move cursor down to select the actual item, not the separator row
	m.table.MoveDown(1)

	// Dashboard: Enter to view detail
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected enter key to return a recall command")
	}

	// Run command
	recallMsg := cmd()
	newM, _ = m.Update(recallMsg)
	m = newM.(Model)

	if m.currentView != ViewDetail {
		t.Errorf("expected view to be ViewDetail, got %v", m.currentView)
	}

	viewStr := m.View()
	cleanView := stripANSI(viewStr)
	if !strings.Contains(cleanView, "This is the distilled state of Alpha") {
		t.Errorf("expected view to contain distilled state, got:\n%s", cleanView)
	}
	if !strings.Contains(cleanView, "Interfaces: Pricing WBR, 1:1") {
		t.Errorf("expected view to contain 'Interfaces: Pricing WBR, 1:1' on one line without wrapped colon, got:\n%s", cleanView)
	}

	// Verify field sequence aligns with viewDashboard: Dossier -> Priority -> Stage -> Lead -> Due
	dossierIdx := strings.Index(cleanView, "Dossier:")
	priorityIdx := strings.Index(cleanView, "Priority:")
	stageIdx := strings.Index(cleanView, "Stage:")
	leadIdx := strings.Index(cleanView, "Lead:")
	interfacesIdx := strings.Index(cleanView, "Interfaces:")

	if dossierIdx == -1 || priorityIdx == -1 || stageIdx == -1 || leadIdx == -1 || interfacesIdx == -1 {
		t.Fatalf("expected all metadata labels in detail view, got view:\n%s", cleanView)
	}
	if !(dossierIdx < priorityIdx && priorityIdx < stageIdx && stageIdx < leadIdx && leadIdx < interfacesIdx) {
		t.Errorf("expected field order Dossier < Priority < Stage < Lead < Interfaces, got indices: Dossier=%d, Priority=%d, Stage=%d, Lead=%d, Interfaces=%d",
			dossierIdx, priorityIdx, stageIdx, leadIdx, interfacesIdx)
	}

	// Press esc to go back
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Errorf("expected view to be ViewDashboard after esc, got %v", m.currentView)
	}
}

func TestTUI_ArtifactFlowRestoresDistilledStateAndScroll(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:        "dos1",
			Name:      "Project Alpha",
			Slug:      "project-alpha",
			Status:    core.StatusActive,
			Priority:  core.PriorityHigh,
			UpdatedAt: testClock{}.Now(),
		},
		DistilledState: core.DistilledState{
			Body: "This is the distilled state of Alpha\n" + strings.Repeat("distilled detail line\n", 60),
		},
	}
	store.artifacts["dos1"] = []core.Artifact{
		{
			ID:            "art_evidence",
			DossierID:     "dos1",
			Type:          core.ArtifactTypeDecisionEvidence,
			Title:         "Lock latency benchmark",
			ContentFormat: core.ContentFormatText,
			Content:       "artifact line one\n" + strings.Repeat("artifact evidence line\n", 60),
			CapturedAt:    testClock{}.Now(),
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 24
	m.recalculateTableLayout()

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m.table.MoveDown(1)

	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(cmd())
	m = newM.(Model)
	if m.currentView != ViewDetail {
		t.Fatalf("expected view to be ViewDetail, got %v", m.currentView)
	}
	m.viewport.SetYOffset(5)
	detailOffset := m.viewport.YOffset
	if detailOffset == 0 {
		t.Fatal("test setup expected a scrollable Distilled State")
	}

	// 'a' from detail opens the artifact index without changing the detail
	// viewport or interfering with the independently-supported 'c' shortcut.
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected 'a' to return a listArtifacts command")
	}
	newM, _ = m.Update(cmd())
	m = newM.(Model)
	if m.currentView != ViewArtifactIndex {
		t.Fatalf("expected view to be ViewArtifactIndex, got %v", m.currentView)
	}
	if len(m.artifactIndex) != 1 || m.artifactIndex[0].ID != "art_evidence" {
		t.Fatalf("expected artifact index to contain art_evidence, got %+v", m.artifactIndex)
	}

	// Enter fetches the artifact into its own viewport, which scrolls without
	// moving the preserved Distilled State viewport.
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected enter to return a readArtifact command")
	}
	newM, _ = m.Update(cmd())
	m = newM.(Model)
	if m.currentView != ViewArtifactContent {
		t.Fatalf("expected view to be ViewArtifactContent, got %v", m.currentView)
	}
	if got := stripANSI(m.View()); !strings.Contains(got, "artifact line one") {
		t.Errorf("expected artifact content in view, got:\n%s", got)
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = newM.(Model)
	if m.artifactViewport.YOffset == 0 {
		t.Fatal("expected artifact content viewport to scroll")
	}
	if m.viewport.YOffset != detailOffset {
		t.Fatalf("artifact scroll changed detail offset to %d, want %d", m.viewport.YOffset, detailOffset)
	}
	newM, _ = m.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	m = newM.(Model)
	if m.viewport.YOffset != detailOffset {
		t.Fatalf("resize in artifact view changed detail offset to %d, want %d", m.viewport.YOffset, detailOffset)
	}

	// Esc steps content -> index -> detail. Neither intermediate nor final view
	// may render stale artifact viewport content.
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.currentView != ViewArtifactIndex {
		t.Fatalf("expected esc from content to return to ViewArtifactIndex, got %v", m.currentView)
	}
	if got := stripANSI(m.View()); strings.Contains(got, "artifact evidence line") {
		t.Fatalf("artifact index rendered stale artifact content:\n%s", got)
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.currentView != ViewDetail {
		t.Fatalf("expected esc from index to return to ViewDetail, got %v", m.currentView)
	}
	got := stripANSI(m.View())
	if !strings.Contains(got, "distilled detail line") || strings.Contains(got, "artifact evidence line") {
		t.Fatalf("detail did not restore the rendered Distilled State:\n%s", got)
	}
	if m.viewport.YOffset != detailOffset {
		t.Fatalf("detail offset = %d after artifact round-trip, want %d", m.viewport.YOffset, detailOffset)
	}
}

func TestArtifactIndexWindowingScrollingAndResize(t *testing.T) {
	m := NewModel(setupTestService(newTestStore()))
	m.currentView = ViewArtifactIndex
	m.recallResult.Frontmatter.Name = "Project Alpha"
	m.width = 120
	m.height = 16 // nine artifact rows after index chrome
	m.loading = false
	for i := 0; i < 40; i++ {
		m.artifactIndex = append(m.artifactIndex, core.ArtifactSummary{
			ID:    fmt.Sprintf("art_%02d", i),
			Title: fmt.Sprintf("Evidence %02d", i),
			Lines: i + 1,
		})
	}

	countRows := func(view string) int {
		n := 0
		for _, line := range strings.Split(stripANSI(view), "\n") {
			if strings.Contains(line, "art_") {
				n++
			}
		}
		return n
	}

	top := stripANSI(m.View())
	if got, want := countRows(top), m.artifactVisibleRows(); got != want {
		t.Fatalf("top rendered %d artifact rows, want %d", got, want)
	}
	if !strings.Contains(top, "art_00") || strings.Contains(top, "more above") || !strings.Contains(top, "more below") {
		t.Fatalf("unexpected top artifact window:\n%s", top)
	}

	for i := 0; i < 20; i++ {
		newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = newM.(Model)
	}
	middle := stripANSI(m.View())
	if !strings.Contains(middle, "art_20") || !strings.Contains(middle, "more above") || !strings.Contains(middle, "more below") {
		t.Fatalf("cursor was not visible in middle artifact window:\n%s", middle)
	}

	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	m = newM.(Model)
	resized := stripANSI(m.View())
	if got, want := countRows(resized), m.artifactVisibleRows(); got != want {
		t.Fatalf("resized view rendered %d artifact rows, want %d", got, want)
	}
	if !strings.Contains(resized, "art_20") {
		t.Fatalf("resize hid the cursor row:\n%s", resized)
	}
	if got := len(strings.Split(resized, "\n")); got > m.height {
		t.Fatalf("resized artifact index uses %d lines, exceeds height %d:\n%s", got, m.height, resized)
	}

	m.artifactCursor = len(m.artifactIndex) - 1
	bottom := stripANSI(m.View())
	if !strings.Contains(bottom, "art_39") || !strings.Contains(bottom, "more above") || strings.Contains(bottom, "more below") {
		t.Fatalf("unexpected bottom artifact window:\n%s", bottom)
	}
}

func TestArtifactIndexEmptyStateAndNavigation(t *testing.T) {
	m := NewModel(setupTestService(newTestStore()))
	m.currentView = ViewArtifactIndex
	m.width = 80
	m.height = 12
	m.loading = false

	for _, key := range []tea.KeyType{tea.KeyUp, tea.KeyDown, tea.KeyEnter} {
		newM, cmd := m.Update(tea.KeyMsg{Type: key})
		m = newM.(Model)
		if cmd != nil {
			t.Fatalf("empty artifact index key %v unexpectedly returned a command", key)
		}
		if m.currentView != ViewArtifactIndex {
			t.Fatalf("empty artifact index key %v changed view to %v", key, m.currentView)
		}
	}
	if got := stripANSI(m.View()); !strings.Contains(got, "No artifacts archived for this dossier") {
		t.Fatalf("missing empty artifact index state:\n%s", got)
	}
}

func TestTUI_InlineEditing(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Project Alpha",
			Slug:   "project-alpha",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	// Load list items
	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m = enterDashboard(t, m)

	// Move cursor down to select actual item
	m.table.MoveDown(1)

	// 1. Test Status Editing (press 's')
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = newM.(Model)
	if m.currentView != ViewStatusPicker {
		t.Fatalf("expected view ViewStatusPicker, got %v", m.currentView)
	}

	// Press enter to confirm selection
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected status picker enter to return setStatus command")
	}
	mutMsg := cmd()
	newM, cmd = m.Update(mutMsg)
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Errorf("expected to return to ViewDashboard after status update, got %v", m.currentView)
	}

	// 2. Test Next Action Editing (press 'n')
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = newM.(Model)
	if m.currentView != ViewNextActionEditor {
		t.Fatalf("expected view ViewNextActionEditor, got %v", m.currentView)
	}
	if m.nextActionInput.CharLimit != core.MaxNextActionLength {
		t.Fatalf("expected next action character limit %d, got %d", core.MaxNextActionLength, m.nextActionInput.CharLimit)
	}
	m.nextActionInput.SetValue("New Next Action")
	// Press enter
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected next action enter to return save command")
	}
	mutMsg = cmd()
	newM, cmd = m.Update(mutMsg)
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Errorf("expected to return to ViewDashboard, got %v", m.currentView)
	}

	// 3. Test Priority Editing (press 'p')
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = newM.(Model)
	if m.currentView != ViewPriorityEditor {
		t.Fatalf("expected view ViewPriorityEditor, got %v", m.currentView)
	}
	// Focus is initially 0 (Priority). Hitting enter on Priority selects it and immediately triggers save.
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected priority enter to trigger immediate save command")
	}
	mutMsg = cmd()
	newM, cmd = m.Update(mutMsg)
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Errorf("expected to return to ViewDashboard after priority save, got %v", m.currentView)
	}
}

// TestStartEditPropagatesBaseRevision guards against a regression where
// startEditStatus/startEditLead forgot to copy the target's base revision into
// the model, leaving a stale (or empty) revision from whatever edit ran
// previously. Against the real store that stale revision causes a spurious
// concurrency-conflict on save, so the user's status/lead change is silently
// rejected instead of applied.
func TestStartEditPropagatesBaseRevision(t *testing.T) {
	target := targetDossier{id: "dos1", name: "Project Alpha", baseRevision: "rev_abc123"}

	var m Model
	m.startEditStatus(target)
	if m.targetBaseRevision != "rev_abc123" {
		t.Errorf("startEditStatus: targetBaseRevision = %q, want %q", m.targetBaseRevision, "rev_abc123")
	}

	m = Model{}
	m.startEditLead(target)
	if m.targetBaseRevision != "rev_abc123" {
		t.Errorf("startEditLead: targetBaseRevision = %q, want %q", m.targetBaseRevision, "rev_abc123")
	}
}

// TestTUI_NoActiveBinding asserts the TUI exposes no per-session "active"
// affordance: pressing 'a' is a no-op, and the dashboard has no ★ marker. The
// per-session active binding (Switch) is intentionally not reachable from the
// TUI — see ADR 0004 and BUILD-DECISIONS B9.
func TestTUI_NoActiveBinding(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Project Alpha",
			Slug:   "project-alpha",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	// Load list items
	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m = enterDashboard(t, m)

	// The dashboard must not render an active-dossier star marker.
	viewStr := m.View()
	if strings.Contains(viewStr, "★") {
		t.Errorf("expected no active dossier star marker, got:\n%s", viewStr)
	}
}

func TestTUI_Link(t *testing.T) {
	store := newTestStore()
	// Seed two dossiers matching "Alpha"
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Alpha project",
			Slug:   "alpha-proj",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
	}
	store.dossiers["dos2"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos2",
			Name:   "Alpha team",
			Slug:   "alpha-team",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	// Load list items
	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m = enterDashboard(t, m)

	// Press 'k' key to link
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = newM.(Model)
	if m.currentView != ViewLinkInput {
		t.Fatalf("expected view ViewLinkInput, got %v", m.currentView)
	}

	m.linkTextInput.SetValue("Alpha content")
	// Press enter to link
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected enter key to return link analyze command")
	}

	// Run first link cmd which detects ambiguity
	resMsg := cmd()
	newM, cmd = m.Update(resMsg)
	m = newM.(Model)

	if m.currentView != ViewLinkSelector {
		t.Fatalf("expected view ViewLinkSelector, got %v", m.currentView)
	}
	if len(m.linkSuggestions) != 2 {
		t.Errorf("expected 2 suggestions, got %d", len(m.linkSuggestions))
	}

	// Select first suggestion and press enter
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected confirm link command")
	}

	confirmMsg := cmd()
	newM, cmd = m.Update(confirmMsg)
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Errorf("expected view to return to ViewDashboard, got %v", m.currentView)
	}
}

func TestTUI_Merge(t *testing.T) {
	store := newTestStore()
	// Seed two dossiers with incompatible statuses to force merge conflict
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Source Dossier",
			Slug:   "source-dossier",
			Status: core.StatusActive, Priority: core.PriorityHigh,
			NextAction: "Action A",
		},
		DistilledState: core.DistilledState{
			Body: "Distilled A",
		},
	}
	store.dossiers["dos2"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos2",
			Name:   "Target Dossier",
			Slug:   "target-dossier",
			Status: core.StatusBlocked, Priority: core.PriorityHigh,
			NextAction: "Action B",
		},
		DistilledState: core.DistilledState{
			Body: "Distilled B",
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	// Load list items
	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m = enterDashboard(t, m)

	// Move cursor down to select actual item
	m.table.MoveDown(1)

	// Press 'm' to merge Source Dossier
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = newM.(Model)

	if m.currentView != ViewMergeSelector {
		t.Fatalf("expected view ViewMergeSelector, got %v", m.currentView)
	}
	if len(m.mergeTargets) != 1 {
		t.Fatalf("expected 1 merge target, got %d", len(m.mergeTargets))
	}

	// Press enter to merge into Target Dossier
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected merge command")
	}

	// Run command which will fail with a conflict
	resMsg := cmd()
	newM, cmd = m.Update(resMsg)
	m = newM.(Model)

	if m.currentView != ViewMergeConflictResolver {
		t.Fatalf("expected ViewMergeConflictResolver, got %v", m.currentView)
	}
	if m.mergeConflict == nil {
		t.Fatal("expected mergeConflict details to be populated")
	}

	// Select Resolve (focus 0) and press Enter
	m.conflictResolverCursor = 0
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected resolve merge command")
	}

	resolveMsg := cmd()
	newM, cmd = m.Update(resolveMsg)
	m = newM.(Model)

	if m.currentView != ViewDashboard {
		t.Errorf("expected view to return to ViewDashboard, got %v", m.currentView)
	}
}

// TestHeaderHasNoSession asserts the TUI carries no session identity: the header
// shows only the app title, with no "Session:" or "Active:" field and no
// standalone-session warning footer. See ADR 0004.
func TestHeaderHasNoSession(t *testing.T) {
	store := newTestStore()
	svc := setupTestService(store)

	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	view := m.View()
	if !strings.Contains(view, "DOSSIER TUI") {
		t.Errorf("expected view to contain the 'DOSSIER TUI' title, got:\n%s", view)
	}
	for _, forbidden := range []string{"Session:", "Active:", "No active Claude session"} {
		if strings.Contains(view, forbidden) {
			t.Errorf("expected view NOT to contain %q, got:\n%s", forbidden, view)
		}
	}
}
func TestLeadAutocomplete(t *testing.T) {
	items := []core.ListItem{
		{ID: "1", Lead: "Ryan"},
		{ID: "2", Lead: "Riley"},
		{ID: "3", Lead: "Ryan"},
		{ID: "4", Lead: "Alice"},
	}
	got := leadAutocomplete(items, "r")
	want := []string{"Riley", "Ryan"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("leadAutocomplete = %v, want %v", got, want)
	}
	if got := leadAutocomplete(items, "Ryan"); fmt.Sprint(got) != "[Ryan]" {
		t.Fatalf("exact lead autocomplete = %v, want [Ryan]", got)
	}
	if got := leadAutocomplete(items, "r", []string{"Rosa", "Alice"}); fmt.Sprint(got) != "[Rosa]" {
		t.Fatalf("configured lead autocomplete = %v, want [Rosa]", got)
	}
}

func TestDeriveLeadOptions(t *testing.T) {
	items := []core.ListItem{
		{ID: "1", Name: "Alpha", Lead: "Bob"},
		{ID: "2", Name: "Beta", Lead: ""},
		{ID: "3", Name: "Gamma", Lead: "alice"},
		{ID: "4", Name: "Delta", Lead: "Bob"},
		{ID: "5", Name: "Epsilon", Lead: "Bob", Status: "archived"},
		{ID: "", Name: "placeholder"}, // header/placeholder row must be ignored
	}

	got := deriveLeadOptions(items)

	// All and Unassigned are pinned first; named leads follow case-insensitively
	// sorted. The archived item is excluded from every count: counts only
	// reflect live (tier-0) work, matching the dashboard's default collapsed
	// view — resolved/archived surfaces via the dashboard's own extras toggle.
	want := []leadOption{
		{filter: leadFilter{kind: filterAll}, count: 4},
		{filter: leadFilter{kind: filterUnassigned}, count: 1},
		{filter: leadFilter{kind: filterByName, name: "alice"}, count: 1},
		{filter: leadFilter{kind: filterByName, name: "Bob"}, count: 2},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDeriveLeadOptionsEmpty(t *testing.T) {
	got := deriveLeadOptions(nil)
	if len(got) != 2 {
		t.Fatalf("expected All + Unassigned even with no items, got %d", len(got))
	}
	if got[0].filter.kind != filterAll || got[0].count != 0 {
		t.Errorf("expected All with count 0, got %+v", got[0])
	}
	if got[1].filter.kind != filterUnassigned || got[1].count != 0 {
		t.Errorf("expected Unassigned with count 0, got %+v", got[1])
	}

	configured := deriveLeadOptions(nil, []string{"Alice", "Bob"})
	if len(configured) != 4 || configured[2].filter.name != "Alice" || configured[2].count != 0 || configured[3].filter.name != "Bob" {
		t.Fatalf("configured zero-count leads = %+v", configured)
	}
}

func TestFilterLeadOptions(t *testing.T) {
	opts := []leadOption{
		{filter: leadFilter{kind: filterAll}},
		{filter: leadFilter{kind: filterUnassigned}},
		{filter: leadFilter{kind: filterByName, name: "Alice"}},
		{filter: leadFilter{kind: filterByName, name: "Bob"}},
	}

	tests := []struct {
		name  string
		query string
		want  []string // expected labels in order
	}{
		{"empty returns all", "", []string{"All", "Unassigned", "Alice", "Bob"}},
		{"case-insensitive substring", "ali", []string{"Alice"}},
		{"matches pinned labels too", "una", []string{"Unassigned"}},
		{"whitespace trimmed", "  bob ", []string{"Bob"}},
		{"no match", "zzz", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterLeadOptions(opts, tc.query)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d results, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, label := range tc.want {
				if got[i].filter.label() != label {
					t.Errorf("result %d = %q, want %q", i, got[i].filter.label(), label)
				}
			}
		})
	}
}

func TestLeadFilterMatches(t *testing.T) {
	bob := core.ListItem{ID: "1", Lead: "Bob"}
	none := core.ListItem{ID: "2", Lead: ""}

	cases := []struct {
		name   string
		filter leadFilter
		item   core.ListItem
		want   bool
	}{
		{"all matches assigned", leadFilter{kind: filterAll}, bob, true},
		{"all matches unassigned", leadFilter{kind: filterAll}, none, true},
		{"unassigned matches empty lead", leadFilter{kind: filterUnassigned}, none, true},
		{"unassigned rejects assigned", leadFilter{kind: filterUnassigned}, bob, false},
		{"byName matches exact", leadFilter{kind: filterByName, name: "Bob"}, bob, true},
		{"byName rejects other", leadFilter{kind: filterByName, name: "Bob"}, none, false},
	}

	for _, tc := range cases {
		if got := tc.filter.matches(tc.item); got != tc.want {
			t.Errorf("%s: matches = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestChooseLeadFiltersDashboard verifies the landing selection narrows the
// visible item set the dashboard's cursor lookups index into.
func TestChooseLeadFiltersDashboard(t *testing.T) {
	store := newTestStore()
	svc := setupTestService(store)
	m := NewModel(svc)

	m.setItems([]core.ListItem{
		{ID: "1", Name: "Alpha", Lead: "Bob"},
		{ID: "2", Name: "Beta", Lead: "Alice"},
		{ID: "3", Name: "Gamma", Lead: "Bob"},
	})
	m.leadOptions = deriveLeadOptions(m.items)
	m.leadResults = m.leadOptions

	// Select "Bob" (index 3: All, Unassigned, Alice, Bob).
	m.leadCursor = 3
	m.chooseLead()

	if m.currentView != ViewDashboard {
		t.Fatalf("expected dashboard after choosing lead, got view %d", m.currentView)
	}
	if got := len(m.visibleItems); got != 2 {
		t.Fatalf("expected 2 visible dossiers for Bob, got %d", got)
	}
	for _, item := range m.visibleItems {
		if item.Lead != "Bob" {
			t.Errorf("visible item %q has lead %q, want Bob", item.Name, item.Lead)
		}
	}
}

// TestEscFromDashboardIsNoOp verifies that dashboard filters are opened only
// through their explicit shortcuts rather than through Esc.
func TestEscFromDashboardIsNoOp(t *testing.T) {
	store := newTestStore()
	svc := setupTestService(store)
	m := NewModel(svc)

	m.setItems([]core.ListItem{
		{ID: "1", Name: "Alpha", Lead: "Bob"},
		{ID: "2", Name: "Beta", Lead: "Alice"},
		{ID: "3", Name: "Gamma", Lead: "Bob"},
	})
	m.leadOptions = deriveLeadOptions(m.items)
	m.leadResults = m.leadOptions

	// Select "Bob" (index 3: All, Unassigned, Alice, Bob) to land on the dashboard.
	m.leadCursor = 3
	m.chooseLead()
	if m.currentView != ViewDashboard {
		t.Fatalf("expected dashboard after choosing lead, got view %d", m.currentView)
	}

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)

	if m.currentView != ViewDashboard {
		t.Fatalf("expected esc to leave the dashboard open, got view %d", m.currentView)
	}
	if got := stripANSI(m.View()); strings.Contains(got, "esc: leads") {
		t.Errorf("dashboard footer should not advertise an Esc binding, got:\n%s", got)
	}
}

// TestStatusTierSort guards against the regression where fetching all statuses
// for lead filtering let a high-priority archived dossier sort above active work.
func TestStatusTierSort(t *testing.T) {
	store := newTestStore()
	store.dossiers["arch"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "arch",
			Name:   "Archived Important",
			Slug:   "arch",
			Status: core.StatusArchived, Priority: core.PriorityHigh,
		},
	}
	store.dossiers["act"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "act",
			Name:   "Active Minor",
			Slug:   "act",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)

	if len(m.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.items))
	}
	if m.items[0].ID != "act" {
		t.Errorf("expected active dossier first despite lower priority, got %q first", m.items[0].ID)
	}
}

// TestArchivedHiddenByDefault verifies resolved/archived dossiers stay out of
// the dashboard's visible items until the user expands them via the trailing
// "Show More..." row, which then flips to "Hide Extras...".
func TestArchivedHiddenByDefault(t *testing.T) {
	store := newTestStore()
	store.dossiers["arch"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "arch",
			Name:   "Old Project",
			Slug:   "arch",
			Status: core.StatusArchived, Priority: core.PriorityHigh,
		},
	}
	store.dossiers["res"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "res",
			Name:   "Done Project",
			Slug:   "res",
			Status: core.StatusResolved, Priority: core.PriorityHigh,
		},
	}
	store.dossiers["act"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "act",
			Name:   "Live Project",
			Slug:   "act",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m = enterDashboard(t, m)

	if m.extrasExpanded {
		t.Fatal("expected extrasExpanded to default to false")
	}
	if len(m.visibleItems) != 1 || m.visibleItems[0].ID != "act" {
		t.Fatalf("expected only the active dossier visible by default, got %+v", m.visibleItems)
	}
	if m.extrasCount != 2 {
		t.Fatalf("expected 2 extras (archived + resolved), got %d", m.extrasCount)
	}
	if got := stripANSI(m.View()); !strings.Contains(got, "Show More...") {
		t.Fatalf("expected rendered dashboard to contain the collapsed toggle row 'Show More...', got:\n%s", got)
	}

	// The toggle row sits right after the live items (row index liveCount);
	// select it and press enter to expand.
	m.table.SetCursor(m.liveCount)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.currentView != ViewDashboard {
		t.Fatalf("expected toggle to stay on ViewDashboard, got %v", m.currentView)
	}
	if !m.extrasExpanded {
		t.Fatal("expected extrasExpanded to flip to true")
	}
	if len(m.visibleItems) != 3 {
		t.Fatalf("expected all 3 dossiers visible after toggling, got %+v", m.visibleItems)
	}

	view := stripANSI(m.View())
	if !strings.Contains(view, "Hide Extras...") {
		t.Fatalf("expected rendered dashboard to contain the expanded toggle row 'Hide Extras...', got:\n%s", view)
	}
	// The toggle row must sit above the extras, not below them.
	toggleIdx := strings.Index(view, "Hide Extras...")
	archivedIdx := strings.Index(view, "Old Project")
	resolvedIdx := strings.Index(view, "Done Project")
	if archivedIdx == -1 || resolvedIdx == -1 {
		t.Fatalf("expected both extra dossiers rendered, got:\n%s", view)
	}
	if toggleIdx > archivedIdx || toggleIdx > resolvedIdx {
		t.Fatalf("expected 'Hide Extras...' to render above the extra rows, got:\n%s", view)
	}

	// Toggling back collapses the extras again.
	m.table.SetCursor(m.liveCount)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.extrasExpanded {
		t.Fatal("expected extrasExpanded to flip back to false")
	}
	if len(m.visibleItems) != 1 || m.visibleItems[0].ID != "act" {
		t.Fatalf("expected only the active dossier visible after collapsing, got %+v", m.visibleItems)
	}
}

// TestArchivedToggleWithNoLiveItems covers the liveCount == 0 edge case: a lead
// scope with only resolved/archived dossiers. The toggle row must land at row
// 0 (there are no live rows above it) and still expand/collapse cleanly.
func TestArchivedToggleWithNoLiveItems(t *testing.T) {
	store := newTestStore()
	store.dossiers["arch"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "arch",
			Name:   "Old Project",
			Slug:   "arch",
			Status: core.StatusArchived, Priority: core.PriorityHigh,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m = enterDashboard(t, m)

	if m.liveCount != 0 {
		t.Fatalf("expected liveCount 0, got %d", m.liveCount)
	}
	if len(m.visibleItems) != 0 {
		t.Fatalf("expected no visible items while collapsed, got %+v", m.visibleItems)
	}
	if m.extrasCount != 1 {
		t.Fatalf("expected 1 extra, got %d", m.extrasCount)
	}
	if got := stripANSI(m.View()); !strings.Contains(got, "Show More...") {
		t.Fatalf("expected rendered dashboard to contain 'Show More...', got:\n%s", got)
	}

	// Toggle row is at row 0 since there are no live items above it.
	m.table.SetCursor(0)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if !m.extrasExpanded {
		t.Fatal("expected extrasExpanded to flip to true")
	}
	if len(m.visibleItems) != 1 || m.visibleItems[0].ID != "arch" {
		t.Fatalf("expected the archived dossier visible after expanding, got %+v", m.visibleItems)
	}
	if got := stripANSI(m.View()); !strings.Contains(got, "Hide Extras...") {
		t.Fatalf("expected rendered dashboard to contain 'Hide Extras...', got:\n%s", got)
	}
}

// TestEnterRecallsFilteredDossier exercises the exact desync the visibleItems
// refactor exists to prevent: with a lead filter active, pressing enter on the
// first visible row must recall that dossier, not the same index of the full list.
func TestEnterRecallsFilteredDossier(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Bob Item",
			Slug:   "bob-item",
			Status: core.StatusActive, Priority: core.PriorityHigh,
			Lead: "Bob",
		},
	}
	store.dossiers["dos2"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos2",
			Name:   "Alice Item",
			Slug:   "alice-item",
			Status: core.StatusActive, Priority: core.PriorityHigh,
			Lead: "Alice",
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)

	// Open the lead selector, search for "Bob", and select the lead.
	m.openLeadSelector()
	for _, r := range "Bob" {
		newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = newM.(Model)
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.currentView != ViewDashboard {
		t.Fatalf("expected dashboard after selecting lead, got view %d", m.currentView)
	}
	if len(m.visibleItems) != 1 || m.visibleItems[0].ID != "dos1" {
		t.Fatalf("expected only Bob's dossier visible, got %+v", m.visibleItems)
	}

	// Enter on row 0 must recall Bob's dossier.
	m.table.SetCursor(0)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected enter to return a recall command")
	}
	newM, _ = m.Update(cmd())
	m = newM.(Model)
	if m.recallResult.Frontmatter.ID != "dos1" {
		t.Errorf("filtered enter recalled %q, want dos1 (Bob)", m.recallResult.Frontmatter.ID)
	}
}

// TestLeadSelectorWindowing verifies the lead selector scrolls a long
// option list: only a height-bounded window renders, the cursor row is always
// visible, and "more above/below" indicators appear when content is clipped.
func TestLeadSelectorWindowing(t *testing.T) {
	store := newTestStore()
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 20 // leadVisibleRows = 20 - 14 = 6
	m.loading = false
	m.setItems([]core.ListItem{{ID: "x", Lead: "anchor"}}) // non-empty so loading branch is skipped

	for i := 0; i < 50; i++ {
		m.leadResults = append(m.leadResults, leadOption{
			filter: leadFilter{kind: filterByName, name: fmt.Sprintf("Lead%02d", i)},
			count:  1,
		})
	}

	countOptionLines := func(view string) int {
		n := 0
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "1 dossier") {
				n++
			}
		}
		return n
	}

	// Cursor at top: window shows the head, only a "below" indicator.
	m.leadCursor = 0
	top := stripANSI(m.renderLeadSelector())
	if got := countOptionLines(top); got > 6 {
		t.Errorf("expected at most 6 visible option rows, got %d", got)
	}
	if !strings.Contains(top, "Lead00") {
		t.Errorf("cursor row Lead00 not visible:\n%s", top)
	}
	if strings.Contains(top, "more above") {
		t.Errorf("did not expect an 'up' indicator at the top:\n%s", top)
	}
	if !strings.Contains(top, "more below") {
		t.Errorf("expected a 'down' indicator at the top:\n%s", top)
	}

	// Cursor at bottom: window shows the tail, only an "above" indicator.
	m.leadCursor = 49
	bottom := stripANSI(m.renderLeadSelector())
	if !strings.Contains(bottom, "Lead49") {
		t.Errorf("cursor row Lead49 not visible:\n%s", bottom)
	}
	if !strings.Contains(bottom, "more above") {
		t.Errorf("expected an 'up' indicator at the bottom:\n%s", bottom)
	}
	if strings.Contains(bottom, "more below") {
		t.Errorf("did not expect a 'down' indicator at the bottom:\n%s", bottom)
	}

	// Cursor in the middle keeps its own row visible.
	m.leadCursor = 25
	mid := stripANSI(m.renderLeadSelector())
	if !strings.Contains(mid, "Lead25") {
		t.Errorf("cursor row Lead25 not visible:\n%s", mid)
	}
}

// --- "open in Claude" handoff (ADR 0006) ---------------------------------

// claudeSpy replaces the Model's launch seams so pressing 'c' resolves a fake
// binary and captures the command instead of ever spawning a real process.
type claudeSpy struct {
	bin     string
	binErr  error
	calls   int
	lastCmd *exec.Cmd
}

func (s *claudeSpy) install(m Model) Model {
	m.claudeBin = func() (string, error) {
		if s.binErr != nil {
			return "", s.binErr
		}
		return s.bin, nil
	}
	m.execProcess = func(cmd *exec.Cmd, fn tea.ExecCallback) tea.Cmd {
		s.calls++
		s.lastCmd = cmd
		return func() tea.Msg { return fn(nil) }
	}
	return m
}

// claudeTestModel seeds one dossier and returns a dashboard model focused on it.
func claudeTestModel(t *testing.T, store *testStore) Model {
	t.Helper()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:       "dos1",
			Name:     "Project Alpha",
			Slug:     "project-alpha",
			Status:   core.StatusActive,
			Priority: core.PriorityHigh,
		},
		DistilledState: core.DistilledState{Body: "Distilled state of Alpha"},
	}
	m := NewModel(setupTestService(store))
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	newM, _ := m.Update(m.listDossiersCmd()())
	m = newM.(Model)
	m = enterDashboard(t, m)
	m.table.MoveDown(1)
	return m
}

// assertHandoff checks the captured command is a bound Claude Code launch for
// dos1 and that exactly one matching session binding was written.
func assertHandoff(t *testing.T, store *testStore, spy *claudeSpy) {
	t.Helper()
	if spy.calls != 1 {
		t.Fatalf("expected exactly one exec, got %d", spy.calls)
	}
	cmd := spy.lastCmd
	if cmd.Path != spy.bin && cmd.Args[0] != spy.bin {
		t.Errorf("expected launch of %q, got %v", spy.bin, cmd.Args)
	}
	if cmd.Dir != "/tmp/dossier_home/project-alpha" {
		t.Errorf("cmd.Dir = %q, want the dossier directory", cmd.Dir)
	}

	args := cmd.Args[1:]
	if len(args) != 3 || args[0] != "--session-id" {
		t.Fatalf("expected --session-id <uuid> <prompt>, got %v", args)
	}
	sessionID := args[1]
	if !strings.Contains(args[2], "project-alpha") {
		t.Errorf("prompt should name the dossier slug, got %q", args[2])
	}

	if len(store.bindings) != 1 {
		t.Fatalf("expected exactly one session binding, got %d", len(store.bindings))
	}
	b, ok := store.bindings[sessionID]
	if !ok {
		t.Fatalf("no binding written for the launched session id %q (have %v)", sessionID, store.bindings)
	}
	if b.DossierID != "dos1" {
		t.Errorf("binding DossierID = %q, want dos1", b.DossierID)
	}
}

func TestOpenInClaudeFromDashboard(t *testing.T) {
	store := newTestStore()
	m := claudeTestModel(t, store)
	spy := &claudeSpy{bin: "/usr/bin/claude"}
	m = spy.install(m)

	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected 'c' to return an exec command")
	}
	if m.err != nil {
		t.Fatalf("unexpected error: %v", m.err)
	}
	assertHandoff(t, store, spy)
}

func TestOpenInClaudeFromDetail(t *testing.T) {
	store := newTestStore()
	m := claudeTestModel(t, store)

	// Enter the detail view first.
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected enter to return a recall command")
	}
	newM, _ = m.Update(cmd())
	m = newM.(Model)
	if m.currentView != ViewDetail {
		t.Fatalf("expected ViewDetail, got %v", m.currentView)
	}

	spy := &claudeSpy{bin: "/usr/bin/claude"}
	m = spy.install(m)

	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected 'c' to return an exec command from the detail view")
	}
	if m.err != nil {
		t.Fatalf("unexpected error: %v", m.err)
	}
	assertHandoff(t, store, spy)

	// The callback must route the refresh back to the detail view.
	msg, ok := cmd().(claudeFinishedMsg)
	if !ok {
		t.Fatalf("expected claudeFinishedMsg, got %T", cmd())
	}
	if msg.fromView != ViewDetail || msg.id != "dos1" {
		t.Errorf("claudeFinishedMsg = %+v, want dos1 from ViewDetail", msg)
	}
}

// A missing claude binary must surface an error and, critically, must not leave
// a binding behind for a session that never starts.
func TestOpenInClaudeMissingBinary(t *testing.T) {
	store := newTestStore()
	m := claudeTestModel(t, store)
	spy := &claudeSpy{binErr: fmt.Errorf("claude was not found on PATH")}
	m = spy.install(m)

	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = newM.(Model)

	if cmd != nil {
		t.Error("expected no command when the binary is missing")
	}
	if m.err == nil {
		t.Fatal("expected the missing binary to be surfaced as an error")
	}
	if spy.calls != 0 {
		t.Errorf("expected no exec, got %d", spy.calls)
	}
	if len(store.bindings) != 0 {
		t.Errorf("expected no orphan binding, got %v", store.bindings)
	}
}

func TestClaudeFinishedRefreshes(t *testing.T) {
	store := newTestStore()
	m := claudeTestModel(t, store)

	newM, cmd := m.Update(claudeFinishedMsg{id: "dos1", fromView: ViewDashboard})
	m = newM.(Model)
	if cmd == nil {
		t.Fatal("expected a refresh command after returning from Claude")
	}
	if m.err != nil {
		t.Fatalf("unexpected error: %v", m.err)
	}

	// An exec failure is surfaced, not swallowed.
	newM, cmd = m.Update(claudeFinishedMsg{err: fmt.Errorf("exec failed"), id: "dos1"})
	m = newM.(Model)
	if cmd != nil {
		t.Error("expected no refresh command when the launch failed")
	}
	if m.err == nil {
		t.Error("expected the launch failure to be surfaced")
	}
}

func TestArtifactAndClaudeKeysCoexistWithDashboardNavigation(t *testing.T) {
	store := newTestStore()
	m := claudeTestModel(t, store)
	m.width = 140 // Keep the key label on one line as other footer actions grow.

	if got := stripANSI(m.View()); !strings.Contains(strings.Join(strings.Fields(got), " "), "c: claude") {
		t.Errorf("dashboard footer should advertise 'c: claude', got:\n%s", got)
	}
	// 'a' remains detail-only; it must not resurrect the removed dashboard
	// active-session behavior or disturb dashboard-first/Esc semantics.
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = newM.(Model)
	if cmd != nil || m.currentView != ViewDashboard || len(store.bindings) != 0 {
		t.Fatalf("dashboard 'a' changed state: view=%v cmd=%v bindings=%d", m.currentView, cmd != nil, len(store.bindings))
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Fatalf("dashboard Esc changed view to %v", m.currentView)
	}

	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(cmd())
	m = newM.(Model)
	got := stripANSI(m.View())
	if !strings.Contains(got, "a: artifacts") || !strings.Contains(got, "c: claude") {
		t.Errorf("detail footer should advertise both artifact and Claude keys, got:\n%s", got)
	}
}

// 'c' is a top-level dashboard/detail key only — text inputs must still receive
// it as an ordinary character.
func TestClaudeKeyDoesNotHijackTextInput(t *testing.T) {
	store := newTestStore()
	m := claudeTestModel(t, store)
	spy := &claudeSpy{bin: "/usr/bin/claude"}
	m = spy.install(m)

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = newM.(Model)
	if m.currentView != ViewLeadEditor {
		t.Fatalf("expected ViewLeadEditor, got %v", m.currentView)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = newM.(Model)
	if got := m.leadInput.Value(); got != "c" {
		t.Errorf("lead input = %q, want the typed character %q", got, "c")
	}
	if spy.calls != 0 {
		t.Errorf("typing in an editor must not launch claude, got %d execs", spy.calls)
	}
}

func TestTUIStatusOptionsAndTiers(t *testing.T) {
	m := NewModel(setupTestService(newTestStore()))

	wantStatuses := []core.Status{
		core.StatusSpark,
		core.StatusDefine,
		core.StatusDelegated,
		core.StatusReview,
		core.StatusBlocked,
		core.StatusDone,
	}

	if len(m.statusOptions) != len(wantStatuses) {
		t.Fatalf("expected %d status options, got %d", len(wantStatuses), len(m.statusOptions))
	}
	for i, st := range wantStatuses {
		if m.statusOptions[i] != st {
			t.Errorf("statusOptions[%d] = %q, want %q", i, m.statusOptions[i], st)
		}
	}

	// Verify statusTier ordering
	for _, openStatus := range []string{"spark", "define", "delegated", "review", "blocked", "active", "waiting"} {
		if tier := statusTier(openStatus); tier != 0 {
			t.Errorf("statusTier(%q) = %d, want 0", openStatus, tier)
		}
	}
	for _, terminalStatus := range []string{"done", "archived", "resolved"} {
		if tier := statusTier(terminalStatus); tier != 1 {
			t.Errorf("statusTier(%q) = %d, want 1", terminalStatus, tier)
		}
	}

	// Verify renderStatusPicker produces valid view
	m.startEditStatus(targetDossier{id: "dos_test", name: "Status Test"})
	pickerView := m.renderStatusPicker()
	for _, st := range wantStatuses {
		if !strings.Contains(pickerView, string(st)) {
			t.Errorf("renderStatusPicker() missing status %q", st)
		}
	}
}

func TestTUI_DashboardWidthDynamicAdaptation(t *testing.T) {
	longName := "2026-q3-pricing-restructure-and-packaging-strategy"
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:       "dos1",
			Name:     longName,
			Slug:     "pricing-restructure",
			Status:   core.StatusActive,
			Priority: core.PriorityHigh,
			Lead:     "Alice Smith",
			DueDate:  "2026-10-15",
		},
	}
	store.dossiers["dos2"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:       "dos2",
			Name:     "Short Name",
			Slug:     "short-name",
			Status:   core.StatusDefine,
			Priority: core.PriorityLow,
		},
	}
	store.dossiers["dos3"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:       "dos3",
			Name:     "Third Item",
			Slug:     "third-item",
			Status:   core.StatusReview,
			Priority: core.PriorityMedium,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)

	// Load items into model
	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)

	// Test widths from narrow to very wide
	testWidths := []int{50, 60, 80, 100, 120, 160, 200}
	for _, w := range testWidths {
		newM, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		m = newM.(Model)

		cols := m.table.Columns()
		if len(cols) > 0 && cols[0].Title != "Dossier" {
			t.Errorf("expected first column title to be 'Dossier', got %q", cols[0].Title)
		}
		totalColWidth := 0
		for _, col := range cols {
			// Each column has 2 chars padding (1 left, 1 right)
			totalColWidth += col.Width + 2
		}

		if totalColWidth != w {
			t.Errorf("width %d: expected table columns to total width %d, got %d (columns: %+v)", w, w, totalColWidth, cols)
		}
		if m.table.Width() != w {
			t.Errorf("width %d: expected m.table.Width() == %d, got %d", w, w, m.table.Width())
		}

		// At wide widths (>= 120), Name column must expand past 40 to fill available width
		if w >= 120 {
			nameCol := cols[0]
			if nameCol.Width <= 40 {
				t.Errorf("width %d: Name column width %d is unexpectedly capped at <= 40", w, nameCol.Width)
			}
			// And the long name should not be truncated in the rendered view
			view := m.View()
			if !strings.Contains(view, longName) {
				t.Errorf("width %d: expected long name %q to not be truncated in view, got:\n%s", w, longName, view)
			}
		}
	}

	// Test cursor preservation across resize
	m.table.SetCursor(2)
	if m.table.Cursor() != 2 {
		t.Fatalf("failed to set cursor to 2, got %d", m.table.Cursor())
	}
	newM, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 50})
	m = newM.(Model)
	if m.table.Cursor() != 2 {
		t.Errorf("expected cursor to be preserved at 2 across window resize, got %d", m.table.Cursor())
	}
}

func TestTUI_FooterSequenceConsistency(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Test Item",
			Slug:   "test-item",
			Status: core.StatusActive, Priority: core.PriorityHigh,
		},
		DistilledState: core.DistilledState{
			Body: "Distilled state content",
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 120
	m.height = 40
	m.recalculateTableLayout()

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)

	// Dashboard view footer
	dashView := stripANSI(m.View())

	// Shared keys that appear in both footers
	sharedKeys := []string{"s: stage", "l: lead", "p: priority", "n: next action", "c: claude"}

	// Verify shared keys appear in order in dashboard footer
	lastIdx := -1
	for _, key := range sharedKeys {
		idx := strings.Index(dashView, key)
		if idx == -1 {
			t.Fatalf("dashboard footer missing key %q", key)
		}
		if idx < lastIdx {
			t.Errorf("dashboard footer key %q appeared out of order", key)
		}
		lastIdx = idx
	}

	// Open Detail view
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd != nil {
		newM, _ = m.Update(cmd())
		m = newM.(Model)
	}

	detailView := stripANSI(m.View())

	// Verify shared keys appear in the same order in detail footer
	lastIdx = -1
	for _, key := range sharedKeys {
		idx := strings.Index(detailView, key)
		if idx == -1 {
			t.Fatalf("detail footer missing key %q", key)
		}
		if idx < lastIdx {
			t.Errorf("detail footer key %q appeared out of order", key)
		}
		lastIdx = idx
	}

	// Also verify navigation is before edits, and esc is at the end of detail
	scrollIdx := strings.Index(detailView, "scroll")
	statusIdx := strings.Index(detailView, "s: stage")
	claudeIdx := strings.Index(detailView, "c: claude")
	escIdx := strings.Index(detailView, "esc: back")
	if scrollIdx > statusIdx {
		t.Errorf("expected navigation (scroll) before stage edit in detail footer")
	}
	if claudeIdx > escIdx {
		t.Errorf("expected claude before esc in detail footer")
	}
}

func TestTUI_FooterConvergenceAtTerminalBottom(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:          "dos1",
			Name:        "Project Alpha",
			Slug:        "project-alpha",
			Description: "Summary of Project Alpha",
			Status:      core.StatusActive,
			Priority:    core.PriorityHigh,
			Interfaces:  []string{"Pricing WBR", "1:1"},
		},
		DistilledState: core.DistilledState{
			Body: "This is the distilled state of Alpha",
		},
	}
	store.dossiers["dos2"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:       "dos2",
			Name:     "Project Beta (No Description)",
			Slug:     "project-beta",
			Status:   core.StatusReview,
			Priority: core.PriorityMedium,
		},
		DistilledState: core.DistilledState{
			Body: "This is Beta without summary",
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)

	testDimensions := []struct {
		w, h int
	}{
		{w: 80, h: 25},
		{w: 90, h: 30},
		{w: 100, h: 40},
		{w: 120, h: 40},
		{w: 140, h: 50},
	}

	for _, dim := range testDimensions {
		newM, _ = m.Update(tea.WindowSizeMsg{Width: dim.w, Height: dim.h})
		m = newM.(Model)

		// 1. Dashboard view: footer must be on bottom line (total lines == dim.h)
		dashView := m.View()
		dashLines := strings.Split(dashView, "\n")
		if len(dashLines) != dim.h {
			t.Errorf("dim %dx%d: expected Dashboard line count %d, got %d", dim.w, dim.h, dim.h, len(dashLines))
		}
		lastDashLine := dashLines[len(dashLines)-1]
		// The help string wraps at narrower widths, so the bottom line is
		// whichever footer fragment lands last: "…c: claude" or "…b: board".
		if !strings.Contains(lastDashLine, "claude") && !strings.Contains(lastDashLine, "board") {
			t.Errorf("dim %dx%d: expected Dashboard bottom line to be footer with 'claude' or 'board', got: %q", dim.w, dim.h, lastDashLine)
		}

		// 2. Detail view with description: footer must be on bottom line
		recallCmd := m.recallDossierCmd("dos1")
		newM, _ = m.Update(recallCmd())
		detailM := newM.(Model)

		detailView1 := detailM.View()
		detailLines1 := strings.Split(detailView1, "\n")
		if len(detailLines1) != dim.h {
			t.Errorf("dim %dx%d with summary: expected Detail line count %d, got %d", dim.w, dim.h, dim.h, len(detailLines1))
		}
		lastDetailLine1 := detailLines1[len(detailLines1)-1]
		if !strings.Contains(lastDetailLine1, "back") && !strings.Contains(lastDetailLine1, "claude") {
			t.Errorf("dim %dx%d with summary: expected Detail bottom line to be footer, got: %q", dim.w, dim.h, lastDetailLine1)
		}

		// 3. Detail view without description: footer must also be on bottom line
		recallCmd2 := m.recallDossierCmd("dos2")
		newM, _ = m.Update(recallCmd2())
		detailM2 := newM.(Model)

		detailView2 := detailM2.View()
		detailLines2 := strings.Split(detailView2, "\n")
		if len(detailLines2) != dim.h {
			t.Errorf("dim %dx%d without summary: expected Detail line count %d, got %d", dim.w, dim.h, dim.h, len(detailLines2))
		}
		lastDetailLine2 := detailLines2[len(detailLines2)-1]
		if !strings.Contains(lastDetailLine2, "back") && !strings.Contains(lastDetailLine2, "claude") {
			t.Errorf("dim %dx%d without summary: expected Detail bottom line to be footer, got: %q", dim.w, dim.h, lastDetailLine2)
		}

		// Return to dashboard
		newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = newM.(Model)
	}
}

func TestTUI_TableHeaderColorMatchesDetailLabels(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:     "dos1",
			Name:   "Test Item",
			Status: core.StatusActive,
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)
	m.recalculateTableLayout()

	viewStr := m.View()
	if !strings.Contains(viewStr, "Durable episodic memory and delegation layer") {
		t.Errorf("expected view to contain new subheadline, got:\n%s", viewStr)
	}

	// Check that color 99 (purple) is used for rendering the table header row
	// lipgloss emits \x1b[...38;5;99...m for Foreground(purple)
	lines := strings.Split(viewStr, "\n")
	foundHeaderWithPurple := false
	for _, l := range lines {
		if strings.Contains(stripANSI(l), "Dossier") && strings.Contains(stripANSI(l), "Stage") {
			if strings.Contains(l, "38;5;99") {
				foundHeaderWithPurple = true
				break
			}
		}
	}
	if !foundHeaderWithPurple {
		t.Errorf("expected table header row with 'Dossier' column to be styled with purple foreground (color 99), view:\n%s", viewStr)
	}
}

func TestTUI_TableColumnSequence(t *testing.T) {
	svc := setupTestService(newTestStore())
	m := NewModel(svc)
	m.width = 120
	m.height = 40
	m.recalculateTableLayout()

	cols := m.table.Columns()
	expectedTitles := []string{"Dossier", "Priority", "Stage", "Lead", "Due"}
	if len(cols) != len(expectedTitles) {
		t.Fatalf("expected %d columns, got %d", len(expectedTitles), len(cols))
	}
	for i, expected := range expectedTitles {
		if cols[i].Title != expected {
			t.Errorf("column %d: expected %q, got %q", i, expected, cols[i].Title)
		}
	}

	// Verify row cell mapping aligns with the column sequence
	row := itemTableRow(core.ListItem{
		ID:       "dos1",
		Name:     "Alpha",
		Priority: "high",
		Status:   "active",
		Lead:     "Alice",
		DueDate:  "2026-12-01",
	}, true, true)

	expectedCells := []string{"Alpha", "high", "active", "Alice", "12/01"}
	if len(row) != len(expectedCells) {
		t.Fatalf("expected row length %d, got %d", len(expectedCells), len(row))
	}
	for i, expected := range expectedCells {
		if row[i] != expected {
			t.Errorf("cell %d: expected %q, got %q", i, expected, row[i])
		}
	}
}

func TestTUI_DetailFieldSequenceWithDueDate(t *testing.T) {
	store := newTestStore()
	store.dossiers["dos1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:         "dos1",
			Name:       "Project Beta",
			Slug:       "project-beta",
			Status:     core.StatusActive,
			Priority:   core.PriorityHigh,
			Lead:       "Bob",
			DueDate:    "2026-11-20",
			Interfaces: []string{"Architecture Review"},
		},
		DistilledState: core.DistilledState{
			Body: "Distilled state for Beta",
		},
	}
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 40
	m.recalculateTableLayout()

	listMsg := m.listDossiersCmd()()
	newM, _ := m.Update(listMsg)
	m = newM.(Model)

	recallCmd := m.recallDossierCmd("dos1")
	newM, _ = m.Update(recallCmd())
	detailM := newM.(Model)

	cleanView := stripANSI(detailM.View())

	dossierIdx := strings.Index(cleanView, "Dossier:")
	priorityIdx := strings.Index(cleanView, "Priority:")
	stageIdx := strings.Index(cleanView, "Stage:")
	leadIdx := strings.Index(cleanView, "Lead:")
	dueIdx := strings.Index(cleanView, "Due:")
	interfacesIdx := strings.Index(cleanView, "Interfaces:")

	if dossierIdx == -1 || priorityIdx == -1 || stageIdx == -1 || leadIdx == -1 || dueIdx == -1 || interfacesIdx == -1 {
		t.Fatalf("expected all labels including Due: in detail view, got:\n%s", cleanView)
	}

	if !(dossierIdx < priorityIdx && priorityIdx < stageIdx && stageIdx < leadIdx && leadIdx < dueIdx && dueIdx < interfacesIdx) {
		t.Errorf("expected sequence Dossier < Priority < Stage < Lead < Due < Interfaces, got: %d, %d, %d, %d, %d, %d",
			dossierIdx, priorityIdx, stageIdx, leadIdx, dueIdx, interfacesIdx)
	}
}
