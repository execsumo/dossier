# Team Sync — Operator Runbook

> Audience: the person who set up the team store. This is a failure-drill reference.
> Each entry: **symptom → what happened → what to do**.

> **Status (Pilot):** The team sync commands are built and work locally, but the shared GitHub flow is being piloted and is not yet validated against live GitHub. Treat this as an experimental feature.

> **Status: corrected after the P0 fixes (review 2026-09-18, updated the same day).** The review struck claims about warnings, commands and TUI screens that did not exist. Those that the P0 fixes made true are restored below; those still untrue stay struck, with the actual behavior. The fixes are validated in a sandbox (`team-sync-validation.md` Parts A and C), not yet against live GitHub (Part D). Fix list: [`team-adoption-plan-review.md`](team-adoption-plan-review.md) §6.
>
> **Before creating a team store:** `team create` publishes **every Dossier already in the store**, archived ones included. It lists them and asks you to confirm, and it refuses a remote that is not empty. Use an empty repo whose default branch is `main`. Pilot policy is a **single store per person** (owner decision 2026-09-18), so your existing `~/.dossier` becomes the team store. First move every Dossier directory that isn't team-safe out of `~/.dossier` to a folder outside it. `dossier archive` is not enough: archived Dossiers stay in the store and sync. Read the list `team create` prints before you answer yes.

**Setup recap (you, once):** you created the team store with `dossier team create <url>`, which initializes and pushes the existing store to an empty private repo and writes `team.remote` to config. Each colleague then joins with `dossier team join <url>`. Sync transport is fully hidden inside the binary; this runbook may reference the mechanism (commits, push/pull, the remote, the working tree) but you never run raw version-control commands yourself.

## Quick reference

| Symptom | What happened | Immediate action |
|---|---|---|
| "can't reach the team store" / sync deferred | Remote unreachable (offline or bad URL) | Local commit still landed; retry `dossier sync`; inspect with `dossier sync --status` |
| `Sync failed: …` (exit code 1) | Any remote failure. The local commit landed | See §1 |
| `sync_auth_failed` | Sign-in missing, expired, or lacks access | Run `dossier signin`; see §2 |
| "Your GitHub account can't see <owner/repo>" | Not added to the repo, or invitation not accepted | Add them as a collaborator; they accept and rerun; see §2a |
| "Install GitHub CLI with …" on join | `gh` not installed | Install `gh`, run the same command again |
| `Warning: no credentials found for <url>` on every command | No token file and `gh` not signed in | Run `dossier signin`; see §2 |
| a `<store>.failed-join-<time>` or `.failed-create-<time>` folder next to the store | An earlier join or create failed; what it created was moved aside | See §6 |
| TUI footer: `Team sync · last sync failed …`, or Claude says the team sync failed | The most recent sync (manual or automatic) failed | Run `dossier sync` to see why; see §1/§2 |
| ">100 MB" exclusion warning | File exceeds GitHub's 100 MB hard limit | Stays local, never enters shared history; move it out of the store or reference it externally |
| new `conflicts/<id>.md`, `kind: sync_concurrent_edit`; TUI footer shows `1 conflict` | Two machines edited the same `dossier.md` body | Remote won the working tree; local version preserved as the conflict note; resolve it (TUI `x`, `dossier resolve`, or ask Claude); see §4 |
| machine-local files absent from the store | `config.yaml`, root `sessions/`, `context/`, locks are excluded by design | Nothing — this is correct; these are per-machine and must not sync |

## 1. Remote unreachable (offline / bad URL)

**Symptom:** a sync warning that the team remote can't be reached (you're offline, or the URL is wrong or changed).

**What happened:** Dossier is local-first. Your save committed locally regardless of connectivity; only the push to — and pull from — the team remote was deferred. Nothing is lost; the change simply waits on your machine.

**What to do:**

- Fix connectivity or correct the remote URL, then retry:

  ```text
  dossier sync
  ```

