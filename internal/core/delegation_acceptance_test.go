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
