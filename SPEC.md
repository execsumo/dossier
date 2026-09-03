# Dossier — Technical Specification (v1)

> Codename: chainlink · Scope: objective-critical core · Local, single-user [Amended to local-first, optionally team-synced (B12/ADR 0005, 2026-07-15)]
> Date: 2026-06-14 · Sources: `PRD.md`, `PRFAQ.md`

---

## 1. Purpose

Build Dossier as a local, single-user (amended to local-first, optionally team-synced per B12) memory layer for long-running agent work. A Dossier is a durable topic with a curated Markdown **Distilled State**, a source-retaining **Archive** of captured artifacts, and an append-only audit log. v1 supports Claude Code through MCP, CLI/TUI, context files, and lifecycle hooks.

The product must optimize for agent-initiated use: when a supported agent session starts, the agent should see the user's Dossier library, understand capability limitations for that harness/session, and help the user continue or create a Dossier without forcing a separate CLI workflow.

---

## 2. Non-Negotiable Product Constraints

1. **Flat topics:** no graph, tree, hierarchy, or persistent cross-Dossier links. Related Dossiers are merged.
2. **Two layers:** Distilled State and Archive are separate. Distilled State carries critical state; Archive carries captured source.
3. **No database:** files are the source of truth. Indexes, if ever added, are derived caches only and not v1.
4. **No human gate for ordinary saves:** distillation writes commit without confirmation, governed by the Distillation Guide and hook cadence.
5. **Human disambiguation for ambiguous/destructive actions:** ambiguous link targets and merge conflicts must ask the user.
6. **Per-session active Dossier:** each agent session has zero or one active Dossier; there is no global active Dossier.
7. **No native deletion:** Dossier supports archive, not delete. Deletion is manual folder removal by the user.
8. **Text-first artifacts:** Markdown, JSON, and TXT are supported. Native binary attachment storage is out of scope.
9. **100k-token Distilled State target:** over-target recall is allowed with explicit warning; never silently truncate critical state.
10. **Visible degradation:** if a harness lacks hooks or transcript capture, say so at install and session start.

---

## 3. System Components

### 3.1 Single Binary

Ship one self-contained executable named `dossier`.

The binary provides:

- CLI commands.
- TUI views for list/select/merge where useful.
- MCP server over stdio.
- Hook installer/config generator.
- Context-file generator.
- File store read/write/search.
- Token estimation.
- Distillation Guide asset delivery.

Implementation language recommendation: Go or Rust. Choose based on the fastest path to reliable cross-platform single-binary distribution and MCP stdio support.

### 3.2 Local Store

Default root:

```text
~/.dossier/
```

The root can be overridden by:

```text
DOSSIER_HOME=/path/to/store
```

Store layout:

```text
~/.dossier/
  config.yaml
  context/
    library.md
    guide.md
  sessions/
    <session-binding-id>.json
  <slug>/
    dossier.md
    artifacts/
      <artifact-id>.md
      <artifact-id>.json
      <artifact-id>.txt
    files/
      <loose-file>
    conflicts/
      <conflict-id>.md
    audit/
      <author>.log
    sessions/
      <author>/
        <session-id>.md
    audit.log
```

`<slug>/artifacts/` is a **frontmatter-only namespace**: it is parsed, not scanned. A file without valid artifact frontmatter is skipped by the evidence index, carries no `art_` id to cite, and never enters the revision hash — while still surfacing in `dossier search`, so it reads as captured evidence while being none. `dossier doctor` reports such a file as an issue. `<slug>/files/` is the namespace for loose deliverables, scratch, and user attachments; promote one to evidence with `dossier link --from-file`.

`config.yaml` records install settings, user-configurable interface and lead vocabularies, global token warning ceiling (`token_limit`, defaulting to 100,000), detected harness capabilities, and optional team-sync settings. New installs include the legacy seven interface defaults; older configs that omit `interfaces` inherit those defaults. An empty `leads` list preserves free-form lead assignment. Readers also accept the retired `token_target` key from pre-simplification configs (mapping it to `token_limit`) and `schema_version` (ignoring its value), omitting them on the next normal config write. All other unknown config keys remain errors.

`context/library.md` is the generated open-work context file for harnesses without deterministic hooks.

`context/guide.md` is the installed Distillation Guide.

`sessions/<session-binding-id>.json` records per-session active Dossier bindings where the harness can provide or persist a session identifier.

---

## 4. Data Model

### 4.1 Dossier Frontmatter

Each `dossier.md` begins with YAML frontmatter:

