package sync

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func assertFile(t *testing.T, dir, path, want string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, string(got), want)
	}
}

func assertFileInTree(t *testing.T, dir, path string) {
	t.Helper()
	repo, tree := headTree(t, dir)
	if _, err := blobContent(repo, tree, path); err != nil {
		t.Fatalf("expected %s in HEAD tree: %v", path, err)
	}
}

func assertNotInGitHistory(t *testing.T, dir, path string) {
	t.Helper()
	repo, tree := headTree(t, dir)
	if _, err := blobContent(repo, tree, path); err == nil {
		t.Fatalf("%s should NOT be in HEAD tree but was found", path)
	}
}

func headTree(t *testing.T, dir string) (*git.Repository, *object.Tree) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	head, err := headHash(repo)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	commit, err := repo.CommitObject(head)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	tree, err := repo.TreeObject(commit.TreeHash)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	return repo, tree
}

// assertNoConflictMarkers walks the working tree (excluding .git) and fails if
// any git conflict marker is present in a file.
func assertNoConflictMarkers(t *testing.T, dir string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if bytes.Contains(data, []byte("<<<<<<<")) || bytes.Contains(data, []byte(">>>>>>>")) {
			t.Fatalf("git conflict marker found in %s", path)
		}
		return nil
	})
}

func assertHeadCommitMessageContains(t *testing.T, dir, substr string) {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	head, err := headHash(repo)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	commit, err := repo.CommitObject(head)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !strings.Contains(commit.Message, substr) {
		t.Fatalf("head commit message %q does not contain %q", commit.Message, substr)
	}
}
