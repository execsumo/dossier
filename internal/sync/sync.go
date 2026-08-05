package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// Clone clones url (or the configured RemoteURL) into dir, making dir a working
// tree. Used by the future "dossier team join" flow. The store-wide .gitignore
// is ensured after the clone.
func (g *GitSync) Clone(url, dir string, depth int) error {
	if url == "" {
		url = g.cfg.RemoteURL
	}
	if url == "" {
		return errors.New("sync: clone requires a remote url")
	}
	if dir == "" {
		dir = g.cfg.StoreDir
	}
	if dir == "" {
		return errors.New("sync: clone requires a target dir")
	}

	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.Name() != "config.yaml" && e.Name() != ".gitignore" {
				return errors.New("target directory is not empty; cannot join into an existing store")
			}
		}
	}

	if _, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:        url,
		RemoteName: originName,
		Auth:       g.cfg.Auth,
		Depth:      depth,
	}); err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return errors.New("target directory is not empty; cannot join into an existing store")
		}
		return fmt.Errorf("sync clone %s: %w", url, err)
	}
	return EnsureGitignore(dir)
}

// Create initializes the git repo, sets HEAD to the configured branch, and pushes the initial commit.
func (g *GitSync) Create(ctx context.Context) error {
	storeDir := g.cfg.StoreDir
	if storeDir == "" {
		return errors.New("sync: StoreDir is required")
	}

	repo, err := git.PlainInit(storeDir, false)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return errors.New("store is already a team store")
		}
		return fmt.Errorf("init repo: %w", err)
	}

	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/"+g.cfg.Branch))
	if err := repo.Storer.SetReference(headRef); err != nil {
		return fmt.Errorf("set HEAD: %w", err)
	}

	if g.cfg.RemoteURL == "" {
		return errors.New("sync: RemoteURL is required for create")
	}
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: originName,
		URLs: []string{g.cfg.RemoteURL},
	})
	if err != nil {
		return fmt.Errorf("create remote: %w", err)
	}

	report, err := g.syncWithCtx(ctx)
	if err != nil {
		return err
	}
	if report.Error != "" {
		return fmt.Errorf("initial sync error: %s", report.Error)
	}
	return nil
}

// Sync runs the full pull → resolve(remote-wins) → commit → push cycle against
// the configured remote. It is local-first: the local commit always lands even
// if the remote is unreachable (in which case [SyncReport.Error] is set).
func (g *GitSync) Sync() (SyncReport, error) {
	return g.syncWithCtx(context.Background())
}

func (g *GitSync) syncWithCtx(ctx context.Context) (SyncReport, error) {
	var report SyncReport
	storeDir := g.cfg.StoreDir
	if storeDir == "" {
		return report, errors.New("sync: StoreDir is required")
	}

	// --- store-wide sync lock: serialize concurrent Sync calls on one store ---
	lock := newSyncLock(storeDir)
	lctx, cancel := context.WithTimeout(ctx, g.cfg.LockTimeout)
	defer cancel()
	if err := lock.acquire(lctx); err != nil {
		return report, err
	}
	defer func() { _ = lock.release() }()

	if err := EnsureGitignore(storeDir); err != nil {
		return report, err
	}

	repo, err := git.PlainOpen(storeDir)
	if err != nil {
		return report, fmt.Errorf("open repo: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return report, fmt.Errorf("worktree: %w", err)
	}

	// --- COMMIT local tracked changes (local-first: lands before any network) ---
	localHead, commitMsg, excluded, err := g.commitLocal(repo, wt)
	if err != nil {
		return report, err
	}
	report.CommitSHA = localHead
	report.CommitMessage = commitMsg
	report.Excluded = excluded

	// --- PULL → RESOLVE (remote-wins): fetch + 3-way merge ---
	pullReport, ferr, perr := g.pullRemoteWins(ctx, repo, wt, localHead)
	if perr != nil {
		return report, perr
	}
	report.Pulled = pullReport.Pulled
	report.Conflicts = pullReport.Conflicts
	if ferr != nil {
		// Fetch failure (e.g. unreachable remote): skip merge; still attempt push
		// so the surfaced error is the network one. The local commit landed.
		report.Error = ferr.Error()
	}

	// --- ahead/behind snapshot (after merge, before push) ---
	report.Ahead, report.Behind = g.divergence(repo)

	// --- PUSH ---
	if g.cfg.RemoteURL != "" {
		pushed, perr := g.doPush(ctx, repo, g.cfg.Branch)
		if perr != nil {
			report.Error = appendErr(report.Error, perr.Error())
		} else {
			report.Pushed = pushed
		}
	}

	// --- persist sync state for Status() ---
	saveState(storeDir, syncState{LastSync: time.Now(), Conflicts: report.Conflicts})

	return report, nil
}

// commitLocal stages tracked changes (excluding oversized + gitignored files),
// commits them with an author-summary message, and returns the new HEAD hash
// string, the commit message, and any oversized files refused. Returns
// ("", "", nil, nil) when there is nothing to commit.
func (g *GitSync) commitLocal(repo *git.Repository, wt *git.Worktree) (string, string, []ExcludedFile, error) {
	status, err := wt.Status()
	if err != nil {
		return "", "", nil, fmt.Errorf("status: %w", err)
	}

	var excluded []ExcludedFile
	var stagedSlugs []string
	for path := range status {
		full := filepath.Join(g.cfg.StoreDir, path)
		if sz, err := os.Stat(full); err == nil && sz.Size() > MaxFileSizeBytes {
			excluded = append(excluded, ExcludedFile{
				Path: path,
				Size: sz.Size(),
				Warning: fmt.Sprintf("excluded from sync: %s is %.1f MB (> %d MB GitHub limit)",
					path, float64(sz.Size())/(1024*1024), MaxFileSizeBytes/(1024*1024)),
			})
			continue
		}
		if _, err := wt.Add(path); err != nil {
			return "", "", nil, fmt.Errorf("stage %s: %w", path, err)
		}
		if slug := topSlug(path); slug != "" {
			stagedSlugs = append(stagedSlugs, slug)
		}
	}

	if len(stagedSlugs) == 0 {
		h, _ := headHash(repo)
		return hashStr(h), "", excluded, nil
	}
	sort.Strings(stagedSlugs)
	stagedSlugs = uniq(stagedSlugs)
	msg := "dossier sync: " + strings.Join(stagedSlugs, ", ")

	name, email := g.author()
	hash, err := wt.Commit(msg, &git.CommitOptions{
		Author:    plumbingSignature(name, email),
		Committer: plumbingSignature(name, email),
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("commit local: %w", err)
	}
	return hash.String(), msg, excluded, nil
}

func (g *GitSync) author() (string, string) {
	name := g.cfg.AuthorName
	if name == "" {
		name = "dossier-sync"
	}
	email := g.cfg.AuthorEmail
	if email == "" {
		email = "sync@dossier.local"
	}
	return name, email
}

// topSlug returns the top-level directory (slug) of a repo-relative path, or the
// bare name if it has no separator. Used to summarize commits by changed slugs.
func topSlug(path string) string {
	if path == "" {
		return ""
	}
	if i := strings.IndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return path
}

func uniq(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func appendErr(a, b string) string {
	switch {
	case a == "" && b == "":
		return ""
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}
