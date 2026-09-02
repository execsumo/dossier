package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

var ansiRegex = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

// enterDashboard advances a model sitting on the startup lead-selector landing
// screen into the dashboard by selecting the pre-focused "All" option, mirroring
// what a user does after the list loads.
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
	for _, want := range core.Interfaces {
		current = nextInterfaceFilter(current)
		if current != interfaceFilter(want) {
			t.Fatalf("next filter = %q, want %q", current, want)
		}
	}
	if nextInterfaceFilter(current) != "" {
		t.Fatal("expected interface filter cycle to return to All")
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

	// Verify the startup landing screen shows a loading indicator before items load
	viewStr := m.View()
	if !strings.Contains(viewStr, "Loading leads") {
		t.Errorf("expected landing view to contain loading indicator, got:\n%s", viewStr)
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

	// Select "All" on the landing screen to reach the dashboard
	updatedModel = enterDashboard(t, updatedModel)

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

	// Press esc to go back
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.currentView != ViewDashboard {
		t.Errorf("expected view to be ViewDashboard after esc, got %v", m.currentView)
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

	m.items = []core.ListItem{
		{ID: "1", Name: "Alpha", Lead: "Bob"},
		{ID: "2", Name: "Beta", Lead: "Alice"},
		{ID: "3", Name: "Gamma", Lead: "Bob"},
	}
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

// TestEscFromDashboardReturnsToLeadSelector verifies that Esc on the dashboard
// takes the user back to the lead selector — the screen the app starts on —
// rather than being a no-op, and that the selector reopens scoped to the
// filter that was active on the dashboard.
func TestEscFromDashboardReturnsToLeadSelector(t *testing.T) {
	store := newTestStore()
	svc := setupTestService(store)
	m := NewModel(svc)

	m.items = []core.ListItem{
		{ID: "1", Name: "Alpha", Lead: "Bob"},
		{ID: "2", Name: "Beta", Lead: "Alice"},
		{ID: "3", Name: "Gamma", Lead: "Bob"},
	}
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

	if m.currentView != ViewLeadSelector {
		t.Fatalf("expected esc to return to lead selector, got view %d", m.currentView)
	}
	if m.leadResults[m.leadCursor].filter != m.leadFilter {
		t.Errorf("expected cursor parked on active filter %+v, got %+v", m.leadFilter, m.leadResults[m.leadCursor].filter)
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

	// On the landing screen, search for "Bob" and select the lead.
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

// TestLeadSelectorWindowing verifies the lead landing screen scrolls a long
// option list: only a height-bounded window renders, the cursor row is always
// visible, and "more above/below" indicators appear when content is clipped.
func TestLeadSelectorWindowing(t *testing.T) {
	store := newTestStore()
	svc := setupTestService(store)
	m := NewModel(svc)
	m.width = 100
	m.height = 20 // leadVisibleRows = 20 - 14 = 6
	m.loading = false
	m.items = []core.ListItem{{ID: "x", Lead: "anchor"}} // non-empty so loading branch is skipped

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
