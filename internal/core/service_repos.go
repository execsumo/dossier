package core

import (
	"context"
	"fmt"
	"strings"
)

// SetRepoLocator wires the machine-local repo resolver (ADR 0015). Without one,
// Dossiers that list repos launch in their own folder with a visible warning.
func (s *Service) SetRepoLocator(l RepoLocator) {
	s.repos = l
}

// LaunchTarget resolves where an agent for the Dossier should start: its
// primary repo when that is checked out on this machine, else the Dossier
// folder. Every repo that does not resolve is a warning, never a silent
// fallback.
func (s *Service) LaunchTarget(ctx context.Context, id string) (Result, error) {
	d, _, err := s.store.Read(id)
	if err != nil {
		return Result{}, err
	}
	dir, err := s.resolveDossierPath(d.Frontmatter.ID, d.Frontmatter)
	if err != nil {
		return Result{}, err
	}
	target := LaunchTarget{DossierDir: dir, WorkDir: dir}
	repos, warnings := s.resolveRepos(d.Frontmatter)
	target.Repos = repos
	if len(repos) > 0 {
		if repos[0].Path != "" {
			target.WorkDir = repos[0].Path
		} else {
			warnings = append(warnings, Warning(fmt.Sprintf("Starting in the Dossier folder because its primary repo %s is not resolved on this machine.", repos[0].Identity)))
		}
	}
	return Result{OK: true, Data: target, Warnings: warnings}, nil
}

func (s *Service) resolveRepos(fm Frontmatter) ([]ResolvedRepo, []Warning) {
	if len(fm.Repos) == 0 {
		return nil, nil
	}
	out := make([]ResolvedRepo, 0, len(fm.Repos))
	var warnings []Warning
	if s.repos == nil {
		for _, id := range fm.Repos {
			out = append(out, ResolvedRepo{Identity: id})
		}
		return out, []Warning{"Repo resolution is unavailable in this build; the Dossier's repos are not resolved."}
	}
	for _, id := range fm.Repos {
		loc, err := s.repos.Locate(id)
		if err != nil {
			warnings = append(warnings, Warning(fmt.Sprintf("Could not resolve repo %s: %v", id, err)))
			out = append(out, ResolvedRepo{Identity: id})
			continue
		}
		if loc.Stale != "" {
			warnings = append(warnings, Warning(fmt.Sprintf("%s no longer holds %s; forgot that location.", loc.Stale, id)))
		}
		switch {
		case loc.Path != "":
		case len(loc.Candidates) > 0:
			warnings = append(warnings, Warning(fmt.Sprintf("Found several checkouts of %s (%s); not guessing — run: dossier repo locate %s <path>", id, strings.Join(loc.Candidates, ", "), fm.Slug)))
		default:
			warnings = append(warnings, Warning(fmt.Sprintf("%s (%s) isn't on this machine — run: dossier repo locate %s <path>", RepoName(id), id, fm.Slug)))
		}
		out = append(out, ResolvedRepo{Identity: id, Path: loc.Path, Via: loc.Via})
	}
	return out, warnings
}

// ObserveSessionDir learns from a bound session's working directory. When dir
// is inside a checkout of one of the Dossier's repos, its path is remembered
// for this machine (a cache update, so silent). When dir is in some other repo,
// the Dossier is not changed: the result suggests linking it, because adding a
// repo to a shared Dossier is the user's call ("no silent link").
func (s *Service) ObserveSessionDir(ctx context.Context, dossierID, dir string) Result {
	if s.repos == nil || dir == "" || dossierID == "" {
		return Result{OK: true}
	}
	identity, root, ok := s.repos.Identify(dir)
	if !ok {
		return Result{OK: true}
	}
	d, _, err := s.store.Read(dossierID)
	if err != nil {
		return Result{OK: true}
	}
	for _, id := range d.Frontmatter.Repos {
		if id == identity {
			if err := s.repos.Remember(identity, root, "session"); err != nil {
				return Result{OK: true, Warnings: []Warning{Warning(fmt.Sprintf("Could not remember %s for %s: %v", root, identity, err))}}
			}
			return Result{OK: true}
		}
	}
	return Result{OK: true, NextActions: []NextAction{NextAction(fmt.Sprintf(
		"This session is in %s (%s), which %q does not list. If the work lives there, add it: dossier_update with repos, or `dossier repo add %s %s`.",
		identity, root, d.Frontmatter.Name, d.Frontmatter.Slug, identity))}}
}

