package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestResolveConflictChoices(t *testing.T) {
	choices := []struct {
		name string
		want string
	}{
		{ConflictChoiceKeepShared, "shared"},
		{ConflictChoiceRestoreMine, "mine"},
		{ConflictChoiceKeepBoth, "shared\n\n## Unresolved disagreement (conflict conf_test)\n\nThe version below was preserved from a concurrent edit on 2026-01-02 03:04:05; reconcile and remove this section.\n\nmine\n"},
	}
	for _, tc := range choices {
		t.Run(tc.name, func(t *testing.T) {
			store := newLocalFakeStore()
			now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			store.dossiers["dos_test"] = &Dossier{
				Frontmatter:    Frontmatter{ID: "dos_test", Name: "Test", Slug: "test", Status: StatusSpark, Priority: PriorityMedium},
				DistilledState: DistilledState{Body: "shared"},
			}
			store.revisions["dos_test"] = "rev_current"
			store.conflicts["conf_test"] = &Conflict{ID: "conf_test", DossierID: "dos_test", TS: now, RejectedBody: "mine"}
			svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{DossierHome: "/tmp", Author: "Alice"}, nil)

			_, err := svc.ResolveConflict(context.Background(), ResolveConflictReq{ConflictID: "conf_test", Choice: tc.name})
			if err != nil {
				t.Fatalf("ResolveConflict: %v", err)
			}
			if _, ok := store.conflicts["conf_test"]; ok {
				t.Fatal("active conflict still present")
			}
			if _, ok := store.resolvedConflicts["conf_test"]; !ok {
				t.Fatal("resolved conflict was not archived by fake store")
			}
			if got := store.dossiers["dos_test"].DistilledState.Body; got != tc.want {
				t.Fatalf("body = %q, want %q", got, tc.want)
			}
			var resolved *AuditEvent
			for i := range store.audits["dos_test"] {
				if store.audits["dos_test"][i].Event == AuditEventConflictResolved {
					resolved = &store.audits["dos_test"][i]
				}
			}
			if resolved == nil || resolved.Author != "Alice" || !strings.Contains(resolved.Message, tc.name) {
				t.Fatalf("resolution audit = %+v", resolved)
			}
			active, err := svc.ListConflicts(context.Background())
			if err != nil || len(active) != 0 {
				t.Fatalf("ListConflicts = %v, %v", active, err)
			}
		})
	}
}

func TestResolveConflictRejectsInvalidChoiceBeforeLookup(t *testing.T) {
	store := newLocalFakeStore()
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	_, err := svc.ResolveConflict(context.Background(), ResolveConflictReq{ConflictID: "missing", Choice: "nope"})
	if err == nil || !strings.Contains(err.Error(), "keep_shared") {
		t.Fatalf("invalid choice error = %v", err)
	}
}

func TestResolveConflictMissingDossierDoesNotArchive(t *testing.T) {
	store := newLocalFakeStore()
	store.conflicts["conf_missing"] = &Conflict{ID: "conf_missing", DossierID: "dos_missing", RejectedBody: "mine"}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{}, nil)
	_, err := svc.ResolveConflict(context.Background(), ResolveConflictReq{ConflictID: "conf_missing", Choice: ConflictChoiceKeepShared})
	if err == nil {
		t.Fatal("missing dossier unexpectedly resolved")
	}
	if domainErr, ok := err.(*DomainError); !ok || domainErr.Code != ErrNotFound {
		t.Fatalf("error = %T %v, want not_found", err, err)
	}
	if _, ok := store.conflicts["conf_missing"]; !ok {
		t.Fatal("missing-dossier conflict was moved")
	}
}
