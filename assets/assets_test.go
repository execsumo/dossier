package assets

import (
	"strings"
	"testing"
)

// TestDelegationContractsHeadingStaysInSyncAcrossGuideAndSkill guards a coupling
// nothing else enforces: dossier-delegate-skill.md instructs an agent to write a
// "## Delegation Contracts" section, and guide.md is where that section's schema
// (its seven block labels) is actually defined. Renaming the heading in one file
// without the other would silently break the skill — it would write a heading
// no reader, including the guide's own "check this section first" instruction,
// looks for.
func TestDelegationContractsHeadingStaysInSyncAcrossGuideAndSkill(t *testing.T) {
	const heading = "## Delegation Contracts"

	guide, err := FS.ReadFile("guide.md")
	if err != nil {
		t.Fatalf("failed to read guide.md: %v", err)
	}
	skill, err := FS.ReadFile("dossier-delegate-skill.md")
	if err != nil {
		t.Fatalf("failed to read dossier-delegate-skill.md: %v", err)
	}

	if !strings.Contains(string(guide), heading) {
		t.Errorf("guide.md no longer contains %q — dossier-delegate-skill.md instructs an agent to "+
			"write this exact heading, so a rename here must be mirrored in the skill", heading)
	}
	if !strings.Contains(string(skill), heading) {
		t.Errorf("dossier-delegate-skill.md no longer contains %q — this is the heading guide.md defines "+
			"the schema for, so a rename here must be mirrored in the guide", heading)
	}

	// The skill writes these seven labels, in this order, under the heading above.
	// guide.md is where the order and wording are defined; if guide.md changes a
	// label, the skill would keep writing the old one under the new schema.
	labels := []string{
		"Objective:",
		"Context:",
		"Success Criteria:",
		"Validation:",
		"Constraints:",
		"Decision Rights:",
		"Escalation:",
	}
	for _, label := range labels {
		if !strings.Contains(string(guide), label) {
			t.Errorf("guide.md no longer contains the Delegation Contracts block label %q", label)
		}
		if !strings.Contains(string(skill), label) {
			t.Errorf("dossier-delegate-skill.md no longer contains the Delegation Contracts block label %q — "+
				"it must write every label guide.md defines", label)
		}
	}
}

// TestReferencesAndMonitorsShareOneConvention guards the body contract that
// keeps ordinary external pointers distinct from live polling obligations while
// still making them easy for an agent to recognize and parse alike.
func TestReferencesAndMonitorsShareOneConvention(t *testing.T) {
	guideBytes, err := FS.ReadFile("guide.md")
	if err != nil {
		t.Fatalf("failed to read guide.md: %v", err)
	}
	instructionsBytes, err := FS.ReadFile("instructions.md")
	if err != nil {
		t.Fatalf("failed to read instructions.md: %v", err)
	}

	guide := string(guideBytes)
	instructions := string(instructionsBytes)
	for _, heading := range []string{"## References", "## Active Monitors"} {
		if !strings.Contains(guide, heading) {
			t.Errorf("guide.md no longer defines %q", heading)
		}
	}
	const lineFormat = "[<kind>: <label>](<URL>)"
	if strings.Count(guide, lineFormat) < 2 {
		t.Errorf("guide.md must define the shared link format for both sections")
	}
	if !strings.Contains(guide, "(Last polled: <YYYY-MM-DD>)") {
		t.Errorf("guide.md must require a last-polled date for monitors")
	}
	if !strings.Contains(instructions, "`## References` is navigational and must not be polled") {
		t.Errorf("instructions.md must explicitly keep References out of the polling loop")
	}
}
