# Repair item 2: machine-local files published by Team Sync

Status: fixed in `internal/sync`, not committed. In-process regression tests fail on the pre-fix source and pass on the fix. Pre-fix CLI behavior was reproduced against a local bare remote with a temporary HOME. The coordinator independently verified baseline/fixed team create and upgraded sync through the CLI with `--home` throwaway stores and local bare remotes (below). The original HOME-override script's post-fix run remains pending; the worktree safety hook refused it and was not bypassed.

Baseline: `7354587` (main at the start of the work). Nothing touched a real remote, store, credentials or harness settings.

## Diagnosis

| Path | Observed defect (pre-fix) | Source |
|---|---|---|
| ignore defaults | `/config.yaml` is anchored and exact, so `config.yaml.bak` is not covered. `config.Migrate` writes exactly that file into the store root when it upgrades a config. Nothing covers `.DS_Store` or `*.bak`. | `internal/sync/gitignore.go:29-47` (pre-fix); `internal/config/config.go:222` (`backup := path + ".bak"`) |
| create | `team create` → `GitSync.Create` → `syncWithCtx` → `commitLocal`. That path stages whatever `wt.Status()` reports, and Status hides only files the `.gitignore` covers. So `config.yaml.bak`, `.DS_Store` and `*.bak` were published in the first push. | `internal/sync/sync.go:149-197`, `:317-361` |
| sync (tracked files) | Adding an ignore rule does not untrack a file that is already committed. go-git `Status` still reports edits to that file, because `excludeIgnoredChanges` drops only untracked inserts. `commitLocal` then republishes the new bytes on every sync. | go-git v5.19.1 `worktree_status.go:164-200`; `internal/sync/sync.go:325-341` |
| sync (merge) | `remoteWinsMerge` staged with `AddOptions{All: true}`, which re-stages edits to tracked files. It also wrote or deleted every remote-changed path in the working tree, machine-local ones included. | `internal/sync/tree.go:94-116` (pre-fix) |
| pull (fast-forward) | `ffToRemote` force-checks out the remote tree. A teammate's tracked copy therefore replaces or plants files on this machine. Only `config.yaml`, `.syncstate.json`, `sessions`, `context`, `.locks` and `.lock` were stashed around the checkout. | `internal/sync/merge.go:82-104` (pre-fix) |
| join | `Clone` checks out the remote tip, so a joiner receives every machine-local file the tip carries. `EnsureGitignore` then merges in the defaults; it runs after the checkout. | `internal/sync/sync.go:23-72` |

## Fix

The fix is limited to `internal/sync`: 5 modified files (+99/−7) and 1 new file.

1. **Defaults** (`gitignore.go:37,53-54`): adds `/config.yaml*`, `*.bak` and `.DS_Store`. `/config.yaml*` is anchored on purpose: a deliverable such as `<slug>/files/config.yaml` is shared work and keeps syncing (tested). `EnsureGitignore` already merges missing defaults into an existing store's `.gitignore`, so current stores pick these up on their next sync.
2. **Untrack without deleting local bytes** (`ignored.go:45`, called from `sync.go:319-342`). Before reading status, `commitLocal` removes from the index every entry that matches the managed set (`git rm --cached` semantics; the working file stays). This also covers content already staged. The removal is committed. Each such path produces a visible `SyncReport.Excluded` warning, and `core.Sync` already surfaces those warnings (`internal/core/service_team.go:418`). The warning text:
   > stopped syncing X: it is machine-local, but an older Dossier build published it. It is removed from the local sync index and kept on this machine; the next successful push removes the current remote copy, and earlier copies remain in the team repo's history
3. **Merge protection** (`tree.go:49,94-123,139-157`). A remote change to a managed path never writes or deletes the local file. Staging goes path by path and skips managed paths, replacing `AddOptions{All: true}`.
4. **Fast-forward protection** (`merge.go:82-123`, `ignored.go:79`). Before the checkout, sync records the local state of each managed path that differs between HEAD and the remote tree, then puts it back after the checkout:
   - a regular file gets its bytes back;
   - a symlink is recreated as the same link and is never written through (`O_EXCL` on the rewrite);
   - a path that was absent is removed again, so a pull cannot plant a teammate's copy.

   A restore failure fails the sync loudly.
5. **Scope guard.** Only the compiled-in managed set (`defaultGitignoreEntries`) drives untracking. A rule a user adds to `.gitignore` keeps normal git meaning and never untracks shared work (tested). Because the set is compiled in, an unreadable or edited `.gitignore` cannot make it fail open, and a `!` negation cannot override it.

## Regression tests

The new file is `internal/sync/ignored_tracked_test.go`. Every case uses temp dirs and local bare remotes.

