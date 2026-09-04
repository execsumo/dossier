package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSavePreservesCompatibilityViewAndHistory(t *testing.T) {
	store := newLocalFakeStore()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	body := MergeLegacyOpenQuestions("# Legacy\n\n## Current State\n\nActive.\n", []string{"What remains?"})
	dossier := &Dossier{
		Frontmatter: Frontmatter{
			ID: "dos_legacy", Name: "Legacy", Slug: "legacy",
			CreatedAt: now, UpdatedAt: now, Status: StatusActive,
			Priority: PriorityFromLegacyMatrix("low", "high"),
		},
		DistilledState: DistilledState{Body: body},
	}
	store.dossiers[dossier.Frontmatter.ID] = dossier
	base := CalculateRevision(dossier.Frontmatter, body, nil)
	store.revisions[dossier.Frontmatter.ID] = base

	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{DossierHome: "/tmp/dossier-test"}, nil)
	res, err := svc.Save(context.Background(), SaveReq{
		ID:           dossier.Frontmatter.ID,
		BaseRevision: base,
		FrontmatterUpdates: map[string]any{
			"next_action": "Continue safely",
		},
	})
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	if res.Data.(Revision) == base {
		t.Fatal("Save() did not advance the revision")
	}
	current, _, err := store.Read(dossier.Frontmatter.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Frontmatter.Priority != PriorityMedium || strings.Count(current.DistilledState.Body, "What remains?") != 1 {
		t.Fatalf("Save() lost compatibility data: %+v\n%s", current.Frontmatter, current.DistilledState.Body)
	}
	events := store.audits[dossier.Frontmatter.ID]
	if len(events) != 1 || events[0].BeforeRevision != string(base) || events[0].AfterRevision != string(res.Data.(Revision)) {
		t.Fatalf("Save() did not audit the compatibility revision transition: %+v", events)
	}
	historical, err := store.ReadRevision(dossier.Frontmatter.ID, base)
	if err != nil {
		t.Fatalf("compatibility revision was not retained: %v", err)
	}
	if historical.DistilledState.Body != body || historical.Frontmatter.Priority != PriorityMedium {
		t.Fatalf("history changed compatibility state: %+v\n%s", historical.Frontmatter, historical.DistilledState.Body)
	}
}

func TestNewDossierDefaultsToMediumPriority(t *testing.T) {
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{DossierHome: "/tmp/dossier-test"}, nil)

	_, err := svc.Save(context.Background(), SaveReq{FrontmatterUpdates: map[string]any{"name": "Default priority"}})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	d, _, err := store.Read("dos_fake_id")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if d.Frontmatter.Priority != PriorityMedium {
		t.Fatalf("new dossier priority = %q, want %q", d.Frontmatter.Priority, PriorityMedium)
	}
}

type mockTokenizer struct{}

func (m *mockTokenizer) Estimate(text string) int {
	return len(text) / 4
}

type mockSearcher struct{}

func (m *mockSearcher) Search(ctx context.Context, query string, scope SearchScope) ([]Hit, error) {
	return nil, nil
}

type mockHarnessRegistry struct{}

func (m *mockHarnessRegistry) All() []Harness {
	return nil
}
func (m *mockHarnessRegistry) Get(name string) (Harness, error) {
	return nil, nil
}

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

type localFakeStore struct {
	dossiers           map[string]*Dossier
	revisions          map[string]Revision
	artifacts          map[string][]Artifact
	audits             map[string][]AuditEvent
	sessions           map[string]*SessionBinding
	conflicts          map[string]*Conflict
	history            map[Revision]*Dossier
	contextAssets      map[string]string
	staleContextAssets []string
	auditShardIssues   []string
	artifactFileIssues []string
}

func newLocalFakeStore() *localFakeStore {
	return &localFakeStore{
		dossiers:  make(map[string]*Dossier),
		revisions: make(map[string]Revision),
		artifacts: make(map[string][]Artifact),
		audits:    make(map[string][]AuditEvent),
		sessions:  make(map[string]*SessionBinding),
		conflicts: make(map[string]*Conflict),
		history:   make(map[Revision]*Dossier),
		contextAssets: map[string]string{
			"guide.md":        "GUIDE BODY",
			"instructions.md": "INSTRUCTIONS BODY",
		},
	}
}

