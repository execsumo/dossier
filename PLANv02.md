# PLAN v02 — Dossier for a distributed team

> Drafted: 2026-08-05 · Status: proposal, not yet settled
> Scope: the next major pass after v1 (all milestones complete, see `HANDOFF.md`)
> Precedence note: this plan **reverses two settled decisions** (the Eisenhower
> priority model, and `urgency` as a human-set field). Those reversals need ADRs
> before implementation — see §10. Everything else extends v1 rather than
> replacing it.

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
  belong in the periodic metrics review (§11), not the daily view.
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

**Identity does not live in the store.** The obvious home is `config.yaml`, and
that is wrong: `me` is a property of *this machine and this person*, while
`~/.dossier` is the corpus. The moment the store is shared or synced (§9), a
`me:` key inside it would hand your identity to whoever pulled the repo. Put it
in `~/.config/dossier/identity.yaml`, outside `DOSSIER_HOME`, overridable by
`DOSSIER_ME` for scripted use.

This costs nothing today and is the difference between a sharable store and one
that has to be untangled later.

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

```
1. Overdue                    (own, then escalated — §6)
2. Requirement past needed_by (someone owes you and it is late)
3. Due within the horizon     (default 3 days)
4. importance: high
5. importance: medium
6. importance: low
   tiebreak within each: due-date proximity, then staleness
```

Every row carries **why it ranks where it does** — `overdue 3d`,
`due tomorrow`, `Alex owes: contacts (5d)`. A rank the user cannot explain is a
rank the user stops trusting.

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
> it behind the drills in §11 and be willing to revert to a manual `waiting` if
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

Structure it around that delta:

```markdown
## Already has          — do NOT re-explain (saves your time)
## Typically lacks      — MUST be supplied (saves their day)
## Access & contacts    — systems/people they don't have yet
## Standing decision rights — what they decide without asking, by default
## Working style        — format preferences, how they signal blocked
## Escalation default   — what they do when stuck and you're asleep
```

Alex's note then reads: *Already has: cash-flow discounting, DCF modelling.
Typically lacks: vendor contacts outside Finance — supply them explicitly.*

**Why this is the magic multiplier:** those six sections map almost 1:1 onto the
seven blocks the `dossier-delegate` skill already gap-checks
(`assets/dossier-delegate-skill.md:65-99`). Today that skill re-asks you the
same questions for every delegation. With a profile loaded, it asks only what is
**new for this person on this topic** — a two-question gap-check instead of a
five-question one, every time.

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

**The co-authoring loop works today with zero infrastructure.** The profile is a
Markdown file: send it to them. *"This is how I've been briefing you — what's
wrong, what's missing?"* A Slack round-trip costs nothing and needs no sharing
architecture. Make this an explicit, documented step of adding a person, not an
afterthought — `dossier person add` should end by offering the note for review.

If shared access lands (§9), the profile becomes **theirs to edit by
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

### 7.5 README rewrite (Phase 5)

The self-improving profile loop (§4.2) is the clearest single expression of what
v02 is *for*, and today's README has no vocabulary for it — it describes a
memory layer for topics, not a working model of a team. Phase 5 should rewrite
the README around the three moves in §0, leading with the loop: **you brief a
teammate, the agent notices what you had to explain, and the next briefing is
shorter.** That is the sentence that makes the product legible to someone who
has not read this plan.

---

## 8. Sequencing

| Phase | Contents | Why here |
|---|---|---|
| **0a** | **UTC canonicalization, alone** — plus `AuditEvent.Actor` wiring | Ships first, as its own PR. See below. |
| **0b** | `Roster` port + people/interfaces files · schema/migration scaffolding · `me` identity outside the store | Everything downstream depends on the roster |
| **1** | Importance 3-way · delete `urgency` · computed rank + reason strings | Small, self-contained, unblocks every view |
| **2** | Requirements model · routing state · derived `waiting` · CLI/MCP/TUI surfaces | Biggest single win; independent of notes |
| **3** | Identity-first views · escalation up and down · `--lead`/`--scope` | Needs 0–2 in place |
| **4** | People/interface notes · settings view · `dossier prep` · delegate-skill integration | Where the compounding value lands |
| **5** | Guide + instructions rewrite · dogfood drills · metrics | Quality comes from the guide, per v1's own lesson |

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

### Recommendation: defer (b), but pick git — not a server, and not a synced folder

When (b) comes, I would build it on **a private git repo, not hosting.**
Reasoning:

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

### Three constraints adopted now to keep (b) reachable

These cost nothing in v02 and are expensive to retrofit:

1. **Identity lives outside the store** (§1.5) — otherwise a shared store hands
   your `me` to everyone who pulls it.
2. **One file per entity** — `people/<handle>.md`, not `people.yaml`. Two people
   editing different colleagues must not conflict. Already the plan; now it is
   load-bearing for a second reason.
3. **Populate `AuditEvent.Actor`** (§4.3) — declared, never written. Without an
   actor, a shared store's audit log cannot answer "who changed this," which is
   the whole point of having one.

## 9.1 Explicitly out of scope for v02

- Shared store, sync, or hosting of any kind — per the above, deferred by
  choice, with the path named.
- Teammate-facing read access (web view, export site, share links).
- Automated chasing — Dossier does not send the Slack message. It composes it;
  you send it. Consistent with D6.
- Per-requirement reminders, recurrence, snooze.
- Calendar integration for interface cadence.

---

## 10. ADRs required before implementation

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
5. **0010 — Multi-machine posture.** Records the (a)/(b) split from §9, the
   choice of git-over-hosting when (b) arrives, and the three reachability
   constraints adopted now. Written *now*, while the reasoning is fresh —
   deferring a decision is still a decision, and this one shapes three others.

`CurrentSchemaVersion` bumps once for the whole v02 frontmatter change
(importance enum, urgency ignored, requirements array). Reuse the existing
`Frontmatter.Normalize` + `Service.Migrate` machinery
(`core/dossier.go:157`, `core/service.go:444`) — do not invent a second
migration path.

---

## 11. Dogfood drills for v02

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
