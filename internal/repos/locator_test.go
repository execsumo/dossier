package repos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// initRepo creates a non-bare repository at dir with the given origin URL.
func initRepo(t *testing.T, dir, origin string) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{origin}}); err != nil {
			t.Fatal(err)
		}
	}
}

func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestIdentify(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "api")
	initRepo(t, repo, "git@github.com:Acme/API.git")
	sub := filepath.Join(repo, "cmd", "server")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	l := New(t.TempDir(), nil)
	id, top, ok := l.Identify(sub)
	if !ok || id != "github.com/acme/api" || top != resolvedPath(t, repo) {
		t.Errorf("Identify(sub) = %q %q %v", id, top, ok)
	}

	noOrigin := filepath.Join(root, "local-only")
	initRepo(t, noOrigin, "")
	if _, _, ok := l.Identify(noOrigin); ok {
		t.Error("a repo without origin has no identity")
	}
	if _, _, ok := l.Identify(t.TempDir()); ok {
		t.Error("a plain directory has no identity")
	}
	if _, _, ok := l.Identify(filepath.Join(root, "missing")); ok {
		t.Error("a missing directory has no identity")
	}
}

func TestLocateResolutionOrder(t *testing.T) {
	home := t.TempDir()
	rootA, rootB := t.TempDir(), t.TempDir()
	initRepo(t, filepath.Join(rootA, "api"), "https://github.com/acme/api")
	initRepo(t, filepath.Join(rootA, "web"), "https://github.com/acme/web")
	initRepo(t, filepath.Join(rootB, "web-fork"), "git@github.com:acme/web.git")
	l := New(home, []string{rootA, rootB, filepath.Join(rootA, "does-not-exist")})

	// Scan hit, then remembered.
	loc, err := l.Locate("github.com/acme/api")
	if err != nil || loc.Path != resolvedPath(t, filepath.Join(rootA, "api")) || loc.Via != "scan" {
		t.Fatalf("scan: %+v %v", loc, err)
	}
	if _, err := os.Stat(filepath.Join(home, "local", "repo-paths.json")); err != nil {
		t.Fatalf("scan hit not remembered: %v", err)
	}
	loc, _ = l.Locate("github.com/acme/api")
	if loc.Via != "scan" || loc.Path == "" {
		t.Errorf("map hit: %+v", loc)
	}

	// Several checkouts: no guess.
	loc, err = l.Locate("github.com/acme/web")
	if err != nil || loc.Path != "" || len(loc.Candidates) != 2 {
		t.Errorf("ambiguous: %+v %v", loc, err)
	}

	// Manual location wins and is used from the map.
	if err := l.Remember("github.com/acme/web", filepath.Join(rootB, "web-fork"), "manual"); err != nil {
		t.Fatal(err)
	}
	loc, _ = l.Locate("github.com/acme/web")
	if loc.Path != filepath.Join(rootB, "web-fork") || loc.Via != "manual" {
		t.Errorf("manual: %+v", loc)
	}

	// Stale entry: the directory moved. It is dropped and reported; with no
	// other checkout the repo is unresolved.
	if err := os.RemoveAll(filepath.Join(rootA, "api")); err != nil {
		t.Fatal(err)
	}
	loc, err = l.Locate("github.com/acme/api")
	if err != nil || loc.Stale != filepath.Join(rootA, "api") || loc.Path != "" {
		t.Errorf("stale: %+v %v", loc, err)
	}

	// Not anywhere.
	loc, err = l.Locate("github.com/acme/nowhere")
	if err != nil || loc.Path != "" || len(loc.Candidates) != 0 || loc.Stale != "" {
		t.Errorf("missing: %+v %v", loc, err)
	}
}

func TestCorruptMapIsAnError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "local", "repo-paths.json"), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(home, nil).Locate("github.com/acme/api"); err == nil {
		t.Error("a corrupt map must surface, not be silently replaced")
	}
}
