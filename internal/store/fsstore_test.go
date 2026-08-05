package store

import (
	"context"
	"dossier/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFSStoreInit(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-home-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	store := NewFSStore(tempHome)
	if err := store.Init(); err != nil {
		t.Fatalf("FSStore.Init() failed: %v", err)
	}

	dirs := []string{
		tempHome,
		filepath.Join(tempHome, "context"),
		filepath.Join(tempHome, "sessions"),
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", dir)
		}
	}

	guidePath := filepath.Join(tempHome, "context", "guide.md")
	guideBytes, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("failed to read guide.md: %v", err)
	}
	if !strings.Contains(string(guideBytes), "Dossier Distillation Guide") {
		t.Errorf("expected guide.md to contain signature title")
	}
}

func TestFSStoreDossierLifecycle(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-lifecycle-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	store := NewFSStore(tempHome)
	if err := store.Init(); err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	fm := core.Frontmatter{
		ID:            "dos_test123",
		Name:          "Pricing model refresh",
		Slug:          "pricing-model-refresh",
		CreatedAt:     now,
		UpdatedAt:     now,
		LastTouchedAt: now,
		Status:        core.StatusActive,
		Importance:    core.ImportanceHigh,
		Urgency:       core.UrgencyLow,
		NextAction:    "Compare revised pricing scenarios",
		OpenQuestions: []string{"Does Sales prefer usage-tier?"},
	}
	body := "# Pricing model refresh\n\n## Situation\nInitial alignment."

	dossier := &core.Dossier{
		Frontmatter:    fm,
		DistilledState: core.DistilledState{Body: body},
	}

	// 1. Write new dossier
	rev1, err := store.Write(dossier, "")
	if err != nil {
		t.Fatalf("Store.Write failed: %v", err)
	}
	if !strings.HasPrefix(string(rev1), "rev_") {
		t.Errorf("expected revision to start with rev_, got %q", rev1)
	}

	// 2. Read back and check values
	readDossier, revRead, err := store.Read("dos_test123")
	if err != nil {
		t.Fatalf("Store.Read failed: %v", err)
	}
	t.Logf("Write FM:\n%s", core.CanonicalFrontmatter(dossier.Frontmatter))
	t.Logf("Read FM:\n%s", core.CanonicalFrontmatter(readDossier.Frontmatter))
	if revRead != rev1 {
		t.Errorf("expected read revision %q to match write revision %q", revRead, rev1)
	}
	if readDossier.Frontmatter.Name != fm.Name {
		t.Errorf("expected name %q, got %q", fm.Name, readDossier.Frontmatter.Name)
	}
	if strings.TrimSpace(readDossier.DistilledState.Body) != strings.TrimSpace(body) {
		t.Errorf("expected body %q, got %q", body, readDossier.DistilledState.Body)
	}

	// 3. Concurrency check: try to write again with wrong base revision
	_, err = store.Write(dossier, "rev_wrong_base_hash")
	if err == nil {
		t.Errorf("expected concurrency check to fail on wrong base revision")
	}

	// 4. Update successfully
	readDossier.DistilledState.Body = "# Updated Title\n\n## Situation\nUpdated situation."
	rev2, err := store.Write(readDossier, rev1)
	if err != nil {
		t.Fatalf("Store.Write update failed: %v", err)
	}
	if rev2 == rev1 {
		t.Errorf("expected new revision to be different from old revision")
	}

	// 5. Read back again
	updatedDossier, revRead2, err := store.Read("pricing-model-refresh") // check by slug
	if err != nil {
		t.Fatalf("Store.Read by slug failed: %v", err)
	}
	if revRead2 != rev2 {
		t.Errorf("expected revision %q to match %q", revRead2, rev2)
	}
	if !strings.Contains(updatedDossier.DistilledState.Body, "Updated situation") {
		t.Errorf("expected body to contain updated text")
	}
}

