# Dossier — Product Requirements Document (v1)

> Codename: chainlink · Scope: objective-critical core · Local, single-user
> Date: 2026-06-14 · Companion: see `PRFAQ.md`

---

## 0. Frontloaded decisions (read this first)

These are settled. They are placed up front because they constrain everything downstream; changing one is a re-architecture, not a tweak.

| # | Decision | Rationale | What it rules out |
|---|----------|-----------|-------------------|
| **D1** | A **flat set of distinct Dossiers**. Artifacts belong to a Dossier. No topic graph/tree/cross-links. | A topic is self-contained; extra material is *artifacts of one topic*, not multiple topics. | Inter-topic link graph, nesting, topic hierarchies. |
| **D2** | **Two layers** per Dossier: curated **Distilled State** + source-retaining **Archive** of captured artifacts. Provenance links connect distilled claims → source artifacts. | The Distilled State holds *all critical information with noise removed* — not a short summary; "be citable" and "carry the full substance of the topic" can't both live in the raw transcript. | A single evolving doc; lossy summaries that discard substance or sources. |
| **D3** | Access via **MCP** (auto-surfaces available Dossiers on agent load) **+ CLI/TUI**. **Local, single-user.** v1 supports **Claude Code and Pi** (§5.5); Pi requires a compatible Claude-like hooks extension. | Meet the agent where it lives; degrade gracefully. | Cloud dependency, web app, account system (v1); other harnesses (Codex, Antigravity). |
| **D4** | Distillation runs **without a human gate**, but it is **governed, not ad hoc**. *What* to retain is steered by a shipped **Distillation Guide** (a skill/instructions the agent loads). *When* to write is **deterministic** — hook-driven cadence + triggers (§4.11), never "the agent remembers to." | A confirm step adds friction; but "agent freely decides whether and what to write" is too loose. Steer content quality up front, enforce update cadence mechanically. Trust on content comes from **non-destruction** + the guide, not a gate. | A blocking human-confirm; relying on the agent's discretion to update. |
| **D5** | Relatedness is resolved by **merge**, producing **one converged Distilled State**; **conflicts and ambiguous targets are surfaced to the human**. | Matches D1 (no persistent links); keeps one source of truth per topic. | Auto-merge that silently reconciles; permanent dossier-to-dossier references. |
| **D6** | Dossier **stores** artifact content provided by the agent/user; it does **not fetch from external sources itself**. The agent (assumed to have its own integrations) fetches **on request**; snapshots are **refreshed while active** and **frozen on resolution**. | Sourcing is the agent's/user's job, not this app's. Accuracy during active work; stable citation after. | The app owning source integrations; live links only (rot); a snapshot that goes stale mid-thread. |
| **D7** | **100k-token target for Distilled State context.** Over-target recall is allowed with an explicit warning; Archive is retrieved on demand. | Predictable, bloat-aware resumes without blocking progress. | Loading whole transcripts/archives into context by default; silently truncating critical state. |
| **D8** | Every Dossier carries **lifecycle status + next action + open questions + staleness**. | "See what's open and needs to progress" is the daily-driver surface. | Treating topics as an undifferentiated list. |
| **D9** | **One plain Markdown file is the human-readable source of truth for each Dossier's distilled state**: YAML **frontmatter** for lifecycle fields, body for the distilled critical information. Each Dossier also has an artifact folder and audit log. **No database.** | Files are inspectable in any Markdown reader (e.g. Obsidian) with no special tool; frontmatter is the natural, Obsidian-native home for status/next-action. Listing and search are file scans (frontmatter read + `ripgrep`). | A SQLite/derived index; a proprietary store; metadata locked away from the user's own tools. |

**Deferred (explicitly not v1):** sharing/multi-user, web app, in-app LLM wrapper, automated ingestion integrations (Slack/email/Drive OAuth), binary attachment storage, a database/index layer, semantic/embedding search beyond fast-follow.

---

## 1. Problem & objective

