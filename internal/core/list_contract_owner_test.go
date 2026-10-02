package core

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

const (
	openContractBody = "## Delegation Contracts\n" +
		"### Copy review — owner: Priya\n" +
		"- Scope: [decided] Own the copy review.\n" +
		"- Acceptance: [proposed] Awaiting Priya.\n" +
		"- Decision Rights: [decided] Typos.\n" +
		"- Escalation: [decided] Flag figures.\n" +
		"- Return Expectations: [decided] Reply.\n"
	acceptedContractBody = "## Delegation Contracts\n" +
		"### Outreach — owner: Priya, accepted 2026-09-08\n" +
		"- Scope: [decided] Own outreach.\n" +
		"- Acceptance: [decided] Accepted 2026-09-08 against rev_1.\n" +
		"- Decision Rights: [decided] Reschedule.\n" +
		"- Escalation: [decided] Discounts.\n" +
		"- Return Expectations: [decided] Reply.\n"
)

func TestListLeadFilterAlsoMatchesContractOwners(t *testing.T) {
	store := &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster: Roster{Manager: "hgill", Members: map[string]string{
			"hgill":  "Herwin Gill",
			"psmith": "Priya Shah",
		}, Former: map[string]string{}},
	}
	add := func(id, slug, lead, body string) {
		store.dossiers[id] = &Dossier{
			Frontmatter:    Frontmatter{ID: id, Slug: slug, Name: slug, Status: StatusExecute, Lead: lead},
			DistilledState: DistilledState{Body: body},
		}
	}
	add("dos_lead", "lead-only", "psmith", "# State\n")
	add("dos_open", "contract-open", "hgill", openContractBody)
	add("dos_accepted", "contract-accepted", "hgill", acceptedContractBody)
	add("dos_both", "lead-and-contract", "psmith", openContractBody)
	add("dos_other", "someone-else", "hgill", "# State\n")

	list := func(t *testing.T, author, lead string) map[string][]string {
		t.Helper()
		svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: author}, nil)
		res, err := svc.List(context.Background(), ListReq{Lead: lead})
		if err != nil {
			t.Fatalf("List(%q): %v", lead, err)
		}
		got := map[string][]string{}
		for _, item := range res.Data.([]ListItem) {
			got[item.Slug] = item.MatchedAs
		}
		return got
	}

	want := map[string][]string{
		"lead-only":         {"lead"},
		"contract-open":     {"contract (open)"},
		"contract-accepted": {"contract (accepted)"},
		"lead-and-contract": {"lead", "contract (open)"},
	}
	for _, tc := range []struct{ name, author, lead string }{
		{"me", "psmith", "me"},
		{"username", "hgill", "psmith"},
		{"display name", "hgill", "Priya Shah"},
		{"first name", "hgill", "Priya"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := list(t, tc.author, tc.lead); !reflect.DeepEqual(got, want) {
				t.Fatalf("matches = %v, want %v", got, want)
			}
		})
	}

	t.Run("no filter reports no reasons", func(t *testing.T) {
		got := list(t, "psmith", "")
		if len(got) != 5 {
			t.Fatalf("unfiltered list has %d dossiers, want 5", len(got))
		}
		for slug, matched := range got {
			if len(matched) != 0 {
				t.Fatalf("%s MatchedAs = %v, want none without a filter", slug, matched)
			}
		}
	})

	t.Run("contract owners are named", func(t *testing.T) {
		svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "hgill"}, nil)
		res, err := svc.List(context.Background(), ListReq{})
		if err != nil {
			t.Fatal(err)
		}
		var owners []string
		for _, item := range res.Data.([]ListItem) {
			if item.Slug == "contract-open" {
				owners = item.ContractOwners
			}
		}
		sort.Strings(owners)
		if !reflect.DeepEqual(owners, []string{"Priya Shah"}) {
			t.Fatalf("ContractOwners = %v, want [Priya Shah]", owners)
		}
	})

	t.Run("a non-owner sees neither contract", func(t *testing.T) {
		store.roster.Members["marco"] = "Marco Diaz"
		got := list(t, "hgill", "marco")
		if len(got) != 0 {
			t.Fatalf("marco matches = %v, want none", got)
		}
	})
}

func TestListLeadFilterResolvesRegisteredAgentContractOwners(t *testing.T) {
	body := "## Delegation Contracts\n" +
		"### Intake — owner: agent:case-officer\n" +
		"- Scope: [decided] Triage the intake.\n" +
		"- Acceptance: [proposed] Awaiting human review.\n"
	store := &rosterTestStore{
		localFakeStore: newLocalFakeStore(),
		roster: Roster{
			Manager: "hgill",
			Members: map[string]string{"hgill": "Herwin Gill", "case-officer": "Case Officer"},
			Kinds:   map[string]string{"case-officer": "agent"},
			Former:  map[string]string{},
		},
	}
	store.dossiers["dos_agent_contract"] = &Dossier{
		Frontmatter:    Frontmatter{ID: "dos_agent_contract", Slug: "agent-contract", Name: "Agent contract", Status: StatusExecute},
		DistilledState: DistilledState{Body: body},
	}

	for _, query := range []string{"case-officer", "Case Officer", "Case"} {
		svc := NewService(store, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, Config{Author: "hgill"}, nil)
		res, err := svc.List(context.Background(), ListReq{Lead: query})
		if err != nil {
			t.Fatalf("List(%q): %v", query, err)
		}
		items := res.Data.([]ListItem)
		if len(items) != 1 || !reflect.DeepEqual(items[0].MatchedAs, []string{"contract (open)"}) {
			t.Fatalf("List(%q) = %+v, want one open contract match", query, items)
		}
		if !reflect.DeepEqual(items[0].ContractOwners, []string{"Case Officer (agent)"}) {
			t.Fatalf("ContractOwners = %v, want agent-kind display name", items[0].ContractOwners)
		}
		if !reflect.DeepEqual(items[0].ContractAssignments, []ContractAssignment{{Owner: "Case Officer (agent)", State: "open"}}) {
			t.Fatalf("ContractAssignments = %+v, want agent owner and open state", items[0].ContractAssignments)
		}
	}
}
