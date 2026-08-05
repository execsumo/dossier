# Team Sync — Development Plan

> Date: 2026-07-15 · Branch: `team-sync` · Decision: [ADR 0005](adr/0005-team-sync-via-github.md), `BUILD-DECISIONS.md` B12.
> Status tracking lives in `HANDOFF.md` ("Current initiative"). Update both as phases land.

## Goal

Multiple colleagues' Claude Code sessions contribute to one shared Dossier store. Session captures are stashed per-dossier so nothing is lost. Transport is a private GitHub repo **fully hidden behind the Dossier binary** (embedded go-git). Colleague experience: install binary → `dossier team join <url>` → sign in once → it just works.

## Non-negotiables (inherit all CLAUDE.md hard rules, plus)

- **Local-first:** every save commits locally whether or not the network is up. Push/pull failure = visible warning + retry, never a blocked save.
- **No last-write-wins across machines:** concurrent `dossier.md` edits become `conflicts/*.md` (`kind: sync_concurrent_edit`), exactly like local concurrent edits. Reuse `core`'s non-overlapping-frontmatter auto-merge before declaring conflict.
- **Single-writer layout:** any file that can be per-author namespaced must be. Only `dossier.md` is allowed to be multi-writer.
- **Machine-local stays local:** `config.yaml`, root `sessions/` (bindings), `context/`, `.lock` files never sync.
- **Degrade visibly:** offline, auth-expired, oversized-artifact (>100 MB GitHub limit) are all surfaced warnings, never silent no-ops.
- **Core stays pure:** git lives behind a `Syncer` port in `internal/sync/`; `internal/core` never imports it.

## Repo/store shape

`DOSSIER_HOME` itself becomes the git working tree (one store, no parallel trees). A generated `.gitignore` excludes the machine-local set above. Slug folders, `history/`, `conflicts/`, per-author `audit/` and `sessions/` all sync. `history/` files are revision-content-addressed (`rev_*.md`) so identical writes are idempotent and conflict-free.

---

## Phase 1 — Identity & store layout v2 *(no networking yet)*

The prerequisite everything else stands on. Pure store/core work, fully testable offline.

1. **`author` config field** (`internal/config/config.go`): default `os/user.Current()` username; `dossier init` keeps working with zero prompts (default applies silently); `dossier team join` (Phase 3) will confirm it interactively.
2. **Stamp author into audit events**: add `Author string` to `core.AuditEvent` (omitempty for backward compat), populated in `Service` from config.
3. **Per-author audit shards**: writes go to `<slug>/audit/<author>.log` (`FSStore.AppendAudit`, `internal/store/fsstore.go:487`); `ReadAuditLog` merges legacy `<slug>/audit.log` + all shards, ordered by `ts` (the ordering contract already exists in `ReadAuditEntries`). Legacy file is never rewritten or renamed — non-destructive.
4. **Per-dossier session stash**: on `session-end`/`pre-compaction` for a *bound* session, snapshot the transcript to `<slug>/sessions/<author>/<session-id>.md` (append-mode per session id; one writer per file by construction). Verify transcript availability against `docs/harness-capabilities.md` first; if a session has no accessible transcript, warn — don't fake it. Root `sessions/*.json` bindings are untouched and stay machine-local.
5. **Schema/migration**: bump `core.CurrentSchemaVersion`; extend `Service.Migrate` (additive only — create `audit/` dirs; no rewrites). `doctor` learns to validate shard names and the merged-ordering invariant.
6. **Docs in the same PR**: SPEC §3.2 layout update, ARCHITECTURE §2/§5 update.

**Tests:** table tests for merged audit ordering across legacy+shards; temp-dir integration for the stash path; migration idempotence (run twice = no-op); doctor fixtures with a foreign-author shard.

**Acceptance:** two different `author` configs writing to one store (simulating two machines post-sync) produce zero overlapping file writes except `dossier.md`.

## Phase 2 — `Syncer` port + go-git adapter + manual `dossier sync`

