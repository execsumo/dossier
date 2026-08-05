package sync

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// isDossierConflictPath reports whether a repo-relative path is a dossier body
// (<slug>/dossier.md) — the only genuinely multi-writer file, and thus the only
// path that can produce a real sync conflict. Everything else (managed
// .gitignore, author-namespaced audit/session shards) is single-writer or
// managed and takes remote-wins silently.
func isDossierConflictPath(path string) bool {
	return path == "dossier.md" || strings.HasSuffix(path, "/dossier.md")
}

// remoteWinsMerge performs the 3-way remote-wins merge. For every file both
// sides changed since the merge base, the remote version wins the working tree
// and the local version is captured in a ConflictRecord. Remote-only changes
// fast-forward into the tree. Local-only changes (already committed) are kept.
// A single merge commit (parents: local, remote) lands the result. No git
// merge markers are ever written to the working tree.
func (g *GitSync) remoteWinsMerge(repo *git.Repository, wt *git.Worktree, local, remote *object.Commit) ([]ConflictRecord, error) {
	var baseTree *object.Tree
	if bases, err := local.MergeBase(remote); err == nil && len(bases) > 0 {
		baseTree, _ = repo.TreeObject(bases[0].TreeHash)
	}
	if baseTree == nil {
		baseTree = &object.Tree{} // no common ancestor: treat base as empty
	}

	localTree, err := repo.TreeObject(local.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("local tree: %w", err)
	}
	remoteTree, err := repo.TreeObject(remote.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("remote tree: %w", err)
	}

	localPaths := changeNames(diffTree(baseTree, localTree))
	remotePaths := changeNames(diffTree(baseTree, remoteTree))

	// both-changed = changed on the local AND remote sides since the base.
	both := intersect(localPaths, remotePaths)
	conflicts := make([]ConflictRecord, 0, len(both))
	for path := range both {
		// Only a dossier.md is a genuine multi-writer conflict worth preserving.
		// Store-managed files (.gitignore) and author-namespaced single-writer
		// files (audit/<author>.log, sessions/<author>/…) must never be routed as
		// dossier conflicts — remote-wins lands them silently below. Without this,
		// two independently-initialized stores (the normal onboarding flow, no
		// common git ancestor) flag their identical .gitignore as a phantom
		// conflict on first sync.
		if !isDossierConflictPath(path) {
			continue
		}
		localContent, _ := blobContent(repo, localTree, path)
		remoteContent, _ := blobContent(repo, remoteTree, path)
		// Identical content is not a conflict (e.g. both sides promoted the same
		// body, or a no-common-ancestor first merge of byte-identical files).
		if bytes.Equal(localContent, remoteContent) {
			continue
		}
		conflicts = append(conflicts, ConflictRecord{
			Path:           path,
			LocalContent:   localContent,
			RemoteContent:  remoteContent,
			LocalRevision:  local.Hash.String(),
			RemoteRevision: remote.Hash.String(),
		})
	}

	// Land ALL remote changes: remote-wins for the both-changed set, plain
	// fast-forward for remote-only changes. Remote deletions remove the file.
	for path := range remotePaths {
		if entry, err := remoteTree.FindEntry(path); err == nil && entry != nil && !entry.Hash.IsZero() {
			if err := checkoutBlob(repo, remoteTree, g.cfg.StoreDir, path); err != nil {
				return nil, fmt.Errorf("land remote %s: %w", path, err)
			}
			wt.Add(path)
		} else {
			removeWorkingFile(g.cfg.StoreDir, path)
			wt.Remove(path)
		}
	}

	// Stage the remote landings (respects .gitignore — machine-local files stay out).
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return nil, fmt.Errorf("stage merged: %w", err)
	}

	name, email := g.author()
	if _, err := wt.Commit(mergeCommitMessage(conflicts), &git.CommitOptions{
		Author:            plumbingSignature(name, email),
		Committer:         plumbingSignature(name, email),
		Parents:           []plumbing.Hash{local.Hash, remote.Hash},
		AllowEmptyCommits: true,
	}); err != nil {
		return nil, fmt.Errorf("merge commit: %w", err)
	}
	return conflicts, nil
}

// diffTree returns the file changes from a -> b, tolerating nil inputs.
func diffTree(a, b *object.Tree) object.Changes {
	changes, err := object.DiffTree(a, b)
	if err != nil {
		return nil
	}
	return changes
}

func changeNames(changes object.Changes) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range changes {
		name := c.To.Name
		if c.From.Name != "" {
			name = c.From.Name
		}
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

func intersect(a, b map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}

// blobContent reads the full content of a file at path in tree via its blob.
func blobContent(repo *git.Repository, tree *object.Tree, path string) ([]byte, error) {
	entry, err := tree.FindEntry(path)
	if err != nil {
		return nil, err
	}
	blob, err := repo.BlobObject(entry.Hash)
	if err != nil {
		return nil, err
	}
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// checkoutBlob writes the remote file at path into the working tree.
func checkoutBlob(repo *git.Repository, tree *object.Tree, storeDir, path string) error {
	entry, err := tree.FindEntry(path)
	if err != nil {
		return err
	}
	blob, err := repo.BlobObject(entry.Hash)
	if err != nil {
		return err
	}
	r, err := blob.Reader()
	if err != nil {
		return err
	}
	defer r.Close()
	full := filepath.Join(storeDir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	os.Remove(full)
	f, err := os.Create(full)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func removeWorkingFile(storeDir, path string) {
	_ = os.Remove(filepath.Join(storeDir, filepath.FromSlash(path)))
}

func mergeCommitMessage(conflicts []ConflictRecord) string {
	if len(conflicts) == 0 {
		return "dossier sync: merge remote"
	}
	names := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		names = append(names, c.Path)
	}
	return fmt.Sprintf("dossier sync: merge remote (%d conflict(s): %s)", len(conflicts), strings.Join(names, ", "))
}