- Inspect the queue without changing state:

  ```text
  dossier sync --status
  ```

  It reports unpushed commits, a diverged remote, or stale credentials: a `Health:` line (the same line as the TUI footer), then the last attempt, last successful pull and push, last error, auth state (`file`, `gh`, `none`, `missing`, `rejected`), ahead/behind, uncommitted changes, and unresolved conflicts. Only successful runs move the "last pull/push" times. The conflict count comes from the files in `conflicts/`, so it stays until each conflict is resolved.

- The next successful sync catches everything up. A deferred push never blocks a save. A deferred push is a visible warning: a manual `dossier sync` exits 1 with "Sync failed", and "Pushed local changes" appears only when commits were actually sent. Background syncs report too: the next Claude session start tells the agent (and so the user) when the last sync failed or a conflict is waiting, and a failed background sync during a session is mentioned once in Claude's next Dossier response. The TUI footer shows `last sync failed …` within about a minute. Off VPN, a Claude session start waits at most about 5 seconds for the team remote; a manual `dossier sync` reports the failure within about 10 seconds.

*Sources: `docs/adr/0005-team-sync-via-github.md` §5; `docs/team-sync-plan.md` Non-negotiables (Local-first); `SPEC.md` §7.2 `dossier sync`.*

## 2. Sign-in missing, expired or rejected

**Symptom:** a `sync_auth_failed` error. `dossier sync` exits 1 and prints the next step. If there is no token file and `gh` is not signed in, every command also warns `no credentials found for <url>`. `sync --status` shows the auth state (`missing` or `rejected`).

**What happened:** Dossier reaches the team repository over HTTPS with a GitHub credential. It first looks for a token file at `~/.dossier/credentials`; if there is none, it asks the GitHub CLI (`gh auth token`). The credential is missing, expired, revoked, or lacks access.

**What to do:**

- Re-run the **re-auth command printed in the `sync_auth_failed` warning**: `dossier signin`. It offers the same browser sign-in as `team join` (`Sign in to GitHub now? Your browser will open. [Y/n]`, then a one-time code on github.com) and reports which method is active (`gh` or the token file). It needs `gh` installed; if it isn't, it prints how to install it (macOS `brew install gh`, Windows `winget install --id GitHub.cli`).
- Token file alternative: a fine-grained PAT with Contents read/write on the team repo, saved to `~/.dossier/credentials`. On macOS and Linux it **must be exactly `0600`**, or every command warns "failed to load credentials"; on Windows the user-profile folder's permissions protect it and the mode is not checked. Dossier never writes this file itself. The path ignores `DOSSIER_HOME`.
- A token file, when present, wins over `gh`. Remove or fix a stale one if `gh` is signed in but sync still fails.
- The `gh` credential is the person's general GitHub sign-in, not limited to the team repo (accepted in BUILD-DECISIONS B17). It is looked up on every `dossier` command that syncs.

## 2a. "Your GitHub account can't see <owner/repo>"

**What happened:** sign-in worked, but GitHub says the account has no access to the team repository. For a private repository GitHub answers "not found" in this case. Usually the colleague hasn't been added yet, or hasn't accepted the invitation email.

**What to do:** add them in the repository's Settings → Collaborators; they accept the email invitation; they run the same `dossier team join <link>` again. Nothing was written to their computer by the failed attempt.

If the message mentions SAML or single sign-on, the organization requires the GitHub CLI to be authorized for it: GitHub → Settings → Applications → authorize the GitHub CLI for the organization, then run the command again.

*Sources: BUILD-DECISIONS B17; `docs/team-adoption-plan-review.md` §6 Team MVP M6; `SPEC.md` §7 `dossier team join` and `dossier signin`.*

## 3. Oversized artifact (>100 MB)

**Symptom:** a persistent warning that a file larger than 100 MB was excluded from sync (GitHub also warns above 50 MB).

**What happened:** GitHub's hard limit is 100 MB per file (it warns above 50 MB). Dossier's local artifact cap is higher (1 GB), so the file is kept locally but **excluded from the shared store** — it never enters shared history. This is a visible, non-silent exclusion.

**What to do:**

