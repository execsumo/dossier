package store

import (
	"dossier/internal/core"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInboxStoreRoundTripAndValidation(t *testing.T) {
	home := t.TempDir()
	fs := NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "topic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := FormatDossierFile(core.Frontmatter{ID: "dos_topic", Name: "Topic", Slug: "topic", CreatedAt: time.Now(), UpdatedAt: time.Now(), Status: core.StatusExecute, Priority: core.PriorityMedium}, "# Topic")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dossier.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	item := &core.InboxItem{DossierID: "dos_topic", Source: core.InboxSource{Kind: "comms", URL: "https://example.test/thread"}, Excerpt: "A routed excerpt", RoutedBy: "agent:router", Confidence: 0.9, ReceivedAt: time.Now().UTC(), State: core.InboxPending}
	if err := fs.CreateInbox(item); err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadInbox("dos_topic", item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Excerpt != item.Excerpt || got.State != core.InboxPending || got.Source.URL != item.Source.URL {
		t.Fatalf("inbox read = %+v", got)
	}
	if issues := fs.ValidateInbox("dos_topic"); len(issues) != 0 {
		t.Fatalf("valid inbox issues = %v", issues)
	}
	got.State = core.InboxDismissed
	if err := fs.WriteInbox(got); err != nil {
		t.Fatal(err)
	}
	items, err := fs.ListInbox("dos_topic")
	if err != nil || len(items) != 1 || items[0].State != core.InboxDismissed {
		t.Fatalf("inbox list = %+v, err=%v", items, err)
	}
	if _, err := fs.ReadInbox("dos_topic", "../dossier"); err == nil {
		t.Fatal("unsafe inbox id accepted")
	}
}
