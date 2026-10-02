package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// A mutating CLI command in a team store pushes after it succeeds, and when the
// remote is unreachable it says so on stderr instead of staying silent. The
// command itself still succeeds: the change is saved locally.
func TestCLIMutationSyncFailureIsLoud(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	bare, err := git.PlainInit(remote, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	store, home := filepath.Join(root, "store"), filepath.Join(root, "home")
	t.Setenv("DOSSIER_HOME", store)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, args := range [][]string{
		{"init", "--yes"},
		{"promote", "Sync topic"},
		{"team", "create", remote, "--yes", "--name", "Manager Name"},
	} {
		if _, stderr, err := executeM6(t, "", args...); err != nil {
			t.Fatalf("%v: %v %s", args, err, stderr)
		}
	}

	// Healthy remote: no warning.
	if _, stderr, err := executeM6(t, "", "next", "sync-topic", "first step"); err != nil || strings.Contains(stderr, "Team sync") {
		t.Fatalf("healthy mutation: err=%v stderr=%q", err, stderr)
	}

	// Remote disappears.
	if err := os.Rename(remote, remote+".gone"); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := executeM6(t, "", "next", "sync-topic", "second step")
	if err != nil {
		t.Fatalf("mutation must still succeed offline: %v", err)
	}
	if !strings.Contains(stderr, "Team sync after this change failed") || !strings.Contains(stderr, "saved locally") {
		t.Fatalf("offline mutation must warn loudly, stderr=%q", stderr)
	}

	// Read-only commands never sync or warn.
	if _, stderr, err := executeM6(t, "", "show", "sync-topic"); err != nil || strings.Contains(stderr, "Team sync") {
		t.Fatalf("read-only command warned: err=%v stderr=%q", err, stderr)
	}
}
