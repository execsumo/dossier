package store

import (
	"strings"
	"testing"

	"dossier/internal/core"
)

func TestParseAndFormatCanonicalFrontmatter(t *testing.T) {
	content := `---
id: dos_example
name: Example
slug: example
description: A progressive disclosure summary
created_at: 2026-01-01T00:00:00Z
updated_at: 2026-01-01T00:00:00Z
status: active
priority: high
next_action: inspect
---
# Example

## Open Questions
- What remains to decide?
`

	fm, body, err := ParseDossierFile(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if fm.Priority != core.PriorityHigh || fm.Description != "A progressive disclosure summary" {
		t.Fatalf("unexpected frontmatter: %+v", fm)
	}
	if !strings.Contains(body, "## Open Questions") {
		t.Fatalf("open questions should be body content: %q", body)
	}

	formatted, err := FormatDossierFile(*fm, body)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if strings.Contains(formatted, "last_touched_at:") || strings.Contains(formatted, "token_target:") || strings.Contains(formatted, "priority_score:") {
		t.Fatalf("removed fields leaked into formatted dossier: %s", formatted)
	}

	legacy := strings.Replace(formatted, "priority: high", "importance: high", 1)
	if _, _, err := ParseDossierFile(legacy); err == nil {
		t.Fatal("legacy frontmatter was accepted; schema should fail explicitly")
	}
}
