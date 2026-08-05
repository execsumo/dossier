# Team Sync — Operator Runbook

> Audience: the person who set up the team store. This is a failure-drill reference.
> Each entry: **symptom → what happened → what to do**.

> **Status (Pilot):** The team sync commands are built and work locally, but the shared GitHub flow is being piloted and is not yet validated against live GitHub. Treat this as an experimental feature.

**Setup recap (you, once):** you created the team store with `dossier team create <url>`, which initializes and pushes the existing store to an empty private repo and writes `team.remote` to config. Each colleague then joins with `dossier team join <url>`. Sync transport is fully hidden inside the binary; this runbook may reference the mechanism (commits, push/pull, the remote, the working tree) but you never run raw version-control commands yourself.

## Quick reference

| Symptom | What happened | Immediate action |
|---|---|---|
| "can't reach the team store" / sync deferred | Remote unreachable (offline or bad URL) | Local commit still landed; retry `dossier sync`; inspect with `dossier sync --status` |
| `sync_auth_failed` warning | PAT missing, expired, or lacks the right access | Run the re-auth command shown in the warning; ensure `~/.dossier/credentials` is `0600` |
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

  It reports unpushed commits, a diverged remote, or stale credentials.

- The next successful sync catches everything up. A deferred push is a visible warning, never a blocked save.

*Sources: `docs/adr/0005-team-sync-via-github.md` §5; `docs/team-sync-plan.md` Non-negotiables (Local-first); `SPEC.md` §7.2 `dossier sync`.*

## 2. PAT missing / expired / wrong mode

**Symptom:** a `sync_auth_failed` warning (the token is absent, expired, or invalid).

**What happened:** Dossier authenticates to the team remote over HTTPS with a fine-grained personal access token (contents: **read and write** on the team repo), stored at `~/.dossier/credentials`. If that file is missing, the token has expired, or the token lacks the required access, the sync cannot authenticate.

**What to do:**

- Re-run the **re-auth command printed in the `sync_auth_failed` warning.** It re-prompts for a fine-grained PAT (contents read/write on the team repo) and stores it at `~/.dossier/credentials`.
- The credentials file **must be `0600`** (owner read/write only). Dossier sets this when it writes the file; if you replaced the file manually, restore `0600`.
- Convenience: if the `gh` CLI is installed and signed in, Dossier can offer its token instead of prompting for a PAT.

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

**Symptom:** a new `conflicts/<id>.md` appears with `kind: sync_concurrent_edit`, plus a sync warning (and the TUI footer shows a conflict count).

**What happened:** two machines edited the same `dossier.md` — the only genuinely multi-writer file. On pull, Dossier first attempts the non-overlapping-frontmatter auto-merge; if the body truly conflicts, it does **not** merge the body with markers. Instead:

- the **remote (already-shared) version wins the working tree**, and
- the **local version is preserved** as a `conflicts/<id>.md` note (`kind: sync_concurrent_edit`).

Nothing is lost; there are **never merge markers** in the store.

**What to do:**

- Reconcile in Dossier's **conflict-resolution view (the TUI)**, which steps through the local and remote sides so you can keep what you want.
- Verify nothing is left unresolved:

  ```text
  dossier doctor
  ```

  `doctor` lists unresolved conflicts and suggested fixes.
- This is the same conflict flow used for local concurrent edits — **one mechanism, two triggers.**

*Sources: `docs/adr/0005-team-sync-via-github.md` §4; `docs/team-sync-plan.md` Phase 2 §3; `BUILD-DECISIONS.md` §5 (conflict artifact format, `doctor` reports unresolved conflicts); `SPEC.md` §7.2.*

## 5. Machine-local files that never sync

**Symptom (informational):** certain files are intentionally absent from the shared store; `dossier doctor` may report machine-local files as healthy / local-only.

**What happened:** Dossier auto-generates an ignore list that keeps machine-specific files out of the shared store. These never sync, by design:

- `config.yaml` — your machine's install settings, detected capabilities, token target.
- root `sessions/` — your machine's session-to-dossier bindings.
- `context/` — Dossier's locally generated context (library, guide).
- `.lock` files — local coordination locks.

**Why:** these are per-machine. Syncing them would clobber another machine's setup. Only team-relevant content syncs: distilled notes (`<slug>/dossier.md`), the archive, per-author audit shards, per-dossier session stashes, and revision history.

**What to do:** nothing — this is correct behavior. If a teammate's `config.yaml` looks different from yours, that's expected and right.

*Sources: `docs/adr/0005-team-sync-via-github.md` §5 + Consequences; `docs/team-sync-plan.md` Non-negotiables (Machine-local stays local) + Repo/store shape; `BUILD-DECISIONS.md` B12; `SPEC.md` §3.2.*

## Healthy-state checks

- `dossier sync --status` — read-only: unpushed commits, diverged remote, or stale credentials.
- `dossier doctor` — store integrity, unresolved conflicts, provenance references, and harness/capability status.
- TUI footer (later surfacing phase) — a glanceable status line, e.g. `synced 2m ago · 1 conflict`.

*Sources: `SPEC.md` §7.2; `docs/team-sync-plan.md` Phase 3 §4 (Surfacing).*

## Sources

- `docs/team-sync-plan.md` — the 4-phase plan and Non-negotiables.
- `docs/adr/0005-team-sync-via-github.md` — decision and conflict model (§4: "one mechanism, two triggers").
- `BUILD-DECISIONS.md` B12 — local-first / conflict-honest rules; machine-local files that never sync.
- `SPEC.md` §7 — the command surface (`dossier sync`, `dossier team create|join`).
- `assets/guide.md` — multi-author distillation etiquette.

