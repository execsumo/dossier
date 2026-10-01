package core

import "testing"

func TestDelegationContractAgentOwner(t *testing.T) {
	body := "## Delegation Contracts\n" +
		"### Shoring execution — owner: agent:case-officer, accepted 2026-10-01 [src:art_evidence]\n" +
		"- Scope: [decided] Own the execution deliverable.\n" +
		"- Acceptance: [decided] Human accepted against rev_abc.\n"
	contracts := ParseDelegationContracts(body)
	if len(contracts) != 1 || contracts[0].Owner != "agent:case-officer" || contracts[0].OwnerKind != "agent" || contracts[0].OwnerID != "case-officer" {
		t.Fatalf("agent owner not parsed: %+v", contracts)
	}
}
