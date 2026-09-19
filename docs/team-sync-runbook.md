# Team Sync — Operator Runbook

> Audience: the person who set up the team store. This is a failure-drill reference.
> Each entry: **symptom → what happened → what to do**.

> **Status (Pilot):** The team sync commands are built and work locally, but the shared GitHub flow is being piloted and is not yet validated against live GitHub. Treat this as an experimental feature.

> **Status: operational but needs correction (review 2026-09-18).** Several entries described warnings, commands and TUI screens that do not exist. Those claims are struck through below, with the actual behavior and evidence. New §6 covers a failed join, and new §7 covers `team create` publishing the whole store. Fix list: [`team-adoption-plan-review.md`](team-adoption-plan-review.md) §6.
>
> **Before creating a team store:** `team create` publishes **every Dossier already in the store**. It does not check that the remote is empty; pointed at a non-empty repo, it merges and pushes (dogfood 2026-09-18; `internal/sync/sync.go:58-99`). Create the team store from a fresh `DOSSIER_HOME` and an empty repo whose default branch is `main` (the branch is hardcoded, `internal/cli/cli.go:1576`).

**Setup recap (you, once):** you created the team store with `dossier team create <url>`, which initializes and pushes the existing store to an empty private repo and writes `team.remote` to config. Each colleague then joins with `dossier team join <url>`. Sync transport is fully hidden inside the binary; this runbook may reference the mechanism (commits, push/pull, the remote, the working tree) but you never run raw version-control commands yourself.

## Quick reference

| Symptom | What happened | Immediate action |
|---|---|---|
| "can't reach the team store" / sync deferred | Remote unreachable (offline or bad URL) | Local commit still landed; retry `dossier sync`; inspect with `dossier sync --status` |
| `Warning: Sync network error: …` followed by "Sync successful" | Any remote failure, including auth. The command still exits 0 | Treat as a failure; see §1/§2 |
| ~~`sync_auth_failed` warning~~ (does not exist) | PAT missing, expired, or lacks the right access | Replace `~/.dossier/credentials` (mode `0600`) or sign in with `gh`; see §2 |
| `team join` retry says "target directory is not empty" | An earlier join failed and left a partial setup | See §6 |
| ">100 MB" exclusion warning | File exceeds GitHub's 100 MB hard limit | Stays local, never enters shared history; move it out of the store or reference it externally |
| new `conflicts/<id>.md`, `kind: sync_concurrent_edit` | Two machines edited the same `dossier.md` body | Remote won the working tree; local version preserved as the conflict note; reconcile in the TUI, verify with `dossier doctor` |
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

  ~~It reports unpushed commits, a diverged remote, or stale credentials.~~ **Actual:** it prints Ahead, Behind, Dirty, Conflicts and Last Sync (`internal/cli/cli.go:1512-1517`). It says nothing about credentials. Two caveats: **Last Sync is the last *attempt*, including failed ones** (`internal/sync/sync.go:170`). **Conflicts counts only the most recent sync run**, so it drops to 0 on the next run even when conflict files remain. Use `dossier doctor`'s issue list for unresolved conflicts.

- The next successful sync catches everything up. A deferred push never blocks a save. ~~A deferred push is a visible warning~~ **Actual:** it is visible only when you run `dossier sync` by hand. Session-hook and background syncs discard their result (`internal/core/service_session.go:218`, `:501`; `internal/mcp/server.go:77,96,106`). "Pushed local changes" can also be printed when nothing was pushed (`internal/sync/status.go:56`). Confirm with `--status` that Ahead is 0.

*Sources: `docs/adr/0005-team-sync-via-github.md` §5; `docs/team-sync-plan.md` Non-negotiables (Local-first); `SPEC.md` §7.2 `dossier sync`.*

## 2. PAT missing / expired / wrong mode