```yaml
id: dos_01jz8example000000000000000
name: Pricing model refresh
description: Pricing strategy and packaging decisions for the next planning cycle.
slug: pricing-model-refresh
created_at: 2026-06-14T15:40:00-07:00
updated_at: 2026-06-14T16:10:00-07:00
status: active
lead: "Alice"
interfaces:
  - "Pricing WBR"
  - "Steerco"
next_action: "Compare revised pricing scenarios with sales feedback."
priority: high
due_date: 2026-06-21
```

Required fields:

- `id`
- `name`
- `slug`
- `created_at`
- `updated_at`
- `status`
- `next_action`
- `priority`

Optional fields:

- `description`
- `lead`
- `interfaces`
- `due_date`

The read schema is backward-compatible with the immediately preceding schema. It additionally accepts `last_touched_at`, `open_questions`, `importance`, `urgency`, and `token_target`, while continuing to reject every other unknown YAML key. When canonical `priority` is absent, the old matrix maps `high/high` → `max`, `high/low` → `high`, `low/high` → `medium`, and `low/low` → `low`; a missing or unknown legacy dimension uses the old normalize-toward-attention behavior and is treated as `high`. A present canonical `priority` takes precedence. Legacy `open_questions` are merged into the body's `## Open Questions` section without duplicates. Compatibility is lazy: reads do not rewrite files; the next ordinary Save writes canonical frontmatter only and preserves the prior bytes in revision history. Retired fields are never re-emitted.

`next_action` must be no more than 140 Unicode characters. The limit is enforced by the core service and does not truncate existing or submitted values.

Valid enums:

- `status`: `spark`, `define`, `delegated`, `review`, `blocked`, `done` (legacy statuses `active` → `define`, `waiting` → `delegated`, and `resolved`/`archived` → `done` are accepted on input and transparently normalized).
- `priority`: `low`, `medium`, `high`, `max`

Interface and lead values are configured in machine-local `config.yaml`. Interface assignments must use configured values. When `leads` is nonempty, new lead assignments must use a configured value (or empty to clear); when it is empty, lead assignment remains free-form. Removing a configured value does not invalidate existing Dossiers or unrelated edits. TUI selectors and MCP schemas read these lists at process startup.

`description` is a short progressive-disclosure summary shown in library/list
surfaces before loading the full Distilled State.

Recall uses a fixed 100,000-token warning ceiling for the estimated token count
of the Distilled State loaded during recall—not the current token count and not
a hard limit. Dossier never truncates the state to satisfy it. The current token
estimate is returned by recall but is not stored in the Dossier.

### 4.2 Distilled State Body

Open questions are ordinary Markdown, not frontmatter. When present, record
them under `## Open Questions` in the Distilled State body.

Required section order:

```markdown
# <Dossier Name>

## Situation

## Decisions

## Findings

## Open Questions

## Current State

## Next Steps
```

Rules:

- Preserve critical substance over brevity.
- Strip greetings, small talk, dead-end reasoning, tool-call mechanics, and redundant restatement.
- Every material claim must include provenance.
- Decisions must include what was decided, rationale, attribution, date if known, and provenance.
- Superseded claims should be replaced in the Distilled State; source remains in Archive/audit.

Recommended provenance syntax:

```markdown
- Sales prefers annual commits for enterprise renewals. [src:art_01jz8salesfeedback#L42-L68]
```

For artifacts without line support:

```markdown
- The original launch date was moved to July. [src:art_01jz8launchthread]
```

### 4.3 Artifact Metadata

Artifacts are Markdown, JSON, or TXT files under `artifacts/`.

Markdown artifact frontmatter:

```yaml
id: art_01jz8example000000000000000
dossier_id: dos_01jz8example000000000000000
type: source_snapshot
title: "Slack thread: pricing approval"
captured_at: 2026-06-14T16:00:00-07:00
refreshed_at: 2026-06-14T16:00:00-07:00
frozen: false
provenance:
  origin: "Slack"
  url: "https://example.slack.com/archives/..."
  captured_by: "Claude Code"
  harness: "claude-code"
content_format: markdown
source_size_bytes: 19420
```

Valid artifact types:

- `transcript`
- `source_snapshot`
- `file_snapshot`
- `link`
- `query`
- `decision_evidence`

Rules:

- No max artifact count per Dossier.
- Reject any single artifact over 1 GB.
- Native binary storage is unsupported. For binary-only or too-large files, store metadata, path/link, provenance, and any agent-provided textual extraction or summary.
- Markdown is preferred for documents and snapshots.
- JSON is acceptable for structured data.
- TXT is acceptable for transcripts and plain text.

### 4.4 Audit Log

Audit events are written to per-author shards in `audit/<author>.log` as append-only JSON Lines (the legacy `audit.log` remains readable but is never rewritten).

