package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSaveLeadUpdateRecordsAudit proves that changing the lead through the unified
// Save path persists the field and records a field-level before→after audit message,
// so the agent-facing path keeps the provenance the dedicated CLI/TUI commands had.
func TestSaveLeadUpdateRecordsAudit(t *testing.T) {
	fakeStore := newLocalFakeStore()
	clk := &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, clk, Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100}, nil)

	ctx := context.Background()
	// Seed a dossier.
	if _, err := svc.Save(ctx, SaveReq{
		DistilledStateMarkdown: "# Test",
		FrontmatterUpdates:     map[string]any{"name": "Lead Test", "status": "active"},
	}); err != nil {
		t.Fatalf("seed Save failed: %v", err)
	}

	var id string
	for did := range fakeStore.dossiers {
		id = did
	}

	// Update the lead via the unified path.
	if _, err := svc.Save(ctx, SaveReq{ID: id, FrontmatterUpdates: map[string]any{"lead": "Alice"}}); err != nil {
		t.Fatalf("lead Save failed: %v", err)
	}

	d, _, err := fakeStore.Read(id)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if d.Frontmatter.Lead != "Alice" {
		t.Errorf("expected lead Alice, got %q", d.Frontmatter.Lead)
	}

	// The most recent audit event must capture the lead change before→after.
	events := fakeStore.audits[id]
	if len(events) == 0 {
		t.Fatalf("expected audit events, got none")
	}
	last := events[len(events)-1]
	if last.Event != AuditEventSave {
		t.Errorf("expected event %q, got %q", AuditEventSave, last.Event)
	}
	if !strings.Contains(last.Message, "lead") || !strings.Contains(last.Message, "Alice") {
		t.Errorf("expected audit message to record the lead change, got %q", last.Message)
	}
}

// TestSaveStatusChangeAuditsStatusChanged proves SPEC §300 holds through the unified
// Save path: a lifecycle status change is recorded as a status_changed audit event,
// not a generic save, even though it arrives via Save/FrontmatterUpdates.
func TestSaveStatusChangeAuditsStatusChanged(t *testing.T) {
	fakeStore := newLocalFakeStore()
	clk := &mockClock{now: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)}
	svc := NewService(fakeStore, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, clk, Config{DossierHome: "/tmp/dossier-test", TokenTarget: 100}, nil)

	ctx := context.Background()
	if _, err := svc.Save(ctx, SaveReq{
		DistilledStateMarkdown: "# Test",
		FrontmatterUpdates:     map[string]any{"name": "Status Test", "status": "active"},
	}); err != nil {
		t.Fatalf("seed Save failed: %v", err)
	}
	var id string
	for did := range fakeStore.dossiers {
		id = did
	}

	if _, err := svc.Save(ctx, SaveReq{ID: id, FrontmatterUpdates: map[string]any{"status": "waiting"}}); err != nil {
		t.Fatalf("status Save failed: %v", err)
	}

	events := fakeStore.audits[id]
	last := events[len(events)-1]
	if last.Event != AuditEventStatusChanged {
		t.Errorf("expected %q event for a status change, got %q", AuditEventStatusChanged, last.Event)
	}
	if !strings.Contains(last.Message, "active") || !strings.Contains(last.Message, "waiting") {
		t.Errorf("expected status before→after in message, got %q", last.Message)
	}
}

// TestDescribeFrontmatterChanges checks the audit summary covers multiple fields and
// returns empty when nothing material changed.
func TestDescribeFrontmatterChanges(t *testing.T) {
	before := Frontmatter{Name: "A", Status: StatusActive, Lead: ""}
	after := Frontmatter{Name: "A", Status: StatusWaiting, Lead: "Bob"}

	msg := describeFrontmatterChanges(before, after)
	if !strings.Contains(msg, "status") || !strings.Contains(msg, "lead") {
		t.Errorf("expected status and lead in summary, got %q", msg)
	}
	if strings.Contains(msg, "name") {
		t.Errorf("name did not change; should not appear: %q", msg)
	}

	if got := describeFrontmatterChanges(before, before); got != "" {
		t.Errorf("expected empty summary for no changes, got %q", got)
	}
}