**Symptom:** ~~a `sync_auth_failed` warning (the token is absent, expired, or invalid).~~ **Actual:** a generic `Warning: Sync network error: …` (typically "authentication required" or a 401/403), followed by "Sync successful". `sync_auth_failed` does not exist in the code. If there is *no* credentials file and `gh` is not signed in, Dossier tries without credentials and does not say so (`internal/sync/credentials.go:59`).

**What happened:** Dossier authenticates to the team remote over HTTPS with a fine-grained personal access token (contents: **read and write** on the team repo), stored at `~/.dossier/credentials`. If that file is missing, the token has expired, or the token lacks the required access, the sync cannot authenticate.

**What to do:**

- ~~Re-run the **re-auth command printed in the `sync_auth_failed` warning.** It re-prompts for a fine-grained PAT (contents read/write on the team repo) and stores it at `~/.dossier/credentials`.~~ **Actual:** there is no re-auth command. Write a new fine-grained PAT (contents read/write on the team repo) to `$HOME/.dossier/credentials` by hand. That path is fixed and ignores `DOSSIER_HOME` (`internal/sync/credentials.go:29`).
- The credentials file **must be exactly `0600`**; any other mode makes every command warn "failed to load credentials" (`credentials.go:33-35`). ~~Dossier sets this when it writes the file~~ **Actual:** Dossier never writes this file. Set the mode yourself.
- Convenience: if the `gh` CLI is installed and signed in, Dossier uses `gh auth token` automatically when no credentials file exists (`credentials.go:51`). ~~instead of prompting~~ Note that this is your broad `gh` token, not a repo-scoped one. It is looked up again on every `dossier` command.

*Sources: `docs/team-sync-plan.md` Phase 2 §4; `docs/adr/0005-team-sync-via-github.md` Consequences; `SPEC.md` §7.2 `dossier team join`.*

## 3. Oversized artifact (>100 MB)

**Symptom:** a persistent warning that a file larger than 100 MB was excluded from sync (GitHub also warns above 50 MB).

**What happened:** GitHub's hard limit is 100 MB per file (it warns above 50 MB). Dossier's local artifact cap is higher (1 GB), so the file is kept locally but **excluded from the shared store** — it never enters shared history. This is a visible, non-silent exclusion.

**What to do:**

- The file is **not lost** — it remains on your machine.
- Don't try to sync it as-is. Dossier's supported artifacts are text (Markdown, JSON, TXT); native binary attachment storage is out of scope. Move the large file out of the store and reference it externally, or remove it from the dossier.
- After removing or moving it, the next sync no longer warns about it.

*Sources: `docs/adr/0005-team-sync-via-github.md` Consequences; `docs/team-sync-plan.md` Phase 2 §5; `SPEC.md` §7.2 `dossier sync` and §2.8 (text-first artifacts).*

## 4. A `dossier.md` sync conflict

**Symptom:** a new `<slug>/conflicts/<id>.md` appears with `kind: sync_concurrent_edit`, plus a sync warning. ~~(and the TUI footer shows a conflict count)~~ **Actual:** the warning appears only on a manual `dossier sync`. Background syncs create the file silently. The TUI has no sync footer, and `show`/`ls` do not mention conflicts. `dossier doctor` lists them as issues.

**What happened:** two machines edited the same `dossier.md` — the only genuinely multi-writer file. On pull, Dossier first attempts the non-overlapping-frontmatter auto-merge; if the body truly conflicts, it does **not** merge the body with markers. Instead:

- the **remote (already-shared) version wins the working tree**, and
- the **local version is preserved** as a `conflicts/<id>.md` note (`kind: sync_concurrent_edit`).

Nothing is lost; there are **never merge markers** in the store.

**What to do:**