// RepoAddReq adds a repo to a Dossier. Ref is a remote, an identity, or a path
// to a local checkout (whose origin is used, and whose path is remembered).
type RepoAddReq struct {
	Actor string
	ID    string
	Ref   string
}

func (s *Service) RepoAdd(ctx context.Context, req RepoAddReq) (Result, error) {
	identity, err := s.repoIdentityFromRef(req.Ref, true)
	if err != nil {
		return Result{}, err
	}
	d, _, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}
	for _, existing := range d.Frontmatter.Repos {
		if existing == identity {
			return Result{OK: true, Data: d.Frontmatter.Repos, Warnings: []Warning{Warning(identity + " is already listed.")}}, nil
		}
	}
	repos := append(append([]string{}, d.Frontmatter.Repos...), identity)
	res, err := s.Save(ctx, SaveReq{Actor: req.Actor, ID: req.ID, FrontmatterUpdates: map[string]any{"repos": repos}})
	if err != nil {
		return res, err
	}
	res.Data = repos
	return res, nil
}

type RepoRemoveReq struct {
	Actor string
	ID    string
	Ref   string
}

func (s *Service) RepoRemove(ctx context.Context, req RepoRemoveReq) (Result, error) {
	identity, err := s.repoIdentityFromRef(req.Ref, false)
	if err != nil {
		return Result{}, err
	}
	d, _, err := s.store.Read(req.ID)
	if err != nil {
		return Result{}, err
	}
	repos := make([]string, 0, len(d.Frontmatter.Repos))
	for _, existing := range d.Frontmatter.Repos {
		if existing != identity {
			repos = append(repos, existing)
		}
	}
	if len(repos) == len(d.Frontmatter.Repos) {
		return Result{}, NewError(ErrNotFound, fmt.Sprintf("%s is not listed on %s", identity, d.Frontmatter.Slug))
	}
	res, err := s.Save(ctx, SaveReq{Actor: req.Actor, ID: req.ID, FrontmatterUpdates: map[string]any{"repos": repos}})
	if err != nil {
		return res, err
	}
	res.Data = repos
	return res, nil
}

// RepoLocate records path as this machine's checkout of one of the Dossier's
// repos. The checkout's origin decides which repo it is.
func (s *Service) RepoLocate(ctx context.Context, id, path string) (Result, error) {
	if s.repos == nil {
		return Result{}, NewError(ErrInternal, "repo resolution is unavailable in this build")
	}
	identity, root, ok := s.repos.Identify(path)
	if !ok {
		return Result{}, NewError(ErrInvalidFrontmatter, fmt.Sprintf("%s is not inside a git repository with an origin remote", path))
	}
	d, _, err := s.store.Read(id)
	if err != nil {
		return Result{}, err
	}
	listed := false
	for _, existing := range d.Frontmatter.Repos {
		listed = listed || existing == identity
	}
	if !listed {
		return Result{}, NewError(ErrInvalidFrontmatter, fmt.Sprintf("%s is a checkout of %s, which %s does not list; add it first with: dossier repo add %s %s", root, identity, d.Frontmatter.Slug, d.Frontmatter.Slug, root))
	}
	if err := s.repos.Remember(identity, root, "manual"); err != nil {
		return Result{}, WrapError(ErrInternal, "could not save the repo location", err)
	}
	return Result{OK: true, Data: ResolvedRepo{Identity: identity, Path: root, Via: "manual"}}, nil
}

// RepoStatus reports each of the Dossier's repos and where it resolves here.
func (s *Service) RepoStatus(ctx context.Context, id string) (Result, error) {
	d, _, err := s.store.Read(id)
	if err != nil {
		return Result{}, err
	}
	repos, warnings := s.resolveRepos(d.Frontmatter)
	return Result{OK: true, Data: repos, Warnings: warnings}, nil
}

// repoIdentityFromRef turns a remote, identity or local path into an identity.
// A path is tried first, so `dossier repo add <slug> .` works; when remember is
// set, the checkout's location is recorded for this machine.
func (s *Service) repoIdentityFromRef(ref string, remember bool) (string, error) {
	if s.repos != nil {
		if identity, root, ok := s.repos.Identify(ref); ok {
			if remember {
				_ = s.repos.Remember(identity, root, "manual")
			}
			return identity, nil
		}
	}
	identity, err := NormalizeRepo(ref)
	if err != nil {
		return "", NewError(ErrInvalidFrontmatter, err.Error())
	}
	return identity, nil
}
