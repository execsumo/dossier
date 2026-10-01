package core

import (
	"context"
	"testing"
	"time"
)

func TestAgentCannotAcceptDelegationContract(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	body := "## Delegation Contracts\n### Intake — owner: agent:case-officer\n- Scope: [decided] Triage.\n- Acceptance: [proposed] Awaiting human review.\n"
	store := newLocalFakeStore()
	store.dossiers["dos_contract"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_contract", Name: "Contract", Slug: "contract", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now}, DistilledState: DistilledState{Body: body}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "alice"}, nil)
	proposed := "## Delegation Contracts\n### Intake — owner: agent:case-officer\n- Scope: [decided] Triage.\n- Acceptance: [decided] Accepted against rev_123 by Alice.\n"
	result, err := svc.Save(context.Background(), SaveReq{ID: "dos_contract", Actor: "agent:case-officer", DistilledStateMarkdown: proposed})
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(map[string]any)
	if data["applied"] != false || len(store.conflicts) != 1 || store.dossiers["dos_contract"].DistilledState.Body != body {
		t.Fatalf("agent acceptance was not held for review: result=%+v conflicts=%+v", result, store.conflicts)
	}
}

func TestSessionEndPreservesProtectedAgentProposalWithoutAssumingRevisionResult(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	body := "# Topic\n\n## Objective\nKeep scope.\n\n## Situation\nCurrent.\n"
	store := newLocalFakeStore()
	store.dossiers["dos_session_proposal"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_session_proposal", Name: "Topic", Slug: "topic", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now}, DistilledState: DistilledState{Body: body}}
	store.SaveSessionBinding(&SessionBinding{SessionBindingID: "sess_agent", Harness: "claude-code", DossierID: "dos_session_proposal", LastSeenRevision: string(store.revisions["dos_session_proposal"])})
	_, revision, err := store.Read("dos_session_proposal")
	if err != nil {
		t.Fatal(err)
	}
	store.sessions["sess_agent"].LastSeenRevision = string(revision)
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "alice"}, nil)
	proposed := "# Topic\n\n## Objective\nExpand scope.\n\n## Situation\nCurrent.\n"
	warnings, err := svc.SessionEndAs(context.Background(), "sess_agent", "agent:case-officer", proposed, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.conflicts) != 1 || store.dossiers["dos_session_proposal"].DistilledState.Body != body {
		t.Fatalf("session-end proposal was applied or lost: conflicts=%+v", store.conflicts)
	}
	if len(warnings) == 0 {
		t.Fatal("session-end proposal was not surfaced in warnings")
	}
}

func TestChangedDelegationAcceptanceOnlyDetectsHumanDecision(t *testing.T) {
	base := "## Delegation Contracts\n### Intake — owner: agent:case-officer\n- Scope: [decided] Triage.\n- Acceptance: [proposed] Awaiting human review.\n"
	for _, tc := range []struct {
		name, updated string
		want          bool
	}{
		{"still proposed", "- Acceptance: [proposed] Updated proposal.\n", false},
		{"human decision", "- Acceptance: [decided] Accepted against rev_123 by Alice.\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updated := "## Delegation Contracts\n### Intake — owner: agent:case-officer\n- Scope: [decided] Triage.\n" + tc.updated
			if got := changedDelegationAcceptance(base, updated); got != tc.want {
				t.Fatalf("changedDelegationAcceptance() = %t, want %t", got, tc.want)
			}
		})
	}
}
