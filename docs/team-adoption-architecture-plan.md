# Team Adoption Architecture Plan

> Status: Proposed product and architecture recommendation; not yet an implementation contract.
>
> Date: 2026-09-18
>
> Audience: Dossier owner and future implementation agents.
>
> **Review status (2026-09-18):** Proposed and **non-canonical**. Reviewed against
> the code and a real-binary dogfood run in
> [`team-adoption-plan-review.md`](team-adoption-plan-review.md). The direction
> holds. The review found release-blocking Team Sync defects that this plan does
> not mention; its P0 list replaces "Phase 0" below. Inline `> Review:` notes mark
> where the plan is wrong or understated. The recommendations are left as
> written. Product decisions stay open; see the review's decision memo (§4).

## Executive decision

Dossier is not yet ready for a mandatory non-developer team rollout. It is a strong local-first memory system and a promising collaboration substrate, but Team Sync currently solves transport more than collaboration.

The recommended near-term product is:

```text
manager assignment
  -> one-click Dossier launcher
  -> correctly bound Claude Code session
  -> automatic local capture
  -> shared curated contribution
  -> visible sync and recovery health
```

Use Claude Code as the team-standard harness for the pilot. Keep Pi supported, but do not make teammates choose between Claude Code and Pi during onboarding.

Use GitHub Team Sync only for a controlled pilot containing non-sensitive work. Do not require teammates to manage PATs, sync commands, Dossier slugs, `dossier_save`, `doctor`, or conflict files manually.

Katana should be considered later, when it provides identity, SSO, ACLs, notification, or central operational capabilities—not merely somewhere to host the same Git-backed store.

## First principles

The product succeeds only if it reliably provides shared context between the manager and teammate, and between teammates where appropriate.

That implies these invariants:

1. **Correct attachment:** the right work context reaches the right agent session without the teammate remembering a slug or choosing from an unscoped list.
2. **Useful persistence:** material decisions, constraints, evidence, and next actions survive the session.
3. **Visible delivery:** users can tell whether their work is local-only, shared, stale, conflicted, or recovered.
4. **Safe disclosure:** shared context is visible to exactly the intended audience; repository access must not be mistaken for Dossier-level privacy.
5. **Recoverability:** a forgotten save, failed sync, interrupted session, or conflict does not turn into apparent data loss.
6. **Low interaction cost:** automation should remove decisions and repeated actions, not merely hide them.
7. **One truth per concern:** the Dossier owns work truth; assignments own relationship terms; contributions own author/session updates; runtime state owns machine-local health.

Dossier should be positioned as a **local-first work launcher and shared memory layer**, not as another project-management system.

## Current build assessment

### Strong foundations to keep

- The Dossier body is the canonical operational brief: Objective, Done When, Validation, Constraints, and shared context.
- Delegation contracts retain relationship terms rather than duplicating the work definition.
- Local-first persistence, non-destructive history, provenance, and conflict artifacts are the right trust model.
- Per-session binding and Claude Code's `--session-id` launch handoff provide a strong deterministic attachment mechanism.
- The sync transport is abstracted behind a port, so GitHub is replaceable later.
- The distillation guide appropriately prioritizes operational signal and resolvable evidence over terse summaries.

### Rollout blockers

#### Onboarding is more technical than the documentation claims

`docs/team-sync-onboarding.md` promises name prompting, sign-in guidance, and a simple join flow. The current `dossier team join` path and `internal/sync/credentials.go` rely on an existing credentials file or `gh auth token`; there is no complete first-run identity, credential-writing, or re-authentication experience. Join also needs to be transactional so a failed clone does not leave a half-configured store.

> Review: This understates the problem. A failed join cannot be retried: `team.remote` is saved before the clone (`internal/cli/cli.go:1621`), and a failed `PlainClone` leaves a partial `.git/`, so the retry is refused as "target directory is not empty" (dogfood, 2026-09-18). With no credentials file and no `gh`, `GetAuth` returns `nil, nil` (`internal/sync/credentials.go:59`), so a missing sign-in is silent. `team create` has the same save-config-first shape (`cli.go:1577`).

#### Automatic sync is not sufficiently observable

MCP sync triggers are not uniformly attached to every mutating path. `dossier_list` does not itself establish freshness. Session-start and session-end sync failures are intentionally discarded and deferred to `doctor`. The TUI does not currently provide the documented Team Sync status surface. A non-developer will not reliably discover a sync failure by running a diagnostic command.

