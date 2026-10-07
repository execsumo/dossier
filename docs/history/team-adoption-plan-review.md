# Review: Team Adoption Architecture Plan

> Date: 2026-09-18 · Reviewer: Claude (Opus 5), read/write review per
> `docs/team-adoption-plan-review-agent-prompt.md` · Branch `review/team-adoption-plan`.
>
> Subject: `docs/team-adoption-architecture-plan.md` (proposed, non-canonical).
>
> Evidence rules used here: a claim is **verified in code** only when I read the
> source path, **verified by dogfood** when I ran the real binary (built from
> `28415bb`) against throwaway `DOSSIER_HOME`s, a fake `HOME`, and a local bare
> repo, and **verified only by tests** when a test asserts it but I did not run
> the path. No live GitHub remote, real `~/.dossier`, or real harness config was
> touched. Labels: **[fact]** = observed; **[rec]** = my recommendation;
> **[decision]** = belongs to the product manager.

## 1. Executive verdict

The plan's direction is right and it is, if anything, **too generous to the
current build**. Its core diagnosis — "Team Sync solves transport more than
collaboration" — holds, but the dogfood pass found that the transport itself has
defects that must be fixed before any colleague touches it. Four are release
blockers independent of every product decision in the plan:

1. **`team create` publishes the whole personal store and will merge into a
   non-empty remote.** SPEC §7 says create "validates the target repo is empty";
   the code has no such check. Pointed at a non-empty remote it reported
   success, merged that remote's Dossiers into the local store, and pushed an
   unrelated personal Dossier to the team repo (dogfood, §2 step 1). This is a
   disclosure defect, not a UX gap.
2. **A failed `team join` is unrecoverable without developer knowledge.** It
   writes `team.remote` to `config.yaml` *before* cloning, and a failed clone
   leaves a partial `.git/`; the retry is refused with "target directory is not
   empty" (dogfood). The colleague must delete a hidden directory to recover.
3. **Sync lies about success.** `dossier sync` prints "Sync successful" and exits
   0 when the remote is unreachable; `Last Sync` advances on failed attempts; a
   push that sent nothing reports "Pushed local changes"; and the sync-status
   conflict count resets to 0 on the next sync while the conflict file remains
   (dogfood). Every health surface the plan wants to build would currently be
   fed wrong data.
4. **Sync conflicts have no resolution path.** No `core.Service` operation
   resolves or retires a `conflicts/*.md`; the TUI resolver only handles
   `dossier merge` conflicts; `doctor` reports every conflict file as an issue
   forever. The onboarding and runbook both promise a guided TUI reconciliation
   that does not exist.

Beyond those, the plan under-states two structural limits that change its
Phase 1–4 shape: **one machine can only integrate one store** (Claude's MCP and
hooks are registered without `--home`, and `team join` refuses a non-empty
store, so "personal + team stores" is not available today), and **a Pi session
has no CLI path to update an existing Dossier's Distilled State** (the only CLI
writer of the body is `promote`), so "keep Pi supported" is weaker than it reads.

Recommended shape: **Phase 0 becomes a hard P0 fix list (below), then a
concierge pilot of one manager + one colleague on Claude Code only, using
`dossier open` and existing `lead` as the manual assignment convention**, before
building Assignment, Contributions, or multi-store. Most of Phases 2–4 are
premature until the pilot shows where the manual steps actually hurt.

## 2. Adoption journey: verified versus assumed

Labels: **VC** verified in executable code · **VD** verified by dogfood (real
binary) · **VT** verified only by tests · **DOC** documented but not verified ·
**CONTRA** contradicted by code · **NI** not implemented.

