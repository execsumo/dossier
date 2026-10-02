package cli

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dossier/internal/config"
	dossiersync "dossier/internal/sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// A headless joiner (a VM agent) has no TTY and no gh. A token file at
// ~/.dossier/credentials must be enough: no sign-in prompt, no gh login, and the
// token must reach the remote as basic auth.
func TestTeamJoinHeadlessTokenFileNeverPromptsOrNeedsGH(t *testing.T) {
	store, home := t.TempDir(), t.TempDir()
	t.Setenv("DOSSIER_HOME", store)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".dossier"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".dossier", "credentials"), []byte("fine-grained-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loginCalled := false
	restore := dossiersync.SetGHRunnerForTests(func(string, ...string) ([]byte, error) { return nil, exec.ErrNotFound },
		func(string, []string, io.Reader, io.Writer, io.Writer) error { loginCalled = true; return nil })
	t.Cleanup(restore)
	oldInteractive := interactiveReader
	interactiveReader = func(io.Reader) bool { return false }
	t.Cleanup(func() { interactiveReader = oldInteractive })

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	stdout, stderr, err := executeM6(t, "", "team", "join", server.URL+"/acme/team.git")
	if err == nil {
		t.Fatalf("join against a 404 remote should fail")
	}
	if loginCalled || strings.Contains(stdout, "Sign in to GitHub now?") || strings.Contains(stderr, "interactive terminal") || strings.Contains(stderr, "brew install gh") {
		t.Fatalf("token-file join prompted or demanded gh: stdout=%q stderr=%q login=%v", stdout, stderr, loginCalled)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:fine-grained-token"))
	if gotAuth != want {
		t.Fatalf("remote saw Authorization %q, want %q", gotAuth, want)
	}
}

// An agent author joins with no TTY and no gh, and is shown as an agent roster
// member under its own author name.
func TestTeamJoinHeadlessAgentAuthorSucceeds(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	bare, err := git.PlainInit(remote, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	manager, managerHome := filepath.Join(root, "manager"), filepath.Join(root, "manager-home")
	t.Setenv("DOSSIER_HOME", manager)
	t.Setenv("HOME", managerHome)
	t.Setenv("USERPROFILE", managerHome)
	for _, args := range [][]string{
		{"init", "--yes"},
		{"promote", "Team topic"},
		{"team", "create", remote, "--yes", "--name", "Manager Name"},
		{"team", "add", "sitroom", "Sit Room", "--kind", "agent"},
		{"sync"},
	} {
		if _, stderr, err := executeM6(t, "", args...); err != nil {
			t.Fatalf("%v: %v %s", args, err, stderr)
		}
	}

	store, home := filepath.Join(root, "vm"), filepath.Join(root, "vm-home")
	t.Setenv("DOSSIER_HOME", store)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(store, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.DossierHome = store
	cfg.Author = "sitroom"
	if err := cfg.SaveDefault(filepath.Join(store, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	restore := dossiersync.SetGHRunnerForTests(func(string, ...string) ([]byte, error) { return nil, errors.New("gh must not be used") }, nil)
	t.Cleanup(restore)
	oldInteractive := interactiveReader
	interactiveReader = func(io.Reader) bool { return false }
	t.Cleanup(func() { interactiveReader = oldInteractive })

	stdout, stderr, err := executeM6(t, "", "team", "join", remote)
	if err != nil || !strings.Contains(stdout, "You'll appear to teammates as Sit Room (sitroom).") {
		t.Fatalf("headless agent join: err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if matches, _ := filepath.Glob(filepath.Join(store, "*", "dossier.md")); len(matches) == 0 {
		t.Fatalf("joined store has no dossier files under %s", store)
	}
}
