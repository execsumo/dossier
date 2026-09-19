package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

type rosterTestStore struct {
	*localFakeStore
	roster        Roster
	rosterWritten bool
	restoreErr    error
}

func (s *rosterTestStore) ReadRoster() (*Roster, error) { return &s.roster, nil }
func (s *rosterTestStore) WriteRoster(roster *Roster) error {
	s.roster = *roster
	s.rosterWritten = true
	return nil
}

func (s *rosterTestStore) SnapshotRoster() (RosterSnapshot, error) {
	if !s.rosterWritten && s.roster.Manager == "" && len(s.roster.Members) == 0 && len(s.roster.Former) == 0 {
		return RosterSnapshot{}, nil
	}
	content, err := s.ReadRosterYAML()
	return RosterSnapshot{Content: []byte(content), Found: err == nil}, err
}

func (s *rosterTestStore) RestoreRoster(snap RosterSnapshot) error {
	if s.restoreErr != nil {
		return s.restoreErr
	}
	if !snap.Found {
		s.roster = Roster{Members: map[string]string{}, Former: map[string]string{}}
		s.rosterWritten = false
		return nil
	}
	roster, err := s.DecodeRosterYAML(string(snap.Content))
	if err != nil {
		return err
	}
	s.roster = *roster
	s.rosterWritten = true
	return nil
}

func (s *rosterTestStore) ReadRosterYAML() (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "manager: %s\n", s.roster.Manager)
	write := func(name string, members map[string]string) {
		if len(members) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s:\n", name)
		keys := make([]string, 0, len(members))
		for key := range members {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, "  %s: %s\n", key, members[key])
		}
	}
	write("members", s.roster.Members)
	write("former", s.roster.Former)
	return b.String(), nil
}

func (s *rosterTestStore) DecodeRosterYAML(content string) (*Roster, error) {
	roster := &Roster{Members: map[string]string{}, Former: map[string]string{}}
	section := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "manager:") {
			roster.Manager = strings.TrimSpace(strings.TrimPrefix(trimmed, "manager:"))
			continue
		}
		if trimmed == "members:" || trimmed == "former:" {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 && section != "" {
			if section == "members" {
				roster.Members[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			} else {
				roster.Former[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	return roster, nil
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

func TestServiceSyncPreservesFullRosterConflictYAML(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, rosterConflictSyncer{})
	if _, err := svc.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	conflicts, err := store.ListConflicts()
	if err != nil || len(conflicts) != 1 || !strings.Contains(conflicts[0].RejectedBody, "manager: alice") || !strings.Contains(conflicts[0].RejectedBody, "members:") {
		t.Fatalf("roster conflicts = %+v, err=%v", conflicts, err)
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

func TestServiceTeamCreateRollsBackRosterOnFailure(t *testing.T) {
	tests := []struct {
		name      string
		createErr error
		wantCode  ErrorCode
		wantErr   string
	}{
		{name: "push error", createErr: errors.New("push failed"), wantErr: "push failed"},
		{name: "existing store", createErr: errors.New("store is already a team store"), wantCode: ErrConflictDetected, wantErr: "store is already a team store"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &rosterTestStore{localFakeStore: newLocalFakeStore()}
			svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, &failingTeamSyncer{createErr: tt.createErr})
			_, err := svc.TeamCreate(context.Background(), TeamCreateReq{RemoteURL: "remote", Confirmed: true})
			if err == nil {
				t.Fatalf("TeamCreate error = nil, want %q", tt.wantErr)
			}
			if tt.wantCode == "" {
				if err.Error() != tt.wantErr {
					t.Fatalf("TeamCreate error = %v, want %q", err, tt.wantErr)
				}
			} else {
				if domainErr, ok := err.(*DomainError); !ok || domainErr.Code != tt.wantCode || !strings.Contains(domainErr.Message, tt.wantErr) {
					t.Fatalf("TeamCreate error = %#v, want code %s", err, tt.wantCode)
				}
			}
			if store.rosterWritten || store.roster.Manager != "" || len(store.roster.Members) != 0 {
				t.Fatalf("roster was not rolled back: %+v, written=%v", store.roster, store.rosterWritten)
			}
		})
	}
}

func TestServiceTeamCreatePreservesExistingRosterOnFailure(t *testing.T) {
	store := &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster:         Roster{Manager: "hgill", Members: map[string]string{"hgill": "Herwin Gill"}},
	}
	before, err := store.ReadRosterYAML()
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice", DisplayName: "Alice"}, &failingTeamSyncer{createErr: errors.New("push failed")})
	if _, err := svc.TeamCreate(context.Background(), TeamCreateReq{RemoteURL: "remote", Confirmed: true}); err == nil {
		t.Fatal("TeamCreate should fail")
	}
	after, err := store.ReadRosterYAML()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("roster changed after rollback:\nbefore:\n%safter:\n%s", before, after)
	}
}

func TestServiceTeamCreateReportsRollbackFailure(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), restoreErr: errors.New("restore unavailable")}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, &failingTeamSyncer{createErr: errors.New("store is already a team store")})
	_, err := svc.TeamCreate(context.Background(), TeamCreateReq{RemoteURL: "remote", Confirmed: true})
	if err == nil {
		t.Fatal("TeamCreate should fail")
	}
	domainErr, ok := err.(*DomainError)
	if !ok || domainErr.Code != ErrConflictDetected || !strings.Contains(domainErr.Message, "team roster") {
		t.Fatalf("rollback failure error = %#v", err)
	}
}

type rosterConflictSyncer struct{}

func (rosterConflictSyncer) Sync(context.Context) (SyncReport, error) {
	return SyncReport{Conflicts: []SyncConflict{{
		Path:           "team.yaml",
		LocalContent:   []byte("manager: alice\nmembers:\n  alice: Alice\n"),
		RemoteContent:  []byte("manager: alice\nmembers:\n  alice: Alice\n  bob: Bob\n"),
		LocalRevision:  "local",
		RemoteRevision: "remote",
	}}}, nil
}
func (rosterConflictSyncer) Status(context.Context) (SyncStatus, error)       { return SyncStatus{}, nil }
func (rosterConflictSyncer) CheckRemoteEmpty(context.Context, string) error   { return nil }
func (rosterConflictSyncer) Create(context.Context, string, string) error     { return nil }
func (rosterConflictSyncer) Clone(context.Context, string, string, int) error { return nil }

type teamTestSyncer struct{}

type failingTeamSyncer struct {
	teamTestSyncer
	createErr error
}

func (s *failingTeamSyncer) Create(context.Context, string, string) error { return s.createErr }

func (*teamTestSyncer) Sync(context.Context) (SyncReport, error)         { return SyncReport{}, nil }
func (*teamTestSyncer) Status(context.Context) (SyncStatus, error)       { return SyncStatus{}, nil }
func (*teamTestSyncer) CheckRemoteEmpty(context.Context, string) error   { return nil }
func (*teamTestSyncer) Create(context.Context, string, string) error     { return nil }
func (*teamTestSyncer) Clone(context.Context, string, string, int) error { return nil }
