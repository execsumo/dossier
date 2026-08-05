package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func testSig(name string) *object.Signature {
	return &object.Signature{Name: name, Email: name + "@dossier.local", When: time.Now()}
}

// seedBareRepo creates a bare repo at bareDir with an initial commit containing
// README.md and the default machine-local .gitignore, so cloned stores inherit
// the gitignore (rather than each machine independently recreating it, which
// would itself look like a both-added conflict).
func seedBareRepo(t *testing.T, bareDir string) {
	t.Helper()
	bare, err := git.PlainInit(bareDir, true)
	if err != nil {
		t.Fatalf("plaininit bare: %v", err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatalf("set bare HEAD: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "seed")
	repo, err := git.PlainInit(tmp, false)
	if err != nil {
		t.Fatalf("plaininit seed: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatalf("set seed HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# team store\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := EnsureGitignore(tmp); err != nil {
		t.Fatalf("ensure gitignore: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add readme: %v", err)
	}
	if _, err := wt.Add(".gitignore"); err != nil {
		t.Fatalf("add gitignore: %v", err)
	}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: testSig("seed"), Committer: testSig("seed"),
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{bareDir}}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	if err := repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec("refs/heads/main:refs/heads/main")},
	}); err != nil {
		t.Fatalf("push seed: %v", err)
	}
}

// setupPair seeds a bare repo and clones two stores (machine A + B).
func setupPair(t *testing.T) (bareDir, storeA, storeB string) {
	t.Helper()
	bareDir = filepath.Join(t.TempDir(), "bare.git")
	seedBareRepo(t, bareDir)
	storeA = filepath.Join(t.TempDir(), "storeA")
	storeB = filepath.Join(t.TempDir(), "storeB")
	for _, dir := range []string{storeA, storeB} {
		g := New(Config{StoreDir: dir, RemoteURL: bareDir, Branch: "main"})
		if err := g.Clone(bareDir, dir, 0); err != nil {
			t.Fatalf("clone %s: %v", dir, err)
		}
	}
	return bareDir, storeA, storeB
}

func newSyncer(storeDir, bareDir, author string) *GitSync {
	return New(Config{
		StoreDir:    storeDir,
		RemoteURL:   bareDir,
		Branch:      "main",
		AuthorName:  author,
		AuthorEmail: author + "@dossier.local",
		LockTimeout: 10 * time.Second,
	})
}

func writeFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustSync(t *testing.T, g *GitSync) SyncReport {
	t.Helper()
	rep, err := g.Sync()
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	return rep
}

// writeFileErr is the non-*testing.T variant of writeFile for use inside
// goroutines (where t.Fatalf is unsafe). Returns the error instead.
func writeFileErr(dir, path, content string) error {
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

// sparseBigFile creates a sparse file of the given size at path (a hole + one
// real byte) so the test does not actually allocate 100+ MB on disk.
func sparseBigFile(t *testing.T, dir, path string, size int64) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(full)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := f.WriteAt([]byte("X"), size-1); err != nil {
		t.Fatalf("writeat: %v", err)
	}
}
