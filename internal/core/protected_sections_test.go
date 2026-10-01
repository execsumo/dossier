package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAgentProtectedSectionSaveBecomesHumanResolvedProposal(t *testing.T) {
	fake := newLocalFakeStore()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	body := "# Topic\n\n## Objective\nShip the accepted scope.\n\n## Situation\nOld state.\n"
	fake.dossiers["dos_proposal"] = &Dossier{Frontmatter: Frontmatter{ID: "dos_proposal", Name: "Topic", Slug: "topic", Status: StatusExecute, Priority: PriorityMedium, CreatedAt: now, UpdatedAt: now}, DistilledState: DistilledState{Body: body}}
	svc := NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "vm"}, nil)
	proposed := strings.Replace(body, "Ship the accepted scope.", "Ship expanded scope.", 1)
	res, err := svc.Save(context.Background(), SaveReq{ID: "dos_proposal", Actor: "agent:case-officer", DistilledStateMarkdown: proposed})
	if err != nil {
		t.Fatal(err)
	}
	data := res.Data.(map[string]any)
	proposalID := data["proposal_id"].(string)
	if data["applied"] != false || fake.dossiers["dos_proposal"].DistilledState.Body != body {
		t.Fatalf("proposal was applied: result=%+v body=%q", data, fake.dossiers["dos_proposal"].DistilledState.Body)
	}
	if len(fake.conflicts) != 1 || fake.conflicts[proposalID].Kind != "agent_proposal" {
		t.Fatalf("agent proposal conflict not preserved: %+v", fake.conflicts)
	}
	if _, err := svc.ResolveConflict(context.Background(), ResolveConflictReq{Actor: "agent:case-officer", ConflictID: proposalID, Choice: ConflictChoiceRestoreMine, Author: "vm"}); err == nil {
		t.Fatal("agent accepted its own protected-section proposal")
	}
	if _, err := svc.ResolveConflict(context.Background(), ResolveConflictReq{Actor: "human:alice", ConflictID: proposalID, Choice: ConflictChoiceRestoreMine, Author: "vm"}); err != nil {
		t.Fatalf("human could not accept proposal: %v", err)
	}
	if !strings.Contains(fake.dossiers["dos_proposal"].DistilledState.Body, "Ship expanded scope.") {
		t.Fatal("human acceptance did not apply proposed body")
	}
}

func TestChangedProtectedSections(t *testing.T) {
	before := "## Objective\nA\n\n## Situation\nX\n"
	for _, tc := range []struct {
		body string
		want []string
	}{
		{body: "## Objective\nA\n\n## Situation\nY\n"},
		{body: "## Objective\nB\n\n## Situation\nX\n", want: []string{"Objective"}},
		{body: "## Objective\nA\n\n## Decisions\nD\n\n## Situation\nX\n", want: []string{"Decisions"}},
	} {
		got := changedProtectedSections(before, tc.body)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Fatalf("changed protected sections = %v, want %v", got, tc.want)
		}
	}
}
