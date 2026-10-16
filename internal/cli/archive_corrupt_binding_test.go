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

func TestArchiveRejectsCorruptBindingsBeforeChangingState(t *testing.T) {
	home := t.TempDir()
	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	d := &core.Dossier{Frontmatter: core.Frontmatter{ID: "dos_corrupt_binding", Name: "Corrupt binding", Slug: "corrupt-binding", CreatedAt: now, UpdatedAt: now, Status: core.StatusDefine, Priority: core.PriorityMedium}, DistilledState: core.DistilledState{Body: "# Keep state\n"}}
	if _, err := fs.Write(d, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "sessions", "corrupt.json"), []byte("not JSON"), 0644); err != nil {
		t.Fatal(err)
	}
	svc, err := wire(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Archive(context.Background(), core.ArchiveReq{ID: d.Frontmatter.ID}); err == nil {
		t.Fatal("archive ignored corrupt binding")
	}
	got, _, err := fs.Read(d.Frontmatter.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frontmatter.Status != core.StatusDefine {
		t.Fatalf("archive changed status to %s", got.Frontmatter.Status)
	}
}