Technically-savvy business users drive many topics through CLI coding agents. Serious topics span days and multiple sessions; `/resume` mixes durable work with throwaway chatter, bloats context with dead ends, and breaks entirely when switching agents. The `handoff.md` pattern shows the demand for durable, portable context — but hand-maintaining one file per topic across ~20 topics/day doesn't scale.

**Objective:** Let a user carry a topic across Claude Code sessions, resuming with the *distilled, citable state of the work* under a clear token target, with the raw material retained and one search away.

**Non-goals (v1):** collaboration, hosted UI, in-app chat, automatic content ingestion, native binary attachment management.

---

## 2. Users & primary jobs

Single user (the operator). Core jobs-to-be-done:

1. Promote a live session into a durable topic.
2. See all open topics and what each needs next.
3. Resume a topic in any agent with the right context, not the noise.
4. Connect an ad-hoc session to an existing topic — including in hindsight.
5. Merge two topics that turn out to be one.
6. Cite a decision back to the source that justifies it.

---

## 3. Core concepts & data model

### 3.1 Dossier
A distinct topic of work. Its distilled state lives in **one Markdown file** (D9), with supporting artifacts and audit history beside it. Composed of:

- **Frontmatter (YAML):** identity (`id`, `name`, `slug`, `created_at`, `updated_at`, `last_touched_at`) and lifecycle (D8):
  - `status ∈ {active, blocked, waiting, resolved, archived}`
  - `lead` (string)
  - `next_action` (string), `open_questions` (list)
  - `importance ∈ {high, low}`, `urgency ∈ {high, low}`, `due_date` (ISO date, optional)
  - `staleness` is derived from `last_touched_at`, not stored.

  These prioritization fields feed surfacing (§4.1). Frontmatter is what the open-work view scans and what Obsidian-style readers render natively.
- **Body — Distilled State (D2):** the topic's **critical information with noise removed** (not a chat recap). Sections: Situation, Decisions, Findings, Current State, Next Steps. The agent keeps everything that informs the topic and strips niceties, small talk, and dead ends. The Distilled State has a **100k-token target** for recall ergonomics; over-target state is allowed but must warn (see §6).
- **Archive (D2):** the Dossier's `artifacts/` directory of **Artifacts**.
- **Audit log:** append-only `audit.log` — writes, merges, snapshot refreshes/freezes (also the provenance backbone).

### 3.2 Artifact
A unit of supporting material under one Dossier.

- `id`, `type ∈ {transcript, source_snapshot, file_snapshot, link, query, decision_evidence}`, `title`
- `captured_at`, `refreshed_at`, `frozen` (bool — set true on Dossier resolution, D6)
- `provenance` (origin description / URL / session ref)
- `content` (inline) or `path` (stored text/Markdown/JSON)
- v1 stores text-first artifacts only. Markdown is preferred for human-readable docs; JSON is acceptable for structured snapshots. There is no max artifact count per Dossier in v1. Native binary attachments are out of scope; if a source file is binary-only, store metadata + path/link/provenance, not the file itself. Any artifact larger than 1 GB is rejected with an explicit message.

### 3.3 Provenance link
A reference from a material claim in Distilled State to the Artifact(s) that justify it. Required on every recorded Decision and every other material claim that could reasonably be challenged or need to be cited later. This is the citation mechanism — a property of the model, not a feature.

### 3.4 Active Dossier binding
An **active Dossier** is bound to one agent session, not globally.

- Multiple agent sessions may work on different Dossiers at the same time, even in the same harness.
- A session may have zero or one active Dossier.
- The user can switch the active Dossier via `/dossier`, natural conversation ("switch this session to the pricing Dossier"), `/clear` followed by a new selection, or a new agent session.
- Clearing a session removes the Dossier from that session's context only; it does not alter the Dossier on disk.
- The user can ask for a link/path to the active Dossier's directory at any time.
- The user can ask the agent to archive a Dossier. Native deletion is intentionally unsupported; if the user wants deletion, they delete the Dossier folder directly.

### 3.5 Lifecycle status semantics