| # | Step | Status | Evidence and failure points |
|---|---|---|---|
| 1 | Manager creates team store and assigns work | **VD, with CONTRA defects** | `team create <url>` → `GitSync.Create` (`internal/sync/sync.go:58`) `PlainInit`s the *entire* store and syncs it: every existing personal Dossier is published. No empty-remote check (SPEC §7 claims one) — against a non-empty remote it merged and pushed (dogfood). Config is saved before the network step (`internal/cli/cli.go:1577`), and a failed push leaves `.git/` so a retry says "already a team store" (`sync.go:64-67`). Branch is hardcoded `main` (`cli.go:1576`). Assignment = `lead`, a free-form string (`internal/core/dossier.go:146`) — see below. |
| 2 | Colleague joins from a clean machine | **VD, CONTRA** | `team join` saves `team.remote` first (`cli.go:1621`), then `PlainClone` (`sync.go:43`). Failed clone ⇒ partial `.git/` + config left behind; retry refused as "not empty" (dogfood). Clone also fails with "reference not found" when the remote's `HEAD` names a branch other than `main` (dogfood with a `git init --bare` remote whose HEAD is `master`); the half-cloned store then synced on local `master`, printed "Pushed local changes", and stayed `Ahead: 2` (nothing reached the remote). A GitHub-created empty repo defaults HEAD to `main`, so this is likely latent on GitHub — **unverified live**. Clone has no timeout (`Clone` ignores ctx, `sync.go:21`). A colleague with *any* existing store at `~/.dossier` cannot join there (`sync.go:35-40`); SPEC's "merge-adopt flow with confirmation" is **NI**. |
| 3 | Authenticate without developer knowledge | **CONTRA / NI** | No prompt for name or PAT anywhere in `team join` (`cli.go:1606-1650`). Auth is resolved once at wire time (`cli.go:1837`) from `$HOME/.dossier/credentials` — which must already exist with mode exactly `0600` (`internal/sync/credentials.go:29-35`) — else `gh auth token` (`credentials.go:51`), else **no auth, silently** (`return nil, nil`). The credentials path ignores `DOSSIER_HOME`. `sync_auth_failed` and the "re-auth command" named by the runbook do not exist in the codebase (grep: zero hits). Author defaults to the OS username silently (`internal/config/config.go:62`). Real-GitHub PAT flow has **never been run** (HANDOFF "Remaining Phase 4"). `gh auth token` is the user's broad OAuth token, not a repo-scoped PAT — a scope decision nobody made explicitly. |
| 4 | Colleague receives assigned work | **NI** | No assignment object, inbox, or notification. `lead` is an unvalidated string whose allowed values come from each machine's own `config.yaml` (`leads:`; machine-local, never synced), unrelated to `author`. Discoverable only via `dossier ls -q <name>` / TUI Lead filter. |
| 5 | Colleague launches Claude through Dossier | **VC** | `dossier open <slug>` / TUI `c`: `harness.PlanOpenWith` (`internal/harness/launch.go:127`) mints a UUID, binds via `Service.Switch` *before* launch, and runs `claude --session-id`. Requires knowing the slug. **No pull before launch** — `open` recalls from local disk; freshness depends on the SessionStart hook's pull. |
| 6 | Session starts bound to the correct Dossier | **VC for Claude Code; CONTRA for Pi** | Claude: SessionStart hook sees the pre-bound id, does a best-effort 5 s pull (`internal/core/service_session.go:215-219`), then inlines the Guide + Distilled State (`:266-290`). `claude --session-id` is recorded as verified (`docs/harness-capabilities.md` §1). Pi: `open` deliberately does **not** pre-bind (`cli.go:1418-1430`); the launch prompt still tells the agent to call the MCP tool `dossier_session`, which Pi does not have. |
| 7 | Claude can access the actual workspace | **CONTRA (for non-Dossier work)** | Every launch profile sets `cmd.Dir` to the Dossier directory (`launch.go:199`, `:240`). There is no workspace mapping. Work in a project repo or business folder starts outside it. |
| 8 | Progress captured without perfect agent discipline | **CONTRA** | Only in-session `dossier_save` writes the Distilled State; the hook cannot distill (`service_session.go:462-490`, `docs/harness-capabilities.md` §"Hook payloads do not carry distilled state"). SessionEnd archives a compiled transcript and warns. That warning is printed to the stdout of a hook process that runs as Claude exits (`cli.go:1344`) — **visibility to the user is unverified and probably nil** in Claude Code. Under Pi there is **no CLI command that updates an existing Dossier's body** (only `promote` writes `DistilledStateMarkdown`, `cli.go:733`), and `dossier.md` is written `0444` (`internal/store/fsstore.go:390`). Unprocessed-session recovery is a proposal only (HANDOFF roadmap). |
| 9 | Colleague submits or returns work | **NI** | No submit/return state. Closest: `dossier status <slug> review` and the delegation contract's Return Expectations (text). |
| 10 | Manager sees contribution + sync health | **Partly VD, mostly CONTRA** | Contribution appears after the manager's next pull (SessionStart hook, MCP recall/save/rename, or manual `dossier sync`). Per-author audit shards make attribution visible via `show`/audit. Health: only `doctor` and `sync --status`, both fed by `.syncstate.json`, which records a failed attempt as `LastSync` (`internal/sync/sync.go:170`) and replaces the conflict list each run. `doctor` prints "Unresolved conflicts: 0" in its sync block while listing an unresolved conflict issue two lines below (dogfood; `internal/core/service.go:441` vs `:396-403`). TUI has no sync surface (grep). |

### Pay-special-attention items

- **Team Sync trigger paths [fact, VC].** SessionStart pull and SessionEnd push
  (both `_, _ =` discarded, `service_session.go:218`, `:501`); MCP debounced sync
  after `dossier_recall`, `dossier_save`, `dossier_rename` only
  (`internal/mcp/tools.go:324`, `:419`, `:611`), results discarded
  (`internal/mcp/server.go:77`, `:96`, `:106`). **Not triggered by:** MCP
  `dossier_list`, `dossier_search`, `dossier_promote`, `dossier_link`,
  `dossier_merge`, `dossier_session`, `dossier_update`; *any* CLI write
  (`promote`, `status`, `lead`, `next`, `rename`, …); `dossier open`; the TUI.
  A manager who assigns work via CLI/TUI publishes nothing until a later sync.
- **Sync failures and conflicts visible without `doctor`? [fact]** Only when the
  user runs `dossier sync` by hand, and even then a network failure is a
  "Warning:" line followed by "Sync successful", exit 0 (`cli.go:1541`; dogfood).
  Background sync conflicts write `conflicts/*.md` silently. `show`/`ls` do not
  mention conflicts (dogfood).
- **What syncs [fact, VC].** `.gitignore` (`internal/sync/gitignore.go:31-42`)
  excludes config, credentials, root and per-slug `sessions/`, `context/`, locks,
  sync state. **Everything else syncs**, including compiled transcript artifacts
  (tool results, file contents the agent read, env output; `thinking` excluded
  per B13) and — contrary to B13's intent — the **byte-preserved raw JSONL
  artifact** that `promote` archives when given transcript content
  (`internal/core/service_promote.go:90-103`), thinking included. `files/`
  (loose working files) also syncs.
- **Claude Code vs Pi [fact].** Claude: deterministic pre-bound launch, hooks,
  MCP. Pi: identity + lifecycle bridge verified live (HANDOFF 2026-09-09), no
  MCP, no pre-bound launch, no CLI Distilled-State update. For a team pilot Pi is
  materially weaker than the plan implies.
- **Working directory [fact].** Always the Dossier directory (step 7).
- **`lead` as assignment [fact].** Not usable as a true mechanism: free text,
  per-machine vocabulary, no identity link to `author`, no state, no
  notification. Usable as a *manual convention* for a concierge pilot.
- **Session end [fact].** Archives transcript only; cannot create Distilled
  State (step 8).
- **One store per machine [fact].** Claude MCP is registered as
  `dossier mcp serve` with no `--home` (`internal/harness/claudecode.go:339`);
  hooks likewise resolve `DOSSIER_HOME`/`~/.dossier`. The plan's
  personal/team/restricted split has no current mechanism.
- **Every-session disclosure [fact].** The unbound SessionStart nudge injects the
  names of *all* open Dossiers in the store into every Claude session
  (`service_session.go:262`). In a team store that is every teammate's topic
  titles in every unrelated session.

## 3. Most important corrections to the saved plan

Applied inline in the plan as `> Review:` notes. Summary:

