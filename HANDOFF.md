# Dossier / chainlink — Handoff

> Updated: 2026-10-07
> Purpose: the entry point for any agent picking up work. Read this first. It covers current
> status and what remains. History lives in git, `BUILD-DECISIONS.md`, the ADRs, and
> `docs/history/`. The pre-2026-10-07 handoff, with the full dated status log and the D1–D11
> decisions log that other docs cite, is [`docs/history/HANDOFF-2026-10-07.md`](docs/history/HANDOFF-2026-10-07.md).

## Start here (reading order)

1. **This file**: status, what remains, watchouts.
2. **`BUILD-DECISIONS.md`**: settled build choices. Do not relitigate them.
3. **`ARCHITECTURE.md`**: how the code is structured (Go, ports and adapters). Keep it current.
4. **`CLAUDE.md`** (`AGENTS.md` points to it): repo working rules, build/test commands, hard rules, definition of done.
5. **`SPEC.md`**: the contract: data model, CLI (§7), MCP (§8), algorithms (§11), acceptance criteria (§14).
6. **`docs/adr/`**: one record per decision since the SPEC. Read the ones your change touches.
7. **`PRD.md`** / **`PRFAQ.md`** / **`VISION.md`**: the product *why*, and the forward direction for a business audience.

Precedence when docs disagree: `BUILD-DECISIONS.md` > `SPEC.md` (mechanics) > `PRD.md`/`PRFAQ.md`/`VISION.md`.
`docs/history/` is design rationale only. Never resolve current behavior from it.

## Status (2026-10-07)

- **Core product: shipped.** Every SPEC milestone is implemented across CLI, MCP and the Bubble Tea TUI, in one Go binary with files as the source of truth.
- **Team Sync (B12, ADR 0005): built and sandbox-validated, not live-validated.**
  - Built: a private GitHub repo hidden behind the binary; per-author audit shards; conflict capture; loud sync failures; `gh` sign-in; roster with human and agent kinds; manager-only roster edits.
  - Validated in a sandbox: [`docs/team-sync-validation.md`](docs/team-sync-validation.md) Parts A–C.
  - Not done: live-GitHub validation (Part D, SPEC §14.11) and the two-colleague pilot (Part E). Both need the owner.