> Review: The health data is also wrong, not just hidden. `.syncstate.json` records `LastSync` on every attempt, including failures (`internal/sync/sync.go:170`). It also replaces the conflict list on each run, so `doctor` can print "Unresolved conflicts: 0" while listing an unresolved conflict. Manual `dossier sync` prints "Sync successful" and exits 0 when the remote is unreachable (`cli.go:1541`), and it reports "Pushed" for a no-op push (`internal/sync/status.go:56`). All verified by dogfood. Sync triggers: the SessionStart and SessionEnd hooks, plus MCP `dossier_recall`/`dossier_save`/`dossier_rename` only (`internal/mcp/tools.go:324,419,611`). No CLI or TUI write triggers a sync.

#### `lead` is not an assignment

The current lead is a free-form label, not a stable identity, inbox, acceptance state, notification, scoped work package, or permission boundary. A colleague still has to discover and bind the correct Dossier.

#### Session capture still depends on agent discipline

Lifecycle hooks can archive a transcript, but cannot ask the model to distill final state. Eager in-session `dossier_save` calls remain load-bearing. A teammate who forgets to save needs a first-class recovery path rather than a warning that appears only later.

#### Privacy is repository-wide

A private GitHub repository gives all repository members access to all synced Dossiers. A `visibility: private` field inside that repository is not a security boundary. The shared store must therefore be limited to team-safe work until separate stores or an identity-backed service exist.

Compiled transcript artifacts can contain prompts, tool results, file content, and environment output. Raw and compiled transcripts should be local-only by default; shared evidence should be explicit and sanitized.

> Review: Disclosure starts before sync. `team create` initializes the *whole existing store* as the team repo (`internal/sync/sync.go:58-99`). It has no empty-remote check: pointed at a non-empty remote, it merged and pushed an unrelated personal Dossier (dogfood). Raw JSONL also already syncs: `promote` with transcript content archives a byte-preserved raw artifact, thinking included, under `artifacts/` (`internal/core/service_promote.go:90-103`). B13 only excludes the session stash. Compiled transcripts sync today.

#### Conflict handling is still developer-oriented

Remote-wins conflict preservation is technically honest, but a teammate should not need to inspect `conflicts/*.md`, understand revisions, or run the TUI to recover a disagreement. Conflicts need plain-language choices and an option to ask Claude to reconcile them.

> Review: There is currently *no* resolution operation for a sync conflict on any surface. `core.Service` has none, and the TUI resolver opens only from a `dossier merge` result (`internal/tui/tui.go:2075-2086`). `doctor` reports every `conflicts/*.md` as an issue indefinitely (`internal/core/service.go:396-403`).

#### The workspace launched by Claude may be wrong

Launching in `~/.dossier/<slug>/` makes the Dossier file easy to find but may prevent Claude from naturally accessing the actual project or business workspace needed for the assignment. A logical shared workspace reference should map to a machine-local path.

> Review: Confirmed. Every launch profile sets `cmd.Dir` to the Dossier directory (`internal/harness/launch.go:199,240`). `dossier open` also does not pull before launching; freshness depends on the SessionStart hook's 5 s pull, whose result is discarded (`internal/core/service_session.go:215-219`).

## Target collaboration model

### 1. Stable identity

Use a stable team identity based on authenticated account information, not an OS username or free-form local configuration.

The team manifest should provide:

- stable person ID;
- display name;
- permitted team stores or audiences;
- optional role information;
- local identity mapping.

Identity is needed for attribution and assignment. It is not sufficient for privacy until the transport enforces access.

### 2. First-class assignments

Add a lightweight Assignment object without duplicating the Dossier's work definition.

An assignment contains:

- assignment ID;
- Dossier ID;
- assignee stable identity;
- scoped deliverable, if narrower than the Dossier;
- decision rights;
- escalation route;
- return expectations;
- accepted Dossier revision;
- state: proposed, accepted, blocked, submitted, or closed.

The Dossier remains the canonical work truth. Assignment records describe who is acting on it and what comes back.

Provide:

```text
dossier assign <dossier> --to <person>
dossier inbox
dossier work <assignment-link-or-id>
```

A colleague should be able to work on an existing Dossier without creating a new one.

### 3. Deterministic launcher

`dossier work` should:

1. resolve the assignment;
2. authenticate or explain the required action;
3. pull with a short bounded timeout;
4. display freshness and health;
5. resolve a machine-local workspace mapping;
6. mint a session ID;
7. bind the Dossier and assignment;
8. launch Claude Code;
9. inject only the assigned context before the first useful turn.

The manager should be able to send a copyable assignment link or token through the team's normal channel. The teammate should not need to remember a slug or browse every open Dossier.

A direct Claude launch should remain possible, but it should be visibly marked as unattached and offer a one-step route to `dossier work`. Do not make the system brittle by blocking all offline or unbound work.

### 4. Logical workspace mapping

The shared Dossier stores a logical reference such as:

```yaml
workspace_ref: pricing-operations
```

Each machine maps that reference to a local path:

```yaml
workspaces:
  pricing-operations: ~/Company/Pricing
```

Local filesystem paths never sync. Claude starts in the mapped work area while the Dossier remains available through its bound session and MCP/CLI surfaces.

### 5. Contributions instead of uncontrolled multi-writer state

Keep `dossier.md` as the canonical curated state, but reduce direct multi-writer pressure with append-only contributions:

```text
contributions/<author>/<contribution-id>.md
```

A contribution records proposed decisions, observations, evidence, open questions, and next actions with provenance. A lead, curator, or reconciliation session can integrate it into the canonical state.

This preserves every teammate's contribution without requiring every session to rewrite the same body.

### 6. Recovery queue

When a session ends with a transcript but no distilled update, record an unprocessed item:

```text
This assignment has one unprocessed session.
Nothing was lost. Recover it now?
```

The next bound Claude session should be able to recover the contribution while preserving provenance, checking the current revision, and never reverting newer curated state.

### 7. Safe shared stores

Near-term stores should be explicit:

```text
personal store
team store
restricted store, if needed
```

New private work should default to the personal store. Manager-assigned work should explicitly target a team store. Do not represent privacy with a field inside a repository that everyone can clone.

> Review: Separate stores are not available today. Claude Code's MCP server is registered as `dossier mcp serve` with no `--home` (`internal/harness/claudecode.go:339`), and hooks resolve `DOSSIER_HOME` or `~/.dossier`, so one machine integrates one store. `team join` also refuses any store with content (`internal/sync/sync.go:35-40`). This is a Phase 1-sized design question, not a Phase 4 add-on. For the pilot, use a fresh dedicated team store as the colleague's only store.

Shared content should default to:

- curated Dossier state;
- assignments;
- append-only contributions;
- explicit, sanitized evidence artifacts;
- relevant audit information.

Raw and compiled transcripts should remain local-only unless explicitly promoted.

### 8. Health and sync status

Every surface should expose a compact durable state such as:

```text
Dossier team · synced 3m ago · 1 local change · 0 conflicts
```

or:

```text
Dossier team · last sync failed 18m ago · work remains safely local
```

Persist last successful pull, last successful push, last error, pending changes, conflicts, and authentication state. A failed background operation must be visible in the next attached session without requiring `doctor`.

### 9. Plain-language conflict resolution

A conflict should explain:

- who changed the Dossier;
- what changed;
- which version is currently shared;
- what is preserved locally.

Offer:

- keep shared version;
- keep my update;
- keep both as a disagreement;
- ask Claude to reconcile.

No teammate should need to understand Git, merge bases, or conflict filenames.

## Recommended delivery sequence

### Phase 0 — Make the current product honest

> Review: The review's P0 list (§6, P0-1…P0-8) replaces this phase. It adds items missing here: a `team create` empty-remote and existing-content guard, honest sync state, a conflict-resolution operation, typed auth failures, and excluding promote's raw JSONL from sync.

- Reconcile stale onboarding, runbook, PRD, and Team Sync claims with actual behavior.
- Complete real private-GitHub testing, including first clone, fast-forward, auth failure, revocation, divergence, offline use, and upgrades.
- Make join transactional.
- Add durable sync health to session start, CLI, MCP, and TUI.
- Make the actual shared-data boundary explicit.

### Phase 1 — Assignment and launcher

- Stable team identity.
- `assign`, `inbox`, and `work` surfaces.
- Assignment acceptance and state.
- One-step Claude Code launch with deterministic binding.
- Logical-to-local workspace mapping.
- Assigned-context injection.

### Phase 2 — Automatic contribution and recovery