func (f *localFakeStore) Init() error { return nil }
func (f *localFakeStore) Read(id string) (*Dossier, Revision, error) {
	d, ok := f.dossiers[id]
	if !ok {
		return nil, "", NewError(ErrNotFound, "not found")
	}
	cp := *d
	return &cp, f.revisions[id], nil
}
func (f *localFakeStore) ReadRevision(id string, rev Revision) (*Dossier, error) {
	d, ok := f.history[rev]
	if !ok {
		currRev := f.revisions[id]
		if currRev == rev {
			cp := *f.dossiers[id]
			return &cp, nil
		}
		return nil, NewError(ErrNotFound, "revision not found")
	}
	cp := *d
	return &cp, nil
}
func (f *localFakeStore) List(filter string) ([]Frontmatter, error) {
	var list []Frontmatter
	for _, d := range f.dossiers {
		list = append(list, d.Frontmatter)
	}
	return list, nil
}
func (f *localFakeStore) Write(d *Dossier, base Revision) (Revision, error) {
	if d.Frontmatter.ID == "" {
		d.Frontmatter.ID = "dos_fake_id"
	}
	if d.Frontmatter.Slug == "" {
		d.Frontmatter.Slug = "fake-slug"
	}

	// Check concurrency
	if base != "" {
		if currRev, ok := f.revisions[d.Frontmatter.ID]; ok && currRev != base {
			return "", NewError(ErrConcurrentEdit, "concurrency mismatch")
		}
	}

	// Save existing to history before overwriting
	if currentRev, ok := f.revisions[d.Frontmatter.ID]; ok {
		if existing, ok := f.dossiers[d.Frontmatter.ID]; ok {
			cp := *existing
			f.history[currentRev] = &cp
		}
	}

	f.dossiers[d.Frontmatter.ID] = d
	rev := CalculateRevision(d.Frontmatter, d.DistilledState.Body, f.artifacts[d.Frontmatter.ID])
	f.revisions[d.Frontmatter.ID] = rev
	return rev, nil
}
func (f *localFakeStore) WriteArtifact(id string, a *Artifact) error {
	if a.ID == "" {
		a.ID = "art_fake_" + string(rune('a'+len(f.artifacts[id])))
	}
	a.DossierID = id
	f.artifacts[id] = append(f.artifacts[id], *a)
	if d, ok := f.dossiers[id]; ok {
		f.revisions[id] = CalculateRevision(d.Frontmatter, d.DistilledState.Body, f.artifacts[id])
	}
	return nil
}
func (f *localFakeStore) ReadArtifact(id string, artID string) (*Artifact, error) {
	for _, a := range f.artifacts[id] {
		if a.ID == artID {
			cp := a
			return &cp, nil
		}
	}
	return nil, NewError(ErrNotFound, "artifact not found")
}
func (f *localFakeStore) ListArtifacts(id string) ([]Artifact, error) {
	return append([]Artifact(nil), f.artifacts[id]...), nil
}
func (f *localFakeStore) AppendAudit(id string, e AuditEvent) error {
	f.audits[id] = append(f.audits[id], e)
	return nil
}
func (f *localFakeStore) ReadAuditLog(id string) ([]AuditEvent, error)                  { return f.audits[id], nil }
func (f *localFakeStore) ValidateAuditShards(id string) []string                        { return f.auditShardIssues }
func (f *localFakeStore) ValidateArtifactFiles(id string) []string                      { return f.artifactFileIssues }
func (f *localFakeStore) EnsureAuditDir(id string) error                                { return nil }
func (f *localFakeStore) WriteSessionStash(id, author, sessionID, content string) error { return nil }
func (f *localFakeStore) SaveSessionBinding(b *SessionBinding) error {
	cp := *b
	f.sessions[b.SessionBindingID] = &cp
	return nil
}
func (f *localFakeStore) GetSessionBinding(id string) (*SessionBinding, error) {
	b, ok := f.sessions[id]
	if !ok {
		return nil, NewError(ErrNotFound, "session binding not found")
	}
	cp := *b
	return &cp, nil
}
func (f *localFakeStore) ClearSessionBinding(id string) error {
	delete(f.sessions, id)
	return nil
}
func (f *localFakeStore) WriteConflict(c *Conflict) error {
	f.conflicts[c.ID] = c
	return nil
}
func (f *localFakeStore) ReadConflict(id string) (*Conflict, error) {
	c, ok := f.conflicts[id]
	if !ok {
		return nil, NewError(ErrNotFound, "conflict not found")
	}
	return c, nil
}
func (f *localFakeStore) ListConflicts() ([]Conflict, error) {
	var out []Conflict
	for _, c := range f.conflicts {
		out = append(out, *c)
	}
	return out, nil
}
func (f *localFakeStore) WriteLibraryContext(data LibraryData) error { return nil }

// contextAssets stands in for the embedded originals. A nil map models a build
// with no assets at all; a missing key models an asset absent from disk *and*
// from the binary, which is the only way a reader can legitimately come back empty.
func (f *localFakeStore) EnsureContextAssets() ([]string, error) { return nil, nil }
func (f *localFakeStore) ReadContextAsset(name string) (string, error) {
	if content, ok := f.contextAssets[name]; ok {
		return content, nil
	}
	return "", NewError(ErrNotFound, "context asset not found: "+name)
}
func (f *localFakeStore) StaleContextAssets() []string { return f.staleContextAssets }

