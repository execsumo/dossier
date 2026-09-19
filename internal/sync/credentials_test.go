package sync

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGitHubAuthStatusAndLogin(t *testing.T) {
	origRunner, origLogin := runner, loginRunner
	defer func() {
		runner = origRunner
		loginRunner = origLogin
	}()

	var calls []string
	runner = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if len(args) == 4 && args[0] == "auth" && args[1] == "status" {
			return nil, errors.New("not logged in")
		}
		return []byte("token\n"), nil
	}
	installed, loggedIn := GitHubAuthStatus()
	if !installed || loggedIn {
		t.Fatalf("GitHubAuthStatus() = %v, %v", installed, loggedIn)
	}
	var gotName string
	var gotArgs []string
	var gotIn io.Reader
	var gotOut, gotErr io.Writer
	loginRunner = func(name string, args []string, in io.Reader, out, errOut io.Writer) error {
		gotName, gotArgs = name, args
		gotIn, gotOut, gotErr = in, out, errOut
		return nil
	}
	in, out, errOut := strings.NewReader(""), &strings.Builder{}, &strings.Builder{}
	if err := GitHubLogin(in, out, errOut); err != nil {
		t.Fatal(err)
	}
	if gotName != "gh" || strings.Join(gotArgs, " ") != "auth login --hostname github.com --git-protocol https --web" {
		t.Fatalf("unexpected login command: %s %v", gotName, gotArgs)
	}
	if gotIn != in || gotOut != out || gotErr != errOut {
		t.Fatal("GitHubLogin did not preserve terminal streams")
	}
	if len(calls) != 1 || calls[0] != "gh auth status --hostname github.com" {
		t.Fatalf("unexpected auth status calls: %v", calls)
	}
}

func TestGitHubAuthStatusMissing(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()
	runner = func(string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }
	installed, loggedIn := GitHubAuthStatus()
	if installed || loggedIn {
		t.Fatalf("missing gh reported as installed/logged in: %v, %v", installed, loggedIn)
	}
}

func TestGetAuth_FileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	// 0644 should be refused
	if err := os.WriteFile(path, []byte("pat123"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := GetAuth(path, "https://github.com/foo/bar")
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Windows ACLs, not Unix mode bits, protect credentials: %v", err)
		}
	} else if !errors.Is(err, ErrInsecureCredentials) {
		t.Fatalf("expected ErrInsecureCredentials for 0644, got %v", err)
	}

	// 0600 should be accepted
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}

	auth, _, err := GetAuth(path, "https://github.com/foo/bar")
	if err != nil {
		t.Fatalf("expected success for 0600, got %v", err)
	}
	if auth == nil || auth.Password != "pat123" {
		t.Fatalf("expected password pat123, got %v", auth)
	}
}

func TestGetAuth_FallbackGH(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist") // File is absent

	// Mock the runner to return a token
	origRunner := runner
	defer func() { runner = origRunner }()

	runner = func(name string, arg ...string) ([]byte, error) {
		if name == "gh" && len(arg) == 2 && arg[0] == "auth" && arg[1] == "token" {
			return []byte("gh_pat_456\n"), nil
		}
		return nil, errors.New("command failed")
	}

	auth, _, err := GetAuth(path, "https://github.com/foo/bar")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if auth == nil || auth.Password != "gh_pat_456" {
		t.Fatalf("expected password gh_pat_456, got %v", auth)
	}
}

func TestGetAuth_HTTPSNoCreds(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()
	runner = func(name string, arg ...string) ([]byte, error) {
		return nil, errors.New("gh not found")
	}
	_, state, err := GetAuth("/does-not-exist", "https://github.com/foo/bar.git")
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
	if state != "missing" {
		t.Errorf("expected state missing, got %s", state)
	}
}

func TestGetAuth_Fallback(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()
	runner = func(name string, arg ...string) ([]byte, error) {
		return []byte("fake_token\n"), nil
	}
	auth, state, err := GetAuth("/does-not-exist", "https://github.com/foo/bar.git")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if state != "gh" || auth.Password != "fake_token" {
		t.Errorf("expected state gh and fake_token, got %s / %v", state, auth)
	}

	runner = func(name string, arg ...string) ([]byte, error) {
		return nil, errors.New("not found")
	}
	_, state, err = GetAuth("/does-not-exist", "file:///tmp/repo")
	if err != nil {
		t.Errorf("expected no error for local path, got %v", err)
	}
	if state != "none" {
		t.Errorf("expected state none, got %s", state)
	}
}