| Test | Guards |
|---|---|
| `TestEnsureGitignoreCoversMachineLocalJunk` | the defaults are present in a fresh store and get merged into an older store's `.gitignore` |
| `TestManagedLocalMatcherScope` (new API, post-fix only) | root `config.yaml*`, nested `.DS_Store` and `*.bak` match; `<slug>/files/config.yaml`, `dossier.md` and `audit/` do not |
| `TestUserIgnoreRuleDoesNotUntrackSharedWork` | a user rule does not untrack a tracked shared file. It passes both before and after: it is an over-scope guard, not a regression test |
| `TestCreateDoesNotPublishMachineLocalJunk` | `GitSync.Create` against an empty bare remote: no junk in the tip, and `files/config.yaml` is published |
| `TestSyncUntracksLegacyJunkWithoutRepublishing` | An older build committed the junk. Local edits are made, including one already staged, and B advances the remote so A's sync takes the merge path. After the sync: the junk is gone from the tip, no remote object holds the new bytes, the local bytes are intact, there are exactly 3 warnings, a second sync is quiet, and B's own `config.yaml.bak` survives B's merge-pull of the removal. |
| `TestFastForwardDoesNotPlantOrReplaceIgnoredFiles` | Fast-forward of an older build's commit. B's own `config.yaml.bak` keeps its bytes, the teammate's `.DS_Store` and `files/draft.bak` are not planted, B's symlink `topic.bak` keeps its target and the outside file is untouched. B's next sync removes the legacy entries from the tip, and B's bytes never reach the remote. |

### Before: pre-fix source (`git archive 7354587`), new tests copied in minus the new-API test

```
$ git archive --format=tar -o /tmp/item2/head.tar HEAD && tar -xf /tmp/item2/head.tar -C /tmp/item2/head-src
$ cd /tmp/item2/head-src && go test ./internal/sync/ -run 'CoversMachineLocalJunk|UserIgnoreRule|CreateDoesNotPublish|UntracksLegacy|FastForwardDoesNotPlant' -v
--- FAIL: TestEnsureGitignoreCoversMachineLocalJunk (0.00s)
    --- FAIL: .../fresh_store            (.gitignore missing "/config.yaml*", "*.bak", ".DS_Store")
    --- FAIL: .../store_from_an_older_build
--- PASS: TestUserIgnoreRuleDoesNotUntrackSharedWork (0.30s)
    ignored_tracked_test.go:169: team create published machine-local config.yaml.bak
    ignored_tracked_test.go:169: team create published machine-local .DS_Store
    ignored_tracked_test.go:169: team create published machine-local topic/.DS_Store
    ignored_tracked_test.go:169: team create published machine-local topic/files/draft.bak
    ignored_tracked_test.go:169: team create published machine-local archive/done/files/notes.bak
--- FAIL: TestCreateDoesNotPublishMachineLocalJunk (0.11s)
    ignored_tracked_test.go:215: remote tip still carries config.yaml.bak ("alice v2 SECRET\n")
    ignored_tracked_test.go:215: remote tip still carries .DS_Store ("finder v2")
    ignored_tracked_test.go:215: remote tip still carries topic/files/draft.bak ("draft v2 SECRET")
    ignored_tracked_test.go:234: remote object store holds republished local bytes: "alice v2 SECRET\n"
    ignored_tracked_test.go:234: remote object store holds republished local bytes: "finder v2"
    ignored_tracked_test.go:234: remote object store holds republished local bytes: "draft v2 SECRET"
    ignored_tracked_test.go:249: want 3 visible 'stopped syncing' warnings, got []
--- FAIL: TestSyncUntracksLegacyJunkWithoutRepublishing (0.40s)
    ignored_tracked_test.go:289: config.yaml.bak local bytes = "alice\n", want "bob own\n"
--- FAIL: TestFastForwardDoesNotPlantOrReplaceIgnoredFiles (0.29s)
FAIL	dossier/internal/sync	1.695s
```

(This excerpt is condensed. The full log is `/tmp/item2/tests-before.out`.)

### After: this worktree

```
$ go test ./internal/sync/ -run 'CoversMachineLocalJunk|ManagedLocalMatcher|UserIgnoreRule|CreateDoesNotPublish|UntracksLegacy|FastForwardDoesNotPlant' -v
--- PASS: TestEnsureGitignoreCoversMachineLocalJunk (0.00s)
--- PASS: TestManagedLocalMatcherScope (0.00s)
--- PASS: TestUserIgnoreRuleDoesNotUntrackSharedWork (0.32s)
--- PASS: TestCreateDoesNotPublishMachineLocalJunk (0.10s)
--- PASS: TestSyncUntracksLegacyJunkWithoutRepublishing (0.52s)
--- PASS: TestFastForwardDoesNotPlantOrReplaceIgnoredFiles (0.32s)
ok  	dossier/internal/sync	1.827s

$ go build ./... && go vet ./... && gofmt -l .      # no output
$ go test ./...
ok  dossier/assets · internal/cli 19.6s · config · core · evaluator · exportout · harness · mcp
ok  repos · search · store · internal/sync 10.7s · tokenizer · tui 4.0s · tools/resumeeval
```

