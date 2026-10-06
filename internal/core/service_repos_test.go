package core_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"dossier/internal/core"
	"dossier/internal/store"
)

// fakeLocator resolves from fixed tables and records what it learns.
type fakeLocator struct {
	checkouts  map[string][2]string // dir -> {identity, root}
	located    map[string]core.RepoLocation
	remembered map[string]string
}

func (f *fakeLocator) Identify(dir string) (string, string, bool) {
	c, ok := f.checkouts[dir]
	return c[0], c[1], ok
}

func (f *fakeLocator) Locate(identity string) (core.RepoLocation, error) {
	if p, ok := f.remembered[identity]; ok {
		return core.RepoLocation{Path: p, Via: "session"}, nil
	}
	return f.located[identity], nil
}

func (f *fakeLocator) Remember(identity, path, via string) error {
	f.remembered[identity] = path
	return nil
}

func repoService(t *testing.T, repos []string) (*core.Service, *store.FakeStore, *fakeLocator) {
	t.Helper()
	st := store.NewFakeStore()
	now := time.Now()
	st.Dossiers["dos_1"] = &core.Dossier{Frontmatter: core.Frontmatter{
		ID: "dos_1", Name: "Billing", Slug: "billing", Status: core.StatusExecute, Priority: core.PriorityHigh,
		CreatedAt: now, UpdatedAt: now, Repos: repos,
	}}
	svc := core.NewService(st, nil, attentionTokenizer{}, nil, &attentionClock{now: now}, core.Config{Author: "herwin"}, nil)
	loc := &fakeLocator{checkouts: map[string][2]string{}, located: map[string]core.RepoLocation{}, remembered: map[string]string{}}
	svc.SetRepoLocator(loc)
	return svc, st, loc
}

func TestLaunchTarget(t *testing.T) {
	ctx := context.Background()

	svc, _, _ := repoService(t, nil)
	res, err := svc.LaunchTarget(ctx, "dos_1")
	if err != nil {
		t.Fatal(err)
	}
	tgt := res.Data.(core.LaunchTarget)
	if tgt.WorkDir != tgt.DossierDir || len(tgt.Repos) != 0 || len(res.Warnings) != 0 {
		t.Errorf("no repos: %+v %v", tgt, res.Warnings)
	}

	svc, _, loc := repoService(t, []string{"github.com/acme/api", "github.com/acme/web", "github.com/acme/infra"})
	loc.located["github.com/acme/api"] = core.RepoLocation{Path: "/src/api", Via: "scan"}
	loc.located["github.com/acme/web"] = core.RepoLocation{Candidates: []string{"/a/web", "/b/web"}}
	res, _ = svc.LaunchTarget(ctx, "dos_1")
	tgt = res.Data.(core.LaunchTarget)
	if tgt.WorkDir != "/src/api" || len(tgt.Repos) != 3 || tgt.Repos[1].Path != "" {
		t.Errorf("resolved primary: %+v", tgt)
	}
	joined := ""
	for _, w := range res.Warnings {
		joined += string(w) + "\n"
	}
	for _, want := range []string{"several checkouts of github.com/acme/web", "infra (github.com/acme/infra) isn't on this machine", "dossier repo locate billing"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q:\n%s", want, joined)
		}
	}

	// Unresolved primary: stay in the Dossier folder and say so.
	svc, _, _ = repoService(t, []string{"github.com/acme/api"})
	res, _ = svc.LaunchTarget(ctx, "dos_1")
	tgt = res.Data.(core.LaunchTarget)
	if tgt.WorkDir != tgt.DossierDir || len(res.Warnings) != 2 {
		t.Errorf("unresolved primary: %+v %v", tgt, res.Warnings)
	}
}

