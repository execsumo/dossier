# Team Sync — Validation After the P0 Fixes

> Created 2026-09-18 from [`team-adoption-plan-review.md`](team-adoption-plan-review.md).
>
> **When to use this:** after P0-1 to P0-7 and P0-9 (review §6) are implemented, before the
> pilot starts. It is a procedure to run, not a list of open work. Every check
> states what it verifies, how to run it, and what passing looks like. A failure
> means the fix is not done; it does not mean this document needs changing.
>
> **Scope:** Parts A–C need nothing but the repo. Part D needs a real GitHub repo
> and a token, and is run by the owner. Part E is the go/no-go for the pilot.
>
> Where a P0 fix introduces a new command or flag, this doc names the *behavior*
> it requires. Use whatever name the implementation chose, and record it here the
> first time you run the check.

## Names chosen by the implementation (recorded 2026-09-18)

| Behavior | Name |
|---|---|
| Skip the `team create` confirmation | `dossier team create <url> --yes` |
| Where a failed join/create is moved | `<DOSSIER_HOME>.failed-join-<UTC>/`, `<DOSSIER_HOME>.failed-create-<UTC>/` (siblings of the store) |
| Auth failure code | `sync_auth_failed` (CLI exit 1; MCP error code) |
| Missing credentials warning | `Warning: no credentials found for <url>` (stderr, every command wired to an http(s) remote) |
| List / resolve conflicts (CLI) | `dossier conflicts`; `dossier resolve <id> --keep-shared\|--restore-mine\|--keep-both` |
| List / resolve conflicts (MCP) | `dossier_conflicts`; `dossier_resolve_conflict` (`conflict_id`, `choice`) |
| Resolve conflicts (TUI) | `x` opens the conflicts overlay; `1`/`2`/`3` choose |
| Health line | `Health:` first line of `dossier doctor` and `dossier sync --status`; TUI footer |
| Full report key (TUI) | `H` |
| Raw promote transcript | `artifacts/art_<n>_raw.*`, gitignored as `*/artifacts/*_raw.*` |

## Run log

- **2026-09-18 (development session):** Part A passed on `review/team-adoption-plan` after the P0 merges. Part C C1–C9 passed in a sandbox (fake `HOME`, throwaway `DOSSIER_HOME`s, local bare repos; C6 against a local HTTP server answering 401, with `gh` off `PATH`; C8 with the real TUI in a terminal pane). Parts D and E not run.

## Part A — Automated gate

Run from the repo root. All must pass.

```bash
go build ./... && go vet ./... && test -z "$(gofmt -l .)" && git diff --check
go test ./...
go test -race ./internal/sync/... ./internal/cli/... ./internal/mcp/...
```

The P0 acceptance tests named in review §6 must exist and pass. There should be
at least one test per fix:

| Fix | The test must show |
|---|---|
| P0-1 | Create against a non-empty remote errors and leaves the remote unchanged. Create on a store with Dossiers lists all of them, archived ones included. Declining leaves no `.git/` and no `team.remote`. |
| P0-2 | A failed join, then a good join, succeeds. A failed create, then a good create, succeeds. What the failed attempt created is moved aside, not deleted. |
| P0-3 | An unreachable remote gives a non-zero CLI exit and no "successful" text. `last_success_pull`/`last_success_push` are unchanged. A no-op push reports `Pushed=false`. |
| P0-4 | An unresolved conflict still counts after a later clean sync. The `doctor` sync block agrees with its issue list. |
| P0-5 | Resolving a conflict (keep shared / restore mine / keep both) archives the conflict file under `conflicts/resolved/` and clears the `doctor` issue. The same result through CLI, MCP and TUI. |
| P0-6 | An https remote with no credentials produces an explicit warning. A 401/403 produces `sync_auth_failed` with a next step. |
| P0-7 | Promote with JSONL content in a team store: the raw JSONL artifact is absent from the remote, and the compiled transcript is present. |
| P0-9 | A core health-summary table test across the fixture states (never synced, synced, failed, conflicts, issues). A TUI test where the footer renders the summary and an unreachable remote doesn't block the first render. A golden test where the TUI footer text equals the CLI summary for the same store. |

## Part B — Sandbox setup

Everything runs in a throwaway directory with a fake `HOME`, so no real store,
credentials or Claude/Pi config is touched. The bare repo stands in for GitHub.

```bash
T=$(mktemp -d /tmp/dossier-validate.XXXX)
go build -o "$T/dossier" ./cmd/dossier
cd "$T"
mkdir fakehome && export HOME="$T/fakehome"
git init -q --bare -b main remote.git      # HEAD -> main, like a new GitHub repo
git init -q --bare -b main empty2.git      # a second empty remote
D="$T/dossier"
mgr()  { DOSSIER_HOME="$T/mgr"  "$D" "$@"; }   # the manager's single store
col()  { DOSSIER_HOME="$T/col"  "$D" "$@"; }   # the colleague's single store
```