## CLI reproduction: local bare remote, temporary HOME

The script is `/tmp/item2/scripts/repro.sh <bin-for-steps-1-2> <scratch> [<bin-for-steps-3-4>]`. It sets `HOME=<scratch>/home`, runs `git init --bare -b main <scratch>/remote.git`, and uses `--home` for each store. Steps:

1. Alice gets a schema-2 config, and `dossier init --yes` migrates it, which writes `config.yaml.bak`. She promotes "Pricing review" and adds `.DS_Store` at the root and in the slug, plus `files/draft.bak`.
2. `dossier team create <bare> --name Alice -y`.
3. Alice edits the junk files and runs `dossier sync`.
4. Bob pre-writes `author: bob`, runs `dossier team join <bare>`, adds his own junk and runs `dossier sync`.

Binaries come from `go build -o /tmp/item2/bin/dossier-{before,after} ./cmd/dossier`, at `7354587` and at this worktree respectively.

### Before (observed, `dossier-before`, `/tmp/item2/before.out`)

This run used the script before the `files/config.yaml` and step-5 lines were added. Steps 1–4 are otherwise identical.

```
== 1. alice: v2 config (upgrade path) + init -> Migrate writes config.yaml.bak
init exit=0
config.yaml
config.yaml.bak
== 2. alice: team create <local bare> -y
Team store created successfully.
-- remote tree, machine-local junk only:
.DS_Store
config.yaml.bak
pricing-review/.DS_Store
pricing-review/files/draft.bak
-- store .gitignore entries of interest:
2:/config.yaml
== 3. alice edits the junk files, then dossier sync
Sync successful
- Pushed local changes
config.yaml.bak  1d4745d -> 6750fc9
.DS_Store        1ae6ea4 -> 0d5b590
draft.bak        b9a6fbb -> eae2a25
== 4. bob joins
-- alice's machine-local files now on bob's disk:
.DS_Store / config.yaml.bak / .DS_Store / draft.bak
config.yaml.bak lines naming alice: 2
Sync successful
-- remote tree after bob sync, machine-local junk only:
.DS_Store
config.yaml.bak
notes.bak
pricing-review/.DS_Store
pricing-review/files/draft.bak
pricing-review/files/sub/.DS_Store
```

What this run shows:

- Alice's `config.yaml.bak` reached the remote. It holds her author name and absolute store path.
- Every edit to it was republished.
- It landed on Bob's disk when he joined.
- Bob's own junk was published too.

### After: PENDING (blocked, not bypassed)

The safety hook now refuses commands that set `HOME`, so the post-fix binary was never run end to end. The commands to run, outside the isolation hook:

```
go build -o /tmp/item2/bin/dossier-after ./cmd/dossier
bash /tmp/item2/scripts/repro.sh /tmp/item2/bin/dossier-after  /tmp/item2/run-after
bash /tmp/item2/scripts/repro.sh /tmp/item2/bin/dossier-before /tmp/item2/run-upgrade /tmp/item2/bin/dossier-after
```

Expected results, from the in-process tests above (these are expectations, not observations):

- In `run-after`: step 2 lists `pricing-review/files/config.yaml` only, and `.gitignore` shows `/config.yaml*`, `*.bak` and `.DS_Store`. Step 3 pushes nothing. Step 4 finds nothing on Bob's disk and leaves the remote junk list with only `files/config.yaml`.
- In `run-upgrade`: step 3 prints three `Warning: stopped syncing …` lines, every blob reads `(absent)`, and Alice's local bytes are kept. Step 4 finds no junk on Bob's disk, because the tip no longer carries it. Step 5 still lists the earlier commits that touched `config.yaml.bak`: that is the history exposure, which this fix does not change.

## Independent combined-worktree CLI verification (observed)

Used the supported `--home` store override without changing HOME or accessing
harness settings. All remotes below are new local bare repositories.

