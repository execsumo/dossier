package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOptimisticConcurrencyAutoMerge(t *testing.T) {
	fakeStore := newLocalFakeStore()
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	hreg := &mockHarnessRegistry{}
	clk := &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}
	cfg := Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100}

	svc := NewService(fakeStore, srch, tok, hreg, clk, cfg, nil)
	ctx := context.Background()

	// 1. Create dossier
	saveRes, err := svc.Save(ctx, SaveReq{
		FrontmatterUpdates: map[string]any{
			"name":        "Concurrency Dossier",
			"status":      "active",
			"next_action": "Original action",
		},
		DistilledStateMarkdown: "Initial body content",
	})
	if err != nil {
		t.Fatalf("failed to create dossier: %v", err)
	}

	dossierID := "dos_fake_id" // from mock store defaults
	rev1 := saveRes.Data.(Revision)

	// 2. Session A updates status to waiting starting from rev1
	saveA, err := svc.Save(ctx, SaveReq{
		ID:           dossierID,
		BaseRevision: rev1,
		FrontmatterUpdates: map[string]any{
			"status": "waiting",
		},
	})
	if err != nil {
		t.Fatalf("Session A save failed: %v", err)
	}
	rev2 := saveA.Data.(Revision)

	// 3. Session B updates next_action to "Do B" starting from rev1 (concurrency mismatch!)
	saveB, err := svc.Save(ctx, SaveReq{
		ID:           dossierID,
		BaseRevision: rev1,
		FrontmatterUpdates: map[string]any{
			"next_action": "Updated by B",
		},
	})
	if err != nil {
		t.Fatalf("Session B save failed: %v", err)
	}
	rev3 := saveB.Data.(Revision)

	if rev3 == rev2 {
		t.Errorf("expected new revision to be different")
	}

	// 4. Retrieve dossier and verify it was auto-merged
	recallRes, err := svc.Recall(ctx, RecallReq{ID: dossierID})
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	recall := recallRes.Data.(RecallResult)

	if recall.Frontmatter.Status != "waiting" {
		t.Errorf("expected merged status to be 'waiting', got %q", recall.Frontmatter.Status)
	}
	if recall.Frontmatter.NextAction != "Updated by B" {
		t.Errorf("expected merged next_action to be 'Updated by B', got %q", recall.Frontmatter.NextAction)
	}
}

func TestOptimisticConcurrencyConflict(t *testing.T) {
	fakeStore := newLocalFakeStore()
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	hreg := &mockHarnessRegistry{}
	clk := &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}
	cfg := Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100}

	svc := NewService(fakeStore, srch, tok, hreg, clk, cfg, nil)
	ctx := context.Background()

	// 1. Create dossier
	saveRes, err := svc.Save(ctx, SaveReq{
		FrontmatterUpdates: map[string]any{
			"name": "Conflict Dossier",
		},
		DistilledStateMarkdown: "Initial body",
	})
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	dossierID := "dos_fake_id"
	rev1 := saveRes.Data.(Revision)

	// 2. Session A writes "Body A" starting from rev1
	_, err = svc.Save(ctx, SaveReq{
		ID:                     dossierID,
		BaseRevision:           rev1,
		DistilledStateMarkdown: "Body A",
	})
	if err != nil {
		t.Fatalf("Session A save failed: %v", err)
	}

	// 3. Session B writes "Body B" starting from rev1 (should conflict!)
	_, err = svc.Save(ctx, SaveReq{
		ID:                     dossierID,
		BaseRevision:           rev1,
		DistilledStateMarkdown: "Body B",
	})

	if err == nil {
		t.Fatalf("expected conflict error, got nil")
	}

	dErr, ok := err.(*DomainError)
	if !ok || dErr.Code != ErrConcurrentEdit {
		t.Errorf("expected DomainError of type ErrConcurrentEdit, got %v", err)
	}
}

