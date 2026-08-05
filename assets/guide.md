# Dossier Distillation Guide
*Principles for High-Density, Lossless Context Preservation*

This guide defines the methodology for maintaining the Distilled State of a Dossier. Its objective is to maximize the signal-to-noise ratio—applying lossless information compression to conversational and operational data. Adhere to these principles to produce context that is cognitively lightweight, analytically dense, and immediately resumable.

## 1. Information Theory & Linguistic Compression

A world-class dossier ruthlessly prunes linguistic fat while preserving all material facts, decisions, and reasoning vectors.

- **Maximize Lexical Density:** Upgrade vocabulary to eliminate phrasal verbs and colloquialisms. Substitute low-density phrases with precise, high-level terminology. (e.g., Use *"investigated and deprecated"* instead of *"looked into it and decided we shouldn't use it anymore"*).
- **Syntactic Pruning (Telegraphic Phrasing):** Strip unnecessary articles (*a, an, the*), auxiliary verbs, and filler words where context allows. Avoid conversational transitions. Rely on structural formatting (bullets, headers) to convey relationships rather than prose.
- **Active Voice & Nominalization:** Convert wordy, passive descriptions into punchy, noun-heavy declarations. (e.g., Change *"The test script was run and it failed"* to *"Test script execution failed"*).
- **Semantic Abstraction:** Consolidate granular, play-by-play actions into their net effect or underlying outcome. Do not log the mechanics of the work; log the result.
- **Encode the Negative Space (Anti-Goals):** Explicitly preserve abandoned trajectories. The knowledge of a failed experiment or rejected alternative is high-value context. Compress dead-ends into dense warnings rather than discarding them as noise.

## 2. Process & State Mechanics

- **No Conversational Noise (Prune Mechanics, Retain Trajectories):** Eliminate greetings, pleasantries, tool-call mechanics, and verbose restatements. However, compress (do not delete) the conclusions of dead-end investigative paths so future resumption avoids repeating mistakes.
- **Durable State Only:** The Distilled State must represent the current, clean, consolidated truth of the topic.
- **Strict Provenance:** Every material claim, metric, and decision MUST trace back to its origin. Append source references to claims. Example: `[src:art_01jz8salesfeedback#L42-L68]` or `[src:art_01jz8launchthread]`.
- **Archive First, Distill Second:** Save raw transcripts, code snapshots, and full threads as source artifacts in the Archive *before* referencing them in the Distilled State.
- **Archival Sources vs Active Monitors:** Distinguish between *static* historical context (inline links like `[src:art_id]`) and *live* context streams that must be polled (like ongoing Slack threads). Live context belongs in `## Active Monitors`.
- **Keep Context Current:** Maintain the session's active Dossier using a best-effort approach each turn. Save state on lifecycle events (session end, `/clear`, `/exit`, pre-compaction).
- **Never Silently Truncate:** Never truncate the Distilled State to meet arbitrary token limits. If approaching limits, warn the user.
- **Optimistic Concurrency & Disambiguation:** Concurrent edits produce conflict files. Prompt the user for ambiguous link targets and manual merge conflict resolution. Never rely on last-write-wins.
- **Degrade Visibly:** If a harness fails to capture transcripts or lifecycle hooks, warn the user explicitly. Never silently ignore failures.

## 3. Structure of the Distilled State

Every `dossier.md` body must rigidly follow this schema:

```markdown
# <Dossier Name>

## Situation
Core problem, goal, or topic. High-density summary of initial state and context.

## Decisions
Irreversible or material agreements. Require attribution, rationale, date, and provenance.
- [YYYY-MM-DD] <Decision>: <Rationale>. (By: <Attribution>) [src:art_<id>]

## Findings
Validated insights, metrics, constraints, or test results. Include abandoned paths to preserve negative space.
- <Finding> [src:art_<id>#L<a>-L<b>]
- [Rejected] <Alternative considered>; <Constraint or reason for rejection>. [src:art_<id>]

## Active Monitors
Live external context streams that must be polled for updates upon resuming this Dossier.
- [<NAMESPACE>: <ID>](<URL>): <Reason to poll>. (Last polled: <YYYY-MM-DD>)

## Current State
Immediate execution context. Active files, blockers, or configurations.

## Next Steps
Immediate required actions. Must align with `next_action` and `open_questions` metadata.
```

## 4. Multi-author Dossiers

- **Attribute opinions:** Attribute contested or opinion-bearing claims via provenance.
- **Update, don't duplicate:** Prefer updating a claim over duplicating it.
- **Surface disagreement:** When two authors' sessions disagree, record the disagreement explicitly rather than averaging it away.

## 5. Distillation Comparison

### BAD DISTILLATION (Low Density, Lossy, High Noise)
> Hey there! So I started looking into the pricing bug. I ran the test script `go test ./...` and it failed on line 12. Then I talked to Herwin and he said we should use usage-tier instead. I tried fixing it by changing the condition and it passed. Next step is to clean up.

### GOOD DISTILLATION (Lossless, High Density, Telegraphic)
> - **Situation:** Enforcing `usage-tier` billing calculation under high concurrency. [src:art_01jz8initial_bug]
> - **Decisions:**
>   - [2026-06-14] Migrated billing model from flat-tier to usage-tier. (By: Herwin). Rationale: Mitigates billing leakage during concurrent user actions. [src:art_01jz8pm_alignment#L12-L28]
> - **Findings:**
>   - [Rejected] Redis distributed lock; introduced unacceptable network latency (>100ms overhead). [src:art_01jz8redis_eval]
>   - Concurrency tests fail at lock timeouts < 200ms. [src:art_01jz8test_results#L102]
> - **Active Monitors:**
>   - [SLACK: #pricing-bug](https://slack.com/...): Ongoing discussion regarding usage-tier lock timeouts. (Last polled: 2026-06-14)
> - **Current State:** Lock timeout increased to 500ms; local test suite passing.
> - **Next Steps:** Merge pricing patch; monitor production telemetry.
