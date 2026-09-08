package core

import (
	"reflect"
	"testing"
)

func TestParseDelegationContracts_NoSection(t *testing.T) {
	body := "# Topic\n\n## Situation\nSomething.\n"
	if got := ParseDelegationContracts(body); got != nil {
		t.Fatalf("expected nil for a body with no Delegation Contracts section, got %#v", got)
	}
}

func TestParseDelegationContracts_CurrentFullyDecided(t *testing.T) {
	body := "" +
		"## Delegation Contracts\n" +
		"### Pricing page copy review — owner: Priya, accepted 2026-09-08 [src:art_01jz8pricing_sheet]\n" +
		"- Scope: [decided] Own the Copy review deliverable.\n" +
		"- Acceptance: [decided] Accepted 2026-09-08 against rev_123; due 2026-09-12.\n" +
		"- Decision Rights: [decided] Priya fixes typos unilaterally.\n" +
		"- Escalation: [decided] Flag contradictory source figures and continue with the next page.\n" +
		"- Return Expectations: [decided] Reply with status, validation result, output link, and decisions made.\n" +
		"\n## Next Steps\nShip it.\n"

	contracts := ParseDelegationContracts(body)
	if len(contracts) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(contracts))
	}
	c := contracts[0]
	if c.Label != "Pricing page copy review" {
		t.Errorf("Label = %q", c.Label)
	}
	if c.Owner != "Priya" {
		t.Errorf("Owner = %q", c.Owner)
	}
	if c.AcceptedDate != "2026-09-08" {
		t.Errorf("AcceptedDate = %q", c.AcceptedDate)
	}
	if c.Legacy {
		t.Fatal("current contract reported as legacy")
	}
	if !c.Complete() {
		t.Errorf("expected Complete() true, open fields: %v", c.OpenFields())
	}
	if len(c.Fields) != len(contractFieldLabels) {
		t.Fatalf("expected %d fields, got %d", len(contractFieldLabels), len(c.Fields))
	}
}

func TestParseDelegationContracts_MixedAndMissingFields(t *testing.T) {
	body := "" +
		"## Delegation Contracts\n" +
		"### Q3 renewal outreach — owner: Marco\n" +
		"- Scope: [decided] Own the renewal-outreach deliverable.\n" +
		"- Acceptance: [proposed] Awaiting Marco's response.\n" +
		"- Decision Rights: [decided] Marco can reschedule calls unilaterally.\n" +
		"- Escalation: [decided] Escalate discounts below list.\n" +
		"\n## Next Steps\nFollow up.\n"

	contracts := ParseDelegationContracts(body)
	if len(contracts) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(contracts))
	}
	c := contracts[0]
	if c.Owner != "Marco" || c.AcceptedDate != "" {
		t.Fatalf("header parsed owner=%q date=%q, want Marco with no accepted date", c.Owner, c.AcceptedDate)
	}
	if c.Complete() {
		t.Fatal("expected Complete() false")
	}
	want := []string{"Acceptance", "Return Expectations"}
	if got := c.OpenFields(); !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenFields() = %v, want %v", got, want)
	}
	if c.Fields[4].Status != ContractFieldMissing {
		t.Errorf("Return Expectations status = %q, want %q", c.Fields[4].Status, ContractFieldMissing)
	}
}

func TestParseDelegationContracts_UntaggedField(t *testing.T) {
	body := "" +
		"## Delegation Contracts\n" +
		"### Vendor eval — owner: Sam, agreed 2026-09-08\n" +
		"- Scope: [decided] Own the vendor evaluation.\n" +
		"- Acceptance: Sam accepted against rev_123.\n" +
		"- Decision Rights: [decided] Sam picks among qualifying vendors.\n" +
		"- Escalation: [decided] Escalate if none qualify.\n" +
		"- Return Expectations: [decided] Return the scorecard and validation notes.\n"

	c := ParseDelegationContracts(body)[0]
	if c.Complete() {
		t.Fatal("expected Complete() false for an untagged field")
	}
	if c.Fields[1].Status != ContractFieldUntagged {
		t.Errorf("Acceptance status = %q, want %q", c.Fields[1].Status, ContractFieldUntagged)
	}
}