- `active`: work is ongoing and may need the user or agent to progress.
- `waiting`: progress depends on an external event, person, or date; still appears in the open-work view.
- `blocked`: progress is stuck because a specific blocker must be resolved; still appears in the open-work view and should rank above ordinary waiting work when urgency/importance are equal.
- `resolved`: the topic reached its intended conclusion. It is hidden from the default open-work view, snapshots are frozen, and recall remains available.
- `archived`: the topic is no longer operationally relevant. It is hidden from default views but remains searchable and recoverable. Archiving does not delete source material.

### 3.6 Storage layout (D9 — plain files, no database)
Local-first. One directory per Dossier; no index, no DB.

```
~/.dossier/
  <slug>/
    dossier.md             # frontmatter (lifecycle) + body (Distilled State)
    artifacts/
      <artifact-id>.{md,json,txt}
    audit.log              # append-only, plain text
```

- `dossier.md` is the source of truth for the distilled working state — inspectable and editable in any Markdown reader (e.g. Obsidian), diff-able, git-friendly. Artifacts and audit log sit beside it as supporting source material.
- **Listing** (open-work view) = read frontmatter across `*/dossier.md`. **Search** = `ripgrep` over the files. At the expected scale (hundreds of Dossiers) this is fast; if it ever isn't, an index can be added later as a pure derived cache without changing the source of truth.

---

## 4. Functional requirements

### 4.1 Surface-on-load (D3) — *must populate available Dossiers when the agent starts*
- **Deterministic where the harness allows it.** Surfacing should not depend on the agent remembering to call a tool. A **SessionStart hook** (§5.4) injects the open-work list into context on every supported hook-capable session start — read from frontmatter across `*/dossier.md` (D9), no index. Harnesses without hooks fall back to the generated context file and MCP/manual refresh.
- The `dossier_list` MCP tool still exists for on-demand refresh within a session, but the *guarantee* of surfacing comes from the hook, not the tool.
- Payload per Dossier: `name`, `status`, `lead`, `next_action`, top `open_questions`, `importance`, `urgency`, `due_date`, `staleness`, `path`, and any harness capability warnings (for example: "transcript archive unavailable in this session").
- CLI/TUI `dossier ls` shows the same, sortable/filterable by any of these.
- **Open-work view:** default filter = `status ∈ {active, blocked, waiting}`. This is the daily driver.
- **Surfacing order:** rank by a priority signal, not raw recency — `urgency × importance` (Eisenhower-style), with `due_date` proximity *escalating* effective urgency (overdue → top), and `staleness` as the tiebreaker. So an overdue high-importance Dossier surfaces above a fresh low-priority one. Weights are configurable; the default puts overdue-or-due-soon + high-importance at the top.

### 4.2 Resume / recall (D7)
- `dossier_recall(id)` returns the full Distilled State. It targets **100k tokens** for the Distilled State context. If the Distilled State is over target, recall still succeeds but returns an explicit warning and recommended next steps (split, archive resolved material, or reorganize with the agent). Archive artifacts are not loaded by default; they are retrieved on demand.
- `dossier_search(query, dossier_id?)` retrieves specific artifacts on demand (scope to a dossier for archive-style retrieval).

### 4.3 Promote (job 1)
- `dossier_promote` creates a new Dossier from the current session: the agent passes content for the initial Distilled State and writes it per the Distillation Guide (D4/§4.11 — no confirm gate).
- Dossier should also deterministically capture the raw session transcript into the Archive where the harness makes a transcript available. If transcript capture is unavailable, Dossier must say so explicitly during installation and again in the session-start Dossier library/loading notice.

### 4.4 Save / update (D4)
- `dossier_save(id)` has the agent pass updated Distilled State content + any new artifacts — **no confirmation step**, but following the **Distillation Guide** and on a **deterministic cadence** (§4.11), not at the agent's whim. Updates `last_touched_at`, `status`, `next_action`. **Never deletes** source; superseded content is retained in Archive/audit, so any distillation choice is recoverable and editable after the fact.

### 4.5 Link in hindsight (job 4)
- `dossier_link(id?)` attaches the current session to an existing Dossier. If `id` omitted, the **suggestion engine** proposes likely matches (§4.8). Recommended v1 behavior: show the top 3 candidates with confidence and require the user/agent to choose one when confidence is not clearly high. If there is a single high-confidence match, the agent may recommend it, but should still make the target clear before attaching. Attachment runs the governed save flow (§4.4), reconciling the session into the target's Distilled State.

