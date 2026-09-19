package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if err := fs.WriteConflict(&core.Conflict{ID: "conf_cli", DossierID: "dos_cli", Kind: "merge_conflict", TS: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), RejectedBody: "mine"}); err != nil {
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

func TestCLIConflictDetailTextAndJSON(t *testing.T) {
	home := t.TempDir()
	fs := store.NewFSStore(home)
	if err := fs.Init(); err != nil {
		t.Fatal(err)
	}
	file, err := store.FormatDossierFile(core.Frontmatter{
		ID: "dos_detail_cli", Name: "Readable conflict", Slug: "readable-conflict",
		Status: core.StatusSpark, Priority: core.PriorityMedium,
	}, "shared current\n")
	if err != nil {
		t.Fatal(err)
	}
	dossierDir := filepath.Join(home, "readable-conflict")
	if err := os.MkdirAll(dossierDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dossierDir, "dossier.md"), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteConflict(&core.Conflict{ID: "conf_detail_cli", DossierID: "dos_detail_cli", Kind: "merge_conflict", TS: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), RejectedBody: "mine\n"}); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()
		read, write, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		old := os.Stdout
		os.Stdout = write
		cmd := NewRootCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			os.Stdout = old
			read.Close()
			write.Close()
			t.Fatalf("command %v: %v", args, err)
		}
		write.Close()
		os.Stdout = old
		out, err := io.ReadAll(read)
		read.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}

	text := run("conflicts", "conf_detail_cli", "--home", home)
	for _, want := range []string{"Dossier: Readable conflict", "Conflict: conf_detail_cli", "Shared (current)", "shared current", "Yours (preserved)", "mine", "Diff"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text output missing %q:\n%s", want, text)
		}
	}
	json := run("conflicts", "conf_detail_cli", "--json", "--home", home)
	for _, want := range []string{`"dossier_name": "Readable conflict"`, `"shared": "shared current\n"`, `"mine": "mine\n"`, `"diff":`} {
		if !strings.Contains(json, want) {
			t.Fatalf("json output missing %q:\n%s", want, json)
		}
	}
}
