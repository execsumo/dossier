package cli

import (
	"context"
	"dossier/internal/core"
	"dossier/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveClearsOnlyMatchingBindingsAndPreservesHistory(t *testing.T) {
	home := t.TempDir()
	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	d := &core.Dossier{Frontmatter: core.Frontmatter{ID: "dos_archive_bindings", Name: "Archive bindings", Slug: "archive-bindings", CreatedAt: now, UpdatedAt: now, Status: core.StatusDefine, Priority: core.PriorityMedium}, DistilledState: core.DistilledState{Body: "# Keep this history\n"}}
	if _, err := fs.Write(d, ""); err != nil {
		t.Fatal(err)
	}
	for id, target := range map[string]string{"one": d.Frontmatter.ID, "two": d.Frontmatter.ID, "other": "dos_other", "unbound": ""} {
		if err := fs.SaveSessionBinding(&core.SessionBinding{SessionBindingID: id, DossierID: target}); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := wire(home)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.Archive(context.Background(), core.ArchiveReq{ID: d.Frontmatter.ID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"one", "two"} {
		if _, err := fs.GetSessionBinding(id); err == nil {
			t.Errorf("archive retained session %s", id)
		}
	}
	for _, id := range []string{"other", "unbound"} {
		if _, err := fs.GetSessionBinding(id); err != nil {
			t.Errorf("archive removed unrelated session %s: %v", id, err)
		}
	}
	got, _, err := fs.Read(d.Frontmatter.ID)
	if err != nil || got.DistilledState.Body != d.DistilledState.Body {
		t.Fatalf("archive lost state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "archive", d.Frontmatter.Slug, "dossier.md")); err != nil {
		t.Fatal(err)
	}
	locks, err := os.ReadDir(filepath.Join(home, ".locks"))
	if err != nil || len(locks) == 0 {
		t.Fatalf("stable lock identity removed: %v", err)
	}
}
