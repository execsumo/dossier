package core

import (
	"context"
	"strings"
	"testing"
)

func TestRosterKindDefaultsHumanAndMarksAgent(t *testing.T) {
	roster := Roster{Members: map[string]string{"alice": "Alice", "sitroom": "Sit Room"}, Kinds: map[string]string{"sitroom": "agent"}}
	if got := roster.Kind("alice"); got != "human" {
		t.Fatalf("legacy roster kind = %q, want human", got)
	}
	if got := roster.View().Members[1].Kind; got != "agent" {
		t.Fatalf("agent roster kind = %q, want agent", got)
	}
}

func TestTeamAddKindAndLeadAuthorization(t *testing.T) {
	store := &rosterTestStore{localFakeStore: newLocalFakeStore(), roster: Roster{Manager: "alice", Members: map[string]string{"alice": "Alice"}}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "alice"}, nil)
	if _, err := svc.TeamAddKind(context.Background(), "sitroom", "Sit Room", "agent"); err != nil {
		t.Fatal(err)
	}
	if got := store.roster.Kind("sitroom"); got != "agent" {
		t.Fatalf("stored kind = %q", got)
	}
	if _, err := svc.normalizeLeadUpdate(map[string]any{"lead": "sitroom"}); err == nil || !strings.Contains(err.Error(), "is an agent") {
		t.Fatalf("agent lead error = %v, want refusal", err)
	}
	if _, err := svc.TeamAddKind(context.Background(), "broken", "Broken", "service"); err == nil {
		t.Fatal("invalid kind accepted")
	}
}

func TestRosterConflictUnionPreservesAgentKind(t *testing.T) {
	shared := Roster{Manager: "alice", Members: map[string]string{"alice": "Alice", "sitroom": "Sit Room"}, Kinds: map[string]string{"sitroom": "agent"}}
	mine := Roster{Manager: "alice", Members: map[string]string{"bob": "Bob", "otherbot": "Other Bot"}, Kinds: map[string]string{"otherbot": "agent"}}
	merged, _ := unionRosters(shared, mine)
	if merged.Kind("sitroom") != "agent" || merged.Kind("otherbot") != "agent" || merged.Kind("bob") != "human" {
		t.Fatalf("merged roster kinds = %#v; want agents preserved and legacy human default", merged.Kinds)
	}
}
