package core

import (
	"context"
	"testing"
	"time"
)

func TestChangesFeedFiltersAndIncludesAuditProvenance(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store := newLocalFakeStore()
	store.dossiers["dos_feed"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_feed", Name: "Feed", Slug: "feed", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now}}
	store.audits["dos_feed"] = []AuditEvent{
		{TS: now, Event: AuditEventSave, DossierID: "dos_feed", Actor: "agent:case-officer", AfterRevision: "rev_new", Message: "Updated situation"},
		{TS: now.Add(-time.Second), Event: AuditEventCreate, DossierID: "dos_feed", Actor: "human:alice", Message: "Created"},
	}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "vm"}, nil)
	changes, err := svc.Changes(context.Background(), now.Add(-time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Actor != "agent:case-officer" || changes[0].Revision != "rev_new" || changes[0].Summary != "Updated situation" {
		t.Fatalf("unexpected change feed: %+v", changes)
	}
}
