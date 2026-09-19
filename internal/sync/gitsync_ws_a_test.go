package sync

import (
	"context"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSync_AuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="GitHub"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer srv.Close()

	storeA := t.TempDir()
	repo, _ := git.PlainInit(storeA, false)
	repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{srv.URL}})
	cfg := Config{
		AuthorName: "Test",
		RemoteURL:  srv.URL,
		StoreDir:   storeA,
		Branch:     "main",
		AuthState:  "missing",
	}
	syncA := New(cfg)

	// Since remote is httptest, it will return 401.
	report, err := syncA.syncWithCtx(context.Background())
	// auth error from fetch
	if err != nil {
		t.Fatalf("unexpected error, got %v", err)
	}
	if !report.AuthFailed {
		t.Logf("report: %+v", report)
	}
	if !report.AuthFailed || report.Error == "" {
		t.Errorf("expected AuthFailed=true and Error!=empty")
	}

	st := loadState(storeA)
	if st.AuthState != "rejected" {
		t.Errorf("expected state 'rejected', got %s", st.AuthState)
	}
}

func TestSync_NoOp(t *testing.T) {
	bare, storeA, _ := setupPair(t)
	syncA := newSyncer(storeA, bare, "alice")

	// first sync
	writeFile(t, storeA, "pricing/dossier.md", "v1")
	mustSync(t, syncA)

	// second sync (no-op)
	rep2, err := syncA.syncWithCtx(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep2.Pushed {
		t.Errorf("expected Pushed to be false for a no-op sync")
	}
}