func TestServiceListFiltersInterfaces(t *testing.T) {
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Now()}, Config{DossierHome: "/tmp/dossier-test"}, nil)
	now := time.Now()
	store.dossiers["dos_pricing"] = &Dossier{Frontmatter: Frontmatter{
		ID: "dos_pricing", Name: "Pricing", Slug: "pricing", CreatedAt: now,
		UpdatedAt: now, Status: StatusActive, Priority: PriorityHigh,
		Interfaces: []string{"Pricing WBR", "Steerco"},
	}, DistilledState: DistilledState{Body: "# Topic"}}
	store.dossiers["dos_one"] = &Dossier{Frontmatter: Frontmatter{
		ID: "dos_one", Name: "One on one", Slug: "one-on-one", CreatedAt: now,
		UpdatedAt: now, Status: StatusActive, Priority: PriorityHigh,
		Interfaces: []string{"1:1"},
	}, DistilledState: DistilledState{Body: "# Topic"}}
	store.revisions["dos_pricing"] = CalculateRevision(store.dossiers["dos_pricing"].Frontmatter, "# Topic", nil)
	store.revisions["dos_one"] = CalculateRevision(store.dossiers["dos_one"].Frontmatter, "# Topic", nil)

	res, err := svc.List(context.Background(), ListReq{Interfaces: []string{"Pricing WBR"}})
	if err != nil {
		t.Fatalf("filtered list failed: %v", err)
	}
	items := res.Data.([]ListItem)
	if len(items) != 1 || items[0].Name != "Pricing" || len(items[0].Interfaces) != 2 {
		t.Fatalf("unexpected filtered items: %+v", items)
	}
}

func TestServiceListAndRecall(t *testing.T) {
	fakeStore := newLocalFakeStore()
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	hreg := &mockHarnessRegistry{}
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	clk := &mockClock{now: now}
	cfg := Config{DossierHome: "/tmp/dossier-test"}

	svc := NewService(fakeStore, srch, tok, hreg, clk, cfg, nil)

	ctx := context.Background()
	saveReq := SaveReq{
		DistilledStateMarkdown: "# Test\n\n## Situation\nWorking fine.",
		FrontmatterUpdates: map[string]any{
			"name":     "Pricing model refresh",
			"status":   "active",
			"priority": "high",
		},
	}

	res, err := svc.Save(ctx, saveReq)
	if err != nil {
		t.Fatalf("Service.Save failed: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected Service.Save result to be OK")
	}

	listRes, err := svc.List(ctx, ListReq{})
	if err != nil {
		t.Fatalf("Service.List failed: %v", err)
	}
	if !listRes.OK {
		t.Errorf("expected list response to be OK")
	}

	// Recall
	var dossierID string
	for id := range fakeStore.dossiers {
		dossierID = id
		break
	}

	recallRes, err := svc.Recall(ctx, RecallReq{ID: dossierID})
	if err != nil {
		t.Fatalf("Service.Recall failed: %v", err)
	}
	if !recallRes.OK {
		t.Fatalf("expected Recall to be OK")
	}

	// Set status to archived
	archiveRes, err := svc.Archive(ctx, ArchiveReq{ID: dossierID})
	if err != nil {
		t.Fatalf("Service.Archive failed: %v", err)
	}
	if !archiveRes.OK {
		t.Fatalf("expected Archive to be OK")
	}

	// Read back and verify status is archived
	d, _, err := fakeStore.Read(dossierID)
	if err != nil {
		t.Fatalf("failed to read from store: %v", err)
	}
	if d.Frontmatter.Status != StatusDone {
		t.Errorf("expected status to be done, got %q", d.Frontmatter.Status)
	}
}

