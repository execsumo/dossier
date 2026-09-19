package sync

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// ErrInsecureCredentials indicates the credentials file has permissions looser than 0600.
var ErrInsecureCredentials = errors.New("credentials file has insecure permissions (must be 0600)")

// ErrNoCredentials indicates that no credentials were found for an https remote.
var ErrNoCredentials = errors.New("no credentials found")

// runner is a hook for tests to intercept exec.Command
var runner = func(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).Output()
}

// GetAuth resolves the GitHub PAT and returns a basic auth configured for go-git,
// the auth state ("file", "gh", "missing", "error", "none"), and any error.
func GetAuth(credsPath string, remoteURL string) (*http.BasicAuth, string, error) {
	if credsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "missing", nil // can't find home, fallback to no auth
		}
		credsPath = home + "/.dossier/credentials"
	}

	info, err := os.Stat(credsPath)
	if err == nil {
		// Must be exactly 0600
		if info.Mode().Perm() != 0600 {
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
