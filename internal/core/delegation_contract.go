package core

import (
	"regexp"
	"strings"
)

// ContractFieldStatus reports whether a Delegation Contract field is settled.
// It is derived mechanically from the `[decided]`/`[proposed]` tag guide.md
// §4 requires on every field — never inferred or self-reported, so it can't
// drift the way a hand-set "done" checkbox would.
type ContractFieldStatus string

const (
	ContractFieldDecided ContractFieldStatus = "decided"
	// ContractFieldProposed mirrors guide.md's "still under discussion" tag.
	ContractFieldProposed ContractFieldStatus = "proposed"
	// ContractFieldMissing means the block never wrote this bullet at all —
	// a schema violation guide.md disallows, but the checklist surfaces it
	// as "still open" rather than failing to render.
	ContractFieldMissing ContractFieldStatus = "missing"
	// ContractFieldUntagged means the bullet is present but its tag is absent
	// or not one of `[decided]`/`[proposed]` — also a schema violation,
	// treated as open until fixed.
	ContractFieldUntagged ContractFieldStatus = "untagged"
)

// contractFieldLabels is the fixed, ordered schema every contract block
// carries (guide.md §4, in the order a triage decision is actually made).
var contractFieldLabels = []string{
	"Objective",
	"Context",
	"Success Criteria",
	"Validation",
	"Constraints",
	"Decision Rights",
	"Escalation",
}

// ContractField is one bullet of a Delegation Contract block.
type ContractField struct {
	Label  string
	Status ContractFieldStatus
	Text   string
}

// DelegationContract is one `###` block under `## Delegation Contracts`.
type DelegationContract struct {
	// Header is the raw, unparsed `###` line, kept so a caller can render it
	// verbatim even when it doesn't match the "owner: ..., agreed ..." shape.
	Header     string
	Label      string
	Owner      string
	AgreedDate string
	// Fields is always exactly len(contractFieldLabels) long and in schema
	// order, regardless of the order or completeness of the source text.
	Fields []ContractField
}

// Complete reports whether every field in the contract is [decided].
func (c DelegationContract) Complete() bool {
	for _, f := range c.Fields {
		if f.Status != ContractFieldDecided {
			return false
		}
	}
	return true
}

// OpenFields returns the labels of fields that are not yet [decided], in
// schema order — what a reader still needs to settle before handoff.
func (c DelegationContract) OpenFields() []string {
	var open []string
	for _, f := range c.Fields {
		if f.Status != ContractFieldDecided {
			open = append(open, f.Label)
		}
	}
	return open
}

var (
	// delegationContractsHeadingRE matches the reserved heading only when it
	// is exactly this text: guide.md §4 requires it "fixed, not templated"
	// so nothing downstream (including this parser) has to guess variants.
	delegationContractsHeadingRE = regexp.MustCompile(`(?m)^##\s+Delegation Contracts\s*$`)
	contractHeaderRE             = regexp.MustCompile(`^###\s+(.+?)\s+—\s+owner:\s*([^,]*),\s*agreed\s+(\S+)`)
	contractFieldRE              = regexp.MustCompile(
		`^-\s+(Objective|Context|Success Criteria|Validation|Constraints|Decision Rights|Escalation):` +
			`\s*(?:\[(decided|proposed)\]\s*)?(.*)$`)
)

// ParseDelegationContracts extracts every `###` block under the Distilled
// State's `## Delegation Contracts` section (guide.md §4). It is a pure,
// best-effort reader: a malformed header, an untagged field, or an omitted
// bullet is reported on the parsed contract rather than rejected outright —
// this feeds a display checklist, not a save-time validator, and a leader
// checking status must see "what's still open" even on a half-written
// contract.
func ParseDelegationContracts(body string) []DelegationContract {
	loc := delegationContractsHeadingRE.FindStringIndex(body)
	if loc == nil {
		return nil
	}
	section := body[loc[1]:]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}

	var contracts []DelegationContract
	var cur *DelegationContract
	var curField *ContractField

	flushField := func() {
		if cur != nil && curField != nil {
			curField.Text = strings.TrimSpace(curField.Text)
			cur.Fields = append(cur.Fields, *curField)
			curField = nil
		}
	}
	flushContract := func() {
		flushField()
		if cur != nil {
			cur.Fields = orderContractFields(cur.Fields)
			contracts = append(contracts, *cur)
			cur = nil
		}
	}

	for _, rawLine := range strings.Split(section, "\n") {
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "### "):
			flushContract()
			c := DelegationContract{Header: line}
			if m := contractHeaderRE.FindStringSubmatch(line); m != nil {
				c.Label = strings.TrimSpace(m[1])
				c.Owner = strings.TrimSpace(m[2])
				c.AgreedDate = strings.TrimSpace(m[3])
			} else {
				c.Label = strings.TrimSpace(strings.TrimPrefix(line, "###"))
			}
			cur = &c
		case cur == nil:
			continue
		default:
			if m := contractFieldRE.FindStringSubmatch(line); m != nil {
				flushField()
				status := ContractFieldUntagged
				if m[2] != "" {
					status = ContractFieldStatus(m[2])
				}
				curField = &ContractField{Label: m[1], Status: status, Text: m[3]}
				continue
			}
			// A continuation line of the current field's wrapped text.
			if curField != nil && line != "" {
				curField.Text += " " + line
			}
		}
	}
	flushContract()
	return contracts
}

// orderContractFields returns fields reordered (and gap-filled) to match
// contractFieldLabels exactly, so a caller can always render or index the
// same seven positions regardless of how the source text was written.
func orderContractFields(fields []ContractField) []ContractField {
	byLabel := make(map[string]ContractField, len(fields))
	for _, f := range fields {
		byLabel[f.Label] = f
	}
	ordered := make([]ContractField, 0, len(contractFieldLabels))
	for _, label := range contractFieldLabels {
		if f, ok := byLabel[label]; ok {
			ordered = append(ordered, f)
		} else {
			ordered = append(ordered, ContractField{Label: label, Status: ContractFieldMissing})
		}
	}
	return ordered
}