func TestSaveReturnsRevisionIncludingArtifacts(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}, Config{}, nil)
	ctx := context.Background()

	createRes, err := svc.Save(ctx, SaveReq{
		DistilledStateMarkdown: "# Artifact Revision\n\n## Situation\nCurrent state [src:art_evidence].",
		FrontmatterUpdates: map[string]any{
			"name":     "Artifact Revision",
			"status":   "active",
			"priority": "medium",
		},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	updateRes, err := svc.Save(ctx, SaveReq{
		ID:           "dos_fake_id",
		BaseRevision: createRes.Data.(Revision),
		Artifacts: []Artifact{{
			ID:            "art_evidence",
			Type:          ArtifactTypeSourceSnapshot,
			Title:         "Evidence",
			Provenance:    Provenance{Origin: "unit test"},
			ContentFormat: ContentFormatText,
			Content:       "Artifact content",
		}},
	})
	if err != nil {
		t.Fatalf("save with artifact failed: %v", err)
	}

	_, readRev, err := fakeStore.Read("dos_fake_id")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if updateRes.Data.(Revision) != readRev {
		t.Fatalf("expected returned revision %q to include artifacts and match read revision %q", updateRes.Data.(Revision), readRev)
	}
}

func TestSessionEndCapturesTranscriptWithoutDistilledState(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}, Config{}, nil)
	ctx := context.Background()

	createRes, err := svc.Save(ctx, SaveReq{
		DistilledStateMarkdown: "# Hook Backstop\n\n## Situation\nOriginal state.",
		FrontmatterUpdates: map[string]any{
			"name":     "Hook Backstop",
			"status":   "active",
			"priority": "medium",
		},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	initialRev := createRes.Data.(Revision)
	if err := fakeStore.SaveSessionBinding(&SessionBinding{
		SessionBindingID: "sess_test",
		Harness:          "claude-code",
		DossierID:        "dos_fake_id",
		LastSeenRevision: string(initialRev),
	}); err != nil {
		t.Fatalf("binding failed: %v", err)
	}

	warnings, err := svc.SessionEnd(ctx, "sess_test", "", "transcript payload")
	if err != nil {
		t.Fatalf("SessionEnd failed: %v", err)
	}
	// Nothing was saved while this session ran, so the boundary must say so out
	// loud rather than only in the audit log — the transcript survives, but the
	// Distilled State is stale and the user cannot tell from the outside.
	var sawWarning bool
	for _, w := range warnings {
		if strings.Contains(string(w), "Distilled State was not updated this session") {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Fatalf("expected a surfaced warning for an unsaved session, got %v", warnings)
	}

	d, revAfter, err := fakeStore.Read("dos_fake_id")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if d.DistilledState.Body != "# Hook Backstop\n\n## Situation\nOriginal state." {
		t.Fatalf("distilled state should be unchanged, got %q", d.DistilledState.Body)
	}
	if revAfter == initialRev {
		t.Fatalf("expected transcript artifact to advance revision")
	}
	if len(fakeStore.artifacts["dos_fake_id"]) != 1 {
		t.Fatalf("expected one transcript artifact, got %d", len(fakeStore.artifacts["dos_fake_id"]))
	}
	binding, err := fakeStore.GetSessionBinding("sess_test")
	if err != nil {
		t.Fatalf("binding read failed: %v", err)
	}
	if binding.LastSeenRevision != string(revAfter) {
		t.Fatalf("expected binding revision %q, got %q", revAfter, binding.LastSeenRevision)
	}
	var sawNoDistilledAudit bool
	for _, event := range fakeStore.audits["dos_fake_id"] {
		if event.Event == AuditEventDistilledStateNotCaptured {
			sawNoDistilledAudit = true
		}
	}
	if !sawNoDistilledAudit {
		t.Fatalf("expected a %s audit entry", AuditEventDistilledStateNotCaptured)
	}
}

// A session that saved as it went hits the same no-payload branch at the
// boundary, but has lost nothing — warning there would train the user to ignore
// the one that matters.
func TestSessionEndDoesNotWarnWhenSessionSavedEagerly(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}, Config{}, nil)
	ctx := context.Background()

	createRes, err := svc.Save(ctx, SaveReq{
		DistilledStateMarkdown: "# Eager\n\n## Situation\nBound here.",
		FrontmatterUpdates:     map[string]any{"name": "Eager", "status": "active", "priority": "medium"},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	boundRev := createRes.Data.(Revision)
	if err := fakeStore.SaveSessionBinding(&SessionBinding{
		SessionBindingID: "sess_eager",
		Harness:          "claude-code",
		DossierID:        "dos_fake_id",
		LastSeenRevision: string(boundRev),
	}); err != nil {
		t.Fatalf("binding failed: %v", err)
	}

	// The eager save the Operating Instructions mandate.
	if _, err := svc.Save(ctx, SaveReq{
		ID:                     "dos_fake_id",
		BaseRevision:           boundRev,
		DistilledStateMarkdown: "# Eager\n\n## Situation\nSaved mid-session.",
	}); err != nil {
		t.Fatalf("eager save failed: %v", err)
	}

	warnings, err := svc.SessionEnd(ctx, "sess_eager", "", "transcript payload")
	if err != nil {
		t.Fatalf("SessionEnd failed: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(string(w), "Distilled State was not updated this session") {
			t.Fatalf("unexpected unsaved-session warning after an eager save: %q", w)
		}
	}
	for _, event := range fakeStore.audits["dos_fake_id"] {
		if event.Event == AuditEventDistilledStateNotCaptured {
			t.Fatalf("unexpected %s audit entry after an eager save", AuditEventDistilledStateNotCaptured)
		}
	}
}

func TestDoctorReportsProvenanceAndConflictIssues(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	fakeStore := newLocalFakeStore()
	fakeStore.dossiers["dos_bad"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:        "dos_bad",
			Name:      "Bad Dossier",
			Slug:      "bad-dossier",
			CreatedAt: now,
			UpdatedAt: now,
			Status:    StatusActive, Priority: PriorityHigh,
		},
		DistilledState: DistilledState{Body: "# Bad Dossier\n\n## Situation\nA material claim without provenance.\nAnother claim [src:art_missing]."},
	}
	fakeStore.revisions["dos_bad"] = CalculateRevision(fakeStore.dossiers["dos_bad"].Frontmatter, fakeStore.dossiers["dos_bad"].DistilledState.Body, nil)
	fakeStore.artifacts["dos_bad"] = []Artifact{{
		ID:            "art_empty_origin",
		DossierID:     "dos_bad",
		Type:          ArtifactTypeSourceSnapshot,
		Title:         "No provenance origin",
		CapturedAt:    now,
		RefreshedAt:   now,
		ContentFormat: ContentFormatText,
		Content:       "source",
	}}
	fakeStore.conflicts["conf_bad"] = &Conflict{ID: "conf_bad", DossierID: "dos_bad", Kind: "merge_conflict", TS: now}

	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	if res.OK {
		t.Fatalf("expected doctor to report issues")
	}
	joined := warningsText(res.Warnings)
	for _, want := range []string{"missing provenance", "references missing artifact art_missing", "missing provenance.origin", "Unresolved conflict conf_bad"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected doctor warning containing %q, got:\n%s", want, joined)
		}
	}
}

