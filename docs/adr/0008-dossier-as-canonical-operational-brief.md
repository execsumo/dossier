# ADR 0008: The Dossier is the canonical operational brief

## Status

Accepted (2026-09-08). Supersedes the seven-field delegation-contract model
documented in HANDOFF.md and assets/dossier-delegate-skill.md. Amends the
Distilled State structure in SPEC §4.2 and the canonical lifecycle status
vocabulary in SPEC §4.1.

## Context

Dossier began with a sound two-layer distinction: Distilled State carries the
curated truth needed to resume work; the Archive retains source material. The
Distillation Guide then over-optimized the curated layer for terseness and
linguistic compression.

Delegation exposed the cost. To make asynchronous work executable, the
dossier-delegate skill persisted a second seven-field specification under
Delegation Contracts:

- Objective
- Context
- Success Criteria
- Validation
- Constraints
- Decision Rights
- Escalation

Five of those seven fields describe the work regardless of who performs it.
They were repeated because the Dossier itself had no explicit canonical home
for Objective, Done When, Validation, or Constraints. A typical user with one
deliverable therefore saw the same work described in the body and again in the
contract. The design produced:

- two authoritative-looking versions of the objective and completion criteria;
- drift detection and reconciliation work that existed only because of the
  duplication;
- a Dossier that became well-defined only when work happened to be delegated;
- extra reads and reasoning to reconstruct terse content and compare documents;
- a delegation-specific structure standing in for the missing work brief.

This is especially harmful for the common case: one Dossier, one primary
deliverable. Users naturally understand the Dossier itself as the thing they
are trying to complete.

At the same time, demanding a complete brief when a thought is first captured
would destroy the zero-friction flow. Real work begins as a loose spark and
becomes clear only when attention or execution requires it. The product needs a
checkpoint, not an intake form.

Multiple contributors add another pressure. Each contribution needs its own
completion conditions, but a new persisted work-package entity would recreate
the same abstraction and synchronization costs. Markdown can represent this
shape without promoting every deliverable into a separate domain object.

Finally, constraints need stronger treatment than either the original Dossier
schema or the delegation contract gave them. Constraints determine which
solutions are feasible. They also reveal where leadership intervention has
leverage: a budget, deadline, policy, dependency, or authority boundary may be
fixed, assumed, or removable by a named decision-maker. Losing that information
changes the solution space and hides opportunities to unblock the work.

## Decision

### 1. The Dossier owns the work truth

The Distilled State is the single canonical operational brief. For work that
has passed its definition checkpoint it contains:

- Objective: the primary outcome;
- Done When: observable conditions that close the Dossier;
- Validation: how completion is checked;
- Constraints: boundaries of the feasible solution space;
- the existing Situation, Decisions, Findings, Evidence, Open Questions,
  References, Active Monitors, Current State, and Next Steps;
- Deliverables only when several contributions combine into the shared outcome;
- Delegation Contracts only when work is assigned to another person.

This is not a third work-definition object. It is the Dossier body itself.

### 2. Signal retention replaces terseness as the optimization target

Distillation removes conversational residue and redundant narration; it does
not minimize document length.

The relevant cost is total compute and error:

- context tokens loaded;
- reasoning needed to reconstruct omitted relationships;
- additional artifact or document reads;
- reconciliation between duplicated statements;
- mistakes and clarification loops caused by ambiguity.

A longer coherent Dossier is preferable when it reduces that total. The
standard is: complete enough to act from, structured enough to scan, and free
of conversational residue.

### 3. Definition is progressive

A spark may remain raw and loosely structured. Define is the stage in which the
canonical brief becomes executable. Before entering execute—and always before
delegation—the work receives a health checkpoint:

> Can the person doing this proceed and know when they are finished?

The checkpoint is qualitative. It names a concrete missing fact or says the
work is ready. It never emits a score and never walks every heading as a form.
Unknowns may remain when they are explicitly recorded with an owner or
resolution path.

### 4. One deliverable stays structurally simple

For the common single-deliverable case, the Dossier-level Objective, Done When,
and Validation define the work. No Deliverables wrapper is added.

When several contributions combine into one shared outcome, a conditional
Deliverables section gives each contribution:

- Outcome
- Owner
- Done When
- Validation
- Completion: open, done, or dropped

Shared context and constraints remain at Dossier level. A narrower,
deliverable-specific constraint is written locally only when its scope
genuinely differs.

The practical boundary is intentionally fuzzy:

- several contributions toward one integrated outcome remain one Dossier;
- independently completable outcomes with substantially different context,
  evidence, timelines, or decisions are candidates for separate Dossiers.

### 5. Constraints are first-class retained signal

Constraints include technical, commercial, legal, timing, budget, dependency,
interface, and authority boundaries.

Every material constraint retains its status, rationale, and provenance:

- observed or decided constraints bind the solution;
- assumed constraints are visibly unverified;
- constraints a leader could alleviate name the decision-maker or relief path.

A constraint is never silently omitted or removed to make a proposed solution
fit. Its alleviation, invalidation, or replacement is recorded as a Decision
with attribution, date, rationale, and provenance.

### 6. Delegation is an agreement around canonical work

A Delegation Contract contains only person-specific terms:

- Scope: the entire Dossier or an exact Deliverables heading;
- Acceptance: accepted, clarification requested, or changes proposed,
  including timing or commitment and the accepted Dossier revision;
- Decision Rights;
- Escalation, including route and useful work while waiting;
- Return Expectations.

Objective, context, Done When, Validation, and Constraints are not copied into
the contract. The outbound note is a rendering that combines canonical Dossier
content with the person-specific contract. It may be verbose for an
asynchronous reader, but it is not another store.

Delegation therefore acts as a forcing function for Dossier health. A work gap
updates the canonical brief; a relationship gap updates the contract.

Acceptance is distinct from sending. If canonical work changes materially
after acceptance, the current brief remains current truth, while the contract's
accepted revision preserves the agreement baseline. The drift is surfaced and
renewed acceptance is required before judging the recipient against the changed
target.

### 7. Lifecycle describes the work, not its staffing

The canonical stages are:

- spark
- define
- execute
- review
- blocked
- done

Delegated was a relationship event masquerading as a Dossier-wide work stage.
It becomes a legacy input normalized to execute. Waiting remains a legacy input
and also normalizes to execute; specific dependencies and blockers belong in
the Dossier state rather than changing the meaning of the execution stage.

The Dossier lead is accountable for the overall outcome. Deliverable owners are
responsible for their contributions. A contributor's submission does not make
the deliverable done; its Validation must pass. The Dossier becomes done only
when every required deliverable is done or explicitly dropped and the overall
Done When conditions validate.

When the primary outcome is complete, the Dossier is done. Follow-on work
starts as a new spark. A future first-class linking capability may connect
successive Dossiers; until then the relationship is recorded in ordinary
Markdown rather than keeping finished work artificially open.

## Compatibility

- Stored status delegated remains accepted and normalizes to execute.
- Stored status waiting remains accepted and normalizes to execute.
- Existing seven-field Delegation Contracts remain readable. The parser
  projects Objective to Scope and an agreed header to Acceptance, preserves
  Decision Rights and Escalation, and leaves Return Expectations visibly open.
- Legacy work-definition fields remain in the source contract until a requested
  delegation pass migrates them into canonical Dossier sections. No automatic
  rewrite claims semantic equivalence or invents missing terms.

## Consequences

- A Dossier can be longer, but it has one coherent source of operational truth.
- Agents spend fewer tokens and less reasoning reconciling repeated documents.
- Ordinary work benefits from clear outcomes and completion criteria even when
  it is never delegated.
- Delegation notes remain detailed enough for asynchronous work without
  becoming an additional store.
- The definition checkpoint adds structure only when execution makes it
  valuable.
- Multiple-deliverable Dossiers remain representable without introducing a
  task database or formal work-package entity.
- Existing contract-checklist surfaces now report ready or name open terms;
  numeric completeness scores are removed.

## Alternatives considered

### Keep the seven-field contract

Rejected. It preserves an accepted snapshot but duplicates person-agnostic work
truth and makes delegation the only route to a healthy brief.

### Create a separate Work Definition entity

Rejected. It adds another document, identity, reference, and lifecycle for
information that naturally belongs in the Dossier. The common single-deliverable
case would pay permanent abstraction cost for uncommon complexity.

### Require full structure when a Dossier is created

Rejected. Users capture ideas before they have time or evidence to define them.
Spark must remain cheap; define provides the later checkpoint.

### Keep delegated as the lifecycle stage

Rejected. A Dossier may mix work performed by the user, agents, and several
people. Staffing does not describe the overall phase of the outcome.

### Force every contribution into a separate Dossier

Rejected. Closely coupled contributions would repeat shared context,
constraints, and integrated completion conditions. Separate Dossiers remain an
available judgment when outcomes become genuinely independent.