### 4.6 Merge (D5, job 5)
- `dossier_merge(a, b)` produces **one converged Distilled State** and a unified Archive. The agent/Dossier must clearly ask which Dossier should be the surviving target, then surface conflicts to the human (e.g., contradictory decisions, divergent next actions) for resolution — no silent auto-merge. Audit log records the merge.

### 4.7 Snapshots (D6)
- Dossier does **not fetch external content itself**. The agent (assumed to have its own integrations — Slack, email, web, etc.) or the user supplies the content; Dossier **stores** it as a `source_snapshot` artifact with its provenance at attach time.
- While the Dossier is `active`, a snapshot can be **refreshed on request** — the agent re-fetches and re-saves; Dossier just persists the new content. (Dossier never polls or fetches on its own.)
- On `resolved`, snapshots are **frozen** (`frozen = true`); refresh disabled to preserve citation stability.

### 4.8 Search & suggestion engine
- **Search:** full-text via `ripgrep` over `dossier.md` files + artifact content/titles, v1 (no DB, D9). Semantic/embedding search is a fast-follow if file scan proves insufficient.
- **Suggestion:** for `promote` vs `link`, rank existing Dossiers against the current session (term overlap from the file scan v1; embeddings later) and propose the top candidates with a confidence signal. Recommendation: `link` without an id is a single command that may open an interactive picker; `promote` should show likely existing matches before creating a duplicate, but should not auto-link silently.

### 4.9 Lifecycle management (D8)
- Commands/tools to set `status`, edit `next_action` and `open_questions`, and set `importance`, `urgency`, `due_date`.
- `staleness` derived and shown; surfaces stalled topics in the open-work view.

### 4.10 Zero-friction capture
- Promote / link / switch must each be a **single command** (CLI) or tool call (MCP) — no multi-step navigation. This is a hard UX constraint given ~20 topics/day. A single command may include one inline disambiguation step when the action is ambiguous; that is preferable to silent wrong attachment.

### 4.11 Distillation governance (D4) — *what* to keep and *when* to write
The agent is steered up front and updates are enforced mechanically, so distillation is never left to discretion.

**(a) Distillation Guide (the *what*).** A first-class, rigorously developed artifact shipped with Dossier — a skill/instructions file the agent loads (surfaced by the SessionStart hook, §5.4). It defines, with examples:
- **Keep:** decisions + their rationale + attribution (who/what decided), current state, open questions, next action, experiment results and findings, hard constraints, key data/figures, and provenance links to the artifacts that justify each claim.
- **Strip:** greetings/niceties/small talk, reasoning that led nowhere, tool-call mechanics, redundant restatement, and anything not informing the topic's current state or future moves.
- **How:** preserve substance over brevity while respecting the 100k-token target as a warning threshold (D2/§6); write in the Dossier's section structure (Situation, Decisions, Findings, Current State, Next Steps); every material claim carries a provenance link; updating supersedes prose in-place while the superseded content remains in the Archive.

This guide is a prompt asset to iterate on like code — its quality is a primary driver of the product (see Risks, §9).

**(b) Update cadence (the *when*).** Two layers:
- **Best-effort every turn** — the Distillation Guide instructs the agent to keep the session's active Dossier current as the conversation progresses. No turn-counter hook; per-turn best-effort is sufficient and avoids over-engineering.
- **Deterministic backstops (hooks, §5.4)** — a final `dossier_save` is forced on **session end** (including `/clear` and `/exit`) and **before context compaction**, so nothing is lost even if the per-turn effort lapses. This is the hard guarantee.
- **Portable fallback:** in harnesses without hooks, the Distillation Guide instructs the agent to self-trigger at those same boundary points; the deterministic guarantee applies where hooks exist (e.g. Claude Code).

---

## 5. Interfaces

### 5.1 MCP tools (backbone)
All tools are namespaced with a **`dossier_` prefix** so they're unambiguously identifiable as Dossier's amid other MCP servers in a harness.

