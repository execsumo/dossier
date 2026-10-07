# Dossier Distillation Guide
*Principles for High-Signal, Operationally Complete Context Preservation*

This guide defines what a Dossier's Distilled State must contain and how to write it. Its objective is not maximum brevity. It is minimum total effort to understand, resume, decide, and act: preserve the complete operational signal while removing conversational residue and redundant narration. A longer coherent Dossier is cheaper than several terse documents that force a reader to reconstruct context or reconcile competing versions. The test of every save: a fresh agent holding only this Distilled State, and none of the conversation, can take the next step without asking what you meant.

How and when to call the tools—saving, concurrency, polling, files, resuming—is covered by the Operating Instructions, delivered alongside this guide.

**The core contract:** the Distilled State is a *view*, not the record. The Archive holds the verbatim record; the Distilled State is the curated projection over it. Compression here is a rendering decision, never a destruction decision. Every compression you perform must leave behind a pointer that resolves back to the source—`dossier_artifact` fetches any cited artifact, and any cited line range within it. Compress what has settled; cite what you compress. Detail you elide without a resolvable citation is not compressed, it is lost.

## 1. Signal Retention & Cognitive Efficiency

A world-class Dossier is complete enough to act from, structured enough to scan, and free of conversational residue. Optimize for recoverable meaning and decision quality, not the fewest possible tokens.

- **Optimize Total Compute, Not Input Tokens Alone:** Count the reasoning needed to reconstruct omitted context, the extra reads needed to follow scattered documents, and the error cost of ambiguity. Preserve an explanation when it makes the next action or decision materially easier.
- **Prefer Plain Precision:** Use direct, readable language. Dense jargon and noun-heavy shorthand are not improvements when the reader must unpack them. Structure carries relationships; prose preserves rationale where the relationship itself is material.
- **Prune Mechanics, Preserve Meaning:** Consolidate the play-by-play into its net effect. Abstract *"I opened the file, scrolled, found the handler, and edited it"* into *"Patched the handler"*—but retain the handler's name, path, the change, and why it mattered.
- **Retain Decision Context:** Keep the causal links a future reader needs: why a constraint exists, how one decision affects another, what completion means operationally, and which apparent contradiction is intentional.
- **Never Compress These:** Reproduce verbatim, always. Identifiers and paths. Numbers, metrics, thresholds, versions, dates. Exact error text and status codes. Command lines and their flags. Config keys and values. API/function signatures. These are what a future reader needs to verify or re-run a decision, and they are precisely what paraphrase destroys. When unsure whether a value is material: keep it. Values are cheap; re-deriving them is not.
- **Keep What Only the Conversation Knew:** The detail a new session loses first is the detail that never existed outside the conversation: corrections the user gave (*"don't mock the database"*), preferences, the scope of an approval (*"may push to `feat/x`; ask before merging"*), the request still in flight, the working hypothesis, and commitments made to the user. None of it is in the code or the Archive's small artifacts. Record it in `## Current State` (see §4), or propose it as a Constraint when it bounds the solution. A resumed session that repeats a behavior the user already corrected has failed, however good the rest of the state is.
- **Encode the Negative Space (Anti-Goals):** Explicitly preserve abandoned trajectories. The knowledge of a failed experiment or rejected alternative is high-value context. Compress dead-ends into dense warnings rather than discarding them as noise.
- **Retain Constraints as First-Class Signal:** Constraints define the feasible solution space. Record technical, commercial, legal, timing, budget, dependency, and authority boundaries; distinguish observed or decided constraints from assumptions. If a leader could alleviate one, name the decision-maker or relief path. Never silently remove a constraint—record its alleviation or invalidation as a Decision.

## 2. Provenance & Update Mechanics

