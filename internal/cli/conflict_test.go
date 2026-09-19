package cli

import (
	"os"
	"path/filepath"
	"testing"

	"dossier/internal/core"
	"dossier/internal/store"
)

func TestCLIConflictsAndResolve(t *testing.T) {
	home := t.TempDir()
	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	file, err := store.FormatDossierFile(core.Frontmatter{
		ID: "dos_cli", Name: "CLI conflict", Slug: "cli-conflict",
		Status: core.StatusSpark, Priority: core.PriorityMedium,
	}, "shared\n")
	if err != nil {
		t.Fatal(err)
	}
	dossierDir := filepath.Join(home, "cli-conflict")
	if err := os.MkdirAll(dossierDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossierDir, "dossier.md"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteConflict(&core.Conflict{ID: "conf_cli", DossierID: "dos_cli", RejectedBody: "mine"}); err != nil {
		t.Fatal(err)
	}

	list := NewRootCmd()
	list.SetArgs([]string{"conflicts", "--json", "--home", home})
	if err := list.Execute(); err != nil {
		t.Fatalf("conflicts: %v", err)
	}

	resolve := NewRootCmd()
	resolve.SetArgs([]string{"resolve", "conf_cli", "--restore-mine", "--home", home})
	if err := resolve.Execute(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := fs.ReadConflict("conf_cli"); err == nil {
		t.Fatal("resolved conflict still listed as active")
	}

	missing := NewRootCmd()
	missing.SetArgs([]string{"resolve", "does-not-exist", "--keep-shared", "--home", home})
	if err := missing.Execute(); err == nil {
		t.Fatal("missing conflict unexpectedly resolved")
	}
	for _, command := range missing.Commands() {
		if command.Name() == "resolve" && !command.SilenceUsage {
			t.Fatal("resolve should silence usage on operational errors")
		}
	}
}
