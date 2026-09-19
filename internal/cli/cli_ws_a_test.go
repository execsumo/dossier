package cli

import (
	"bytes"
	"context"
	"dossier/internal/store"
	"dossier/internal/sync"
	github_git "github.com/go-git/go-git/v5"
	github_config "github.com/go-git/go-git/v5/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSync_UnreachableRemote(t *testing.T) {
	tmpDir := t.TempDir()

	dossierPath := filepath.Join(tmpDir, "dossier")
	cmd := exec.Command("go", "build", "-o", dossierPath, "../../cmd/dossier")
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	storeDir := filepath.Join(tmpDir, "store")
	os.MkdirAll(storeDir, 0700)
	repo, _ := github_git.PlainInit(storeDir, false)
	repo.CreateRemote(&github_config.RemoteConfig{Name: "origin", URLs: []string{"/does-not-exist/repo.git"}})
	os.WriteFile(filepath.Join(storeDir, "config.yaml"), []byte("team:\n  remote: /does-not-exist/repo.git\n  branch: main\n"), 0600)

	syncCmd := exec.Command(dossierPath, "sync")
	syncCmd.Env = append(os.Environ(), "DOSSIER_HOME="+storeDir)
	var out bytes.Buffer
	syncCmd.Stdout = &out
	syncCmd.Stderr = &out

	err := syncCmd.Run()
	if err == nil {
		t.Errorf("expected non-zero exit, got nil")
	}

	output := out.String()
	if strings.Contains(output, "Sync successful") {
		t.Errorf("output should not contain 'successful', got:\n%s", output)
	}

	_ = store.NewFSStore(storeDir)

	gs := sync.New(sync.Config{
		StoreDir:  storeDir,
		RemoteURL: "/does-not-exist/repo.git",
	})

	st, _ := gs.Status(context.Background())
	if !st.LastSuccessPull.IsZero() || !st.LastSuccessPush.IsZero() {
		t.Errorf("expected success times to be zero")
	}
	if st.LastAttempt.IsZero() || st.LastError == "" {
		t.Errorf("expected last attempt/error to be set, output:\n%s", output)
	}
}
