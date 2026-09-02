# PLAN v02 — Dossier for a distributed team

> Drafted: 2026-08-05 · Status: **superseded historical proposal**
> Scope: retained for design history only.
> Precedence note: this document is not an implementation contract. The current
> breaking schema is defined by `SPEC.md`: canonical `priority` only, no legacy
> frontmatter migration, and open questions in the Markdown body.

---

## 0. The thesis

v1 built an excellent **single-operator memory layer**. Its trust mechanics
(non-destruction, provenance, append-only audit, files-are-truth) are sound and
this plan does not touch them.

But v1's model of *other people* is one free-text string: `lead`. Everything a
people leader actually manages — who owes what, how long it has been sitting,
what time it is where they are, what they already know, which forum the ask
belongs in — lives in prose today, which means it is unqueryable, unsortable,
and invisible to the agent.

**v02's job is to make the team a first-class part of the data model**, so the
agent can reason about people the way it already reasons about topics.

Three structural moves carry most of the value:

| Move | From | To |
|---|---|---|
| **A. A roster exists** | `lead: "Alex"` free text | People and Interfaces as durable entities with notes |
| **B. Waiting becomes asking** | passive `status: waiting` | structured `requirements[]` — what, from whom, via which forum, by when |
| **C. The viewer has an identity** | no notion of "me" | `me` scopes every surface; escalation follows the reporting line |

Everything else in this plan is downstream of those three.

### 0.1 A standing constraint: informational, never anxious

A tool for someone running a team across timezones can very easily become a
machine for generating low-grade dread. Every deadline, every aging counter,
every escalation is an opportunity to nag. If it nags, it gets abandoned — and
an abandoned memory layer is worse than none.

So this is a hard design constraint on every surface v02 adds, not a styling
preference:

- **A boundary, not a countdown.** "Send by 16:40 for Alex to have a full day"
  is a helpful transition marker. "⚠ 3h20m REMAINING" is a stress generator
  describing the identical fact.
- **No alarm palette.** Overdue is rendered as information — a neutral marker
  and a plain reason string — not red, not bold, not iconographic urgency.
- **Descriptive language, never imperative or evaluative.** "Not yet raised,"
  not "You haven't asked!" "Past needed-by," not "LATE."
- **No guilt accumulation.** No streaks, no running totals of missed items, no
  "you have 12 overdue." Show the next thing that needs a decision. Aggregates
  belong in the periodic metrics review (§12), not the daily view.
- **Absence of pressure is not absence of signal.** The information still has to
  be *there* and sortable. Calm and useless is also a failure.

Applies to the dashboard, the session-start nudge, escalation rows, prep output,
and anything the agent says about a requirement.

---

## 1. Foundations (Phase 0)

### 1.1 Time canonicalization — UTC in storage, local at the edges

**This is partly a bug fix, not only a feature.** Timestamps are currently
written inconsistently: `store/fsstore.go:283` writes `UpdatedAt` in *local*
time, `fsstore.go:370` writes `CapturedAt` local, while
`core/service.go:1124` writes `LastTouchedAt` in UTC. Two machines, or one
machine after a DST shift, produce files whose timestamps cannot be compared.

Rules:

1. **Every instant is stored RFC3339 in UTC.** One helper, used everywhere;
   no bare `time.Now()` reaching a persisted field.
2. **Every instant is displayed in the viewer's local zone**, labelled.
3. **`due_date` stays a calendar date**, not an instant — humans commit to days.
   But it resolves against the **owning person's timezone** (§4.3). "Due Friday"
   for a dossier led by Alex in IST ends when Friday ends in Kolkata.
4. Embed the IANA database (`import _ "time/tzdata"`) so the single binary keeps
   working with no system tzdata dependency.

**Migration is revision-stable.** `core/revision.go` already canonicalizes
every timestamp through `.UTC().Format(time.RFC3339)` before hashing, so
rewriting the stored representation to UTC does **not** change any revision
hash and cannot invalidate an in-flight `base_revision`. Verify this with a
test before running the sweep.

### 1.2 The roster: people and interfaces as files

Consistent with D9 (files are truth, no database, Obsidian-readable). Two new
reserved directories beside `context/` and `sessions/`:

```
~/.dossier/
  people/<handle>.md         # frontmatter = the facts; body = the delegation profile
  interfaces/<slug>.md       # frontmatter = the cadence; body = objectives & scope
```

**Implementation note that will bite if missed:** `store/fsstore.go:97` skips
`context`, `sessions`, and dotfiles when scanning for dossiers. `people` and
`interfaces` must join that skip list, or the roster will be listed as two
malformed dossiers.

**Port:** add a `Roster` interface to `internal/core/ports.go`, implemented in
`internal/store` against the same home dir, with a fake for tests. Deliberately
*not* folded into `Store` — `Store` is already 20 methods and its fake is
load-bearing in every core test.

```go
type Roster interface {
    ListPeople() ([]Person, error)
    GetPerson(handle string) (*Person, error)
    SavePerson(p *Person) error          // archive semantics, never delete
    ListInterfaces() ([]Interface, error)
    GetInterface(slug string) (*Interface, error)
    SaveInterface(i *Interface) error
}
```

### 1.3 Person schema (item 8, plus two additions)

```yaml
handle: alex                    # stable key; referenced by lead / requirements.from
name: Alex Moreau
position: Senior Analyst
email: alex@example.com
slack: "@alex"
timezone: Asia/Kolkata          # IANA
working_hours: "09:30-18:30"    # local to their timezone
preferred_channel: slack        # where a chase actually reaches them
reports_to: ryan                # single field — this is the whole org chart
status: active | inactive       # never deleted
```

Two additions beyond the eight requested:

- **`working_hours`** — required for honest aging (§4.4) and send-by math (§7.1).
  Without it, timezone data is decorative.
- **`preferred_channel`** — knowing someone's Slack handle and email doesn't say
  which one gets answered.

Trimmed on purpose: phone, start date, skills matrix, seniority band. That is an
HRIS, and none of it changes what the agent does.

### 1.4 Interface schema (items 2 and 6)

```yaml
slug: pricing-wbr
name: Pricing WBR
cadence: weekly                 # weekly | biweekly | monthly | adhoc
day_time: "Tue 09:00"           # optional
timezone: America/New_York      # the meeting's zone, not the viewer's
participants: [ryan, alex, priya]
status: active | inactive
```