Example:

```json
{"ts":"2026-06-14T16:10:00-07:00","event":"save","dossier_id":"dos_...","actor":"agent:claude-code","author":"alice","session_id":"sess_...","before_revision":"rev_a","after_revision":"rev_b","artifacts_added":["art_..."],"token_estimate":8420}
```

Required event types:

- `create`
- `save`
- `promote`
- `link`
- `merge_started`
- `merge_completed`
- `merge_conflict`
- `status_changed`
- `archived`
- `snapshot_refreshed`
- `snapshot_frozen`
- `ambiguity_confirmed`
- `conflict_created`
- `conflict_resolved`
- `transcript_capture_unavailable`
- `install_warning`

Audit writes to per-author shards must be append-only; because they are single-writer, they do not conflict across machines.

---

## 5. Session Model

### 5.1 Active Dossier Binding

An active Dossier is bound to one agent session.

Binding fields:

```json
{
  "session_binding_id": "sess_01jz8example000000000000000",
  "harness": "claude-code",
  "dossier_id": "dos_01jz8example000000000000000",
  "bound_at": "2026-06-14T16:00:00-07:00",
  "last_seen_revision": "rev_01jz8example000000000000000",
  "capabilities": {
    "mcp": true,
    "session_start_hook": true,
    "session_end_hook": false,
    "pre_compaction_hook": false,
    "transcript_capture": false
  }
}
```

If a harness cannot expose a durable session id, the agent may maintain binding in context only. In that case, Dossier must still support `dossier_session(id?)` during the session, but deterministic recovery of the binding after restart is not guaranteed.

### 5.2 Switching

Switching active Dossier must:

1. Save the current active Dossier first where possible.
2. Clear the session binding.
3. Bind the new Dossier.
4. Return the new Distilled State, token estimate, and capability warnings.
5. Audit `status_changed` only if lifecycle status changes; switching alone is not a Dossier lifecycle change.

`/clear` removes the Dossier from session context but does not alter the Dossier on disk.

---

## 6. Harness Capabilities

v1 supports **Claude Code and Pi.** Claude Code provides the full capability set Dossier relies on:

- Session-start surfacing is deterministic.
- Session-end save is deterministic.
- Pre-compaction save is deterministic.
- MCP tools work.
- Raw transcript capture works.

**Pi does not provide that set** (verified against Pi 0.83.0; ADR 0005). Pi has no
built-in MCP client, and its `PI_SESSION_ID`/`PI_SESSION_FILE` reach bash-tool
children only. Dossier therefore installs its own Pi extension
(`assets/pi-extension.ts` → `<pi agent dir>/extensions/dossier/index.ts`) which
supplies **session identity** — a per-Pi-pid session pointer plus environment
mirroring — and nothing else yet. For Pi, session-start surfacing, session-end
save, pre-compaction save, and MCP are **unavailable** and must be reported as
such (§6.1); raw transcript capture is available as a file path only, through
the pointer's `session_file`.

Other harnesses (e.g. Codex, Antigravity) remain out of scope for v1.

### 6.1 Visible Degradation

Even within Claude Code, an expected capability may be unavailable in a given session (e.g. transcript access). When that happens, Dossier must degrade visibly, never silently: install and session-start notices must warn about the missing capability rather than skip it quietly.

### 6.2 Capability Discovery Task

Before implementation, verify for Claude Code:

- MCP server registration path.
- Slash command support.
- SessionStart hook support.
- session-end hook support, including `/clear` and `/exit` if available.
- pre-compaction hook support.
- raw transcript access.
- session identifier access.
- ability to inject context at session start.
- ability to show install/session-start notices.

The first implementation milestone should produce a capability matrix in `docs/harness-capabilities.md`.

---

## 7. CLI

### 7.1 Commands

```text
dossier init
dossier ls [--status spark|define|delegated|review|blocked|done|all] [--interface <interface>] [--json]
dossier show <slug-or-id> [--json]
dossier promote [--name <name>] [--from-file <path>] [--json]
dossier link [<slug-or-id>] [--from-file <path>] [--json]
dossier merge <source> <target> [--json]
dossier recall <slug-or-id> [--json]
dossier search <query> [--dossier <slug-or-id>] [--json]
dossier artifact <slug-or-id> [<artifact-id>] [-L <a-b>] [--json]
dossier sync [--status] [--json]
dossier team create <url> [--json]
dossier team join <url> [--json]
dossier status <slug-or-id> <spark|define|delegated|review|blocked|done>
dossier lead <slug-or-id> "<lead-name>"
dossier description <slug-or-id> "<summary>"
dossier interface <slug-or-id> "<interface>"...
dossier next <slug-or-id> "<next action>"
dossier priority <slug-or-id> --priority <low|medium|high|max> [--due <date>]
dossier active [--session <session-id>] [--json]
dossier switch <slug-or-id> [--session <session-id>] [--json]
dossier path [<slug-or-id>] [--json]
dossier archive <slug-or-id> [--json]
dossier done <slug-or-id> [--json]
dossier context refresh
dossier doctor
```

