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

// contractFieldLabels is the fixed, ordered schema every current contract
// carries (guide.md §4). Work-definition fields live once in the canonical
// Dossier body; a Delegation Contract contains only the person-specific terms.
var contractFieldLabels = []string{
	"Scope",
	"Acceptance",
	"Decision Rights",
	"Escalation",
	"Return Expectations",
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
	Header       string
	Label        string
	Owner        string
	AcceptedDate string
	// Legacy is true when the block uses the former seven-field schema. The
	// parser projects what it can into the current person-specific contract so
	// existing Dossiers stay visible without pretending migration is complete.
	Legacy bool
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
	contractHeaderRE             = regexp.MustCompile(`^###\s+(.+?)\s+—\s+owner:\s*([^,]*?)(?:,\s*(?:agreed|accepted)\s+(\S+))?(?:\s+\[src:|$)`)
	contractFieldRE              = regexp.MustCompile(
		`^-\s+(Scope|Acceptance|Decision Rights|Escalation|Return Expectations|Objective|Context|Success Criteria|Validation|Constraints):` +
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
			cur.Fields, cur.Legacy = normalizeContractFields(cur.Fields, cur.AcceptedDate)
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
				c.AcceptedDate = strings.TrimSpace(m[3])
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

// normalizeContractFields provides a read-only compatibility projection for
// the original seven-field contract. Its person-agnostic Objective becomes the
// closest available Scope signal and an agreed header supplies Acceptance.
// Context, Success Criteria, Validation, and Constraints remain in the source
// block for a human or agent to migrate into the canonical Dossier; they are
// not duplicated into the new contract model. Return Expectations stays
// missing so the readiness marker honestly surfaces what legacy contracts did
// not persist.
func normalizeContractFields(fields []ContractField, agreedDate string) ([]ContractField, bool) {
	legacy := false
	hasScope := false
	hasAcceptance := false
	var objective ContractField
	for _, field := range fields {
		switch field.Label {
		case "Scope":
			hasScope = true
		case "Acceptance":
			hasAcceptance = true
		case "Objective", "Context", "Success Criteria", "Validation", "Constraints":
			legacy = true
			if field.Label == "Objective" {
				objective = field
			}
		}
	}
	if !legacy {
		return fields, false
	}

	normalized := make([]ContractField, 0, len(fields)+2)
	for _, field := range fields {
		switch field.Label {
		case "Scope", "Acceptance", "Decision Rights", "Escalation", "Return Expectations":
			normalized = append(normalized, field)
		}
	}
	if !hasScope && objective.Label != "" {
		objective.Label = "Scope"
		objective.Text = "Legacy objective: " + objective.Text
		normalized = append(normalized, objective)
	}
	if !hasAcceptance && agreedDate != "" {
		normalized = append(normalized, ContractField{
			Label:  "Acceptance",
			Status: ContractFieldDecided,
			Text:   "Accepted " + agreedDate + "; baseline revision not recorded (legacy contract).",
		})
	}
	return normalized, true
}

// HasOpenDelegationContract reports whether body's Delegation Contracts
// section (if any) contains at least one contract with a field that isn't yet
// [decided] — the cheap per-dossier attention signal a list surface (TUI
// dashboard/board) can show without opening the dossier, computed from a body
// the caller already had in hand rather than a second read.
func HasOpenDelegationContract(body string) bool {
	for _, c := range ParseDelegationContracts(body) {
		if !c.Complete() {
			return true
		}
	}
	return false
}

// orderContractFields returns fields reordered (and gap-filled) to match
// contractFieldLabels exactly, so a caller can always render or index the
// same positions regardless of how the source text was written.
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
