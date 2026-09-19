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
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Clone clones url (or the configured RemoteURL) into dir, making dir a working
// tree. CloneContext keeps join cancellation effective. Any partial clone is
// moved outside the store before the error is returned.
func (g *GitSync) Clone(ctx context.Context, url, dir string, depth int) error {
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

	var existing map[string]bool
	if entries, err := os.ReadDir(dir); err == nil {
		existing = make(map[string]bool, len(entries))
		for _, e := range entries {
			existing[e.Name()] = true
			// credentials lives at $HOME/.dossier/credentials, which is the
			// default store, so it must be written before joining.
			if e.Name() != "config.yaml" && e.Name() != ".gitignore" && e.Name() != "credentials" {
				return errors.New("target directory is not empty; cannot join into an existing store")
			}
		}
	}
	if err := g.validateCloneBranch(ctx, url); err != nil {
		return moveFailedJoin(dir, existing, err)
	}

	if _, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:        url,
		RemoteName: originName,
		Auth:       g.cfg.Auth,
		Depth:      depth,
	}); err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return errors.New("target directory is not empty; cannot join into an existing store")
		}
		return fmt.Errorf("sync clone %s: %w", url, moveFailedJoin(dir, existing, err))
	}
	if err := EnsureGitignore(dir); err != nil {
		return fmt.Errorf("sync clone %s: %w", url, moveFailedJoin(dir, existing, err))
	}
	// A clone is a successful pull; without this, health reads "never synced"
	// right after a join.
	now := time.Now()
	saveState(dir, syncState{LastAttempt: now, LastSuccessPull: now, AuthState: g.cfg.AuthState})
	return nil
}

// CheckRemoteAccess verifies that a remote can be listed without changing the
// local store. Empty repositories count as reachable.
func (g *GitSync) CheckRemoteAccess(ctx context.Context, url string) error {
	if url == "" {
		url = g.cfg.RemoteURL
	}
	if url == "" {
		return errors.New("sync: remote URL is required")
	}
	_, err := g.remoteRefs(ctx, url)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "remote repository is empty") {
		return classifyAccessError(fmt.Errorf("unable to access remote %s: %w", url, err))
	}
	return nil
}

// CheckRemoteEmpty verifies that a team-create target has no refs without
// changing the local store. It works for local bare repositories and network
// remotes alike.
func (g *GitSync) CheckRemoteEmpty(ctx context.Context, url string) error {
	if url == "" {
		url = g.cfg.RemoteURL
	}
	if url == "" {
		return errors.New("sync: remote URL is required")
	}
	refs, err := g.remoteRefs(ctx, url)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "remote repository is empty") {
			return nil
		}
		return classifyAccessError(fmt.Errorf("unable to inspect remote %s: %w", url, err))
	}
	for _, ref := range refs {
		if ref.Name() == plumbing.HEAD {
			continue
		}
		return fmt.Errorf("the remote %s is not empty; team create needs an empty repository", url)
	}
	return nil
}

func CheckRemoteAccess(ctx context.Context, url string, auth transport.AuthMethod) error {
	return New(Config{RemoteURL: url, Auth: auth}).CheckRemoteAccess(ctx, url)
}

func (g *GitSync) remoteRefs(ctx context.Context, url string) ([]*plumbing.Reference, error) {
	remote := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{
		Name: originName,
		URLs: []string{url},
	})
	return remote.ListContext(ctx, &git.ListOptions{Auth: g.cfg.Auth})
}

func (g *GitSync) validateCloneBranch(ctx context.Context, url string) error {
	refs, err := g.remoteRefs(ctx, url)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "remote repository is empty") {
			return errors.New("the remote repository is empty; cannot join a team store")
		}
		return fmt.Errorf("sync clone %s: %w", url, err)
	}
	for _, ref := range refs {
		if ref.Name() != plumbing.HEAD || ref.Type() != plumbing.SymbolicReference {
			continue
		}
		branch := strings.TrimPrefix(ref.Target().String(), "refs/heads/")
		if branch != "" && branch != g.cfg.Branch {
			return fmt.Errorf("the remote's default branch is %s; Dossier team stores use %s", branch, g.cfg.Branch)
		}
	}
	return nil
}