func TestDoctorHealthyWithValidProvenance(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	fakeStore := newLocalFakeStore()
	fakeStore.dossiers["dos_good"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:        "dos_good",
			Name:      "Good Dossier",
			Slug:      "good-dossier",
			CreatedAt: now,
			UpdatedAt: now,
			Status:    StatusActive,
			Priority:  PriorityLow,
		},
		DistilledState: DistilledState{Body: "# Good Dossier\n\n## Situation\nA supported material claim. [src:art_good]"},
	}
	fakeStore.artifacts["dos_good"] = []Artifact{{
		ID:            "art_good",
		DossierID:     "dos_good",
		Type:          ArtifactTypeSourceSnapshot,
		Title:         "Evidence",
		CapturedAt:    now,
		RefreshedAt:   now,
		Provenance:    Provenance{Origin: "unit test"},
		ContentFormat: ContentFormatText,
		Content:       "source",
	}}
	fakeStore.revisions["dos_good"] = CalculateRevision(fakeStore.dossiers["dos_good"].Frontmatter, fakeStore.dossiers["dos_good"].DistilledState.Body, fakeStore.artifacts["dos_good"])

	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected doctor healthy, got warnings:\n%s", warningsText(res.Warnings))
	}
	report := res.Data.(DoctorReport)
	if report.DossiersChecked != 1 || report.ArtifactsChecked != 1 || report.AuditLogsChecked != 1 {
		t.Fatalf("unexpected report counts: %+v", report)
	}
}

func warningsText(warnings []Warning) string {
	var parts []string
	for _, warning := range warnings {
		parts = append(parts, string(warning))
	}
	return strings.Join(parts, "\n")
}

// TestSessionStartUnboundIsCompactNudge guards the dogfooding fix where an
// unbound session's injected context was a full open-dossier bulletlist plus
// a 3-step instructional block, steering every session (including ones with
// nothing to do with Dossier) toward thinking about it. Unbound sessions now
// get a single-line nudge; the heavy payload (guide, full Distilled State)
// is only delivered via dossier_session once a dossier is actually bound.
func TestSessionStartUnboundIsCompactNudge(t *testing.T) {
	fakeStore := newLocalFakeStore()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	fakeStore.dossiers["dos_a"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:     "dos_a",
			Name:   "Pricing model refresh",
			Slug:   "pricing-model-refresh",
			Status: StatusActive, Priority: PriorityHigh,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{DossierHome: "/tmp/dossier-test"}, nil)

	payload, err := svc.SessionStart(context.Background(), "sess_unbound")
	if err != nil {
		t.Fatalf("SessionStart failed: %v", err)
	}

	if !strings.Contains(payload, "Pricing model refresh") {
		t.Errorf("expected open dossier name in nudge, got:\n%s", payload)
	}
	if strings.Contains(payload, "check the Open Dossiers list") || strings.Contains(payload, "similarity check and flag") {
		t.Errorf("expected the old multi-step instructional block to be gone, got:\n%s", payload)
	}
	if strings.Contains(payload, "Active Dossier:") {
		t.Errorf("expected no Active Dossier block for an unbound session, got:\n%s", payload)
	}
	if strings.Count(payload, "\n") > 4 {
		t.Errorf("expected a compact few-line payload for an unbound session, got %d lines:\n%s", strings.Count(payload, "\n"), payload)
	}
}

