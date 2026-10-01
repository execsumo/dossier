package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHumanView(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no refs untouched", "plain text", "plain text"},
		{"citation and its leading space removed", "Launch slipped [src:art_1#L1-L2]. Next.", "Launch slipped. Next."},
		{"adjacent citations", "x [src:art_1][src:art_2] y", "x y"},
		{"fenced code untouched", "```\n[src:art_1]\n```", "```\n[src:art_1]\n```"},
		{"evidence section dropped to next heading",
			"## Decisions\n- a [src:art_1]\n\n## Evidence\nIndex of the Archive.\n- `art_1` (transcript, 9 lines)\n\n## Open Questions\n- q\n",
			"## Decisions\n- a\n\n## Open Questions\n- q\n"},
		{"evidence section at end of body", "## Findings\nf\n\n## Evidence\n- `art_1`\n", "## Findings\nf\n"},
		{"fence inside evidence does not end it", "## Evidence\n```\n## Not a heading\n```\n## Next\nkeep\n", "## Next\nkeep\n"},
		{"similarly named heading kept", "## Evidence Review\nkeep [src:art_1]\n", "## Evidence Review\nkeep\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HumanView(tt.in); got != tt.want {
				t.Errorf("HumanView(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
	if got := HumanView("a [src:x] b"); strings.Contains(got, "Sources") || strings.Contains(got, "---") {
		t.Errorf("no footnote list should be appended: %q", got)
	}
}

func TestRecallHumanViewOmitsUncitedAdvisory(t *testing.T) {
	store := newLocalFakeStore()
	now := time.Now().Truncate(time.Second)
	d := &Dossier{Frontmatter: Frontmatter{
		ID: "dos_hv", Name: "Human view", Slug: "human-view",
		CreatedAt: now, UpdatedAt: now, Status: StatusSpark, Priority: PriorityMedium,
	}, DistilledState: DistilledState{Body: "# State\n"}}
	store.dossiers[d.Frontmatter.ID] = d
	store.revisions[d.Frontmatter.ID] = CalculateRevision(d.Frontmatter, d.DistilledState.Body, nil)
	store.artifacts[d.Frontmatter.ID] = []Artifact{{ID: "art_uncited", DossierID: d.Frontmatter.ID, Title: "t", Lines: 3}}
	svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{now: now}, Config{Author: "hgill"}, nil)

	hasAdvisory := func(res Result) bool {
		for _, w := range res.Warnings {
			if strings.Contains(string(w), "not cited by the Distilled State") {
				return true
			}
		}
		return false
	}
	agent, err := svc.Recall(context.Background(), RecallReq{ID: "dos_hv"})
	if err != nil || !hasAdvisory(agent) {
		t.Fatalf("agent recall must keep the uncited advisory: err=%v warnings=%v", err, agent.Warnings)
	}
	human, err := svc.Recall(context.Background(), RecallReq{ID: "dos_hv", HumanView: true})
	if err != nil || hasAdvisory(human) {
		t.Fatalf("human recall must omit the uncited advisory: err=%v warnings=%v", err, human.Warnings)
	}
	if len(human.Data.(RecallResult).Artifacts) != 1 {
		t.Fatalf("human recall must still carry the artifact index")
	}
}
