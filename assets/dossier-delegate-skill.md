---
name: dossier-delegate
description: "Turn a Dossier into a delegation-ready handoff for a human teammate (not another agent — for agent-to-agent delegation see the `delegate` skill). Explicit-invocation only: use when the user asks to define/scope delegated work, e.g. 'help me define this exercise', 'let's clarify so we can delegate', 'write a delegation note', or runs `/dossier-delegate`. Never trigger automatically off dossier state — this skill is pull-only by design. Reads/writes the bound Dossier via the dossier MCP tools; reasons about what a teammate reading async, with no way to reach the sender for hours, would stall on."
---

# dossier-delegate — a HANDOFF.md for the human on the other end

A Dossier captures *your* state on a topic. This skill turns that state into a
**delegation contract** for a teammate picking up a piece of it — specifically
tuned for an overseas team on an offset schedule, where an unanswered
ambiguity doesn't cost a Slack reply, it costs a full day.

**Explicit invocation only.** This skill does not run because a dossier has a
`lead` set, or because `next_action` looks thin, or on any other passive
signal. It runs because the user asked — by name, by phrase ("help me define
this exercise", "let's clarify so we can delegate", "write a delegation
note"), or via `/dossier-delegate`. Do not suggest invoking it unprompted more
than once in a session; the user owns when structure gets added.

**Disambiguation:** the `delegate` skill orchestrates *other coding agents* in
herdr panes (subprocess, machine-checkable DoD, `notify`/`monitor` protocol).
This skill is for *a human teammate* who has no MCP access, works async, and
replies in prose in Slack/Jira — the mechanics differ throughout, but the
underlying discipline (unambiguous objective, symmetric success criteria,
explicit escalation) is the same lineage.

## The two things this skill does, in order

1. **Gap-check** — read the bound Dossier, find the load-bearing pieces of a
   delegation contract that are missing or ambiguous, ask only about those.
2. **Render** — once resolved, persist a compressed contract into the Dossier,
   then emit a verbose, paste-able note built from it.

Never skip straight to rendering from a thin dossier — an invented success
criterion is worse than an absent one, because the reader can't tell the
difference.

## 1. Gap-check: read as the stalled reader

Pull the Dossier's current `next_action`, `open_questions`, `lead`, `status`,
`due_date`, and body (via `dossier_recall`). Then run one framing pass, not a
checklist walk:

> **Read this as the teammate — waking up at the start of their day, with no
> way to reach the sender until theirs ends. Where do they stall?**

That framing does the prioritization for you. For an offset team, the two
categories that cause a full-day loss are disproportionately likely to be the
gap:

- **Decision Rights** — can they decide this themselves, or must they wait?
- **Escalation** — if blocked, what do they do *instead of waiting silently*?

Check those first. Then check the other five blocks (below) only if the
dossier's existing content doesn't already answer them. **Only surface a
block if you can name the specific missing fact** — never ask about a block
that's already clear from context, and never ask all seven as a form.

### The seven blocks

Adapted from Anthropic's agent-design guidance and this environment's
`delegate` skill's Spec Contract, for a *human, async, offset* delegate
rather than a supervised agent:

```
1. OBJECTIVE       — one sentence: what "done" produces. An end state, not a
                     task list.

2. CONTEXT         — self-contained. Assume zero shared memory: they don't
                     have your conversation, only what's in the Dossier and
                     the note. Pull from the Dossier's Situation/Decisions/
                     Findings rather than re-deriving.

3. SUCCESS CRITERIA — the target state in testable terms ("the deck has 3
                     pricing scenarios with margin shown"), not adjectives
                     ("make it look good"). This is the piece you'll check
                     later — get it right or the check is meaningless.

4. VALIDATION       — how you (or they) will confirm each success criterion
                     is met. Must be the SAME check whether you run it or
                     they self-report against it. If you can't write this,
                     the delegation isn't ready yet — that's a real signal,
                     not a formality to skip.

5. CONSTRAINTS      — what must NOT change, be touched, or be assumed.
                     Separate from success criteria: this is what stops a
                     reasonable-but-wrong shortcut, not what defines "done."

6. DECISION RIGHTS  — what they can decide unilaterally vs. what needs your
                     sign-off. This is the inverse of Escalation: naming it
                     explicitly is what prevents needless waiting.

7. ESCALATION       — the conditions under which they stop and flag rather
                     than guess (spec conflicts with what they find, the
                     real scope is much bigger, a decision outside their
                     rights, missing access) — AND what to do while they
                     wait for you, given the offset (park it and move to the
                     next thing; don't block the whole day on one answer).
```

An eighth, implicit block — **Return Contract** — isn't something you ask the
user to define; you write it yourself once the other seven are set (see
Persist, below).

### Asking

Ask only the questions the stall-simulation actually surfaced — usually 1–3,
rarely all seven. Keep it binary and qualitative:

- "Escalation path isn't defined — if they hit an API rate limit at 9pm your
  time, what should they do: wait, work around it, or stop entirely?"
- "Looks ready — Objective, Success Criteria, and Decision Rights are all
  clear from the dossier as-is."

**Never** produce a completeness score ("4 of 7 defined"). A score turns this
into a form to fill out, which is exactly the overhead the user is avoiding.
Name the gap or say it's ready — nothing in between.

## 2. Persist: the contract is durable, the note is not

This is the resolution to a real tension: the delegation *note* (the thing
you paste into Slack) is verbose by design — frontloading is the whole
point. The Dossier's Distilled State is terse by design — the Distillation
Guide actively fights bloat. Those two disciplines don't have to fight each
other if you separate **what's committed** from **how it's presented**:

- **The contract is committed state.** Once the seven blocks are resolved,
  write them into the Dossier body as a compressed, telegraphic section via
  `dossier_save` (or `dossier_update` for `next_action`/`open_questions`),
  e.g.:

  ```
  ## Delegated: <short task label> (<date>, owner: <lead>)
  - Objective: ...
  - Success Criteria: ...
  - Validation: ...
  - Constraints: ...
  - Decision Rights: ...
  - Escalation: ...
  ```

  This is what answers "how do I check success criteria was met without
  moving the goalpost": the contract goes through the same `Save` path as
  everything else in the Dossier, which means it's optimistic-concurrency
  protected and every edit lands in the audit log with a field-level diff.
  The goalpost can still move — but never silently. If you (or the skill, on
  a later invocation) find the current `next_action`/body has drifted from
  the persisted contract, **say so explicitly** rather than rendering the new
  state as if it were what was originally agreed.

- **The note is a rendering, not a store.** Every time this skill is invoked
  to produce the paste-able message, it expands the *currently persisted*
  contract into full sentences, greeting, sign-off — whatever the channel
  needs. It never invents a criterion at render time that isn't already in
  the persisted contract. If nothing has changed, re-rendering is just
  reformatting; it should never require re-asking the gap-check questions.

- **Return Contract**, written by you into the note (not asked of the user):
  since the teammate has no MCP access, define the exact shape of the reply
  that lets their answer get pasted back into the Dossier cleanly later —
  e.g. "reply in-thread with: (1) status — done / blocked, (2) one line per
  success criterion — met / not met / n-a, (3) link to the output, (4)
  anything you decided under your own decision rights, so it gets logged."

## Checking completion later

A second use of this skill, on an already-delegated Dossier: compare the
teammate's reply (or the Dossier's current state) against the *persisted*
Validation block, criterion by criterion. Report pass/fail/unclear per
criterion — never a percentage or score. If a criterion can't be checked
from what's available, say that plainly rather than guessing a pass.