- **Elision Requires a Resolvable Pointer:** Every claim that has a source carries `[src:art_<id>]`, and every claim compressed from a *span* of a source carries the span: `[src:art_<id>#L42-L68]`. Line numbers address the artifact's own physical lines—the same coordinates `dossier_search` reports and `dossier_artifact` resolves. A citation whose range does not exist in the artifact is flagged by `dossier doctor`; a dangling pointer reads as evidence while being none.
- **Tag What Cannot Be Cited:** Some claims have no artifact behind them: something a person said in conversation, a belief not yet checked, the in-flight execution state. Do not paper over that with a citation into the whole session transcript. Give the claim a role tag (§3)—`[stated]` with who said it, or `[assumed]`—so its standing is visible. `## Current State` and `## Next Steps` are working context and need no citations.
- **Cite Narrowly:** Prefer a range over a whole artifact, and a small purpose-built artifact over a range into a large one. `[src:art_x]` pointing at a 9,000-line transcript technically satisfies provenance and practically communicates nothing.
- **Archive First, Distill Second:** Save raw transcripts, code snapshots, and full threads as source artifacts in the Archive *before* referencing them in the Distilled State.
- **Compress on a Delay:** Material from the current and immediately preceding session stays at low compression—concrete, specific, still carrying its working detail. Apply §1's full density discipline only once a topic has settled. Detail destroyed at first write is destroyed at the moment you are least able to judge what will matter; deferring the lossy step costs a few hundred tokens and preserves the ability to make that call correctly later.
- **Durable Sections vs Working Sections:** Objective through Findings, Evidence, and Files hold the consolidated truth of the topic: no narration, no history of how it was reached. `## Current State` and `## Next Steps` hold working context and are expected to churn every save. "Compress on a Delay" applies to both: settled truth moves into the durable sections at full density; unsettled detail stays concrete where it is.
- **Update, Don't Rewrite:** Most saves change a few sections. Edit those; carry every other section over unchanged, word for word. Rewriting an untouched section re-paraphrases it, and paraphrase of a paraphrase is how preserved values erode across sessions. When you do need to re-distill a section, work from its cited sources, not from the previous distilled wording.
- **Retire, Don't Erase:** Superseded content leaves the Distilled State only by being consolidated, and the revision history keeps every prior version. A finished Next Step becomes its outcome—a Finding or a line in Current State—then leaves the list. An answered Open Question becomes a Finding carrying its answer. When either settles something binding, it belongs in Decisions, which is protected: propose it in its own save rather than folding it into a routine one. A superseded Finding is replaced, and the replacement says what it overturned when the old belief would otherwise be re-derived. A dead end stays, as a `[rejected]` or `[attempted]` entry.
- **A Dossier Can Be Too Thin:** The token target (`token_limit`) is a ceiling, not a goal. Under-citation is the more common failure: if the Archive holds evidence the Distilled State never points at, the curated view has drifted off its own record. `dossier_recall` returns the evidence index and warns about uncited artifacts—treat that warning as a defect, not noise.
- **Never Silently Truncate:** Never cut content to meet the token target. If the state exceeds it, warn the user and propose what could be consolidated; the decision to drop signal is theirs.
- **No Conversational Noise (Prune Mechanics, Retain Trajectories):** Eliminate greetings, pleasantries, tool-call mechanics, and verbose restatements. However, compress (do not delete) the conclusions of dead-end investigative paths so future resumption avoids repeating mistakes.
- **Protected Work Definition:** Objective, Done When, Validation, Constraints, and Decisions change only by decision, never as editorial cleanup. Routine saves leave them byte-for-byte unchanged; a proposed change goes in its own save, where it becomes a conflict for a human to accept or reject (see the Operating Instructions).
- **References vs Active Monitors:** Distinguish between *navigational* external pointers and *live* context streams that must be polled. Both use the same canonical Markdown link line:
  `- [<kind>: <label>](<URL>) — <purpose or description>.`
  Use `kind` values such as `comms`, `ticket`, `document`, or `other`; the kind is intentionally tool-agnostic. Put ordinary pointers in `## References`; put live streams that require resumption polling in `## Active Monitors`. A monitor is not duplicated in both sections. A URL alone is not evidence: when external content supports a claim, capture it as an Archive artifact and cite it with `[src:art_id]`.

## 3. Role Tags

Identical text means different things in different positions: an *intention* is not an *observation*, and a *proposal* is not a *commitment*. Telegraphic phrasing erases that distinction unless you mark it. Tag any claim whose status is not obvious from its section:

- `[observed]` — measured, returned by a tool, or read from a real system. The strongest claim.
- `[attempted]` — tried; outcome recorded alongside.
- `[decided]` — settled and binding until explicitly revisited.
- `[proposed]` — on the table, not agreed.
- `[stated]` — asserted by a person (a requirement, a fact about their world, a preference) and not independently verified. Name who: `[stated] (By: <name>)`. Distinct from the agent's own inference.
- `[assumed]` — believed but unverified. Carries the highest re-check priority on resumption.
- `[rejected]` — considered and ruled out. Always retain the reason.

`[observed] Lock contention at 200ms timeout` and `[assumed] Lock contention at 200ms timeout` differ by one word and are completely different facts.

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