| Plan claim (section) | Correction | Confidence |
|---|---|---|
| "Onboarding is more technical than the documentation claims" (Rollout blockers) | Correct but understated: join is not merely non-transactional, it is **unrecoverable after one failure** without deleting `.git/`; auth failure is silent (`GetAuth` returns `nil, nil`). | high (dogfood) |
| "Automatic sync is not sufficiently observable" | Correct, and worse: the recorded health data is **wrong**, not just unsurfaced (`LastSync` on failure, false "Pushed", conflict count reset). Fixing observability starts by fixing `syncState`. | high (dogfood) |
| "Privacy is repository-wide" | Correct, plus: `team create` publishes the manager's *entire existing store*, and can merge into a non-empty remote. The shared-data boundary is breached at creation, not only at sync. | high (dogfood) |
| "Raw and compiled transcripts should remain local-only" | Raw JSONL already reaches the shared repo via `promote` content capture; B13 only covers the session stash. | high (VC) |
| "Conflict handling is still developer-oriented" | It is *absent*: no resolution operation exists for any conflict file. | high (VC) |
| Phase 4 "Personal/team/restricted stores" | Blocked by one-store-per-machine harness registration; this is a Phase 1-sized design issue, not a Phase 4 add-on. If separate stores are the privacy answer for the pilot, the colleague must use the team store *as* their only store. | high (VC) |
| "Keep Pi supported" (Harness recommendation) | Pi cannot update an existing Dossier's Distilled State via CLI; say so rather than implying parity-minus-MCP. | medium (VC; did not run a live Pi session) |
| Launcher step 3 "pull with a short bounded timeout" | Already partially true via the SessionStart hook (5 s); the gap is that the result is discarded and `open` itself does not pull. | high (VC) |
| Contributions `contributions/<author>/…` | Overbuilt for the pilot; per-author audit shards + conflict preservation already give attribution. See §B.3. | medium |

### B. Stress-test answers

