package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// remoteHeadHash fetches the remote branch into a remote-tracking ref (it does
// NOT touch the checked-out local branch) and returns the remote commit hash.
// Returns a zero hash + a non-nil error if the remote is unreachable or the ref
// is absent; git.NoErrAlreadyUpToDate is not treated as an error.
func (g *GitSync) remoteHeadHash(ctx context.Context, repo *git.Repository, branch string) (plumbing.Hash, error) {
	remote, err := repo.Remote(originName)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if err := remote.FetchContext(ctx, &git.FetchOptions{
		RemoteName: originName,
		RefSpecs:   []config.RefSpec{config.RefSpec("+refs/heads/*:refs/remotes/origin/*")},
		Auth:       g.cfg.Auth,
	}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) && err.Error() != "remote repository is empty" {
		return plumbing.ZeroHash, err
	}
	rt := plumbing.NewRemoteReferenceName(originName, branch)
	remoteHash, err := repo.ResolveRevision(plumbing.Revision(rt))
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return plumbing.ZeroHash, nil
		}
		return plumbing.ZeroHash, err
	}
	return *remoteHash, nil
}

// doPush pushes the local branch to the remote. Returns pushed=true on success
// (including "already up to date"), or an error if the push failed.
func (g *GitSync) doPush(ctx context.Context, repo *git.Repository, branch string) (bool, error) {
	remote, err := repo.Remote(originName)
	if err != nil {
		return false, fmt.Errorf("remote: %w", err)
	}
	branchRef := plumbing.NewBranchReferenceName(branch)
	err = remote.PushContext(ctx, &git.PushOptions{
		RemoteName: originName,
		RefSpecs:   []config.RefSpec{config.RefSpec(branchRef.String() + ":" + branchRef.String())},
		Auth:       g.cfg.Auth,
	})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return true, nil
	}
	return false, fmt.Errorf("push: %w", err)
}

// Status returns a read-only snapshot: ahead/behind counts (via a best-effort
// remote fetch into tracking refs — no change to the local branch), last sync
// time + pending conflicts (from the persisted sync state), and the count of
// uncommitted tracked changes. Fetch failures degrade to zero counts rather
// than erroring — Status is advisory.
func (g *GitSync) Status() (SyncStatus, error) {
	var st SyncStatus
	repo, err := git.PlainOpen(g.cfg.StoreDir)
	if err != nil {
		return st, fmt.Errorf("open repo: %w", err)
	}
	if wt, err := repo.Worktree(); err == nil {
		if status, err := wt.Status(); err == nil {
			st.Dirty = len(status)
		}
	}
	head, err := headHash(repo)
	if err == nil && !head.IsZero() {
		remote, ferr := g.remoteHeadHash(context.Background(), repo, g.cfg.Branch)
		if ferr == nil {
			st.Ahead, st.Behind = countDivergence(repo, head, remote)
		}
	}
	s := loadState(g.cfg.StoreDir)
	st.LastSync = s.LastSync
	st.Conflicts = s.Conflicts
	return st, nil
}

// divergence returns ahead/behind vs the current remote HEAD (best-effort).
func (g *GitSync) divergence(repo *git.Repository) (int, int) {
	head, err := headHash(repo)
	if err != nil || head.IsZero() {
		return 0, 0
	}
	remote, _ := g.remoteHeadHash(context.Background(), repo, g.cfg.Branch)
	return countDivergence(repo, head, remote)
}

// countDivergence returns (#commits reachable from local but not remote,
// #commits reachable from remote but not local). Either hash may be zero.
func countDivergence(repo *git.Repository, local, remote plumbing.Hash) (int, int) {
	if local.IsZero() {
		return 0, 0
	}
	if remote.IsZero() {
		return len(ancestors(repo, local)), 0
	}
	if local == remote {
		return 0, 0
	}
	localAnc := ancestors(repo, local)
	remoteAnc := ancestors(repo, remote)
	ahead, behind := 0, 0
	for h := range localAnc {
		if _, ok := remoteAnc[h]; !ok {
			ahead++
		}
	}
	for h := range remoteAnc {
		if _, ok := localAnc[h]; !ok {
			behind++
		}
	}
	return ahead, behind
}

// ancestors returns the set of commit hashes reachable from start (inclusive)
// via all parents.
func ancestors(repo *git.Repository, start plumbing.Hash) map[plumbing.Hash]struct{} {
	seen := map[plumbing.Hash]struct{}{}
	if start.IsZero() {
		return seen
	}
	stack := []plumbing.Hash{start}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		c, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		for i := 0; i < c.NumParents(); i++ {
			if p, err := c.Parent(i); err == nil {
				stack = append(stack, p.Hash)
			}
		}
	}
	return seen
}
