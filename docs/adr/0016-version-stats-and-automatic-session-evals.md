# ADR 0016: Session outcomes by version, with automatic session evals

## Status
Accepted (2026-10-07). **Implemented 2026-10-07** (`internal/core/session_eval.go`, `internal/core/stats.go`,
`internal/evaluator`, `internal/cli/eval.go`, the `dossier_stats` MCP tool, and the `eval:` config knob).

## Context
The product question is whether each release makes resumption better in real, everyday use.
`tools/resumeeval` answers that offline against curated cases, but cases have to be collected and
probes written by hand. Nothing recorded which binary or which guide a real session ran under:
`main.version` existed but was never persisted. The on-disk guide also takes precedence over the
embedded one when edited, so the version alone does not say which rules the agent followed.

## Decision

### 1. Stamp every session end with version and guide hash
At the true end of a bound session (the `session-end` hook, not `pre-compaction`), core appends a
`session_ended` audit event carrying:
- `version`: the release tag, or `dev+<rev>[-dirty]` from Go's VCS build info for unstamped builds,
  so dogfood builds don't all pool under `dev`;
- `guide_hash`: the first 12 hex characters of the sha256 of the Guide and Operating Instructions in force.

MCP saves now carry the calling harness session on their audit event, so saves per session are
derivable.

### 2. `dossier stats` / `dossier_stats`
These aggregate, from synced audit logs only, per (version, guide hash):
- sessions;
- **unsaved**: sessions with at least one boundary where `distilled_state_not_captured` fired;
- saves per session;
- eval coverage and score, by probe kind, with skip reasons and eval cost.

They default to your own sessions; `--all-authors` widens the scope. Sessions that ended before
this ADR have no `session_ended` event and are not counted.

### 3. Automatic session evals, behind a knob
When a session ends having saved something, the hook spawns `dossier eval run` detached and
returns at once. The eval makes three model calls:
1. extract up to 8 probes from the session's archived transcript (values, corrections, approvals,
   decisions, rejected options, assumptions, where work stood);
2. answer them from the **current Distilled State alone**;
3. judge the answers.

The result is a synced `session_eval` audit event (counts by kind, model, cost, revision evaluated,
version, guide hash). Probe text, answers and judge reasons stay machine-local in
`local/evals/<dossier>/<session>.json`. `local/` is gitignored by Team Sync.

```yaml
eval:
  enabled: true   # default on while tracking is established
  model: haiku
```

The knob gates inference only; session tracking and stats always run. Running `dossier eval run`
by hand works regardless of the knob.

### 4. Isolation and failure handling
- The evaluator adapter runs `claude -p` with hooks disabled, no MCP servers, no settings sources,
  CLAUDE.md files or memory, no tools, no session persistence, and an empty cwd and `DOSSIER_HOME`.
  An eval can therefore neither recurse into Dossier's own hooks nor read the user's configuration.
- Every outcome is recorded: a skip or a mid-pipeline failure becomes a `session_eval` event with
  `skipped` set, so coverage gaps show in stats with their cause.
- Transcripts over 400 KB are skipped, not truncated. A truncated transcript would score only the
  part that survived.

## Consequences
- Every saved session costs three small-model calls while the knob is on (about $0.05 and 80s of
  background time on haiku in the first end-to-end run). The default is on by product decision
  for the current rapid-iteration phase. It applies to every machine that doesn't set the knob,
  including teammates', until the default is flipped in `internal/config/config.go`.
- An eval scores the Distilled State as it stands when the eval runs, seconds after the session
  ends, which is the state the next session would see.
- The score depends on the judge model and on probes the model extracts itself. Compare versions
  by trend over many sessions, not single runs. Use `tools/resumeeval` for controlled A/B tests on
  fixed cases.
- Large sessions are a known coverage gap (the size skip). Stats show how often it happens.
- Audit events gain optional fields (`version`, `guide_hash`, `eval`). Older binaries ignore them.

## Alternatives considered
- **Store stats in a separate synced file.** Rejected: the audit log is already append-only,
  per-author and synced, so stats stay derived like everything else (no new truth to drift).
- **Run the eval inside the hook.** Rejected: it would hold Claude Code's exit for over a minute.
- **Evaluate unsaved sessions.** Rejected: they have nothing to score. The unsaved rate measures them.
- **Version alone, without a guide hash.** Rejected: an edited on-disk guide would be invisible.