**B.1 First-class Assignment object?** **[rec]** Yes eventually, not for the
pilot. Smallest viable schema (file `assignments/<asg_id>.md`, single writer =
assignee after acceptance): `id`, `dossier_id`, `assignee` (stable id),
`assigned_by`, `state` (`proposed|accepted|submitted|closed`), `accepted_revision`,
`deliverable` (optional heading anchor into the Dossier's `## Deliverables`).
Decision Rights / Escalation / Return Expectations already live in the
Delegation Contract (B16) — **reuse it, don't create a second relationship
record**; the assignment should *point to* the contract. Everything describing
the work stays in the Dossier body. Confidence **medium**. Experiment: run the
pilot with `lead` + contract + a manager-maintained checklist; count how often
state was ambiguous. *Product decision* (whether acceptance is a state at all);
schema is implementation.

**B.2 `inbox` + `dossier work <assignment>`?** **[rec]** Simpler first:
`dossier open` already does 5 of the 9 launcher steps. Add `dossier ls --mine`
(filter `lead == author`) and make `open` pull + print health before launch.
That is an inbox without a new object. Confidence **medium-high**. Experiment:
concierge pilot with the manager sending the exact `dossier open <slug>` line;
measure failures. Implementation decision, except "must the colleague never see
a slug" (product).

**Superseded by owner decision (2026-09-18): MCP is the colleague's primary
path, and colleagues never handle slugs.** The colleague starts Claude as usual,
in the folder where their work lives, and asks for their work in plain language
("what's assigned to me?", "let's continue the pricing review"). This already
works without new code:
- every Claude start pulls first (`internal/core/service_session.go:215-219`);
- `dossier_list` search matches name and `lead` (`internal/core/query.go:34`);
- `dossier_session` binds by slug or id (the agent gets the slug from
  `dossier_list`; correction 2026-09-19, it does not bind by name) and returns
  the full state plus the Guide.

The dashboard (`dossier tui`, `f` for the lead filter, `c` to launch) is the
visual alternative. It reads the local copy, so run `dossier sync` first.
`dossier open <slug>` remains for the manager.

Two conditions:
- ~~**"Me" must be resolvable.** Set `lead` to a name the colleague will say, and
  have them say it; the agent doesn't know which lead is "me". `lead` isn't
  linked to `author` (see step 4).~~ **Resolved by Team MVP M3–M5 (2026-09-19):**
  `lead` stores the roster username, the agent is told "You are working as
  <Name> (<username>)", and `dossier_list` accepts `lead: "me"`.
- **Ambiguity must stop the agent.** If two Dossiers match, the agent must ask,
  per the no-silent-link rule.

This reframes `ls --mine` and `inbox` as optional.

**B.3 Append-only Contributions?** **[rec]** Defer. With one colleague per
Dossier (the plan's own pilot shape) multi-writer pressure is low, and
contributions would add a reconciliation step whose owner is undefined. The
immediate need is a working conflict-resolution path. Confidence **medium**.
Experiment: count `sync_concurrent_edit` conflicts in the pilot; build
contributions only if >1 per Dossier-week. Product decision (who curates).

**B.4 Transcripts local-only by default?** **[rec]** Yes, for *both* raw and
compiled. Tradeoff, exactly: local-only means a teammate or manager cannot
follow a `[src:art_…]` citation into another person's session, and if that
person's machine is lost the transcript is gone (their Distilled State and
explicit evidence artifacts survive). Shared-by-default means every file an
agent `cat`ed, every env dump and tool result is in git history on every clone
**permanently** (B13's own asymmetry argument applies). Confidence **high**.
Experiment: in the pilot, grep compiled transcripts for tokens/paths/customer
names before any push. Product decision.

**Overruled by owner decision #5 (2026-09-18): transcripts sync, now and in
future.** The recovery and followability side of the tradeoff was chosen. The
cost, permanent disclosure of whatever an agent reads, is carried by the data
boundary (decision #2).

**B.5 Separate stores sufficient for pilot?** **[rec]** Not buildable cheaply
(one store per machine). ~~Manager keeps personal work in a separate
`DOSSIER_HOME` and must **not** run `team create` on their personal store.~~
**Superseded by decision #3 (single store, 2026-09-18):** everyone, the manager
included, uses one store: the team store at `~/.dossier`. Anything not
team-safe stays out of Dossier. Confidence **high**. Product decision.

**B.6 GitHub transport before Katana?** P0 list §6 plus a live-GitHub drill.
Nothing about Katana is needed to fix any of them. Confidence **high**.

**B.7 What can stay concierge/manual?** Store creation (manager, scripted),
credential setup (screen-shared, fine-grained PAT written by a helper script),
assignment (message with `dossier open <slug>`), conflict resolution (manager),
recovery of unsaved sessions (manager reads transcript). ~~Health checks
(manager runs `doctor` daily).~~ Health is automated in the pilot via the TUI
footer (P0-9). Confidence **high**.

**B.8 Overbuilt / defer:** Contributions, recovery queue UX, restricted stores,
audience projections, redaction policy engine, Katana adapter, stable-identity
manifest (use GitHub login when needed). Confidence **medium-high**.

## 4. Product-manager decision memo

Each: decision · options · **recommendation** · cost of being wrong · evidence
that would change it.

1. **Pilot harness.** Claude Code only / Claude + Pi. **Claude Code only.** Wrong
   ⇒ Pi colleague cannot update state from CLI, pilot measures a harness gap not
   the product. Change if: a CLI Distilled-State update command ships and a live
   Pi drill passes.
   **DECIDED (owner, 2026-09-18): Claude Code is the pilot harness.**
2. **Pilot data boundary.** Prohibit: customer PII, credentials/secrets, HR and
   compensation, unreleased financials, anything under NDA with third parties /
   Allow "internal operational work" / Allow all. **Prohibit the first four
   categories plus any work whose agent session would read those files**
   (transcripts sync). Wrong ⇒ permanent disclosure in git history on every
   clone. Change if: transcripts become local-only and a pre-push scan exists.
   **DECIDED (owner, 2026-09-18): banned from the shared store are personal
   data (PII), credentials, and HR and compensation.** Because transcripts sync
   (decision #5), the ban also covers any session in which the agent would
   *read* such material, not only what gets written into a Dossier. Unreleased
   financials and NDA material were proposed but not adopted, so they are
   allowed unless the owner adds them.
3. **Privacy model.** Separate stores now / repo-wide visibility temporarily.
   ~~**Repo-wide visibility, temporarily, with a single dedicated pilot store
   created fresh (never from a personal store).**~~
   **DECIDED (owner, 2026-09-18): single store.** Each person, manager
   included, has exactly one store: the team store at `~/.dossier`. Everything
   in it is visible to everyone with repo access. Work that is not team-safe
   does not go into Dossier. Rejected alternative: a personal store plus a team
   store selected through `DOSSIER_HOME`, because of the day-to-day switching
   cost. Consequences:
   - Before `team create`, the manager must move every non-team-safe Dossier
     directory *out of* `~/.dossier` to a folder outside the store. Archiving
     (`dossier archive`) is **not** enough, because archived Dossiers stay in
     the store and sync.
   - After the pilot starts, Dossier is no longer available for private work on
     that machine.
   - Revisit if a second team or restricted topic appears.
4. **Assignment authority.** Manager-only create/assign/close; assignee accepts /
   anyone assigns / owner-lead model. **Manager creates, assigns, closes;
   colleague may mark `review`/`blocked`; reassignment manager-only.** Wrong ⇒
   ambiguous ownership, duplicate Dossiers. Change if: colleagues need to
   delegate onward.
   **DECIDED (owner, 2026-09-18): as recommended, plus: only the Dossier's
   lead edits its body (from decision #7).**
5. **Capture policy.** Curated only / curated + explicit evidence / + compiled
   transcripts. **Curated + explicit evidence shared; transcripts local.** Wrong ⇒
   either leaks (too much) or unrecoverable context (too little). Change if:
   pilot shows managers repeatedly need a colleague's transcript.
   **DECIDED (owner, 2026-09-18), overruling the recommendation: compiled
   transcripts sync, for the pilot and going forward.** Curated state, explicit
   evidence and compiled transcripts are all shared. B13's exclusions still
   stand: the raw session stash and `thinking` turns stay local.
6. **Offline policy.** Work continues offline; how stale before launch.
   **Always continue; show "last successful pull" at launch; no blocking
   threshold in the pilot.** Wrong ⇒ blocked colleagues (too strict) or work on
   stale briefs (too loose). Change if: a pilot incident traces to staleness.
7. **Conflict policy.** Remote-wins + recovery / guided reconciliation /
   single-writer owner. **Single-writer by convention (the `lead` edits
   `dossier.md`; others comment via the manager) plus existing remote-wins as the
   safety net.** Wrong ⇒ conflict backlog nobody can clear (there is no resolve
   operation). Change if: two people must co-edit one Dossier weekly.
8. **Workspace model.** Dossier dir / mapped workspace / user-selected.
   **User-selected (launch from cwd) as a flag on `open`, mapped workspace
   later.** Wrong ⇒ Claude can't see the work files. Change if: most pilot work
   has no local files (then Dossier dir is fine).
   *Effectively settled for colleagues by the MCP-primary decision:* Claude
   starts wherever the colleague opens it. The question remains only for
   `dossier open` and the dashboard, which still launch in the Dossier
   directory.
9. **Enforcement.** Warn / require Dossier launch. **Warn only.** Wrong ⇒ brittle
   adoption or offline lock-out. Change if: direct launches exceed ~50% and
   produce unattached work.
   *Moot under the MCP-primary decision:* a direct Claude launch **is** the
   supported path. What remains is detecting a session that did Dossier work
   without binding. That is a pilot metric (unbound sessions), not an
   enforcement mechanism.
10. **Katana threshold.** **Adopt only when SSO identity *or* per-Dossier ACLs
    become a hard requirement** (e.g., a restricted topic must be shared with a
    subset). Wrong ⇒ approval/hosting cost for no user-visible gain. Change if:
    Katana offers identity + ACL out of the box with low approval cost.
11. **Success criteria for mandatory rollout.** **All ten plan gates, plus: zero
    P0 defects open; two consecutive pilot weeks with zero manual git/file
    interventions; ≥80% of sessions on assigned Dossiers end with a
    Distilled-State save; median assignment-to-first-useful-turn < 5 min.**
    Wrong ⇒ mandate on a tool that loses trust in week one.
    **DECIDED (owner, 2026-09-18): as recommended.**
12. **Colleague value.** **"I start every assigned task already briefed, and I
    never re-explain context to the manager."** If the colleague's only benefit
    is the manager's visibility, adoption decays. Change if: pilot interviews
    name a different benefit.

**Decide first:** #2 (data boundary), ~~#3 (privacy model)~~ (decided: single
store), #1 (harness). They gate whether the pilot can start safely. With a
single store, #2 matters more: it is now the only boundary.

## 5. Documentation reconciliation

| Document | Classification | Action taken / proposed |
|---|---|---|
| `SPEC.md` | canonical/current — **§7 sync/team entries and §14.11 contradicted** | Proposed (below); not edited (out of scope). |
| `BUILD-DECISIONS.md` | canonical; must not be deleted | B13 should acknowledge promote's raw JSONL artifact (proposed). |
| `ARCHITECTURE.md` | canonical | No contradictions found in the reviewed areas. |
| `PRD.md` / `PRFAQ.md` | canonical (why) | PRFAQ "In pilot" is accurate; add that team create publishes the whole store (proposed). |
| `HANDOFF.md` | operational, needed correction | Status entry added (applied). "Stage: All Milestones Completed (Project Fully Finished)" and reading-order item 7 ("read VISION/PLANv02 before v02 work") conflict with PLANv02's own HISTORICAL banner — proposed for owner review, not rewritten. |
| `VISION.md` | proposed/non-canonical | "Being built next" (§ near line 590) lists work nobody is building (proposed banner). |
| `PLANv02.md` | historical/superseded; **must not be deleted** (records design rationale) | Already carries a correct banner; no change. |
| `docs/team-adoption-architecture-plan.md` | proposed/non-canonical | Review banner + inline notes (applied). |
| `docs/team-sync-plan.md` | historical plan, partially implemented | Status banner + implementation-divergence list (applied). |
| `docs/team-sync-onboarding.md` | operational but wrong — **would mislead a colleague** | Banner, superseded claims struck with replacements (applied). |
| `docs/team-sync-runbook.md` | operational but wrong | Banner, corrections, new failed-join entry (applied). |
| `docs/harness-capabilities.md` | canonical for verified capabilities | Added unverified SessionEnd-visibility note and Pi CLI write gap (applied). |
| ADRs 0001–0009 | decision records; **must not be deleted** | ADR 0005 §2 ("onboarding is `team join` + one GitHub sign-in") and Consequences ("entered once") describe unbuilt behavior — propose a dated status note. |
| `docs/tui-plan.md` | historical (per HANDOFF) | Not reviewed further. |
| `docs/spikes/gitsync-findings.md` | historical | Keep. |

### Exact contradictory claims

- `docs/team-sync-onboarding.md` §"One-time setup": "Ask for your name" /
  "Ask you to sign in once with a PAT" — **NI** (`cli.go:1606-1650`).
- Onboarding §"If two of us edited…": "Dossier's dashboard walks you through it
  step by step" — **CONTRA** (TUI resolver is merge-only).
- Onboarding §"What never leaves your machine": "your per-topic session captures
  … does sync" — **CONTRA** (`*/sessions/` ignored, `gitignore.go:36`); what
  *does* sync is compiled transcript artifacts.
- Onboarding §"Day-to-day": "a later phase makes syncing automatic" — partially
  shipped (hooks + three MCP tools).
- Runbook quick-reference + §2: `sync_auth_failed`, "re-auth command printed in
  the warning", "re-prompts for a fine-grained PAT" — **NI**.
- Runbook §1 / healthy-state: `sync --status` "reports … stale credentials" — **NI**
  (fields are Ahead/Behind/Dirty/Conflicts/LastSync only, `cli.go:1512-1517`).
- Runbook §4: "reconcile in the TUI", "TUI footer shows a conflict count" — **NI**.
- Runbook §5: "per-dossier session stashes … sync" — **CONTRA**.
- SPEC §7 `team create`: "Validates the target repo is empty" — **CONTRA** (dogfood).
- SPEC §7 `team join`: "merge-adopt flow with confirmation", "Confirms author",
  "Prompts for and stores a GitHub PAT" — **NI**.
- SPEC §7 `sync`: "expired/invalid auth … explicit surfaced warnings" — generic
  "Sync network error" only; exit 0.
- SPEC §14.11 status: "all criteria covered by automated tests … EXCEPT" PAT
  onboarding — "push retries later with a visible warning" and "persistent
  visible warning" for >100 MB are not true on background paths.
- `docs/team-sync-plan.md` Phase 3 §4 surfacing (TUI footer, `dossier_list` sync
  slot) — **NI**; Phase 2 §4 `sync_auth_failed` — **NI**.
- HANDOFF D8 says silent auto-sync failures are "surfaced via `dossier doctor`"
  — true, but `doctor` reports the wrong `LastSync` and conflict count.

### Proposed text for out-of-scope canonical docs (not applied)

**SPEC.md §7, `dossier team create`** — replace "Validates the target repo is
empty." with:
> - *Specified, not yet implemented (2026-09-18 review):* validates the target
>   repo is empty. Today it does not; pointed at a non-empty remote it merges and
>   pushes. It also publishes every Dossier already in the store, including
>   archived ones. Before creating, move anything not team-safe out of the
>   store directory.

**SPEC.md §7, `dossier team join`** — append:
> *Status (2026-09-18 review):* clone and post-join `init` are implemented. Not
> implemented: merge-adopt flow, author confirmation, PAT prompt. Credentials
> must pre-exist at `$HOME/.dossier/credentials` (mode 0600) or come from
> `gh auth token`. A failed clone leaves `team.remote` and a partial `.git/`
> behind and blocks retry.

**SPEC.md §7, `dossier sync`** — append:
> *Status (2026-09-18 review):* auth failures surface as a generic
> "Sync network error"; the command exits 0 and prints "Sync successful" even
> when the network step failed. `--status` does not report credential state.

**SPEC.md §14.11 status line** — replace with:
> Status (2026-09-18): convergence, remote-wins conflict capture, oversized
> exclusion and machine-local exclusion are covered by tests against local bare
> repos. Not met: two-command/one-sign-in onboarding (no auth prompt); visible
> warnings on background sync (results discarded); persistent oversized-file
> warning (per-run only). Never exercised against live GitHub.

**BUILD-DECISIONS.md B13** — append to Decision:
> *Gap found 2026-09-18:* `Service.Promote` archives byte-preserved raw JSONL
> (thinking included) as an artifact when given transcript content; artifacts
> sync. B13's exclusion does not cover this path.

**ADR 0005** — add under Status:
> *Implementation note (2026-09-18):* Decision §2's "one GitHub sign-in" and the
> Consequences' "PAT entered once" are not built; see
> `docs/team-adoption-plan-review.md`.

**VISION.md** — add above "Being built next":
> *Status note (2026-09-18):* the list below is aspirational, not in progress.
> Current direction: `docs/team-adoption-architecture-plan.md` and its review.

## 6. Revised roadmap and acceptance criteria

All items preserve the hard rules: files are truth, pure core, one service for
CLI/MCP/TUI, no native delete, no LWW on Distilled State, no silent truncation,
visible degradation, machine-local files never sync, no capability claim without
harness verification.

### P0 — release blockers (before any colleague touches Team Sync)

> **Status (2026-09-18, second session):** P0-1 to P0-7 and P0-9 are implemented on
> `review/team-adoption-plan` and pass `team-sync-validation.md` Parts A and C. Part D
> (live GitHub) is next. See `HANDOFF.md`.

| # | Fix | Acceptance |
|---|---|---|
| P0-1 | `team create` refuses a non-empty remote. Because of the single-store decision, publishing existing Dossiers is the *normal* path, so create first **lists every Dossier it will publish, archived ones included, and requires confirmation** (`--yes` for scripts). | Test: create against non-empty bare repo ⇒ error, remote unchanged. Test: store with 2 Dossiers ⇒ both listed; declining leaves no `.git/` and no `team.remote`. |
| P0-2 | `team create`/`join` transactional: config written only after success; failed clone/push removes what it created (moved to a `.failed-join-<ts>/` archive dir, not deleted). Retry succeeds. | Dogfood + test: bad URL, then good URL ⇒ joined. |
| P0-3 | Honest sync state: `syncState` records `last_attempt`, `last_success_pull`, `last_success_push`, `last_error`, `auth_state`; CLI `sync` exits non-zero and does not print "successful" when `report.Error != ""`; `Pushed` false on no-op. | Tests: unreachable remote ⇒ exit≠0, `last_success_*` unchanged. |
| P0-4 | Unresolved-conflict count derived from `ListConflicts`, not the last sync run; `doctor` sync block agrees with its issue list. | Test: conflict persists across a clean sync. |
| P0-5 | A conflict resolution operation in `core.Service` (keep current / restore mine / both-as-disagreement), which archives the conflict file to `conflicts/resolved/` (non-destructive), exposed in CLI, MCP, TUI. | Table tests + one TUI test; SPEC §7/§8 amended. |
| P0-6 | Auth failure explicit: `GetAuth` returns a typed "no credentials" warning instead of `nil, nil` when `team.remote` is https; 401/403 mapped to `sync_auth_failed` with a concrete next step. | Test with fake runner + fake remote returning 401. |
| P0-7 | Promote's raw JSONL artifact excluded from sync (or not written to `artifacts/` in team stores), per B13. *Still required after decision #5:* that decision shares **compiled** transcripts, whereas this artifact is raw and carries `thinking`, which B13 excludes. | Test: promote with JSONL in a team store ⇒ raw artifact absent from remote. |
| P0-9 | **Automated health in the TUI (pilot scope, owner 2026-09-18).** When the TUI starts, and on fsnotify refreshes (throttled, at most once a minute), it runs a health check **asynchronously with a timeout** and renders one footer line on the dashboard and detail views, e.g. `Team sync · synced 3m ago · 1 local change · 1 conflict · 2 issues`, or `Team sync · last sync failed 18m ago · work is safe locally`. A key (e.g. `H`) opens the full `doctor` report as an overlay. The summary is computed **in core**, from the same data `Doctor` uses (e.g. a `HealthSummary` on the doctor report or a `Service.Health`), so CLI (`dossier doctor`/`sync --status`) and the TUI cannot disagree; the TUI only renders it. `Syncer.Status` gets a context and bounded timeout, because it currently fetches with `context.Background()` (`internal/sync/status.go:67-80`). Without team sync configured, the footer shows local issues only. No role gating. **Depends on P0-3 and P0-4.** | Core table test: health summary from fixtures (never synced / synced / failed / conflicts / issues). TUI test: footer renders the summary and an unreachable remote doesn't block `Init`/first render. Golden test: TUI footer text matches the CLI summary for the same store. |
| ~~P0-8~~ | ~~Live-GitHub drill (owner)~~. Not a development item. The owner runs it after P0-1 to P0-7, as Part D of [`team-sync-validation.md`](team-sync-validation.md). | — |

### Post-P0 follow-ups (2026-09-18, branch `feat/conflict-view-and-sync-warnings`)

| Item | Status |
|---|---|
| Side-by-side conflict view: `Service.ConflictDetail` (current shared body, preserved body, fresh diff); `dossier conflicts <id>`; MCP `dossier_conflicts` with `conflict_id`; TUI `x` overlay shows two aligned columns (stacked when narrow), `d` toggles the diff | **Done** (WS-E) |
| `team join` accepts a pre-written `~/.dossier/credentials` (the default store is `~/.dossier`, so the documented setup was refused as "existing store") | **Done** |
| Merge-adopt join | **Dropped** (owner): nobody on the team has a store |
| Background-sync warnings (Increment 1 item 1): SessionStart line, `dossier_session`/`dossier_recall` warnings, one-time warning after an MCP background sync, `open` health line; plus the pre-existing ~35 s offline SessionStart hang it exposed (`GitSync.divergence` ignored the context), now 5.1 s against an unroutable remote | **Done** (WS-F) |
| Manual `dossier sync` against an unreachable remote took ~90 s (three 30 s default dial timeouts) | **Done**: 10 s connect/TLS timeout on git HTTP(S); now ~10 s |

### Team MVP (before the pilot; owner decisions 2026-09-18, recorded as BUILD-DECISIONS B17)

**Goal:** a non-technical colleague on a work Mac or Windows PC joins with one
command and a browser click, appears to the team under their name, and asks
Claude "what's assigned to me?" with no setup beyond `gh`.

**Constraints (owner):** the team has GitHub accounts and the GitHub CLI (`gh`),
which the org allows; a new GitHub App would need org approval, so none is
used. The org assigns usernames. One work machine per person. Both macOS and
Windows must work.

| # | Work | Acceptance |
|---|---|---|
| M1 | **Done in CI (2026-09-19, PR #18); real-machine smoke test pending.** **Platform spike: macOS and Windows first-class.** CI runs `go test ./...` on `ubuntu-latest`, `macos-latest`, `windows-latest`; the release workflow publishes `windows/amd64` (and `windows/arm64` if cheap) next to darwin/linux. Fix what Windows breaks, at least: the `0600` credentials check (Windows has no Unix modes; use an owner-only check or skip with a documented rationale); replacing the read-only (`0444`) `dossier.md` by rename; file locks; Claude Code config, hook and MCP paths on Windows (`%USERPROFILE%`); the `.dossier` home and `gh` lookup. | CI green on all three OSes. A real-machine smoke test on one Mac and one Windows PC (owner or a colleague): `init`, `promote`, `team join` against a sandbox repo, `sync`, a Claude session that binds by name and saves. Record results in `docs/harness-capabilities.md`. |
| M2 | **Done 2026-09-19** (`core.NormalizeUsername`). **Identity = org username.** `author` defaults to the OS login name with any domain prefix (`DOMAIN\`, `AzureAD\`) stripped, lowercased; still overridable in `config.yaml`. | Table test of normalization for `ACME\PSmith`, `AzureAD\psmith`, `psmith`, `Priya.Shah`. Confirm on real machines what `id -un` (Mac) and `whoami` (Windows) return and that normalization yields the org username (owner provides the two outputs). **macOS confirmed 2026-09-18:** `id -un` on the owner's work laptop returns `hgill`, the org username, with no prefix. **Windows pending** (a colleague's `whoami`, expected within days). |
| M3 | **Done 2026-09-19** (roster, `team add/remove/members`, `team create --name`, `dossier_team`, roster conflicts resolvable with all three choices). **Roster `team.yaml`** (synced, store root): `manager: <username>`, `members: {<username>: <Display Name>}`. Written by `team create` (manager = creator) and by `dossier team add <username> "<Display Name>"` / `team remove` (manager only, by convention; warn if the caller is not the manager). A concurrent edit is captured as a conflict (extend the `dossier.md` conflict path), never dropped. Core exposes `Service.Members()`; the per-machine `leads:` list is ignored when a roster exists. | Round-trip and conflict tests; `team add` on a non-manager machine warns; `doctor` flags a `lead` not in the roster. |
| M4 | **Done 2026-09-19.** **Lead = username, shown as display name.** Lead pickers (TUI, CLI `lead`, MCP `dossier_update`) offer roster members; `lead` stores the username; list, detail, `ls` and MCP results render the display name. Matching "Priya", "Priya Shah" or "psmith" finds the same Dossiers. | Tests on all three surfaces; existing free-text leads keep working and are flagged by `doctor` when not in the roster. |
| M5 | **Done 2026-09-19.** **"Me".** SessionStart context and the `dossier_session` / `dossier_list` responses state the current user ("You are working as Priya Shah (psmith)"); `dossier_list` accepts `lead: "me"`. | Test: the unbound SessionStart text names the user; `dossier_list` with `lead: me` returns only their Dossiers. Live check in Claude: "what's assigned to me?" binds without a name. |
| M6 | **Done 2026-09-19** (sandbox; live GitHub pending). **`gh`-assisted join.** When `team join` or `team create` finds no credentials and `gh` is installed, it offers to run `gh auth login --web` (confirm first; never silently), then verifies it can list the remote before cloning; if `gh` is missing, it prints the install link and the token fallback. Join prints "You'll appear to teammates as <Display Name> (<username>)", or asks the colleague to have the manager add them if they're not in the roster. | Tests with a fake `gh` runner (logged out → offers login; declined → clean exit; logged in → proceeds). Owner runs it live in validation Part D. |
| M7 | **Done 2026-09-19** except the non-developer walkthrough, which happens in the real-machine smoke test. **Docs.** Onboarding rewritten for the new flow (install Dossier + `gh`, run one command, click Authorize); runbook entries for `gh` sign-in failures and roster conflicts; SPEC §7/§8/§14; validation Part C gains checks for M2–M6. | Onboarding followed end to end by a non-developer in the smoke test. |

**Order:** M1 first (largest unknown), with M2–M5 in parallel; M6 after M1
(it touches process spawning on Windows); M7 last; then validation Part D
(live GitHub) and Part E.

**Not in the MVP:** GitHub App or our own OAuth flow; merge-adopt join;
multiple devices per person; roles and permissions; a side-by-side merge
editor.

### Smallest viable concierge pilot (after P0 and the Team MVP)

> *Sequencing (owner, 2026-09-18):* the pilot now follows the Team MVP. The
> MVP replaces the concierge token setup with `gh` sign-in and the free-text
> lead name with the roster; the rest of this section stands.

1 manager, 1 colleague, 3 Dossiers, Claude Code only. Everyone uses a **single
store**, the team store at `~/.dossier` (decision #3). Before `team create`,
the manager moves non-team-safe Dossiers out of the store. Manager assigns by setting
`lead` to the colleague's name, syncing, and telling them the Dossier's *name*.
The colleague opens Claude in their work folder and asks for it (MCP primary
path, §B.2). The manager watches the TUI health footer (P0-9), opens the full
report when it shows a problem, and
resolves conflicts. Transcripts: accept that compiled transcripts sync *only if*
decision #2's prohibited categories are enforced by convention; otherwise wait
for Increment 1 item 4.
**Acceptance:** 2 weeks, zero manual git/file interventions, every session on an
assigned Dossier ends with a save, colleague interview answers decision #12.

### Increment 1 (after pilot)

0. ~~**Automatic health for the manager.** On TUI start (and on each fsnotify
   refresh, throttled), the TUI calls the existing `Service.Doctor` **async**
   and renders a one-line footer: sync age, pending changes, unresolved
   conflicts, and issue count. Enter opens the full report. The call must be
   async and bounded: `Doctor` → `Syncer.Status` does a remote fetch with no
   timeout (`internal/sync/status.go:67-80` passes `context.Background()`).
   Depends on P0-3 and P0-4; otherwise the footer shows the current wrong
   `LastSync` and conflict count. No role gating: everyone sees it.~~ Moved
   into pilot scope as **P0-9** (owner, 2026-09-18).
1. The **`dossier_session` bind response** (the primary path) and SessionStart
   carry one health line (last successful pull/push, pending changes) plus any
   unresolved conflict for the bound Dossier. `open` pulls (bounded) and prints
   the same line. *Done 2026-09-18 (WS-F); see "Post-P0 follow-ups".*
2. ~~Resolve "me": link `lead` to `author` (or a display name in config), so
   "what's assigned to me?" needs no name.~~ Moved into the Team MVP (M3–M5).
   `open --here` (cwd workspace) for the non-MCP launchers remains here.
3. A CLI `dossier save <slug> --distilled-file … --base-revision …` (Pi parity).
4. ~~Transcripts local-only by default in team stores (gitignore compiled
   transcript artifacts or store them outside `artifacts/`), with explicit
   promotion.~~ Dropped by decision #5 (transcripts sync).
5. Sync after every mutating service call, not only three MCP tools — done once
   in core or in each adapter's shared wrapper, never forked.

**Acceptance:** each item has a SPEC §14 entry and tests; health line matches
`doctor` in a golden test.

### Explicitly not yet

Assignment objects, inbox, Contributions, recovery queue UI, multi-store,
ACLs/projections, Katana adapter, identity manifest, PLANv02 requirements /
roster / timezone features (the Team MVP's `team.yaml` is a name map only, not the PLANv02 roster). **Roles in config** (asked 2026-09-18): not yet.
`config.yaml` is machine-local and never syncs, so a role there is
self-asserted and enforces nothing, since everyone with repo access can write
everything. The one identity need, resolving "me" (Increment 1 item 2), is a
name, not a role. If roles are needed later, they belong in a synced team
manifest, and they stay advisory until the transport enforces access.

### Drills before claiming readiness

Revoked token mid-push; remote force-pushed; clock skew; two machines editing
one body; offline week then reconnect; upgrade binary between two pilot
machines; ~~join from a machine with an existing `~/.dossier`~~ (requirement dropped by the owner, 2026-09-18: no one on the team has a store); live Claude session
ending without a save (confirm whether the warning is ever seen).

## 7. Open uncertainties and cheapest next tests

> **Sequencing (owner, 2026-09-18; updated):** P0-1 to P0-7 and P0-9 are done.
> Next is the Team MVP (§6), then validation Part D and Part E, then the pilot.
> Originally: the pilot waits for P0-1 to P0-7 and P0-9, which
> are to be implemented in a separate session. After that session, run
> [`team-sync-validation.md`](team-sync-validation.md) (sandbox checks, then
> the owner's live GitHub test, then the pilot go/no-go). The first and fifth
> rows below are covered by that doc's Part D. The others remain open.

| Uncertainty | Cheapest test |
|---|---|
| Does GitHub's empty-repo HEAD make the `main`-hardcode latent? | Create an empty private repo, `team create`, `team join` from a second `DOSSIER_HOME`. 10 min. |
| Is the SessionEnd warning ever visible in Claude Code? | End a bound Claude session without saving; look for the text in the UI and in `claude --debug` output. 5 min. |
| Does the MCP debouncer drain before Claude kills the server? | Save via MCP, `/exit` within 2 s, check remote. 5 min. |
| Can a Pi agent update an existing Dossier at all? | Live Pi session on a bound Dossier: ask it to record a decision; inspect `history/`. 10 min. |
| Does `gh auth token` fallback work with a fine-grained-only org? | One colleague machine with `gh` logged in; `team join`. 10 min. |
| How often do conflicts actually occur with a lead-only-edits convention? | Pilot metric. |

## 8. Changes applied

Commits on `review/team-adoption-plan` (none pushed):

| File | Change | Commit |
|---|---|---|
| `docs/team-adoption-plan-review-agent-prompt.md` | Made the review read/write with a write scope | `docs: make team adoption plan review read/write` |
| `docs/team-adoption-plan-review.md` | This report | `docs: add team adoption plan review` |
| `docs/team-adoption-architecture-plan.md` | Review banner + inline `> Review:` notes | `docs: annotate adoption plan with review findings` |
| `docs/team-sync-onboarding.md`, `docs/team-sync-runbook.md`, `docs/team-sync-plan.md` | Status banners; superseded claims struck with evidence-cited replacements; failed-join runbook entry | `docs: correct Team Sync operational docs against code` |
| `docs/harness-capabilities.md` | Pi CLI write gap; SessionEnd visibility unverified | same commit as above |
| `HANDOFF.md` | Dated status entry with verified findings | `docs: record team adoption review status in HANDOFF` |
| Review, runbook, plan | Single-store decision | `docs: record single-store pilot decision` |
| Review, onboarding | MCP as the colleague's primary path | `docs: make MCP the colleague's primary path` |
| Review, plan | Harness, data boundary and transcript decisions; TUI health and roles on the roadmap | `docs: record harness, data boundary, and transcript decisions` |
| `docs/team-sync-validation.md`, review, `HANDOFF.md` | Post-development validation procedure; assignment and success-criteria decisions; P0-8 moved to validation | `docs: add Team Sync validation procedure` |
| Review, validation, runbook, plan, `HANDOFF.md` | P0-9: automated TUI health footer in pilot scope | `docs: add automated TUI health footer to pilot scope` |

**Proposed, not applied (out of write scope):** SPEC §7 ×3 and §14.11,
BUILD-DECISIONS B13, ADR 0005 status note, VISION.md banner, PRFAQ "Can I share"
addendum, HANDOFF "Stage" line and reading-order item 7 — text in §5 above.
