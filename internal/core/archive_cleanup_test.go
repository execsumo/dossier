package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type archiveCleanupStore struct {
	*localFakeStore
}

func (s *archiveCleanupStore) ListSessionBindings() ([]SessionBinding, error) {
	return []SessionBinding{{SessionBindingID: "blocked", DossierID: "dos_cleanup"}}, nil
}

func (s *archiveCleanupStore) ClearSessionBinding(string) error {
	return errors.New("binding file not writable")
}

func TestArchiveSurfacesBindingCleanupFailure(t *testing.T) {
	now := time.Now().UTC()
	st := &archiveCleanupStore{newLocalFakeStore()}
	d := &Dossier{Frontmatter: Frontmatter{ID: "dos_cleanup", Name: "Cleanup", Slug: "cleanup", CreatedAt: now, UpdatedAt: now, Status: StatusDefine, Priority: PriorityMedium}, DistilledState: DistilledState{Body: "# Preserve\n"}}
	st.dossiers[d.Frontmatter.ID] = d
	st.revisions[d.Frontmatter.ID] = CalculateRevision(d.Frontmatter, d.DistilledState.Body, nil)
	svc := NewService(st, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "alice"}, nil)
	res, err := svc.Archive(context.Background(), ArchiveReq{ID: d.Frontmatter.ID})
	if err != nil || !res.OK {
		t.Fatalf("archive failed: %+v, %v", res, err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(string(res.Warnings[0]), "binding file not writable") || !strings.Contains(string(res.Warnings[0]), "retry dossier archive cleanup") {
		t.Fatalf("cleanup failure not actionable: %v", res.Warnings)
	}
	if st.dossiers[d.Frontmatter.ID].Frontmatter.Status != StatusDone {
		t.Fatal("archive was not completed")
	}
}
