# Dossier Distillation Guide
*Principles for High-Signal, Operationally Complete Context Preservation*

This guide defines the methodology for maintaining the Distilled State of a Dossier. Its objective is not maximum brevity. It is minimum total effort to understand, resume, decide, and act: preserve the complete operational signal while removing conversational residue and redundant narration. A longer coherent Dossier is cheaper than several terse documents that force a reader to reconstruct context or reconcile competing versions.

**The core contract:** the Distilled State is a *view*, not the record. The Archive holds the verbatim record; the Distilled State is the curated projection over it. Compression here is a rendering decision, never a destruction decision. Every compression you perform must leave behind a pointer that resolves back to the source—`dossier_artifact` fetches any cited artifact, and any cited line range within it. Compress hard; cite harder. Detail you elide without a resolvable citation is not compressed, it is lost.

## 1. Signal Retention & Cognitive Efficiency

A world-class Dossier is complete enough to act from, structured enough to scan, and free of conversational residue. Optimize for recoverable meaning and decision quality, not the fewest possible tokens.

- **Optimize Total Compute, Not Input Tokens Alone:** Count the reasoning needed to reconstruct omitted context, the extra reads needed to follow scattered documents, and the error cost of ambiguity. Preserve an explanation when it makes the next action or decision materially easier.
- **Prefer Plain Precision:** Use direct, readable language. Dense jargon and noun-heavy shorthand are not improvements when the reader must unpack them. Structure carries relationships; prose preserves rationale where the relationship itself is material.
- **Prune Mechanics, Preserve Meaning:** Consolidate the play-by-play into its net effect. Abstract *"I opened the file, scrolled, found the handler, and edited it"* into *"Patched the handler"*—but retain the handler's name, path, the change, and why it mattered.
- **Retain Decision Context:** Keep the causal links a future reader needs: why a constraint exists, how one decision affects another, what completion means operationally, and which apparent contradiction is intentional.
- **Never Compress These:** Reproduce verbatim, always. Identifiers and paths. Numbers, metrics, thresholds, versions, dates. Exact error text and status codes. Command lines and their flags. Config keys and values. API/function signatures. These are what a future reader needs to verify or re-run a decision, and they are precisely what paraphrase destroys. When unsure whether a value is material: keep it. Values are cheap; re-deriving them is not.
- **Encode the Negative Space (Anti-Goals):** Explicitly preserve abandoned trajectories. The knowledge of a failed experiment or rejected alternative is high-value context. Compress dead-ends into dense warnings rather than discarding them as noise.
- **Retain Constraints as First-Class Signal:** Constraints define the feasible solution space. Record technical, commercial, legal, timing, budget, dependency, and authority boundaries; distinguish observed or decided constraints from assumptions. If a leader could alleviate one, name the decision-maker or relief path. Never silently remove a constraint—record its alleviation or invalidation as a Decision.

## 2. Process & State Mechanics

- **Elision Requires a Resolvable Pointer:** Every claim carries `[src:art_<id>]`, and every claim compressed from a *span* of a source carries the span: `[src:art_<id>#L42-L68]`. Line numbers address the artifact's own physical lines—the same coordinates `dossier_search` reports and `dossier_artifact` resolves. A citation whose range does not exist in the artifact is flagged by `dossier doctor`; a dangling pointer reads as evidence while being none.
- **Cite Narrowly:** Prefer a range over a whole artifact, and a small purpose-built artifact over a range into a large one. `[src:art_x]` pointing at a 9,000-line transcript technically satisfies provenance and practically communicates nothing.
- **Archive First, Distill Second:** Save raw transcripts, code snapshots, and full threads as source artifacts in the Archive *before* referencing them in the Distilled State.
- **Compress on a Delay:** Material from the current and immediately preceding session stays at low compression—concrete, specific, still carrying its working detail. Apply §1's full density discipline only once a topic has settled. Detail destroyed at first write is destroyed at the moment you are least able to judge what will matter; deferring the lossy step costs a few hundred tokens and preserves the ability to make that call correctly later.
- **A Dossier Can Be Too Thin:** The token target is a ceiling, not a goal. Under-citation is the more common failure: if the Archive holds evidence the Distilled State never points at, the curated view has drifted off its own record. `dossier_recall` returns the evidence index and warns about uncited artifacts—treat that warning as a defect, not noise.
- **No Conversational Noise (Prune Mechanics, Retain Trajectories):** Eliminate greetings, pleasantries, tool-call mechanics, and verbose restatements. However, compress (do not delete) the conclusions of dead-end investigative paths so future resumption avoids repeating mistakes.
- **Durable State Only:** The Distilled State must represent the current, clean, consolidated truth of the topic.
- **References vs Active Monitors:** Distinguish between *navigational* external pointers and *live* context streams that must be polled. Both use the same canonical Markdown link line:
  `- [<kind>: <label>](<URL>) — <purpose or description>.`
  Use `kind` values such as `comms`, `ticket`, `document`, or `other`; the kind is intentionally tool-agnostic. Put ordinary pointers in `## References`; put live streams that require resumption polling in `## Active Monitors`. A monitor is not duplicated in both sections. A URL alone is not evidence: when external content supports a claim, capture it as an Archive artifact and cite it with `[src:art_id]`.