### 7.2 Command Behavior

`dossier init`

- Detects when running from a volatile/temporary path and offers to self-install to a stable PATH location (default `~/.local/bin/dossier`) first.
- Creates store directories.
- Writes default config.
- Installs Distillation Guide.
- Generates `context/library.md`.
- Detects configured harnesses where possible.
- Prompts for confirmation before updating user/global configurations to register the Dossier MCP server and lifecycle hooks, preserving existing third-party servers and hooks (never-clobber behavior). Writes each to the file Claude Code actually reads (hooks → `~/.claude/settings.json`, MCP → `~/.claude.json`) and idempotently migrates stale entries an older build wrote to the wrong file.
- Prints detected capabilities and warnings for Claude Code.
- Does not fail if a harness config cannot be updated or hooks are unsupported; warns visibly.

`dossier ls`

- Reads frontmatter across `*/dossier.md`.
- Default status filter: open work (`spark`, `define`, `delegated`, `review`, `blocked`).
- Sorts by priority (`max` first, then `high`, `medium`, `low`).
- Includes capability warning column if invoked inside a known harness/session.

`dossier promote`

- Creates a new Dossier from agent-provided content or `--from-file`.
- Accepts optional `--description` and `--priority low|medium|high|max`.
- If likely existing matches are found, returns top candidates and requires disambiguation unless `--name` and explicit create intent are provided.
- Captures transcript artifact if available.
- Warns if transcript capture is unavailable.

`dossier link`

- With id: attaches current session content to the target Dossier.
- Without id: returns top 3 candidates with confidence and asks for selection when ambiguous.
- Must not silently attach to a low-confidence match.

`dossier merge`

- Asks/records which Dossier is surviving target.
- Creates unified Archive.
- Produces converged Distilled State.
- Surfaces conflicts for human resolution.
- Does not silently reconcile contradictions.

`dossier recall`

- Returns full Distilled State.
- Estimates tokens.
- Warns if above the configured token limit (`token_limit` in `config.yaml`, default 100,000).
- Does not load Archive artifacts by default.

`dossier done` / `dossier archive`

- Sets status to `done`.
- Hides Dossier from default open-work view.
- Does not delete files.

`dossier sync` (Team Sync — implemented)

- Pulls, resolves, commits, and pushes to the configured team remote.
- With `--status`: reports unpushed commits, diverged remote, or stale credentials without modifying state.
- Supports `--json` output for MCP/TUI wrappers.
- Failure modes degrade visibly: network offline, expired/invalid auth, and oversized artifacts (>100 MB) return explicit surfaced warnings, never silent failures.

`dossier team create <url>` (Team Sync — implemented)

- Initializes and pushes the existing store to an empty private repo.
- Validates the target repo is empty.
- Writes `team.remote` to config.
- Supports `--json`.

`dossier team join <url>` (Team Sync — implemented)

- Clones the team repo into `DOSSIER_HOME`.
- Refuses to clobber a non-empty unsynced store (requires merge-adopt flow with confirmation).
- Confirms `author` identity.
- Prompts for and stores a GitHub PAT.
- Runs capability detect and hook install (existing `init` path).
- Supports `--json`.

`dossier doctor`

- Validates store integrity.
- Checks YAML frontmatter.
- Checks missing artifact links.
- Checks provenance references.
- Checks harness installation/capability status.
- Prints warnings and suggested fixes.

---

## 8. MCP Server

### 8.1 Tool Names

All MCP tools are prefixed with `dossier_`.

Required tools:

- `dossier_list` (supports `interfaces` filtering)
- `dossier_recall`
- `dossier_search`
- `dossier_artifact`
- `dossier_artifacts`
- `dossier_save`
- `dossier_promote`
- `dossier_link`
- `dossier_merge`
- `dossier_session`
- `dossier_update`

> **Note on `dossier_artifact`:** it takes `dossier_id` + `artifact_id`, and optionally either a `fragment` (a citation fragment such as `"L42-L68"`) or `start_line`/`end_line`. Content is returned with absolute 1-indexed line numbers, so the span read is the span cited. An unranged fetch returns the whole artifact and warns past 500 lines rather than truncating. `dossier_artifacts` returns the same evidence index that `dossier_recall` now carries in `artifacts[]`: one entry per archived artifact with its type, line count, and whether the Distilled State cites it.

