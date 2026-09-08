---
name: dossier-delegate
description: "Assess a Dossier's readiness for human delegation, strengthen the canonical work brief, and produce a person-specific handoff agreement and note. Explicit-invocation only: use when the user asks to define, scope, delegate, or check delegated work. Never trigger automatically from status or lead."
---

# dossier-delegate — make the work executable before handing it over

A Dossier is the canonical operational brief for its outcome. This skill does
not create a second work specification. It uses delegation as a practical
checkpoint: when another person must act without a same-day clarification loop,
the work itself needs to be clear enough to execute and validate.

The user may have captured only a loose spark. That is healthy. Do not demand a
fully structured brief at creation time. Structure becomes necessary when the
work leaves define and enters execute, especially when someone else is about to
own all or part of it.

Explicit invocation only. Run because the user asked by name, asked for help
defining or delegating work, or requested a handoff note. Never auto-run because
a lead exists, a Dossier looks thin, or a status changed.

## Core distinction

Person-agnostic truth belongs once in the Dossier:

- Objective
- Done When
- Validation
- Constraints
- Situation, Decisions, Findings, and Current State
- for a multi-deliverable Dossier, each deliverable's Outcome, Done When,
  Validation, Owner, and Completion

The Delegation Contract contains only what becomes true because a particular
person is taking a particular scope:

- Scope
- Acceptance
- Decision Rights
- Escalation
- Return Expectations

If removing the assignee would not make the information irrelevant, update the
canonical Dossier rather than the contract.

## The three things this skill does

1. Health-check the canonical work as the person receiving it.
2. Persist missing work clarity into the Dossier and person-specific terms into
   a Delegation Contract.
3. Render a paste-able note from those two sources without inventing or
   duplicating criteria.

## 1. Read the bound Dossier

Use dossier_recall. Read the current revision, frontmatter, full body, artifact
index, and Open Questions.

Determine the work shape:

- One deliverable: Objective / Done When / Validation describe the work
  directly. Do not introduce a Deliverables section.
- Several contributions that combine into one outcome: use one Dossier with a
  Deliverables section. Each contribution needs its own Outcome, Owner, Done
  When, Validation, and Completion.
- Independently completable outcomes with substantially different context,
  evidence, timelines, or decisions may be separate Dossiers. Treat this as a
  judgment call, not a mechanical rule.

Check Delegation Contracts before asking anything. Settled person-specific
terms are not reopened without a concrete contradiction or material change.

## 2. Run the health checkpoint

Read the Dossier as the teammate waking up with no way to reach the sender for
hours:

> Where would they be unable to proceed, choose, or know they are finished?

Check the work before the relationship.

### Objective

Is the primary outcome explicit? It must describe the end state rather than a
task list. If only a loose idea exists, help the user state the outcome now;
that is the purpose of define.

### Done When

Are there observable conditions that close the overall Dossier? For multiple
deliverables, does each one also have local Done When conditions? Never accept
adjectives such as "good", "complete", or "reviewed" without the observable
meaning behind them.

### Validation

Is there a concrete check for the overall result and every deliverable? The
check must be symmetric: the same evidence should support completion whether
the owner self-reports or the sender reviews it. Submission is not completion;
validation is.

### Constraints

Are the boundaries of the feasible solution space retained? Look for technical,
commercial, legal, timing, budget, dependency, interface, and authority
constraints.

Distinguish:

- observed or decided constraints, which bind the work;
- assumed constraints, which need verification;
- constraints a leader could alleviate, which must name the decision-maker or
  relief path.

Never silently omit or remove a constraint to make a solution fit. If one is
alleviated or disproved, record that change in Decisions with rationale,
attribution, date, and provenance.

### Person-specific terms

Only after the work is healthy, check:

- Scope: whole Dossier or exact deliverable?
- Acceptance: have they accepted, requested clarification, or proposed a
  change? What timing or commitment did they accept, and against which Dossier
  revision?
- Decision Rights: what can they decide without waiting?
- Escalation: when do they stop rather than guess, who or where do they raise
  it, and what should they work on while waiting?
- Return Expectations: what status, validation evidence, output link, and
  decisions should come back?

Ask only about specific load-bearing gaps, usually one to three. Never walk the
user through every heading as a form and never produce a completeness score.
Name the missing fact or say the work is ready.

## 3. Persist in the correct place

Write incrementally with dossier_save and the recalled base revision. Do not
leave settled work only in conversation.

### Canonical Dossier content

When the checkpoint clarifies Objective, Done When, Validation, Constraints, or
a deliverable, update those canonical sections. Preserve relevant rationale and
citations. Shared context and constraints appear once at Dossier level; only
genuinely narrower constraints belong under a deliverable.

