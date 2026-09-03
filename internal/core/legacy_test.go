package core

import (
	"strings"
	"testing"
)

func TestMergeLegacyOpenQuestions(t *testing.T) {
	t.Run("inserts before current state and removes duplicates", func(t *testing.T) {
		body := "# Topic\n\n## Findings\n\nKnown.\n\n## Current State\n\nActive.\n"
		got := MergeLegacyOpenQuestions(body, []string{"First question?", "First question?", "Second question?"})
		wantSection := "## Open Questions\n\n- First question?\n- Second question?\n\n## Current State"
		if !strings.Contains(got, wantSection) {
			t.Fatalf("merged body missing canonical section:\n%s", got)
		}
		if strings.Count(got, "First question?") != 1 {
			t.Fatalf("duplicate legacy question survived:\n%s", got)
		}
	})

	t.Run("merges into existing section semantically", func(t *testing.T) {
		body := "# Topic\n\n## Open Questions\n\n- Already here?\n1. Numbered too?\n\n## Current State\n"
		got := MergeLegacyOpenQuestions(body, []string{" already   HERE? ", "Numbered too?", "- New question?"})
		if strings.Count(strings.ToLower(got), "already here?") != 1 {
			t.Fatalf("existing bullet duplicated:\n%s", got)
		}
		if strings.Count(got, "Numbered too?") != 1 {
			t.Fatalf("existing numbered item duplicated:\n%s", got)
		}
		if !strings.Contains(got, "- New question?\n\n## Current State") {
			t.Fatalf("new question not merged into existing section:\n%s", got)
		}
	})

	t.Run("no legacy questions leaves body byte-identical", func(t *testing.T) {
		body := "# Topic\r\n\r\nText\r\n"
		if got := MergeLegacyOpenQuestions(body, nil); got != body {
			t.Fatalf("body changed without legacy questions: %q", got)
		}
	})
}
