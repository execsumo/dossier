package sync

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Item 2 regression tests: machine-local junk (config.yaml backups, Finder
// metadata, *.bak) must not be published by team create, republished by sync
// when an older build already committed it, or clobbered on this machine by a
// pull.

// remoteTip returns the tree at refs/heads/main of the bare remote.
func remoteTip(t *testing.T, bareDir string) *object.Tree {
	t.Helper()
	repo, err := git.PlainOpen(bareDir)
	if err != nil {
		t.Fatalf("open bare: %v", err)
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatalf("main ref: %v", err)
	}
	c, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("tip commit: %v", err)
	}
	tree, err := c.Tree()
	if err != nil {
		t.Fatalf("tip tree: %v", err)
	}
	return tree
}

func remoteHas(t *testing.T, bareDir, path string) bool {
	t.Helper()
	_, err := remoteTip(t, bareDir).File(path)
	return err == nil
}

// legacyPublish commits files the way an older build did (its .gitignore did
// not cover them) and pushes, bypassing the current sync code.
func legacyPublish(t *testing.T, storeDir string, files map[string]string) {
	t.Helper()
	repo, err := git.PlainOpen(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		writeFile(t, storeDir, path, content)
		if _, err := wt.Add(path); err != nil {
			t.Fatalf("legacy add %s: %v", path, err)
		}
	}
	if _, err := wt.Commit("older build sync", &git.CommitOptions{Author: testSig("old"), Committer: testSig("old")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("legacy push: %v", err)
	}
}

func assertBytes(t *testing.T, dir, path, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("%s missing locally: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s local bytes = %q, want %q", path, got, want)
	}
}

func assertAbsent(t *testing.T, dir, path string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(path))); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist locally (err=%v)", path, err)
	}
}

func TestEnsureGitignoreCoversMachineLocalJunk(t *testing.T) {
	for _, tc := range []struct{ name, existing string }{
		{"fresh store", ""},
		{"store from an older build", "# Dossier sync — machine-local, never committed (managed by internal/sync)\n/config.yaml\n/credentials\n*.swp\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.existing != "" {
				writeFile(t, dir, ".gitignore", tc.existing)
			}
			if err := EnsureGitignore(dir); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
			lines := splitGitignoreLines(data)
			for _, want := range []string{"/config.yaml*", "*.bak", ".DS_Store"} {
				if !containsLine(lines, want) {
					t.Errorf(".gitignore missing %q:\n%s", want, data)
				}
			}
		})
	}
}

func TestManagedLocalMatcherScope(t *testing.T) {
	ignored := managedLocalMatcher()
	for path, want := range map[string]bool{
		"config.yaml":                  true,
		"config.yaml.bak":              true,
		".DS_Store":                    true,
		"topic/.DS_Store":              true,
		"topic/files/draft.bak":        true,
		"topic/files/config.yaml":      false, // a deliverable named config.yaml is shared work
		"topic/dossier.md":             false,
		"topic/audit/alice.log":        false,
		"topic/sessions/alice/s.jsonl": true,
	} {
		if got := ignored(path); got != want {
			t.Errorf("ignored(%q) = %v, want %v", path, got, want)
		}
	}
}

// A user's own .gitignore rule keeps git's meaning: it never untracks a file
// that is already shared.
func TestUserIgnoreRuleDoesNotUntrackSharedWork(t *testing.T) {
	bareDir, storeA, _ := setupPair(t)
	writeFile(t, storeA, "topic/files/report.md", "shared\n")
	mustSync(t, newSyncer(storeA, bareDir, "alice"))
	gi, _ := os.ReadFile(filepath.Join(storeA, ".gitignore"))
	writeFile(t, storeA, ".gitignore", string(gi)+"topic/files/report.md\n")
	writeFile(t, storeA, "topic/files/report.md", "shared v2\n")
	rep := mustSync(t, newSyncer(storeA, bareDir, "alice"))
	if len(rep.Excluded) != 0 {
		t.Fatalf("user rule produced exclusions: %+v", rep.Excluded)
	}
	f, err := remoteTip(t, bareDir).File("topic/files/report.md")
	if err != nil {
		t.Fatal("user ignore rule untracked shared work")
	}
	if c, _ := f.Contents(); c != "shared v2\n" {
		t.Fatalf("tracked shared file stopped syncing: %q", c)
	}
}

