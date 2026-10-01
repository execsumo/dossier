package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingRosterAuditStore struct {
	*rosterTestStore
	err error
}

func (s *failingRosterAuditStore) AppendTeamAudit(AuditEvent) error { return s.err }

func TestTeamAdministrationIsHumanOnly(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}, Former: map[string]string{}}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, nil)
	if _, err := svc.TeamAddKindAs(context.Background(), "agent:case-officer", "agent-one", "Agent One", "agent"); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent team add error = %v", err)
	}
	if _, err := svc.TeamRemoveAs(context.Background(), "agent:case-officer", "alice"); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent team remove error = %v", err)
	}
	if _, err := svc.TeamCreate(context.Background(), TeamCreateReq{Actor: "agent:case-officer", RemoteURL: "remote", Confirmed: true}); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent team create error = %v", err)
	}
	if _, err := svc.TeamJoin(context.Background(), TeamJoinReq{Actor: "agent:case-officer", RemoteURL: "remote"}); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent team join error = %v", err)
	}
	if len(store.teamAudits) != 0 || len(store.roster.Members) != 1 {
		t.Fatalf("refused agent operations mutated state: roster=%+v audits=%+v", store.roster, store.teamAudits)
	}
}

func TestRosterConflictResolutionIsHumanOnlyAndAudited(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}, Former: map[string]string{}}}
	store.conflicts["conf_roster"] = &Conflict{ID: "conf_roster", DossierID: RosterConflictDossierID, Kind: "sync_concurrent_roster_edit", RejectedBody: "manager: bob\nmembers:\n  bob: Bob\n"}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, nil)
	if _, err := svc.ResolveConflict(context.Background(), ResolveConflictReq{Actor: "agent:case-officer", ConflictID: "conf_roster", Choice: ConflictChoiceRestoreMine}); err == nil || !strings.Contains(err.Error(), "requires a human") {
		t.Fatalf("agent roster resolution error = %v", err)
	}
	if _, err := svc.ResolveConflict(context.Background(), ResolveConflictReq{Actor: "human:alice", ConflictID: "conf_roster", Choice: ConflictChoiceRestoreMine}); err != nil {
		t.Fatalf("human roster resolution failed: %v", err)
	}
	if len(store.teamAudits) != 1 || store.teamAudits[0].Event != "team_roster_conflict_resolved" || store.teamAudits[0].Actor != "human:alice" {
		t.Fatalf("roster resolution audit = %+v", store.teamAudits)
	}
}

func TestTeamRosterWriteRollsBackWhenAuditAppendFails(t *testing.T) {
	base := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}, Former: map[string]string{}}}
	store := &failingRosterAuditStore{rosterTestStore: base, err: errors.New("disk full")}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, nil)
	if _, err := svc.TeamAdd(context.Background(), "bob", "Bob"); err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("TeamAdd error = %v, want audit failure rollback", err)
	}
	if _, ok := base.roster.Members["bob"]; ok {
		t.Fatalf("unaudited member addition survived: %+v", base.roster)
	}
}