- ~~Reconcile in Dossier's **conflict-resolution view (the TUI)**, which steps through the local and remote sides so you can keep what you want.~~ **Actual:** the TUI resolver opens only for `dossier merge` results (`internal/tui/tui.go:2075-2086`), and no command resolves a sync conflict. Manual procedure for now:
  1. Read the conflict file. Its body is your preserved version, followed by a diff against the shared version.
  2. Ask the Dossier's lead (or a bound Claude session) to fold whatever should survive into `dossier.md` through a normal save.
  3. Move the conflict file out of `conflicts/` into an archive folder you keep outside the store, rather than deleting it. `doctor` reports every file left in `conflicts/` as an unresolved issue (`internal/core/service.go:396-403`).
- Verify nothing is left unresolved:

  ```text
  dossier doctor
  ```

  `doctor` lists unresolved conflicts as issues. Ignore the "Unresolved conflicts: N" line in its Team Sync block; it counts only the last sync run (`internal/core/service.go:441`).
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

**Actual (2026-09-18):** per-dossier session stashes (`<slug>/sessions/`) do **not** sync (`internal/sync/gitignore.go:36`). "The archive" (`artifacts/`) **includes compiled session transcripts**, which contain tool results, file contents and command output. It also includes, when a Dossier was promoted from a transcript, a **byte-preserved raw JSONL copy with the model's thinking** (`internal/core/service_promote.go:90-103`). `files/` syncs too.

**What to do:** nothing — this is correct behavior. If a teammate's `config.yaml` looks different from yours, that's expected and right.

*Sources: `docs/adr/0005-team-sync-via-github.md` §5 + Consequences; `docs/team-sync-plan.md` Non-negotiables (Machine-local stays local) + Repo/store shape; `BUILD-DECISIONS.md` B12; `SPEC.md` §3.2.*

## Healthy-state checks

- `dossier sync --status` — read-only: unpushed commits, diverged remote, or stale credentials.
- `dossier doctor` — store integrity, unresolved conflicts, provenance references, and harness/capability status.
- TUI footer (later surfacing phase) — a glanceable status line, e.g. `synced 2m ago · 1 conflict`. **Not built** (no sync reference in `internal/tui/`).

*Sources: `SPEC.md` §7.2; `docs/team-sync-plan.md` Phase 3 §4 (Surfacing).*

## 6. `team join` failed and a retry says "target directory is not empty"

**What happened:** `team join` saves `team.remote` into `config.yaml` *before* cloning (`internal/cli/cli.go:1621`). A failed clone (wrong link, no access, no network, or a remote whose default branch is not `main`) leaves a partial `.git/` folder. The next join sees it and refuses (`internal/sync/sync.go:35-48`). Verified by dogfood, 2026-09-18.

**What to do (operator, on the colleague's machine):**

1. Move the store folder aside rather than deleting it, e.g. rename `~/.dossier` to `~/.dossier.failed-join-<date>`.
2. Fix the cause: the link, credentials (§2), or the repo's default branch.
3. Run `dossier team join <url>` again.

Do **not** run `dossier sync` in a half-joined store. In the dogfood run it synced on the wrong local branch and printed "Pushed local changes" while nothing reached the remote.

## 7. `team create` published more than intended

**What happened:** `team create` turns the *entire current store* into the team repo and pushes all of it (`internal/sync/sync.go:58-99`). It does not check that the remote is empty. If the remote already had content, the two are merged and both sides are pushed.

**What to do:** treat anything pushed as disclosed to everyone with repo access, because git history is kept on every clone. Removing it needs a history rewrite on GitHub plus a re-clone on every machine. That is outside Dossier; escalate. To prevent it, always create a team store from a fresh `DOSSIER_HOME`.

## Sources

- `docs/team-sync-plan.md` — the 4-phase plan and Non-negotiables.
- `docs/adr/0005-team-sync-via-github.md` — decision and conflict model (§4: "one mechanism, two triggers").
- `BUILD-DECISIONS.md` B12 — local-first / conflict-honest rules; machine-local files that never sync.
- `SPEC.md` §7 — the command surface (`dossier sync`, `dossier team create|join`).
- `assets/guide.md` — multi-author distillation etiquette.

