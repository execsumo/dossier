package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestCheckRemoteEmptyRefusesNonEmpty(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.git")
	repo, err := git.PlainInit(empty, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	g := New(Config{StoreDir: t.TempDir(), Branch: "main"})
	if err := g.CheckRemoteEmpty(context.Background(), empty); err != nil {
		t.Fatalf("empty remote rejected: %v", err)
	}

	nonEmpty := filepath.Join(t.TempDir(), "nonempty.git")
	seedBareRepo(t, nonEmpty)
	if err := g.CheckRemoteEmpty(context.Background(), nonEmpty); err == nil || !strings.Contains(err.Error(), "is not empty") {
		t.Fatalf("non-empty remote was not refused: %v", err)
	}
}

func TestCreateFailureMovesGitAsideAndRetryWorks(t *testing.T) {
	storeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(storeDir, "local.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := New(Config{StoreDir: storeDir, Branch: "main"})
	if err := g.Create(context.Background(), filepath.Join(t.TempDir(), "missing.git"), "main"); err == nil {
		t.Fatal("unreachable create should fail")
	}
	if _, err := os.Stat(filepath.Join(storeDir, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed create left .git in store: %v", err)
	}
	matches, err := filepath.Glob(storeDir + ".failed-create-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("failed-create archive missing: %v (%v)", matches, err)
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	bare, err := git.PlainInit(remote, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	if err := g.Create(context.Background(), remote, "main"); err != nil {
		t.Fatalf("retry create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(storeDir, ".git")); err != nil {
		t.Fatalf("retry did not recreate .git: %v", err)
	}
}

func TestCloneRejectsNonMainDefaultBranch(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "master.git")
	bare, err := git.PlainInit(remote, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("master"))); err != nil {
		t.Fatal(err)
	}
	seed := t.TempDir()
	seedRepo, err := git.PlainInit(seed, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedRepo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("master"))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedWT, err := seedRepo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seedWT.Add("README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := seedWT.Commit("seed", &git.CommitOptions{Author: testSig("seed"), Committer: testSig("seed")}); err != nil {
		t.Fatal(err)
	}
	if _, err := seedRepo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{remote}}); err != nil {
		t.Fatal(err)
	}
	if err := seedRepo.Push(&git.PushOptions{RemoteName: "origin", RefSpecs: []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"}}); err != nil {
		t.Fatal(err)
	}
	g := New(Config{StoreDir: filepath.Join(t.TempDir(), "store"), Branch: "main"})
	err = g.Clone(context.Background(), remote, g.cfg.StoreDir, 0)
	if err == nil || !strings.Contains(err.Error(), "default branch is master") {
		t.Fatalf("expected named default-branch error, got %v", err)
	}
}
