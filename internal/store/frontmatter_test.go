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

	unknown := strings.Replace(formatted, "priority: high", "priority: high\nfuture_field: true", 1)
	if _, _, err := ParseDossierFile(unknown); err == nil || !strings.Contains(err.Error(), "future_field") {
		t.Fatalf("unknown frontmatter field error = %v, want strict rejection", err)
	}
}

func TestParseLegacyFrontmatterUsesCanonicalPriorityWhenPresent(t *testing.T) {
	content := `---
id: dos_example
name: Example
slug: example
created_at: 2026-01-01T00:00:00Z
updated_at: 2026-01-01T00:00:00Z
last_touched_at: 2026-01-02T00:00:00Z
status: active
next_action: inspect
priority: low
importance: high
urgency: high
open_questions: []
token_target: 100000
---
# Example
`
	fm, _, err := ParseDossierFile(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if fm.Priority != core.PriorityLow {
		t.Fatalf("canonical priority did not take precedence: %q", fm.Priority)
	}
}
