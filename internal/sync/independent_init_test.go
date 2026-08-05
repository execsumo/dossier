package sync

import (
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// initIndependentStore creates a store as a real onboarding user would when they
// run `dossier init` and then point at a team remote WITHOUT cloning it: a fresh
// git repo with its own machine-local .gitignore and an origin remote. Two such
// stores share NO common git ancestor, so every commonly-named file (notably the
// identical, managed .gitignore) is "both-added" on first sync.
func initIndependentStore(t *testing.T, dir, bareDir string) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("plaininit %s: %v", dir, err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}
	if err := EnsureGitignore(dir); err != nil {
		t.Fatalf("ensure gitignore: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{bareDir}}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
}

// TestSync_IndependentInitsNoPhantomConflict is the regression guard for the
// onboarding flow where two colleagues each `dossier init` their own store
// (no shared ancestor) and then sync through a common remote. The identical,
// store-managed .gitignore must NOT be reported as a sync conflict, and store B
// must still receive store A's dossier. (Before the fix, .gitignore showed up as
// a both-added ConflictRecord, producing a phantom `Conflicts: 1` and a failed
// "dossier .gitignore not found" routing on every first join.)
func TestSync_IndependentInitsNoPhantomConflict(t *testing.T) {
	bareDir := filepath.Join(t.TempDir(), "bare.git")
	bare, err := git.PlainInit(bareDir, true)
	if err != nil {
		t.Fatalf("plaininit bare: %v", err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatalf("set bare HEAD: %v", err)
	}

	storeA := filepath.Join(t.TempDir(), "storeA")
	storeB := filepath.Join(t.TempDir(), "storeB")
	initIndependentStore(t, storeA, bareDir)
	initIndependentStore(t, storeB, bareDir)

	// A creates a dossier and pushes it (first commit on the remote).
	writeFile(t, storeA, "pricing/dossier.md", "# Pricing\nAlice's notes\n")
	repA := mustSync(t, newSyncer(storeA, bareDir, "alice"))
	if len(repA.Conflicts) != 0 {
		t.Fatalf("store A first sync should have no conflicts, got %d: %+v", len(repA.Conflicts), repA.Conflicts)
	}

	// B (independently initialized, its own .gitignore) joins by syncing. No
	// common ancestor with A, so .gitignore is both-added — but it must NOT be a
	// conflict.
	repB := mustSync(t, newSyncer(storeB, bareDir, "bob"))
	if len(repB.Conflicts) != 0 {
		t.Fatalf("store B first sync must report ZERO conflicts (managed .gitignore is not a conflict), got %d: %+v", len(repB.Conflicts), repB.Conflicts)
	}

	// B must have actually received A's dossier (remote-wins landed it).
	assertFile(t, storeB, "pricing/dossier.md", "# Pricing\nAlice's notes\n")
	assertNoConflictMarkers(t, storeB)
}