## Files
*Conditional*—omit when no working files exist. Index of loose deliverables and attachments in `files/` (decks, HTML, spreadsheets, binaries) that are not citable Archive evidence. One line per file, path relative to the Dossier directory, so the next session can find the work without browsing the folder.
- `files/<name>` (<kind>): <what it is and its status: draft, final, superseded>.
- `<repo identity>:<path in repo>` (<kind>): <same> — for deliverables kept in one of the Dossier's repos, e.g. `github.com/acme/api:docs/plan.md`. Never an absolute path.

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
The most perishable section: where the work stands right now, written so a fresh session can pick it up mid-stride. Open with an as-of stamp so a reader can tell when it has gone stale.
- As of: <YYYY-MM-DD>, <locator: repo branch@commit, document version, environment>.
- In flight: <the latest request being worked and how far it got; any half-applied or uncommitted change>.
- Working hypothesis: <what is currently believed and why, if an investigation is open>.
- Active context: <files, blockers, configurations that matter now>.
- Working agreements: <corrections, preferences, and approvals the user gave in conversation, each with its scope; commitments made to the user>.
Omit a bullet that does not apply; never omit the as-of stamp.

## Deliverables
*Conditional*—use only when two or more contributions combine into the Dossier's shared outcome. A single-deliverable Dossier uses the top-level Objective / Done When / Validation directly and does not wrap them in a redundant Deliverables section.
### <Deliverable label>
- Outcome: <The distinct contribution this piece produces.>
- Owner: <Person, `agent:<rolodex-slug>`, or unassigned. The Dossier lead remains accountable for the overall outcome. An agent owner is accepted by a human against the named Dossier revision.>
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
Immediate required actions, in order, each concrete enough to start without re-deriving it. Must align with `next_action` and the `## Open Questions` section in the Distilled State body. On every explicit save of active work, keep `status` and the machine-visible `next_action` baton current; the body carries the supporting context for the next person or timezone.
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
> - **Objective:** Billing charges every concurrent usage event exactly once under the `usage-tier` model.
> - **Done When:** `TestConcurrentBilling` passes at production lock settings; no double-charge or missed-charge rows in the 2026-06-20 load-test replay.
> - **Validation:** `go test ./internal/billing/... -run TestConcurrentBilling -count=20` green; replay diff report shows 0 mismatched rows.
> - **Constraints:**
>   - Lock overhead must stay under 100ms at p50; checkout SLO. [src:art_01jz8pm_alignment#L30-L34]
>   - [assumed] No schema migration this release; relief path: the billing lead can approve one.
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
> - **Open Questions:**
>   - Does a 500ms lock timeout breach the checkout SLO under production load?
> - **References:**
>   - [ticket: PROJ-123](https://jira.example.com/browse/PROJ-123) — Pricing migration work.
>   - [document: Pricing launch plan](https://confluence.example.com/display/PRICING/Launch+plan) — Current rollout plan.
> - **Active Monitors:**
>   - [comms: #pricing-bug](https://slack.com/...) — Ongoing discussion regarding usage-tier lock timeouts. (Last polled: 2026-06-14)
> - **Current State:**
>   - As of: 2026-06-14, `github.com/acme/api` branch `fix/billing-lock` @ `4e1c9a2`.
>   - In flight: lock timeout raised to 500ms in `internal/billing/lock.go`; local suite green; not yet pushed.
>   - Working agreements: Herwin approved pushing to `fix/billing-lock`; ask before merging to `main`. Use the real Postgres test container, not mocks (correction given 2026-06-14).
> - **Next Steps:**
>   1. Push `fix/billing-lock` and open the PR.
>   2. Pull p50/p99 lock-wait from production telemetry to check the load-test assumption and answer the SLO question.

The good version is longer than the thin one. That is correct. It is longer because it kept the values, kept the rejected path, marked what is assumed rather than observed, stamped the perishable state with when and where it was true, carried the user's corrections forward, and pointed each sourced claim at a span someone can actually open.

## 8. Before You Save

Run this check on every save. Each question names a failure this guide exists to prevent.

1. **Cold start:** Could a fresh agent with only this Distilled State take the first Next Step without asking what anything means?
2. **Values:** Did every identifier, number, path, command, and error string you touched survive verbatim?
3. **Standing:** Is every claim either cited to a span or tagged for what it is (`[stated]`, `[assumed]`, `[proposed]`)?
4. **Negative space:** Is every abandoned path from this session recorded with its reason?
5. **Conversation-only context:** Are the user's corrections, preferences, and approval scopes from this session in Current State, with a fresh as-of stamp?
6. **Untouched sections:** Did sections you meant to leave alone come through unchanged, and protected sections byte-for-byte?
7. **Baton:** Do `status` and `next_action` match `## Next Steps`?