The Dossier may stay in define while questions remain. Move to execute only when
the person doing the work can proceed and know when they are finished. This is
a semantic checkpoint, not a parser-enforced form.

### Delegation Contract

Use the reserved heading and fixed field order:

    ## Delegation Contracts
    ### <Assignment label> — owner: <Person>[, accepted <YYYY-MM-DD>] [src:art_<id>]
    - Scope: [decided|proposed] <Entire Dossier or exact Deliverables heading.>
    - Acceptance: [decided|proposed] <Response, timing/commitment, and accepted revision.>
    - Decision Rights: [decided|proposed] <Unilateral decisions vs sign-off.>
    - Escalation: [decided|proposed] <Conditions, route, and work while waiting.>
    - Return Expectations: [decided|proposed] <Status, validation, output/evidence, decisions.>

Every field is present and tagged. A proposed field is mirrored in Open
Questions so the next session can resume without reconstructing the gap.

Acceptance is not inferred from sending the note. Before the recipient replies,
write Acceptance as proposed. Once accepted, record the date, any commitment,
and the Dossier revision they accepted. If they propose changes, update the
canonical work first, then ask them to accept the resulting revision.

Do not copy Objective, context, Done When, Validation, or Constraints into the
contract. Scope points to the canonical work.

### Existing seven-field contracts

Legacy contracts may still contain Objective / Context / Success Criteria /
Validation / Constraints / Decision Rights / Escalation. Preserve them. During
the next requested delegation pass:

- move person-agnostic work truth into the canonical Dossier sections without
  losing citations or rationale;
- keep Decision Rights and Escalation in the contract;
- replace Objective with a Scope reference;
- record explicit Acceptance against the current revision;
- add Return Expectations.

Never silently claim that a legacy contract has been migrated merely because a
reader can project some of its fields.

## 4. Render the outbound note

The note is a view, not another store. Build it from:

- canonical Objective, Done When, Validation, Constraints, and relevant context;
- the scoped deliverable, if applicable;
- the recipient's Delegation Contract;
- any relevant person-calibration note when that capability exists.

Use full sentences and enough context for an asynchronous reader. A clear note
may be longer than the stored contract; that is presentation, not duplication.
Never invent a success criterion, constraint, authority, or deadline while
rendering.

Ask the recipient to reply with one of:

- accepted;
- clarification needed, naming the blocking ambiguity;
- proposed change, naming the requested change and reason.

Also request the agreed return shape: status, each validation result, output or
evidence link, decisions made under their authority, and any escalation.

## Leaving mid-checkpoint

If the session ends before the work is ready:

1. Persist every settled clarification in its canonical Dossier section.
2. Mark unresolved contract terms proposed.
3. Add one Open Questions entry per load-bearing unresolved fact.
4. Set next_action to the next clarification and keep status at define.

Do not fill a gap by inference merely to render a complete note.

## Checking completion later

Evaluate each deliverable against its canonical Done When and Validation.
Report pass, fail, or unclear per condition, never a percentage. Mark a
deliverable done only after validation; preserve evidence. Mark the Dossier done
only when every required deliverable is done or explicitly dropped and the
overall Done When conditions validate.

When the primary outcome is complete, the Dossier is done. Capture follow-on
work as a new spark; until first-class linking exists, record the relationship
plainly rather than keeping the completed Dossier artificially open.

Before checking completion, compare the current revision and material work
definition with the accepted baseline. If Objective, Done When, Validation,
Constraints, or scoped deliverable changed, state the drift and seek renewed
acceptance before judging the recipient against the new target.

## Worked example

The Pricing page launch Dossier already contains the shared Objective, Done
When, Validation, Constraints, and three deliverables: Copy review,
Implementation, and Analytics verification.

For Priya, persist only:

    ## Delegation Contracts
    ### Pricing copy review — owner: Priya
    - Scope: [decided] Deliverables / Pricing copy review.
    - Acceptance: [proposed] Awaiting Priya's reply; proposed due 2026-09-12.
    - Decision Rights: [decided] Priya may fix factual errors and typos; claim or tone changes need sign-off.
    - Escalation: [decided] If approved sources conflict, flag in the launch Steerco and continue with the next page.
    - Return Expectations: [decided] Return status, the line-by-line validation result, output link, and decisions made.

The rendered note includes the canonical work criteria and constraints, but the
contract does not copy them. When Priya accepts, update Acceptance with the date,
commitment, and accepted Dossier revision, then move the Dossier to execute if
the remaining work is ready.