// Create initializes the git repo, sets HEAD to the configured branch, and pushes the initial commit.
func (g *GitSync) Create(ctx context.Context, remoteURL, branch string) error {
	storeDir := g.cfg.StoreDir
	if remoteURL == "" {
		remoteURL = g.cfg.RemoteURL
	}
	if branch == "" {
		branch = g.cfg.Branch
	}
	if storeDir == "" {
		return errors.New("sync: StoreDir is required")
	}
	gitignoreExists := fileExists(filepath.Join(storeDir, ".gitignore"))

	repo, err := git.PlainInit(storeDir, false)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return errors.New("store is already a team store")
		}
		return g.failCreate(err, gitignoreExists)
	}

	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/"+branch))
	if err := repo.Storer.SetReference(headRef); err != nil {
		return g.failCreate(fmt.Errorf("set HEAD: %w", err), gitignoreExists)
	}

	if remoteURL == "" {
		return g.failCreate(errors.New("sync: RemoteURL is required for create"), gitignoreExists)
	}
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: originName,
		URLs: []string{remoteURL},
	})
	if err != nil {
		return g.failCreate(fmt.Errorf("create remote: %w", err), gitignoreExists)
	}

	createCfg := *g
	createCfg.cfg.RemoteURL = remoteURL
	createCfg.cfg.Branch = branch
	report, err := createCfg.syncWithCtx(ctx)
	if err != nil {
		return g.failCreate(err, gitignoreExists)
	}
	if report.Error != "" {
		return g.failCreate(fmt.Errorf("initial sync error: %s", report.Error), gitignoreExists)
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

	st := loadState(storeDir)
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
	pullSuccess := ferr == nil
	if errors.Is(ferr, transport.ErrAuthenticationRequired) || errors.Is(ferr, transport.ErrAuthorizationFailed) || errors.Is(ferr, ErrInsecureCredentials) || errors.Is(ferr, ErrNoCredentials) {
		report.AuthFailed = true
	}
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
	// A failed pull already consumed the bounded network budget. Use the last
	// remote-tracking ref and do not fetch again (or attempt a push) offline.
	if ferr != nil {
		report.Ahead, report.Behind = g.localDivergence(repo)
	} else {
		report.Ahead, report.Behind = g.divergence(ctx, repo)
	}

	// --- PUSH ---
	pushSuccess := false
	if ferr == nil && g.cfg.RemoteURL != "" {
		succ, pushed, perr := g.doPush(ctx, repo, g.cfg.Branch)
		if perr != nil {
			report.Error = appendErr(report.Error, perr.Error())
			if errors.Is(perr, transport.ErrAuthenticationRequired) || errors.Is(perr, transport.ErrAuthorizationFailed) || errors.Is(perr, ErrInsecureCredentials) || errors.Is(perr, ErrNoCredentials) {
				report.AuthFailed = true
			}
		} else {
			pushSuccess = succ
			report.Pushed = pushed
		}
	}

	// --- persist sync state for Status() ---
	now := time.Now()
	st.LastAttempt = now
	if pullSuccess {
		st.LastSuccessPull = now
	}
	if pushSuccess {
		st.LastSuccessPush = now
	}
	if report.Error != "" {
		st.LastError = report.Error
	} else {
		st.LastError = ""
	}
	if g.cfg.AuthState != "" {
		if report.AuthFailed {
			st.AuthState = "rejected"
		} else {
			st.AuthState = g.cfg.AuthState
		}
	}
	st.Conflicts = report.Conflicts
	saveState(storeDir, st)

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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (g *GitSync) failCreate(err error, gitignoreExists bool) error {
	storeDir := g.cfg.StoreDir
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	failedDir := storeDir + ".failed-create-" + stamp
	moved := false
	if err := os.Mkdir(failedDir, 0700); err == nil {
		if os.Rename(filepath.Join(storeDir, ".git"), filepath.Join(failedDir, ".git")) == nil {
			moved = true
		}
		if !gitignoreExists && os.Rename(filepath.Join(storeDir, ".gitignore"), filepath.Join(failedDir, ".gitignore")) == nil {
			moved = true
		}
		if !moved {
			_ = os.Remove(failedDir)
		}
	}
	if moved {
		return fmt.Errorf("%w (created files moved aside to %s)", err, failedDir)
	}
	return err
}

func moveFailedJoin(dir string, existing map[string]bool, cause error) error {
	entries, _ := os.ReadDir(dir)
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	failedDir := dir + ".failed-join-" + stamp
	if err := os.Mkdir(failedDir, 0700); err != nil {
		return cause
	}
	for _, entry := range entries {
		if existing[entry.Name()] {
			continue
		}
		_ = os.Rename(filepath.Join(dir, entry.Name()), filepath.Join(failedDir, entry.Name()))
	}
	return fmt.Errorf("%w (created files moved aside to %s)", cause, failedDir)
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
