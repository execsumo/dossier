package core

import "testing"

func TestConflictBodyStripsDossierEnvelope(t *testing.T) {
	content := "---\nid: dos_test\nname: Test\n---\n# Distilled State\n\nMine\n"
	if got := conflictBody(content); got != "# Distilled State\n\nMine\n" {
		t.Fatalf("conflictBody = %q", got)
	}
}

func TestConflictBodyKeepsPlainMarkdown(t *testing.T) {
	content := "Mine\n"
	if got := conflictBody(content); got != content {
		t.Fatalf("conflictBody changed plain markdown to %q", got)
	}
}