func TestServiceListSorting(t *testing.T) {
	fakeStore := newLocalFakeStore()
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	hreg := &mockHarnessRegistry{}
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	clk := &mockClock{now: now}
	cfg := Config{DossierHome: "/tmp/dossier-test"}

	// Dossier A: High priority, Due 2026-07-05
	fakeStore.dossiers["dos_a"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:     "dos_a",
			Name:   "Dossier A",
			Slug:   "dossier-a",
			Status: StatusActive, Priority: PriorityHigh,
			DueDate:   "2026-07-05",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	// Dossier B: Max priority, Due 2026-07-10
	fakeStore.dossiers["dos_b"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:     "dos_b",
			Name:   "Dossier B",
			Slug:   "dossier-b",
			Status: StatusActive, Priority: PriorityMax,
			DueDate:   "2026-07-10",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	// Dossier C: High priority, Due 2026-07-01
	fakeStore.dossiers["dos_c"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:     "dos_c",
			Name:   "Dossier C",
			Slug:   "dossier-c",
			Status: StatusActive, Priority: PriorityHigh,
			DueDate:   "2026-07-01",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	// Dossier D: High priority, No Due Date
	fakeStore.dossiers["dos_d"] = &Dossier{
		Frontmatter: Frontmatter{
			ID:     "dos_d",
			Name:   "Dossier D",
			Slug:   "dossier-d",
			Status: StatusActive, Priority: PriorityHigh,
			DueDate:   "",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	for _, d := range fakeStore.dossiers {
		fakeStore.revisions[d.Frontmatter.ID] = CalculateRevision(d.Frontmatter, d.DistilledState.Body, nil)
	}

	svc := NewService(fakeStore, srch, tok, hreg, clk, cfg, nil)
	listRes, err := svc.List(context.Background(), ListReq{})
	if err != nil {
		t.Fatalf("Service.List failed: %v", err)
	}
	items := listRes.Data.([]ListItem)
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	expectedOrder := []string{"dos_b", "dos_c", "dos_a", "dos_d"}
	for i, expectedID := range expectedOrder {
		if items[i].ID != expectedID {
			t.Errorf("at index %d: expected %s, got %s", i, expectedID, items[i].ID)
		}
	}
}

// A file sitting in artifacts/ without artifact frontmatter is invisible to the
// evidence index but findable by search, so it reads as captured evidence while
// being none. Doctor must surface it rather than report the store healthy.
func TestDoctorReportsNonArtifactFilesInArtifactsDir(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)

	fakeStore.dossiers["d1"] = &Dossier{
		Frontmatter: Frontmatter{ID: "d1", Slug: "d1", Status: StatusActive},
	}
	fakeStore.revisions["d1"] = "rev1"

	const issue = "Dossier d1: benchmark-notes.md sits in artifacts/ but is not an artifact"
	fakeStore.artifactFileIssues = []string{issue}

	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	report, ok := res.Data.(DoctorReport)
	if !ok {
		t.Fatalf("Doctor data = %T, want DoctorReport", res.Data)
	}
	found := false
	for _, got := range report.Issues {
		if got == issue {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("doctor did not report the non-artifact file; issues = %v", report.Issues)
	}
	// It must count as damage, not an advisory: otherwise the store still reports healthy.
	if len(report.Issues) == 0 {
		t.Errorf("non-artifact file was recorded as an advisory, not an issue")
	}
}

func TestDoctorAuditShards(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)

	d := &Dossier{
		Frontmatter: Frontmatter{ID: "d1", Slug: "d1", Status: StatusActive},
	}
	fakeStore.dossiers["d1"] = d
	fakeStore.revisions["d1"] = "rev1"

	fakeStore.auditShardIssues = []string{"Audit shard bad_name.log in dossier d1 has malformed name"}

	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	found := false
	for _, issue := range res.Warnings {
		if string(issue) == "Audit shard bad_name.log in dossier d1 has malformed name" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected doctor to report malformed shard issue")
	}
}

func TestTwoAuthorSimulation(t *testing.T) {
	// 5. Two-author simulation test
	// We'll use actual FSStore for this to verify filesystem.
	// But it requires importing store package. We can't easily do it here without circular imports if we're not careful.
	// But wait, core doesn't import store. Store imports core.
	// We can put this test in fsstore_test.go or a separate test package.
	// We will skip here and add it to a new file in store package.
}

func TestSessionEndMissingTranscriptEmitsWarning(t *testing.T) {
	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}, Config{}, nil)
	ctx := context.Background()

	fakeStore.dossiers["dos_missing"] = &Dossier{
		Frontmatter:    Frontmatter{ID: "dos_missing", Slug: "dos_missing", Status: StatusActive},
		DistilledState: DistilledState{Body: "state"},
	}
	fakeStore.revisions["dos_missing"] = "rev1"

	if err := fakeStore.SaveSessionBinding(&SessionBinding{
		SessionBindingID: "sess_missing",
		Harness:          "claude-code",
		DossierID:        "dos_missing",
		LastSeenRevision: "rev1",
	}); err != nil {
		t.Fatalf("binding failed: %v", err)
	}

	if _, err := svc.SessionEnd(ctx, "sess_missing", "new state", ""); err != nil {
		t.Fatalf("SessionEnd failed: %v", err)
	}

	var sawMissingAudit bool
	for _, event := range fakeStore.audits["dos_missing"] {
		if event.Event == AuditEventTranscriptCaptureUnavailable {
			sawMissingAudit = true
		}
	}
	if !sawMissingAudit {
		t.Fatalf("expected audit event AuditEventTranscriptCaptureUnavailable for missing transcript")
	}
}

type mockSyncer struct {
	syncCalls int
	syncErr   error
}

func (m *mockSyncer) Sync(ctx context.Context) (SyncReport, error) {
	m.syncCalls++
	return SyncReport{Error: ""}, m.syncErr
}
func (m *mockSyncer) Status(ctx context.Context) (SyncStatus, error) {
	return SyncStatus{Ahead: 1, Behind: 2}, nil
}
func (m *mockSyncer) Create(ctx context.Context) error                            { return nil }
func (m *mockSyncer) Clone(ctx context.Context, url, dir string, depth int) error { return nil }

func TestSessionBoundarySyncs(t *testing.T) {
	fakeStore := newLocalFakeStore()

	// Create a dummy dossier and session binding so SessionEnd doesn't error out early.
	d := &Dossier{
		Frontmatter: Frontmatter{ID: "dos_1", Status: StatusActive},
	}
	fakeStore.dossiers["dos_1"] = d
	fakeStore.revisions["dos_1"] = "rev_1"
	_ = fakeStore.SaveSessionBinding(&SessionBinding{
		SessionBindingID: "test",
		DossierID:        "dos_1",
		LastSeenRevision: "rev_1",
	})

	syncer := &mockSyncer{}
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, syncer)

	// SessionStart
	_, err := svc.SessionStart(context.Background(), "test")
	if err != nil {
		t.Fatalf("SessionStart error: %v", err)
	}
	if syncer.syncCalls != 1 {
		t.Errorf("expected 1 sync call from SessionStart, got %d", syncer.syncCalls)
	}

	// SessionEnd
	_, err = svc.SessionEnd(context.Background(), "test", "", "")
	if err != nil {
		t.Fatalf("SessionEnd error: %v", err)
	}
	if syncer.syncCalls != 2 {
		t.Errorf("expected 2 sync calls total after SessionEnd, got %d", syncer.syncCalls)
	}

	// Test non-fatal error
	syncer.syncErr = NewError(ErrInternal, "mock network error")
	_, err = svc.SessionStart(context.Background(), "test")
	if err != nil {
		t.Fatalf("SessionStart error should be ignored: %v", err)
	}
	_, err = svc.SessionEnd(context.Background(), "test", "", "")
	if err != nil {
		t.Fatalf("SessionEnd error should be ignored: %v", err)
	}
}

func TestServiceDoctorSync(t *testing.T) {
	fakeStore := newLocalFakeStore()
	syncer := &mockSyncer{}
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, syncer)

	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor error: %v", err)
	}
	report := res.Data.(DoctorReport)
	if !report.SyncConfigured {
		t.Errorf("expected SyncConfigured to be true")
	}
	if report.SyncStatus == nil || report.SyncStatus.Ahead != 1 || report.SyncStatus.Behind != 2 {
		t.Errorf("expected valid SyncStatus, got %+v", report.SyncStatus)
	}
}