func TestObserveSessionDir(t *testing.T) {
	ctx := context.Background()
	svc, st, loc := repoService(t, []string{"github.com/acme/api"})
	loc.checkouts["/src/api/cmd"] = [2]string{"github.com/acme/api", "/src/api"}
	loc.checkouts["/src/other"] = [2]string{"github.com/acme/other", "/src/other"}

	res := svc.ObserveSessionDir(ctx, "dos_1", "/src/api/cmd")
	if loc.remembered["github.com/acme/api"] != "/src/api" || len(res.NextActions) != 0 {
		t.Errorf("listed repo should be learned silently: %v %v", loc.remembered, res.NextActions)
	}

	res = svc.ObserveSessionDir(ctx, "dos_1", "/src/other")
	if len(res.NextActions) != 1 || !strings.Contains(string(res.NextActions[0]), "github.com/acme/other") {
		t.Errorf("unlisted repo should be suggested: %v", res.NextActions)
	}
	if got := st.Dossiers["dos_1"].Frontmatter.Repos; len(got) != 1 {
		t.Errorf("observing must never change the Dossier, repos = %v", got)
	}
	if _, ok := loc.remembered["github.com/acme/other"]; ok {
		t.Error("an unlisted repo must not be learned")
	}

	if res := svc.ObserveSessionDir(ctx, "dos_1", "/not/a/repo"); len(res.NextActions)+len(res.Warnings) != 0 {
		t.Errorf("a non-repo dir is ignored: %+v", res)
	}
}

func TestRepoAddRemoveLocate(t *testing.T) {
	ctx := context.Background()
	svc, st, loc := repoService(t, nil)
	loc.checkouts["/src/api"] = [2]string{"github.com/acme/api", "/src/api"}

	if _, err := svc.RepoAdd(ctx, core.RepoAddReq{ID: "dos_1", Ref: "/src/api"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RepoAdd(ctx, core.RepoAddReq{ID: "dos_1", Ref: "git@github.com:acme/web.git"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.RepoAdd(ctx, core.RepoAddReq{ID: "dos_1", Ref: "https://github.com/acme/web"})
	if err != nil || len(res.Warnings) != 1 {
		t.Errorf("re-adding warns, does not duplicate: %v %v", res.Warnings, err)
	}
	if got := strings.Join(st.Dossiers["dos_1"].Frontmatter.Repos, ","); got != "github.com/acme/api,github.com/acme/web" {
		t.Errorf("repos = %s", got)
	}
	if loc.remembered["github.com/acme/api"] != "/src/api" {
		t.Error("adding by path should remember the checkout")
	}
	if _, err := svc.RepoAdd(ctx, core.RepoAddReq{ID: "dos_1", Ref: "not a repo"}); err == nil {
		t.Error("garbage ref must be rejected")
	}

	loc.checkouts["/elsewhere/web"] = [2]string{"github.com/acme/web", "/elsewhere/web"}
	if _, err := svc.RepoLocate(ctx, "dos_1", "/elsewhere/web"); err != nil || loc.remembered["github.com/acme/web"] != "/elsewhere/web" {
		t.Errorf("locate: %v %v", err, loc.remembered)
	}
	loc.checkouts["/src/other"] = [2]string{"github.com/acme/other", "/src/other"}
	if _, err := svc.RepoLocate(ctx, "dos_1", "/src/other"); err == nil {
		t.Error("locating an unlisted repo must fail with a pointer to repo add")
	}

	if _, err := svc.RepoRemove(ctx, core.RepoRemoveReq{ID: "dos_1", Ref: "github.com/acme/api"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(st.Dossiers["dos_1"].Frontmatter.Repos, ","); got != "github.com/acme/web" {
		t.Errorf("after remove repos = %s", got)
	}
	if _, err := svc.RepoRemove(ctx, core.RepoRemoveReq{ID: "dos_1", Ref: "github.com/acme/api"}); err == nil {
		t.Error("removing an unlisted repo must fail")
	}
}

func TestSaveRejectsInvalidRepos(t *testing.T) {
	svc, _, _ := repoService(t, nil)
	_, err := svc.Save(context.Background(), core.SaveReq{ID: "dos_1", FrontmatterUpdates: map[string]any{"repos": []any{"github.com/acme/a", "https://github.com/acme/a.git"}}})
	if err == nil {
		t.Error("duplicate repos after normalization must be rejected")
	}
	_, err = svc.Save(context.Background(), core.SaveReq{ID: "dos_1", FrontmatterUpdates: map[string]any{"repos": "github.com/acme/a"}})
	if err == nil {
		t.Error("repos must be a list")
	}
}
