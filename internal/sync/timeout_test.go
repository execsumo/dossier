package sync

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
)

func blockingHTTPRemote(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		<-release
		_ = conn.Close()
	}()
	cleanup := func() {
		close(release)
		_ = listener.Close()
	}
	return "http://" + listener.Addr().String() + "/team.git", cleanup
}

func TestSyncWithContextSkipsFurtherNetworkAfterPullTimeout(t *testing.T) {
	_, storeDir, _ := setupPair(t)
	remoteURL, cleanup := blockingHTTPRemote(t)
	defer cleanup()

	repo, err := git.PlainOpen(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Remotes[originName].URLs = []string{remoteURL}
	if err := repo.Storer.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	g := New(Config{StoreDir: storeDir, RemoteURL: remoteURL, Branch: "main"})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	report, err := g.syncWithCtx(ctx)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("syncWithCtx() error = %v", err)
	}
	if report.Error == "" {
		t.Fatal("expected the pull timeout to be reported")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("sync exceeded context budget: %s", elapsed)
	}
}
