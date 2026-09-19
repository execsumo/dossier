# Agent Prompt: Review the Team Adoption Architecture Plan

> Recommended model: `gpt-5.6-luna`
>
> Recommended effort: `xhigh`
>
> This is a read/write review. Apply the documentation corrections you
> find, within the write scope below. Product code and settled decisions
> stay unchanged; propose those changes instead of making them.

## Write scope

Work on a new branch (`review/team-adoption-plan`). Commit in small, reviewable steps. Do not push, open a PR, or merge.

**You may edit:**

- `docs/team-adoption-architecture-plan.md`: correct factual errors in place, mark unsupported claims, and add a status banner saying the plan is proposed and non-canonical. Keep the plan's recommendations. Where you disagree with one, add an inline `> Review:` note that gives the evidence, and don't rewrite the recommendation itself.
- Operational docs that contradict executable reality: `docs/team-sync-onboarding.md`, `docs/team-sync-runbook.md`, `docs/team-sync-plan.md`, `docs/harness-capabilities.md`, and `PLANv02.md`. Fix instructions that would fail or mislead. Add a status banner (current, superseded, or historical) at the top of each doc you classify.
- `HANDOFF.md`: update only the status section, and only to reflect verified facts.
- A new report file, `docs/team-adoption-plan-review.md`, containing the full output described below.

**You may not edit:**

- Anything under `internal/`, `cmd/`, or `assets/`, or any test file. Record defects you find as roadmap items with evidence.
- `SPEC.md`, `BUILD-DECISIONS.md`, `ARCHITECTURE.md`, `PRD.md`, `PRFAQ.md`, `CLAUDE.md`, or anything under `docs/adr/`. These record contracts and decisions. Put the exact changes you propose for them, as diffs or quoted replacement text, in the report's documentation-reconciliation section.

**Rules for every edit:**

- Nothing is deleted. Superseded text is struck through or moved under a clearly labelled "Superseded" heading, with a pointer to what replaces it. Do not delete files or move them out of the repo.
- Every correction must cite its evidence: a `file:line` in the code or tests, or a command you ran and its output. Never correct a doc using only another doc as the source.
- Do not settle product-manager decisions (section C) in any doc. Record them as open decisions, give your recommendation, and link to the decision memo.
- Behavior you could not verify stays marked as unverified. Never upgrade it to verified.
- You may run the binary against a throwaway store (`DOSSIER_HOME=$(mktemp -d)`) to verify behavior. Do not touch `~/.dossier`, do not modify real harness configs, and do not contact live GitHub remotes.
- Before finishing, run `go build ./... && go vet ./... && go test ./...` to confirm nothing outside the docs changed, and run `git status` to confirm that only files in the write scope were modified.

## Mission

Review `docs/team-adoption-architecture-plan.md` against the actual Dossier repository and produce a decision-ready correction pass.

The plan was written for adoption by overseas, non-developer colleagues who primarily use Claude Code. The intended operating model is:

```text
manager packages work as a Dossier
  -> teammate receives an assignment
  -> Dossier launches the correct Claude session
  -> context is injected automatically
  -> useful work is captured and shared safely
  -> manager and teammates can see health and progress
```

The plan is deliberately first-principles-based and is not anchored to the current implementation. Your job is to determine which recommendations are supported by executable reality, which are incomplete, and which need a product-manager decision.

## Required reading

Read these completely before forming conclusions:

1. `docs/team-adoption-architecture-plan.md`
2. `HANDOFF.md`
3. `CLAUDE.md`
4. `BUILD-DECISIONS.md`
5. `ARCHITECTURE.md`
6. `SPEC.md`
7. `PRD.md` and `PRFAQ.md`
8. `docs/team-sync-plan.md`
9. `docs/team-sync-onboarding.md`
10. `docs/team-sync-runbook.md`
11. `docs/adr/0005-team-sync-via-github.md`
12. `docs/adr/0008-dossier-as-canonical-operational-brief.md`
13. `docs/harness-capabilities.md`

Then inspect the relevant implementation in:

- `internal/cli/`
- `internal/core/`
- `internal/mcp/`
- `internal/sync/`
- `internal/harness/`
- `internal/config/`
- `internal/tui/`
- `assets/`

Use structural code navigation where available, then read the actual source and tests. Do not infer behavior from documentation when code or tests can establish it.

## Questions to answer

### A. Verify the executable adoption journey

Trace these journeys from a fresh local store and identify every manual decision or failure point:

1. Manager creates a team store and assigns work.
2. Colleague joins from a clean machine.
3. Colleague authenticates without developer knowledge.
4. Colleague receives assigned work.
5. Colleague launches Claude through Dossier.
6. The session starts already bound to the correct Dossier.
7. Claude can access the actual workspace needed for the work.
8. Material progress is captured without relying on perfect agent discipline.
9. The colleague submits or returns work.
10. The manager sees the contribution and its sync health.

For each step, label the behavior:

- verified in executable code;
- verified only by tests;
- documented but not verified;
- contradicted by code;
- not implemented.

