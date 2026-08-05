# ADR 0005 — Team Sync via an embedded git remote (GitHub)

> Date: 2026-07-15 · Status: **accepted** (plan phase; supersedes the "single-user" premise in PRD §0 / CLAUDE.md)
> Companion: `docs/team-sync-plan.md` (the phased development plan), `BUILD-DECISIONS.md` B12.

## Context

Dossier v1 is local and single-user. The user (product owner) wants **portability across people**: multiple colleagues' sessions contribute to one shared Distilled State, and session captures are stashed inside the dossier's slug folder so no information is lost. Constraints, verbatim from the owner: low complexity, no babysitting, "it just works," colleagues have mixed technical ability. Everyone on the team has GitHub access; everyone also has a corporate shared drive.

Options considered:

- **A — Shared drive as the live store** (`DOSSIER_HOME` on OneDrive/SharePoint). Rejected as the primary mechanism: drive sync clients resolve concurrent writes with last-write-wins or silent "conflicted copy" files, bypassing Dossier's revision checks entirely. That violates two hard rules (*no last-write-wins for Distilled State*, *non-destructive always*). Advisory `flock` does not hold across machines on these mounts; files-on-demand placeholders break scans.
- **B — GitHub as the sync layer, fully hidden inside Dossier.** Accepted; see Decision.
- **C — Shared drive as a dumb per-author mailbox** with local merge. Viable fallback (single-writer files can't conflict) but is more code than B — a hand-rolled poor man's git — with worse latency. Not built unless B's auth proves impossible for some teammate.
- **D — Hosted sync service.** Rejected: most complexity, needs operating, contradicts local-first files-are-truth.

## Decision

1. **A private GitHub repo is the team store's transport.** The store is already all Markdown/JSONL, append-only, non-destructive — practically pre-formatted for git. Git supplies transport, auth, atomic updates, full history, and refuses silent overwrites: exactly the guarantees Dossier already promises locally.
2. **Colleagues never see git.** Dossier embeds **go-git** (pure Go — preserves the single-binary promise, no "install git first" step). Clone/pull/commit/push happen inside `dossier sync` and automatically around normal operations. Onboarding is `dossier team join <repo-url>` + one GitHub sign-in.
3. **Single-writer file layout everywhere possible.** Session captures land under `<slug>/sessions/<author>/…`; audit appends move to per-author shards `<slug>/audit/<author>.log` (legacy `audit.log` remains readable, never rewritten). Files with exactly one writer cannot conflict under any sync mechanism. `history/` files are revision-content-addressed and idempotent. That leaves **`dossier.md` as the only genuinely multi-writer file**.
4. **Sync conflicts route into the existing conflict machinery.** A pull that finds concurrent edits to `dossier.md` does **not** git-merge the body: remote (already-pushed) content wins the working copy, and the local version becomes `conflicts/<conf_id>.md` (`kind: sync_concurrent_edit`), surfaced via warnings/doctor/TUI — identical UX to a local concurrent edit. The existing non-overlapping-frontmatter auto-merge is reused before declaring conflict. One mechanism, two triggers.
5. **Local-first, never blocked by network.** Every save commits locally regardless of connectivity; push/pull failures are visible warnings with retry, never errors that block work. Machine-local files (`config.yaml`, root `sessions/` bindings, `context/`, locks) are excluded from the repo.
6. **Author identity is a config field**, `author:` in `config.yaml`, defaulting to the OS username, prompted once at `team join`. It stamps audit events, session-stash paths, and synthesized git commits. No accounts, no server-side identity.

## Consequences

- The "local, single-user" premise in PRD §0 / CLAUDE.md is amended to **"local-first; optionally team-synced."** Files remain truth; the git repo is transport + history, not a database — the no-database rule stands.
- New port `core.Syncer` + adapter `internal/sync/gitsync.go`; core stays pure.
- Store layout changes (per-author audit shards, per-dossier session stash) require a `schema_version` bump and a `Migrate` extension; all migrations are additive/non-destructive.
- GitHub rejects files >100 MB (warns >50 MB); Dossier's artifact cap is 1 GB. Oversized artifacts stay local and are excluded from sync **with a visible warning** (degrade visibly, never silently).
- Auth: fine-grained PAT (contents read/write on the team repo) entered once, stored `0600` outside the repo; if the `gh` CLI is present its token is offered as a convenience. OAuth device flow is a possible later upgrade.
