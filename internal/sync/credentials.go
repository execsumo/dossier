package sync

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// ErrInsecureCredentials indicates the credentials file has permissions looser than 0600.
var ErrInsecureCredentials = errors.New("credentials file has insecure permissions (must be 0600)")

// ErrNoCredentials indicates that no credentials were found for an https remote.
var ErrNoCredentials = errors.New("no credentials found")

// runner is a hook for tests to intercept exec.Command.
var runner = func(name string, arg ...string) ([]byte, error) {
	return exec.Command(resolveGH(name), arg...).Output()
}

func resolveGH(name string) string {
	// exec.Command resolves PATHEXT on Windows, but use LookPath explicitly so
	// the installed gh.exe is found consistently there.
	if runtime.GOOS == "windows" && name == "gh" {
		if path, err := exec.LookPath("gh.exe"); err == nil {
			return path
		}
	}
	return name
}

// loginRunner is separate from runner because gh auth login must keep the
// user's terminal attached while it opens the browser and prints its code.
var loginRunner = func(name string, args []string, in io.Reader, out, errOut io.Writer) error {
	cmd := exec.Command(resolveGH(name), args...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}

// GitHubAuthStatus reports whether gh is installed and whether it is logged in
// to github.com. A non-zero auth status means installed-but-logged-out; a
// missing executable means not installed.
func GitHubAuthStatus() (installed, loggedIn bool) {
	_, err := runner("gh", "auth", "status", "--hostname", "github.com")
	if err == nil {
		return true, true
	}
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "executable file not found") {
		return false, false
	}
	return true, false
}

// GitHubLogin runs the interactive browser login with the supplied terminal
// streams. It deliberately never writes the resulting token to disk.
func GitHubLogin(in io.Reader, out, errOut io.Writer) error {
	return loginRunner("gh", []string{"auth", "login", "--hostname", "github.com", "--git-protocol", "https", "--web"}, in, out, errOut)
}

// SetGHRunnerForTests replaces the gh command hooks and returns a restore
// function. It is intentionally narrow: callers can test CLI onboarding
// without installing gh or opening a browser.
func SetGHRunnerForTests(command func(string, ...string) ([]byte, error), login func(string, []string, io.Reader, io.Writer, io.Writer) error) func() {
	oldRunner, oldLogin := runner, loginRunner
	if command != nil {
		runner = command
	}
	if login != nil {
		loginRunner = login
	}
	return func() {
		runner, loginRunner = oldRunner, oldLogin
	}
}

// GetAuth resolves the GitHub PAT and returns a basic auth configured for go-git,
// the auth state ("file", "gh", "missing", "error", "none"), and any error.
func GetAuth(credsPath string, remoteURL string) (*http.BasicAuth, string, error) {
	if credsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "missing", nil // can't find home, fallback to no auth
		}
		credsPath = filepath.Join(home, ".dossier", "credentials")
	}

	info, err := os.Stat(credsPath)
	if err == nil {
		// Windows reports synthetic Unix permission bits for ACL-protected files;
		// the user's profile ACL is the access control boundary there. Keep the
		// strict 0600 check on Unix, where those bits are meaningful.
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			return nil, "error", ErrInsecureCredentials
		}
		data, err := os.ReadFile(credsPath)
		if err != nil {
			return nil, "error", fmt.Errorf("read credentials file: %w", err)
		}
		pat := strings.TrimSpace(string(data))
		if pat != "" {
			return &http.BasicAuth{Username: "x-access-token", Password: pat}, "file", nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "error", fmt.Errorf("stat credentials file: %w", err)
	}

	// Fallback: gh auth token
	out, err := runner("gh", "auth", "token")
	if err == nil {
		pat := strings.TrimSpace(string(out))
		if pat != "" {
			return &http.BasicAuth{Username: "x-access-token", Password: pat}, "gh", nil
		}
	}

	if strings.HasPrefix(remoteURL, "http://") || strings.HasPrefix(remoteURL, "https://") {
		return nil, "missing", ErrNoCredentials
	}

	return nil, "none", nil
}