```sh
cd /tmp/dossier-repairs-858363ad
go build -o /tmp/dossier-repairs-final ./cmd/dossier
T=$(mktemp -d /tmp/dossier-sync-final.XXXXXX)
for phase in before after; do
  mkdir -p "$T/$phase/store"
  git init --bare -b main "$T/$phase/remote.git" >/dev/null
  printf 'author: alice\n' > "$T/$phase/store/config.yaml"
  printf 'old local settings\n' > "$T/$phase/store/config.yaml.bak"
  printf 'finder\n' > "$T/$phase/store/.DS_Store"
  BIN=/tmp/dossier-before-item4
  if test "$phase" = after; then BIN=/tmp/dossier-repairs-final; fi
  "$BIN" --home "$T/$phase/store" promote 'Sync check' --distilled '# Check' --force
  printf 'backup\n' > "$T/$phase/store/sync-check/files/draft.bak"
  printf 'shared: true\n' > "$T/$phase/store/sync-check/files/config.yaml"
  "$BIN" --home "$T/$phase/store" team create "$T/$phase/remote.git" --name Alice -y
  git --git-dir="$T/$phase/remote.git" ls-tree -r --name-only main
done
T=/tmp/dossier-sync-final.eE0TCf
printf 'new local settings\n' > "$T/before/store/config.yaml.bak"
printf 'finder new\n' > "$T/before/store/.DS_Store"
/tmp/dossier-repairs-final --home "$T/before/store" sync
git --git-dir="$T/before/remote.git" ls-tree -r --name-only main
ls -l "$T/before/store/config.yaml.bak" "$T/before/store/.DS_Store" "$T/before/store/sync-check/files/draft.bak"
/tmp/dossier-repairs-final --home "$T/before/store" sync
git --git-dir="$T/before/remote.git" log --oneline --all -- config.yaml.bak
```

Observed `/tmp/dossier-sync-final.eE0TCf`: baseline create published `.DS_Store`,
`config.yaml.bak`, and `sync-check/files/draft.bak`; repaired create published
none of those. Both published the ordinary `sync-check/files/config.yaml`.
Upgraded sync on the baseline store printed **three stopped-syncing warnings**,
pushed the index removals, retained all three local files, and removed them from
the current remote tree. Second sync printed **Already up to date**. Remote
history still listed both the original and cleanup commits for the backup,
confirming that this fix does not retract historical exposure. No real remote
was contacted or changed.

## Existing remote history exposure

What the fix does:

- Machine-local files committed by older builds stop being republished.
- The next successful push after upgrading removes them from the current tip.
- Joiners stop receiving them, and pulls never overwrite, plant or delete this machine's copies.

What the fix does not do:

- Rewrite history. Every commit that held `config.yaml.bak`, `.DS_Store` or `*.bak` remains readable by anyone with repo access, and in every existing clone.
- Contact any real remote. No cleanup was run anywhere.

For the owner to decide, not done here:

1. **Assess what leaked.** List the affected paths and commits with `git log --all --name-only -- config.yaml.bak '*.bak' '*.DS_Store'`. A `config.yaml.bak` holds `author`, `display_name`, `dossier_home` (an absolute path), `team.remote` and `repo_roots`. Credentials live in `/credentials`, which was already ignored. A user's `*.bak` file, though, could hold anything.
2. **Rotate anything sensitive** found in a leaked backup. Rotation is the only remedy that also covers existing clones.
3. **Upgrade everyone before the cleanup push lands.** An older build that pulls the removal deletes its local copy: the merge calls `removeWorkingFile`, and a fast-forward force-checks out the tree. Teammates on older builds should copy any `*.bak` they want to keep before their next sync.
4. **Rewrite history only if needed** (for example with `git filter-repo`, followed by a force-push). That is a coordinated, irreversible action: every teammate must re-join, and the "never last-write-wins" guarantee does not cover it. It needs explicit owner approval.

## Limitations and adjacent observations

- **An older build can reintroduce a file.** After a fast-forward pull of such a commit, the file stays in the tip until this machine's *next* sync commits the removal. The fast-forward itself pushes nothing. Local bytes are protected throughout (tested). This was accepted as a limitation; a post-pull scrub was not added.
- **Join still checks out whatever the tip carries.** If no upgraded machine has synced yet, a joiner receives those files, and the joiner's first sync then untracks them.
- **A user negation cannot override the managed set.** A user who needs a `.bak` file shared cannot do so with `!x.bak`; they would have to rename it.
- **Inference, not verified:** a teammate who joined a store that carries someone else's `config.yaml.bak`, and whose own config later needs migrating, would see `init` fail on "config.yaml.bak already exists" (`writeBackup` uses `O_EXCL`; `internal/config/config.go:228-236`). With the fix, the next sync removes the file from the tip. A copy that is already local stays.
- **Inference, not verified:** before this change, `AddOptions{All: true}` in the merge would also have staged a modified file over 100 MB that `commitLocal` had excluded. The new per-path staging does not check size either, so that gap is unchanged; it is out of scope here.
- `Status().Dirty` is unaffected: once a file is untracked and committed, it no longer appears in status.
- No `ARCHITECTURE.md`, `HANDOFF.md` or runbook edits were made in this worktree, per the coordinator's instruction.
