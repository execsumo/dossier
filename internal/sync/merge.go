package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// mergeResult is the outcome of the pull/merge phase.
type mergeResult struct {
	Pulled    bool
	Conflicts []ConflictRecord
}

// pullRemoteWins fetches the remote and, if the branches diverged, performs a
// remote-wins 3-way merge. Returns (result, fetchError, hardError): a non-nil
// fetchError means the remote could not be reached (the caller surfaces it but
// the local commit already landed); a non-nil hardError is an unexpected local
// failure that aborts the sync.
func (g *GitSync) pullRemoteWins(ctx context.Context, repo *git.Repository, wt *git.Worktree, localHead string) (mergeResult, error, error) {
	var res mergeResult
	remoteHash, ferr := g.remoteHeadHash(ctx, repo, g.cfg.Branch)
	if ferr != nil {
		if errors.Is(ferr, git.NoErrAlreadyUpToDate) {
			return res, nil, nil
		}
		return res, ferr, nil // fetch error: remote unreachable; caller surfaces it
	}
	if remoteHash.IsZero() {
		return res, nil, nil
	}
	localHash := plumbing.NewHash(localHead)
	if localHash == remoteHash {
		return res, nil, nil // already in sync
	}

	localCommit, err := repo.CommitObject(localHash)
	if err != nil {
		// no local commits yet: fast-forward straight to remote
		if err := ffToRemote(repo, wt, g.cfg, remoteHash); err != nil {
			return res, nil, fmt.Errorf("fast-forward: %w", err)
		}
		res.Pulled = true
		return res, nil, nil
	}
	remoteCommit, err := repo.CommitObject(remoteHash)
	if err != nil {
		return res, nil, fmt.Errorf("remote commit %s: %w", remoteHash, err)
	}

	// local is a strict ancestor of remote -> fast-forward (no new commit)
	if localAncestor, err := localCommit.IsAncestor(remoteCommit); err == nil && localAncestor {
		if err := ffToRemote(repo, wt, g.cfg, remoteHash); err != nil {
			return res, nil, fmt.Errorf("fast-forward: %w", err)
		}
		res.Pulled = true
		return res, nil, nil
	}
	// remote is a strict ancestor of local -> nothing to pull
	if remoteAncestor, err := remoteCommit.IsAncestor(localCommit); err == nil && remoteAncestor {
		return res, nil, nil
	}

	conflicts, err := g.remoteWinsMerge(repo, wt, localCommit, remoteCommit)
	if err != nil {
		return res, nil, fmt.Errorf("remote-wins merge: %w", err)
	}
	res.Pulled = true
	res.Conflicts = conflicts
	return res, nil, nil
}

// ffToRemote performs a true fast-forward: moves the current branch ref to
// remoteHash and syncs the working tree + index to it (no new commit).
func ffToRemote(repo *git.Repository, wt *git.Worktree, cfg Config, remoteHash plumbing.Hash) error {
	branchRefName := headBranchRefName(repo)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branchRefName, remoteHash)); err != nil {
		return fmt.Errorf("set branch ref: %w", err)
	}
	// go-git's Force checkout removes working-tree files that are absent from the
	// target tree — including untracked/gitignored ones. The machine-local files
	// (config.yaml, root sessions/, context/, sync state) are gitignored and thus
	// never in the tree, so a naive Force checkout would delete them (nuking a
	// joined colleague's team config on their first fast-forward pull). Stash them
	// across the checkout and restore. They are purely local, so the remote tree
	// can never legitimately carry them.
	restore := stashMachineLocal(cfg.StoreDir)
	defer restore()
	if err := wt.Checkout(&git.CheckoutOptions{Branch: branchRefName, Force: true}); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}
	return nil
}

// machineLocalPreserve are the store-root machine-local paths that must survive a
// force checkout. (.sync.lock is deliberately excluded — it is held during sync.)
var machineLocalPreserve = []string{"config.yaml", ".syncstate.json", "sessions", "context"}

// stashMachineLocal moves the machine-local paths out of storeDir into a temp dir
// and returns a restore func that puts them back (overwriting anything the
// checkout wrote in their place). Best-effort: a path that can't be moved is left
// as-is rather than failing the sync.
func stashMachineLocal(storeDir string) func() {
	if storeDir == "" {
		return func() {}
	}
	tmp, err := os.MkdirTemp("", "dossier-ml-")
	if err != nil {
		return func() {}
	}
	moved := map[string]string{}
	for _, name := range machineLocalPreserve {
		src := filepath.Join(storeDir, name)
		if _, err := os.Lstat(src); err != nil {
			continue // does not exist
		}
		dst := filepath.Join(tmp, name)
		if err := os.Rename(src, dst); err == nil {
			moved[src] = dst
		}
	}
	return func() {
		for src, dst := range moved {
			_ = os.RemoveAll(src)   // remove whatever the checkout put here
			_ = os.Rename(dst, src) // restore the original
		}
		_ = os.RemoveAll(tmp)
	}
}