## Part C — Sandbox checks (the defects found on 2026-09-18)

Each check reproduces a defect the review observed. Run them in order; later
checks reuse earlier state.

### C1 — `team create` publishes only what you confirm (P0-1)

```bash
mgr init --yes >/dev/null
mgr promote "Team pricing review" >/dev/null
mgr promote "Private topic" >/dev/null && mgr archive private-topic >/dev/null
# Put something in empty2.git so it is non-empty:
DOSSIER_HOME="$T/other" "$D" init --yes >/dev/null
DOSSIER_HOME="$T/other" "$D" team create "$T/empty2.git" --yes
mgr team create "$T/empty2.git"            # non-empty remote
```

**Pass:** refused, with a message saying the remote is not empty.
`git --git-dir=empty2.git ls-tree --name-only main` shows no `team-pricing-review`.
`mgr/` has no `.git/`, and `config.yaml` has no `team.remote`.

```bash
mgr team create "$T/remote.git"            # empty remote; answer "no"
```

**Pass:** lists **both** Dossiers, including the archived `private-topic`, and
asks for confirmation. Answering no leaves no `.git/` and no `team.remote`.

Move `mgr/private-topic` out of the store (`mv mgr/private-topic "$T/"`). Then
run create again and answer yes.

**Pass:** succeeds. `git --git-dir=remote.git ls-tree --name-only main` lists
`team-pricing-review` and not `private-topic`.

### C2 — A failed join can be retried (P0-2)

```bash
col team join "$T/does-not-exist.git"; echo "exit=$?"
ls -A col 2>/dev/null; grep -c remote col/config.yaml 2>/dev/null
col team join "$T/remote.git"; echo "exit=$?"
```

**Pass:** the first join exits non-zero with a plain message. Afterwards `col/`
holds no `.git/` and no `team.remote` (a moved-aside folder is fine). The
second join exits 0 and `col ls` shows "Team pricing review".

Also check a remote whose default branch is not `main`:
`git init -q --bare -b master odd.git`, then join it from a fresh
`DOSSIER_HOME`. **Pass:** either it works, or it fails with a message naming
the branch, and a retry against `remote.git` then succeeds.

### C3 — Sync tells the truth (P0-3)

```bash
col sync; echo "exit=$?"                                # nothing to do
mv remote.git remote.moved
col sync; echo "exit=$?"
col sync --status
mv remote.moved remote.git
```

**Pass:**
- The first sync does **not** say "Pushed local changes" when nothing was
  pushed.
- The offline sync exits **non-zero**, does not print "successful", and names
  the failure.
- `--status` shows the last *successful* pull and push, unchanged by the failed
  attempt, and shows the last error.

### C4 — Conflicts stay counted until resolved (P0-4)

```bash
f=team-pricing-review/dossier.md
chmod u+w mgr/$f col/$f; echo "manager edit" >> mgr/$f; echo "colleague edit" >> col/$f; chmod a-w mgr/$f col/$f
mgr sync; col sync                          # colleague gets the conflict
col sync                                    # a clean sync afterwards
col sync --status; col doctor
```

**Pass:** the second `col sync` warns that a conflict file was written. After
the clean third sync, `--status` still shows **1** conflict. `doctor`'s Team
Sync block and its issue list both show 1.

> Editing `dossier.md` directly (it is written read-only) only simulates two
> machines saving at once. In real use, the edits come from `dossier_save` in
> two Claude sessions.

### C5 — A conflict can be resolved without touching files (P0-5)

Resolve the C4 conflict three times, once per choice (keep shared, restore
mine, keep both), recreating the conflict as in C4 between runs. Use the CLI
once, the TUI once, and an MCP call once:

```bash
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | col mcp serve | grep -o '"name":"dossier_[a-z_]*"'
```

That shows the resolve tool's name.

**Pass:** each choice gives the expected `dossier.md` body. The conflict file
moves to `conflicts/resolved/` and is not deleted. `col doctor` reports no
unresolved conflict. The audit log records who resolved it and how.

### C6 — Missing or bad credentials are explicit (P0-6)

The sandbox remote is a local path, so it needs no auth. To exercise this,
point a scratch store's `team.remote` at `https://github.com/<you>/<nonexistent>.git`
with no `$HOME/.dossier/credentials` and `gh` absent from `PATH`.

**Pass:** any `dossier` command warns once that no credentials were found. It
does not fail silently. `sync` shows `sync_auth_failed` (or its successor) with
a concrete next step. Part D repeats this against real GitHub.

### C7 — Raw transcripts stay local; compiled ones sync (P0-7, decision #5)

