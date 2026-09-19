package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGitignoreMergesRawArtifactEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("# user rule\n/private/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignore(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "# user rule\n/private/") {
		t.Fatalf("user entries changed: %q", got)
	}
	if !strings.Contains(got, "*/artifacts/*_raw.*") {
		t.Fatalf("raw artifact entry missing: %q", got)
	}
}