The seven hardcoded values in `core/dossier.go:96-104` are **seeded into
`interfaces/` on first run** so existing dossiers keep validating. Validation
moves from a compiled enum to a roster lookup; an unknown interface becomes a
**warning, not a hard error**, matching the backward-compatibility posture
already established by `Frontmatter.Normalize`.

### 1.5 Viewer identity (item 5)

The existing `ViewLeadSelector` landing screen (`internal/tui/tui.go:42`)
becomes the **identity** picker on first run — "who are you?" — and persists the
answer instead of asking every launch. Filtering by *other* people remains
available as a secondary lens (`f`).

#### Identity does not live in the store — worked through concretely

The obvious home for `me` is `~/.dossier/config.yaml`. Here is what happens if
we do that and the store later syncs (§10):

1. You set `me: herwin`. It lands in `config.yaml`, inside the store, and gets
   committed and pushed.
2. Ryan pulls. **His `config.yaml` now says `me: herwin`.** His dashboard shows
   your plate; escalation runs against your reporting line, not his.
3. Ryan sets `me: ryan` and pushes. Next time you pull, *your* identity flips
   to Ryan.
4. From then on, `config.yaml` conflicts on **every single sync, forever**, over
   a field that should never have been shared in the first place.

The underlying rule, which is worth stating once and applying everywhere:

> **The store holds the corpus. Anything describing *this machine* or *this
> person* lives outside it.**

Applying that rule sorts today's state cleanly:

| Lives in the store (shared) | Lives outside (machine-local) |
|---|---|
| dossiers, artifacts, audit, history | `me` — who is looking |
| `people/`, `interfaces/` | `dossier_home` — a local path |
| `schema_version` — describes the corpus | `sessions/` — session bindings are meaningless on another machine |
| `token_target` — a shared preference | `.lock` files, temp files, sync state |

So: `~/.config/dossier/identity.yaml` holds `me` (overridable by `DOSSIER_ME`),
and `sessions/` plus lock/temp files are excluded from sync. Both cost nothing
now and are painful to untangle later — `sessions/` in particular would
otherwise have two machines fighting over each other's live session bindings.

**Scope honesty:** `me` is a lens, not authentication. See §9 for what that does
and does not enable.

---

## 2. Prioritization cleanup (item 3, Phase 1)

### 2.1 What changes

- **`importance ∈ {high, medium, low}`** — three buckets, as requested.
- **`urgency` is deleted as a human-set field.** This is the real cleanup. Today
  a human maintains two axes and both go stale. Urgency is *computable* from
  `due_date`, overdue state, and requirement aging — so compute it.
- `CalculatePriorityScore` (`core/priority.go:13`) is replaced by a **ranking
  function with an explainable reason string**, not an opaque 1–4.

### 2.2 The new ordering

**Importance is the primary key; time pressure orders within it.** The one
exception is blocking someone else, which gets a tier of its own above
everything.

```
0. BLOCKING SOMEONE ELSE — anything overdue that another person is waiting on
   (a requirement where from == me, past needed_by; a delegated item whose
    lateness stalls its owner)

then, by importance:
1. high     → overdue · due within horizon · rest
2. medium   → overdue · due within horizon · rest
3. low      → overdue · due within horizon · rest

tiebreak within each cell: due-date proximity, then staleness
```

**Why tier 0 exists and nothing else does.** An earlier draft of this plan put
*everything* overdue on top, which ranks a trivial late item above a critical one
due tomorrow — not how anyone triages, and a fast way to teach yourself to
ignore the top of the list. Importance-first fixes that. But being late to
yourself and blocking a colleague are categorically different, and on an offset
team the second costs someone a full working day. That difference earns a tier;
nothing else does.

This also means the **downward escalation** in §6 falls out of the sort itself
rather than being bolted on beside it — "what I owe my team" *is* tier 0.

Every row carries **why it ranks where it does** — `overdue 3d`,
`due tomorrow`, `Alex is waiting (5d)`. A rank the user cannot explain is a rank
the user stops trusting. Per §0.1, these read as reasons, not alarms.

**Confirm against a real list in Phase 1.** The shape above is a considered
guess, not a validated one; sort your actual dossiers both ways before the
ordering is fixed in code.

### 2.3 Two consequences to accept knowingly

- **`medium` returns.** The 2026-06-25 change removed it, and
  `Importance.Normalize` currently coerces every unrecognized value *to `high`*.
  Any store that had `medium` before that change was already healed to `high`
  and is **not recoverable** — those items will need re-triage by hand. One-way,
  low-volume, acceptable.
- Dropping `urgency` means old files carry a field the schema no longer reads.
  Leave it in place on disk (harmless, non-destructive) and ignore it; do not
  strip it on migration.

---

## 3. Requirements — "what's required, and from whom" (item 4, Phase 2)

This is the highest-value single change in the plan.

### 3.1 The lean attribute set

Six human-set fields, two system-stamped:

```yaml
requirements:
  - id: req_01j8x…            # stable; referenced by audit entries and answers
    need: "Contacts for the Website Builder vendor"
    from: alex                 # roster handle, or free text for externals
    via: "1:1"                 # interface slug — the forum this gets raised in
    needed_by: 2026-08-08      # date, resolved in the requester's zone
    state: needed              # needed → requested → answered | dropped
    requested_at: …            # system-stamped on the needed → requested move
    answered_at: …             # system-stamped on answer
    answer_ref: art_01j8y…     # provenance once it lands
```

**`asked_on` is gone — you were right that maintaining a date is overhead.**
The human moves a state; Dossier stamps the time. This is the pattern the
product already uses everywhere else (`last_touched_at`, audit entries with
field-level diffs) and it produces the same information for zero effort.

The state is also a **better** carrier of the bottleneck signal than a nullable
date, because it is legible at a glance rather than requiring a null check:

- `needed` → **you** are the bottleneck. It has not been raised with anyone yet.
- `requested` → **they** are the bottleneck, and aging runs from `requested_at`.
- `answered` / `dropped` → closed, never deleted (non-destruction rule).

A leader's most common failure is the first state, and no tool surfaces it.

### 3.2 Routing state: raise, chase, suppress

