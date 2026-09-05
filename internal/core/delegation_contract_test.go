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

func TestParseDelegationContracts_FullyDecided(t *testing.T) {
	body := "" +
		"## Delegation Contracts\n" +
		"### Pricing page copy review — owner: Priya, agreed 2026-06-30 [src:art_01jz8pricing_sheet]\n" +
		"- Objective: [decided] Pricing page copy is factually correct and ready to\n" +
		"  publish.\n" +
		"- Context: [decided] Copy follows the pricing decision.\n" +
		"- Success Criteria: [decided] Every figure matches the sheet.\n" +
		"- Validation: [decided] Priya diffs the copy against the sheet.\n" +
		"- Constraints: [decided] Don't touch layout/design — copy only.\n" +
		"- Decision Rights: [decided] Priya fixes typos unilaterally.\n" +
		"- Escalation: [decided] Flag and move on rather than blocking.\n" +
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
	if c.AgreedDate != "2026-06-30" {
		t.Errorf("AgreedDate = %q", c.AgreedDate)
	}
	if !c.Complete() {
		t.Errorf("expected Complete() true, open fields: %v", c.OpenFields())
	}
	if len(c.Fields) != len(contractFieldLabels) {
		t.Fatalf("expected %d fields, got %d", len(contractFieldLabels), len(c.Fields))
	}
	wantObjective := "Pricing page copy is factually correct and ready to publish."
	if got := c.Fields[0].Text; got != wantObjective {
		t.Errorf("Objective text = %q, want %q (continuation line should be joined)", got, wantObjective)
	}
}

func TestParseDelegationContracts_MixedAndMissingFields(t *testing.T) {
	body := "" +
		"## Delegation Contracts\n" +
		"### Q3 renewal outreach — owner: Marco, agreed 2026-07-01\n" +
		"- Objective: [decided] Renewal outreach sent to all at-risk accounts.\n" +
		"- Context: [decided] Follows the churn-risk model output.\n" +
		"- Success Criteria: [proposed] Not yet discussed.\n" +
		"- Validation: [proposed] Not yet discussed.\n" +
		"- Constraints: [decided] Do not discount below list without sign-off.\n" +
		"- Decision Rights: [decided] Marco can reschedule calls unilaterally.\n" +
		"\n## Next Steps\nFollow up.\n"

	contracts := ParseDelegationContracts(body)
	if len(contracts) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(contracts))
	}
	c := contracts[0]
	if c.Complete() {
		t.Fatalf("expected Complete() false")
	}
	want := []string{"Success Criteria", "Validation", "Escalation"}
	if got := c.OpenFields(); !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenFields() = %v, want %v", got, want)
	}

	var escalation ContractField
	for _, f := range c.Fields {
		if f.Label == "Escalation" {
			escalation = f
		}
	}
	if escalation.Status != ContractFieldMissing {
		t.Errorf("Escalation status = %q, want %q (bullet was never written)", escalation.Status, ContractFieldMissing)
	}
}

func TestParseDelegationContracts_UntaggedField(t *testing.T) {
	body := "" +
		"## Delegation Contracts\n" +
		"### Vendor eval — owner: Sam, agreed 2026-08-01\n" +
		"- Objective: [decided] Choose a vendor by end of quarter.\n" +
		"- Context: [decided] Three vendors shortlisted.\n" +
		"- Success Criteria: [decided] Scorecard total above 80.\n" +
		"- Validation: Sam and I both review the scorecard.\n" +
		"- Constraints: [decided] Budget cap $50k/yr.\n" +
		"- Decision Rights: [decided] Sam picks among vendors scoring above 80.\n" +
		"- Escalation: [decided] If none score above 80, escalate to me.\n"

	contracts := ParseDelegationContracts(body)
	if len(contracts) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(contracts))
	}
	c := contracts[0]
	if c.Complete() {
		t.Fatalf("expected Complete() false for an untagged field")
	}
	var validation ContractField
	for _, f := range c.Fields {
		if f.Label == "Validation" {
			validation = f
		}
	}
	if validation.Status != ContractFieldUntagged {
		t.Errorf("Validation status = %q, want %q", validation.Status, ContractFieldUntagged)
	}
	if validation.Text != "Sam and I both review the scorecard." {
		t.Errorf("Validation text = %q", validation.Text)
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
			"- Objective: [decided] X.\n" +
			"- Context: [decided] X.\n" +
			"- Success Criteria: [decided] X.\n" +
			"- Validation: [decided] X.\n" +
			"- Constraints: [decided] X.\n" +
			"- Decision Rights: [decided] X.\n" +
			"- Escalation: [decided] X.\n", false},
		{"one proposed field", "" +
			"## Delegation Contracts\n" +
			"### Task — owner: A, agreed 2026-01-01\n" +
			"- Objective: [decided] X.\n" +
			"- Context: [decided] X.\n" +
			"- Success Criteria: [decided] X.\n" +
			"- Validation: [decided] X.\n" +
			"- Constraints: [decided] X.\n" +
			"- Decision Rights: [decided] X.\n" +
			"- Escalation: [proposed] Not yet discussed.\n", true},
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
		"- Objective: [decided] Do the first thing.\n" +
		"- Context: [decided] Context one.\n" +
		"- Success Criteria: [decided] Criteria one.\n" +
		"- Validation: [decided] Validation one.\n" +
		"- Constraints: [decided] Constraint one.\n" +
		"- Decision Rights: [decided] Rights one.\n" +
		"- Escalation: [decided] Escalation one.\n" +
		"### Second — owner: B, agreed 2026-01-02\n" +
		"- Objective: [proposed] Do the second thing.\n" +
		"- Context: [decided] Context two.\n" +
		"- Success Criteria: [decided] Criteria two.\n" +
		"- Validation: [decided] Validation two.\n" +
		"- Constraints: [decided] Constraint two.\n" +
		"- Decision Rights: [decided] Rights two.\n" +
		"- Escalation: [decided] Escalation two.\n" +
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
		t.Errorf("expected second contract incomplete")
	}
	if want := []string{"Objective"}; !reflect.DeepEqual(contracts[1].OpenFields(), want) {
		t.Errorf("second contract OpenFields() = %v, want %v", contracts[1].OpenFields(), want)
	}
}
