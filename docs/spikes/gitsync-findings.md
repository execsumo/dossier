# Spike Findings — go-git Sync Adapter (Team Sync Phase 2 de-risk)

> Date: 2026-07-15 · Branch: `delegate/cline-gitsync-spike` · Package: `internal/sync`
> Companion: `docs/team-sync-plan.md` (Phase 2), ADR 0005, `BUILD-DECISIONS.md` B12.
>
> This is a **spike**: `internal/sync` is self-contained and is NOT wired into
> `core.Service`, the CLI, or MCP. Everything below is verified by tests in
> `internal/sync/*_test.go`, all of which run against **local bare repos** (no
> network, no GitHub).

## TL;DR

go-git (v5.19.1) is viable for the Phase 2 sync engine. The one genuinely
non-trivial part is the **remote-wins-per-file conflict policy with no git merge
markers**: go-git's built-in `Worktree.Pull` (fetch + merge) does not expose a
per-file remote-wins strategy and, on divergence, either fails or leaves the
working tree conflicted. We therefore bypass `Pull`'s merge entirely and
implement a custom 3-way remote-wins merge on top of go-git primitives
(`DiffTree` + `Commit.MergeBase` + per-file blob checkout + a manual merge
commit). Everything else (clone, fetch, push, commit, ahead/behind, locking,
gitignore, oversized exclusion) maps cleanly onto go-git's public API.

## What go-git CAN do (and how the spike uses it)

- **Clone / open:** `git.PlainClone`, `git.PlainOpen`, `git.PlainInit` all work
  against local paths with no auth. `CloneOptions{URL, RemoteName}` is enough.
- **Fetch (object download, no working-tree mutation):** `Remote.FetchContext`
  fetches into **remote-tracking refs** (`+refs/heads/*:refs/remotes/origin/*`).
  Fetching into the checked-out local branch ref would desync HEAD from the
  working tree — we deliberately fetch into tracking refs and read
  `refs/remotes/origin/<branch>`.
- **Push:** `Remote.PushContext` with a `refs/heads/<b>:refs/heads/<b>` refspec.
  Returns `git.NoErrAlreadyUpToDate` when there's nothing to push (not treated
  as an error). On an unreachable remote it returns a transport error, which
  we surface in `SyncReport.Error` while the local commit still lands
  (local-first).
- **Commit graph reasoning:** `object.Commit.IsAncestor(other)` and
  `object.Commit.MergeBase(other) ([]*Commit, error)` let us classify a remote
  as fast-forwardable (local is ancestor), stale (remote is ancestor), or
  divergent. `MergeBase` returns a slice; take `[0]`.
- **Tree/blob access:** `repo.TreeObject(hash)`, `tree.FindEntry(path)` →
  `TreeEntry.Hash`, `repo.BlobObject(hash)`, `blob.Reader() (io.ReadCloser,
  error)`. Streaming via `io.Copy` (used for landing remote files) avoids
  loading whole blobs into memory.
- **Tree diffing:** `object.DiffTree(a, b *Tree) (Changes, error)` yields the
  file changes between two trees; each `*Change` exposes `From`/`To`
  `ChangeEntry` with `.Name` (the path) and `.TreeEntry.Hash`. We build
  base→local and base→remote path sets and intersect them to find both-changed
  files (conflicts). Note: `Change` has no exported `Name` field — use
  `c.From.Name` (modify/delete) or `c.To.Name` (insert).
- **Fast-forward:** a true FF is `repo.Storer.SetReference(NewHashReference(branchRef,
  remoteHash))` (move the branch ref) + `wt.Checkout(&CheckoutOptions{Branch:
  branchRef, Force: true})` (sync working tree). No new commit is produced.
- **Merge commit:** `wt.Commit(msg, &CommitOptions{Parents: []plumbing.Hash{local,
  remote}, Author, Committer})` creates a 2-parent merge commit and advances
  the checked-out branch ref. `Author`/`Committer` are `*object.Signature`.

## What go-git CAN'T easily do / behaviors worked around

