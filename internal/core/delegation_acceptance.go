package core

import "strings"

// changedDelegationAcceptance detects an attempted decision of the Acceptance
// field. Other contract edits remain writable by agents, but a transition to
// or change of a decided acceptance is a human-only act.
func changedDelegationAcceptance(before, after string) bool {
	old := delegationAcceptances(before)
	for key, accepted := range delegationAcceptances(after) {
		if accepted.Status != ContractFieldDecided {
			continue
		}
		previous, exists := old[key]
		if !exists || previous.Status != ContractFieldDecided || strings.TrimSpace(previous.Text) != strings.TrimSpace(accepted.Text) {
			return true
		}
	}
	return false
}

func delegationAcceptances(body string) map[string]ContractField {
	contracts := ParseDelegationContracts(body)
	result := make(map[string]ContractField, len(contracts))
	for _, contract := range contracts {
		key := strings.TrimSpace(contract.Label) + "\x00" + strings.TrimSpace(contract.Owner)
		for _, field := range contract.Fields {
			if field.Label == "Acceptance" {
				result[key] = field
				break
			}
		}
	}
	return result
}