- The file is **not lost** — it remains on your machine.
- Don't try to sync it as-is. Dossier's supported artifacts are text (Markdown, JSON, TXT); native binary attachment storage is out of scope. Move the large file out of the store and reference it externally, or remove it from the dossier.
- After removing or moving it, the next sync no longer warns about it.

*Sources: `docs/adr/0005-team-sync-via-github.md` Consequences; `docs/team-sync-plan.md` Phase 2 §5; `SPEC.md` §7.2 `dossier sync` and §2.8 (text-first artifacts).*

## 4. A `dossier.md` sync conflict

**Symptom:** a new `<slug>/conflicts/<id>.md` appears with `kind: sync_concurrent_edit`, plus a sync warning (and the TUI footer shows a conflict count). The warning line itself appears only on a manual `dossier sync`; background syncs write the file without printing, and the footer picks it up within about a minute. `show`/`ls` still do not mention conflicts. `dossier conflicts` lists them.

**What happened:** two machines edited the same `dossier.md` — the only genuinely multi-writer file. On pull, Dossier first attempts the non-overlapping-frontmatter auto-merge; if the body truly conflicts, it does **not** merge the body with markers. Instead:

- the **remote (already-shared) version wins the working tree**, and
- the **local version is preserved** as a `conflicts/<id>.md` note (`kind: sync_concurrent_edit`).

Nothing is lost; there are **never merge markers** in the store.

**What to do:**