- `dossier_list` — open-work list for in-session refresh (deterministic surfacing is the hook's job, §4.1)
- `dossier_recall(id)` — token-targeted resume, returns the filesystem path of the dossier (§4.2)
- `dossier_search(query, dossier_id?)`
- `dossier_save(id)` — governed write, no gate (§4.4, §4.11)
- `dossier_promote()` — promote current session
- `dossier_link(id?)` — with suggestions
- `dossier_merge(a, b)` — conflicts surfaced
- `dossier_session(id?)` — inspect or change the Dossier bound to the current agent session
- `dossier_update(id, name?, status?, lead?, next_action?, open_questions?, importance?, urgency?, due_date?)` — update any metadata or frontmatter fields in one call

Write tools **commit without a human gate** (D4), but writes are governed by the Distillation Guide and fire on a deterministic cadence (§4.11) — not at the agent's discretion. Safety is structural: nothing is deleted, the audit log records every write, and the user can edit any Distilled State after the fact. Exceptions: ambiguous link targets and merge conflict resolution require human disambiguation; that's contradiction/target resolution, not a distillation review gate.

### 5.2 CLI/TUI (universal fallback + primary local UX)
- `dossier init`, `dossier ls` (open-work view default), `dossier show <slug>`
- `dossier promote`, `dossier link [<slug>]`, `dossier merge <a> <b>`
- `dossier recall <slug>`, `dossier search <query>`
- `dossier status <slug> <state>`, `dossier next <slug> "<action>"`
- `dossier active`, `dossier switch <slug>`, `dossier path [<slug>]`, `dossier archive <slug>`
- `dossier priority <slug> --importance <h|m|l> --urgency <h|m|l> --due <date>`
- Generates/refreshes a **context file** per Dossier for harnesses without MCP.

### 5.3 Slash command (in-session)
- **`/dossier`** — lists Dossiers grouped by `status`, in surfacing order (§4.1), without leaving the agent session. Optional args: `/dossier active`, `/dossier blocked`, etc. to filter to one status. It also supports selecting/switching the session's active Dossier from the list. Thin wrapper over `dossier_list` + `dossier_session`; this is the quick "what's on my plate right now" view mid-conversation.

### 5.4 Hooks (deterministic behavior)
Some behavior must not depend on the agent choosing to act; hooks make it deterministic where the harness supports them (e.g. Claude Code), with the context-file/skill fallback elsewhere.

- **SessionStart hook** → injects the open-work list (§4.1), capability warnings, and a pointer to the **Distillation Guide**. If a session already has an active Dossier binding, it also injects that Dossier's current Distilled State. If not, the agent is instructed to help the user choose or continue without an active Dossier.
- **Session-end + pre-compaction hooks** → force a final `dossier_save` against the session's active Dossier per the Guide (§4.11). **Session end includes `/clear` and `/exit`.** These are the hard backstops behind the per-turn best-effort updates — no separate turn-counter hook.
- **Transcript capture hook/capability** → when the harness exposes transcripts, Dossier captures the raw transcript as an Archive artifact. When unavailable, installation and session-start notices must explicitly say transcript archiving is unavailable for that harness/session.

Hooks ship as part of `dossier init` (it installs/updates the harness hook config). The MCP tools remain available for explicit, on-demand use; the hooks provide the *guarantee*.

### 5.5 v1 harness support

v1 supports **Claude Code and Pi.** Claude Code provides the full capability set: hooks + MCP + transcript capture — full deterministic surfacing, deterministic save backstops, MCP tools, and transcript archive capture. Pi provides the same contract through a compatible Claude-like hooks extension.

Pi is supported through a compatible Claude-like hooks extension that provides deterministic lifecycle calls, session identity, and transcript access. Other harnesses (e.g. Codex, Antigravity) remain out of scope for v1 because they do not currently provide the required capability contract.

Even within Claude Code, if an expected capability is unavailable in a given session (e.g. transcript access), product behavior must degrade visibly, never silently — Dossier warns at install and session start.

---

## 6. Token target (D7)

The token target governs the **Distilled State context loaded on recall**. The default target is **100k tokens**. This is a guideline and warning threshold, not a hard failure condition. The agent keeps *all critical information* and strips only noise (niceties, small talk, dead ends); it should not silently drop material merely to satisfy the target.

| Component | Budget | On overflow |
|-----------|--------|-------------|
| Distilled State | target 100k tokens, loads in full by default | Return an explicit warning and ask the agent/user to decide whether to proceed, reorganize, archive resolved material, split the topic, or remove non-critical material. |
| Archive artifacts on recall | not loaded by default | Retrieve specific artifacts via `dossier_search(query, dossier_id)`; avoid carrying raw artifacts into context unless requested or directly needed. |

**Tokenizer:** use a single reasonable tokenizer benchmarked against **Opus 4.8** as the reference; precision per target model is not required. Caveat this in the README. The 100k figure is configurable.

---

## 7. Non-functional requirements

- **Local-first / single-user:** all data on the user's machine; no network dependency for core flows.
- **Inspectable & recoverable:** plain Markdown files are the source of truth; readable/editable in any Markdown reader (e.g. Obsidian) with no special tool and no database (D9).
- **Non-destructive:** no flow deletes source material; supersession is via Archive + audit log. This is the primary trust mechanism in place of a distillation confirm gate (D4).
- **Auditable:** every write, ambiguity confirmation, merge, archive, and freeze recorded in `audit.log` — also the provenance backbone.
- **Harness:** v1 supports Claude Code and Pi (§5.5). Claude Code provides the full capability set; Pi requires a compatible Claude-like hooks extension. If a capability is missing in a given session, Dossier warns rather than silently degrading.
- **Concurrency (light, v1):** single user but multiple agent sessions may touch one Dossier. Recommended v1 behavior: optimistic concurrency with a base revision/hash recorded when a session recalls or switches to a Dossier. On save, if the on-disk revision changed, do not blindly overwrite; create a conflict artifact/draft, surface the conflict to the agent/user, and require reconciliation. If only non-overlapping frontmatter changed, auto-merge and audit it. Last-write-wins is acceptable only for append-only audit entries and new artifacts.

---

## 8. Success metrics

1. **Cross-session resume:** user resumes a real topic in a *different* Claude Code session than created it and reaches productive work without re-explaining context — repeatedly, across days. Target: at least 8 of 10 dogfood attempts succeed without manual context paste beyond choosing the Dossier.
2. **Token target visibility:** Distilled State recall reports token estimate and warns above the configured target. Target: no silent over-target recalls; no silent truncation.
3. **Trust without a gate:** near-zero "it summarized away something I needed and I couldn't get it back" incidents (captured source is recoverable from Archive); after-the-fact edits to Distilled State are rare and minor. Target: every material claim in sampled Dossiers has provenance.
4. **Capture friction:** promote/link/switch each complete in one command/tool call, allowing at most one inline disambiguation step for ambiguous targets.
5. **Open-work clarity:** user can answer "what topic needs me next?" from one view. Target: library/open-work list renders in under 2 seconds for 500 Dossiers on a typical laptop.
6. **Install transparency:** install output and session-start notices clearly identify the configured Claude Code integration and its capability (MCP, hooks, transcript-capture) availability.

---

## 9. Risks & mitigations

| Risk | Mitigation |
|------|------------|
| Poor distillation erodes trust | **Distillation Guide** steers content quality up front; deterministic cadence (§4.11) prevents missed updates. Plus non-destruction (captured Archive artifacts remain searchable), audit log, optional after-the-fact edits, provenance. No confirm gate by design (D4). |
| Agent forgets to update the Dossier | Hook-driven cadence (§5.4) makes updates deterministic, not discretionary; session-end/pre-compaction trigger backstops it. |
| Merge mangles history | D5 surface conflicts; audit log; non-destructive. |
| Context bloat returns | D7 100k target + explicit warnings; over-target state is a sprawl signal to reorganize/archive/split, not to silently truncate. |
| Suggestion engine misfires | Propose with confidence, never auto-link; user always confirms. |
| Snapshot staleness / rot | D6 refresh-while-active, freeze-on-resolve, store content not just links. |
| Scope creep into deferred surfaces | §0 deferral list is explicit; v1 = local core only. |

---

## 10. Out of scope (v1) — roadmap

Sharing & multi-user · web app · in-app LLM wrapper · automated ingestion (Slack/email/Drive OAuth) · native binary attachment storage · semantic search at scale · automated snapshot refresh. These layer onto the core; the core must stand alone first.

### Pre-create binding confirmation

**Problem.** A user starting a new session says "let's work on the auth refactor." The agent calls `dossier_promote` or `dossier create` and a new Dossier is born — even though one already exists for that exact topic, sitting at 80% completion. Over time, the library fragments: multiple thin Dossiers for the same thread, none with full context.

**The fix.** The agent already has everything it needs: the SessionStart hook injects the full unarchived Dossier list into context at the top of every session. Before creating a new Dossier, the agent should scan that in-context list for anything that looks related — no extra MCP call required. If plausible matches exist, surface them and ask the user to confirm before opening a new record.

Because the hook output is silent to the user (injected into the agent's context window but not displayed as chat), the hook can also carry standing instructions directly to the agent, for example:

> *Before creating a new Dossier, check the library above for an existing one on the same topic. If you find close matches, show them to the user and ask which to continue — or confirm that this is genuinely a new thread.*

That makes the behavior automatic without any user-visible ceremony at session start.

The confirmation itself should be lightweight:
> "I see a couple of Dossiers that look related — *Auth refactor (active, last updated 3 days ago)* and *Login flow cleanup (blocked)*. Is one of these the right one to continue, or is this a separate thread?"

If the user picks one, bind to it (`dossier_session`) and resume. If none fit, proceed with creation.

**Design notes:**
- The check is zero-latency because the library is already in context — this is not a search call, just an in-context scan.
- Hook instructions live alongside the injected library listing; keep them brief and imperative so they don't consume tokens unnecessarily.
- v1 already returns `ambiguous_target` from `dossier_promote`/`dossier create` when the server-side suggestion step fires (SPEC §8.5). This roadmap item moves the check earlier — to the agent's judgment before the tool is called at all — which is faster and avoids a round-trip.
- "None of these match" is a valid answer that should unlock creation without a second confirmation loop.
- Binding to an existing Dossier is the happy path when the topic exists; the confirmation should feel like a quick sanity check, not an interrogation.

---

## 11. Build decisions (resolved)

1. **Packaging — single self-contained binary** (Go or Rust), serving the MCP server over stdio and the CLI/TUI from the same executable. Rationale: zero runtime dependency = lowest adoption friction and trivial cross-platform install. Requiring node/python would be acceptable but adds an install step and a friction point we'd rather not impose.
2. **Tokenizer — one reasonable tokenizer benchmarked against Opus 4.8** as the reference; not precise per target model. Caveat in the README. (See §6.)
3. **After-the-fact review — no dedicated UX needed.** `dossier.md` is plain Markdown, so the user reviews on demand in their own reader (e.g. Obsidian) on any device. Revisions, when needed, go back through the agent — expected to be rare.
4. **Sourcing — agent-fetch-on-request.** Dossier assumes the agent already has its integrations and stores whatever content the agent/user provides; acquiring source material is not this app's responsibility (D6, §4.7).
5. **Ambiguous link/merge flow — single command with inline disambiguation.** This is the right compromise between speed and safety: no multi-screen workflow, no silent wrong attachment.
6. **Artifact format — text-first.** Markdown for human-readable docs, JSON for structured snapshots, TXT for transcripts/plain text. Binary storage is deferred; store metadata/path/provenance for binary or too-large sources.
7. **Concurrency — optimistic with conflict artifacts.** Avoid last-write-wins for distilled state; preserve both versions and ask the agent/user to reconcile.
8. **Happy path — agent-initiated.** At session start, the agent receives the Dossier library and should proactively say what is open, whether transcript capture is available, and ask whether this session should continue an existing Dossier or start without one. CLI commands remain available but are fallback/control surfaces, not the primary expected behavior.
