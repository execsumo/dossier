package cli

import (
	"bytes"
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
	"dossier/internal/core"
	dossiersync "dossier/internal/sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func executeM6(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	dossierHomeFlag = ""
	cmd := NewRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func fakeGH(t *testing.T, token func() ([]byte, error), login func(string, []string, io.Reader, io.Writer, io.Writer) error) {
	t.Helper()
	restore := dossiersync.SetGHRunnerForTests(func(name string, args ...string) ([]byte, error) {
		if name != "gh" {
			return nil, errors.New("unexpected command " + name)
		}
		if len(args) == 2 && args[0] == "auth" && args[1] == "token" {
			return token()
		}
		if len(args) == 4 && args[0] == "auth" && args[1] == "status" {
			return nil, errors.New("not logged in")
		}
		return nil, errors.New("unexpected gh args")
	}, login)
	t.Cleanup(restore)
}

func TestTeamSetupMissingGHLeavesStoreUntouched(t *testing.T) {
	for _, subcommand := range []string{"join", "create"} {
		t.Run(subcommand, func(t *testing.T) {
			store, home := t.TempDir(), t.TempDir()
			t.Setenv("DOSSIER_HOME", store)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			restore := dossiersync.SetGHRunnerForTests(func(string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }, nil)
			t.Cleanup(restore)

			_, stderr, err := executeM6(t, "", "team", subcommand, "https://github.com/acme/team.git")
			if err == nil || !strings.Contains(stderr, "brew install gh") || !strings.Contains(stderr, "winget install --id GitHub.cli") || !strings.Contains(stderr, "~/.dossier/credentials") {
				t.Fatalf("missing gh: err=%v stderr=%q", err, stderr)
			}
			if _, statErr := os.Stat(filepath.Join(store, ".git")); !os.IsNotExist(statErr) {
				t.Fatalf("missing gh changed store: %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(store, "config.yaml")); !os.IsNotExist(statErr) {
				t.Fatalf("missing gh wrote team config: %v", statErr)
			}
		})
	}
}

func TestTeamJoinLoggedOutPromptsAndRunsExactLogin(t *testing.T) {
	store, home := t.TempDir(), t.TempDir()
	t.Setenv("DOSSIER_HOME", store)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	loggedIn := false
	var loginName string
	var loginArgs []string
	fakeGH(t, func() ([]byte, error) {
		if loggedIn {
			return []byte("token\n"), nil
		}
		return nil, errors.New("not logged in")
	}, func(name string, args []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		loginName, loginArgs = name, append([]string(nil), args...)
		loggedIn = true
		return nil
	})
	oldInteractive := interactiveReader
	interactiveReader = func(io.Reader) bool { return true }
	t.Cleanup(func() { interactiveReader = oldInteractive })

	server := testGitHTTPServer(401, "")
	defer server.Close()
	stdout, stderr, err := executeM6(t, "y\n", "team", "join", server.URL+"/acme/team.git")
	if err == nil || !strings.Contains(stderr, "dossier signin") {
		t.Fatalf("login flow should reach access check: err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if loginName != "gh" || strings.Join(loginArgs, " ") != "auth login --hostname github.com --git-protocol https --web" {
		t.Fatalf("login command = %s %v", loginName, loginArgs)
	}
}

func TestTeamJoinDeclineAndNonInteractiveNeverLogin(t *testing.T) {
	for _, tt := range []struct {
		name        string
		stdin       string
		interactive bool
		wantPrompt  bool
	}{
		{"declined", "n\n", true, true},
		{"non-interactive", "y\n", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, home := t.TempDir(), t.TempDir()
			t.Setenv("DOSSIER_HOME", store)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			loginCalled := false
			fakeGH(t, func() ([]byte, error) { return nil, errors.New("not logged in") }, func(string, []string, io.Reader, io.Writer, io.Writer) error {
				loginCalled = true
				return nil
			})
			oldInteractive := interactiveReader
			interactiveReader = func(io.Reader) bool { return tt.interactive }
			t.Cleanup(func() { interactiveReader = oldInteractive })

			stdout, stderr, err := executeM6(t, tt.stdin, "team", "join", "https://github.com/acme/team.git")
			if err == nil || loginCalled || !strings.Contains(stderr, "credentials") {
				t.Fatalf("%s: err=%v stdout=%q stderr=%q login=%v", tt.name, err, stdout, stderr, loginCalled)
			}
			if tt.wantPrompt != strings.Contains(stdout, "Sign in to GitHub now?") {
				t.Fatalf("%s: prompt mismatch stdout=%q", tt.name, stdout)
			}
			if _, statErr := os.Stat(filepath.Join(store, ".git")); !os.IsNotExist(statErr) {
				t.Fatalf("%s changed store: %v", tt.name, statErr)
			}
			if _, statErr := os.Stat(filepath.Join(store, "config.yaml")); !os.IsNotExist(statErr) {
				t.Fatalf("%s wrote team config: %v", tt.name, statErr)
			}
		})
	}
}

func TestTeamJoinLoggedInDoesNotPrompt(t *testing.T) {
	store, home := t.TempDir(), t.TempDir()
	t.Setenv("DOSSIER_HOME", store)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	loginCalled := false
	fakeGH(t, func() ([]byte, error) { return []byte("token\n"), nil }, func(string, []string, io.Reader, io.Writer, io.Writer) error {
		loginCalled = true
		return nil
	})
	server := testGitHTTPServer(401, "")
	defer server.Close()
	stdout, _, err := executeM6(t, "", "team", "join", server.URL+"/acme/team.git")
	if err == nil || loginCalled || strings.Contains(stdout, "Sign in to GitHub now?") {
		t.Fatalf("logged-in flow prompted or did not reach access check: err=%v stdout=%q login=%v", err, stdout, loginCalled)
	}
}

func TestRemoteAccessMessagesThroughCLI(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
		status           int
	}{
		{"401", "", "dossier signin", 401},
		{"403", "", "can't see", 403},
		{"404", "", "can't see", 404},
		{"saml", "SAML SSO authorization required", "authorize the GitHub CLI", 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, home := t.TempDir(), t.TempDir()
			t.Setenv("DOSSIER_HOME", store)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			fakeGH(t, func() ([]byte, error) { return []byte("token\n"), nil }, nil)
			server := testGitHTTPServer(tt.status, tt.body)
			defer server.Close()
			_, stderr, err := executeM6(t, "", "team", "join", server.URL+"/acme/team.git")
			if err == nil || !strings.Contains(strings.ToLower(stderr), strings.ToLower(tt.want)) {
				t.Fatalf("err=%v stderr=%q want %q", err, stderr, tt.want)
			}
		})
	}
}

