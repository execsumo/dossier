package core

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

type bindingListStore struct {
	*localFakeStore
	listErr error
}

func (s *bindingListStore) ListSessionBindings() ([]SessionBinding, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []SessionBinding
	for _, b := range s.sessions {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionBindingID < out[j].SessionBindingID })
	return out, nil
}

// TestDoctorReportsOrphanedSessionBindings: a machine-local binding whose
// Dossier no longer exists (a folder removed by hand) is reported by session
// and Dossier ID with a non-destructive remedy, and is never removed by doctor.
func TestDoctorReportsOrphanedSessionBindings(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &bindingListStore{localFakeStore: newLocalFakeStore()}
	d := &Dossier{Frontmatter: Frontmatter{
		ID: "dos_live", Name: "Live", Slug: "live",
		CreatedAt: now, UpdatedAt: now, Status: StatusSpark, Priority: PriorityMedium,
	}, DistilledState: DistilledState{Body: "# State\n"}}
	store.dossiers[d.Frontmatter.ID] = d
	store.revisions[d.Frontmatter.ID] = CalculateRevision(d.Frontmatter, d.DistilledState.Body, nil)
	store.sessions["sess_ok"] = &SessionBinding{SessionBindingID: "sess_ok", DossierID: "dos_live"}
	store.sessions["sess_orphan"] = &SessionBinding{SessionBindingID: "sess_orphan", DossierID: "dos_gone"}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "hgill"}, nil)

	res, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	joined := warningsText(res.Warnings)
	for _, want := range []string{
		`Session "sess_orphan" is bound to dossier dos_gone, which does not exist`,
		"dossier switch <slug-or-id> --session sess_orphan",
		"sessions/sess_orphan.json",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "sess_ok") {
		t.Fatalf("a binding to an existing dossier must be silent:\n%s", joined)
	}
	if !res.OK {
		t.Fatalf("an orphaned machine-local binding is advisory; doctor must not be harsher than before: %v", res.Warnings)
	}
	if _, ok := store.sessions["sess_orphan"]; !ok {
		t.Fatal("doctor must not remove the orphaned binding")
	}

	store.listErr = errors.New("parse sessions/bad.json: invalid character")
	res, err = svc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warningsText(res.Warnings), "Session bindings could not be listed: parse sessions/bad.json") {
		t.Fatalf("an unreadable binding must be surfaced: ok=%v %v", res.OK, res.Warnings)
	}
}