Your point about `via` is the one that changes the model rather than tidying it.
`via` is not a label saying which forum a thing *belongs* to — it is a **queue
with a state**. Walking into a 1:1, you do not want everything you have ever
asked Alex. You want what you still need to **raise**, plus anything you already
raised that has since gone past its date.

That falls straight out of `state` + `needed_by`, with nothing new to store:

| Bucket | Condition | Why it is there |
|---|---|---|
| **Raise** | `state: needed`, `via` = this interface | Not yet asked — this is the agenda |
| **Chase** | `state: requested`, past `needed_by` | Asked, went quiet, now late |
| *(suppressed)* | `state: requested`, not past `needed_by` | In flight and on time — clutter |

The suppression is the valuable half. An agenda that re-lists everything
in-flight trains you to skim it, and then you miss the one item that matters.

**Re-routing rule:** changing `via` on a `requested` item resets it to `needed`
— escalating an unanswered ask from the 1:1 to Steerco means it has not been
raised *there* yet, so it belongs back on the Raise list. The prior routing is
not lost; it is in the audit log, and the Chase framing at the next 1:1 can
still reference it.

### 3.3 What I would deliberately leave out

Per-requirement priority (inherit the dossier's), a separate assignee from
`from`, reminder cadence / snooze (derive it from `needed_by`), effort
estimates, threaded comments. Each is individually reasonable and collectively
they turn Dossier into a ticket tracker — which is the thing you are using
Dossier to escape.

### 3.4 `waiting` becomes derived

A dossier with ≥1 open requirement **is** waiting. Stop asking a human to
maintain that fact.

- Human-set statuses: `active`, `blocked`, `resolved`, `archived`.
- Derived: `waiting` (computed at read time, never persisted).
- `blocked` stays manual — a blocker is not always a person.
- Non-person dependencies (a vendor, an auditor, a date) are expressed as a
  requirement with a free-text `from`, so they still derive `waiting` correctly.

> **Flag for dogfooding before this is settled:** deriving a status is the one
> genuinely clever move in this plan, and clever is where things go wrong. Ship
> it behind the drills in §12 and be willing to revert to a manual `waiting` if
> real use produces states that do not fit.

### 3.5 Surfaces

- CLI: `dossier require <slug> "<need>" --from alex --via "1:1" --by 2026-08-08`,
  `dossier require <slug> --raised req_…` (the `needed → requested` toggle),
  `dossier require <slug> --answer req_… --ref <artifact>`, `dossier requires`
  (cross-dossier view: everything owed to me, everything I owe).
- MCP: `dossier_require` (create/answer/drop), plus `requirements` on
  `dossier_update` and in `dossier_recall`/`dossier_list` payloads.
- TUI: a requirements pane in detail view; inline add/answer.

---

## 4. Notes on people and interfaces (item 2, Phase 4)

### 4.1 The interface note — objectives and scope

Free Markdown body, loaded whenever you prep for that interface. Suggested
skeleton (a template, not a schema):

```markdown
## Objective          — what this forum decides
## Scope              — what belongs here
## Out of scope       — what gets redirected, and where to
## Standing agenda    — recurring items
## Participants & roles
```

`## Out of scope` is the one most people omit and the one that earns its keep:
it lets the agent say *"this belongs in Solutioning, not Steerco"* during prep.

### 4.2 The person note — a **calibration delta**, not a bio

This is the strongest idea in your list and it deserves precision. Your Alex
example encodes the actual insight: the useful content is not *who Alex is*, it
is **the delta between what this task assumes and what Alex already has.**

Structure it around that delta — **in both directions**, because a profile is
read by whoever is briefing whom:

```markdown
# Working with <name>

## → Delegating to them
## Already has          — do NOT re-explain (saves your time)
## Typically lacks      — MUST be supplied (saves their day)
## Access & contacts    — systems/people they don't have yet
## Standing decision rights — what they decide without asking, by default
## Escalation default   — what they do when stuck and you're asleep

## ← Reporting to them
## Questions they always ask   — answer these before they are asked
## What a good update looks like — format, cadence, level of detail
## What "done" means to them   — their bar, stated plainly

## Working style        — applies both ways
```

Alex's note reads: *Already has: cash-flow discounting, DCF modelling. Typically
lacks: vendor contacts outside Finance — supply them explicitly.*

**The upward half is not filler, and for your own profile it is the whole
point.** You are in the roster (§13.5). Nobody delegates to you — but five
people report to you, and every one of them is guessing at the same three things:
what you will ask, what shape of update you want, and when you will consider it
finished. Writing that down once means they answer your standing questions
*before* you ask them, which on an eight-hour offset saves an entire round-trip
per update. The same round-trip the downward half saves in the other direction.

Framed generally: a profile is not "how to manage this person." It is **how to
exchange work with this person without a wasted round-trip** — and round-trips
are expensive in both directions.

**Why this is the magic multiplier:** the downward sections map almost 1:1 onto
the seven blocks the `dossier-delegate` skill already gap-checks
(`assets/dossier-delegate-skill.md:65-99`). Today that skill re-asks you the
same questions for every delegation. With a profile loaded, it asks only what is
**new for this person on this topic** — a two-question gap-check instead of a
five-question one, every time. The upward sections do the same job for
`dossier prep` (§7.2): preparing an update *to* someone is the same problem
viewed from the other end.

**And it should learn.** After a delegation closes, the skill proposes a profile
update: *"Alex now has the Website Builder contacts — move to 'Already has'?"*
The profile improves as a by-product of work you were doing anyway. That is a
`Save` on a Markdown file — mechanically trivial, compounding in value.

### 4.3 The profile is co-authored, and eventually self-authored

The guardrail and the product design turn out to be the same thing: **the more
the subject owns the note, the more useful and the less fraught it is.**

Rules, in order of strength:

1. **Factual and task-scoped only.** What they know, what they need, what access
   they have, how they prefer to receive work. Explicitly **not** performance,
   personality, pace, or evaluative judgement of any kind. The template enforces
   the shape; the guide (§7.3) enforces the tone, because the agent is what will
   actually be writing these.
2. **Written as though they will read it** — because they should.
3. **Co-authored now, self-authored later.** Alex knows what Alex does not know
   far better than you do. Every step toward their authorship makes the note
   more accurate *and* removes the liability of holding an unreviewed file about
   a colleague.

This rule needs to be stated where people actually encounter it, not buried in a
plan: the README carries it in Phase 5 (§7.5), the Distillation Guide carries it
for the agent (§7.3), and the person-note template carries it inline.

**The co-authoring loop works today with zero infrastructure.** The profile is a
Markdown file: send it to them. *"This is how I've been briefing you — what's
wrong, what's missing?"* A Slack round-trip costs nothing and needs no sharing
architecture. Make this an explicit, documented step of adding a person, not an
afterthought — `dossier person add` should end by offering the note for review.

If shared access lands (§10), the profile becomes **theirs to edit by
convention**, with the audit log recording who changed what. That requires
audit entries to record an actor, which brings us to a cheap thing to do now:
`AuditEvent.Actor` already exists in `core/audit.go:10` and is populated **in
exactly zero places**. Wire it to `me`. It is meaningless-but-harmless in
single-user, and essential — and expensive to backfill — the moment two people
touch one store.

### 4.4 Reporting structure (item 2)

One field — `reports_to` — gives the whole tree. Derive `direct_reports`, detect
cycles at load, render read-only in settings. With a team of five the depth is
two; do not build a graph editor for it.

Aging is measured in **their working days**, not wall-clock: "asked 4 days ago"
is wrong when two of them were their weekend. This needs `timezone` +
`working_hours`, which is why §1.3 makes both required.

---

## 5. Settings in the TUI (item 2 + 6) — and what I would trim

**Trim recommendation, stated plainly:** do not build full CRUD forms for people
and interfaces in Bubble Tea. `internal/tui/tui.go` is already 2,247 lines, and
this is data you touch perhaps five times a year. That is a lot of view code for
very little use.

Instead, lean on D9 — *files are truth, and therefore files are the editor*:

- **Settings view lists** people and interfaces, shows the note rendered, shows
  the reporting tree.
- **Inline-edit only the high-churn fields** — timezone, `reports_to`, active
  status, interface participants. Reuse the existing inline-editor pattern
  (`ViewLeadEditor`, `ViewStatusPicker`).
- **Everything else routes out**: `e` opens `$EDITOR` on the Markdown file, and
  — more interesting — the notes are primarily meant to be written *by the
  agent*, in conversation: *"add to Alex's profile that he now has the vendor
  contacts."* That is the natural authoring path for a non-dev leader, and it
  needs no TUI code at all.

Roughly 60% less TUI work for about 95% of the value.

---

## 6. Escalation (item 7, Phase 3)

Escalation is a function of `me` + `reports_to` + time.

```
My view =
    my own items
  + items where a direct report is overdue or at risk      (rolls up to me)
  + requirements where from == me, aging since requested_at (I am the blocker)
```

- **`escalation_depth`** (default 1 = direct reports) keeps a deep org from
  flooding a leader.
- **Escalate the requirement, not just the dossier.** More precise: Alex's
  dossier may be healthy while a single thing he owes has gone past
  `needed_by`. That requirement is the item that needs your attention, not the
  whole topic.
- **Noise guard:** escalate on *overdue*, or *due-within-horizon AND untouched
  for N days*. A leader who sees all five reports' full plates stops looking at
  the screen. Escalation earns its place only by being rare.
- Escalated rows are visually distinct and always state the reason and the owner
  (`↑ Alex · overdue 3d`).

**The addition I would make here:** escalation should also run **downward**. If
`from: me` on someone else's requirement and it is aging, that belongs at the
very top of my list — I am the bottleneck for someone whose day starts in four
hours. Leaders systematically under-track what they owe their own team, and this
costs a full day every time. Cheap to build, disproportionately valuable.

---

## 7. What else I would add

### 7.1 "Their day starts in…" — the timezone payoff

Derived from `timezone` + `working_hours`, surfaced in the dashboard header and
in the session-start nudge:

> *Alex's day starts in 3h20m — 2 open requests routed to him.*
> *Send by 16:40 your time for Priya to have a full working day on this.*

The **last-responsible-moment** calculation — given a `needed_by`, the person's
zone, and their working hours, when must this leave your hands — is the single
most useful thing timezone data can produce, and it is perhaps fifty lines of
arithmetic. This is the item I would build first among the additions.

**Per §0.1, this is a horizon marker, not a deadline.** It states a fact about
someone else's day so you can choose; it never counts down, turns red, or tells
you that you have missed it. Passing 16:40 means Priya starts tomorrow — which
is usually fine. Rendering that as a failure would be both inaccurate and the
exact anxiety this tool should not manufacture.

### 7.2 `dossier prep <interface> [--with <handle>]` — composition, not filtering

Today the TUI can *filter* by lead and interface. Prep should **compose**.

**`--with` is optional, because most interfaces are group forums.** Steerco, the
standups, and OpsRev have no single counterpart; 1:1 does. The flag narrows, it
is not required:

```
dossier prep "1:1" --with alex     # one counterpart
dossier prep "steerco"             # group forum — grouped by person
```

Both assemble: the interface note's Objective / Scope / Out-of-scope · dossiers
tagged with that interface · the **Raise** and **Chase** buckets from §3.2 ·
what is overdue · **what I owe them** · relevant profile notes for anything
needing pre-context.

The difference is only the grouping. With `--with`, output is one narrative for
one person. Without it, output is **grouped by person**, drawing the attendee
list from the interface's `participants` field, so you can see whose items are
whose as you go round the table. An `--unassigned` group catches topics tagged
to the forum with no lead.

This is the command that makes items 2, 4, 5, and 6 pay off together, and it is
the thing you actually do every week. Available on CLI, MCP, and TUI.

### 7.3 Distillation guide addendum

Every new structure needs a guide entry or agents will not populate it — the
guide is the product's steering wheel. `assets/guide.md` and
`assets/instructions.md` need: when to open a requirement instead of writing
prose, how to consult a person's profile before delegating, how to propose a
profile update after one, and the calibration-not-evaluation constraint from
§4.3.

Also needs a **tone clause** carrying §0.1 into agent output, since the agent
narrates most of what the user actually reads.

### 7.4 The `--lead` gap, closed explicitly

`dossier ls` has no `--lead` (`cli.go:305-307`) and `dossier_list` exposes only
`status` and `interfaces` (`mcp/tools.go:37-49`) — so from inside Claude Code,
the main surface, you cannot ask for one person's plate. Item 5 implies fixing
this; listing it explicitly so it does not fall through. Add `lead` and `scope`
(`me` | `reports` | `all`) to both.

### 7.5 README — updated as the last step of every phase

**Not a Phase 5 rewrite.** The README is updated at the end of each phase to
cover what that phase actually shipped, so it never describes capability that
does not exist yet. This is a standing addition to the definition of done (§8),
not a separate work item.

Two rules make that work:

- **Describe only what has merged.** No forward-looking feature prose. If a
  phase ships requirements but not escalation, the README gains requirements
  and stays silent on escalation.
- **A rule may precede its capability; a feature may not.** Guidance about *how
  to write something* can land with the thing being written, even when the
  reason it matters arrives later. That distinction is what resolves the
  co-authoring case below.

By the end of Phase 4, the README should have arrived — incrementally — at
leading with the self-improving profile loop (§4.2), which is the clearest
single expression of what v02 is *for*: **you brief a teammate, the agent
notices what you had to explain, and the next briefing is shorter.** Today's
README has no vocabulary for that; it describes a memory layer for topics, not
a working model of a team.

#### The co-authoring rule, split across two phases

The rule itself lands in **Phase 4**, with the person notes it governs, and it
stands on its own merits without reference to sync:

> A person note is a **task-calibration note**: what this colleague already
> knows, what they need supplied, what access they lack, how they prefer to
> receive work. It is not a performance file, and it is not a personality
> sketch. Write it as though the person will read it — and share it with them,
> because they know what they don't know better than you do.

The **reason it was load-bearing** lands in **Phase 6**, with the capability
that makes it so:

> This was not only good manners. Now that the store can be shared, **Alex can
> read Priya's profile.** A note that is factual, task-scoped, and co-authored
> from day one needs no cleanup; one written as private commentary becomes a
> liability the moment the repo has a second reader. The discipline is what
> makes the store safe to share.

This split is the general pattern, not a special case: **write the rule when the
thing it governs ships; write the justification when the capability that makes
it matter ships.** The Phase 4 reader gets a rule they can follow. The Phase 6
reader learns why they are glad they did. Neither reads about software that does
not exist.

#### The profile is bidirectional (§4.2, §13.5) — another README-worthy sentence

Once Phase 4 ships, the README should carry this alongside the co-authoring
rule, because it reframes what a person note *is* rather than adding a feature:

> A profile isn't "how to manage this person" — it's **how to exchange work
> with them without a wasted round-trip**, in whichever direction the work is
> moving. Delegating to Alex, it says what he already knows and what to hand
> him explicitly. Reporting to Ryan, it says what he'll ask before he asks it,
> what a good update looks like, and what "done" means to him — so his standing
> questions get answered before he raises them, not after.

It belongs next to the self-improving-loop sentence from §0.1 of this section:
both are single sentences that make the product legible to someone who has not
read this plan, and both describe the same underlying idea — the tool exists to
remove wasted round-trips between people — from two different angles.

---

## 8. Sequencing

| Phase | Contents | Why here |
|---|---|---|
| **0a** | **UTC canonicalization, alone** — plus `AuditEvent.Actor` wiring | Ships first, as its own PR. See below. |
| **0b** | `Roster` port + people/interfaces files · schema/migration scaffolding · `me` identity outside the store | Everything downstream depends on the roster |
| **1** | Importance 3-way · delete `urgency` · computed rank + reason strings | Small, self-contained, unblocks every view |
| **2** | Requirements model · routing state · derived `waiting` · CLI/MCP/TUI surfaces | Biggest single win; independent of notes |
| **3** | Identity-first views · escalation up and down · `--lead`/`--scope` | Needs 0–2 in place. **Gated on §13.1 sign-off (ADR 0011).** |
| **4** | People/interface notes · settings view · `dossier prep` · delegate-skill integration | Where the compounding value lands |
| **5** | Guide + instructions rewrite · dogfood drills · metrics | Quality comes from the guide, per v1's own lesson |
| **6** | Shared store over git — merge driver · item-wise requirement merge · section-wise body merge · non-dev conflict resolution · `dossier sync` (§10) | Distribution on top of a validated model, not instead of one |

**Why 0a ships alone.** The timestamp inconsistency (`fsstore.go:283` and `:370`
local, `service.go:1124` UTC) is a **live correctness bug**, not a v02 feature:
any two machines, or one machine across a DST boundary, already produce
timestamps that cannot be compared, which silently corrupts staleness and
due-date ordering today. It is independently valuable, revision-stable (§1.1),
touches no schema, and every later phase computes on those timestamps. It should
merge before the rest of v02 is even reviewed.

Each phase ships to the repo's existing definition of done: compiles, `go vet` +
`gofmt` clean, tests pass, SPEC §14 criteria updated and demonstrably met,
`ARCHITECTURE.md` updated if structure changed, `HANDOFF.md` status refreshed.

**Plus one addition to the definition of done, for every phase: update the
README as the final step**, covering what that phase shipped and nothing more
(§7.5). The README then tracks the binary instead of running ahead of it, and
there is no end-of-project documentation debt to pay down.

---

## 9. The multi-machine question — recommendation

You want Ryan working from his machine while you work from yours, are open to
hosting later, and said colleague access is not strictly critical. Those three
things resolve cleanly, because the request contains two very different asks
that should be separated:

**(a) Ryan runs Dossier on his own machine, his own store.** Works the day v02
ships, with zero additional work — that is precisely what `me` as a lens buys.
Each person gets their own durable memory layer, their own dossiers, their own
requirements. You and Ryan exchange *rendered output* (delegation notes, prep
summaries) the way you do today. **This is the v02 answer, and I think it is the
right one to validate against first** — it tests whether the model is correct
before adding any distribution problem on top of it.

**(b) You and Ryan against one shared store.** This is the genuinely different
architecture, and its prize is real: a requirement you file as `from: ryan`
appears on Ryan's own dashboard as something he owes, without anyone pasting
anything. That is the thing worth eventually building toward.

**Decision (locked): (b) is the destination.** It becomes **Phase 6**, specified
in §10. (a) is not a fallback or a lesser goal — it is Phase 3's natural state
and the thing that validates the model before distribution is added on top.

### Why git — not a server, and not a synced folder

Phase 6 builds on **a private git repo, not hosting.** Reasoning:

- Files are already the source of truth, history is already a product value, and
  conflict artifacts already exist as a first-class concept. Git is not a
  workaround here; it is the same model with a remote.
- **Sync is explicit, which dissolves the locking problem.** The reason I flagged
  synced folders as risky is that `flock` (`store/lock.go`) is unreliable over
  Dropbox/iCloud, and continuous background sync can write under a live process.
  Git has neither property: each person has a local clone where local `flock`
  works normally, and cross-person divergence surfaces at merge time — exactly
  the case v1's conflict machinery was built for. `audit.log` needs
  `merge=union` in `.gitattributes` and then even it merges cleanly.
- No server, no auth system, no hosting cost, no multi-tenancy — none of which
  you want to own before the model is validated.
- The non-dev friction is real but bounded: wrap it as `dossier sync`
  (fetch, merge, surface conflicts as conflict artifacts, push). One command.

A hosted MCP server remains possible later, but it should be a deployment
choice for a model that already works, not the thing that makes it work.

### Three constraints adopted in Phases 0–4 so Phase 6 is cheap

These cost nothing early and are expensive to retrofit:

1. **Identity and machine state live outside the store** (§1.5).
2. **One file per entity** — `people/<handle>.md`, not `people.yaml`. Two people
   editing different colleagues must not conflict. Already the plan; now it is
   load-bearing for a second reason.
3. **Populate `AuditEvent.Actor`** (§4.3) — declared, never written. Without an
   actor, a shared store's audit log cannot answer "who changed this," which is
   the whole point of having one.

## 9.1 Explicitly out of scope for v02

- **Hosting.** A server or hosted MCP remains a later deployment choice for a
  model that already works, not the thing that makes it work.
- Teammate-facing read access for people *not* running Dossier (web view,
  export site, share links).
- Automated chasing — Dossier does not send the Slack message. It composes it;
  you send it. Consistent with D6.
- Per-requirement reminders, recurrence, snooze.
- Calendar integration for interface cadence.

---

## 10. Phase 6 — shared store over git

The seamless handoff: a requirement you file as `from: ryan` shows up on Ryan's
dashboard as something he owes, and his answer shows up on yours. No pasting.

### 10.1 Mechanism

`DOSSIER_HOME` becomes a git working tree with a private remote. Dossier shells
out to the `git` binary — **it does not vendor a Go git library.** Reasons: the
merge behavior below depends on git's own merge-driver machinery; a normal repo
can be rescued with normal git commands when something exotic happens; and it
keeps a large dependency out of a binary whose whole pitch is that it has none.
`doctor` gains a `git present` check.

### 10.2 What syncs and what does not

Per the rule in §1.5. The store's `.gitignore`:

```
sessions/          # session bindings are machine-local
**/.lock           # advisory locks
**/*.tmp*          # atomic-write temp files
*.bak              # harness config backups
```

Everything else — dossiers, artifacts, history, audit logs, `people/`,
`interfaces/` — is corpus and syncs.

### 10.3 Merge behavior: never conflict markers in a parsed file

This is the core correctness requirement. If git writes `<<<<<<<` markers into
`dossier.md`, the YAML frontmatter stops parsing and the store is broken for a
non-dev user with no way back. So git is configured never to do it:

```gitattributes
dossier.md          merge=dossier
people/*.md         merge=dossier
interfaces/*.md     merge=dossier
audit.log           merge=union
history/**          -merge          # revision-hash filenames; never collide
artifacts/**        -merge          # ULID filenames; never collide
```

`merge=dossier` is a **custom merge driver registered locally**, implemented as
a hidden subcommand of the binary (`dossier git-merge-driver %O %A %B %P`). Its
contract:

1. Attempt the structured merge Dossier already knows how to do — non-overlapping
   frontmatter auto-merges (v1, Milestone 7), and this extends to
   non-overlapping **body sections** and to `requirements[]` (see 10.4).
2. Anything genuinely contradictory: **keep ours, write theirs to
   `conflicts/`**, append an audit entry, exit 0. The working tree stays valid
   and parseable; the conflict surfaces through the machinery that already
   exists for it in the CLI, MCP, and TUI.
3. Never exit non-zero into a half-merged tree.

`audit.log merge=union` is correct because it is append-only; interleaving two
machines' entries loses nothing, and entries carry `Actor` (§4.3) and
timestamps (now reliably UTC, §1.1) to be re-sorted on read.

### 10.4 The risk that decides whether this works

**Conflicts stop being exceptional and become routine, and today they are
resolved with a dev-grade interaction.**

v1's optimistic concurrency captures `base_revision` at recall and compares on
save — within one machine, the window is seconds. Across machines with periodic
sync, that window is *however long since the last pull*, so the conflict rate
rises by orders of magnitude. And today's resolution path is a syntax-highlighted
side-by-side merge in the TUI, which is a reasonable ask of a developer and an
unreasonable one of a business leader mid-meeting.

Two people using this for a week will produce a conflict backlog they cannot
discharge, and they will stop trusting the store. **So Phase 6 is not "add git";
it is "make concurrent edits mostly not conflict."** Three things must land with
it:

- **`requirements[]` merges item-wise, not as a blob.** Each has a stable `id`
  (§3.1). "Alex answered req_A while I added req_B" is the single most common
  concurrent edit under sharing, and it must be a clean auto-merge, not a
  conflict. Independent items, independent merge — same-`id` divergence is the
  only real conflict.
- **Body sections merge independently.** Situation / Decisions / Findings /
  Active Monitors / Current State / Next Steps are a fixed schema
  (`assets/guide.md` §3). Two people editing different sections is not a
  conflict. Only same-section divergence is.
- **Conflict resolution gets a non-dev path.** At minimum: "keep mine / keep
  theirs / keep both" on a per-section, per-requirement basis, in plain language,
  with the full side-by-side view still available underneath for when it matters.

If those three do not land, do not ship the sync.

### 10.5 When sync runs

Never blocking, never surprising:

- **Pull on read boundaries** — session-start hook, TUI launch, and the first
  `dossier_list`/`dossier_recall` of a session. Debounced (default: at most once
  per 60s), with the debounce timestamp in machine-local state, not the store.
- **Push after write boundaries** — coalesced after `Save`/`Promote`/requirement
  changes and on the session-end hook, so a burst of edits is one push.
- **Async and non-fatal.** Offline, or a remote that rejects, degrades to local
  operation with a visible "last synced 14m ago" indicator — consistent with the
  existing hard rule that missing capability is surfaced, never silent.
- **Store-wide read/write lock.** A merge rewrites many files at once, so it
  takes an exclusive lock on `~/.dossier/.sync.lock` while ordinary writes take
  a shared one. `gofrs/flock` (already vendored, v0.13.0) provides `RLock`, so
  this needs no new dependency. **The sync engine goes through the store's locks
  — it never touches the filesystem behind them.**

### 10.6 Slug collisions

Two people creating "pricing-migration" independently produce the same directory
path with two different frontmatter `id`s. The merge driver detects add/add with
divergent `id`, renames one to `<slug>-2`, and surfaces it as an ambiguity —
which routes into the existing `dossier merge` flow rather than inventing a
second reconciliation path.

### 10.7 Recovery is a product requirement

Sync must never leave the repo in a state that requires git knowledge to escape.

- Fast-forward or merge commit only. **Never rebase**, never rewrite history,
  never force-push.
- On any unexpected repo state (detached HEAD, merge in progress, diverged with
  no common ancestor), **stop, touch nothing, and report in plain language**
  through `dossier doctor` and `dossier sync --status`.
- `dossier sync --status` shows: last pull, last push, unpushed changes,
  unresolved conflicts. One command answers "am I current?"

### 10.8 Two non-technical prerequisites

Worth naming because neither is solved by code:

- **Everyone in the loop runs Dossier.** Sharing only pays off if teammates
  actually adopt it. **Validate with one teammate before building for five** —
  if Ryan does not stick with it for a month, Phase 6 has no users, and (a) was
  the right answer all along.
- **A shared repo means Alex can read Priya's profile.** That turns §4.3 from
  good practice into a load-bearing constraint: the factual, task-scoped,
  co-authored discipline is *what makes the store safe to share*. Person notes
  should be written from day one as though every colleague will read them —
  because in Phase 6 they can. The repo must be private, and `dossier sync init`
  should say all of this out loud before the first push.

---

## 11. ADRs required before implementation

Per `CLAUDE.md`, settled decisions are not relitigated silently. Each of these
needs a short ADR in `docs/adr/` recording what changed and why:

1. **0006 — Replace the Eisenhower matrix with importance + computed time
   pressure.** Reverses the 2026-06-25 simplification. Must state the one-way
   `medium` migration loss (§2.3).
2. **0007 — People and Interfaces as first-class roster entities.** Introduces
   the `Roster` port and two reserved directories; confirms D9 (files are truth)
   still holds for non-dossier entities.
3. **0008 — `waiting` becomes derived from open requirements.** Includes the
   revert criteria from §3.4.
4. **0009 — Viewer identity (`me`) and reporting-line escalation.** Must state
   that this is not multi-user, and record why identity is stored outside
   `DOSSIER_HOME` (§1.5).
5. **0010 — Shared store over git (Phase 6).** Records the (a)/(b) split, the
   decision that (b) is the destination, git-over-hosting and the rejection of
   synced folders, the merge-driver contract (§10.3), and the three constraints
   adopted in earlier phases to make it cheap. Written *now*, while the reasoning
   is fresh — this decision shapes three earlier ones.
6. **0011 — Private vs. shared topics.** Written to close the §13.1 gate. Records
   the option chosen, the four sign-off items, and the rejection of a
   `visibility:` field inside the shared store. **Phase 3 does not start until
   this ADR is merged.**

`CurrentSchemaVersion` bumps once for the whole v02 frontmatter change
(importance enum, urgency ignored, requirements array). Reuse the existing
`Frontmatter.Normalize` + `Service.Migrate` machinery
(`core/dossier.go:157`, `core/service.go:444`) — do not invent a second
migration path.

---

## 12. Dogfood drills for v02

v1's lesson was that quality came from the guide and real use, not more
structure. The same applies here:

1. **Delegation with a cold profile vs. a warm one** — measure the drop in
   gap-check questions asked.
2. **Profile co-authoring** — send two profiles to their subjects for correction.
   How much was wrong? Did they want to keep editing it? That answer decides how
   hard to push toward (b) in §9.
3. **Group prep vs. 1:1 prep** — walk into a real Steerco and a real 1:1 with
   `dossier prep` output only. What was missing? What was noise?
4. **Requirement round-trip** — open a requirement, raise it in the named forum,
   land the answer with provenance, verify the derived `waiting` cleared and the
   item left the Raise bucket.
5. **Suppression check** — the sharpest test of §3.2: at a 1:1, was anything
   suppressed that you actually needed to discuss? One miss means the Chase
   threshold is wrong; zero misses over a month means suppression is working.
6. **Escalation noise test** — run for two weeks with five reports. If the
   escalation section is being ignored by week two, the thresholds are wrong.
7. **The `needed`-state audit** — how many open requirements were never actually
   raised with anyone? This number is the plan's own success metric.
8. **Timezone honesty** — verify aging in working days matches your intuition
   about "how long has Alex had this."
9. **Cortisol check (§0.1)** — after two weeks, does opening the dashboard feel
   like orientation or like a list of ways you are behind? If the latter, the
   tone rules were not followed, and that is a defect, not a preference.

Before Phase 6 specifically:

10. **One-teammate adoption trial** — Ryan runs his own store (mode (a)) for a
    month. If it does not stick, Phase 6 has no users. This gates the phase.
11. **Conflict-rate measurement** — with two stores and manual exchange,
    estimate how often you and Ryan would have touched the same dossier in the
    same window. That number sizes the risk in §10.4 before any code is written.
12. **Merge-driver adversarial fixtures** — simultaneous edits to: different
    requirements, the same requirement, different body sections, the same
    section, the same person note, and two same-slug creations. Every case must
    end with a parseable `dossier.md` and, where genuinely contradictory, a
    conflict artifact. Golden-file tested, per the repo's existing bar.

---

## 13. Open questions and their status

Each was cheap to decide now and expensive to discover late. Three are resolved
below; one is deferred to Phase 2 by decision; one is a **hard gate**.

| # | Question | Status |
|---|---|---|
| 13.1 | Private topics in a shared store | **GATE — sign-off required before Phase 3 dev starts** |
| 13.2 | Sort ordering | Resolved — §2.2 rewritten |
| 13.3 | Delegations vs. requirements | Resolved — a delegation creates a requirement |
| 13.4 | Where requirements live | Open by decision — settle in Phase 2 |
| 13.5 | Leader in their own roster | Resolved — yes, and the profile is bidirectional |

### 13.1 Private topics in a shared store — **GATE: sign-off required before Phase 3 development begins**

> **This is a blocking gate, not a note.** No Phase 3 code is written until an
> option below is chosen and recorded in ADR 0011. Phase 3 introduces `me`,
> scoped views, and escalation — all of which acquire a second meaning under a
> two-store model, and all of which are painful to re-cut afterwards. Phases 0–2
> may proceed in parallel; they do not depend on the answer.

**The gap:** there is no privacy model anywhere in this plan. In single-user
mode that is correct. The moment the store is shared (§10), *everything* in it
is readable by every teammate — and a people leader unavoidably has topics that
cannot be: compensation, performance concerns, a reorg, a hiring decision, an
exit. That is not an edge case, it is a normal week, and it is the single
likeliest reason Phase 6 gets abandoned after being built.

Three options:

- **(a) `visibility: private` frontmatter + exclusion from sync.** Rejected on
  sight: one field, one bug, one mistaken commit, and confidential material is
  in a repo five people have cloned. Never build a privacy boundary out of a
  field inside the thing being shared.
- **(b) Two stores** — a personal `~/.dossier` and a team store, with the binary
  reading both and every write targeting one explicitly. Matches how people
  already think ("is this a me thing or a team thing?"), and the boundary is a
  directory, which is hard to get wrong.
- **(c) Team store only, private topics stay out of Dossier entirely.** Honest
  and free, but it means the tool cannot hold a meaningful slice of a leader's
  actual work — which undercuts the whole premise.

**Lean: (b).** It is more work than it looks — `me`, roster, prep, and
escalation all have to span two stores coherently, and "which store am I writing
to?" becomes a question every write path must answer unambiguously, including
for the agent over MCP. That cost is exactly why it is a gate.

**What sign-off requires** (all four, recorded in ADR 0011):

1. The option chosen, with its rejected alternatives and why.
2. If (b): where the roster lives when there are two stores — shared, duplicated,
   or personal-overrides-team — and what `prep`/escalation do across the boundary.
3. The default target for a new dossier, and how a write states its target
   unambiguously on CLI, MCP, and TUI. A wrong default here is a disclosure bug.
4. How a topic moves between stores after the fact, since the first draft of a
   sensitive topic is often written before anyone realises it is sensitive.

### 13.2 Sort ordering — **RESOLVED, folded into §2.2**

As written, §2.2 puts *everything* overdue in tier 1. That means a trivial
overdue item outranks a critical one due tomorrow, which is not how anyone
actually triages, and it will train you to distrust the top of the list.

The likely fix is that **importance is the primary key and time pressure the
secondary** — high (overdue → due soon → rest), then medium, then low — with one
exception that earns its own tier:

> **Anything overdue that another person is waiting on jumps to the top**,
> regardless of importance. Blocking someone else's day is categorically
> different from being late to yourself, and on an offset team it is the
> expensive kind of late.

That reconciles both instincts: importance orders *your* work, and blocking
*someone else* is its own class. It also makes the downward escalation in §6 fall
out of the sort rather than being bolted on beside it.

**Accepted.** §2.2 now specifies this ordering. Still worth a quick A/B against
a real dossier list in Phase 1 before it is fixed in code — the shape is a
considered judgement, not a measured one.

### 13.3 Delegations and requirements overlap — **RESOLVED**

Two structures will exist for "Alex owes me something":

- the `dossier-delegate` skill's persisted contract (Objective / Success
  Criteria / Validation / … in the body), and