- Append-only per-author contributions.
- Unprocessed-session queue.
- One-command recovery.
- Assignment submission and manager-visible status.
- Reconciliation into canonical Dossier state.

### Phase 3 — Safe shared evidence

- Curated state and explicit artifact promotion.
- Local-only transcripts by default.
- Sensitive-content warnings and redaction policy.
- Store-level disclosure policy.

### Phase 4 — Privacy and scale

- Personal/team/restricted stores.
- Identity-backed ACLs.
- Audience-specific projections.
- Optional hosted service or Katana adapter.

Do not implement the full historical `PLANv02.md` project-management surface before the assignment and launcher path works. Requirements, meeting-prep, timezone, and roster features may become useful later, but they do not solve the first adoption bottleneck.

## Pilot recommendation

Run a concierge-supported pilot with:

- one manager;
- one or two colleagues;
- three to five non-sensitive Dossiers;
- Claude Code only;
- real private GitHub, not only local bare repositories;
- no requirement for colleagues to use the TUI;
- no requirement for colleagues to create Dossiers.

Measure:

- time from assignment to useful first Claude turn;
- manual commands per assignment;
- direct Claude launches that bypass Dossier;
- percentage of sessions with a durable contribution;
- sync freshness and failure visibility;
- clarification cycles caused by missing context;
- duplicate Dossier creation;
- recovery success after an unsaved session;
- conflict backlog;
- privacy incidents or near misses.

## Adoption gates

Do not mandate broad usage until:

1. An assignment link opens a correctly bound Claude session without slug discovery.
2. No teammate must edit YAML, Git configuration, MCP configuration, or credential files.
3. Authentication is browser/device-based or already managed by company SSO.
4. Offline work continues safely and stale state is visible.
5. Failed sync is visible without running `doctor`.
6. A colleague can complete assigned work without creating a Dossier.
7. An unsaved session is recoverable on the next launch.
8. Shared transcripts and artifacts cannot expose unintended local data by default.
9. Conflicts are resolvable in plain language.
10. The manager can see assignment, acceptance, progress, blockage, submission, and reconciliation state.

## Harness recommendation

Use **Claude Code + the Dossier launcher** as the team standard for the first pilot. Claude Code currently provides the most direct path to deterministic session identity, lifecycle hooks, MCP, and transcript handling.

Keep the Pi extension supported for users who prefer Pi.

> Review: Pi is weaker than "parity minus MCP". `dossier open` does not pre-bind a Pi session (`internal/cli/cli.go:1418-1430`), yet its launch prompt still points the agent at MCP `dossier_session`. More importantly, no CLI command updates an existing Dossier's Distilled State; only `promote` writes the body (`cli.go:733`), and `dossier.md` is written `0444` (`internal/store/fsstore.go:390`). Not yet confirmed in a live Pi session. Revisit making Pi a team standard only after a live end-to-end test matrix demonstrates equivalent assignment launch, context injection, capture, recovery, and upgrade behavior.

## Katana decision rule

Use Katana when it materially reduces adoption or risk by providing:

- SSO and stable identity;
- per-Dossier or audience ACLs;
- managed notifications and assignment delivery;
- centralized sync health;
- audit and retention controls;
- a service API that can replace Git transport without changing the Dossier model.

Do not move to Katana merely to host the current Git-backed store. Approval and hosting friction would add cost without fixing assignment, launcher, capture, or privacy design by itself.

## Related documentation

This document is a product and architecture recommendation. It should not silently override the repository's canonical mechanics.

- `BUILD-DECISIONS.md` — settled structural choices.
- `ARCHITECTURE.md` — implementation boundaries and ports.
- `SPEC.md` — current mechanics and acceptance criteria.
- `docs/adr/0005-team-sync-via-github.md` — GitHub transport decision.
- `docs/adr/0008-dossier-as-canonical-operational-brief.md` — canonical work-definition decision.
- `docs/harness-capabilities.md` — verified harness capabilities.
- `docs/team-sync-plan.md` — current Team Sync implementation plan.
- `docs/team-sync-onboarding.md` and `docs/team-sync-runbook.md` — teammate/operator material requiring reconciliation before rollout.

When implementation begins, convert the accepted portions of this document into amendments to `SPEC.md`, `BUILD-DECISIONS.md`, and new ADRs rather than treating this proposal itself as the executable contract.