> **Note on `dossier_update`:** it accepts `name`, `description`, `status`, `lead`, `interfaces`, `next_action`, and priority fields, and routes them all through the single `Save` write path (so CLI/MCP/TUI behave identically and edits get optimistic-concurrency handling). Open questions are edited by replacing the Markdown body. Changing `name` updates the **display name only** — the `slug` (and the on-disk directory) is the durable identifier and never changes on rename.

### 8.2 Tool Contracts

All tools return:

```json
{
  "ok": true,
  "data": {},
  "warnings": [],
  "next_actions": []
}
```

Errors return:

```json
{
  "ok": false,
  "error": {
    "code": "ambiguous_target",
    "message": "Multiple likely Dossiers match this session.",
    "details": {}
  },
  "warnings": [],
  "next_actions": []
}
```

Required error codes:

- `not_found`
- `ambiguous_target`
- `conflict_detected`
- `invalid_frontmatter`
- `artifact_too_large`
- `binary_artifact_unsupported`
- `transcript_unavailable`
- `over_token_target`
- `concurrent_edit`
- `harness_capability_unavailable`

### 8.3 `dossier_list`

Input:

```json
{
  "status": ["define", "delegated", "review", "blocked"],
  "limit": 50,
  "include_warnings": true
}
```

Output Dossier item:

```json
{
  "id": "dos_...",
  "name": "Pricing model refresh",
  "slug": "pricing-model-refresh",
  "status": "define",
  "lead": "Alice",
  "description": "Pricing strategy and packaging decisions for the next planning cycle.",
  "next_action": "Compare revised pricing scenarios with sales feedback.",
  "priority": "high",
  "due_date": "2026-06-21",
  "path": "/Users/me/.dossier/pricing-model-refresh",
  "warnings": ["Transcript archive unavailable in this session."]
}
```

### 8.4 `dossier_save`

Input:

```json
{
  "id": "dos_...",
  "base_revision": "rev_...",
  "frontmatter_updates": {
    "status": "define",
    "next_action": "...",
    "priority": "high"
  },
  "distilled_state_markdown": "...\n\n## Open Questions\n- ...",
  "artifacts": [
    {
      "type": "source_snapshot",
      "title": "Slack thread: pricing approval",
      "content_format": "markdown",
      "content": "...",
      "provenance": {
        "origin": "Slack",
        "url": "..."
      }
    }
  ]
}
```

Behavior:

- Validate frontmatter updates.
- Validate artifact sizes and formats.
- Check optimistic concurrency using `base_revision`.
- If concurrent edit is detected, create conflict artifact/draft and return `concurrent_edit`.
- Estimate tokens.
- Write `dossier.md` atomically.
- Append audit event.
- Return new revision and warnings.

### 8.5 `dossier_promote`

Input:

```json
{
  "name": "Pricing model refresh",
  "distilled_state_markdown": "...",
  "session_content": "...",
  "transcript": null,
  "harness": "claude-code"
}
```

Behavior:

- Run suggestion against existing Dossiers before creating.
- If high ambiguity exists, return `ambiguous_target` with candidates.
- Create Dossier if create intent is clear.
- Store `session_content` as artifact if useful and provided.
- Store `transcript` as a `transcript` artifact if available. When JSONL is compiled into a citable view, retain the byte-preserved raw input as a second `transcript` artifact and audit both; plain-text passthrough remains one artifact.
- Preserve unknown/non-text transcript content blocks visibly as raw JSON in the compiled view and emit a counted warning.
- Warn if transcript unavailable.

### 8.6 `dossier_link`

Input:

```json
{
  "id": "dos_...",
  "base_revision": "rev_...",
  "distilled_state_markdown": "...",
  "session_content": "..."
}
```

If `id` is omitted, return candidates:

```json
{
  "candidates": [
    {"id": "dos_...", "name": "Pricing model refresh", "confidence": 0.82, "reason": "Shared terms: pricing, sales, packaging"}
  ],
  "requires_selection": true
}
```

### 8.7 `dossier_merge`

Input:

```json
{
  "source_id": "dos_a",
  "target_id": "dos_b",
  "resolved_conflicts": []
}
```

Behavior:

- Target survives.
- Source folder is retained but marked merged/archived or moved under target audit metadata; do not delete source files in v1.
- Unified Archive contains all artifacts.
- Conflict detection covers contradictory decisions, divergent next actions, incompatible lifecycle states, and material claims with incompatible values.
- If conflicts exist and `resolved_conflicts` is empty, return `conflict_detected`.

