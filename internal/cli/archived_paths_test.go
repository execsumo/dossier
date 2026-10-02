package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dossier/internal/core"
	"dossier/internal/store"
)

func TestServicePathsResolveArchivedAndLegacyDoneDossiers(t *testing.T) {
	home := t.TempDir()
	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	d := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_archived_path", Name: "Archived path", Slug: "archived-path",
			CreatedAt: now, UpdatedAt: now, Status: core.StatusExecute, Priority: core.PriorityMedium,
		},
		DistilledState: core.DistilledState{Body: "# Archived path\n"},
	}
	if _, err := fs.Write(d, ""); err != nil {
		t.Fatal(err)
	}
	svc, err := wire(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Archive(context.Background(), core.ArchiveReq{ID: d.Frontmatter.ID}); err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(home, "archive", "archived-path")
	assertServicePaths := func(id, want string) {
		t.Helper()
		pathRes, err := svc.Path(context.Background(), core.PathReq{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if got := pathRes.Data.(string); got != want {
			t.Fatalf("Path(%s) = %q, want %q", id, got, want)
		}
		recall, err := svc.Recall(context.Background(), core.RecallReq{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if got := recall.Data.(core.RecallResult).Path; got != want {
			t.Fatalf("Recall(%s).Path = %q, want %q", id, got, want)
		}
		list, err := svc.List(context.Background(), core.ListReq{Status: "all"})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range list.Data.([]core.ListItem) {
			if item.ID == id {
				if item.Path != want {
					t.Fatalf("List(%s).Path = %q, want %q", id, item.Path, want)
				}
				return
			}
		}
		t.Fatalf("List(all) omitted %s", id)
	}
	assertServicePaths(d.Frontmatter.ID, wantPath)

	rename, err := svc.Rename(context.Background(), core.RenameReq{ID: d.Frontmatter.ID, NewName: "Renamed archived path"})
	if err != nil {
		t.Fatal(err)
	}
	if got := rename.Data.(core.RenameResult).Path; got != wantPath {
		t.Fatalf("Rename(...).Path = %q, want %q", got, wantPath)
	}

	// Older stores may contain done dossiers at the root until their next write.
	legacyDir := filepath.Join(home, "legacy-done")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyFM := core.Frontmatter{
		ID: "dos_legacy_done", Name: "Legacy done", Slug: "legacy-done",
		CreatedAt: now, UpdatedAt: now, Status: core.StatusDone, Priority: core.PriorityLow,
	}
	formatted, err := store.FormatDossierFile(legacyFM, "# Legacy done\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "dossier.md"), []byte(formatted), 0o644); err != nil {
		t.Fatal(err)
	}
	assertServicePaths(legacyFM.ID, legacyDir)
}
