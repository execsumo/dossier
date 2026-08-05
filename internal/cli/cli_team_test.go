package cli

import (
	"context"
	"dossier/internal/config"
	"dossier/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// setupTeamStore writes a config.yaml with the team remote set and returns a
// wired Service, mirroring what the `dossier team` CLI command does (load →
// set Team → save → wire).
func setupTeamStore(t *testing.T, dir, author, remote string) *core.Service {
	t.Helper()
	cfg := config.Default()
	cfg.DossierHome = dir
	cfg.Author = author
	cfg.Team.Remote = remote
	cfg.Team.Branch = "main"
	if err := cfg.Save(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("save config: %v", err)
	}
	svc, err := wire(dir)
	if err != nil {
		t.Fatalf("wire %s: %v", author, err)
	}
	return svc
}

// TestService_TeamCreateJoin_RoundTrip is the committed regression test for the
// Phase 3a onboarding commands: `team create` bootstraps + pushes a store, and
// `team join` clones it into a fresh store — no phantom conflicts, and the two
// converge. Error paths (join into a non-empty store, unreachable remote) must
// surface clearly, never panic.
func TestService_TeamCreateJoin_RoundTrip(t *testing.T) {
	ctx := context.Background()

	remoteDir := t.TempDir()
	bare, err := git.PlainInit(remoteDir, true)
	if err != nil {
		t.Fatalf("init bare: %v", err)
	}
	if err := bare.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatalf("set bare HEAD: %v", err)
	}

	// --- Store A: build content, then `team create` ---
	dirA := t.TempDir()
	svcA := setupTeamStore(t, dirA, "alice", remoteDir)
	if _, err := svcA.Init(ctx, core.InitReq{YesToAll: true}); err != nil {
		t.Fatalf("A init: %v", err)
	}
	if _, err := svcA.Promote(ctx, core.PromoteReq{Name: "Pricing model", DistilledStateMarkdown: "Alice notes"}); err != nil {
		t.Fatalf("A promote: %v", err)
	}
	if _, err := svcA.TeamCreate(ctx, core.TeamCreateReq{RemoteURL: remoteDir, Branch: "main"}); err != nil {
		t.Fatalf("team create: %v", err)
	}
	// The remote must now carry A's dossier.
	if _, err := os.Stat(filepath.Join(dirA, "pricing-model", "dossier.md")); err != nil {
		t.Fatalf("A store missing its own dossier after create: %v", err)
	}

	// team create on an already-created store must fail clearly (not panic).
	if _, err := svcA.TeamCreate(ctx, core.TeamCreateReq{RemoteURL: remoteDir, Branch: "main"}); err == nil {
		t.Fatalf("second team create should fail (store already a team store)")
	}

	// --- Store B: `team join` the remote ---
	dirB := t.TempDir()
	svcB := setupTeamStore(t, dirB, "bob", remoteDir)
	if _, err := svcB.TeamJoin(ctx, core.TeamJoinReq{RemoteURL: remoteDir, Branch: "main"}); err != nil {
		t.Fatalf("team join: %v", err)
	}
	// B must have received A's dossier.
	if _, err := os.Stat(filepath.Join(dirB, "pricing-model", "dossier.md")); err != nil {
		t.Fatalf("B did not receive A's dossier after join: %v", err)
	}
	// And a status check must show zero phantom conflicts (the .gitignore fix
	// holds through the join path too).
	statusRes, err := svcB.SyncStatus(ctx)
	if err != nil {
		t.Fatalf("B sync status: %v", err)
	}
	if st, ok := statusRes.Data.(core.SyncStatus); ok {
		if len(st.Conflicts) != 0 {
			t.Fatalf("B join produced phantom conflicts: %+v", st.Conflicts)
		}
	}

	// --- Error path: join into a non-empty store ---
	if _, err := svcA.TeamJoin(ctx, core.TeamJoinReq{RemoteURL: remoteDir, Branch: "main"}); err == nil {
		t.Fatalf("join into non-empty store should fail")
	} else if !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected 'not empty' error joining a populated store, got: %v", err)
	}

	// --- Error path: unreachable remote (no panic, clear error) ---
	dirC := t.TempDir()
	svcC := setupTeamStore(t, dirC, "carol", "https://invalid.invalid/nope.git")
	if _, err := svcC.TeamCreate(ctx, core.TeamCreateReq{RemoteURL: "https://invalid.invalid/nope.git", Branch: "main"}); err == nil {
		t.Fatalf("team create against unreachable remote should error")
	}
}