- **No per-file remote-wins merge strategy.** go-git's `Worktree.Pull` does
  fetch + a merkletrie merge; it does not offer a "theirs"/"remote-wins-per-file"
  policy and on a divergent `dossier.md` it would either fail or leave conflict
  markers in the working tree. The spec forbids markers *anywhere* and requires
  remote-wins. **Workaround:** we never call `Pull`; instead we fetch, compute
  the 3-way diff ourselves via `DiffTree` against the merge base, capture local
  content for both-changed files into `ConflictRecord`, then write each
  remote-changed file's blob into the working tree and create a single merge
  commit. This is the single most important finding for Phase 2.
- **Spec ordering vs. dirty working tree.** The spec describes
  `pull → resolve → commit-all-tracked-changes`. Merging into a *dirty*
  working tree (uncommitted local changes) is fragile in go-git. **Workaround:**
  we commit local changes *first*, then run the 3-way merge between commits.
  This produces **identical observable results** (convergence, fast-forward,
  conflict capture, local-commit-lands-on-push-failure) and is exactly how real
  git merging works (merges operate on commits, not a dirty tree).
- **Unborn HEAD / default branch.** `git.PlainInit` leaves HEAD pointing at an
  unborn `master` (go-git's default). The first commit must go to `main`, so we
  set `HEAD → refs/heads/main` symbolically before the first commit. Phase 2
  must do the same and make the branch configurable.
- **Fetch must not touch the checked-out branch.** A refspec like
  `+refs/heads/main:refs/heads/main` would rewrite the branch you're standing
  on and desync HEAD. Always fetch into `refs/remotes/origin/*`.
- **Deletions are best-effort in the spike.** Remote deletions are removed from
  the working tree and staged via `AddWithOptions{All: true}`. For robustness,
  Phase 2 should also call `wt.Remove(path)` explicitly for deleted paths
  (the 6 spike scenarios don't exercise deletions, but real stores will).
- **`AddWithOptions` returns only `error`** (not `(hash, error)`, unlike `Add`).
  And `CommitOptions.Author`/`Committer` are `*object.Signature` (pointer), not
  values.
- **`flock.TryLockContext(ctx, retryDelay)`** is the blocking-with-timeout
  acquire (there is no `LockContext` on `gofrs/flock` v0.13). The store-wide
  `.sync.lock` serializes concurrent `Sync()` calls; without it, concurrent
  go-git index writes would error/corrupt.

## HTTPS + PAT auth mechanics (for the real Phase 2 wiring)

The spike uses local bare repos, so **no auth** is exercised. For real GitHub,
go-git's transport layer supports a fine-grained PAT cleanly:

- Set `Auth` on `CloneOptions` / `FetchOptions` / `PushOptions` to
  `&https.BasicAuth{Username: "x-access-token", Password: pat}` (username is
  ignored for token auth; the PAT goes in the password field). This is the
  standard go-git PAT path.
- Store the PAT `0600` at `~/.dossier/credentials` (outside the repo, gitignored)
  per ADR 0005. Auto-detect `gh auth token` as a convenience path (the `gh` CLI
  holds a token that can be reused verbatim).
- Expired/invalid tokens surface as a transport error from fetch/push → degrade
  to a `sync_auth_failed` warning with the exact re-auth command (degrade
  visibly; the local commit still lands).
- No SSH key path is needed for the GitHub-only v1 (PAT over HTTPS keeps the
  single-binary, no-ssh-agent story).

## Shallow-clone support

- `CloneOptions{Depth: n}` produces a shallow clone (`--depth n`). For
  `dossier team join`, a bounded shallow clone (e.g. `Depth: 50`) keeps the
  initial clone fast on stores with long histories. The store's truth is files,
  not history, so deep history is rarely needed for sync correctness.
- Caveats: shallow clones can have limitations pushing to non-shallow remotes
  and with `MergeBase` across the shallow boundary. For *team join* (clone a
  fresh store) shallow is fine; for ongoing sync, a full (or deep-enough) fetch
  is safer so `MergeBase`/`IsAncestor` always resolve. Recommendation: shallow
  only the initial join, then fetch normally.

## Fetch / merge granularity

- Fetch is refspec-based and can target a single branch
  (`+refs/heads/main:refs/remotes/origin/main`) or all branches
  (`+refs/heads/*:refs/remotes/origin/*`). The spike fetches all branches for
  simplicity; Phase 2 can narrow to the configured branch.
- Merge granularity is **tree-level** via `DiffTree` (merkletrie). go-git does
  not expose a line-level or per-hunk remote-wins policy; our file-level
  remote-wins is the right granularity for the "single multi-writer file is
  `dossier.md`" design (per the plan, everything else is single-writer and
  cannot conflict).

## Recommendation list for the real Phase 2 wiring

1. **Define `core.Syncer` port** in `internal/core` (read-only in this spike)
   and make `internal/sync.GitSync` satisfy it. Core stays pure; `internal/core`
   never imports `internal/sync`.
2. **Reuse the existing conflict machinery.** Today the spike captures the *full*
   local content in `ConflictRecord`. Phase 2 should first attempt `core`'s
   **non-overlapping-frontmatter auto-merge**; only when bodies actually diverge
   should it escalate to a `conflicts/<id>.md` artifact (`kind:
   sync_concurrent_edit`), exactly like a local concurrent edit (one mechanism,
   two triggers — per ADR 0005 §4).
3. **Inject config:** `author` from `config.yaml`, remote URL from a new
   `team.remote` config key, branch default `main`.
4. **Auth:** PAT at `~/.dossier/credentials` (0600, outside repo); pass to
   go-git as `https.BasicAuth`; fall back to `gh auth token`.
5. **Auto-sync (Phase 3):** pull-before-`Recall`/`SessionStart`,
   commit+push-after-`Save`/`session-end`, debounced/async so no operation
   blocks on the network; failures degrade to warnings and the next sync
   retries. Keep the `.sync.lock` so debounced pushes don't collide with an
   explicit `dossier sync`.
6. **Shallow join:** `Depth: ~50` on `team join` only; full fetch thereafter.
7. **Deletions:** explicitly `wt.Remove` deleted remote paths (don't rely solely
   on `Add All`).
8. **Stream large files:** land remote blobs via `blob.Reader()` + `io.Copy`
   (already done in the spike); avoid `File.Contents()` for anything that could
   be large.
9. **Branch handling:** always fetch into `refs/remotes/origin/*`, never the
   checked-out branch; set HEAD → `main` explicitly on init.
10. **Version:** pin `github.com/go-git/go-git/v5` (v5.19.1 used here). A v6 is
    tagged but is not the default module; stay on v5 unless an explicit upgrade
    is undertaken.
11. **Real-GitHub dogfood (Phase 4):** the spike proves the engine on local bare
    repos; the remaining unknowns (PAT flow UX, GitHub rate limits, large-repo
    clone time, revoked-token-mid-push) are dogfood questions, not go-git
    capability questions.

## Test coverage (all green, all local bare repos)

- (i) two clones converge after alternating writes+syncs — `TestSync_TwoClonesConverge`
- (ii) clean fast-forward pull (both directions) — covered by the above
- (iii) both-modified `dossier.md` → remote in working tree, local in a
  `ConflictRecord`, **no git conflict markers anywhere** — `TestSync_BothModifiedDossierMD`
- (iv) >100 MB file excluded from commit + reported, never in git history
  (generated as a sparse hole, not 101 MB on disk) — `TestSync_OversizedFileExcluded`
- (v) concurrent `Sync()` calls serialize via `.sync.lock` — `TestSync_ConcurrentSerialize`
- (vi) push to an unreachable remote → visible `report.Error` while the local
  commit still lands — `TestSync_UnreachableRemoteErrorLocalCommitLands`
- plus: `.gitignore` machine-local exclusion + read/merge idempotence
  (`TestSync_GitignoreMachineLocal`, `TestSync_GitignoreMergePreservesUserEntries`)
  and `Status()` ahead/behind + last-sync (`TestStatus_ReportsAheadBehind`).

## Files (scope of this spike)

`internal/sync/`: `doc.go`, `gitsync.go` (types), `sync.go` (Sync/Clone/commit),
`merge.go` + `tree.go` (remote-wins 3-way merge), `status.go` (Status/divergence/push),
`util.go`, `lock.go`, `gitignore.go`, `state.go`, and `*_test.go`. Plus
`go.mod`/`go.sum` (added `go-git/v5` + transitive deps; `gofrs/flock` was already
present). No changes to `internal/core`, `internal/store`, `internal/config`,
CLI/MCP/TUI, `cmd/`, or any docs outside `docs/spikes/`.
