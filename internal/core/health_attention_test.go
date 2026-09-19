package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
	"dossier/internal/store"
)

type FakeSyncer struct {
	fakeStatus   core.SyncStatus
	fakeReport   core.SyncReport
	syncErr      error
	statusCalled chan struct{}
}

func (s *FakeSyncer) Sync(ctx context.Context) (core.SyncReport, error) {
	return s.fakeReport, s.syncErr
}
func (s *FakeSyncer) Status(ctx context.Context) (core.SyncStatus, error) {
	if s.statusCalled != nil {
		select {
		case s.statusCalled <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return s.fakeStatus, ctx.Err()
	}
	return s.fakeStatus, nil
}
func (s *FakeSyncer) LocalStatus(ctx context.Context) (core.SyncStatus, error) {
	return s.fakeStatus, nil
}
func (s *FakeSyncer) CheckRemoteEmpty(ctx context.Context, url string) error      { return nil }
func (s *FakeSyncer) Create(ctx context.Context, url, branch string) error        { return nil }
func (s *FakeSyncer) Clone(ctx context.Context, url, dir string, depth int) error { return nil }

type attentionClock struct{ now time.Time }

func (c *attentionClock) Now() time.Time { return c.now }

type attentionTokenizer struct{}

func (attentionTokenizer) Estimate(string) int { return 0 }

func TestSyncAttention(t *testing.T) {
	st := store.NewFakeStore()

	// Add dossier1 and dossier2 to the store
	d1 := core.Dossier{
		Frontmatter:    core.Frontmatter{ID: "dos_1", Name: "d1", Slug: "dos-1", Status: core.StatusExecute, Priority: core.PriorityMedium, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		DistilledState: core.DistilledState{Body: ""},
	}
	st.Dossiers["dos_1"] = &d1
	d2 := core.Dossier{
		Frontmatter:    core.Frontmatter{ID: "dos_2", Name: "d2", Slug: "dos-2", Status: core.StatusExecute, Priority: core.PriorityMedium, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		DistilledState: core.DistilledState{Body: ""},
	}
	st.Dossiers["dos_2"] = &d2

	syncer := &FakeSyncer{}
	svc := core.NewService(st, nil, attentionTokenizer{}, nil, &attentionClock{now: time.Now()}, core.Config{}, syncer)
	ctx := context.Background()

	// 1. Healthy (but unbound config because no syncer status)
	line, ids, ok := svc.SyncAttention(ctx, "dos_1")
	if ok {
		t.Fatalf("expected no attention when unconfigured, got %v", line)
	}

	// 2. Healthy with sync configured
	syncer.fakeStatus = core.SyncStatus{
		LastSuccessPull: time.Now(),
		LastSuccessPush: time.Now(),
		AuthState:       "file",
	}
	line, ids, ok = svc.SyncAttention(ctx, "dos_1")
	if ok {
		t.Fatalf("expected no attention when healthy, got %v", line)
	}

	// 3. Failed sync
	syncer.fakeStatus.LastError = "connection refused"
	line, ids, ok = svc.SyncAttention(ctx, "dos_1")
	if !ok || !strings.Contains(line, "tell the user") {
		t.Fatalf("expected attention on failed sync, got: %v", line)
	}
	syncer.fakeStatus.LastError = ""

	// 4. Missing creds
	syncer.fakeStatus.AuthState = "missing"
	line, ids, ok = svc.SyncAttention(ctx, "dos_1")
	if !ok || !strings.Contains(line, "tell the user") {
		t.Fatalf("expected attention on missing creds, got: %v", line)
	}
	syncer.fakeStatus.AuthState = "file"

	// 5. Conflicts on bound dossier vs elsewhere
	now := time.Now()
	st.Conflicts["c1"] = &core.Conflict{ID: "c1", DossierID: "dos_2", TS: now}

	// unbound -> should flag
	line, ids, ok = svc.SyncAttention(ctx, "")
	if !ok || !strings.Contains(line, "tell the user") {
		t.Fatalf("expected attention when unbound and store has conflicts")
	}

	// bound to dos_1 -> should NOT flag (conflict is on dos_2)
	line, ids, ok = svc.SyncAttention(ctx, "dos_1")
	if ok {
		t.Fatalf("expected NO attention when bound dossier has no conflicts, got: %v", line)
	}

	// bound to dos_2 -> should flag
	line, ids, ok = svc.SyncAttention(ctx, "dos_2")
	if !ok || len(ids) == 0 || ids[0] != "c1" {
		t.Fatalf("expected attention for bound dossier with conflicts")
	}
}

func TestSessionStartDoesNotFetchRemoteForAttention(t *testing.T) {
	st := store.NewFakeStore()
	st.Dossiers["dos_1"] = &core.Dossier{Frontmatter: core.Frontmatter{ID: "dos_1", Name: "Test", Slug: "test", Status: core.StatusExecute, Priority: core.PriorityMedium}}
	st.Revisions["dos_1"] = "rev_1"
	st.Sessions["sess_1"] = &core.SessionBinding{SessionBindingID: "sess_1", DossierID: "dos_1", LastSeenRevision: "rev_1"}

	statusCalled := make(chan struct{}, 1)
	syncer := &FakeSyncer{fakeStatus: core.SyncStatus{LastSuccessPull: time.Now()}, statusCalled: statusCalled}
	svc := core.NewService(st, nil, attentionTokenizer{}, nil, &attentionClock{now: time.Now()}, core.Config{}, syncer)
	started := time.Now()
	if _, err := svc.SessionStart(context.Background(), "sess_1"); err != nil {
		t.Fatalf("SessionStart() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("SessionStart performed a remote status fetch: %s", elapsed)
	}
	select {
	case <-statusCalled:
		t.Fatal("SessionStart called network-fetching Status")
	default:
	}
}

func TestSessionStartReportsSyncAttention(t *testing.T) {
	st := store.NewFakeStore()
	st.Dossiers["dos_1"] = &core.Dossier{
		Frontmatter: core.Frontmatter{ID: "dos_1", Name: "Test", Slug: "test", Status: core.StatusExecute, Priority: core.PriorityMedium},
	}
	st.Revisions["dos_1"] = "rev_1"
	st.Sessions["sess_1"] = &core.SessionBinding{SessionBindingID: "sess_1", DossierID: "dos_1", LastSeenRevision: "rev_1"}

	syncer := &FakeSyncer{fakeStatus: core.SyncStatus{LastSuccessPull: time.Now()}, syncErr: errors.New("connection refused")}
	svc := core.NewService(st, nil, attentionTokenizer{}, nil, &attentionClock{now: time.Now()}, core.Config{}, syncer)
	got, err := svc.SessionStart(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("SessionStart() error = %v", err)
	}
	if !strings.Contains(got, "tell the user") {
		t.Fatalf("unhealthy session start omitted sync attention: %q", got)
	}

	syncer.syncErr = nil
	got, err = svc.SessionStart(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("healthy SessionStart() error = %v", err)
	}
	if strings.Contains(got, "tell the user") {
		t.Fatalf("healthy session start emitted sync attention: %q", got)
	}
}