func TestTeamJoinPrintsRosterLine(t *testing.T) {
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
	if _, _, err := executeM6(t, "", "init", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeM6(t, "", "promote", "Team topic"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeM6(t, "", "team", "create", remote, "--yes", "--name", "Manager Name"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeM6(t, "", "team", "add", "guest", "Guest Name"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := executeM6(t, "", "sync"); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		author, want string
	}{
		{"guest", "You'll appear to teammates as Guest Name (guest)."},
		{"stranger", "You're not in the team roster yet. Ask Manager Name to run: dossier team add stranger \"Your Name\""},
	} {
		t.Run(tt.author, func(t *testing.T) {
			store, home := filepath.Join(root, tt.author), filepath.Join(root, tt.author+"-home")
			t.Setenv("DOSSIER_HOME", store)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			if err := os.MkdirAll(store, 0700); err != nil {
				t.Fatal(err)
			}
			cfg := config.Default()
			cfg.DossierHome = store
			cfg.Author = tt.author
			if err := cfg.SaveDefault(filepath.Join(store, "config.yaml")); err != nil {
				t.Fatal(err)
			}
			stdout, _, err := executeM6(t, "", "team", "join", remote)
			if err != nil || !strings.Contains(stdout, tt.want) {
				t.Fatalf("join output: err=%v stdout=%q want=%q", err, stdout, tt.want)
			}
		})
	}
}

func TestJoinRosterMessages(t *testing.T) {
	member := joinRosterMessage(core.Roster{Manager: "manager", Members: map[string]string{"alice": "Alice Able"}}, "alice", "alice")
	if member != "You'll appear to teammates as Alice Able (alice)." {
		t.Fatalf("member message = %q", member)
	}
	notMember := joinRosterMessage(core.Roster{Manager: "manager", Members: map[string]string{"manager": "Manager Name"}}, "alice", "alice")
	if notMember != "You're not in the team roster yet. Ask Manager Name to run: dossier team add alice \"Your Name\"" {
		t.Fatalf("non-member message = %q", notMember)
	}
	if got := joinRosterMessage(core.Roster{}, "alice", "alice"); got != "" {
		t.Fatalf("empty roster message = %q", got)
	}
}

func TestSigninGHAndTokenFileMessages(t *testing.T) {
	store, home := t.TempDir(), t.TempDir()
	t.Setenv("DOSSIER_HOME", store)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	fakeGH(t, func() ([]byte, error) { return []byte("token\n"), nil }, nil)
	stdout, _, err := executeM6(t, "", "signin")
	if err != nil || !strings.Contains(stdout, "active via gh") {
		t.Fatalf("gh signin: err=%v stdout=%q", err, stdout)
	}

	if err := os.MkdirAll(filepath.Join(home, ".dossier"), 0700); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(home, ".dossier", "credentials")
	if err := os.WriteFile(credentials, []byte("file-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fakeGH(t, func() ([]byte, error) { return nil, errors.New("gh not used") }, nil)
	stdout, _, err = executeM6(t, "", "signin")
	if err != nil || !strings.Contains(stdout, "active via ~/.dossier/credentials") {
		t.Fatalf("file signin: err=%v stdout=%q", err, stdout)
	}
}

func testGitHTTPServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			_, _ = io.WriteString(w, body)
		}
	}))
}