## Worked example

**Dossier before** (thin, organic — exactly as it should be day-to-day):
```
next_action: "Get the new pricing page copy reviewed."
open_questions: []
lead: "Priya"
```

**Stall-simulation finds:** Objective and Context are fine (the body already
has the pricing decision). Success Criteria is vague ("reviewed" — reviewed
against what?). Decision Rights and Escalation are both undefined — the
highest-leverage gaps for an 8-hour offset.

**Questions asked (2, not 7):**
- "What does 'reviewed' mean concretely — sign-off from Legal, or just no
  factual errors?"
- "If Priya finds a factual error, can she fix it herself, or does it need
  your sign-off before it ships?"

**Contract persisted into the Dossier body:**
```
## Delegated: Pricing page copy review (2026-06-30, owner: Priya)
- Objective: Pricing page copy is factually correct and ready to publish.
- Success Criteria: Every dollar figure and plan name matches the approved
  pricing sheet; no claims beyond what Legal already signed off on.
- Validation: Priya diffs the copy against the pricing sheet line by line;
  same check I'd run.
- Constraints: Don't touch layout/design — copy only.
- Decision Rights: Priya can fix factual errors and typos unilaterally.
  Anything that changes claims/tone needs my sign-off.
- Escalation: If a figure in the sheet itself looks wrong, flag and move to
  the next page rather than blocking — don't wait on me to unblock.
```

**Rendered note** (what actually gets pasted to Priya): the same content
expanded into full sentences with a greeting and the return-contract shape,
built entirely from the block above — nothing new introduced at render time.
