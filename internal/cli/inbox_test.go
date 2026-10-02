package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"dossier/internal/core"
	"dossier/internal/store"
)

func TestInboxCLIWorkflow(t *testing.T) {
	home := t.TempDir()
	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	dossier := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_inbox_cli", Name: "Inbox CLI", Slug: "inbox-cli",
			CreatedAt: now, UpdatedAt: now, Status: core.StatusExecute, Priority: core.PriorityMedium,
		},
		DistilledState: core.DistilledState{Body: "# Inbox CLI\n"},
	}
	if _, err := fs.Write(dossier, ""); err != nil {
		t.Fatal(err)
	}
	excerptPath := filepath.Join(t.TempDir(), "excerpt.md")
	if err := os.WriteFile(excerptPath, []byte("Routed from a file.\nSecond line."), 0o600); err != nil {
		t.Fatal(err)
	}

	execute := func(args ...string) string {
		t.Helper()
		args = append(args, "--home", home)
		cmd := NewRootCmd()
		cmd.SetArgs(args)
		var output strings.Builder
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("dossier %s: %v\n%s", strings.Join(args, " "), err, output.String())
		}
		return output.String()
	}

	execute("inbox", "capture", "inbox-cli", "--source-kind", "email", "--url", "https://example.test/mail/1", "--excerpt-file", excerptPath, "--confidence", "0.8")
	svc, err := wire(home)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.Inbox(context.Background(), core.InboxListReq{ID: "inbox-cli"})
	if err != nil {
		t.Fatal(err)
	}
	items := listed.Data.([]core.InboxItem)
	if len(items) != 1 || items[0].Excerpt != "Routed from a file.\nSecond line." {
		t.Fatalf("captured inbox items = %+v", items)
	}
	itemID := items[0].ID

	if output := execute("inbox", "list", "inbox-cli"); !strings.Contains(output, itemID) {
		t.Fatalf("inbox list output omitted %s: %s", itemID, output)
	}
	if output := execute("inbox", "read", "inbox-cli", itemID); !strings.Contains(output, "Second line.") {
		t.Fatalf("inbox read output omitted excerpt: %s", output)
	}
	execute("inbox", "resolve", "inbox-cli", itemID, "dismiss")
	read, err := svc.ReadInbox(context.Background(), core.InboxReadReq{ID: "inbox-cli", InboxID: itemID})
	if err != nil {
		t.Fatal(err)
	}
	if got := read.Data.(*core.InboxItem).State; got != core.InboxDismissed {
		t.Fatalf("dismissed inbox state = %q", got)
	}

	execute("inbox", "capture", "inbox-cli", "--source-kind", "chat", "--excerpt", "Absorb through CLI")
	listed, err = svc.Inbox(context.Background(), core.InboxListReq{ID: "inbox-cli"})
	if err != nil {
		t.Fatal(err)
	}
	items = listed.Data.([]core.InboxItem)
	var pending core.InboxItem
	for _, item := range items {
		if item.State == core.InboxPending {
			pending = item
		}
	}
	if pending.ID == "" {
		t.Fatalf("no pending item after second capture: %+v", items)
	}
	execute("inbox", "resolve", "inbox-cli", pending.ID, "absorb")
	read, err = svc.ReadInbox(context.Background(), core.InboxReadReq{ID: "inbox-cli", InboxID: pending.ID})
	if err != nil {
		t.Fatal(err)
	}
	if item := read.Data.(*core.InboxItem); item.State != core.InboxAbsorbed || item.ArtifactID == "" {
		t.Fatalf("absorbed inbox record = %+v", item)
	}
}

func TestInboxResolveSyncClassification(t *testing.T) {
	root := NewRootCmd()
	resolve, _, err := root.Find([]string{"inbox", "resolve"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		action string
		want   bool
	}{
		{"absorb", true},
		{"ABSORB", true},
		{"dismiss", false},
	} {
		if got := isSyncedMutation(resolve, "topic", "inbox_item", tc.action); got != tc.want {
			t.Errorf("isSyncedMutation(inbox resolve %s) = %t, want %t", tc.action, got, tc.want)
		}
	}
}

func TestTruncateRunesPreservesUTF8(t *testing.T) {
	value := strings.Repeat("é", 16)
	if got := truncateRunes(value, 28); got != value || !utf8.ValidString(got) {
		t.Fatalf("truncateRunes corrupted a valid 16-rune action: %q", got)
	}
	long := strings.Repeat("é", 32)
	got := truncateRunes(long, 28)
	if !utf8.ValidString(got) || got != strings.Repeat("é", 25)+"..." {
		t.Fatalf("truncateRunes(%q) = %q, want valid UTF-8 with a safe ellipsis", long, got)
	}
}
