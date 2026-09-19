package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDoctorAdvisesUnknownRosterLead(t *testing.T) {
	store := &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster:         Roster{Manager: "hgill", Members: map[string]string{"hgill": "Herwin Gill"}},
	}
	now := time.Now().Truncate(time.Second)
	dossier := &Dossier{Frontmatter: Frontmatter{
		ID: "dos_unknown_lead", Name: "Unknown lead", Slug: "unknown-lead",
		CreatedAt: now, UpdatedAt: now, Status: StatusSpark, Priority: PriorityMedium, Lead: "missing",
	}, DistilledState: DistilledState{Body: "# State\n"}}
	store.dossiers[dossier.Frontmatter.ID] = dossier
	store.revisions[dossier.Frontmatter.ID] = CalculateRevision(dossier.Frontmatter, dossier.DistilledState.Body, nil)
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "hgill"}, nil)
	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range res.Warnings {
		if strings.Contains(string(warning), "not a member of the team roster") {
			return
		}
	}
	t.Fatalf("unknown lead advisory missing: %v", res.Warnings)
}
