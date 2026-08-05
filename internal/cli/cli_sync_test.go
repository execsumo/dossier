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
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestService_Sync_EndToEnd(t *testing.T) {
	// Setup bare remote repo
	remoteDir := t.TempDir()
	_, err := git.PlainInit(remoteDir, true)
	if err != nil {
		t.Fatal(err)
	}

	// Setup store A
	dirA := t.TempDir()
	repoA, _ := git.PlainInit(dirA, false) // Init local repo
	repoA.CreateRemote(&gitConfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteDir},
	})
	refA := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main"))
	repoA.Storer.SetReference(refA)

	cfgA := config.Default()
	cfgA.DossierHome = dirA
	cfgA.Author = "alice"
	cfgA.Team.Remote = remoteDir
	cfgA.Team.Branch = "main"
	if err := cfgA.Save(filepath.Join(dirA, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	svcA, err := wire(dirA)
	if err != nil {
		t.Fatal(err)
	}
	svcA.Init(context.Background(), core.InitReq{YesToAll: true})

	// Setup store B
	dirB := t.TempDir()
	repoB, _ := git.PlainInit(dirB, false) // Init local repo
	repoB.CreateRemote(&gitConfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteDir},
	})
	refB := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main"))
	repoB.Storer.SetReference(refB)

	cfgB := config.Default()
	cfgB.DossierHome = dirB
	cfgB.Author = "bob"
	cfgB.Team.Remote = remoteDir
	cfgB.Team.Branch = "main"
	if err := cfgB.Save(filepath.Join(dirB, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	svcB, err := wire(dirB)
	if err != nil {
		t.Fatal(err)
	}

	// a. two stores sharing a bare remote converge after alternating Service.Sync calls
	resA, err := svcA.Promote(context.Background(), core.PromoteReq{
		Name:                   "Feature X",
		DistilledStateMarkdown: "Initial content X",
	})
	if err != nil {
		t.Fatal(err)
	}
	idA := resA.Data.(string)

	resY, err := svcA.Promote(context.Background(), core.PromoteReq{
		Name:                   "Feature Y",
		DistilledStateMarkdown: "Initial content Y",
		Force:                  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	idY := resY.Data.(string)

	syncA, err := svcA.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reportA1 := syncA.Data.(core.SyncReport)
	t.Logf("Alice sync 1 report: %+v", reportA1)
	if !syncA.OK || reportA1.Error != "" {
		t.Fatalf("svcA sync failed: %v", syncA.Data)
	}

	syncB, err := svcB.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reportB1 := syncB.Data.(core.SyncReport)
	t.Logf("Bob sync 1 report: %+v", reportB1)
	if !syncB.OK {
		t.Fatalf("svcB sync failed: %v", syncB.Data)
	}

	// b. a both-modified dossier.md produces a real conflicts/<id>.md with kind: sync_concurrent_edit
	// Alice edits X and Y
	_, err = svcA.Save(context.Background(), core.SaveReq{
		ID:                     idA,
		DistilledStateMarkdown: "Alice's change X",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svcA.Save(context.Background(), core.SaveReq{
		ID:                     idY,
		DistilledStateMarkdown: "Alice's change Y",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Bob edits X and Y (before syncing)
	_, err = svcB.Save(context.Background(), core.SaveReq{
		ID:                     idA,
		DistilledStateMarkdown: "Bob's change X",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svcB.Save(context.Background(), core.SaveReq{
		ID:                     idY,
		DistilledStateMarkdown: "Bob's change Y",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Alice syncs (pushes)
	resSyncA2, err := svcA.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reportA2 := resSyncA2.Data.(core.SyncReport)
	t.Logf("Alice sync 2 report: %+v", reportA2)
	if !resSyncA2.OK {
		t.Fatalf("svcA second sync failed: %v", resSyncA2.Data)
	}

	// Bob syncs (pulls Alice's change, resulting in conflict)
	resSyncB, err := svcB.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reportB := resSyncB.Data.(core.SyncReport)
	t.Logf("Bob sync report: %+v", reportB)
	var found bool
	for _, c := range reportB.Conflicts {
		if strings.HasSuffix(c.Path, "dossier.md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Expected dossier.md conflict on Bob's side, got conflicts: %+v", reportB.Conflicts)
	}

	// Verify Bob's store has the conflict file for feature-x
	filesX, err := os.ReadDir(filepath.Join(dirB, "feature-x", "conflicts"))
	if err != nil || len(filesX) == 0 {
		t.Fatalf("Conflict file not found in conflicts/ directory: %v", err)
	}

	// Verify Bob's store has the conflict file for feature-y
	filesY, err := os.ReadDir(filepath.Join(dirB, "feature-y", "conflicts"))
	if err != nil || len(filesY) == 0 {
		t.Fatalf("Conflict file not found in feature-y conflicts/ directory: %v", err)
	}

	if filesX[0].Name() == filesY[0].Name() {
		t.Fatalf("Conflict IDs collided! Both are %s", filesX[0].Name())
	}

	dossierPath := filepath.Join(dirB, "feature-x", "dossier.md")
	content, _ := os.ReadFile(dossierPath)
	if strings.Contains(string(content), "<<<<<<<") {
		t.Fatalf("Git conflict markers found in dossier.md")
	}

	// Remote wins the working tree: Bob's dossier.md should have Alice's change
	if !strings.Contains(string(content), "Alice's change X") {
		t.Fatalf("Remote (Alice's) change did not win working tree. Content: %s", content)
	}

	// c. an oversized (>100 MB) artifact is excluded
	sparsePath := filepath.Join(dirA, "feature-x", "large_artifact.bin")
	f, err := os.Create(sparsePath)
	if err != nil {
		t.Fatal(err)
	}
	// Seek past 100MB
	f.Seek(105*1024*1024, 0)
	f.Write([]byte("end"))
	f.Close()

	resSyncA, err := svcA.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reportA := resSyncA.Data.(core.SyncReport)
	if len(reportA.Excluded) == 0 {
		t.Fatalf("Expected oversized file to be excluded")
	}

	// d. Service.Sync with no Syncer configured returns a clear error, not a panic.
	dirC := t.TempDir()
	cfgC := config.Default()
	cfgC.DossierHome = dirC
	cfgC.Team.Remote = "" // No remote
	cfgC.Save(filepath.Join(dirC, "config.yaml"))
	svcC, _ := wire(dirC)

	_, err = svcC.Sync(context.Background())
	if err == nil || !strings.Contains(err.Error(), "team sync is not configured") {
		t.Fatalf("Expected 'team sync is not configured' error, got %v", err)
	}
}
