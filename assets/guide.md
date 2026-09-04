# Dossier Distillation Guide
*Principles for High-Density, Lossless Context Preservation*

This guide defines the methodology for maintaining the Distilled State of a Dossier. Its objective is to maximize the signal-to-noise ratio—applying lossless information compression to conversational and operational data. Adhere to these principles to produce context that is cognitively lightweight, analytically dense, and immediately resumable.

**The core contract:** the Distilled State is a *view*, not the record. The Archive holds the verbatim record; the Distilled State is the curated projection over it. Compression here is a rendering decision, never a destruction decision. Every compression you perform must leave behind a pointer that resolves back to the source—`dossier_artifact` fetches any cited artifact, and any cited line range within it. Compress hard; cite harder. Detail you elide without a resolvable citation is not compressed, it is lost.

## 1. Information Theory & Linguistic Compression

A world-class dossier ruthlessly prunes linguistic fat while preserving all material facts, decisions, and reasoning vectors.

- **Maximize Lexical Density:** Upgrade vocabulary to eliminate phrasal verbs and colloquialisms. Substitute low-density phrases with precise, high-level terminology. (e.g., Use *"investigated and deprecated"* instead of *"looked into it and decided we shouldn't use it anymore"*).
- **Telegraphic Phrasing:** Drop conversational transitions and filler. Rely on structural formatting (bullets, headers) to convey relationships rather than prose. Do not strip words to the point of ambiguity—an article or auxiliary verb that disambiguates *who did what to what* earns its tokens. Density is measured in recoverable meaning per token, not in tokens removed.
- **Active Voice & Nominalization:** Convert wordy, passive descriptions into punchy, noun-heavy declarations. (e.g., Change *"The test script was run and it failed"* to *"Test suite execution failed"*).
- **Semantic Abstraction (narration only):** Consolidate the *play-by-play of the work* into its net effect. Compress the narration, never the values. Abstract *"I opened the file, scrolled, found the handler, and edited it"* into *"Patched the handler"*—but the handler's name, path, and the change stay.
- **Never Compress These:** Reproduce verbatim, always. Identifiers and paths. Numbers, metrics, thresholds, versions, dates. Exact error text and status codes. Command lines and their flags. Config keys and values. API/function signatures. These are what a future reader needs to verify or re-run a decision, and they are precisely what paraphrase destroys. When unsure whether a value is material: keep it. Values are cheap; re-deriving them is not.
- **Encode the Negative Space (Anti-Goals):** Explicitly preserve abandoned trajectories. The knowledge of a failed experiment or rejected alternative is high-value context. Compress dead-ends into dense warnings rather than discarding them as noise.

## 2. Process & State Mechanics

- **Elision Requires a Resolvable Pointer:** Every claim carries `[src:art_<id>]`, and every claim compressed from a *span* of a source carries the span: `[src:art_<id>#L42-L68]`. Line numbers address the artifact's own physical lines—the same coordinates `dossier_search` reports and `dossier_artifact` resolves. A citation whose range does not exist in the artifact is flagged by `dossier doctor`; a dangling pointer reads as evidence while being none.
- **Cite Narrowly:** Prefer a range over a whole artifact, and a small purpose-built artifact over a range into a large one. `[src:art_x]` pointing at a 9,000-line transcript technically satisfies provenance and practically communicates nothing.
- **Archive First, Distill Second:** Save raw transcripts, code snapshots, and full threads as source artifacts in the Archive *before* referencing them in the Distilled State.
- **Compress on a Delay:** Material from the current and immediately preceding session stays at low compression—concrete, specific, still carrying its working detail. Apply §1's full density discipline only once a topic has settled. Detail destroyed at first write is destroyed at the moment you are least able to judge what will matter; deferring the lossy step costs a few hundred tokens and preserves the ability to make that call correctly later.
- **A Dossier Can Be Too Thin:** The token target is a ceiling, not a goal. Under-citation is the more common failure: if the Archive holds evidence the Distilled State never points at, the curated view has drifted off its own record. `dossier_recall` returns the evidence index and warns about uncited artifacts—treat that warning as a defect, not noise.
- **No Conversational Noise (Prune Mechanics, Retain Trajectories):** Eliminate greetings, pleasantries, tool-call mechanics, and verbose restatements. However, compress (do not delete) the conclusions of dead-end investigative paths so future resumption avoids repeating mistakes.
- **Durable State Only:** The Distilled State must represent the current, clean, consolidated truth of the topic.
- **Archival Sources vs Active Monitors:** Distinguish between *static* historical context (inline links like `[src:art_id]`) and *live* context streams that must be polled (like ongoing Slack threads). Live context belongs in `## Active Monitors`.
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

Every `dossier.md` body must rigidly follow this schema. Sections marked *conditional* are omitted entirely when they do not apply—an empty header is not neutral, it asserts that the section was considered and came back empty. Every other section is unconditional and keeps its position even while thin.

```markdown
# <Dossier Name>

## Situation
Core problem, goal, or topic. High-density summary of initial state and context.

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

## Active Monitors
Live external context streams that must be polled for updates upon resuming this Dossier.
- [<NAMESPACE>: <ID>](<URL>): <Reason to poll>. (Last polled: <YYYY-MM-DD>)

## Current State
Immediate execution context. Active files, blockers, or configurations.

## Delegation Contracts
*Conditional*—present only when a piece of this topic is delegated to someone. One `###` per contract; a Dossier can carry several over its life. Blocks appear in this fixed order, every time, so a reader (or a later session) finds them without searching. Tag each block: `[decided]` once settled and binding, `[proposed]` while still under discussion. An unsettled block stays in the contract as `[proposed]` and its resolution is mirrored as an entry in `## Open Questions`—that pairing is what makes a half-written contract legible on resumption.
### <Task label> — owner: <Lead>, agreed <YYYY-MM-DD> [src:art_<id>#L<a>-L<b>]
- Objective: <One sentence; the end state "done" produces, not a task list.>
- Context: <Self-contained; assume the reader has no shared memory beyond this Dossier.>
- Success Criteria: <The target state in testable terms, not adjectives.>
- Validation: <How the criteria get checked—the same check whether the owner self-reports or you run it.>
- Constraints: <What must not change, be touched, or be assumed.>
- Decision Rights: <What the owner decides unilaterally vs. what needs sign-off.>
- Escalation: <Conditions to stop and flag rather than guess, and what to do while waiting.>

## Next Steps
Immediate required actions. Must align with `next_action` and the `## Open Questions` section in the Distilled State body.
```

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
> - **Active Monitors:**
>   - [SLACK: #pricing-bug](https://slack.com/...): Ongoing discussion regarding usage-tier lock timeouts. (Last polled: 2026-06-14)
> - **Current State:** Lock timeout increased to 500ms in `internal/billing/lock.go`; local suite green.
> - **Next Steps:** Merge pricing patch; verify the load-test assumption against production telemetry.

The good version is longer than the thin one. That is correct. It is longer because it kept the values, kept the rejected path, marked what is assumed rather than observed, and pointed each claim at a span someone can actually open.