```bash
printf '%s\n' '{"type":"user","message":{"role":"user","content":"hello"}}' \
  '{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"hi"}]}}' > s.jsonl
col promote "Transcript check" --from-file s.jsonl >/dev/null
col sync >/dev/null
git --git-dir=remote.git grep -l "secret" main -- 'transcript-check/*' ; echo "raw-hits=$?"
git --git-dir=remote.git ls-tree -r --name-only main transcript-check/artifacts
```

**Pass:** no remote file contains `secret`, so the grep prints nothing and
`raw-hits=1`. The compiled transcript artifact **is** present in the remote.

### C8 — The TUI shows health without anyone running `doctor` (P0-9)

Run the TUI against the colleague store in a second terminal (`col tui`), then
repeat C3's offline step and C4's conflict step in the first terminal.

**Pass:**
- The TUI opens immediately, even while `remote.git` is moved away (offline).
- Within about a minute, with no keypress, the footer changes: it shows "last
  sync failed …" while offline, and "1 conflict" after C4.
- The footer's numbers match `col sync --status` and `col doctor`.
- The health key opens the full report as an overlay.
- On a store with no team remote, the footer shows local issues only and no
  sync wording.

### C9 — Regressions: behavior that already worked must still work

| Check | Pass |
|---|---|
| Two stores converge after `mgr sync; col sync; mgr sync` | Identical `dossier.md` bytes on both |
| Conflict never produces merge markers | `grep -rl '^<<<<<<<' mgr col` finds nothing |
| Machine-local files never reach the remote | `git --git-dir=remote.git ls-tree -r --name-only main` shows no `config.yaml`, `credentials`, `sessions/`, `context/`, `.syncstate.json` |
| Per-slug raw session stash never reaches the remote | No `*/sessions/*` paths in the listing above |
| >100 MB file excluded with a warning | `truncate -s 101M col/team-pricing-review/files/big.bin; col sync` warns; file absent from remote |

## Part D — Live GitHub test (owner)

Run once, on one machine, with two separate `DOSSIER_HOME`s standing in for the
manager and the colleague. About 15 minutes.

**Prepare:**
1. On GitHub, create an **empty** private repo with no README, keeping the
   default branch `main`.
2. Create a fine-grained token with Contents read/write on that repo only.
   Write it to `~/.dossier/credentials` and run `chmod 600` on it. Note that
   this path is shared by both test stores, because it ignores `DOSSIER_HOME`.
3. Use throwaway store paths, e.g. `DOSSIER_HOME=/tmp/live-mgr` and
   `/tmp/live-col`.

| # | Do | Pass |
|---|---|---|
| D1 | Manager: `init`, promote one Dossier, `team create https://github.com/<you>/<repo>.git` | Lists the Dossier, asks to confirm, pushes. The repo on GitHub shows the slug folder and `.gitignore` only. |
| D2 | Colleague: `team join <url>` | Exits 0. `ls` shows the Dossier. |
| D3 | Manager edits `lead`/`next`, then syncs. Colleague starts a Claude session (or runs `sync`) | The change is visible to the colleague, fast-forwarded. `config.yaml` is still present in the colleague's store. |
| D4 | Both change the same Dossier body via two Claude sessions, then both sync | One conflict on the later syncer. The C5 resolve path clears it. |
| D5 | Disconnect the network; save and sync | Save succeeds; sync fails **visibly** with a non-zero exit. Reconnect and sync: it catches up. |
| D6 | Revoke the token on GitHub, then sync | `sync_auth_failed` (or its successor) with a next step, not "Sync successful". |
| D7 | Delete `~/.dossier/credentials` with `gh` signed in; sync | Works through `gh`, and says it is using `gh`. |
| D8 | Restore the token; open Claude in any folder with the colleague store as `DOSSIER_HOME` and ask "what's assigned to <name>?" | Finds and binds the Dossier (the MCP primary path). |

Afterwards, delete the test repo and revoke the token.

## Part E — Pilot go/no-go

Start the pilot only when all of these are true:

- Parts A, C and D pass.
- The manager has seen the TUI health footer report a real problem during Part
  D (offline or revoked token), and clear again afterwards.
- The runbook and onboarding status banners are updated: struck claims that
  are now true are restored, and anything still untrue stays struck.
- **Pre-create check (manager's real store):** every Dossier directory in
  `~/.dossier` is team-safe. Nothing contains PII, credentials, or HR and
  compensation material, and no session that read such material is archived in
  it (transcripts sync). Anything else has been moved out of `~/.dossier`.
  Archiving is not enough.
- The colleague's `~/.dossier` does not exist before joining.
- The colleague knows the one primary path: open Claude in the work folder, ask
  for their Dossier by name, and ask Claude to save as decisions land.

Decisions in force (review §4): Claude Code only · a single store per person ·
MCP primary path · compiled transcripts sync · banned content as above · the
manager creates, assigns and closes, the colleague marks `review`/`blocked`,
and only the lead edits the body · success criteria per decision #11.