---

## 9. Hooks And Context Injection

### 9.1 Session Start

Session start injection fires on **every** session regardless of whether that
session has anything to do with Dossier, so its unbound-session payload is
deliberately a single-line nudge, not a full listing — the heavy payload
(Distillation Guide, full Distilled State) is delivered by the MCP tool
calls themselves (`dossier_session`'s response), the moment an agent
actually enters a Dossier's context, not passively on every session start.

When supported, session start injection includes:

```markdown
# Dossier Library

Warning: Transcript archive is unavailable in this session.

3 open dossier(s): Alpha, Beta, Gamma. Use dossier_list for details, dossier_session to resume one, or dossier_promote for a new thread (it flags likely duplicates automatically). Guide: ~/.dossier/context/guide.md
```

If a session has an active binding, also inject the Distillation Guide and the active Dossier's Distilled State in full.

If no active binding exists, the one-line nudge above is the entire payload — no further instructions block. `dossier_promote`'s own similarity check (returned as `next_actions` on ambiguous matches) is what actually guides the agent's decision, not prose injected up front.

### 9.2 Session End And Pre-Compaction

When supported:

- Trigger `dossier_save` for the active Dossier.
- Include latest Distilled State from agent context.
- Capture transcript if available.
- Audit success/failure.

If unsupported:

- Distillation Guide instructs the agent to self-trigger a save at boundaries.
- Session-start notices must warn that deterministic save backstops are unavailable.

### 9.3 Context File Fallback

Generate:

```text
~/.dossier/context/library.md
```

This file contains:

- Capability warnings from last `dossier init` or `dossier doctor`.
- Open-work list.
- Instructions for loading a Dossier.
- Pointer to Distillation Guide.

`dossier context refresh` regenerates it.

---

## 10. Distillation Guide

The Distillation Guide is a shipped prompt asset stored at:

```text
~/.dossier/context/guide.md
```

It must instruct agents to:

- Keep the session's active Dossier current best-effort each turn.
- Save at session end, `/clear`, `/exit`, and before compaction where possible.
- Preserve material claims, decisions, rationale, attribution, constraints, current state, open questions, next action, and findings.
- Strip greetings, small talk, dead ends, tool mechanics, and redundant restatement.
- Add provenance to every material claim.
- Ask the user for ambiguous link targets and merge conflict resolution.
- Never silently truncate Distilled State to satisfy the token target.
- Warn when transcript capture or deterministic hooks are unavailable.

The guide should include examples of good and bad distillation.

---

## 11. Algorithms

### 11.1 Priority Sort

Default scoring:
Priority is a single human-set attention level. It is ordered from highest to lowest as `max`, `high`, `medium`, `low`.

Sort by priority, then earliest due date when present, then oldest `updated_at`.

The priority order is fixed; it is not configurable.

### 11.2 Suggestion Ranking

v1 uses lexical ranking.

Inputs:

- Current session content.
- Candidate Dossier name.
- Frontmatter fields.
- Distilled State.
- Artifact titles.

Scoring:

- Normalize lowercase.
- Remove stop words.
- Tokenize words.
- Weight exact name/slug matches highest.
- Weight `next_action` above ordinary body text.
- Search the full Distilled State body, including its `## Open Questions` section.
- Weight recently updated Dossiers slightly above older ones, but do not use recency as primary ranking.

Return top 3 candidates with confidence:

- `high`: strong exact or repeated domain match.
- `medium`: plausible overlap.
- `low`: weak overlap.

Only high confidence can be recommended as likely; never silently link.

### 11.3 Search

Use file scan, not database.

Preferred implementation:

- Native recursive text search in binary, or shell out to `ripgrep` if available.
- Search `dossier.md`, artifact `.md`, `.json`, `.txt`, and titles/frontmatter.
- Exclude unsupported binary files.

Return:

- Dossier id/name.
- Artifact id/title if artifact match.
- Path.
- Snippet.
- Line reference where available.

### 11.4 Token Estimation

Use one tokenizer benchmarked against Opus 4.8 as reference. Precision per target model is not required.

Behavior:

- Estimate Distilled State tokens on recall/save.
- Default target: 100,000 tokens.
- If estimate exceeds target, return warning with recommended next actions.
- Do not fail recall solely due to target overflow.
- Do not silently truncate.

### 11.5 Optimistic Concurrency

Each save includes `base_revision`.

Revision can be a hash of canonical `dossier.md` content plus artifact manifest.

On save:

1. Read current revision.
2. If current revision equals `base_revision`, write atomically.
3. If current revision differs:
   - If only non-overlapping frontmatter changed, auto-merge and audit.
   - Otherwise create `conflicts/<conflict-id>.md`.
   - Return `concurrent_edit`.
   - Preserve proposed write in conflict file.

No last-write-wins for Distilled State.

---

## 12. File Operations

### 12.1 Atomic Writes

For `dossier.md`:

1. Read current content.
2. Validate concurrency.
3. Write temp file in same directory.
4. fsync temp file where practical.
5. Rename temp file over target.
6. Append audit event.

For artifacts:

- Generate id first.
- Write artifact file atomically.
- Append audit event after successful write.

### 12.2 Slug Generation

Slug rules:

- Lowercase.
- ASCII preferred.
- Replace spaces and punctuation with `-`.
- Collapse repeated `-`.
- Trim leading/trailing `-`.
- If conflict, append short id suffix.

### 12.3 ID Generation

Use sortable unique ids with prefixes:

- `dos_`
- `art_`
- `sess_`
- `rev_`
- `conf_`

ULID-style ids are acceptable.

---

## 13. Install And Doctor

### 13.1 `dossier init`

Must:

- Create `~/.dossier`.
- Create `context/`.
- Create `sessions/`.
- Write `config.yaml`.
- Write Distillation Guide.
- Generate library context file.
- Detect Claude Code config availability where possible.
- Attempt to install/register MCP and hooks only where supported and safe.
- Print detected capabilities and warnings.

Example output:

```text
Dossier initialized at ~/.dossier

Claude Code integration:
- detected
- MCP: available
- Session-start hook: available
- Session-end hook: available
- Pre-compaction hook: available
- Transcript capture: available
```

### 13.2 `dossier doctor`

Checks:

- Store exists.
- Config parses.
- Dossier frontmatter parses.
- Required sections exist.
- Artifact references resolve.
- Files in `<slug>/artifacts/` are artifacts (a file without valid frontmatter is uncitable and absent from the evidence index — reported as an issue, not an advisory).
- Provenance links resolve.
- Audit log is parseable JSONL.
- Context files are current.
- Harness registrations exist.
- Capability warnings are current.

---

## 14. Acceptance Criteria

### 14.1 Core Storage

- Creating a Dossier writes `dossier.md`, `artifacts/`, and `audit/<author>.log`.
- `dossier.md` is readable as plain Markdown.
- Frontmatter scan over 500 Dossiers completes under 2 seconds on a typical laptop.
- No database file is created.
- Pre-simplification config and Dossier files load without an eager rewrite; unknown YAML keys still fail.
- The next normal Save emits canonical frontmatter, preserves legacy open questions in the body, and keeps the original legacy file readable through revision history.

### 14.2 Recall

- `dossier_recall` returns full Distilled State, plus the evidence index of archived artifacts.
- A `[src:art_x#L<a>-L<b>]` citation resolves through `dossier_artifact` to the verbatim lines it names; `doctor` reports a citation whose range does not fit its artifact.
- Recall includes token estimate.
- Over-target Distilled State returns warning and does not truncate.
- Archive artifacts are not loaded by default.

### 14.3 Provenance

- Sample generated Dossiers include provenance on every material claim.
- `dossier doctor` reports missing provenance.
- Provenance links resolve to artifacts or line ranges where available.
- A working file written into `<slug>/artifacts/` without artifact frontmatter is reported by `dossier doctor` rather than silently omitted from the evidence index; `<slug>/files/` holds loose work and is not reported.

### 14.4 Promote/Link

- Promote creates a new Dossier from agent-provided content.
- Promote warns when transcript capture is unavailable.
- Link without id returns candidates when matches exist.
- Ambiguous link requires user/agent selection.
- No low-confidence candidate is silently linked.

### 14.5 Merge

- Merge requires surviving target.
- Contradictory decisions produce conflict.
- Conflict is preserved in `conflicts/`.
- Source Dossier files are not deleted.
- Merge audit events are written.

### 14.6 Active Session

- Each session can bind zero or one active Dossier.
- Two sessions can bind different Dossiers.
- `dossier_session` reports or changes session binding and returns new Distilled State when switching.
- `/clear` removes session context but does not alter Dossier files.

### 14.7 Harness Transparency

- `dossier init` reports detected Claude Code capabilities.
- Session-start context reports missing hooks/transcript capture.
- recall/save/search remain available through the MCP/context-file path even when hooks are unavailable.

### 14.8 Concurrency

- Concurrent Distilled State edit creates conflict artifact instead of overwriting.
- Non-overlapping frontmatter update can auto-merge.
- Audit log records conflict creation and resolution.

### 14.9 Archive/Delete

- `dossier archive` hides Dossier from default open-work view.
- Archived Dossier remains searchable.
- No CLI/MCP delete command exists.

### 14.10 Team Sync

> Status (2026-07-16): implemented and integrated; all criteria are covered by automated tests against local bare repos (internal/sync, internal/cli) EXCEPT the "exactly two commands and one sign-in" PAT onboarding, which remains pending the Phase 4 real-GitHub pilot.
- Two stores converge through one remote.
- Concurrent `dossier.md` edit yields exactly one `conflicts/*.md` (`kind: sync_concurrent_edit`) on the later syncer with no content lost anywhere.
- Save never blocks on network (offline save succeeds, push retries later with a visible warning).
- Machine-local files (`config.yaml`, credentials, root `sessions/`, `context/`) never appear in the remote.
- >100 MB artifacts excluded from sync with a persistent visible warning.
- `team join` onboarding completes with exactly two commands and one sign-in.

---

## 15. Implementation Milestones

### Milestone 1 — Harness Discovery And Skeleton

- Choose Go or Rust.
- Implement config/store root.
- Produce `docs/harness-capabilities.md`.
- Stub CLI and MCP server.
- Implement `dossier init` and `dossier doctor` baseline.

### Milestone 2 — File Store And Core CLI

- Implement Dossier create/read/update.
- Implement frontmatter parsing.
- Implement artifact write/read.
- Implement audit JSONL.
- Implement list, show, path, archive.

### Milestone 3 — Recall, Search, Token Warnings

- Implement recall.
- Implement token estimation.
- Implement over-target warning.
- Implement full-text search.
- Implement generated context library.

### Milestone 4 — MCP Tools

- Implement MCP stdio server.
- Implement required MCP tools.
- Validate tool contracts against at least one harness.

### Milestone 5 — Promote, Link, Suggestions

- Implement promote.
- Implement lexical suggestion engine.
- Implement link flow and ambiguity handling.
- Implement transcript warning behavior.

### Milestone 6 — Active Session And Hooks

- Implement session binding.
- Implement switch/active.
- Install hooks for Claude Code.
- Add session-start library injection.
- Add session-end/pre-compaction save where supported.

### Milestone 7 — Merge And Concurrency

- Implement optimistic concurrency.
- Implement conflict artifacts.
- Implement merge target selection and conflict reporting.
- Implement merge audit events.

### Milestone 8 — Distillation Guide And Dogfood

- Ship initial Distillation Guide with examples.
- Dogfood across real Claude Code sessions.
- Measure success metrics from PRD.
- Tighten guide and harness warnings.

---

## 16. Open Implementation Questions

These are not product blockers; they are discovery tasks for build planning.

1. Which v1 language, Go or Rust, gives the fastest reliable MCP + TUI + cross-platform binary path?
2. For Claude Code, what exact config files and hooks are available?
3. Can Claude Code expose raw transcripts safely and deterministically?
4. Can Claude Code provide a stable session id?
5. What is the safest install behavior for modifying Claude Code's config?
6. Should hook installation require explicit confirmation in `dossier init`?
7. Which tokenizer library best approximates Opus 4.8 while staying lightweight?
8. Should `ripgrep` be a soft dependency, or should v1 implement native search to preserve single-binary purity?
9. What exact Markdown provenance syntax is easiest for agents to maintain and for `doctor` to validate?
10. How much TUI is needed in v1 versus plain CLI plus MCP responses?

---

## 17. Partnering Plan

The tooling will become effective fastest through dogfood loops rather than abstract polish.

Recommended partnership rhythm:

1. **Harness reality pass:** use your actual Claude Code setup to map hooks, transcript access, config files, and session ids.
2. **Dossier seed set:** create 10-20 real Dossiers from your current work, including messy ones, clean ones, blocked ones, and resolved ones.
3. **Distillation review sessions:** compare generated Distilled State against your memory of the work; tune the Distillation Guide rather than adding confirmation gates.
4. **Resume drills:** intentionally resume topics in a different Claude Code session than the one that created them; record what was missing or bloated.
5. **Ambiguity drills:** test promote/link against similar topics to tune suggestion confidence and wording.
6. **Failure drills:** force unavailable transcript, over-target Distilled State, concurrent edits, and merge conflicts; verify warnings feel clear rather than noisy.
7. **Weekly metric check:** track cross-session resume success, provenance misses, over-target warnings, and time-to-find-next-topic.

The important collaboration mode: you provide real topic workflows and judgment about whether the agent resumed with the right state; the implementation should make every hidden limitation explicit and every recovery path easy.