Pay special attention to:

- `dossier team join` authentication and transactional behavior;
- whether live GitHub behavior has actually been tested;
- all Team Sync trigger paths, including list, recall, save, promote, link, merge, rename, session start, and session end;
- whether sync failures and conflicts are visible without `dossier doctor`;
- what content is actually synced, especially compiled transcripts and artifacts;
- Claude Code versus Pi session identity and lifecycle behavior;
- the working directory used by `dossier open` and the TUI launcher;
- whether `lead` is usable as a true assignment mechanism;
- whether session-end behavior can create distilled state or only archive a transcript.

### B. Stress-test the proposed architecture

Evaluate whether the plan's recommendations are minimal and sufficient:

1. Is a first-class Assignment object necessary? If yes, define the smallest viable schema and identify what must remain in the Dossier body.
2. Is an `inbox` plus `dossier work <assignment>` the right user experience, or is there a simpler launcher model?
3. Are append-only Contributions the right way to reduce multi-writer conflicts, or would they create too much reconciliation work?
4. Should raw and compiled transcripts be local-only by default? Identify the exact privacy and recovery tradeoff.
5. Are separate personal/team/restricted stores sufficient for the first pilot?
6. What must be solved in GitHub transport before considering Katana?
7. What can safely remain concierge/manual during the first two-colleague pilot?
8. Which recommendations in the plan are overbuilt and should be deferred?

For every recommendation, provide:

- confidence: high, medium, or low;
- evidence;
- the smallest reversible experiment that would raise confidence;
- whether it is a product decision or an implementation decision.

### C. Identify product-manager decisions

Produce a dedicated decision memo. Do not hide decisions inside technical recommendations.

At minimum, address:

1. **Pilot harness:** Claude Code only, or Claude Code plus Pi?
2. **Pilot data boundary:** what categories of work are prohibited from the shared store?
3. **Privacy model:** separate stores now, or tolerate repository-wide visibility temporarily?
4. **Assignment authority:** who may create, assign, accept, reassign, and close work?
5. **Capture policy:** curated state only, explicit evidence, compiled transcripts, or some combination?
6. **Offline policy:** should work continue when the team store is unavailable, and how stale may context be before launch?
7. **Conflict policy:** remote-wins plus recovery, guided reconciliation, or a single-writer owner model?
8. **Workspace model:** should a Dossier launch in the Dossier directory, a mapped project workspace, or a user-selected workspace?
9. **Enforcement:** should direct Claude launches merely warn, or should team policy require Dossier launch paths?
10. **Katana threshold:** what concrete capability or risk justifies accepting its approval and hosting friction?
11. **Success criteria:** what must be true before mandatory rollout, and what metrics determine that?
12. **Manager versus colleague value:** what immediate benefit must the colleague experience for adoption to be durable?

For each decision, give:

- the decision statement;
- 2–4 viable options;
- your recommendation;
- consequence of choosing incorrectly;
- the earliest evidence that could change the recommendation.

### D. Reconcile the documentation set

Classify each relevant document as:

- canonical/current;
- operational but needs correction;
- proposed/non-canonical;
- historical/superseded;
- safe to archive;
- must not be deleted because it records a decision.

Identify exact contradictory claims, with file paths and sections, especially in:

- Team Sync onboarding;
- Team Sync runbook;
- Team Sync plan;
- `PLANv02.md`;
- the PRD/PRFAQ;
- harness capability documentation;
- the ADRs.

Apply the minimum documentation changes needed to stop future agents from following stale instructions, within the write scope. For docs outside the write scope, give the exact proposed change.

### E. Produce a corrected implementation roadmap

End with a revised roadmap containing:

1. P0 release blockers;
2. the smallest viable concierge pilot;
3. the first implementation increment after the pilot;
4. what should explicitly not be built yet;
5. acceptance criteria for each increment;
6. tests or dogfood drills required before claiming readiness.

The roadmap must preserve Dossier's existing hard rules:

- files are truth;
- core remains pure;
- CLI, MCP, and TUI share one core service;
- no native delete;
- no last-write-wins for Distilled State;
- no silent truncation;
- no silent harness degradation;
- machine-local configuration and bindings do not sync;
- no claim of capability without real harness verification.

## Output format

Write the report to `docs/team-adoption-plan-review.md` and commit it. The report should be concise but evidence-rich, with these headings:

1. `Executive verdict`
2. `Adoption journey: verified versus assumed`
3. `Most important corrections to the saved plan`
4. `Product-manager decision memo`
5. `Documentation reconciliation`
6. `Revised roadmap and acceptance criteria`
7. `Open uncertainties and cheapest next tests`
8. `Changes applied`: every file you modified, what changed, and the commit it landed in. Also list the proposed changes to docs outside your write scope that you left unapplied.

In your final reply, summarize the executive verdict, list the commits, and name the decisions the product manager must make first.

Do not write product code or tests. Do not merely restate the saved plan. Challenge it, correct it, and clearly separate facts, recommendations, and decisions.