func TestDossierMergeHappyPath(t *testing.T) {
	fakeStore := newLocalFakeStore()
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	hreg := &mockHarnessRegistry{}
	clk := &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}
	cfg := Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100}

	svc := NewService(fakeStore, srch, tok, hreg, clk, cfg, nil)
	ctx := context.Background()

	// Create source dossier
	sourceFM := Frontmatter{
		ID:            "dos_source",
		Slug:          "source-slug",
		Name:          "Source Dossier",
		Status:        StatusActive,
		OpenQuestions: []string{"Question 1?"},
	}
	sourceD := &Dossier{
		Frontmatter:    sourceFM,
		DistilledState: DistilledState{Body: "Source body content"},
	}
	_, _ = fakeStore.Write(sourceD, "")

	// Create target dossier
	targetFM := Frontmatter{
		ID:            "dos_target",
		Slug:          "target-slug",
		Name:          "Target Dossier",
		Status:        StatusActive,
		OpenQuestions: []string{"Question 2?"},
	}
	targetD := &Dossier{
		Frontmatter:    targetFM,
		DistilledState: DistilledState{Body: "Target body content"},
	}
	_, _ = fakeStore.Write(targetD, "")

	// Perform Merge
	res, err := svc.Merge(ctx, MergeReq{
		SourceID: "dos_source",
		TargetID: "dos_target",
	})
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected Merge result to be OK")
	}

	// Verify target dossier
	mergedTarget, _, err := fakeStore.Read("dos_target")
	if err != nil {
		t.Fatalf("failed to read target: %v", err)
	}

	if len(mergedTarget.Frontmatter.OpenQuestions) != 2 {
		t.Errorf("expected 2 open questions, got %d", len(mergedTarget.Frontmatter.OpenQuestions))
	}

	if !strings.Contains(mergedTarget.DistilledState.Body, "Source body content") {
		t.Errorf("expected merged body to contain source body")
	}

	// Verify source dossier is archived
	mergedSource, _, err := fakeStore.Read("dos_source")
	if err != nil {
		t.Fatalf("failed to read source: %v", err)
	}
	if mergedSource.Frontmatter.Status != StatusArchived {
		t.Errorf("expected source status to be archived, got %q", mergedSource.Frontmatter.Status)
	}
}

func TestDossierMergeConflictAndResolution(t *testing.T) {
	fakeStore := newLocalFakeStore()
	tok := &mockTokenizer{}
	srch := &mockSearcher{}
	hreg := &mockHarnessRegistry{}
	clk := &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}
	cfg := Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100}

	svc := NewService(fakeStore, srch, tok, hreg, clk, cfg, nil)
	ctx := context.Background()

	// Create source (status waiting)
	sourceFM := Frontmatter{
		ID:     "dos_source",
		Slug:   "source",
		Name:   "Source",
		Status: StatusWaiting,
	}
	_, _ = fakeStore.Write(&Dossier{Frontmatter: sourceFM, DistilledState: DistilledState{Body: "Body A"}}, "")

	// Create target (status active)
	targetFM := Frontmatter{
		ID:     "dos_target",
		Slug:   "target",
		Name:   "Target",
		Status: StatusActive,
	}
	_, _ = fakeStore.Write(&Dossier{Frontmatter: targetFM, DistilledState: DistilledState{Body: "Body B"}}, "")

	// Perform merge (should fail due to status and body mismatch)
	res, err := svc.Merge(ctx, MergeReq{
		SourceID: "dos_source",
		TargetID: "dos_target",
	})

	if err == nil {
		t.Fatalf("expected merge conflict error, got nil")
	}
	dErr, ok := err.(*DomainError)
	if !ok || dErr.Code != ErrConflictDetected {
		t.Errorf("expected ErrConflictDetected, got %v", err)
	}

	conflict := res.Data.(*Conflict)
	if conflict.Kind != "merge_conflict" {
		t.Errorf("expected Kind merge_conflict, got %q", conflict.Kind)
	}

	// Resolve the conflict by passing the conflict ID
	res2, err := svc.Merge(ctx, MergeReq{
		SourceID:          "dos_source",
		TargetID:          "dos_target",
		ResolvedConflicts: []string{conflict.ID},
	})
	if err != nil {
		t.Fatalf("resolved merge failed: %v", err)
	}
	if !res2.OK {
		t.Fatalf("expected resolved merge result to be OK")
	}
}