func TestParseDelegationContracts_LegacyProjection(t *testing.T) {
	body := "" +
		"## Delegation Contracts\n" +
		"### Pricing page copy review — owner: Priya, agreed 2026-06-30\n" +
		"- Objective: [decided] Pricing page copy is ready to publish.\n" +
		"- Context: [decided] Copy follows the pricing decision.\n" +
		"- Success Criteria: [decided] Every figure matches the sheet.\n" +
		"- Validation: [decided] Diff copy against the sheet.\n" +
		"- Constraints: [decided] Do not touch layout.\n" +
		"- Decision Rights: [decided] Priya fixes typos.\n" +
		"- Escalation: [decided] Flag contradictory figures.\n"

	c := ParseDelegationContracts(body)[0]
	if !c.Legacy {
		t.Fatal("legacy contract was not identified")
	}
	if c.Complete() {
		t.Fatal("legacy contract without Return Expectations must remain open")
	}
	if got := c.OpenFields(); !reflect.DeepEqual(got, []string{"Return Expectations"}) {
		t.Fatalf("OpenFields() = %v, want Return Expectations", got)
	}
	if c.Fields[0].Label != "Scope" || c.Fields[0].Text != "Legacy objective: Pricing page copy is ready to publish." {
		t.Errorf("legacy objective was not projected to Scope: %+v", c.Fields[0])
	}
	if c.Fields[1].Label != "Acceptance" || c.Fields[1].Status != ContractFieldDecided {
		t.Errorf("legacy agreed date was not projected to Acceptance: %+v", c.Fields[1])
	}
}

func TestHasOpenDelegationContract(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"no section", "## Situation\nNothing here.\n", false},
		{"fully decided", "" +
			"## Delegation Contracts\n" +
			"### Task — owner: A, agreed 2026-01-01\n" +
			"- Scope: [decided] Entire Dossier.\n" +
			"- Acceptance: [decided] Accepted against rev_123.\n" +
			"- Decision Rights: [decided] X.\n" +
			"- Escalation: [decided] X.\n" +
			"- Return Expectations: [decided] X.\n", false},
		{"one proposed field", "" +
			"## Delegation Contracts\n" +
			"### Task — owner: A, agreed 2026-01-01\n" +
			"- Scope: [decided] Entire Dossier.\n" +
			"- Acceptance: [decided] Accepted against rev_123.\n" +
			"- Decision Rights: [decided] X.\n" +
			"- Escalation: [proposed] Not yet discussed.\n" +
			"- Return Expectations: [decided] X.\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasOpenDelegationContract(tc.body); got != tc.want {
				t.Errorf("HasOpenDelegationContract() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseDelegationContracts_MultipleContractsAndSectionBoundary(t *testing.T) {
	body := "" +
		"## Situation\nBackground.\n" +
		"\n## Delegation Contracts\n" +
		"### First — owner: A, agreed 2026-01-01\n" +
		"- Scope: [decided] First deliverable.\n" +
		"- Acceptance: [decided] Accepted against rev_1.\n" +
		"- Decision Rights: [decided] X.\n" +
		"- Escalation: [decided] X.\n" +
		"- Return Expectations: [decided] X.\n" +
		"### Second — owner: B, agreed 2026-01-02\n" +
		"- Scope: [decided] Second deliverable.\n" +
		"- Acceptance: [proposed] Awaiting reply.\n" +
		"- Decision Rights: [decided] X.\n" +
		"- Escalation: [decided] X.\n" +
		"- Return Expectations: [decided] X.\n" +
		"\n## Next Steps\n" +
		"- This line must not be parsed as a contract field.\n"

	contracts := ParseDelegationContracts(body)
	if len(contracts) != 2 {
		t.Fatalf("expected 2 contracts, got %d", len(contracts))
	}
	if !contracts[0].Complete() {
		t.Errorf("expected first contract complete, open: %v", contracts[0].OpenFields())
	}
	if contracts[1].Complete() {
		t.Error("expected second contract incomplete")
	}
	if want := []string{"Acceptance"}; !reflect.DeepEqual(contracts[1].OpenFields(), want) {
		t.Errorf("second contract OpenFields() = %v, want %v", contracts[1].OpenFields(), want)
	}
}