- Reconcile in Dossier's **conflict-resolution view (the TUI)**, which shows the local and remote sides so you can keep what you want. It is a choice between three outcomes, not a line-by-line merge editor. From any surface:
  - TUI: press `x` on the dashboard or a Dossier's detail view and select the conflict. The shared version and yours appear side by side (`d` shows the diff). Then `1` keep shared, `2` restore mine, or `3` keep both.
  - CLI: `dossier conflicts` lists ids; `dossier conflicts <id>` shows both versions and the diff; `dossier resolve <conflict-id> --keep-shared|--restore-mine|--keep-both`.
  - Claude: `dossier_conflicts` (with `conflict_id` for the comparison), then `dossier_resolve_conflict`.
  For a partial merge, choose **keep both**: the preserved version is appended under `## Unresolved disagreement (conflict <id>)`, and the lead edits the body down in a normal save.
  - Restore mine and keep both each create a new revision through the normal save path; nothing is overwritten.
  - The conflict file moves to `conflicts/resolved/`, stamped with who resolved it, when, and how. The audit log records a `conflict_resolved` event. Nothing is deleted.
  - Pilot convention (review §4 #7): the Dossier's lead resolves conflicts on that Dossier.
- Verify nothing is left unresolved:

  ```text
  dossier doctor
  ```

  `doctor` lists unresolved conflicts as issues, and its Team Sync block's "Unresolved conflicts: N" agrees with that list. Resolved conflicts are not counted.
- This is the same conflict flow used for local concurrent edits — **one mechanism, two triggers.**

*Sources: `docs/adr/0005-team-sync-via-github.md` §4; `docs/team-sync-plan.md` Phase 2 §3; `BUILD-DECISIONS.md` §5 (conflict artifact format, `doctor` reports unresolved conflicts); `SPEC.md` §7.2.*

## 5. Machine-local files that never sync

**Symptom (informational):** certain files are intentionally absent from the shared store; `dossier doctor` may report machine-local files as healthy / local-only.

**What happened:** Dossier auto-generates an ignore list that keeps machine-specific files out of the shared store. These never sync, by design:

- `config.yaml` — your machine's install settings and detected capabilities.
- root `sessions/` — your machine's session-to-dossier bindings.
- `context/` — Dossier's locally generated context (library, guide).
- `.lock` files — local coordination locks.

**Why:** these are per-machine. Syncing them would clobber another machine's setup. Only team-relevant content syncs: distilled notes (`<slug>/dossier.md`), the archive, per-author audit shards, ~~per-dossier session stashes,~~ and revision history.

**Actual (2026-09-18, after P0-7):** per-dossier session stashes (`<slug>/sessions/`) do **not** sync. The raw JSONL copy that `promote` keeps (`artifacts/art_<n>_raw.*`, which includes the model's thinking) does **not** sync either. "The archive" (`artifacts/`) **does include compiled session transcripts** (owner decision: transcripts sync), which contain what was typed, tool results, file contents and command output, with thinking removed. `files/` syncs too. Existing team stores pick up the new exclusion on their next sync; a raw copy committed *before* that is still in the history.

**What to do:** nothing — this is correct behavior. If a teammate's `config.yaml` looks different from yours, that's expected and right.

*Sources: `docs/adr/0005-team-sync-via-github.md` §5 + Consequences; `docs/team-sync-plan.md` Non-negotiables (Machine-local stays local) + Repo/store shape; `BUILD-DECISIONS.md` B12; `SPEC.md` §3.2.*

## Healthy-state checks

- `dossier sync --status` — read-only: health line, last successful pull/push, last error, auth state, unpushed commits, diverged remote, unresolved conflicts.
- `dossier doctor` — store integrity, unresolved conflicts, provenance references, and harness/capability status.
- TUI footer — a glanceable status line on the dashboard and detail views, e.g. `Team sync · synced 2m ago · 1 conflict`, or `Team sync · last sync failed 18m ago · work is safe locally`. It is checked in the background when the TUI starts, when the store changes (at most once a minute), and every minute. It never blocks the TUI, even offline. `H` opens the full `doctor` report. The line is the same as the `Health:` line of `dossier doctor` and `dossier sync --status`. The footer replaces the daily `doctor` run.

*Sources: `SPEC.md` §7.2; `docs/team-sync-plan.md` Phase 3 §4 (Surfacing).*

## 6. `team join` or `team create` failed

**What happened:** the command printed why (wrong link, no access, no network, a remote that is not empty, or a remote whose default branch is not `main`) and exited non-zero. It saved nothing to `config.yaml`. Whatever it had created was moved to a folder next to the store, named `<store>.failed-join-<UTC time>` or `<store>.failed-create-<UTC time>`, so the store is back where it started. Nothing is deleted.

~~A failed clone leaves a partial `.git/` folder and the retry is refused as "not empty".~~ Fixed (P0-2); verified in the sandbox (validation C2).

**What to do:**

1. Fix the cause: the link, credentials (§2), or the repo's default branch.
2. Run the same `dossier team join <url>` or `dossier team create <url>` again.
3. Once joined, you may delete the moved-aside folder. It only holds the failed attempt.

## 7. `team create` published more than intended

**What happened:** `team create` publishes the *entire current store*, archived Dossiers included. Since the P0 fixes it lists every Dossier and asks before pushing, and it refuses a remote that already has content. Anything you confirmed is published.

**What to do:** treat anything pushed as disclosed to everyone with repo access, because git history is kept on every clone. Removing it needs a history rewrite on GitHub plus a re-clone on every machine. That is outside Dossier; escalate. To prevent it, follow the pre-create check in the status note at the top and read the list before answering yes.

## 8. A `team.yaml` roster conflict

**Symptom:** `dossier conflicts` lists a `sync_concurrent_roster_edit` conflict for "Team roster", and the health line shows a conflict.

**What happened:** two machines changed the team roster (`team add`/`team remove`) before syncing. The shared roster won; the other version is preserved in `conflicts/`.

**What to do (manager):** `dossier conflicts <id>` shows both rosters. Usually choose **keep both** (`dossier resolve <id> --keep-both`): members added on either side are kept; if one username got two different display names, the shared one wins and the clash is printed so you can fix it with `dossier team add`. To avoid this, only the manager edits the roster.

## Sources

- `docs/team-sync-plan.md` — the 4-phase plan and Non-negotiables.
- `docs/adr/0005-team-sync-via-github.md` — decision and conflict model (§4: "one mechanism, two triggers").
- `BUILD-DECISIONS.md` B12 — local-first / conflict-honest rules; machine-local files that never sync.
- `SPEC.md` §7 — the command surface (`dossier sync`, `dossier team create|join`).
- `assets/guide.md` — multi-author distillation etiquette.