func TestFSStoreArtifacts(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-artifacts-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	store := NewFSStore(tempHome)
	_ = store.Init()

	now := time.Now().Truncate(time.Second)
	dossier := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:            "dos_art_test",
			Name:          "Artifact Testing",
			Slug:          "artifact-testing",
			CreatedAt:     now,
			UpdatedAt:     now,
			LastTouchedAt: now,
			Status:        core.StatusActive,
			Importance:    core.ImportanceLow,
			Urgency:       core.UrgencyLow,
		},
		DistilledState: core.DistilledState{Body: "# Artifact testing"},
	}

	_, _ = store.Write(dossier, "")

	art := &core.Artifact{
		ID:            "art_test123",
		Type:          core.ArtifactTypeTranscript,
		Title:         "Sample transcript",
		ContentFormat: core.ContentFormatText,
		Content:       "Hello agent transcript contents",
	}

	// 1. Write artifact
	err = store.WriteArtifact(dossier.Frontmatter.ID, art)
	if err != nil {
		t.Fatalf("WriteArtifact failed: %v", err)
	}

	// 2. Read artifact back
	readArt, err := store.ReadArtifact(dossier.Frontmatter.ID, "art_test123")
	if err != nil {
		t.Fatalf("ReadArtifact failed: %v", err)
	}
	if readArt.Title != art.Title {
		t.Errorf("expected title %q, got %q", art.Title, readArt.Title)
	}
	if readArt.Content != art.Content {
		t.Errorf("expected content %q, got %q", art.Content, readArt.Content)
	}

	// 3. List artifacts
	list, err := store.ListArtifacts(dossier.Frontmatter.ID)
	if err != nil {
		t.Fatalf("ListArtifacts failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(list))
	}
	if list[0].ID != art.ID {
		t.Errorf("expected artifact ID %q, got %q", art.ID, list[0].ID)
	}
}

func TestFSStoreArtifactWriteAdvancesRevisionAndPreservesHistory(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-artifact-revision-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	store := NewFSStore(tempHome)
	if err := store.Init(); err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	dossier := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:            "dos_art_rev",
			Name:          "Artifact Revision",
			Slug:          "artifact-revision",
			CreatedAt:     now,
			UpdatedAt:     now,
			LastTouchedAt: now,
			Status:        core.StatusActive,
			Importance:    core.ImportanceLow,
			Urgency:       core.UrgencyLow,
		},
		DistilledState: core.DistilledState{Body: "# Artifact Revision\n\n## Situation\nBody before artifact."},
	}

	revBefore, err := store.Write(dossier, "")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	art := &core.Artifact{
		ID:            "art_revision",
		Type:          core.ArtifactTypeSourceSnapshot,
		Title:         "Revision evidence",
		Provenance:    core.Provenance{Origin: "unit test"},
		ContentFormat: core.ContentFormatText,
		Content:       "evidence",
	}
	if err := store.WriteArtifact(dossier.Frontmatter.ID, art); err != nil {
		t.Fatalf("WriteArtifact failed: %v", err)
	}

	_, revAfter, err := store.Read(dossier.Frontmatter.ID)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if revAfter == revBefore {
		t.Fatalf("expected artifact write to advance revision")
	}

	historical, err := store.ReadRevision(dossier.Frontmatter.ID, revBefore)
	if err != nil {
		t.Fatalf("expected pre-artifact revision to be readable from history: %v", err)
	}
	if historical.DistilledState.Body != dossier.DistilledState.Body {
		t.Fatalf("unexpected historical body: %q", historical.DistilledState.Body)
	}
}