- **Keep Context Current:** Maintain the session's active Dossier using a best-effort approach each turn. Save state on lifecycle events (session end, `/clear`, `/exit`, pre-compaction).
- **Never Silently Truncate:** Never truncate the Distilled State to meet arbitrary token limits. If approaching limits, warn the user.
- **Optimistic Concurrency & Disambiguation:** Concurrent edits produce conflict files. Prompt the user for ambiguous link targets and manual merge conflict resolution. Never rely on last-write-wins.
- **Degrade Visibly:** If a harness fails to capture transcripts or lifecycle hooks, warn the user explicitly. Never silently ignore failures.

## 3. Role Tags

Identical text means different things in different positions: an *intention* is not an *observation*, and a *proposal* is not a *commitment*. Telegraphic phrasing erases that distinction unless you mark it. Tag any claim whose status is not obvious from its section:

- `[observed]` — measured, returned by a tool, or read from a real system. The strongest claim.
- `[attempted]` — tried; outcome recorded alongside.
- `[decided]` — settled and binding until explicitly revisited.
- `[proposed]` — on the table, not agreed.
- `[assumed]` — believed but unverified. Carries the highest re-check priority on resumption.
- `[rejected]` — considered and ruled out. Always retain the reason.

`[observed] Lock contention at 200ms timeout` and `[assumed] Lock contention at 200ms timeout` are the same nine tokens and completely different facts.

## 4. Structure of the Distilled State

A `spark` may begin as raw, loosely structured thought; demanding a completed brief at capture time defeats zero-friction promotion. During `define`, shape it into the canonical structure below. Before work moves to `execute`—and always before delegation—the Objective, Done When, Validation, Constraints, and any deliverable-specific completion conditions must be clear enough that the person doing the work can proceed and know when they are finished. This is a judgment checkpoint, not a form or completeness score: name the specific missing fact or say the work is ready.

Sections marked *conditional* are omitted when they do not apply. Once a Dossier has passed the definition checkpoint, every other section keeps its position; an explicitly empty section records that it was considered.

