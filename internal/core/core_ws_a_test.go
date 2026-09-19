package core_test

import (
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"strings"
	"testing"
	"time"
)

type dummySyncer struct {
	status core.SyncStatus
}

func (d *dummySyncer) Sync(ctx context.Context) (core.SyncReport, error) {
	return core.SyncReport{}, nil
}
func (d *dummySyncer) Status(ctx context.Context) (core.SyncStatus, error)   { return d.status, nil }
func (d *dummySyncer) Create(ctx context.Context, branch, url string) error  { return nil }
func (d *dummySyncer) Clone(ctx context.Context, u, h string, dth int) error { return nil }
func (d *dummySyncer) CheckRemoteEmpty(ctx context.Context, u string) error  { return nil }

func TestDoctor_ConflictCounts(t *testing.T) {
	tmpDir := t.TempDir()
	st := store.NewFSStore(tmpDir)
	syncer := &dummySyncer{status: core.SyncStatus{Ahead: 1, LastAttempt: time.Now()}}
	svc := core.NewService(st, nil, nil, nil, nil, core.Config{DossierHome: tmpDir}, syncer)

	// Create a dossier
	fm := core.Frontmatter{ID: "d1", Slug: "d1", Name: "D1", Status: "active", Priority: core.PriorityMedium}
	_, errw := st.Write(&core.Dossier{Frontmatter: fm}, "")
	if errw != nil {
		t.Fatalf("failed to write dossier: %v", errw)
	}
	// Create a dummy conflict
	err := st.WriteConflict(&core.Conflict{ID: "c1", DossierID: "d1", Kind: "sync_concurrent_edit"})
	if err != nil {
		t.Fatalf("failed to write conflict: %v", err)
	}

	res, _ := svc.Doctor(context.Background())
	rep := res.Data.(core.DoctorReport)

	if rep.SyncStatus.ConflictsFound != 1 {
		t.Errorf("expected Doctor ConflictsFound 1, got %d", rep.SyncStatus.ConflictsFound)
	}

	// Check issue count
	issueFound := false
	for _, issue := range rep.Issues {
		if strings.Contains(issue, "Unresolved conflict c1 for dossier") {
			issueFound = true
		}
	}
	if !issueFound {
		t.Errorf("expected unresolved conflict issue")
	}

	stRes, _ := svc.SyncStatus(context.Background())
	stData := stRes.Data.(core.SyncStatus)
	if stData.UnresolvedConflicts != 1 {
		t.Errorf("expected SyncStatus UnresolvedConflicts 1, got %d", stData.UnresolvedConflicts)
	}
}