func TestFSStoreAuditLog(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-audit-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	store := NewFSStore(tempHome)
	_ = store.Init()

	now := time.Now().Truncate(time.Second)
	dossier := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:            "dos_audit_test",
			Name:          "Audit Testing",
			Slug:          "audit-testing",
			CreatedAt:     now,
			UpdatedAt:     now,
			LastTouchedAt: now,
			Status:        core.StatusActive,
			Importance:    core.ImportanceLow,
			Urgency:       core.UrgencyLow,
		},
		DistilledState: core.DistilledState{Body: "# Audit testing"},
	}

	_, _ = store.Write(dossier, "")

	event := core.AuditEvent{
		TS:             now,
		Event:          core.AuditEventCreate,
		DossierID:      dossier.Frontmatter.ID,
		Actor:          "agent:unit-test",
		BeforeRevision: "",
		AfterRevision:  "rev_init_1",
	}

	// Append audit
	err = store.AppendAudit(dossier.Frontmatter.ID, event)
	if err != nil {
		t.Fatalf("AppendAudit failed: %v", err)
	}

	// Read log back
	events, err := store.ReadAuditLog(dossier.Frontmatter.ID)
	if err != nil {
		t.Fatalf("ReadAuditLog failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Event != core.AuditEventCreate {
		t.Errorf("expected event %q, got %q", core.AuditEventCreate, events[0].Event)
	}
	if events[0].Actor != "agent:unit-test" {
		t.Errorf("expected actor %q, got %q", "agent:unit-test", events[0].Actor)
	}
}

func TestSanitizeAuthorString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Alice", "alice"},
		{"Bob-Smith", "bob-smith"},
		{"user@domain.com", "user-domain-com"},
		{"  Spaces! ", "spaces"},
		{"", "unknown"},
		{"---", "unknown"},
		{"a_b_c", "a-b-c"},
	}
	for _, tc := range tests {
		actual := SanitizeAuthorString(tc.input)
		if actual != tc.expected {
			t.Errorf("SanitizeAuthorString(%q) = %q, expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestFSStoreAuditLogShardsAndLegacy(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-audit-shards-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	store := NewFSStore(tempHome)
	_ = store.Init()

	now := time.Now().Truncate(time.Second)
	dossier := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:            "dos_audit_shards",
			Name:          "Audit Shards",
			Slug:          "audit-shards",
			Status:        core.StatusActive,
			Importance:    core.ImportanceLow,
			Urgency:       core.UrgencyLow,
			CreatedAt:     now,
			UpdatedAt:     now,
			LastTouchedAt: now,
		},
		DistilledState: core.DistilledState{Body: "# Body"},
	}
	_, err = store.Write(dossier, "")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// 1. Manually write legacy audit.log
	legacyPath := filepath.Join(tempHome, "audit-shards", "audit.log")
	e1 := core.AuditEvent{TS: now.Add(-10 * time.Minute), Event: core.AuditEventCreate, Actor: "legacy"}
	err = AppendAuditLine(legacyPath, e1)
	if err != nil {
		t.Fatalf("failed to write legacy audit: %v", err)
	}
	legacyStatBefore, _ := os.Stat(legacyPath)

	// 2. Write new shards
	e2 := core.AuditEvent{TS: now.Add(-5 * time.Minute), Event: core.AuditEventSave, Author: "Alice"}
	e3 := core.AuditEvent{TS: now.Add(-2 * time.Minute), Event: core.AuditEventSave, Author: "Bob"}

	_ = store.AppendAudit(dossier.Frontmatter.ID, e3) // Write Bob's first to test sorting
	_ = store.AppendAudit(dossier.Frontmatter.ID, e2)

	// 3. Read back
	events, err := store.ReadAuditLog(dossier.Frontmatter.ID)
	if err != nil {
		t.Fatalf("ReadAuditLog failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 audit events, got %d", len(events))
	}
	if events[0].Actor != "legacy" || events[1].Author != "Alice" || events[2].Author != "Bob" {
		t.Errorf("unexpected event ordering or missing events: %+v", events)
	}

	// 4. Verify legacy file untouched
	legacyStatAfter, _ := os.Stat(legacyPath)
	if legacyStatBefore.Size() != legacyStatAfter.Size() || legacyStatBefore.ModTime() != legacyStatAfter.ModTime() {
		t.Errorf("legacy audit.log was modified!")
	}
}