```markdown
# <Dossier Name>

## Objective
The single primary outcome this Dossier exists to produce. Describe the end state, not a task list.

## Done When
Observable conditions that make the overall outcome complete. For a multi-deliverable Dossier, state the integrated result rather than repeating each deliverable's local criteria.

## Validation
How the overall Done When conditions will be checked. A contributor's submission is not completion until the relevant validation passes.

## Constraints
Boundaries that determine feasible solutions: what must not change, be touched, be exceeded, or be assumed. Preserve rationale and provenance. Mark assumptions as assumptions; when a leader can alleviate a constraint, name that relief path.

## Situation
Relevant context needed to understand the work without shared conversational memory.

## Decisions
Irreversible or material agreements. Require attribution, rationale, date, and provenance.
- [YYYY-MM-DD] [decided] <Decision>: <Rationale>. (By: <Attribution>) [src:art_<id>#L<a>-L<b>]

## Findings
Validated insights, metrics, constraints, or test results. Include abandoned paths to preserve negative space.
- [observed] <Finding> [src:art_<id>#L<a>-L<b>]
- [rejected] <Alternative considered>; <Constraint or reason for rejection>. [src:art_<id>]

## Evidence
Index of the Archive: what is stored, what is in it, and where the citable spans are. One line per artifact. Keep it current—an artifact absent from this index is one nobody will think to fetch.
- `art_<id>` (<type>, <n> lines): <what it contains>. Key spans: L<a>-L<b> <what is there>.

## Open Questions
Unresolved questions that materially affect the topic or next move.
- <Question that needs an answer or decision>

## References
*Conditional*—omit when there are no durable external pointers. These links are for navigation and context; they do not imply polling or constitute citable evidence.
- [<kind>: <label>](<URL>) — <purpose or description>.

## Active Monitors
*Conditional*—omit when there are no live external streams to check. Use the same link line as `## References`, adding the required polling marker.
- [<kind>: <label>](<URL>) — <reason to poll>. (Last polled: <YYYY-MM-DD>)

## Current State
Immediate execution context. Active files, blockers, or configurations.

## Deliverables
*Conditional*—use only when two or more contributions combine into the Dossier's shared outcome. A single-deliverable Dossier uses the top-level Objective / Done When / Validation directly and does not wrap them in a redundant Deliverables section.
### <Deliverable label>
- Outcome: <The distinct contribution this piece produces.>
- Owner: <Person, agent, user, or unassigned. The Dossier lead remains accountable for the overall outcome.>
- Done When: <Observable local completion conditions.>
- Validation: <How this deliverable gets checked.>
- Completion: [open|done|dropped] <For done, cite validation evidence; for dropped, retain the reason.>

## Delegation Contracts
*Conditional*—present only when the whole Dossier or a deliverable is delegated. The contract does not restate Objective, context, completion criteria, Validation, or Constraints: those are canonical Dossier content. It records only what becomes true because this person is doing this scope. Every bullet carries `[decided]` once settled or `[proposed]` while open. A proposed field is mirrored in `## Open Questions`. Readiness is expressed by naming open terms, never by a numeric completeness score.
### <Assignment label> — owner: <Person>[, accepted <YYYY-MM-DD>] [src:art_<id>#L<a>-L<b>]
- Scope: [decided|proposed] <"Entire Dossier" or the exact `## Deliverables` heading; do not duplicate its work definition.>
- Acceptance: [decided|proposed] <Accepted, clarification requested, or changes proposed; when accepted, record date, commitment/timing, and the accepted Dossier revision.>
- Decision Rights: [decided|proposed] <What the owner decides unilaterally vs. what needs sign-off.>
- Escalation: [decided|proposed] <Conditions to stop and flag rather than guess, who/where to escalate, and what to do while waiting.>
- Return Expectations: [decided|proposed] <The status, validation result, output/evidence, and decisions the owner returns.>

## Next Steps
Immediate required actions. Must align with `next_action` and the `## Open Questions` section in the Distilled State body.
```

### Work-shape rules

- **One deliverable is the default.** The Dossier's Objective / Done When / Validation define it directly. Do not create a redundant Deliverables section.
- **Several contributions, one shared outcome:** keep one Dossier and give every deliverable its own Done When and Validation. Shared context and constraints stay at the Dossier level; put a narrower constraint under a deliverable only when its scope truly differs.
- **Independent outcomes:** when pieces can finish independently and carry substantially different context, evidence, timelines, or decisions, prefer separate Dossiers. This boundary is intentionally judgment-based.
- **Completion is validated, not reported.** A deliverable is `done` only when its local validation passes. The Dossier becomes `done` only when every required deliverable is done or explicitly dropped and the overall Done When conditions validate.
- **Primary work closes the Dossier.** Follow-on work begins as a new spark rather than keeping a completed Dossier indefinitely alive. Until first-class Dossier linking exists, record the follow-on plainly in Decisions or Next Steps.
- **Accepted baselines do not drift silently.** A delegation Acceptance names the revision agreed. If the canonical Objective, Done When, Validation, Constraints, or scoped deliverable changes materially, surface the drift and obtain renewed acceptance; do not copy the old work definition into the contract.

## 5. Choosing What to Archive

Provenance is only as good as the artifact underneath it. A session transcript is a fallback, not a citation target—reach for a purpose-built artifact at the moment the evidence appears:

- `decision_evidence` — the specific exchange, benchmark, or output that settled a decision. Cite this from `## Decisions` instead of the whole session.
- `file_snapshot` — a file's state at a moment that matters (the failing config, the schema before migration).
- `query` — a query and its result set, when the result is the finding.
- `link` — an external source, with its fetched content captured so the claim survives link rot.
- `source_snapshot` — a code or document excerpt under discussion.
- `transcript` — full session capture. Broad, coarse, automatic. The floor, not the target.

