package sync

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGetAuth_FileMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")

	// 0644 should be refused
	if err := os.WriteFile(path, []byte("pat123"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := GetAuth(path, "https://github.com/foo/bar")
	if !errors.Is(err, ErrInsecureCredentials) {
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