1. **Port** (`internal/core/ports.go`): `Syncer` with `Clone(url)`, `Sync() (SyncReport, error)` (pull → resolve → commit → push), `Status() (SyncStatus, error)`. `SyncReport` carries pulled/pushed/conflict counts into the standard `Result` envelope so CLI/MCP/TUI render it identically.
2. **Adapter** (`internal/sync/gitsync.go`): go-git over HTTPS+PAT. Store-wide sync lock (`~/.dossier/.sync.lock`) so a sync and a save serialize; per-dossier locks stay as-is. Synthesized commits: author from config, message = summary of changed slugs.
3. **Conflict resolution on pull** (the heart): for a `dossier.md` both sides changed — attempt `core`'s frontmatter auto-merge; body conflict ⇒ remote wins working tree, local version written via existing `WriteConflict` with `kind: sync_concurrent_edit`. Never a git merge-marker file in the store.
4. **Auth**: PAT prompt on `team join`, stored `0600` at `~/.dossier/credentials` (gitignored, outside repo scope); auto-detect `gh auth token` as a convenience path. Expired/invalid token ⇒ `sync_auth_failed` warning with the exact re-auth command.
5. **Oversized artifacts**: >100 MB excluded from commits with a persistent warning listing them (GitHub hard limit; cap remains 1 GB locally per SPEC).
6. **CLI**: `dossier sync` + `dossier sync --status`; `--json` like everything else.

**Tests:** gitsync against local bare repos (no network needed) — two working trees simulating two machines: clean fast-forward, frontmatter auto-merge, body conflict → conflict artifact + remote-wins, offline push queue, oversized artifact exclusion. MCP envelope test for the new warnings.

**Acceptance:** two temp stores syncing through one bare repo converge; a simultaneous body edit yields exactly one `conflicts/*.md` on the later syncer and no lost content anywhere (SPEC §14 non-destructive criteria extended).

## Phase 3 — Team onboarding + auto-sync (the "no babysitting" phase)

1. **`dossier team create <url>`** (owner, once): initializes/pushes the existing store to an empty private repo the owner creates on GitHub; validates it's empty; writes `team.remote` to config.
2. **`dossier team join <url>`** (each colleague): clone into `DOSSIER_HOME` (refusing to clobber a non-empty unsynced store — merge-adopt flow with confirmation, per B7 spirit), confirm `author`, store PAT, run capability detect + hook install (existing `init` path).
3. **Auto-sync**: pull-before-`Recall`/`SessionStart`, commit+push-after-`Save`/`session-end` — debounced/async so no operation ever waits on the network; failures degrade to warnings and the next sync retries. A background-less design (sync piggybacks on operations) — **no daemon to babysit**.
4. **Surfacing**: TUI footer shows sync status (`synced 2m ago · 1 conflict`); `doctor` reports unpushed commits, diverged remote, stale credentials; `dossier_list` MCP payload gains a sync warning slot.

**Tests:** join-into-existing-store fixtures; debounce/async save path (save returns before push completes); TUI status rendering; doctor fixtures for diverged/unpushed/stale-auth.

**Acceptance:** a fresh machine goes from zero → contributing sessions with exactly two commands and one sign-in; a week of two-author dogfood produces zero manual git interventions.

## Phase 4 — Docs, dogfood, hardening

README team-setup section (written for the least technical teammate — screenshots of the PAT flow), PRD/PRFAQ premise amendment, SPEC acceptance criteria additions (§14), guide.md note on multi-author distillation etiquette (attribute contested claims via provenance), failure drills (revoked token mid-push, force-pushed remote, clock skew across machines), then a real two-colleague pilot on one live dossier.

---

## Explicitly not building (guardrails)

- No CRDTs, no operational transforms — revision hashes + conflict artifacts are the merge model.
- No hosted service, no daemon, no cron.
- No per-dossier ACLs/permissions — repo access *is* the permission model in v1.
- No git CLI dependency, no shelling out — go-git only.
- No sync of bindings, credentials, or generated context.
- Shared-drive mailbox (Option C) stays unbuilt unless GitHub auth defeats a teammate.

## Sequencing note

Phases are strictly ordered; each is one focused PR off `team-sync` (which serves as the integration branch), merged to `main` only when the whole feature dogfoods clean. Phase 1 has no networking and no new dependencies — it is safely delegatable and cannot destabilize v1 behavior (all changes additive behind the existing schema-migration gate).
