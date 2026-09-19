package sync

import (
	"context"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
	"dossier/internal/store"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type rawPromoteClock struct{ now time.Time }

func (c rawPromoteClock) Now() time.Time { return c.now }

type rawPromoteTokenizer struct{}

func (rawPromoteTokenizer) Estimate(text string) int { return len(text) }

type rawPromoteSearcher struct{}

func (rawPromoteSearcher) Search(context.Context, string, core.SearchScope) ([]core.Hit, error) {
	return nil, nil
}

type rawPromoteHarnesses struct{}

func (rawPromoteHarnesses) All() []core.Harness              { return nil }
func (rawPromoteHarnesses) Get(string) (core.Harness, error) { return nil, nil }

func TestPromoteRawTranscriptStaysLocalAcrossTeamSync(t *testing.T) {
	bare, storeA, storeB := setupPair(t)
	fsA := store.NewFSStore(storeA)
	if err := fsA.Init(); err != nil {
		t.Fatal(err)
	}
	svc := core.NewService(fsA, rawPromoteSearcher{}, rawPromoteTokenizer{}, rawPromoteHarnesses{}, rawPromoteClock{now: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)}, core.Config{DossierHome: storeA, Author: "alice"}, nil)
	content := `{"type":"message","message":{"role":"assistant","content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"visible"}]}}`
	if _, err := svc.Promote(context.Background(), core.PromoteReq{Name: "Raw sync", Content: content, Force: true}); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	mustSync(t, newSyncer(storeA, bare, "alice"))

	repo, err := git.PlainOpen(bare)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	compiled := false
	if err := tree.Files().ForEach(func(file *object.File) error {
		body, err := file.Contents()
		if err != nil {
			return err
		}
		if strings.Contains(body, "secret") {
			t.Fatalf("secret thinking text leaked into remote blob %s", file.Name)
		}
		if strings.Contains(file.Name, "/artifacts/") && !strings.Contains(file.Name, "_raw.") {
			compiled = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !compiled {
		t.Fatal("compiled transcript artifact was not synced")
	}

	mustSync(t, newSyncer(storeB, bare, "bob"))
	fsB := store.NewFSStore(storeB)
	if err := fsB.Init(); err != nil {
		t.Fatal(err)
	}
	cloneSvc := core.NewService(fsB, rawPromoteSearcher{}, rawPromoteTokenizer{}, rawPromoteHarnesses{}, rawPromoteClock{}, core.Config{DossierHome: storeB}, nil)
	result, err := cloneSvc.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	report := result.Data.(core.DoctorReport)
	for _, issue := range report.Issues {
		if strings.Contains(issue, "artifact") {
			t.Fatalf("second clone Doctor reported artifact issue: %s", issue)
		}
	}
}
