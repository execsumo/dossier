package core

import (
	"context"
	"strings"
	"testing"
)

type rosterTestStore struct {
	*localFakeStore
	roster Roster
}

func (s *rosterTestStore) ReadRoster() (*Roster, error) { return &s.roster, nil }
func (s *rosterTestStore) WriteRoster(roster *Roster) error {
	s.roster = *roster
	return nil
}

func TestServiceTeamAddRemoveAndFormerResolution(t *testing.T) {
	store := &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster:         Roster{Manager: "hgill", Members: map[string]string{"hgill": "Herwin Gill"}, Former: map[string]string{}},
	}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "colleague"}, nil)
	res, err := svc.TeamAdd(context.Background(), `ACME\PSmith`, "Priya Shah")
	if err != nil {
		t.Fatalf("TeamAdd: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(string(res.Warnings[0]), "hgill") {
		t.Fatalf("non-manager warning = %v", res.Warnings)
	}
	if _, ok := store.roster.Members["psmith"]; !ok {
		t.Fatalf("normalized member missing: %+v", store.roster)
	}
	if _, err := svc.TeamRemove(context.Background(), "PSMITH"); err != nil {
		t.Fatalf("TeamRemove: %v", err)
	}
	if _, ok := store.roster.Former["psmith"]; !ok || store.roster.Former["psmith"] != "Priya Shah" {
		t.Fatalf("former member missing: %+v", store.roster)
	}
	if got, ok, _ := store.roster.ResolvePerson("Priya Shah"); !ok || got != "psmith" {
		t.Fatalf("former member did not resolve: %q, %v", got, ok)
	}
}

func TestServiceStoresRosterLeadAsUsername(t *testing.T) {
	store := &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster: Roster{Manager: "hgill", Members: map[string]string{
			"psmith": "Priya Shah",
		}, Former: map[string]string{}},
	}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "hgill"}, nil)
	res, err := svc.Promote(context.Background(), PromoteReq{Name: "Assigned", Lead: "Priya Shah", DistilledStateMarkdown: "# State\n", Force: true})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	dossier, _, err := store.Read(res.Data.(string))
	if err != nil {
		t.Fatal(err)
	}
	if dossier.Frontmatter.Lead != "psmith" {
		t.Fatalf("stored lead = %q, want psmith", dossier.Frontmatter.Lead)
	}
}

func TestServiceTeamCreateWritesManagerRoster(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{Members: map[string]string{}, Former: map[string]string{}}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: `ACME\PSmith`, DisplayName: "Priya Shah"}, &teamTestSyncer{})
	if _, err := svc.TeamCreate(context.Background(), TeamCreateReq{RemoteURL: "remote", Confirmed: true}); err != nil {
		t.Fatalf("TeamCreate: %v", err)
	}
	if store.roster.Manager != "psmith" || store.roster.Members["psmith"] != "Priya Shah" {
		t.Fatalf("manager roster = %+v", store.roster)
	}
}

type teamTestSyncer struct{}

func (*teamTestSyncer) Sync(context.Context) (SyncReport, error)         { return SyncReport{}, nil }
func (*teamTestSyncer) Status(context.Context) (SyncStatus, error)       { return SyncStatus{}, nil }
func (*teamTestSyncer) CheckRemoteEmpty(context.Context, string) error   { return nil }
func (*teamTestSyncer) Create(context.Context, string, string) error     { return nil }
func (*teamTestSyncer) Clone(context.Context, string, string, int) error { return nil }
