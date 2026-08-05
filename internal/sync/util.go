package sync

import (
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// plumbingSignature builds an *object.Signature for the git commit
// author/committer.
func plumbingSignature(name, email string) *object.Signature {
	return &object.Signature{Name: name, Email: email, When: time.Now()}
}

// hashStr returns the SHA string of a hash, "" if zero.
func hashStr(h plumbing.Hash) string {
	if h.IsZero() {
		return ""
	}
	return h.String()
}

// headHash returns the current HEAD commit hash, resolving a symbolic HEAD to
// its branch ref. Returns a zero hash (with error) if HEAD is unborn.
func headHash(repo *git.Repository) (plumbing.Hash, error) {
	ref, err := repo.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if ref.Type() == plumbing.SymbolicReference {
		ref, err = repo.Reference(ref.Target(), true)
		if err != nil {
			return plumbing.ZeroHash, err
		}
	}
	return ref.Hash(), nil
}

// headBranchRefName returns the branch ref name HEAD points at (e.g.
// "refs/heads/main"), or the default branch ref name if HEAD is detached.
func headBranchRefName(repo *git.Repository) plumbing.ReferenceName {
	if ref, err := repo.Head(); err == nil && ref.Type() == plumbing.SymbolicReference {
		return ref.Target()
	}
	return plumbing.NewBranchReferenceName("main")
}
