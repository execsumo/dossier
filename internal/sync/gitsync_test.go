package sync

import (
	"strconv"
	"sync"
	"testing"
)

// TestSync_TwoClonesConverge (DoD i + ii): two clones of one bare repo converge
// after alternating writes+syncs, exercising a clean fast-forward pull in both
// directions.
func TestSync_TwoClonesConverge(t *testing.T) {
	bare, storeA, storeB := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")
	syncB := newSyncer(storeB, bare, "bob")

	// A writes pricing/dossier.md and syncs.
	writeFile(t, storeA, "pricing/dossier.md", "A-v1")
	repA := mustSync(t, syncA)
	if !repA.Pushed {
		t.Fatalf("A's first sync should push")
	}

	// B writes onboarding/dossier.md and syncs; B pulls A's pricing.
	writeFile(t, storeB, "onboarding/dossier.md", "B-v1")
	repB := mustSync(t, syncB)
	if !repB.Pulled {
		t.Fatalf("B should pull A's changes")
	}
	// B now has both: its own onboarding + A's pricing (merged).
	assertFile(t, storeB, "onboarding/dossier.md", "B-v1")
	assertFile(t, storeB, "pricing/dossier.md", "A-v1")

	// A syncs again: clean fast-forward pull of B's onboarding.
	repA2 := mustSync(t, syncA)
	if !repA2.Pulled {
		t.Fatalf("A should pull B's changes (fast-forward)")
	}
	assertFile(t, storeA, "onboarding/dossier.md", "B-v1")
	assertFile(t, storeA, "pricing/dossier.md", "A-v1")
	assertNoConflictMarkers(t, storeA)
	assertNoConflictMarkers(t, storeB)
}

// TestSync_BothModifiedDossierMD (DoD iii): when both machines modify the same
// dossier.md, the remote version wins the working tree, the local version is
// captured in a ConflictRecord, and no git conflict markers appear anywhere.
func TestSync_BothModifiedDossierMD(t *testing.T) {
	bare, storeA, storeB := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")
	syncB := newSyncer(storeB, bare, "bob")

	// Shared base: both have "base" committed.
	writeFile(t, storeA, "pricing/dossier.md", "base")
	mustSync(t, syncA) // A commits+pushes "base".
	mustSync(t, syncB) // B fast-forwards to A's "base".

	// Both now modify the same file to different values.
	writeFile(t, storeA, "pricing/dossier.md", "A-edit")
	writeFile(t, storeB, "pricing/dossier.md", "B-edit")
	mustSync(t, syncA)         // A commits+pushes "A-edit".
	repB := mustSync(t, syncB) // B: divergent remote-wins merge -> conflict.

	// Remote wins the working tree.
	assertFile(t, storeB, "pricing/dossier.md", "A-edit")

	// Exactly one conflict, capturing local "B-edit" and remote "A-edit".
	if len(repB.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(repB.Conflicts))
	}
	c := repB.Conflicts[0]
	if c.Path != "pricing/dossier.md" {
		t.Fatalf("conflict path = %q, want pricing/dossier.md", c.Path)
	}
	if string(c.LocalContent) != "B-edit" {
		t.Fatalf("local content = %q, want B-edit", string(c.LocalContent))
	}
	if string(c.RemoteContent) != "A-edit" {
		t.Fatalf("remote content = %q, want A-edit", string(c.RemoteContent))
	}
	if c.LocalRevision == "" || c.RemoteRevision == "" {
		t.Fatalf("conflict revisions should be populated: %+v", c)
	}

	// No git conflict markers anywhere in the working tree.
	assertNoConflictMarkers(t, storeB)
}

// TestSync_OversizedFileExcluded (DoD iv): a file >100 MB is excluded from the
// commit, reported in SyncReport.Excluded, and never lands in git history.
func TestSync_OversizedFileExcluded(t *testing.T) {
	bare, storeA, _ := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")

	// 101 MB sparse file (does not allocate 101 MB on disk).
	sparseBigFile(t, storeA, "artifacts/huge.bin", 101*1024*1024)
	// Plus a normal small file so there is something real to commit.
	writeFile(t, storeA, "pricing/dossier.md", "small")

	rep := mustSync(t, syncA)
	if len(rep.Excluded) != 1 {
		t.Fatalf("expected 1 excluded file, got %d", len(rep.Excluded))
	}
	if rep.Excluded[0].Path != "artifacts/huge.bin" {
		t.Fatalf("excluded path = %q, want artifacts/huge.bin", rep.Excluded[0].Path)
	}
	if rep.Excluded[0].Size <= MaxFileSizeBytes {
		t.Fatalf("excluded size %d should be > %d", rep.Excluded[0].Size, MaxFileSizeBytes)
	}
	if rep.Excluded[0].Warning == "" {
		t.Fatalf("excluded file should carry a human-readable warning")
	}

	// The oversized file must NOT be in git history; the small file must be.
	assertNotInGitHistory(t, storeA, "artifacts/huge.bin")
	assertFileInTree(t, storeA, "pricing/dossier.md")
}

// TestSync_ConcurrentSerialize (DoD v): N concurrent Sync() calls on one store
// serialize via the .sync.lock; all complete without corruption and every
// change lands. (Without the lock, concurrent go-git index writes would error or
// corrupt the repo.)
func TestSync_ConcurrentSerialize(t *testing.T) {
	bare, storeA, _ := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			if err := writeFileErr(storeA, dirFile(i), contentFor(i)); err != nil {
				errs[i] = err
				return
			}
			<-start // release all goroutines together
			_, errs[i] = syncA.Sync()
		}()
	}
	close(start)
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("concurrent sync %d failed: %v", i, e)
		}
	}
	// Every file landed in the HEAD tree.
	repo, tree := headTree(t, storeA)
	for i := 0; i < n; i++ {
		got, err := blobContent(repo, tree, dirFile(i))
		if err != nil {
			t.Fatalf("file %d missing from HEAD tree: %v", i, err)
		}
		if string(got) != contentFor(i) {
			t.Fatalf("file %d content = %q, want %q", i, string(got), contentFor(i))
		}
	}
	assertNoConflictMarkers(t, storeA)
}

func dirFile(i int) string    { return "d" + strconv.Itoa(i) + "/file.txt" }
func contentFor(i int) string { return "data-" + strconv.Itoa(i) }
