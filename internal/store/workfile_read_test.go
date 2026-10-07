package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"dossier/internal/core"
)

func TestFSStoreReadWorkingFile(t *testing.T) {
	home := t.TempDir()
	st := NewFSStore(home)
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	d := &core.Dossier{
		Frontmatter: core.Frontmatter{
			ID: "dos_rf", Name: "Read Files", Slug: "read-files",
			CreatedAt: now, UpdatedAt: now, Status: core.StatusActive, Priority: core.PriorityHigh,
		},
		DistilledState: core.DistilledState{Body: "# Read files"},
	}
	if _, err := st.Write(d, ""); err != nil {
		t.Fatal(err)
	}
	dossierDir := filepath.Join(home, "read-files")
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dossierDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("files/notes.md", "hello")
	write("files/sub/deck.bin", "bin\x00ary")
	write("dossier-secret.txt", "SECRET")
	write("audit/alice.log", "AUDIT")
	if err := os.WriteFile(filepath.Join(home, "outside.txt"), []byte("OUTSIDE"), 0o644); err != nil {
		t.Fatal(err)
	}

	type rejection struct {
		name string
		path string
		code core.ErrorCode
	}
	rejected := []rejection{
		{"parent escape", "files/../dossier-secret.txt", core.ErrInvalidFrontmatter},
		{"deep parent escape", "files/../../outside.txt", core.ErrInvalidFrontmatter},
		{"audit via traversal", "files/../audit/alice.log", core.ErrInvalidFrontmatter},
		{"absolute path inside dossier", filepath.Join(dossierDir, "files", "notes.md"), core.ErrInvalidFrontmatter},
		{"absolute path elsewhere", filepath.Join(home, "outside.txt"), core.ErrInvalidFrontmatter},
		{"outside files namespace", "dossier-secret.txt", core.ErrInvalidFrontmatter},
		{"bare files dir", "files", core.ErrInvalidFrontmatter},
		{"empty", "", core.ErrInvalidFrontmatter},
		{"directory", "files/sub", core.ErrInvalidFrontmatter},
		{"missing", "files/nope.md", core.ErrNotFound},
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(home, "outside.txt"), filepath.Join(dossierDir, "files", "link.md")); err != nil {
			t.Fatal(err)
		}
		rejected = append(rejected, rejection{"symlink out of files/", "files/link.md", core.ErrInvalidFrontmatter})
	}

	t.Run("reads text and binary bytes", func(t *testing.T) {
		got, err := st.ReadWorkingFile("dos_rf", "files/notes.md")
		if err != nil || string(got) != "hello" {
			t.Fatalf("got %q, %v", got, err)
		}
		got, err = st.ReadWorkingFile("dos_rf", "files/sub/deck.bin")
		if err != nil || string(got) != "bin\x00ary" {
			t.Fatalf("got %q, %v", got, err)
		}
		if _, err := st.ReadWorkingFile("read-files", "files/sub/../notes.md"); err != nil {
			t.Fatalf("a path that normalizes within files/ is fine: %v", err)
		}
	})

	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.ReadWorkingFile("dos_rf", tt.path)
			if err == nil {
				t.Fatalf("expected rejection, read %q", got)
			}
			var de *core.DomainError
			if !errors.As(err, &de) || de.Code != tt.code {
				t.Fatalf("error = %v, want code %s", err, tt.code)
			}
		})
	}

	if _, err := st.ReadWorkingFile("dos_missing", "files/notes.md"); err == nil {
		t.Fatal("unknown dossier must error")
	}
}