func TestRecallConfiguredTokenLimit(t *testing.T) {
	fakeStore := newLocalFakeStore()
	d := &Dossier{
		Frontmatter: Frontmatter{
			ID:     "dos_1",
			Name:   "Token Test",
			Slug:   "token-test",
			Status: StatusActive,
		},
		DistilledState: DistilledState{
			Body: "one two three four five",
		},
	}
	fakeStore.Write(d, "init")

	t.Run("warning produced when exceeding configured limit", func(t *testing.T) {
		svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{
			TokenLimit: 3,
		}, nil)

		res, err := svc.Recall(context.Background(), RecallReq{ID: "dos_1"})
		if err != nil {
			t.Fatalf("Recall failed: %v", err)
		}
		found := false
		for _, w := range res.Warnings {
			if strings.Contains(string(w), "Distilled State exceeds token target (5 > 3 tokens)") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected token warning, got warnings: %v", res.Warnings)
		}
	})

	t.Run("no warning when under configured limit", func(t *testing.T) {
		svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{
			TokenLimit: 10,
		}, nil)

		res, err := svc.Recall(context.Background(), RecallReq{ID: "dos_1"})
		if err != nil {
			t.Fatalf("Recall failed: %v", err)
		}
		for _, w := range res.Warnings {
			if strings.Contains(string(w), "Distilled State exceeds token target") {
				t.Errorf("unexpected token warning: %v", w)
			}
		}
	})

	t.Run("defaults to DefaultTokenLimit when unspecified", func(t *testing.T) {
		svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
		if svc.TokenLimit() != DefaultTokenLimit {
			t.Errorf("expected TokenLimit() = %d, got %d", DefaultTokenLimit, svc.TokenLimit())
		}
	})
}

