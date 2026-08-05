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

// runner is a hook for tests to intercept exec.Command
var runner = func(name string, arg ...string) ([]byte, error) {
	return exec.Command(name, arg...).Output()
}

// GetAuth resolves the GitHub PAT and returns a basic auth configured for go-git.
// Returns nil, nil if no auth is available (for local bare repos).
func GetAuth(credsPath string) (*http.BasicAuth, error) {
	if credsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil // can't find home, fallback to no auth
		}
		credsPath = home + "/.dossier/credentials"
	}

	info, err := os.Stat(credsPath)
	if err == nil {
		// Must be exactly 0600
		if info.Mode().Perm() != 0600 {
			return nil, ErrInsecureCredentials
		}
		data, err := os.ReadFile(credsPath)
		if err != nil {
			return nil, fmt.Errorf("read credentials file: %w", err)
		}
		pat := strings.TrimSpace(string(data))
		if pat != "" {
			return &http.BasicAuth{Username: "x-access-token", Password: pat}, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat credentials file: %w", err)
	}

	// Fallback: gh auth token
	out, err := runner("gh", "auth", "token")
	if err == nil {
		pat := strings.TrimSpace(string(out))
		if pat != "" {
			return &http.BasicAuth{Username: "x-access-token", Password: pat}, nil
		}
	}

	return nil, nil
}