- **Pi: supported through Dossier's own extension** (ADR 0009). Session identity and lifecycle bridging work. One known gap, from a code read not yet confirmed in a live Pi session: the CLI cannot update an existing Dossier's Distilled State, so a Pi agent cannot save mid-session. See `docs/harness-capabilities.md` §2.
- **Session switcher over herdr (ADR 0014) and repo identity per machine (ADR 0015): shipped** (PRs #31, #32).
- **Continuity and measurement: shipped 2026-10-07** (PRs #32, #33).
  - **Guide/instructions split.** `assets/guide.md` covers Distilled State content only: a dated Current State that carries the user's corrections and approval scope, `[stated]` for claims with no source, update-in-place rules, and a pre-save checklist. `assets/instructions.md` covers tool protocol, including the On Resume check. SessionStart sends both.
  - **Unsaved-session recovery notice.** Derived from the audit log. It clears once the Distilled State cites the transcript. Other authors' sessions are attributed to them, never offered for recovery.
  - **`tools/resumeeval`.** An offline A/B harness for guide versions on fixed cases.
  - **Session outcomes by version + automatic session evals** (ADR 0016, B28).
    - `session_ended` records the version, guide hash, and the session's model and effort.
    - `dossier stats --by version|guide|model|effort|eval` reports the results.
    - Each saved session gets a detached three-call eval. The `eval:` knob in `config.yaml` controls it: on by default, model and effort configurable.

## What remains

**Owner-gated**
1. **Team Sync live validation and pilot**: [`docs/team-sync-validation.md`](docs/team-sync-validation.md) Parts D and E, then the pilot go/no-go.
2. **Dogfood data.** Build `main` on the primary-use machine so sessions start recording. Read `dossier stats` weekly. Collect 5–10 real cases for `tools/resumeeval` (see its README).
3. **Eval knob default.** It is on for every machine that doesn't set it, including teammates'. Flip `defaultEvalEnabled` in `internal/config/config.go` when tracking no longer justifies the inference cost.

**Proposed, not started (decide with data from item 2)**
4. **Mechanical turn checkpoint.** A `Stop` hook would record files edited and repo branch@commit per turn as audit events, and surface "since the last save" on resume. This targets stale saves, which the unsaved-session notice cannot see. `Stop` behavior is verified in `docs/harness-capabilities.md`.
5. **Section-level `dossier_save` with a section-policy table.** Each section would be free, protected, schema-checked or machine-owned. Saves get cheaper and more frequent, untouched sections can't be rewritten, and the Deliverables / Delegation Contracts schema moves out of guide prose and into checks in code.

**Unprocessed-session recovery: remaining pieces** (discovery is built)
6. A `doctor` advisory and a TUI marker. Discovery is agent-facing only so far.
7. Marking recovered content as recovered in the audit trail.
8. A transcript excluded from sync as oversized leaves the notice naming an artifact that is missing on other machines. Surface that case explicitly.

**Open questions**
- Which session id Cursor, Codex and Antigravity report through herdr. Unchecked; the in-memory launched map covers it meanwhile.
- Whether `open_layout: workspace` (TUI and agent side by side per Dossier) is worth adding.
- Whether Claude Code reads a bare `AGENTS.md`, and whether SessionEnd hook stdout reaches the user.
- Eval coverage of large sessions: transcripts over 400 KB are skipped. If `dossier stats` shows this often, add chunked probe extraction.

**Watch items** (no action until triggered)
- Dual Lip Gloss majors (`lipgloss` v1 and `lipgloss/v2`) are a deliberate pin pending a Bubbles upgrade. Consolidate then.
- `Store` port width (25+ methods): split by capability only if a second store implementation ever exists.

**Deferred** (named, not forgotten): a `delegation_note` artifact type; per-teammate formatting and escalation preferences; first-class links from a finished Dossier to its follow-on spark. Also see the deferred items in `BUILD-DECISIONS.md` and `PRD.md`.

## Invariants worth not breaking

- **Context assets refresh themselves.** The embedded `guide.md`/`instructions.md` are authoritative and `<home>/context/` is a projection of them (`Store.EnsureContextAssets` on every `wire()`). Refresh **never creates** the context directory: `team join` refuses a target holding anything but `config.yaml`. The embedded bytes reach core through the Store port, because `TestCorePackageIsPure` forbids `internal/core` importing `dossier/assets`.
- **Eval isolation.** Eval calls must keep hooks disabled (`disableAllHooks`), or an eval would fire Dossier's own SessionStart/SessionEnd and recurse.
- **Stats and recovery are derived, not stored.** Both are computed from the synced audit logs. Don't add a parallel store of truth.

## Workflow

- Branch off `main`. Open one focused PR per change and merge after CI is green on Linux, macOS and Windows.
- **Definition of done** (`CLAUDE.md`):
  - it compiles;
  - `go vet` and `gofmt` are clean;
  - tests pass;
  - the relevant SPEC §14 criteria are demonstrably met;
  - `ARCHITECTURE.md` is updated if structure changed;
  - this file's Status and What remains are updated when something lands.
- **Flag, don't diverge.** If the harness can't do what a doc assumes, stop. Record the finding in `docs/harness-capabilities.md`, record new decisions as an ADR plus a `BUILD-DECISIONS.md` row, then change course.
- **Docs to keep current:**
  - `ARCHITECTURE.md`, updated in the same PR as any structural change;
  - `docs/harness-capabilities.md` for verified harness behavior;
  - an ADR for each new decision;
  - `README.md` for user-facing changes;
  - `assets/guide.md` when dogfooding shows distillation gaps.

## Watchouts (hard rules; also in CLAUDE.md)

- The token limit is a warning threshold. Never cut content to fit it.
- Never silently link or merge ambiguous topics.
- Don't promise transcript capture universally; degrade visibly.
- No database or persistent topic graph.
- No native deletion. Archive only.
- No last-write-wins for the Distilled State. Concurrent edits become conflict artifacts.
- No logic in the CLI/MCP/TUI adapters. They are thin shims over one `core.Service`.
- Improve quality through guide iteration and dogfooding, not confirmation gates.

## Dogfood rhythm

1. **Resume drills.** Resume a topic in a different agent than the one that created it. Note what was missing or bloated.
2. **Failure drills.** Force an unavailable transcript, an over-target state, concurrent edits and a merge conflict. Check that the warnings read clearly.
3. **Weekly `dossier stats`.**
   - Compare versions on the unsaved rate and the eval recovery rate.
   - Use `--by model,effort` before blaming or crediting the guide.
   - Use `--by version,eval` if the eval setup changed.
4. **Tune `assets/guide.md` from what the drills and evals show.** Check each change with `tools/resumeeval` on fixed cases before shipping it.
