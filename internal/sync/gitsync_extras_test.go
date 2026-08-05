package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSync_UnreachableRemoteErrorLocalCommitLands (DoD vi): a push to an
// unreachable remote returns a visible error in the report while the local
// commit still lands in history.
func TestSync_UnreachableRemoteErrorLocalCommitLands(t *testing.T) {
	bare, storeA, _ := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")

	// Establish a good baseline.
	writeFile(t, storeA, "pricing/dossier.md", "v1")
	mustSync(t, syncA)

	// Make the configured remote unreachable (bare repo removed).
	if err := os.RemoveAll(bare); err != nil {
		t.Fatalf("remove bare: %v", err)
	}

	writeFile(t, storeA, "pricing/dossier.md", "v2")
	rep, err := syncA.Sync()
	if err != nil {
		t.Fatalf("Sync itself should not hard-error on network failure (error in report): %v", err)
	}
	if rep.Error == "" {
		t.Fatalf("expected a visible error for unreachable remote, got empty report.Error")
	}
	if rep.CommitSHA == "" {
		t.Fatalf("local commit should still land (CommitSHA empty)")
	}

	// The local commit (v2) is in git history; the working tree has v2.
	assertHeadCommitMessageContains(t, storeA, "dossier sync")
	assertFile(t, storeA, "pricing/dossier.md", "v2")
	assertFileInTree(t, storeA, "pricing/dossier.md")
}

// TestSync_GitignoreMachineLocal: the machine-local exclusion set is present
// after clone; config.yaml is never committed.
func TestSync_GitignoreMachineLocal(t *testing.T) {
	bare, storeA, _ := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")

	// .gitignore exists after clone and contains the machine-local set.
	gi, err := os.ReadFile(filepath.Join(storeA, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gi), "/config.yaml") {
		t.Fatalf(".gitignore missing /config.yaml:\n%s", string(gi))
	}
	if !strings.Contains(string(gi), "/sessions/") {
		t.Fatalf(".gitignore missing /sessions/:\n%s", string(gi))
	}

	// A machine-local config.yaml + a real dossier file.
	writeFile(t, storeA, "config.yaml", "secret: token\n")
	writeFile(t, storeA, "pricing/dossier.md", "v1")
	mustSync(t, syncA)

	// config.yaml must NOT be in git history; the dossier must be.
	assertNotInGitHistory(t, storeA, "config.yaml")
	assertFileInTree(t, storeA, "pricing/dossier.md")
}

// TestSync_GitignoreMergePreservesUserEntries: a pre-existing .gitignore is
// merged, not overwritten — user entries survive and missing defaults are added.
func TestSync_GitignoreMergePreservesUserEntries(t *testing.T) {
	bare, storeA, _ := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")

	// Add a user entry not in the default set, to prove we don't clobber.
	writeFile(t, storeA, ".gitignore", "# my local rule\n/tmp-secrets/\n")
	mustSync(t, syncA) // EnsureGitignore merges during sync (already present -> merge)

	gi, _ := os.ReadFile(filepath.Join(storeA, ".gitignore"))
	s := string(gi)
	if !strings.Contains(s, "# my local rule") {
		t.Fatalf("user entry lost after merge:\n%s", s)
	}
	if !strings.Contains(s, "/tmp-secrets/") {
		t.Fatalf("user pattern lost after merge:\n%s", s)
	}
	if !strings.Contains(s, "/config.yaml") {
		t.Fatalf("default /config.yaml not merged in:\n%s", s)
	}
}

// TestStatus_ReportsAheadBehind: Status() reports behind>0 before a pull and 0
// after, and carries the last-sync time.
func TestStatus_ReportsAheadBehind(t *testing.T) {
	bare, storeA, storeB := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")
	syncB := newSyncer(storeB, bare, "bob")

	writeFile(t, storeA, "pricing/dossier.md", "A1")
	mustSync(t, syncA) // A pushes; B is now behind.

	st, err := syncB.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Behind < 1 {
		t.Fatalf("expected B behind >=1 before pull, got %d", st.Behind)
	}

	mustSync(t, syncB) // B pulls; now in sync.
	st, _ = syncB.Status()
	if st.Behind != 0 {
		t.Fatalf("expected B behind 0 after pull, got %d", st.Behind)
	}
	if st.LastSync.IsZero() {
		t.Fatalf("expected last sync time populated")
	}
}