- `requirements[]` (§3.1).

The intended line is that a **requirement** is a discrete input you need (a
contact, a number, an approval) while a **delegation** is a piece of work with
success criteria. In practice nobody will sort them that way reliably, and if
they diverge, "what does Alex owe me?" returns half the truth — which is exactly
the question the whole plan exists to answer.

**Accepted resolution: a delegation *creates* a requirement.** Delegating work
opens a requirement (`from: alex`, `need: "<the objective, one line>"`,
`via`, `needed_by`) whose detail field points at the persisted contract. One
list, two levels of zoom: the requirement is the tracking unit, the contract is
what you open when you need the specifics. Requirements stay the single answer
to "who owes what," and the contract stays the single answer to "what exactly
did we agree."

Build order: `requirements[]` must carry a pointer to the contract from Phase 2
(one optional field), so Phase 4's delegate-skill integration is a wiring job
rather than a schema change.

### 13.4 Where requirements live — **OPEN by decision; settle in Phase 2**

Specced as an array in dossier frontmatter. The alternative is
`requirements/<id>.md`, one file per requirement, the same argument that made
`people/<handle>.md` right.

| | Frontmatter array | One file per requirement |
|---|---|---|
| D9 "one file is the truth" | preserved | fragmented |
| Obsidian-readable in place | yes | no |
| Cross-dossier query | scan (same as `List` today) | scan (same) |
| Item-wise merge in Phase 6 | needs driver logic | free |

**Lean: frontmatter.** D9 and human-readability are load-bearing product
properties, cross-dossier querying is a scan either way, and v1 already
auto-merges non-overlapping frontmatter, so the driver work is an extension
rather than a new mechanism.

**What would flip it:** if requirements turn out to be high-churn — several
edits a day across multiple people — one-file-per-item wins on merge behavior
alone. Drill 11 should capture enough signal to tell.

### 13.5 Is the leader in their own roster? — **RESOLVED: yes, and it made the profile bidirectional**

Yes. Escalation needs a `reports_to` for you, `prep` needs your working hours,
and under Phase 6 your profile is arguably the most useful one in the store.

The resolution also surfaced something the original design had backwards. A
profile is not only read by the person delegating *down* — it is read by five
people reporting *up*, and what they need from it is different: what you will
ask, what shape of update you want, what you consider finished. Answering those
once means they arrive in the update instead of costing a round-trip after it.

So the person note gained an upward half (§4.2), and the underlying frame got
more accurate: a profile is **how to exchange work with this person without a
wasted round-trip**, in whichever direction the work is moving.

The residual awkwardness — writing a profile about yourself in Phase 4, before
anyone else can read it — resolves itself: the upward half is the part your team
will read first in Phase 6, so it is worth writing before they can.