Small artifacts captured at decision time beat large ones captured at session end: they are precisely citable, they survive summarization, and they cost less to fetch back.

## 6. Multi-author Dossiers

- **Attribute opinions:** Attribute contested or opinion-bearing claims via provenance.
- **Update, don't duplicate:** Prefer updating a claim over duplicating it.
- **Surface disagreement:** When two authors' sessions disagree, record the disagreement explicitly rather than averaging it away.

## 7. Distillation Comparison

### BAD DISTILLATION (Low Density, Lossy, High Noise)
> Hey there! So I started looking into the pricing bug. I ran the test script `go test ./...` and it failed on line 12. Then I talked to Herwin and he said we should use usage-tier instead. I tried fixing it by changing the condition and it passed. Next step is to clean up.

### ALSO BAD (Dense, But Thin and Unrecoverable)
> - **Situation:** Billing bug. [src:art_01jz8session]
> - **Decisions:** Migrated billing model. [src:art_01jz8session]
> - **Findings:** Concurrency issue resolved; tests pass. [src:art_01jz8session]

Every line is terse and every line is cited, so this passes a mechanical provenance check. It is still a failure: the values are gone (which model? what timeout? which tests?), the rejected alternative is gone, and all three citations point at one 9,000-line transcript, so nothing can be recovered by following them.

### GOOD DISTILLATION (Lossless, High Density, Recoverable)
> - **Situation:** Enforcing `usage-tier` billing calculation under high concurrency. [src:art_01jz8initial_bug#L1-L40]
> - **Decisions:**
>   - [2026-06-14] [decided] Migrated billing model from flat-tier to usage-tier. (By: Herwin). Rationale: Mitigates billing leakage during concurrent user actions. [src:art_01jz8pm_alignment#L12-L28]
> - **Findings:**
>   - [rejected] Redis distributed lock; introduced unacceptable network latency (>100ms overhead measured at p50). [src:art_01jz8redis_eval#L44-L61]
>   - [observed] `TestConcurrentBilling` fails at lock timeouts < 200ms; passes at 500ms. [src:art_01jz8test_results#L102-L118]
>   - [assumed] Production concurrency resembles the load-test profile; unverified against telemetry.
> - **Evidence:**
>   - `art_01jz8redis_eval` (decision_evidence, 88 lines): Redis lock latency benchmark. Key spans: L44-L61 p50/p99 table.
>   - `art_01jz8test_results` (decision_evidence, 210 lines): concurrency suite output. Key spans: L102-L118 timeout sweep.
>   - `art_01jz8session` (transcript, 9,140 lines): full session capture; background only.
> - **References:**
>   - [ticket: PROJ-123](https://jira.example.com/browse/PROJ-123) — Pricing migration work.
>   - [document: Pricing launch plan](https://confluence.example.com/display/PRICING/Launch+plan) — Current rollout plan.
> - **Active Monitors:**
>   - [comms: #pricing-bug](https://slack.com/...) — Ongoing discussion regarding usage-tier lock timeouts. (Last polled: 2026-06-14)
> - **Current State:** Lock timeout increased to 500ms in `internal/billing/lock.go`; local suite green.
> - **Next Steps:** Merge pricing patch; verify the load-test assumption against production telemetry.

The good version is longer than the thin one. That is correct. It is longer because it kept the values, kept the rejected path, marked what is assumed rather than observed, and pointed each claim at a span someone can actually open.