// team create must not publish machine-local junk, and must still publish a
// deliverable that happens to be named config.yaml.
func TestCreateDoesNotPublishMachineLocalJunk(t *testing.T) {
	storeDir := t.TempDir()
	for path, content := range map[string]string{
		"config.yaml":                  "author: alice\n",
		"config.yaml.bak":              "author: alice\nschema_version: 2\n",
		".DS_Store":                    "finder",
		"topic/.DS_Store":              "finder",
		"topic/dossier.md":             "# Topic\n",
		"topic/files/draft.bak":        "old draft",
		"topic/files/config.yaml":      "deliverable: yes\n",
		"archive/done/files/notes.bak": "old",
	} {
		writeFile(t, storeDir, path, content)
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	bare, err := git.PlainInit(remote, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	if err := New(Config{StoreDir: storeDir, Branch: "main"}).Create(context.Background(), remote, "main"); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, path := range []string{"config.yaml", "config.yaml.bak", ".DS_Store", "topic/.DS_Store", "topic/files/draft.bak", "archive/done/files/notes.bak"} {
		if remoteHas(t, remote, path) {
			t.Errorf("team create published machine-local %s", path)
		}
	}
	for _, path := range []string{"topic/dossier.md", "topic/files/config.yaml", ".gitignore"} {
		if !remoteHas(t, remote, path) {
			t.Errorf("team create did not publish %s", path)
		}
	}
}

// An older build committed junk; the current sync must remove it from the
// index (keeping local bytes), never push the newer local bytes, including
// bytes already staged in the index, and do so through the merge path too.
func TestSyncUntracksLegacyJunkWithoutRepublishing(t *testing.T) {
	bareDir, storeA, storeB := setupPair(t)
	legacyPublish(t, storeA, map[string]string{
		"config.yaml.bak":       "alice v1\n",
		".DS_Store":             "finder v1",
		"topic/files/draft.bak": "draft v1",
		"topic/dossier.md":      "# Topic\n",
	})
	oldBlob, _ := remoteTip(t, bareDir).File("config.yaml.bak")

	// Local edits on A; one of them is already staged, as git add would leave it.
	writeFile(t, storeA, "config.yaml.bak", "alice v2 SECRET\n")
	writeFile(t, storeA, ".DS_Store", "finder v2")
	writeFile(t, storeA, "topic/files/draft.bak", "draft v2 SECRET")
	repo, _ := git.PlainOpen(storeA)
	wt, _ := repo.Worktree()
	if _, err := wt.Add("topic/files/draft.bak"); err != nil {
		t.Fatal(err)
	}

	// B advances the remote so A's sync takes the merge path.
	mustSync(t, newSyncer(storeB, bareDir, "bob"))
	writeFile(t, storeB, "other/dossier.md", "# Other\n")
	mustSync(t, newSyncer(storeB, bareDir, "bob"))

	rep := mustSync(t, newSyncer(storeA, bareDir, "alice"))
	if rep.Error != "" {
		t.Fatalf("sync error: %s", rep.Error)
	}
	for _, path := range []string{"config.yaml.bak", ".DS_Store", "topic/files/draft.bak"} {
		if remoteHas(t, bareDir, path) {
			f, _ := remoteTip(t, bareDir).File(path)
			c, _ := f.Contents()
			t.Errorf("remote tip still carries %s (%q)", path, c)
		}
	}
	if !remoteHas(t, bareDir, "other/dossier.md") || !remoteHas(t, bareDir, "topic/dossier.md") {
		t.Fatal("ordinary content did not sync")
	}
	assertBytes(t, storeA, "config.yaml.bak", "alice v2 SECRET\n")
	assertBytes(t, storeA, ".DS_Store", "finder v2")
	assertBytes(t, storeA, "topic/files/draft.bak", "draft v2 SECRET")

	// Nothing on the remote, in any commit, carries A's v2 bytes.
	bare, _ := git.PlainOpen(bareDir)
	blobs, _ := bare.BlobObjects()
	_ = blobs.ForEach(func(b *object.Blob) error {
		r, _ := b.Reader()
		defer r.Close()
		buf := new(strings.Builder)
		_, _ = io.Copy(buf, r)
		if strings.Contains(buf.String(), "SECRET") || buf.String() == "finder v2" {
			t.Errorf("remote object store holds republished local bytes: %q", buf.String())
		}
		return nil
	})
	if oldBlob == nil {
		t.Fatal("legacy publish did not land; test setup is wrong")
	}

	warned := 0
	for _, e := range rep.Excluded {
		if strings.Contains(e.Warning, "stopped syncing") {
			warned++
		}
	}
	if warned != 3 {
		t.Fatalf("want 3 visible 'stopped syncing' warnings, got %+v", rep.Excluded)
	}
	// A second sync is quiet: the files are no longer tracked.
	if rep2 := mustSync(t, newSyncer(storeA, bareDir, "alice")); len(rep2.Excluded) != 0 {
		t.Fatalf("second sync still reports %+v", rep2.Excluded)
	}

	// B's pull of the removal (merge path: B has its own commit) keeps B's bytes.
	writeFile(t, storeB, "config.yaml.bak", "bob own\n")
	writeFile(t, storeB, "other/dossier.md", "# Other\nbob edit\n")
	mustSync(t, newSyncer(storeB, bareDir, "bob"))
	assertBytes(t, storeB, "config.yaml.bak", "bob own\n")
}

// A fast-forward pull of an older build's commit must neither plant the
// teammate's machine-local file nor replace this machine's own copy, and must
// not write through a symlink.
func TestFastForwardDoesNotPlantOrReplaceIgnoredFiles(t *testing.T) {
	bareDir, storeA, storeB := setupPair(t)

	writeFile(t, storeB, "config.yaml.bak", "bob own\n")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOK := runtime.GOOS != "windows" && os.Symlink(outside, filepath.Join(storeB, "topic.bak")) == nil

	legacyPublish(t, storeA, map[string]string{
		"config.yaml.bak":       "alice\n",
		".DS_Store":             "alice finder",
		"topic.bak":             "alice bak",
		"topic/files/draft.bak": "alice draft",
		"topic/dossier.md":      "# Topic\n",
	})

	rep := mustSync(t, newSyncer(storeB, bareDir, "bob"))
	if !rep.Pulled {
		t.Fatal("expected a fast-forward pull")
	}
	assertBytes(t, storeB, "topic/dossier.md", "# Topic\n")
	assertBytes(t, storeB, "config.yaml.bak", "bob own\n")
	assertAbsent(t, storeB, ".DS_Store")
	assertAbsent(t, storeB, "topic/files/draft.bak")
	if symlinkOK {
		target, err := os.Readlink(filepath.Join(storeB, "topic.bak"))
		if err != nil || target != outside {
			t.Fatalf("symlink replaced: target=%q err=%v", target, err)
		}
		assertBytes(t, filepath.Dir(outside), filepath.Base(outside), "outside")
	}

	// The fast-forward itself pushes nothing, so the older build's entries stay
	// in the tip until B's next sync, which removes them and keeps B's file.
	mustSync(t, newSyncer(storeB, bareDir, "bob"))
	for _, path := range []string{"config.yaml.bak", ".DS_Store", "topic.bak", "topic/files/draft.bak"} {
		if remoteHas(t, bareDir, path) {
			t.Errorf("legacy %s still in the remote tip after B's next sync", path)
		}
	}
	assertBytes(t, storeB, "config.yaml.bak", "bob own\n")
	bare, _ := git.PlainOpen(bareDir)
	blobs, _ := bare.BlobObjects()
	_ = blobs.ForEach(func(b *object.Blob) error {
		r, _ := b.Reader()
		defer r.Close()
		buf := new(strings.Builder)
		_, _ = io.Copy(buf, r)
		if buf.String() == "bob own\n" {
			t.Error("B's machine-local bytes reached the remote")
		}
		return nil
	})
}