func TestFSStoreSessionStash(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-session-stash-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	store := NewFSStore(tempHome)
	_ = store.Init()

	now := time.Now().Truncate(time.Second)
	dossier := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:            "dos_stash",
			Name:          "Stash",
			Slug:          "dos-stash",
			Status:        core.StatusActive,
			Importance:    core.ImportanceLow,
			Urgency:       core.UrgencyLow,
			CreatedAt:     now,
			UpdatedAt:     now,
			LastTouchedAt: now,
		},
		DistilledState: core.DistilledState{Body: "# Body"},
	}
	_, err = store.Write(dossier, "")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	err = store.WriteSessionStash("dos_stash", "Alice@Work", "sess-123", "transcript content here")
	if err != nil {
		t.Fatalf("WriteSessionStash failed: %v", err)
	}

	path := filepath.Join(tempHome, "dos-stash", "sessions", "alice-work", "sess-123.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read expected stash file: %v", err)
	}
	if string(b) != "transcript content here" {
		t.Errorf("unexpected content: %s", string(b))
	}
}

type dummySearcher struct{}

func (dummySearcher) Search(ctx context.Context, q string, scope core.SearchScope) ([]core.Hit, error) {
	return nil, nil
}

type dummyTok struct{}

func (dummyTok) Estimate(t string) int { return len(t) }

type dummyHreg struct{}

func (dummyHreg) All() []core.Harness                   { return nil }
func (dummyHreg) Get(name string) (core.Harness, error) { return nil, nil }

type dummyClock struct{ now time.Time }

func (d dummyClock) Now() time.Time { return d.now }

func TestTwoAuthorSimulation(t *testing.T) {
	tempHome, err := os.MkdirTemp("", "dossier-test-two-author-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tempHome)

	fs := NewFSStore(tempHome)
	_ = fs.Init()

	now := time.Now().Truncate(time.Second)
	dossier := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID:            "dos_sim",
			Name:          "Sim",
			Slug:          "dos-sim",
			Status:        core.StatusActive,
			Importance:    core.ImportanceLow,
			Urgency:       core.UrgencyLow,
			CreatedAt:     now,
			UpdatedAt:     now,
			LastTouchedAt: now,
		},
		DistilledState: core.DistilledState{Body: "# Start"},
	}
	_, _ = fs.Write(dossier, "")

	cfgAlice := core.Config{DossierHome: tempHome, Author: "Alice"}
	svcAlice := core.NewService(fs, dummySearcher{}, dummyTok{}, dummyHreg{}, dummyClock{now}, cfgAlice, nil)

	cfgBob := core.Config{DossierHome: tempHome, Author: "Bob"}
	svcBob := core.NewService(fs, dummySearcher{}, dummyTok{}, dummyHreg{}, dummyClock{now}, cfgBob, nil)

	_ = fs.SaveSessionBinding(&core.SessionBinding{
		SessionBindingID: "sess-alice",
		DossierID:        "dos_sim",
		Harness:          "test",
		LastSeenRevision: "rev_fake_1",
	})
	_ = fs.SaveSessionBinding(&core.SessionBinding{
		SessionBindingID: "sess-bob",
		DossierID:        "dos_sim",
		Harness:          "test",
		LastSeenRevision: "rev_fake_2",
	})

	ctx := context.Background()
	_, _ = svcAlice.Save(ctx, core.SaveReq{ID: "dos_sim", BaseRevision: "rev_fake_1", DistilledStateMarkdown: "# Alice"})
	_ = svcAlice.SessionEnd(ctx, "sess-alice", "", "alice transcript")

	_, _ = svcBob.Save(ctx, core.SaveReq{ID: "dos_sim", BaseRevision: "rev_fake_2", DistilledStateMarkdown: "# Bob"})
	_ = svcBob.SessionEnd(ctx, "sess-bob", "", "bob transcript")

	aliceAuditPath := filepath.Join(tempHome, "dos-sim", "audit", "alice.log")
	bobAuditPath := filepath.Join(tempHome, "dos-sim", "audit", "bob.log")
	aliceStashPath := filepath.Join(tempHome, "dos-sim", "sessions", "alice", "sess-alice.md")
	bobStashPath := filepath.Join(tempHome, "dos-sim", "sessions", "bob", "sess-bob.md")

	for _, p := range []string{aliceAuditPath, bobAuditPath, aliceStashPath, bobStashPath} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("expected file to exist: %s", p)
		}
	}
}
