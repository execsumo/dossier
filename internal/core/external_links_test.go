package core

import "testing"

func TestParseExternalLinksSeparatesReferencesAndMonitors(t *testing.T) {
	body := `# Pricing

## References
- [ticket: PROJ-123](https://jira.example/PROJ-123) — Pricing migration work.
- [document: Launch plan](https://docs.example/launch) — Current rollout plan.

## Active Monitors
- [comms: #pricing-bug](https://slack.example/thread) — Watch for approval changes. (Last polled: 2026-09-04)

## Current State
- [not-a-reference](https://example.invalid) — ordinary body link
`

	got := ParseExternalLinks(body)
	if len(got.References) != 2 {
		t.Fatalf("got %d references, want 2: %+v", len(got.References), got.References)
	}
	if got.References[0].Kind != "ticket" || got.References[0].Label != "PROJ-123" || got.References[0].URL != "https://jira.example/PROJ-123" {
		t.Fatalf("unexpected first reference: %+v", got.References[0])
	}
	if len(got.ActiveMonitors) != 1 {
		t.Fatalf("got %d monitors, want 1: %+v", len(got.ActiveMonitors), got.ActiveMonitors)
	}
	monitor := got.ActiveMonitors[0]
	if monitor.Kind != "comms" || monitor.Label != "#pricing-bug" || monitor.LastPolled != "2026-09-04" {
		t.Fatalf("unexpected monitor: %+v", monitor)
	}
	if monitor.Description != "Watch for approval changes." {
		t.Errorf("monitor description = %q, want polling suffix removed", monitor.Description)
	}
}

func TestParseExternalLinksIgnoresMalformedAndUnrelatedLinks(t *testing.T) {
	body := `## References
- [missing URL] — no URL
- [ticket: PROJ-123](https://jira.example/PROJ-123) — valid

## Findings
- [ticket: not-a-reference](https://jira.example/other) — outside section
`
	got := ParseExternalLinks(body)
	if len(got.References) != 1 || got.References[0].Label != "PROJ-123" {
		t.Fatalf("got %+v, want one valid reference", got.References)
	}
}