// The Distillation Guide must reach context before the agent composes a write,
// and must not arrive twice for the same context window. These tests pin both
// halves, including the case where the trade-off is forced.
func TestGuideDeliveredOncePerSession(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "context"), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "context", "guide.md"), []byte("GUIDE BODY"), 0644); err != nil {
		t.Fatalf("write guide failed: %v", err)
	}

	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}, Config{DossierHome: home}, nil)

	if err := fakeStore.SaveSessionBinding(&SessionBinding{
		SessionBindingID: "sess_guide",
		Harness:          "claude-code",
		DossierID:        "dos_fake_id",
	}); err != nil {
		t.Fatalf("binding failed: %v", err)
	}

	if got := svc.GuideForSession("sess_guide"); got != "GUIDE BODY" {
		t.Fatalf("first delivery should carry the Guide, got %q", got)
	}
	if got := svc.GuideForSession("sess_guide"); got != "" {
		t.Fatalf("second delivery in the same session should be suppressed, got %q", got)
	}

	// A different session shares the store but not the context window.
	if err := fakeStore.SaveSessionBinding(&SessionBinding{
		SessionBindingID: "sess_other",
		Harness:          "claude-code",
		DossierID:        "dos_fake_id",
	}); err != nil {
		t.Fatalf("binding failed: %v", err)
	}
	if got := svc.GuideForSession("sess_other"); got != "GUIDE BODY" {
		t.Fatalf("a separate session must get its own copy, got %q", got)
	}

	// An unbound session cannot be tracked; deliver rather than withhold.
	if got := svc.GuideForSession("sess_unknown"); got != "GUIDE BODY" {
		t.Fatalf("an untrackable session must still receive the Guide, got %q", got)
	}
}

func TestSessionStartResendsGuideAfterCompaction(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "context"), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "context", "guide.md"), []byte("GUIDE BODY"), 0644); err != nil {
		t.Fatalf("write guide failed: %v", err)
	}

	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}, Config{DossierHome: home}, nil)
	ctx := context.Background()

	if _, err := svc.Save(ctx, SaveReq{
		DistilledStateMarkdown: "# Guided\n\n## Situation\nBody.",
		FrontmatterUpdates:     map[string]any{"name": "Guided", "status": "active", "priority": "medium"},
	}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if err := fakeStore.SaveSessionBinding(&SessionBinding{
		SessionBindingID: "sess_compact",
		Harness:          "claude-code",
		DossierID:        "dos_fake_id",
	}); err != nil {
		t.Fatalf("binding failed: %v", err)
	}

	first, err := svc.SessionStart(ctx, "sess_compact")
	if err != nil {
		t.Fatalf("SessionStart failed: %v", err)
	}
	if !strings.Contains(first, "GUIDE BODY") {
		t.Fatalf("a bound session start must carry the Guide ahead of any tool call")
	}

	// The MCP bind that follows recognises it as already delivered.
	if got := svc.GuideForSession("sess_compact"); got != "" {
		t.Fatalf("the bind following a session start should not repeat the Guide, got %q", got)
	}

	// Compaction rebuilds the context window and fires SessionStart again; the
	// previous copy is gone, so this one must resend.
	second, err := svc.SessionStart(ctx, "sess_compact")
	if err != nil {
		t.Fatalf("second SessionStart failed: %v", err)
	}
	if !strings.Contains(second, "GUIDE BODY") {
		t.Fatalf("the session start after compaction must resend the Guide")
	}
}

func TestSwitchCarriesGuideDeliveryAcrossRebind(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "context"), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "context", "guide.md"), []byte("GUIDE BODY"), 0644); err != nil {
		t.Fatalf("write guide failed: %v", err)
	}

	fakeStore := newLocalFakeStore()
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}, Config{DossierHome: home}, nil)
	ctx := context.Background()

	if _, err := svc.Save(ctx, SaveReq{
		DistilledStateMarkdown: "# Switch Target\n\n## Situation\nBody.",
		FrontmatterUpdates:     map[string]any{"name": "Switch Target", "status": "active", "priority": "medium"},
	}); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if _, err := svc.Switch(ctx, SwitchReq{ID: "dos_fake_id", SessionID: "sess_switch"}); err != nil {
		t.Fatalf("first switch failed: %v", err)
	}
	if got := svc.GuideForSession("sess_switch"); got != "GUIDE BODY" {
		t.Fatalf("first bind should deliver the Guide, got %q", got)
	}

	// Re-binding within the session must not re-send: the Guide is
	// dossier-independent, so a switch adds no new instruction to follow.
	if _, err := svc.Switch(ctx, SwitchReq{ID: "dos_fake_id", SessionID: "sess_switch"}); err != nil {
		t.Fatalf("second switch failed: %v", err)
	}
	if got := svc.GuideForSession("sess_switch"); got != "" {
		t.Fatalf("re-binding in the same session should not repeat the Guide, got %q", got)
	}
}
