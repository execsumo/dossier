package store

import (
	"strings"
	"testing"
	"time"

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

// A Dossier with attention or repos must read back what was written; the
// strict schema once omitted attention, so any agent that set it made the
// Dossier unreadable.
func TestAttentionAndReposRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	fm := core.Frontmatter{
		ID: "dos_1", Name: "n", Slug: "n", CreatedAt: now, UpdatedAt: now,
		Status: core.StatusExecute, Priority: core.PriorityHigh,
		Attention: &core.Attention{Level: "decide", Summary: "pick a vendor", Since: now, By: "agent:x"},
		Repos:     []string{"github.com/acme/api", "github.com/acme/web"},
	}
	text, err := FormatDossierFile(fm, "body\n")
	if err != nil {
		t.Fatal(err)
	}
	got, body, err := ParseDossierFile(text)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, text)
	}
	if got.Attention == nil || got.Attention.Level != "decide" || got.Attention.By != "agent:x" {
		t.Errorf("attention = %+v", got.Attention)
	}
	if strings.Join(got.Repos, ",") != "github.com/acme/api,github.com/acme/web" {
		t.Errorf("repos = %v", got.Repos)
	}
	if body != "body\n" {
		t.Errorf("body = %q", body)
	}
}
